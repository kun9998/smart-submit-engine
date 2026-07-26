package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type opsObservation struct {
	AuditID         int64
	StartedAt       time.Time
	Deadline        time.Time
	BaselineFailPct float64
	BaselineWindowFail uint64
	TargetHIDs      []int
	Snapshot        *opsExecSnapshot
}

var (
	opsObserveMu          sync.Mutex
	opsPendingObservations []opsObservation
)

func scheduleOpsObservation(auditID int64, snap *opsExecSnapshot, plan opsPlanDTO) {
	if snap == nil || len(snap.Actions) == 0 {
		return
	}
	cfg := getOpsConfig()
	if cfg.ObserveDurationMinutes <= 0 {
		return
	}
	hids := make([]int, 0)
	for _, item := range snap.Actions {
		switch item.Action {
		case "pause_channel", "adjust_workers", "enable_dlq_auto_retry":
			if item.HID > 0 {
				hids = append(hids, item.HID)
			}
		}
	}
	if len(hids) == 0 {
		return
	}

	ctx := context.Background()
	stats := collectEngineStats(ctx)
	var baselineFail uint64
	var baselineTotal uint64
	for _, ch := range stats.Channels {
		for _, hid := range hids {
			if ch.HID == hid {
				baselineFail += ch.WindowFail
				baselineTotal += ch.WindowSuccess + ch.WindowFail
			}
		}
	}
	baselinePct := 0.0
	if baselineTotal > 0 {
		baselinePct = float64(baselineFail) * 100 / float64(baselineTotal)
	}

	now := time.Now()
	obs := opsObservation{
		AuditID:            auditID,
		StartedAt:          now,
		Deadline:           now.Add(time.Duration(cfg.ObserveDurationMinutes) * time.Minute),
		BaselineFailPct:    baselinePct,
		BaselineWindowFail: baselineFail,
		TargetHIDs:         hids,
		Snapshot:           snap,
	}
	opsObserveMu.Lock()
	opsPendingObservations = append(opsPendingObservations, obs)
	opsObserveMu.Unlock()
	_ = plan
}

func runOpsObservationChecks(ctx context.Context) {
	cfg := getOpsConfig()
	if !cfg.Enabled {
		return
	}
	opsObserveMu.Lock()
	pending := append([]opsObservation(nil), opsPendingObservations...)
	opsObserveMu.Unlock()
	if len(pending) == 0 {
		return
	}

	now := time.Now()
	stats := collectEngineStats(ctx)
	remaining := make([]opsObservation, 0, len(pending))

	for _, obs := range pending {
		if now.Before(obs.Deadline) {
			if shouldRollbackOpsObservation(obs, stats) {
				rollbackOpsObservation(ctx, obs)
				continue
			}
			remaining = append(remaining, obs)
			continue
		}
		// 观测期结束，未触发回滚则移除
	}
	opsObserveMu.Lock()
	opsPendingObservations = remaining
	opsObserveMu.Unlock()
}

func shouldRollbackOpsObservation(obs opsObservation, stats engineStatsDTO) bool {
	var currentFail, currentTotal uint64
	for _, ch := range stats.Channels {
		for _, hid := range obs.TargetHIDs {
			if ch.HID != hid {
				continue
			}
			currentFail += ch.WindowFail
			currentTotal += ch.WindowSuccess + ch.WindowFail
		}
	}
	if currentTotal == 0 {
		return false
	}
	currentPct := float64(currentFail) * 100 / float64(currentTotal)
	if currentPct > obs.BaselineFailPct+5 {
		return true
	}
	if currentFail > obs.BaselineWindowFail && obs.BaselineWindowFail > 0 {
		// 失败数持续上升且未见改善
		if currentPct >= obs.BaselineFailPct {
			return true
		}
	}
	return false
}

func rollbackOpsObservation(ctx context.Context, obs opsObservation) {
	results, err := rollbackOpsSnapshot(ctx, obs.Snapshot)
	status := "rolled_back"
	errMsg := ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
	}
	_ = updateOpsAudit(ctx, obs.AuditID, status, actionResultsJSON(results), snapshotJSON(obs.Snapshot), errMsg)
	opsObserveMu.Lock()
	next := opsPendingObservations[:0]
	for _, o := range opsPendingObservations {
		if o.AuditID != obs.AuditID {
			next = append(next, o)
		}
	}
	opsPendingObservations = next
	opsObserveMu.Unlock()

	summary := fmt.Sprintf("观测期内指标恶化，已自动回滚 audit=%d", obs.AuditID)
	if cfg := getOpsConfig(); cfg.NotifyOnRollback {
		_ = opsPushNotify(ctx, "AI运维：自动回滚", summary)
	}
}
