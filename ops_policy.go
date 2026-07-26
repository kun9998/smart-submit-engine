package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	opsActionCooldownMu sync.Mutex
	opsLastActionAt     = map[string]time.Time{}
)

func validateOpsPlan(plan opsPlanDTO) (opsPlanDTO, []string) {
	cfg := getOpsConfig()
	warnings := make([]string, 0)
	if len(plan.RecommendedActions) > cfg.Policy.MaxActionsPerPlan {
		plan.RecommendedActions = plan.RecommendedActions[:cfg.Policy.MaxActionsPerPlan]
		warnings = append(warnings, fmt.Sprintf("动作数超过上限，已截断为 %d 项", cfg.Policy.MaxActionsPerPlan))
	}
	filtered := make([]opsActionSpec, 0, len(plan.RecommendedActions))
	for _, a := range plan.RecommendedActions {
		action := strings.TrimSpace(a.Action)
		if action == "" || action == "noop" {
			continue
		}
		if !opsActionAllowed(action) {
			warnings = append(warnings, "拒绝未授权动作: "+action)
			continue
		}
		a.Action = action
		filtered = append(filtered, a)
	}
	plan.RecommendedActions = filtered
	return plan, warnings
}

func opsActionAllowed(action string) bool {
	switch action {
	case "pause_channel", "resume_channel", "adjust_workers", "set_runtime",
		"enable_dlq_auto_retry", "reload_rules", "notify", "noop":
		return true
	default:
		return false
	}
}

func opsActionOnCooldown(action string, hid int) bool {
	cfg := getOpsConfig()
	key := action
	if hid > 0 {
		key = fmt.Sprintf("%s:%d", action, hid)
	}
	opsActionCooldownMu.Lock()
	defer opsActionCooldownMu.Unlock()
	last, ok := opsLastActionAt[key]
	if !ok {
		return false
	}
	cooldown := time.Duration(cfg.Policy.ActionCooldownSeconds) * time.Second
	if hid > 0 {
		hidCooldown := time.Duration(cfg.Policy.HidCooldownSeconds) * time.Second
		if hidCooldown > cooldown {
			cooldown = hidCooldown
		}
	}
	return time.Since(last) < cooldown
}

func markOpsActionExecuted(action string, hid int) {
	key := action
	if hid > 0 {
		key = fmt.Sprintf("%s:%d", action, hid)
	}
	opsActionCooldownMu.Lock()
	opsLastActionAt[key] = time.Now()
	opsActionCooldownMu.Unlock()
}

func shouldExecuteOpsPlan(plan opsPlanDTO, manualExecute bool, fromWatcher bool) bool {
	cfg := getOpsConfig()
	if !cfg.Enabled {
		return false
	}
	if !planHasAutoActions(plan) {
		return false
	}
	if fromWatcher {
		if cfg.Mode != "auto" {
			return false
		}
		return opsSeverityAllowsAuto(plan.Severity)
	}
	return manualExecute
}

func filterExecutableActions(plan opsPlanDTO) []opsActionSpec {
	out := make([]opsActionSpec, 0, len(plan.RecommendedActions))
	for _, a := range plan.RecommendedActions {
		if !a.AutoExecute {
			continue
		}
		hid := opsActionHID(a)
		if opsActionOnCooldown(a.Action, hid) {
			continue
		}
		out = append(out, a)
	}
	return out
}

func opsActionHID(a opsActionSpec) int {
	if a.Params == nil {
		return 0
	}
	switch v := a.Params["hid"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func parseActionHID(params map[string]interface{}) (int, error) {
	if params == nil {
		return 0, fmt.Errorf("缺少 hid")
	}
	switch v := params["hid"].(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, err
		}
		return n, nil
	default:
		return 0, fmt.Errorf("hid 无效")
	}
}

func parseActionIntParam(params map[string]interface{}, key string, def int) int {
	if params == nil {
		return def
	}
	switch v := params[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return def
		}
		return n
	default:
		return def
	}
}

func parseActionBoolParam(params map[string]interface{}, key string, def bool) bool {
	if params == nil {
		return def
	}
	switch v := params[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	default:
		return def
	}
}
