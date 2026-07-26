package main

import (
	"context"
	"fmt"
	"strconv"
	"sync"
)

var (
	opsResumeStableCount = map[int]int{}
	opsResumeStableMu    sync.Mutex
)

func resetOpsResumeStable(hid int) {
	opsResumeStableMu.Lock()
	delete(opsResumeStableCount, hid)
	opsResumeStableMu.Unlock()
}

func requiredOpsResumeStableScans() int {
	cfg := getOpsConfig()
	sec := cfg.ScanIntervalSeconds
	if sec <= 0 {
		sec = 60
	}
	minutes := cfg.Thresholds.ResumeStableMinutes
	if minutes <= 0 {
		minutes = 10
	}
	n := minutes * 60 / sec
	if n < 1 {
		n = 1
	}
	return n
}

func runOpsAutoResumeCheck(ctx context.Context) {
	cfg := getOpsConfig()
	if !cfg.Enabled {
		return
	}
	paused := listPausedChannelHIDs()
	if len(paused) == 0 {
		return
	}

	opsCtx := collectOpsContext(ctx, "auto_resume")
	channels := enrichOpsChannels(opsCtx)
	byHID := make(map[int]opsChannelContextDTO, len(channels))
	for _, ch := range channels {
		byHID[ch.HID] = ch
	}

	required := requiredOpsResumeStableScans()
	resumeFailPct := cfg.Thresholds.ResumeFailRatePct
	if resumeFailPct <= 0 {
		resumeFailPct = 10
	}
	minEvents := cfg.Thresholds.ResumeMinWindowEvents
	if minEvents <= 0 {
		minEvents = 5
	}
	stableMinutes := cfg.Thresholds.ResumeStableMinutes
	if stableMinutes <= 0 {
		stableMinutes = 10
	}
	for _, hid := range paused {
		ch, ok := byHID[hid]
		if !ok {
			resetOpsResumeStable(hid)
			continue
		}
		total := ch.WindowSuccess + ch.WindowFail
		if total < uint64(minEvents) {
			resetOpsResumeStable(hid)
			continue
		}
		if ch.FailRatePct >= resumeFailPct {
			resetOpsResumeStable(hid)
			continue
		}

		opsResumeStableMu.Lock()
		opsResumeStableCount[hid]++
		count := opsResumeStableCount[hid]
		opsResumeStableMu.Unlock()
		if count < required {
			continue
		}

		if err := opsResumeChannel(hid); err != nil {
			resetOpsResumeStable(hid)
			continue
		}
		resetOpsResumeStable(hid)
		resetOpsFailRateHistoryForHID(hid)

		summary := fmt.Sprintf("HID %d（%s）失败率已降至 %.1f%% 并稳定 %d 分钟，已自动恢复消费",
			hid, ch.Name, ch.FailRatePct, stableMinutes)
		plan := defaultOpsPlan("rule", "upstream_outage", "low", summary)
		plan.MatchedPlaybook = "PB-001-resume"
		var execResults []opsActionResult
		if cfg.NotifyOnAutoAction {
			notifyPlan := []opsActionSpec{{
				Action: "notify",
				Params: map[string]interface{}{
					"title":    "AI运维：渠道已恢复",
					"body":     summary,
					"severity": "low",
				},
				AutoExecute: true,
				Risk:        "low",
			}}
			execResults, _, _ = executeOpsActions(ctx, notifyPlan)
		}
		_, _ = insertOpsAuditQuiet(ctx, opsAuditInsert{
			TriggerType:     "auto_resume:" + strconv.Itoa(hid),
			Source:          "rule",
			Severity:        "low",
			IncidentType:    "upstream_outage",
			Summary:         summary,
			Operator:        "watcher",
			ContextJSON:     opsContextJSON(opsCtx),
			PlanJSON:        opsPlanJSON(plan),
			Status:          "executed",
			ExecutedActions: actionResultsJSON(execResults),
		})
	}
}

func insertOpsAuditQuiet(ctx context.Context, row opsAuditInsert) (int64, error) {
	if pluginDB == nil {
		return 0, nil
	}
	return insertOpsAudit(ctx, row)
}
