package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

type aiConvertRequest struct {
	PHP              string `json:"php"`
	PlatformTypeHint string `json:"platform_type_hint,omitempty"`
}

type aiConvertResponse struct {
	PlatformType    string           `json:"platform_type"`
	RuleConfig      SubmitRuleConfig `json:"rule_config"`
	Warnings        []string         `json:"warnings,omitempty"`
	Notes           string           `json:"notes,omitempty"`
	ParseSource     string           `json:"parse_source,omitempty"` // local | hybrid | ai
	ValidationHints []string         `json:"validation_hints,omitempty"`
}

var aiJSONFenceRe = regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")

var (
	aiConvertRateMu sync.Mutex
	aiConvertRate   = map[string][]time.Time{}
)

const (
	aiConvertRateWindow = time.Minute
	aiConvertRateMax    = 10
)

func allowAIConvert(rateKey string) bool {
	rateKey = strings.TrimSpace(rateKey)
	if rateKey == "" {
		return false
	}
	now := time.Now()
	aiConvertRateMu.Lock()
	defer aiConvertRateMu.Unlock()
	calls := aiConvertRate[rateKey]
	kept := make([]time.Time, 0, len(calls))
	for _, t := range calls {
		if now.Sub(t) < aiConvertRateWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) >= aiConvertRateMax {
		aiConvertRate[rateKey] = kept
		return false
	}
	kept = append(kept, now)
	aiConvertRate[rateKey] = kept
	return true
}

func adminRuleAIStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	cfg := getAIConfig()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  "ok",
		"data": map[string]interface{}{
			"configured": aiConfigReady(),
			"enabled":    cfg.Enabled,
			"model":      cfg.Model,
		},
	})
}

func adminRuleAIConvertHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
		return
	}
	var req aiConvertRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	php := strings.TrimSpace(req.PHP)
	if php == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请粘贴 PHP 代码"})
		return
	}
	if len(php) > 120000 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "PHP 代码过长"})
		return
	}

	if !aiConfigReady() {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"code": -1,
			"msg":  "请先在「系统设置 → AI 转换」中启用并配置 API Key",
		})
		return
	}

	rateKey := sessionUserFromRequest(r.Context(), r)
	if rateKey == "" {
		rateKey = strings.TrimSpace(r.RemoteAddr)
	}
	if !allowAIConvert(rateKey) {
		writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"code": -1,
			"msg":  "AI 转换请求过于频繁，请稍后再试",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	out, err := convertPhpToRuleWithAI(ctx, php, strings.TrimSpace(req.PlatformTypeHint))
	if err != nil {
		log.Printf("[AI规则] 转换失败: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": out})
}

func convertPhpToRuleWithAI(ctx context.Context, php, hint string) (*aiConvertResponse, error) {
	draft := parseXdjkPhpFromGo(php)

	useHybrid := draft != nil && draft.Error == "" && aiRuleHasActionableURL(draft.RuleConfig)
	userPrompt := ruleAIUserPrompt(php, hint)
	if useHybrid {
		normalizeParsedRule(&draft.RuleConfig)
		userPrompt = buildLocalDraftPrompt(draft, php, hint)
	}

	out, err := callAIForRuleConvert(ctx, userPrompt)
	if err != nil {
		return nil, err
	}

	if useHybrid {
		out.RuleConfig = mergeRuleWithLocalDraft(draft.RuleConfig, out.RuleConfig, draft.TrustedTemplate)
		out.Warnings = appendUniqueStrings(out.Warnings, draft.Warnings...)
		out.Warnings = appendUniqueStrings(out.Warnings, draft.SpecialNotes...)
		out.ParseSource = "hybrid"
		if strings.TrimSpace(out.Notes) == "" {
			out.Notes = "本地草稿 + AI 补全"
		}
	} else {
		out.ParseSource = "ai"
		if draft != nil {
			out.Warnings = appendUniqueStrings(out.Warnings, draft.Warnings...)
			out.Warnings = appendUniqueStrings(out.Warnings, draft.SpecialNotes...)
			if draft.Error != "" {
				out.Warnings = appendUniqueStrings(out.Warnings, "本地解析: "+draft.Error)
			}
		}
	}

	if hint != "" {
		out.PlatformType = hint
	} else if out.PlatformType == "" && draft != nil {
		out.PlatformType = draft.PlatformType
	}

	if err := validateAIConvertRule(out); err != nil {
		return nil, err
	}
	attachRuleValidationHints(out)
	return out, nil
}

// fixRuleFromSubmitFailure 根据真实下单失败信息，用 AI 修正已有 rule_config
func fixRuleFromSubmitFailure(ctx context.Context, platformType string, current SubmitRuleConfig, failMsg, upstreamBody, php string) (*aiConvertResponse, error) {
	platformType = strings.TrimSpace(platformType)
	if platformType == "" {
		return nil, fmt.Errorf("platform_type 为空")
	}
	failMsg = strings.TrimSpace(failMsg)
	if failMsg == "" {
		return nil, fmt.Errorf("失败信息为空")
	}

	userPrompt := buildRuleFixFromFailurePrompt(platformType, current, failMsg, upstreamBody, php)
	out, err := callAIForRuleConvert(ctx, userPrompt)
	if err != nil {
		return nil, err
	}
	out.PlatformType = platformType
	normalizeParsedRule(&out.RuleConfig)
	if err := validateAIConvertRule(out); err != nil {
		return nil, err
	}
	attachRuleValidationHints(out)
	out.ParseSource = "submit_fail_fix"
	if strings.TrimSpace(out.Notes) == "" {
		out.Notes = "下单失败触发的 AI 规则修正"
	}
	out.Warnings = appendUniqueStrings(out.Warnings, "由试单失败触发的 AI 修正，请核对后再保存")
	return out, nil
}

func buildRuleFixFromFailurePrompt(platformType string, current SubmitRuleConfig, failMsg, upstreamBody, php string) string {
	ruleJSON, _ := json.Marshal(current)
	var b strings.Builder
	b.WriteString("【任务】真实下单失败，请修正 rule_config JSON。\n")
	b.WriteString("platform_type: ")
	b.WriteString(platformType)
	b.WriteString("\n\n【当前 rule_config（有误，需修正）】\n")
	b.Write(ruleJSON)
	b.WriteString("\n\n【下单失败信息】\n")
	b.WriteString(failMsg)
	b.WriteString("\n")
	if strings.TrimSpace(upstreamBody) != "" {
		b.WriteString("\n【上游响应片段（已脱敏）】\n")
		b.WriteString(upstreamBody)
		b.WriteString("\n")
	}
	if strings.TrimSpace(php) != "" {
		b.WriteString("\n【原始 PHP 参考（若有冲突以 PHP 为准）】\n")
		b.WriteString(php)
		b.WriteString("\n")
	}
	b.WriteString(`
【修正要求】
1. 输出完整 rule_config JSON（含 platform_type、rule_config、warnings、notes）
2. 重点检查：url/method/content_type/body/body_raw、response.success_codes、code_field、msg_field、yid_field/yid_path、failure_msg_rules
3. 若失败是「响应解析失败/格式错误/成功码不匹配」，优先修正 response 段
4. 若上游 msg 提示缺字段/参数名错误，修正 body/body_raw/headers
5. 若失败含「货源地址/请求地址/URL 格式/404/接口地址不存在/e_url」等：
   - 检查 rule url 是否与 PHP 路径一致
   - 禁止 http://{{huoyuan.url}}（货源 url 已含协议时用 {{huoyuan.url}}/path）
   - 检查是否缺少或多余 /、act 参数、分支 branches 中的 url
   - 若错误明确指向「货源配置地址为空/格式错误」，在 warnings 中说明可能还需检查货源配置页的 url 字段
6. 不要改 platform_type；不要输出 markdown
`)
	return b.String()
}

func callAIForRuleConvert(ctx context.Context, userPrompt string) (*aiConvertResponse, error) {
	cfg := getAIConfig()
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	model := normalizeDeepSeekModel(strings.TrimSpace(cfg.Model))
	if model == "" {
		model = "gpt-4o-mini"
	}
	endpoint := aiChatCompletionsURL(baseURL)
	if err := ValidateOutboundHTTPURL(ctx, endpoint); err != nil {
		return nil, fmt.Errorf("AI 接口地址被安全策略拦截")
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": ruleAISystemPrompt()},
			{"role": "user", "content": userPrompt},
		},
	}
	applyAIRuleConversionOptions(payload, baseURL, model)
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))

	client := NewOutboundHTTPClient(120 * time.Second)
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("调用 AI 失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 AI 响应失败")
	}
	if resp.StatusCode != http.StatusOK {
		msg := extractAIErrorMessage(respBody)
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("AI 返回错误: %s", msg)
	}

	content, err := extractAIChatContent(respBody)
	if err != nil {
		return nil, err
	}

	rawJSON := extractJSONFromAIText(content)
	out, parseErr := parseAIConvertResponse(rawJSON)
	if parseErr != nil {
		log.Printf("[AI规则] JSON解析失败 model=%s err=%v snippet=%q", model, parseErr, RedactSecrets(truncateForLog(rawJSON, 400)))
		return nil, fmt.Errorf("AI 返回的 JSON 无法解析，请重试或手动调整")
	}
	return out, nil
}

func validateAIConvertRule(out *aiConvertResponse) error {
	out.PlatformType = strings.TrimSpace(out.PlatformType)
	if out.PlatformType == "" {
		return fmt.Errorf("AI 未识别 platform_type")
	}
	if !aiRuleHasActionableURL(out.RuleConfig) {
		return fmt.Errorf("AI 返回的规则缺少 url（单次 http 必填；pipeline/script 可在步骤内或 script.source 中表达）")
	}
	normalizeParsedRule(&out.RuleConfig)
	if len(out.RuleConfig.Response.SuccessCodes) == 0 && !out.RuleConfig.Response.SuccessHTTP {
		return fmt.Errorf("AI 返回的规则缺少 response.success_codes，请重试或手动填写")
	}
	return nil
}

func appendUniqueStrings(base []string, extra ...string) []string {
	seen := map[string]struct{}{}
	for _, s := range base {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		seen[s] = struct{}{}
	}
	for _, s := range extra {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		base = append(base, s)
	}
	return base
}

func aiRuleHasActionableURL(rule SubmitRuleConfig) bool {
	if strings.TrimSpace(rule.URL) != "" {
		return true
	}
	handler := strings.ToLower(strings.TrimSpace(rule.Handler))
	steps := rule.Pipeline
	if rule.Script != nil {
		if strings.TrimSpace(rule.Script.Source) != "" {
			return true
		}
		if len(rule.Script.Steps) > 0 {
			steps = rule.Script.Steps
		}
	}
	for _, s := range steps {
		if strings.TrimSpace(s.URL) != "" {
			return true
		}
	}
	for _, br := range rule.Branches {
		if strings.TrimSpace(br.URL) != "" {
			return true
		}
	}
	if handler == "pipeline" && len(steps) > 0 {
		return true
	}
	return false
}

func extractAIChatContent(respBody []byte) (string, error) {
	var result struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析 AI 响应失败")
	}
	if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
		return "", fmt.Errorf("%s", strings.TrimSpace(result.Error.Message))
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("AI 未返回内容")
	}
	msg := result.Choices[0].Message
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		if rc := strings.TrimSpace(msg.ReasoningContent); rc != "" {
			return "", fmt.Errorf("模型返回了思考过程但未返回 JSON，请使用 deepseek-v4-flash（非思考模式）")
		}
		return "", fmt.Errorf("AI 未返回内容")
	}
	return content, nil
}

func extractAIErrorMessage(body []byte) string {
	var errObj struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &errObj) == nil {
		if m := strings.TrimSpace(errObj.Error.Message); m != "" {
			return m
		}
		if m := strings.TrimSpace(errObj.Message); m != "" {
			return m
		}
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func extractJSONFromAIText(text string) string {
	text = strings.TrimSpace(text)
	if m := aiJSONFenceRe.FindStringSubmatch(text); len(m) > 1 {
		text = strings.TrimSpace(m[1])
	}
	if obj := extractOutermostJSONObject(text); obj != "" {
		return repairJSONTrailingCommas(obj)
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return repairJSONTrailingCommas(text[start : end+1])
	}
	return repairJSONTrailingCommas(text)
}

func extractOutermostJSONObject(s string) string {
	start := -1
	depth := 0
	inString := false
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					return s[start : i+1]
				}
			}
		}
	}
	return ""
}

var jsonTrailingCommaRe = regexp.MustCompile(`,(\s*[}\]])`)

func repairJSONTrailingCommas(s string) string {
	return jsonTrailingCommaRe.ReplaceAllString(s, "$1")
}

func parseAIConvertResponse(rawJSON string) (*aiConvertResponse, error) {
	rawJSON = strings.TrimSpace(rawJSON)
	if rawJSON == "" {
		return nil, fmt.Errorf("empty json")
	}
	var flex struct {
		PlatformType string          `json:"platform_type"`
		RuleConfig   json.RawMessage `json:"rule_config"`
		Warnings     []string        `json:"warnings"`
		Notes        string          `json:"notes"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &flex); err != nil {
		return nil, err
	}
	rule, err := decodeFlexibleRuleConfig(flex.RuleConfig)
	if err != nil {
		return nil, err
	}
	return &aiConvertResponse{
		PlatformType: flex.PlatformType,
		RuleConfig:   rule,
		Warnings:     flex.Warnings,
		Notes:        flex.Notes,
	}, nil
}

func decodeFlexibleRuleConfig(raw json.RawMessage) (SubmitRuleConfig, error) {
	var flex map[string]interface{}
	if err := json.Unmarshal(raw, &flex); err != nil {
		return SubmitRuleConfig{}, err
	}
	normalizeFlexibleRuleBody(flex)
	if headers, ok := flex["headers"].(map[string]interface{}); ok {
		normalized := make(map[string]string, len(headers))
		for k, v := range headers {
			normalized[k] = fmt.Sprint(v)
		}
		flex["headers"] = normalized
	}
	b, err := json.Marshal(flex)
	if err != nil {
		return SubmitRuleConfig{}, err
	}
	var rule SubmitRuleConfig
	if err := json.Unmarshal(b, &rule); err != nil {
		return SubmitRuleConfig{}, err
	}
	if rule.Body == nil {
		rule.Body = map[string]string{}
	}
	return rule, nil
}

func normalizeFlexibleRuleBody(flex map[string]interface{}) {
	bodyMode, _ := flex["body_mode"].(string)
	if strings.TrimSpace(bodyMode) == "raw" {
		if raw, ok := flex["body_raw"].(string); ok && strings.TrimSpace(raw) != "" {
			flex["body"] = map[string]interface{}{}
			return
		}
	}
	switch body := flex["body"].(type) {
	case map[string]interface{}:
		normalized := make(map[string]string, len(body))
		for k, v := range body {
			normalized[k] = fmt.Sprint(v)
		}
		flex["body"] = normalized
	case []interface{}:
		rawBytes, err := json.Marshal(body)
		if err != nil {
			return
		}
		flex["body_mode"] = "raw"
		flex["body_raw"] = string(rawBytes)
		flex["body"] = map[string]interface{}{}
	case string:
		if strings.TrimSpace(body) == "" {
			flex["body"] = map[string]interface{}{}
			return
		}
		flex["body_mode"] = "raw"
		flex["body_raw"] = body
		flex["body"] = map[string]interface{}{}
	default:
		if flex["body"] == nil {
			flex["body"] = map[string]interface{}{}
		}
	}
}

func truncateForLog(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func ruleAIUserPrompt(php, hint string) string {
	var b strings.Builder
	b.WriteString(`请将以下 xdjk.php 平台「下单/提交」PHP 片段，完整转换为 rule_config JSON。

## 转换步骤（请按顺序分析后再输出）
1. 识别 platform_type：从 $type == "xxx" 或 else if ($type == "xxx") 提取
2. 识别请求方式：get_url / post / httpRequest / curl / customizeHttpClientPost / llv2_submit
3. 拼接 url：$a["url"] → {{huoyuan.url}}；路径有前导 / 则 {{huoyuan.url}}/path；PHP 写 $a["url"]."api/..." 无斜杠时，tj 应写 {{huoyuan.url}}/api/...（货源 url 会自动去掉末尾 /）
   PHP 写 http://{$url}/path 时，规则仍用 {{huoyuan.url}}/path（货源可只填域名，引擎自动补 http://）
4. 判断 content_type：json_encode / Content-Type: application/json → json；否则 form
5. 解析 body：一维 array → body 对象；嵌套 array / json_encode 整段 → body_mode:"raw" + body_raw
6. 解析 headers / use_cookie：Authorization Bearer、Token、cookie 参数
7. 解析成功条件：if ($result["code"]==...) 或 success_http（无 code 字段）
8. 解析 yid：$result["id"] / data.0 / orderList[0].orderId 等 → yid_field 或 yid_path
9. 解析 failure_msg_rules：PHP 失败分支里 strpos($result["msg"], "关键字") → 自定义 msg
10. 判断 handler：单次 HTTP 省略；两次及以上 HTTP → pipeline；极复杂 → script 或 warnings
11. 若有查课/同步进度 HTTP → 补 process 段
12. 无法静态表达的逻辑（SQL、array_rand、按 kcname 多 URL）→ warnings

## 输出前自检
- rule_config.url 非空（或 pipeline/script 步骤内有 url）
- response.success_codes 非空，且与 PHP 判断完全一致（"0"/0、"1"/1、200 等）
- 嵌套 JSON 未误写入 body 对象（body 只能是扁平键值）
- failure_msg_rules 已收录 PHP 里所有 strpos 映射
- success_msg 与 PHP 成功返回的 msg 一致

`)
	if hint != "" {
		b.WriteString("## 平台类型提示\n")
		b.WriteString(hint)
		b.WriteString("\n\n")
	}
	b.WriteString("## PHP 源码\n```php\n")
	b.WriteString(php)
	b.WriteString("\n```")
	return b.String()
}

func ruleAISystemPrompt() string {
	return `你是「智能提交引擎」的规则配置助手。用户粘贴的是 xdjk.php 里某平台的下单 PHP 片段，你需要输出 JSON（不要 markdown 说明），格式严格如下：

{
  "platform_type": "从 $type == \"xxx\" 提取的平台标识",
  "rule_config": { ...SubmitRuleConfig... },
  "warnings": ["无法完全表达的逻辑说明"],
  "notes": "一句话总结"
}

═══════════════════════════════════════
一、转换工作流（必须遵循）
═══════════════════════════════════════
Step A — 读 PHP 结构
  - 找 $type == "平台名" 作为 platform_type
  - 找 URL 赋值：$xx_url = $a["url"]."路径" 或 $a['url']."api/..."
  - 找 $data / $postData / $requestData 及 json_encode
  - 找 HTTP 调用：get_url($url,$data) / post($url,$data,$header) / httpRequest('POST',...) / llv2_submit(...)
  - 找 json_decode($result,true) 后的 if 成功/失败分支
  - 找 return $b 或 return ["code"=>...] 的最终结构

Step B — 选 handler
  - 仅 1 次 HTTP → 省略 handler 或 "http"
  - 先 login 再 submit、先查再下单 → handler:"pipeline"，步骤用 action:http/finish/extract
  - 含 sleep 轮询同一 URL → pipeline action:poll（interval_ms=秒×1000）
  - 仅 sleep 一次再请求 → 顶层 delay_ms 或 pipeline action:delay
  - 课代表 kcid base64 解码改 JSON → body_mode:"kcid_json" + kcid_json_patches
  - 按 order.noun/kcname 不同 URL 或 body → branches
  - 实在无法用 JSON 表达 → script.source Starlark + warnings

Step C — 填 rule_config 必填项
  - method: GET 或 POST（get_url 单参数多为 GET，双参数多为 POST）
  - url: 含 {{huoyuan.url}}，路径与 PHP 一致，末尾不要多余 /
  - content_type: form | json
  - body: {} 或扁平键值（禁止嵌套）
  - response.success_codes: 必填，与 PHP 一致

Step D — 填 response
  - 对照 PHP：if ($result["code"] == "0") → success_codes:["0",0]
  - if ($result["code"] == "1") 或 == 1 → success_codes:["1",1]（hzw、yumeng、nx 等）
  - if ($result['code'] == 200) → success_codes:[200,"200"]
  - 无 code 字段、仅看 HTTP 状态（龙龙 V2）→ success_http:true
  - yid：$result["id"]→yid_field:"id"；$result["data"][0]→yid_path:"data.0"
  - $result['data']['orderList'][0]['orderId']→yid_path:"data.orderList.0.orderId"
  - 响应为 UUID 字符串数组 → yid_path:"0"
  - failure_msg_rules：逐条提取 strpos($result["msg"],'xxx') 后的 $msg 赋值

═══════════════════════════════════════
二、PHP → 模板变量对照表
═══════════════════════════════════════
货源 $a[...]：
  $a["url"]→{{huoyuan.url}}  $a["user"]→{{huoyuan.user}}  $a["pass"]→{{huoyuan.pass}}
  $a["token"]→{{huoyuan.token}}  $a["cookie"]→{{huoyuan.cookie}}
订单变量：
  $noun→{{order.noun}}  $school→{{order.school}}  $user→{{order.user}}  $pass→{{order.pass}}
  $kcname→{{order.kcname}}  $kcid→{{order.kcid}}  $oid→{{order.oid}}
  $d["name"]→{{order.name}}  $uScore→{{order.uScore}}  $uTime→{{order.uTime}}
  $b["isck"]→{{order.isck}}  $token（全局）→{{huoyuan.token}}
  urlencode($pass)→{{urlencode order.pass}}
  $user." ".$pass 拼接→在 body_raw 写 "{{order.user}} {{order.pass}}" 或分别模板后拼接

═══════════════════════════════════════
三、常见 PHP 模式识别
═══════════════════════════════════════
【模式1】标准 ssrs/hzw 系 form 下单
  PHP: $data=array("uid"=>$a["user"],"key"=>$a["pass"],"platform"=>$noun,...);
       $url=$a["url"]."/api.php?act=add"; get_url($url,$data);
       if($result["code"]=="0") 或 "1" → 注意各平台 success_codes 不同
  JSON: content_type:"form", body 扁平映射, url:"{{huoyuan.url}}/api.php?act=add"

【模式2】code=0 成功（27/ssrs/hb/2023/hh/pup/ml/HEI/wufu 等）
  success_codes:["0",0], yid_field:"id"（若有）

【模式3】code=1 成功（hzw/nx/yumeng/coco 等）
  success_codes:["1",1]

【模式4】Bearer Token + JSON post
  PHP: header=["Authorization: Bearer ".$a['token']]; json_encode($data); post($url,$data,$header)
  JSON: content_type:"json", headers:{"Authorization":"Bearer {{huoyuan.token}}"}
  若 body 含嵌套数组 → body_mode:"raw" + body_raw

【模式5】8090 / jxjyyjy 继续教育
  - URL: {{huoyuan.url}}/api/order/submit 或 /api/order/buy（PHP 虽无斜杠拼接，tj 规则须加 /）
  - JSON body 含 websiteId/websiteNumber、accountInfo、selectedCourseKeys 或 children 嵌套
  - success_codes:[200,"200"], msg_field:"message", yid_field:"data" 或 yid_path:"data.orderList.0.orderId"
  - isck==0 时 kcname 置空：warnings 说明，body_raw 中 kcname 仍用 {{order.kcname}}（引擎按订单字段）
  - 8090 特殊：code=200 但 message 含「失败」→ failure_msg_on_success:true + failure_msg_rules:[{"contains":"失败"}]；success_use_upstream_msg:true 返回上游 message

【模式6】tesla 外部下单
  - url:"{{huoyuan.url}}/api/api/external/submit-order"
  - body 用 "cid"=>$noun 而非 platform

【模式7】THOTH OpenAPI（t.thoth8.com/api/open）
  - url:"{{huoyuan.url}}/api/open/add", content_type:"form"
  - 鉴权在 Header：X-Uid/X-Api-Key（huoyuan.user/pass），禁止 body/query 传 uid/key
  - body：platform/school/user/pass/name/kcname/kcid/score/shichang；kcname/kcid 为 JSON 字符串数组如 ["课名"]
  - success_codes:["0",0]；yid_field:"id"

【模式8】goStudy JSON 数组 body
  - body_mode:"raw", body_raw 为 JSON 数组字符串，success_codes:[0,"0"], yid_path:"data.0"

【模式9】龙龙 longlong V2（LongLongV2.php / llv2_submit）
  - 不是 api.php?act=add；是 {{huoyuan.url}}/api/submit/{{order.noun}}
  - headers: X-Uid/X-Api-Key；success_http:true；yid_path:"0"
  - 若有查课：process 段 GET {{huoyuan.url}}/api/order/uuid/{{order.yid}}

【模式10】nx 奶昔 + failure_msg_rules
  - post JSON, success_codes:["1",1]
  - 必须把 PHP 里所有 strpos 失败映射写入 failure_msg_rules

【模式11】huangzu 等 use_cookie
  - get_url($url,$data,$cookie) → use_cookie:true

【模式12】GET 查询串（kunba 等）
  - url 含 ?platform= 等查询参数，method:"GET", body:{}

【模式13】白泽 baize：curl JSON，success_codes:["0000"]，yid_path:"data.order_id"

【模式14】西游 xiyou：code_field:"status"，success_codes:["success"]

【模式15】YYY：/api/order，success_codes:[200,"200"]，yid_path:"data.yid"

═══════════════════════════════════════
四、SubmitRuleConfig 字段详解
═══════════════════════════════════════
- handler: 省略或 "http"=单次 HTTP（默认）；多步用 "pipeline"；复杂用 "script"
- pipeline: 步骤数组。action: set|delay|http|finish|extract|return|poll|process_finish
  - http: 发请求，save_body_as 存响应 JSON 到变量
  - finish: 发请求并按 response 解析为最终结果
  - poll: 循环 HTTP，poll.until 与 response 同结构，poll.interval_ms / max_attempts
  - extract: {"from":"login_body","path":"data.token","to":"token"}
  - set: {"token":"{{json_path var.login_body data.token}}"}
  - 模板: {{var.变量名}}、{{json_path var.login_body data.token}}、{{random_port}}
- script: {"source":"Starlark"} 或 {"steps":[...]}；最后 result={"code":1,"msg":"...","yid":"..."}
- process: 查课/同步进度。handler http|pipeline|script；map.fields 映射 process/kcname/yid/status_text 等
- method: GET 或 POST
- url: {{huoyuan.url}} + PHP 路径，可含 {{order.noun}} 等
- content_type: "form"（get_url 表单）或 "json"（json_encode / application/json）
- use_cookie: PHP 使用 $cookie 或 get_url 第三参数 $cookie 时为 true
- headers: Authorization、Token、Content-Type、X-Uid 等
- body: 仅扁平键值；嵌套结构禁止写进 body
- body_mode: 空=普通；raw=整段 JSON 字符串；kcid_json=解码 kcid 后打补丁
- body_raw: body_mode=raw 时的模板字符串（注意 JSON 内双引号转义 \"）
- branches: 按 order 字段分支，when.field 如 order.noun / order.kcname / order.isck
  示例：{"when":{"field":"order.isck","equals":"0"},"body_raw":"...kcname为空..."}
- url_port_pool: KUN 随机端口 → 写固定端口数组，url 用 {{random_port}}
- kcid_json_patches: body_mode=kcid_json 时按 noun 补丁字段
- delay_ms: PHP sleep(N) 一次 → N*1000
- response: 必填
  - code_field: 默认 code；8090/jxjyyjy 失败时用 message
  - success_codes: 成功值数组，字符串和数字都要写 ["0",0] ["1",1] [200,"200"]
  - msg_field: msg 或 message
  - yid_field: 顶层字段名如 id、data
  - yid_path: 嵌套路径如 data.0、data.orderList.0.orderId
  - success_http: true=HTTP 2xx 即成功（无 code 时）
  - success_msg: 返回主站的成功文案
  - failure_msg_rules: [{"contains":"重复提交","msg":"已获取最新订单，等待进度同步"}]
  - failure_msg_on_success: true 时 success_codes 命中后仍检查 failure_msg_rules（8090）
  - success_use_upstream_msg: true 时成功返回上游 msg_field 文案（8090 的 message）

═══════════════════════════════════════
五、完整示例（对照学习）
═══════════════════════════════════════

示例1 — hzw 标准 form（code=1 成功）：
{"platform_type":"hzw","rule_config":{"method":"POST","url":"{{huoyuan.url}}/api.php?act=add","content_type":"form","body":{"uid":"{{huoyuan.user}}","key":"{{huoyuan.pass}}","platform":"{{order.noun}}","school":"{{order.school}}","user":"{{order.user}}","pass":"{{order.pass}}","kcid":"{{order.kcid}}","kcname":"{{order.kcname}}"},"response":{"code_field":"code","success_codes":["1",1],"msg_field":"msg","yid_field":"id","success_msg":"下单成功"}},"warnings":[],"notes":"标准 form 下单"}

示例2 — ssrs/27 系（code=0 成功）：
{"platform_type":"27","rule_config":{"method":"POST","url":"{{huoyuan.url}}/api.php?act=add","content_type":"form","body":{"uid":"{{huoyuan.user}}","key":"{{huoyuan.pass}}","platform":"{{order.noun}}","school":"{{order.school}}","user":"{{order.user}}","pass":"{{order.pass}}","kcid":"{{order.kcid}}","kcname":"{{order.kcname}}"},"response":{"code_field":"code","success_codes":["0",0],"msg_field":"msg","yid_field":"id","success_msg":"下单成功"}},"warnings":[],"notes":"code=0 成功"}

示例3 — nx JSON + failure_msg_rules：
{"platform_type":"nx","rule_config":{"method":"POST","url":"{{huoyuan.url}}/api/v1/order/submit","content_type":"json","body":{"token":"{{huoyuan.token}}","school":"{{order.school}}","account":"{{order.user}}","password":"{{order.pass}}","coursename":"{{order.kcname}}","value":"{{order.noun}}"},"response":{"code_field":"code","success_codes":["1",1],"msg_field":"msg","success_msg":"下单成功","failure_msg_rules":[{"contains":"重复提交","msg":"已获取最新订单，等待进度同步"},{"contains":"Repeated","msg":"已获取最新订单，等待进度同步"},{"contains":"积分不足","msg":"学时不足，请联系上级!"},{"contains":"Insufficient","msg":"学时不足，请联系上级!"},{"contains":"不能为空","msg":"参数提交不完整"},{"contains":"cannot be empty","msg":"参数提交不完整"}]}},"warnings":[],"notes":"nx 下单"}

示例4 — goStudy 嵌套 JSON 数组（body_raw）：
{"platform_type":"goStudy","rule_config":{"method":"POST","url":"{{huoyuan.url}}/open/submitCourse","content_type":"json","headers":{"token":"{{huoyuan.token}}","Content-Type":"application/json;charset=UTF-8"},"body_mode":"raw","body_raw":"[{\"platformId\":\"{{order.noun}}\",\"studentName\":\"{{order.name}}\",\"school\":\"{{order.school}}\",\"account\":\"{{order.user}}\",\"password\":\"{{order.pass}}\",\"code\":\"{{order.kcid}}\",\"name\":\"{{order.kcname}}\"}]","body":{},"response":{"code_field":"code","success_codes":[0,"0"],"msg_field":"msg","yid_path":"data.0","success_msg":"已添加至服务器，开始执行刷课！"}},"warnings":[],"notes":"JSON 数组 body"}

示例5 — 8090 Bearer + 嵌套 JSON（含 success 后失败判定）：
{"platform_type":"8090","rule_config":{"method":"POST","url":"{{huoyuan.url}}/api/order/submit","content_type":"json","headers":{"Content-Type":"application/json; charset=utf-8","Authorization":"Bearer {{huoyuan.token}}"},"body_mode":"raw","body_raw":"{\"websiteId\":\"{{order.noun}}\",\"accountInfo\":\"{{order.user}} {{order.pass}}\",\"selectedCourseKeys\":[\"{{order.kcname}}\"]}","body":{},"response":{"code_field":"code","success_codes":[200,"200"],"msg_field":"message","yid_field":"data","success_use_upstream_msg":true,"failure_msg_on_success":true,"failure_msg_rules":[{"contains":"失败"}]}},"warnings":["isck=0 时 PHP 将 kcname 置 null，需业务侧处理","含 UPDATE 数据库逻辑无法表达"],"notes":"8090 继续教育"}

示例5b — 白泽 baize（code=0000）：
{"platform_type":"baize","rule_config":{"method":"POST","url":"{{huoyuan.url}}/api/v2/docking/add","content_type":"json","body":{"token":"{{huoyuan.token}}","platform_id":"{{order.noun}}","school":"{{order.school}}","account":"{{order.user}}","pwd":"{{order.pass}}","course_id":"{{order.kcid}}","course_name":"{{order.kcname}}","duration":"{{order.uTime}}","fraction":"{{order.uScore}}"},"response":{"code_field":"code","success_codes":["0000"],"msg_field":"msg","yid_path":"data.order_id","success_msg":"下单成功"}},"warnings":[],"notes":"白泽 curl JSON"}

示例5c — 西游 xiyou（status=success）：
{"platform_type":"xiyou","rule_config":{"method":"POST","url":"{{huoyuan.url}}/api/order/xiadanForPublic","content_type":"form","body":{"username":"{{huoyuan.user}}","token":"{{huoyuan.token}}","classId":"{{order.noun}}","schoolName":"{{order.school}}","user":"{{order.user}}","pass":"{{order.pass}}","courseName":"{{order.kcname}}","courseId":"{{order.kcid}}"},"response":{"code_field":"status","success_codes":["success"],"msg_field":"msg","success_msg":"下单成功"}},"warnings":[],"notes":"西游 status 字段"}

示例5d — YYY（code=200）：
{"platform_type":"yyy","rule_config":{"method":"POST","url":"{{huoyuan.url}}/api/order","content_type":"form","body":{"uid":"{{huoyuan.user}}","key":"{{huoyuan.pass}}","platform":"{{order.noun}}","school":"{{order.school}}","user":"{{order.user}}","pass":"{{order.pass}}","kcname":"{{order.kcname}}","kcid":"{{order.kcid}}"},"response":{"code_field":"code","success_codes":[200,"200"],"msg_field":"msg","yid_path":"data.yid","success_msg":"下单成功"}},"warnings":[],"notes":"YYY api/order"}

示例6 — jxjyyjy 嵌套 children：
{"platform_type":"jxjyyjy","rule_config":{"method":"POST","url":"{{huoyuan.url}}/api/order/buy","content_type":"json","headers":{"Content-Type":"application/json; charset=utf-8","Authorization":"Bearer {{huoyuan.token}}"},"body_mode":"raw","body_raw":"{\"websiteNumber\":\"{{order.noun}}\",\"data\":[{\"username\":\"{{order.user}}\",\"password\":\"{{order.pass}}\",\"children\":[{\"name\":\"{{order.kcname}}\"}]}]}","body":{},"response":{"code_field":"code","success_codes":[200,"200"],"msg_field":"message","yid_path":"data.orderList.0.orderId","success_msg":"下单成功"}},"warnings":["isck=0 时 kcname 置空"],"notes":"jxjyyjy 继续教育"}

示例7 — THOTH OpenAPI（Header 鉴权 + form）：
{"platform_type":"THOTH","rule_config":{"method":"POST","url":"{{huoyuan.url}}/api/open/add","content_type":"form","headers":{"X-Uid":"{{huoyuan.user}}","X-Api-Key":"{{huoyuan.pass}}"},"body":{"platform":"{{order.noun}}","school":"{{order.school}}","user":"{{order.user}}","pass":"{{order.pass}}","name":"{{order.name}}","kcname":"[\"{{order.kcname}}\"]","kcid":"[\"{{order.kcid}}\"]","score":"{{order.uScore}}","shichang":"{{order.uTime}}"},"response":{"code_field":"code","success_codes":["0",0],"msg_field":"msg","yid_field":"id","success_msg":"下单成功"}},"warnings":[],"notes":"THOTH OpenAPI /api/open/add"}

示例8 — tesla（cid 字段）：
{"platform_type":"tesla","rule_config":{"method":"POST","url":"{{huoyuan.url}}/api/api/external/submit-order","content_type":"form","body":{"uid":"{{huoyuan.user}}","key":"{{huoyuan.pass}}","cid":"{{order.noun}}","school":"{{order.school}}","user":"{{order.user}}","pass":"{{order.pass}}","kcname":"{{order.kcname}}","kcid":"{{order.kcid}}"},"response":{"code_field":"code","success_codes":["0",0],"msg_field":"msg","yid_field":"id","success_msg":"下单成功"}},"warnings":[],"notes":"tesla 用 cid 而非 platform"}

示例9 — coco JSON post：
{"platform_type":"coco","rule_config":{"method":"POST","url":"{{huoyuan.url}}/api/useAPI/addOrder","content_type":"json","body":{"uid":"{{huoyuan.user}}","key":"{{huoyuan.pass}}","platform":"{{order.noun}}","school":"{{order.school}}","user":"{{order.user}}","pass":"{{order.pass}}","kcname":"{{order.kcname}}","kcid":"{{order.kcid}}"},"response":{"code_field":"code","success_codes":["1",1],"msg_field":"msg","success_msg":"下单成功"}},"warnings":[],"notes":"coco JSON 下单"}

示例10 — 龙龙 longlong V2：
{"platform_type":"longlong","rule_config":{"method":"POST","url":"{{huoyuan.url}}/api/submit/{{order.noun}}","content_type":"json","headers":{"X-Uid":"{{huoyuan.user}}","X-Api-Key":"{{huoyuan.pass}}","Accept":"application/json"},"body_mode":"raw","body_raw":"{\"username\":\"{{order.user}}\",\"password\":\"{{order.pass}}\",\"courses\":[\"{{order.kcid}}\"]}","body":{},"response":{"success_http":true,"yid_path":"0","success_msg":"下单成功"},"process":{"handler":"http","method":"GET","url":"{{huoyuan.url}}/api/order/uuid/{{order.yid}}","headers":{"X-Uid":"{{huoyuan.user}}","X-Api-Key":"{{huoyuan.pass}}","Accept":"application/json"},"map":{"fields":{"yid":"uuid","kcname":"courseName","status_text":"status","process":"finish","remarks":"result","kcks":"courseStartTime","kcjs":"courseEndTime","ksks":"examStartTime","ksjs":"examEndTime"}}}},"warnings":["expand/city/tag/remark/config 按需补进 body_raw"],"notes":"龙龙 Open API V2"}

示例11 — pipeline 两步登录：
{"platform_type":"demo","rule_config":{"handler":"pipeline","method":"POST","url":"","content_type":"form","body":{},"response":{"code_field":"code","success_codes":["1",1],"msg_field":"msg","yid_field":"id","success_msg":"下单成功"},"pipeline":[{"action":"http","method":"POST","url":"{{huoyuan.url}}/login","content_type":"json","body":{"user":"{{huoyuan.user}}","pass":"{{huoyuan.pass}}"},"save_body_as":"login_body"},{"action":"extract","extract":{"from":"login_body","path":"data.token","to":"token"}},{"action":"finish","method":"POST","url":"{{huoyuan.url}}/submit","content_type":"json","body":{"token":"{{var.token}}","user":"{{order.user}}","pass":"{{order.pass}}","platform":"{{order.noun}}"}}]},"warnings":[],"notes":"两步 pipeline"}

示例12 — poll 轮询：
{"platform_type":"demo_poll","rule_config":{"handler":"pipeline","method":"POST","url":"","content_type":"form","body":{},"response":{"code_field":"code","success_codes":[1,"1"],"msg_field":"msg"},"pipeline":[{"action":"poll","method":"GET","url":"{{huoyuan.url}}/status?id={{order.yid}}","poll":{"interval_ms":2000,"max_attempts":15,"until":{"code_field":"status","success_codes":["done",1]}}}]},"warnings":[],"notes":"poll 查状态"}

示例13 — process 查课：
{"platform_type":"demo_cx","rule_config":{"method":"POST","url":"","content_type":"form","body":{},"response":{"code_field":"code","success_codes":[1],"msg_field":"msg"},"process":{"handler":"http","method":"GET","url":"{{huoyuan.url}}/query?yid={{order.yid}}","map":{"code_field":"code","success_codes":[0,"0"],"fields":{"kcname":"data.name","process":"data.progress","status_text":"data.status"}}}},"warnings":[],"notes":"ProcessOrder HTTP 查课"}

═══════════════════════════════════════
六、常见错误（务必避免）
═══════════════════════════════════════
1. success_codes 写错：hzw 是 1 不是 0；27/ssrs 是 0 不是 1；8090 是 200
2. body 里写嵌套 array/object — 必须用 body_raw
3. 硬编码域名 — 一律 {{huoyuan.url}}
4. 龙龙写成 api.php?act=add — 应是 /api/submit/{{order.noun}}
5. 两次 HTTP 硬塞进单次 http — 应 pipeline
6. strpos 映射只写 warnings — 应写 failure_msg_rules
7. 漏 yid_field/yid_path — PHP 有 yid 时必须填
8. content_type 误判：post($url, json_encode(...)) 是 json 不是 form
9. tesla 误用 platform — 应用 cid
10. THOTH 的 kcname/kcid 应是 JSON 字符串数组形式

═══════════════════════════════════════
七、输出规则
═══════════════════════════════════════
1. 只输出一个 JSON 对象，不要 markdown 代码块，不要任何前后说明文字
2. 尽量完整还原 URL、body、成功码；success_codes 必须与 PHP 判断一致
3. body 只能是扁平对象；PHP json_encode 嵌套结构一律 body_mode:raw + body_raw，body 写 {}
4. PHP 失败分支 strpos 映射写入 response.failure_msg_rules
5. 含 sleep、写数据库、array_rand、无法静态化的复杂分支，写入 warnings
6. PHP sleep(N) 用 delay_ms 或 pipeline delay（秒×1000），不是 poll
7. 多个 if 分支可合并为 branches 或选最主要的一段并 warning
8. PHP 含两次及以上 HTTP 必须用 handler:pipeline
9. 极复杂分支优先 pipeline/branches/kcid_json；仅无法用 JSON 表达时才用 script.source`
}
