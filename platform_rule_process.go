package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func executeProcessRule(ctx context.Context, order *Order, hy *Huoyuan, httpClient *http.Client, proc ProcessRuleConfig) ([]*ProcessCxResult, error) {
	handler := strings.ToLower(strings.TrimSpace(proc.Handler))
	if handler == "" {
		handler = "http"
	}
	switch handler {
	case "http", "":
		return executeHTTPProcessRule(ctx, order, hy, httpClient, proc)
	case "pipeline":
		return runProcessPipelineSteps(ctx, order, hy, httpClient, proc)
	case "script":
		if proc.Script == nil {
			return nil, fmt.Errorf("process.script 为空")
		}
		src := strings.TrimSpace(proc.Script.Source)
		if src != "" {
			return executeStarlarkProcessRule(ctx, order, hy, httpClient, proc, src, proc.Script.TimeoutMS)
		}
		if len(proc.Script.Steps) == 0 {
			return nil, fmt.Errorf("process.script.steps 为空")
		}
		wrapped := proc
		wrapped.Pipeline = proc.Script.Steps
		wrapped.Handler = "pipeline"
		return runProcessPipelineSteps(ctx, order, hy, httpClient, wrapped)
	default:
		return nil, fmt.Errorf("未知 process handler: %s", handler)
	}
}

func executeHTTPProcessRule(ctx context.Context, order *Order, hy *Huoyuan, httpClient *http.Client, proc ProcessRuleConfig) ([]*ProcessCxResult, error) {
	client := pluginHTTPClient(httpClient)
	tctx := newSubmitTemplateCtx(submitRuleFromProcess(proc))
	rule := processToSubmitRule(proc)
	body, err := executeRuleHTTPRequest(ctx, client, order, hy, tctx, rule)
	if err != nil {
		return nil, err
	}
	return parseProcessResults(body, proc.Map)
}

func runProcessPipelineSteps(ctx context.Context, order *Order, hy *Huoyuan, httpClient *http.Client, proc ProcessRuleConfig) ([]*ProcessCxResult, error) {
	if len(proc.Pipeline) == 0 {
		return nil, fmt.Errorf("process.pipeline 步骤为空")
	}
	client := pluginHTTPClient(httpClient)
	base := submitRuleFromProcess(proc)
	tctx := newSubmitTemplateCtx(base)

	for i := range proc.Pipeline {
		step := proc.Pipeline[i]
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
				return nil, fmt.Errorf("已取消")
			case <-time.After(time.Duration(ms) * time.Millisecond):
			}
		case "set":
			for k, tpl := range step.Set {
				val, err := renderSubmitTemplate(tpl, order, hy, tctx)
				if err != nil {
					return nil, fmt.Errorf("process set 模板失败: %w", err)
				}
				tctx.varSet(k, val)
			}
		case "extract":
			if step.Extract == nil {
				return nil, fmt.Errorf("extract 步骤缺少 extract 配置")
			}
			raw := tctx.varGet(step.Extract.From)
			val, err := jsonPathString(raw, step.Extract.Path)
			if err != nil {
				name := step.Name
				if name == "" {
					name = fmt.Sprintf("步骤%d", i+1)
				}
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			tctx.varSet(step.Extract.To, val)
		case "http":
			body, err := executeRuleHTTPRequest(ctx, client, order, hy, tctx, step.toHTTPRule(base))
			if err != nil {
				return nil, err
			}
			if step.SaveBodyAs != "" {
				tctx.varSet(step.SaveBodyAs, body)
			}
		case "poll":
			body, ok, err := executePollStep(ctx, client, order, hy, tctx, base, step)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("轮询超时: %s", responseMsg(body, step.Poll.Until))
			}
		case "process_finish":
			body, err := executeRuleHTTPRequest(ctx, client, order, hy, tctx, step.toHTTPRule(base))
			if err != nil {
				return nil, err
			}
			mapCfg := proc.Map
			if step.ProcessMap != nil {
				mapCfg = *step.ProcessMap
			}
			return parseProcessResults(body, mapCfg)
		default:
			if action == "" {
				continue
			}
			return nil, fmt.Errorf("process pipeline 不支持 action: %s", action)
		}
	}
	return nil, fmt.Errorf("process pipeline 未产生结果")
}

func submitRuleFromProcess(proc ProcessRuleConfig) SubmitRuleConfig {
	return SubmitRuleConfig{
		Method:      proc.Method,
		URL:         proc.URL,
		ContentType: proc.ContentType,
		UseCookie:   proc.UseCookie,
		Headers:     proc.Headers,
		Body:        proc.Body,
		BodyMode:    proc.BodyMode,
		BodyRaw:     proc.BodyRaw,
	}
}

func processToSubmitRule(proc ProcessRuleConfig) SubmitRuleConfig {
	return submitRuleFromProcess(proc)
}

func parseProcessResults(body string, mapCfg ProcessResultMap) ([]*ProcessCxResult, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("查课响应为空")
	}
	if len(mapCfg.Fields) == 0 {
		return nil, fmt.Errorf("process.map.fields 为空")
	}

	var root interface{}
	if err := json.Unmarshal([]byte(body), &root); err != nil {
		return nil, fmt.Errorf("查课响应 JSON 无效")
	}

	if mapCfg.CodeField != "" && len(mapCfg.SuccessCodes) > 0 {
		m, ok := root.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("查课响应格式错误")
		}
		codeField := mapCfg.CodeField
		codeVal := m[codeField]
		okCode := false
		for _, sc := range mapCfg.SuccessCodes {
			if codeMatches(codeVal, sc) {
				okCode = true
				break
			}
		}
		if !okCode {
			msgField := mapCfg.MsgField
			if msgField == "" {
				msgField = "msg"
			}
			return []*ProcessCxResult{{
				Code: -1,
				Msg:  firstNonEmpty(mapGetString(m, msgField), "查课失败"),
			}}, nil
		}
	}

	itemsPath := strings.TrimSpace(mapCfg.ItemsPath)
	if itemsPath == "" {
		item := mapProcessItem(root, mapCfg.Fields)
		return []*ProcessCxResult{item}, nil
	}

	itemsVal := getNestedValueAny(root, itemsPath)
	arr, ok := itemsVal.([]interface{})
	if !ok {
		return nil, fmt.Errorf("items_path %q 不是数组", itemsPath)
	}
	out := make([]*ProcessCxResult, 0, len(arr))
	for _, it := range arr {
		out = append(out, mapProcessItem(it, mapCfg.Fields))
	}
	return out, nil
}

func mapProcessItem(item interface{}, fields map[string]string) *ProcessCxResult {
	res := &ProcessCxResult{Code: 1, Msg: "ok"}
	for key, path := range fields {
		val := flexString(getNestedValueAny(item, path))
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "code":
			res.Code = flexInt(getNestedValueAny(item, path), 1)
		case "msg":
			res.Msg = val
		case "yid":
			res.YID = val
		case "kcname", "kc_name":
			res.KCName = val
		case "user":
			res.User = val
		case "pass":
			res.Pass = val
		case "ksks":
			res.KSKS = val
		case "ksjs":
			res.KSJS = val
		case "status_text", "statustext":
			res.StatusText = val
		case "process":
			res.Process = val
		case "remarks":
			res.Remarks = val
		case "kcks":
			res.Kcks = val
		case "kcjs":
			res.Kcjs = val
		}
	}
	if res.Msg == "" {
		res.Msg = "ok"
	}
	return res
}
