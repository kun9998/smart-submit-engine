package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type opsActionResult struct {
	Action  string `json:"action"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type opsExecSnapshot struct {
	Actions []opsExecSnapshotItem `json:"actions"`
}

type opsExecSnapshotItem struct {
	Action        string                 `json:"action"`
	HID           int                    `json:"hid,omitempty"`
	WorkersBefore int                    `json:"workers_before,omitempty"`
	PausedBefore  bool                   `json:"paused_before,omitempty"`
	RuntimeBefore *RuntimeConfigPayload  `json:"runtime_before,omitempty"`
	RuntimePatch  *RuntimeConfigPayload  `json:"runtime_patch,omitempty"`
}

func executeOpsActions(ctx context.Context, actions []opsActionSpec) ([]opsActionResult, *opsExecSnapshot, error) {
	results := make([]opsActionResult, 0, len(actions))
	snap := &opsExecSnapshot{Actions: make([]opsExecSnapshotItem, 0, len(actions))}

	for _, a := range actions {
		item, res := executeOpsAction(ctx, a)
		results = append(results, res)
		if item != nil {
			snap.Actions = append(snap.Actions, *item)
		}
		if res.OK {
			markOpsActionExecuted(a.Action, opsActionHID(a))
		}
	}
	return results, snap, nil
}

func executeOpsAction(ctx context.Context, a opsActionSpec) (*opsExecSnapshotItem, opsActionResult) {
	action := strings.TrimSpace(a.Action)
	res := opsActionResult{Action: action, OK: false}
	var snap *opsExecSnapshotItem

	switch action {
	case "pause_channel":
		hid, err := parseActionHID(a.Params)
		if err != nil {
			res.Message = err.Error()
			return nil, res
		}
		snap = snapshotBeforeMutate(hid, action, nil)
		if err := opsPauseChannel(hid); err != nil {
			res.Message = err.Error()
			return snap, res
		}
		resetOpsResumeStable(hid)
		res.OK = true
		res.Message = fmt.Sprintf("HID %d 已暂停消费", hid)
	case "resume_channel":
		hid, err := parseActionHID(a.Params)
		if err != nil {
			res.Message = err.Error()
			return nil, res
		}
		snap = snapshotBeforeMutate(hid, action, nil)
		if err := opsResumeChannel(hid); err != nil {
			res.Message = err.Error()
			return snap, res
		}
		res.OK = true
		res.Message = fmt.Sprintf("HID %d 已恢复消费", hid)
	case "adjust_workers":
		hid, err := parseActionHID(a.Params)
		if err != nil {
			res.Message = err.Error()
			return nil, res
		}
		delta := parseActionIntParam(a.Params, "delta", 0)
		snap = snapshotBeforeMutate(hid, action, nil)
		if err := opsAdjustWorkers(hid, delta); err != nil {
			res.Message = err.Error()
			return snap, res
		}
		res.OK = true
		res.Message = fmt.Sprintf("HID %d worker 调整 %+d", hid, delta)
	case "enable_dlq_auto_retry":
		hid, _ := parseActionHID(a.Params)
		enabled := parseActionBoolParam(a.Params, "enabled", true)
		minAge := parseActionIntParam(a.Params, "min_age_minutes", 30)
		patch := &RuntimeConfigPayload{
			Resubmit: &RuntimeResubmitSection{
				DLQAutoRetry: &RuntimeDLQAutoRetrySection{
					Enabled:             boolPtr(enabled),
					MinAgeMinutes:       intPtr(minAge),
					ScanIntervalMinutes: intPtr(30),
					MaxPerScan:          intPtr(50),
				},
			},
		}
		snap = snapshotBeforeMutate(hid, action, patch)
		if hid > 0 {
			if err := saveRuntimeHIDConfig(ctx, hid, patch, ""); err != nil {
				res.Message = err.Error()
				return snap, res
			}
		} else {
			if err := saveRuntimeGlobalConfig(ctx, patch); err != nil {
				res.Message = err.Error()
				return snap, res
			}
		}
		res.OK = true
		res.Message = "已更新 DLQ 自动重试配置"
	case "reload_rules":
		n, err := reloadSubmitRulesAndRegister(ctx)
		if err != nil {
			res.Message = err.Error()
			return nil, res
		}
		res.OK = true
		res.Message = fmt.Sprintf("已 reload %d 条规则", n)
	case "notify":
		title := "AI运维"
		body := a.Reason
		severity := "medium"
		if a.Params != nil {
			if s, ok := a.Params["title"].(string); ok && strings.TrimSpace(s) != "" {
				title = s
			}
			if s, ok := a.Params["body"].(string); ok && strings.TrimSpace(s) != "" {
				body = s
			}
			if s, ok := a.Params["severity"].(string); ok {
				severity = s
			}
		}
		if strings.TrimSpace(body) == "" {
			body = "运维事件通知"
		}
		body = fmt.Sprintf("[%s] %s", strings.ToUpper(severity), body)
		if err := opsPushNotify(ctx, title, body); err != nil {
			res.Message = err.Error()
			return nil, res
		}
		res.OK = true
		res.Message = "通知已发送"
	case "noop":
		res.OK = true
		res.Message = a.Reason
	default:
		res.Message = "未支持的动作: " + action
	}
	return snap, res
}

func snapshotBeforeMutate(hid int, action string, patch *RuntimeConfigPayload) *opsExecSnapshotItem {
	item := &opsExecSnapshotItem{Action: action, HID: hid, RuntimePatch: patch}
	concurrencyMu.RLock()
	item.WorkersBefore = currWorkers[hid]
	concurrencyMu.RUnlock()
	item.PausedBefore = opsIsChannelPaused(hid)
	if patch != nil {
		if hid > 0 {
			item.RuntimeBefore = cloneRuntimeConfig(getEffectiveMergedConfig(hid))
		} else {
			item.RuntimeBefore = cloneRuntimeConfig(getEffectiveMergedConfig(0))
		}
	}
	return item
}

func opsPushNotify(ctx context.Context, title, content string) error {
	url := strings.TrimSpace(alertShowdocURL)
	if url == "" {
		loadAlertShowdocFromPluginDB(ctx)
		url = strings.TrimSpace(alertShowdocURL)
	}
	if url == "" {
		return fmt.Errorf("未配置 Showdoc 推送地址")
	}
	cfg := getOpsConfig()
	if !cfg.NotifyOnAutoAction && !cfg.NotifyOnRollback {
		// still allow if explicitly requested via notify action
	}
	return pushShowdoc(ctx, url, title, content)
}

func rollbackOpsSnapshot(ctx context.Context, snap *opsExecSnapshot) ([]opsActionResult, error) {
	if snap == nil || len(snap.Actions) == 0 {
		return nil, fmt.Errorf("无可回滚快照")
	}
	results := make([]opsActionResult, 0, len(snap.Actions))
	for i := len(snap.Actions) - 1; i >= 0; i-- {
		item := snap.Actions[i]
		res := opsActionResult{Action: "rollback:" + item.Action}
		switch item.Action {
		case "pause_channel":
			if item.PausedBefore {
				err := opsPauseChannel(item.HID)
				res.OK = err == nil
				if err != nil {
					res.Message = err.Error()
				} else {
					res.Message = fmt.Sprintf("HID %d 已重新暂停", item.HID)
				}
			} else {
				err := opsResumeChannel(item.HID)
				res.OK = err == nil
				if err != nil {
					res.Message = err.Error()
				} else {
					res.Message = fmt.Sprintf("HID %d 已恢复消费", item.HID)
				}
			}
		case "enable_dlq_auto_retry":
			if item.RuntimeBefore != nil {
				var err error
				if item.HID > 0 {
					err = saveRuntimeHIDConfig(ctx, item.HID, item.RuntimeBefore, "")
				} else {
					err = saveRuntimeGlobalConfig(ctx, item.RuntimeBefore)
				}
				res.OK = err == nil
				if err != nil {
					res.Message = err.Error()
				} else {
					res.Message = "运行时配置已恢复"
				}
			}
		case "adjust_workers":
			concurrencyMu.RLock()
			curr := currWorkers[item.HID]
			concurrencyMu.RUnlock()
			delta := item.WorkersBefore - curr
			err := opsAdjustWorkers(item.HID, delta)
			res.OK = err == nil
			if err != nil {
				res.Message = err.Error()
			} else {
				res.Message = fmt.Sprintf("worker 已恢复为 %d", item.WorkersBefore)
			}
		default:
			res.OK = true
			res.Message = "无需回滚"
		}
		results = append(results, res)
	}
	cfg := getOpsConfig()
	if cfg.NotifyOnRollback {
		_ = opsPushNotify(ctx, "AI运维：已回滚", "自动运维动作已回滚，请查看审计日志")
	}
	return results, nil
}

func snapshotJSON(snap *opsExecSnapshot) []byte {
	if snap == nil {
		return nil
	}
	b, err := json.Marshal(snap)
	if err != nil {
		log.Printf("[AI运维] 快照序列化失败: %v", err)
	}
	return b
}

func actionResultsJSON(results []opsActionResult) []byte {
	b, _ := json.Marshal(results)
	return b
}
