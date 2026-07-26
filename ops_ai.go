package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	opsAIRateMu sync.Mutex
	opsAICalls  []time.Time
)

func allowOpsAI() bool {
	cfg := getOpsConfig()
	limit := cfg.AIRateLimitPerHour
	if limit <= 0 {
		limit = 20
	}
	now := time.Now()
	opsAIRateMu.Lock()
	defer opsAIRateMu.Unlock()
	kept := make([]time.Time, 0, len(opsAICalls))
	for _, t := range opsAICalls {
		if now.Sub(t) < time.Hour {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		opsAICalls = kept
		return false
	}
	kept = append(kept, now)
	opsAICalls = kept
	return true
}

func analyzeOpsWithAI(ctx context.Context, opsCtx opsContextDTO, events []string) (*opsPlanDTO, error) {
	if !opsAIReady() {
		return nil, fmt.Errorf("AI 运维未启用或未配置 API Key")
	}
	if !allowOpsAI() {
		return nil, fmt.Errorf("AI 运维调用过于频繁，请稍后再试")
	}

	cfg := getAIConfig()
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	model := normalizeDeepSeekModel(strings.TrimSpace(cfg.Model))
	if strings.TrimSpace(getOpsConfig().OpsModel) != "" {
		model = normalizeDeepSeekModel(strings.TrimSpace(getOpsConfig().OpsModel))
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	endpoint := aiChatCompletionsURL(baseURL)
	if err := ValidateOutboundHTTPURL(ctx, endpoint); err != nil {
		return nil, fmt.Errorf("AI 接口地址被安全策略拦截")
	}

	ctxJSON, _ := json.Marshal(opsCtx)
	userPrompt := fmt.Sprintf("以下是订单引擎运维上下文（JSON）。触发事件: %s\n\n%s\n\n请输出 OpsPlan JSON。",
		strings.Join(events, ", "), string(ctxJSON))

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": opsAISystemPrompt()},
			{"role": "user", "content": userPrompt},
		},
	}
	applyAIRuleConversionOptions(payload, baseURL, model)
	if maxTok := getOpsConfig().OpsMaxTokens; maxTok > 0 {
		payload["max_tokens"] = maxTok
	} else {
		payload["max_tokens"] = 2048
	}

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

	client := NewOutboundHTTPClient(30 * time.Second)
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
	plan, err := parseOpsPlanJSON(rawJSON)
	if err != nil {
		log.Printf("[AI运维] JSON解析失败: %v snippet=%q", err, truncateForLog(rawJSON, 400))
		return nil, fmt.Errorf("AI 返回的 JSON 无法解析")
	}
	plan.Source = "ai"
	return plan, nil
}

func opsAISystemPrompt() string {
	return `你是订单提交引擎的 SRE 助手。根据监控、引擎统计与错误日志，输出 JSON 处置方案。
规则：
1. 只能推荐以下 action：pause_channel、resume_channel、adjust_workers、enable_dlq_auto_retry、reload_rules、notify、noop
2. 禁止：升级、重启、修改 Redis/MySQL、修改平台规则、清空 DLQ
3. 上游 502/503/504/超时/连接异常 → 优先考虑 pause_channel
4. 终端业务错误（余额不足、参数错误、重复下单）→ 只 notify，不要 pause_channel，不要 enable_dlq_auto_retry
5. 404/接口地址不存在/CDN 拦截页 → config_error，只 notify，建议核对平台规则 URL，不要 pause 或 DLQ 重试
6. 401/403/IP 白名单/WAF/Cloudflare → auth_error，只 notify，建议核对密钥与上游 IP 白名单，不要 pause 或 DLQ 重试
7. 「目标域名不在白名单」→ outbound_policy，提示修改 config host_whitelist，不要 pause 或 DLQ 重试
8. 不确定时 auto_execute=false，写入 manual_suggestions
9. 输出纯 JSON，字段：incident_type、severity、summary、root_cause_hypothesis、confidence、recommended_actions（含 action、params、risk、auto_execute、reason）、manual_suggestions、matched_playbook`
}

func parseOpsPlanJSON(raw string) (*opsPlanDTO, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty")
	}
	var plan opsPlanDTO
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, err
	}
	plan.IncidentType = strings.TrimSpace(plan.IncidentType)
	if plan.IncidentType == "" {
		plan.IncidentType = "unknown"
	}
	plan.Severity = strings.TrimSpace(plan.Severity)
	if plan.Severity == "" {
		plan.Severity = "low"
	}
	plan.Summary = strings.TrimSpace(plan.Summary)
	if plan.Summary == "" {
		plan.Summary = "AI 分析完成"
	}
	return &plan, nil
}
