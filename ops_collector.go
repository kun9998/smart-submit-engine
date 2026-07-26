package main

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type opsLogEntryDTO struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

type opsChannelContextDTO struct {
	HID           int     `json:"hid"`
	Name          string  `json:"name"`
	QueueDepth    int64   `json:"queue_depth"`
	ProcessingDepth int64 `json:"processing_depth"`
	DLQDepth      int64   `json:"dlq_depth"`
	Workers       int     `json:"workers"`
	WindowSuccess uint64  `json:"window_success"`
	WindowFail    uint64  `json:"window_fail"`
	WindowDLQ     uint64  `json:"window_dlq"`
	FailRatePct   float64 `json:"fail_rate_pct"`
	Paused        bool    `json:"paused"`
}

type opsContextDTO struct {
	CollectedAt      time.Time              `json:"collected_at"`
	Trigger          string                 `json:"trigger"`
	ProductVersion   string                 `json:"product_version"`
	Monitor          interface{}            `json:"monitor,omitempty"`
	Engine           engineStatsDTO         `json:"engine"`
	RecentErrorLogs  []opsLogEntryDTO       `json:"recent_error_logs"`
	RuntimeSummary   map[string]interface{} `json:"runtime_summary"`
	ActiveOpsState   map[string]interface{} `json:"active_ops_state"`
}

var opsSensitiveQueryRe = regexp.MustCompile(`(?i)([?&])(key|token|sign|pass|password|secret|auth)=([^&\s]+)`)

func collectOpsContext(ctx context.Context, trigger string) opsContextDTO {
	out := opsContextDTO{
		CollectedAt:    time.Now(),
		Trigger:        trigger,
		ProductVersion: getProductVersion(),
		Engine:         collectEngineStats(ctx),
		RuntimeSummary: collectOpsRuntimeSummary(),
		ActiveOpsState: map[string]interface{}{
			"paused_hids": listPausedChannelHIDs(),
		},
	}
	out.Monitor = collectSystemMonitor()
	out.RecentErrorLogs = collectOpsRecentLogs(200)
	return out
}

func collectOpsRuntimeSummary() map[string]interface{} {
	resubmit := getEffectiveResubmitForHID(0)
	runtimeConfigMu.RLock()
	hidCount := len(runtimeHIDOverrides)
	runtimeConfigMu.RUnlock()
	return map[string]interface{}{
		"dlq_auto_retry_enabled": resubmit.DLQAutoRetry.Enabled,
		"hid_overrides_count":    hidCount,
	}
}

func collectOpsRecentLogs(limit int) []opsLogEntryDTO {
	entries := appLogHub.recent(limit)
	out := make([]opsLogEntryDTO, 0, 64)
	for _, e := range entries {
		if e.Level != "error" && e.Level != "warn" {
			continue
		}
		out = append(out, opsLogEntryDTO{
			Time:    e.Time,
			Level:   e.Level,
			Message: redactOpsLogMessage(e.Message),
		})
		if len(out) >= 64 {
			break
		}
	}
	return out
}

func redactOpsLogMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 500 {
		msg = msg[:500] + "…"
	}
	msg = opsSensitiveQueryRe.ReplaceAllString(msg, `${1}${2}=***`)
	for _, key := range []string{"pass=", "password=", "token=", "apikey=", "api_key="} {
		if idx := strings.Index(strings.ToLower(msg), key); idx >= 0 {
			end := strings.IndexAny(msg[idx:], " \t,&")
			if end < 0 {
				msg = msg[:idx] + key + "***"
			} else {
				msg = msg[:idx+len(key)] + "***" + msg[idx+end:]
			}
		}
	}
	return msg
}

func opsContextJSON(ctx opsContextDTO) []byte {
	b, _ := json.Marshal(ctx)
	return b
}

func channelFailRate(ch engineChannelDTO) float64 {
	total := ch.WindowSuccess + ch.WindowFail
	if total == 0 {
		return 0
	}
	return float64(ch.WindowFail) * 100 / float64(total)
}

func enrichOpsChannels(ctx opsContextDTO) []opsChannelContextDTO {
	out := make([]opsChannelContextDTO, 0, len(ctx.Engine.Channels))
	paused := pausedChannelSet()
	for _, ch := range ctx.Engine.Channels {
		out = append(out, opsChannelContextDTO{
			HID:             ch.HID,
			Name:            ch.Name,
			QueueDepth:      ch.QueueDepth,
			ProcessingDepth: ch.ProcessingDepth,
			DLQDepth:        ch.DLQDepth,
			Workers:         ch.Workers,
			WindowSuccess:   ch.WindowSuccess,
			WindowFail:      ch.WindowFail,
			WindowDLQ:       ch.WindowDLQ,
			FailRatePct:     channelFailRate(ch),
			Paused:          paused[ch.HID],
		})
	}
	return out
}

func detectOpsEvents(ctx opsContextDTO) []string {
	cfg := getOpsConfig()
	events := make([]string, 0, 4)
	channels := enrichOpsChannels(ctx)

	if !ctx.Engine.Connections.Redis.Ready {
		events = append(events, "redis_unhealthy")
	}
	if !ctx.Engine.Connections.MainMySQL.Ready {
		events = append(events, "main_mysql_unhealthy")
	}
	if OrderEngineReady() && !ctx.Engine.EngineRunning {
		events = append(events, "engine_stopped")
	}

	for _, ch := range channels {
		if ch.WindowFail >= 10 && ch.FailRatePct >= cfg.Thresholds.ChannelFailRatePct {
			events = append(events, "channel_fail_rate_high:"+strconv.Itoa(ch.HID))
		}
		if ch.DLQDepth >= cfg.Thresholds.DLQDepth {
			events = append(events, "dlq_depth_high:"+strconv.Itoa(ch.HID))
		}
		qCfg := getEffectiveQueueForHID(ch.HID)
		if ch.QueueDepth >= cfg.Thresholds.QueueBacklog && ch.Workers >= qCfg.MaxWorkersPerHID {
			events = append(events, "queue_backlog_high:"+strconv.Itoa(ch.HID))
		}
	}

	recordOpsFailRateSamples(channels)
	events = append(events, detectChannelFailRateSpikes(channels, cfg.Thresholds.ChannelFailRateSpikePP)...)

	if logHasTerminalBusinessError(ctx.RecentErrorLogs) {
		events = append(events, "terminal_business_error")
	}
	if issue := detectOpsClientIssue(ctx.RecentErrorLogs); issue != nil {
		events = append(events, "client_"+issue.IncidentType)
	}
	return events
}

func logHasUpstreamFault(logs []opsLogEntryDTO) bool {
	for _, e := range logs {
		m := strings.ToLower(e.Message)
		for _, kw := range []string{"502", "503", "504", "connection reset", "timeout", "超时", "eof", "connection refused"} {
			if strings.Contains(m, kw) {
				return true
			}
		}
	}
	return false
}

func logHasTerminalBusinessError(logs []opsLogEntryDTO) bool {
	kws := getEffectiveResubmitForHID(0).TerminalKeywords
	for _, e := range logs {
		msg := logMessageLower(e.Message)
		for _, kw := range kws {
			kw = strings.TrimSpace(kw)
			if kw != "" && strings.Contains(msg, strings.ToLower(kw)) {
				return true
			}
		}
	}
	return false
}
