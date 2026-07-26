package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s SubmitPipelineStep) resolvedAction() string {
	if a := strings.ToLower(strings.TrimSpace(s.Action)); a != "" {
		return a
	}
	if s.Extract != nil {
		return "extract"
	}
	if s.ReturnMsg != "" || s.ReturnYID != "" || s.ReturnCode != 0 {
		return "return"
	}
	if strings.TrimSpace(s.URL) != "" {
		return "http"
	}
	if len(s.Set) > 0 {
		return "set"
	}
	if s.DelayMS > 0 {
		return "delay"
	}
	if s.Poll != nil && strings.TrimSpace(s.URL) != "" {
		return "poll"
	}
	return ""
}

func (s SubmitPipelineStep) toHTTPRule(base SubmitRuleConfig) SubmitRuleConfig {
	out := SubmitRuleConfig{
		Method:            s.Method,
		URL:               s.URL,
		ContentType:       s.ContentType,
		UseCookie:         s.UseCookie,
		Headers:           s.Headers,
		Body:              s.Body,
		BodyMode:          s.BodyMode,
		BodyRaw:           s.BodyRaw,
		URLPortPool:       base.URLPortPool,
		KcidJSONPatches:   s.KcidJSONPatches,
		KcidJSONValidate:  base.KcidJSONValidate,
	}
	if out.Method == "" {
		out.Method = "POST"
	}
	if len(out.KcidJSONPatches) == 0 {
		out.KcidJSONPatches = base.KcidJSONPatches
	}
	if out.ContentType == "" {
		out.ContentType = base.ContentType
	}
	if out.ContentType == "" {
		out.ContentType = "form"
	}
	return out
}

func executePipelineSubmitRule(ctx context.Context, order *Order, hy *Huoyuan, httpClient *http.Client, rule SubmitRuleConfig) (*AddOrderResult, error) {
	if len(rule.Pipeline) == 0 {
		return &AddOrderResult{Code: -1, Msg: "pipeline 步骤为空"}, nil
	}
	if rule.DelayMS > 0 {
		select {
		case <-ctx.Done():
			return &AddOrderResult{Code: -1, Msg: "已取消"}, nil
		case <-time.After(time.Duration(rule.DelayMS) * time.Millisecond):
		}
	}
	return runPipelineSteps(ctx, order, hy, httpClient, rule, rule.Pipeline, rule.Response)
}

func runPipelineSteps(ctx context.Context, order *Order, hy *Huoyuan, httpClient *http.Client, base SubmitRuleConfig, steps []SubmitPipelineStep, defaultResp SubmitRuleResp) (*AddOrderResult, error) {
	client := pluginHTTPClient(httpClient)
	tctx := newSubmitTemplateCtx(base)
	var lastBody string

	for i := range steps {
		step := steps[i]
		if step.When != nil && !matchSubmitWhen(step.When, order) {
			continue
		}
		action := step.resolvedAction()
		switch action {
		case "delay":
			ms := step.DelayMS
			if ms <= 0 {
				continue
			}
			if ms > 30000 {
				ms = 30000
			}
			select {
			case <-ctx.Done():
				return &AddOrderResult{Code: -1, Msg: "已取消"}, nil
			case <-time.After(time.Duration(ms) * time.Millisecond):
			}
		case "set":
			for k, tpl := range step.Set {
				val, err := renderSubmitTemplate(tpl, order, hy, tctx)
				if err != nil {
					return &AddOrderResult{Code: -1, Msg: "pipeline set 模板失败: " + err.Error()}, nil
				}
				tctx.varSet(k, val)
			}
		case "extract":
			if step.Extract == nil {
				return &AddOrderResult{Code: -1, Msg: "extract 步骤缺少 extract 配置"}, nil
			}
			raw := tctx.varGet(step.Extract.From)
			val, err := jsonPathString(raw, step.Extract.Path)
			if err != nil {
				name := step.Name
				if name == "" {
					name = fmt.Sprintf("步骤%d", i+1)
				}
				return &AddOrderResult{Code: -1, Msg: name + ": " + err.Error()}, nil
			}
			tctx.varSet(step.Extract.To, val)
		case "http", "finish":
			body, err := executeRuleHTTPRequest(ctx, client, order, hy, tctx, step.toHTTPRule(base))
			if err != nil {
				return &AddOrderResult{Code: -1, Msg: err.Error()}, nil
			}
			lastBody = body
			if step.SaveBodyAs != "" {
				tctx.varSet(step.SaveBodyAs, body)
			}
			if action == "finish" {
				resp := defaultResp
				if step.Response != nil {
					resp = *step.Response
				}
				return parseSubmitResponse(body, resp)
			}
		case "poll":
			body, ok, err := executePollStep(ctx, client, order, hy, tctx, base, step)
			if err != nil {
				return &AddOrderResult{Code: -1, Msg: err.Error()}, nil
			}
			lastBody = body
			if !ok {
				until := step.Poll.Until
				return &AddOrderResult{Code: -1, Msg: "轮询超时: " + responseMsg(body, until)}, nil
			}
		case "return":
			code := step.ReturnCode
			if code == 0 {
				code = -1
			}
			msg, err := renderSubmitTemplate(step.ReturnMsg, order, hy, tctx)
			if err != nil {
				return &AddOrderResult{Code: -1, Msg: "return msg 模板失败: " + err.Error()}, nil
			}
			yid, err := renderSubmitTemplate(step.ReturnYID, order, hy, tctx)
			if err != nil {
				return &AddOrderResult{Code: -1, Msg: "return yid 模板失败: " + err.Error()}, nil
			}
			return &AddOrderResult{Code: code, Msg: firstNonEmpty(msg, "完成"), YID: yid}, nil
		default:
			if action == "" {
				continue
			}
			return &AddOrderResult{Code: -1, Msg: "未知 pipeline action: " + action}, nil
		}
	}

	if lastBody != "" {
		return parseSubmitResponse(lastBody, defaultResp)
	}
	return &AddOrderResult{Code: -1, Msg: "pipeline 未产生 HTTP 响应"}, nil
}

func executeRuleHTTPRequest(ctx context.Context, client *http.Client, order *Order, hy *Huoyuan, tctx *submitTemplateCtx, rule SubmitRuleConfig) (string, error) {
	reqURL, err := renderSubmitTemplate(rule.URL, order, hy, tctx)
	if err != nil {
		return "", fmt.Errorf("URL 模板解析失败: %w", err)
	}
	reqURL = strings.TrimSpace(reqURL)
	if reqURL == "" {
		return "", fmt.Errorf("请求 URL 为空")
	}

	method := strings.ToUpper(strings.TrimSpace(rule.Method))
	if method == "" {
		method = "POST"
	}

	var hdr []string
	for k, v := range rule.Headers {
		rendered, err := renderSubmitTemplate(v, order, hy, tctx)
		if err != nil {
			return "", fmt.Errorf("Header 模板解析失败: %w", err)
		}
		hdr = append(hdr, k+": "+rendered)
	}
	if rule.UseCookie && hy.Cookie != "" {
		hdr = append(hdr, "Cookie: "+hy.Cookie)
	}

	isJSON := strings.EqualFold(rule.ContentType, "json")
	body, isJSON, bodyHdr, errResult := buildSubmitBody(rule, order, hy, tctx)
	if errResult != nil {
		return "", fmt.Errorf("%s", errResult.Msg)
	}
	hdr = append(hdr, bodyHdr...)

	if method == "GET" {
		if body != "" {
			sep := "?"
			if strings.Contains(reqURL, "?") {
				sep = "&"
			}
			reqURL = reqURL + sep + body
		}
		respBody, err := httpRequestCommon(ctx, client, "GET", reqURL, nil, hdr, false)
		if err != nil {
			return "", fmt.Errorf("请求失败: %w", err)
		}
		return respBody, nil
	}
	respBody, err := httpRequestCommon(ctx, client, method, reqURL, strings.NewReader(body), hdr, isJSON)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	return respBody, nil
}

func executePollStep(ctx context.Context, client *http.Client, order *Order, hy *Huoyuan, tctx *submitTemplateCtx, base SubmitRuleConfig, step SubmitPipelineStep) (body string, ok bool, err error) {
	if step.Poll == nil {
		return "", false, fmt.Errorf("poll 步骤缺少 poll 配置")
	}
	poll := step.Poll
	maxAttempts := poll.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	if maxAttempts > 60 {
		maxAttempts = 60
	}
	intervalMS := poll.IntervalMS
	if intervalMS <= 0 {
		intervalMS = 1000
	}
	if intervalMS > 30000 {
		intervalMS = 30000
	}

	var lastBody string
	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return "", false, fmt.Errorf("已取消")
		default:
		}
		lastBody, err = executeRuleHTTPRequest(ctx, client, order, hy, tctx, step.toHTTPRule(base))
		if err != nil {
			return "", false, err
		}
		if step.SaveBodyAs != "" {
			tctx.varSet(step.SaveBodyAs, lastBody)
		}
		if responseMatches(lastBody, poll.Until) {
			return lastBody, true, nil
		}
		if poll.Fail != nil && responseMatches(lastBody, *poll.Fail) {
			return lastBody, false, fmt.Errorf("%s", responseMsg(lastBody, *poll.Fail))
		}
		if attempt < maxAttempts-1 {
			select {
			case <-ctx.Done():
				return "", false, fmt.Errorf("已取消")
			case <-time.After(time.Duration(intervalMS) * time.Millisecond):
			}
		}
	}
	return lastBody, false, nil
}
