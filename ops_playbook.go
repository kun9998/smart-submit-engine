package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type opsActionSpec struct {
	Action       string                 `json:"action"`
	Params       map[string]interface{} `json:"params,omitempty"`
	Risk         string                 `json:"risk,omitempty"`
	AutoExecute  bool                   `json:"auto_execute"`
	Reason       string                 `json:"reason,omitempty"`
}

type opsPlanDTO struct {
	IncidentType          string          `json:"incident_type"`
	Severity              string          `json:"severity"`
	Summary               string          `json:"summary"`
	RootCauseHypothesis   string          `json:"root_cause_hypothesis,omitempty"`
	Confidence            float64         `json:"confidence,omitempty"`
	RecommendedActions    []opsActionSpec `json:"recommended_actions"`
	ManualSuggestions     []string        `json:"manual_suggestions,omitempty"`
	MatchedPlaybook       string          `json:"matched_playbook,omitempty"`
	Source                string          `json:"source"`
}

func defaultOpsPlan(source, incidentType, severity, summary string) opsPlanDTO {
	return opsPlanDTO{
		IncidentType:       incidentType,
		Severity:           severity,
		Summary:            summary,
		RecommendedActions: []opsActionSpec{},
		ManualSuggestions:  []string{},
		Source:             source,
	}
}

func matchOpsPlaybooks(ctx opsContextDTO, events []string) *opsPlanDTO {
	cfg := getOpsConfig()
	if !cfg.PlaybooksEnabled {
		return nil
	}
	logs := ctx.RecentErrorLogs
	channels := enrichOpsChannels(ctx)

	eventSet := map[string]bool{}
	for _, e := range events {
		eventSet[e] = true
	}

	// PB-005: 终端业务错误，不重试
	if logHasTerminalBusinessError(logs) {
		plan := defaultOpsPlan("rule", "account_issue", "medium", "检测到终端业务错误（如余额不足、参数错误），不建议自动重试")
		plan.MatchedPlaybook = "PB-005"
		plan.RootCauseHypothesis = "货源返回不可重试的业务终态（余额/参数/重复下单等）"
		plan.ManualSuggestions = []string{"请人工核对货源账户余额与订单参数", "不建议开启 DLQ 自动重试", "不要 pause 渠道，修正账户或参数即可"}
		plan.RecommendedActions = []opsActionSpec{
			opsNotifyAction("AI运维：终端业务错误", plan.Summary, "medium", "终端错误需人工处理"),
		}
		return &plan
	}

	// PB-006: 404/CDN、401/403/IP 白名单、出站 host_whitelist — 只通知，不 pause、不重试
	if issue := detectOpsClientIssue(logs); issue != nil {
		plan := defaultOpsPlan("rule", issue.IncidentType, "medium", issue.Summary)
		plan.MatchedPlaybook = "PB-006"
		plan.RootCauseHypothesis = issue.Hypothesis
		plan.ManualSuggestions = issue.Suggestions
		plan.RecommendedActions = []opsActionSpec{
			opsNotifyAction("AI运维："+issue.NotifyTitle, plan.Summary, "medium", "配置/鉴权类错误需人工修正"),
		}
		return &plan
	}

	// PB-001: 上游 HTTP 故障 / 失败率突增
	if logHasUpstreamFault(logs) {
		for _, ch := range channels {
			keyHigh := "channel_fail_rate_high:" + strconv.Itoa(ch.HID)
			keySpike := "channel_fail_rate_spike:" + strconv.Itoa(ch.HID)
			if !eventSet[keyHigh] && !eventSet[keySpike] && ch.FailRatePct < cfg.Thresholds.ChannelFailRatePct {
				continue
			}
			if ch.Paused {
				continue
			}
			plan := defaultOpsPlan("rule", "upstream_outage", "high",
				fmt.Sprintf("HID %d（%s）疑似上游故障，建议暂停消费", ch.HID, ch.Name))
			plan.MatchedPlaybook = "PB-001"
			plan.Confidence = 0.85
			plan.RootCauseHypothesis = "上游返回 5xx/超时/连接异常"
			plan.RecommendedActions = []opsActionSpec{
				{
					Action: "pause_channel", Params: map[string]interface{}{"hid": ch.HID},
					AutoExecute: true, Risk: "low", Reason: "避免继续提交扩大 DLQ",
				},
				{
					Action: "notify", Params: map[string]interface{}{
						"title": "AI运维：上游故障", "body": plan.Summary, "severity": "high",
					},
					AutoExecute: true, Risk: "low",
				},
			}
			return &plan
		}
	}

	// PB-001b: 失败率突增（配置/鉴权/终端类错误时不 pause，避免误止血）
	if !logHasNonRetryableOpsError(logs) {
		for _, ch := range channels {
			keySpike := "channel_fail_rate_spike:" + strconv.Itoa(ch.HID)
			if !eventSet[keySpike] || ch.Paused {
				continue
			}
			if ch.FailRatePct < 20 {
				continue
			}
			plan := defaultOpsPlan("rule", "upstream_outage", "high",
				fmt.Sprintf("HID %d（%s）失败率 5 分钟内突增，建议暂停消费", ch.HID, ch.Name))
			plan.MatchedPlaybook = "PB-001b"
			plan.RecommendedActions = []opsActionSpec{
				{
					Action: "pause_channel", Params: map[string]interface{}{"hid": ch.HID},
					AutoExecute: true, Risk: "low", Reason: "失败率突增止血",
				},
				opsNotifyAction("AI运维：失败率突增", plan.Summary, "high", "失败率突增"),
			}
			return &plan
		}
	}

	// PB-002: DLQ 堆积（瞬态错误才建议自动重试）
	for _, ch := range channels {
		key := "dlq_depth_high:" + strconv.Itoa(ch.HID)
		if !eventSet[key] && ch.DLQDepth < cfg.Thresholds.DLQDepth {
			continue
		}
		if logHasUpstreamFault(logs) || logHasNonRetryableOpsError(logs) {
			continue
		}
		plan := defaultOpsPlan("rule", "backlog", "medium",
			fmt.Sprintf("HID %d（%s）DLQ 堆积 %d，建议开启 DLQ 自动重试", ch.HID, ch.Name, ch.DLQDepth))
		plan.MatchedPlaybook = "PB-002"
		plan.RecommendedActions = []opsActionSpec{
			{
				Action: "enable_dlq_auto_retry",
				Params: map[string]interface{}{"hid": ch.HID, "enabled": true, "min_age_minutes": 30},
				AutoExecute: true, Risk: "low", Reason: "transient 错误可自动重投",
			},
		}
		return &plan
	}

	// PB-003: 队列积压
	if ctx.Engine.Connections.Redis.Ready && ctx.Engine.Connections.MainMySQL.Ready {
		for _, ch := range channels {
			key := "queue_backlog_high:" + strconv.Itoa(ch.HID)
			if !eventSet[key] {
				continue
			}
			if ch.Paused {
				continue
			}
			plan := defaultOpsPlan("rule", "backlog", "medium",
				fmt.Sprintf("HID %d（%s）队列积压 %d，建议增加 worker", ch.HID, ch.Name, ch.QueueDepth))
			plan.MatchedPlaybook = "PB-003"
			plan.RecommendedActions = []opsActionSpec{
				{
					Action: "adjust_workers", Params: map[string]interface{}{"hid": ch.HID, "delta": 2},
					AutoExecute: true, Risk: "low", Reason: "提升消费能力",
				},
			}
			return &plan
		}
	}

	// PB-004: Redis 异常
	if eventSet["redis_unhealthy"] {
		plan := defaultOpsPlan("rule", "infra_fault", "critical", "Redis 连接异常，请人工检查配置与网络")
		plan.MatchedPlaybook = "PB-004"
		plan.ManualSuggestions = []string{"检查 config.yaml 中 Redis 地址与密码", "确认 Redis 服务可达"}
		plan.RecommendedActions = []opsActionSpec{
			{Action: "notify", Params: map[string]interface{}{"title": "AI运维：Redis 异常", "body": plan.Summary, "severity": "critical"}, AutoExecute: true, Risk: "low"},
		}
		return &plan
	}

	if eventSet["main_mysql_unhealthy"] {
		plan := defaultOpsPlan("rule", "infra_fault", "critical", "主库 MySQL 连接异常，请人工检查")
		plan.MatchedPlaybook = "PB-004"
		plan.ManualSuggestions = []string{"检查主库 DSN 与网络", "确认数据库服务正常"}
		plan.RecommendedActions = []opsActionSpec{
			{Action: "notify", Params: map[string]interface{}{"title": "AI运维：MySQL 异常", "body": plan.Summary, "severity": "critical"}, AutoExecute: true, Risk: "low"},
		}
		return &plan
	}

	return nil
}

func opsPlanJSON(plan opsPlanDTO) []byte {
	b, _ := json.Marshal(plan)
	return b
}

func mergeOpsPlans(rulePlan, aiPlan *opsPlanDTO) opsPlanDTO {
	if rulePlan != nil {
		return *rulePlan
	}
	if aiPlan != nil {
		return *aiPlan
	}
	return defaultOpsPlan("manual", "unknown", "low", "未发现明显异常，系统运行正常")
}

func planHasAutoActions(plan opsPlanDTO) bool {
	for _, a := range plan.RecommendedActions {
		if a.AutoExecute && strings.TrimSpace(a.Action) != "" && a.Action != "noop" {
			return true
		}
	}
	return false
}
