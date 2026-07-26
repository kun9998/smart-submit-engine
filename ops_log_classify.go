package main

import (
	"strings"
)

type opsClientIssue struct {
	IncidentType string
	Summary      string
	Hypothesis   string
	NotifyTitle  string
	Suggestions  []string
}

func logMessageLower(msg string) string {
	return strings.ToLower(strings.TrimSpace(msg))
}

func logMatchesAny(msg string, keywords ...string) bool {
	m := logMessageLower(msg)
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if strings.Contains(m, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func detectOutboundPolicyIssue(logs []opsLogEntryDTO) *opsClientIssue {
	for _, e := range logs {
		if !logMatchesAny(e.Message,
			"目标域名不在白名单",
			"e_host_not_whitelist",
			"host not whitelist",
		) {
			continue
		}
		return &opsClientIssue{
			IncidentType: "outbound_policy",
			Summary:      "检测到出站域名不在 host_whitelist，订单无法提交",
			Hypothesis:   "config.yaml 的 host_whitelist 未包含目标货源域名",
			NotifyTitle:  "出站白名单限制",
			Suggestions: []string{
				"检查 config.yaml 中 security.host_whitelist 是否包含货源域名",
				"修改后无需 pause 渠道，修正白名单即可恢复",
				"不要开启 DLQ 自动重试",
			},
		}
	}
	return nil
}

func detectAuthClientIssue(logs []opsLogEntryDTO) *opsClientIssue {
	for _, e := range logs {
		if !logMatchesAny(e.Message,
			"上游接口未授权(401)",
			"上游接口拒绝访问(403)",
			"e_http_status_401",
			"e_http_status_403",
			"(401)",
			"(403)",
			"未授权",
			"拒绝访问",
			"ip白名单",
			"ip 白名单",
			"白名单限制",
			"access denied",
			"forbidden",
			"unauthorized",
			"cloudflare",
			"waf",
		) {
			continue
		}
		return &opsClientIssue{
			IncidentType: "auth_error",
			Summary:      "检测到上游鉴权/访问拒绝（401/403/IP 白名单/WAF），需人工处理",
			Hypothesis:   "上游 API Key 失效、IP 未加白、或 CDN/WAF 拦截",
			NotifyTitle:  "鉴权或访问拒绝",
			Suggestions: []string{
				"核对货源 API 密钥、签名参数是否正确",
				"联系上游将服务器出口 IP 加入白名单",
				"若返回 Cloudflare/WAF 页面，检查请求头与频率限制",
				"不要 pause 渠道或开启 DLQ 自动重试，修正配置后再观察",
			},
		}
	}
	return nil
}

func detectConfigClientIssue(logs []opsLogEntryDTO) *opsClientIssue {
	for _, e := range logs {
		if !logMatchesAny(e.Message,
			"上游接口地址不存在(404)",
			"e_http_status_404",
			"(404)",
			"接口地址不存在",
			"地址不存在",
			"请求地址格式错误",
			"e_url_invalid",
			"e_url_host",
			"e_url_scheme",
			"cdn",
			"not found",
		) {
			continue
		}
		return &opsClientIssue{
			IncidentType: "config_error",
			Summary:      "检测到上游路径/地址错误（404/CDN 拦截/URL 配置问题），需修正规则或货源地址",
			Hypothesis:   "平台规则 URL 错误、货源路径变更，或 CDN 返回 404 拦截页",
			NotifyTitle:  "接口地址或 CDN 异常",
			Suggestions: []string{
				"核对平台规则中的下单 URL、act 参数是否与货源文档一致",
				"在浏览器或 curl 验证接口是否可达",
				"若 CDN 返回 HTML 404 页，联系货源或更换正确 endpoint",
				"不要 pause 渠道或开启 DLQ 自动重试",
			},
		}
	}
	return nil
}

func detectOpsClientIssue(logs []opsLogEntryDTO) *opsClientIssue {
	if issue := detectOutboundPolicyIssue(logs); issue != nil {
		return issue
	}
	if issue := detectAuthClientIssue(logs); issue != nil {
		return issue
	}
	if issue := detectConfigClientIssue(logs); issue != nil {
		return issue
	}
	return nil
}

func logHasNonRetryableOpsError(logs []opsLogEntryDTO) bool {
	return logHasTerminalBusinessError(logs) || detectOpsClientIssue(logs) != nil
}

func opsNotifyAction(title, body, severity, reason string) opsActionSpec {
	return opsActionSpec{
		Action: "notify",
		Params: map[string]interface{}{
			"title":    title,
			"body":     body,
			"severity": severity,
		},
		AutoExecute: true,
		Risk:        "low",
		Reason:      reason,
	}
}
