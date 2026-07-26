package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

const opsConfigMetaKey = "ops_config"

type OpsThresholds struct {
	ChannelFailRatePct     float64 `json:"channel_fail_rate_pct"`
	ChannelFailRateSpikePP float64 `json:"channel_fail_rate_spike_pp"`
	DLQDepth               int64   `json:"dlq_depth"`
	QueueBacklog           int64   `json:"queue_backlog"`
	ResumeFailRatePct      float64 `json:"resume_fail_rate_pct"`
	ResumeStableMinutes    int     `json:"resume_stable_minutes"`
	ResumeMinWindowEvents  int     `json:"resume_min_window_events"`
}

type OpsPolicyConfig struct {
	MaxActionsPerPlan    int `json:"max_actions_per_plan"`
	ActionCooldownSeconds int `json:"action_cooldown_seconds"`
	HidCooldownSeconds   int `json:"hid_cooldown_seconds"`
}

type OpsConfig struct {
	Enabled                     bool            `json:"enabled"`
	Mode                        string          `json:"mode"`
	AIEnabled                   bool            `json:"ai_enabled"`
	ScanIntervalSeconds         int             `json:"scan_interval_seconds"`
	AIAnalysisIntervalSeconds   int             `json:"ai_analysis_interval_seconds"`
	AIRateLimitPerHour          int             `json:"ai_rate_limit_per_hour"`
	AutoExecuteMinSeverity      string          `json:"auto_execute_min_severity"`
	PlaybooksEnabled            bool            `json:"playbooks_enabled"`
	NotifyOnAutoAction          bool            `json:"notify_on_auto_action"`
	NotifyOnRollback            bool            `json:"notify_on_rollback"`
	ObserveDurationMinutes      int             `json:"observe_duration_minutes"`
	DailyReportEnabled          bool            `json:"daily_report_enabled"`
	DailyReportHour             int             `json:"daily_report_hour"`
	OpsModel                    string          `json:"ops_model,omitempty"`
	OpsMaxTokens                int             `json:"ops_max_tokens,omitempty"`
	Thresholds                  OpsThresholds   `json:"thresholds"`
	Policy                      OpsPolicyConfig `json:"policy"`
}

var (
	opsConfig   OpsConfig
	opsConfigMu sync.RWMutex
)

func defaultOpsConfig() OpsConfig {
	return OpsConfig{
		Mode:                      "assist",
		ScanIntervalSeconds:       60,
		AIAnalysisIntervalSeconds: 300,
		AIRateLimitPerHour:        20,
		AutoExecuteMinSeverity:    "medium",
		PlaybooksEnabled:          true,
		NotifyOnAutoAction:        true,
		NotifyOnRollback:          true,
		ObserveDurationMinutes:    10,
		DailyReportEnabled:        true,
		DailyReportHour:           8,
		Thresholds: OpsThresholds{
			ChannelFailRatePct:     30,
			ChannelFailRateSpikePP: 15,
			DLQDepth:               100,
			QueueBacklog:           2000,
			ResumeFailRatePct:      10,
			ResumeStableMinutes:    10,
			ResumeMinWindowEvents:  5,
		},
		Policy: OpsPolicyConfig{
			MaxActionsPerPlan:     3,
			ActionCooldownSeconds: 900,
			HidCooldownSeconds:    300,
		},
	}
}

func normalizeOpsConfig(c OpsConfig) OpsConfig {
	d := defaultOpsConfig()
	if c.Mode != "auto" {
		c.Mode = "assist"
	}
	if c.ScanIntervalSeconds <= 0 {
		c.ScanIntervalSeconds = d.ScanIntervalSeconds
	}
	if c.AIAnalysisIntervalSeconds <= 0 {
		c.AIAnalysisIntervalSeconds = d.AIAnalysisIntervalSeconds
	}
	if c.AIRateLimitPerHour <= 0 {
		c.AIRateLimitPerHour = d.AIRateLimitPerHour
	}
	sev := strings.ToLower(strings.TrimSpace(c.AutoExecuteMinSeverity))
	if sev != "low" && sev != "medium" && sev != "high" && sev != "critical" {
		c.AutoExecuteMinSeverity = d.AutoExecuteMinSeverity
	} else {
		c.AutoExecuteMinSeverity = sev
	}
	if c.ObserveDurationMinutes <= 0 {
		c.ObserveDurationMinutes = d.ObserveDurationMinutes
	}
	if c.Thresholds.ChannelFailRatePct <= 0 {
		c.Thresholds.ChannelFailRatePct = d.Thresholds.ChannelFailRatePct
	}
	if c.Thresholds.ChannelFailRateSpikePP <= 0 {
		c.Thresholds.ChannelFailRateSpikePP = d.Thresholds.ChannelFailRateSpikePP
	}
	if c.Thresholds.DLQDepth <= 0 {
		c.Thresholds.DLQDepth = d.Thresholds.DLQDepth
	}
	if c.Thresholds.QueueBacklog <= 0 {
		c.Thresholds.QueueBacklog = d.Thresholds.QueueBacklog
	}
	if c.Thresholds.ResumeFailRatePct <= 0 {
		c.Thresholds.ResumeFailRatePct = d.Thresholds.ResumeFailRatePct
	}
	if c.Thresholds.ResumeStableMinutes <= 0 {
		c.Thresholds.ResumeStableMinutes = d.Thresholds.ResumeStableMinutes
	}
	if c.Thresholds.ResumeMinWindowEvents <= 0 {
		c.Thresholds.ResumeMinWindowEvents = d.Thresholds.ResumeMinWindowEvents
	}
	if c.DailyReportHour < 0 || c.DailyReportHour > 23 {
		c.DailyReportHour = d.DailyReportHour
	}
	if c.Policy.MaxActionsPerPlan <= 0 {
		c.Policy.MaxActionsPerPlan = d.Policy.MaxActionsPerPlan
	}
	if c.Policy.ActionCooldownSeconds <= 0 {
		c.Policy.ActionCooldownSeconds = d.Policy.ActionCooldownSeconds
	}
	if c.Policy.HidCooldownSeconds <= 0 {
		c.Policy.HidCooldownSeconds = d.Policy.HidCooldownSeconds
	}
	return c
}

func getOpsConfig() OpsConfig {
	opsConfigMu.RLock()
	defer opsConfigMu.RUnlock()
	return opsConfig
}

func loadOpsConfigFromPluginDB(ctx context.Context) {
	if pluginDB == nil {
		return
	}
	raw, err := getSystemMeta(ctx, opsConfigMetaKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		opsConfigMu.Lock()
		opsConfig = defaultOpsConfig()
		opsConfigMu.Unlock()
		return
	}
	var cfg OpsConfig
	if json.Unmarshal([]byte(raw), &cfg) != nil {
		opsConfigMu.Lock()
		opsConfig = defaultOpsConfig()
		opsConfigMu.Unlock()
		return
	}
	opsConfigMu.Lock()
	opsConfig = normalizeOpsConfig(cfg)
	opsConfigMu.Unlock()
}

func saveOpsConfig(ctx context.Context, cfg OpsConfig) error {
	if pluginDB == nil {
		return fmt.Errorf("插件数据库未就绪")
	}
	cfg = normalizeOpsConfig(cfg)
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := setSystemMeta(ctx, opsConfigMetaKey, string(b)); err != nil {
		return err
	}
	opsConfigMu.Lock()
	opsConfig = cfg
	opsConfigMu.Unlock()
	return nil
}

func opsAIReady() bool {
	cfg := getOpsConfig()
	return cfg.AIEnabled && aiConfigReady()
}

func severityRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return 0
	}
}

func opsSeverityAllowsAuto(severity string) bool {
	cfg := getOpsConfig()
	return severityRank(severity) >= severityRank(cfg.AutoExecuteMinSeverity)
}
