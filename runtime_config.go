package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const runtimeGlobalMetaKey = "runtime_global_config"

// RuntimeConfigPayload 货源运行时配置（存插件库），字段为空表示继承上层；默认值来自代码内置
type RuntimeConfigPayload struct {
	Queue       *RuntimeQueueSection       `json:"queue,omitempty"`
	OrderStatus *RuntimeOrderStatusSection `json:"order_status,omitempty"`
	RateLimit   *RuntimeRateLimitSection   `json:"rate_limit,omitempty"`
	Resubmit    *RuntimeResubmitSection    `json:"resubmit,omitempty"`
	Submit      *RuntimeSubmitSection      `json:"submit,omitempty"`
}

type RuntimeQueueSection struct {
	ProducerIntervalMS        *int `json:"producer_interval_ms,omitempty"`
	MinWorkersPerHID          *int `json:"min_workers_per_hid,omitempty"`
	MaxWorkersPerHID          *int `json:"max_workers_per_hid,omitempty"`
	ScaleCheckIntervalMS      *int `json:"scale_check_interval_ms,omitempty"`
	ScaleStepThreshold        *int `json:"scale_step_threshold,omitempty"`
	ProcessingTimeoutMinutes  *int `json:"processing_timeout_minutes,omitempty"`
	ReaperIntervalMinutes     *int `json:"reaper_interval_minutes,omitempty"`
	TimeoutConfirmWaitSeconds *int `json:"timeout_confirm_wait_seconds,omitempty"`
	StatsIntervalMinutes      *int `json:"stats_interval_minutes,omitempty"`
	IdleSleepMS               *int `json:"idle_sleep_ms,omitempty"`
	SubmitPoolWorkers         *int `json:"submit_pool_workers,omitempty"`
	SubmitPoolQueueCap        *int `json:"submit_pool_queue_cap,omitempty"`
	ConfirmPoolWorkers        *int `json:"confirm_pool_workers,omitempty"`
	ConfirmPoolQueueCap       *int `json:"confirm_pool_queue_cap,omitempty"`
}

type RuntimeOrderStatusSection struct {
	SubmittedStatus  *string `json:"submitted_status,omitempty"`
	SubmittedRemarks *string `json:"submitted_remarks,omitempty"`
	SuccessCodes     []int   `json:"success_codes,omitempty"`
}

type RuntimeRateLimitSection struct {
	Enabled            *bool `json:"enabled,omitempty"`
	GlobalMaxPerSecond *int  `json:"global_max_per_second,omitempty"`
	PerHIDMaxPerSecond *int  `json:"per_hid_max_per_second,omitempty"`
}

type RuntimeResubmitSection struct {
	Enabled                  *bool                        `json:"enabled,omitempty"`
	MaxAttempts              *int                         `json:"max_attempts,omitempty"`
	InitialDelaySeconds      *int                         `json:"initial_delay_seconds,omitempty"`
	BackoffMultiplier        *float64                     `json:"backoff_multiplier,omitempty"`
	MaxDelaySeconds          *int                         `json:"max_delay_seconds,omitempty"`
	RetryOnTimeout           *bool                        `json:"retry_on_timeout,omitempty"`
	RateLimitCountsAsAttempt *bool                        `json:"rate_limit_counts_as_attempt,omitempty"`
	TerminalKeywords         []string                     `json:"terminal_keywords,omitempty"`
	DLQAutoRetry             *RuntimeDLQAutoRetrySection  `json:"dlq_auto_retry,omitempty"`
}

type RuntimeDLQAutoRetrySection struct {
	Enabled             *bool `json:"enabled,omitempty"`
	ScanIntervalMinutes *int  `json:"scan_interval_minutes,omitempty"`
	MaxPerScan          *int  `json:"max_per_scan,omitempty"`
	MinAgeMinutes       *int  `json:"min_age_minutes,omitempty"`
}

type RuntimeSubmitSection struct {
	TimeoutSeconds *int `json:"timeout_seconds,omitempty"`
}

type effectiveQueueSettings struct {
	MinWorkersPerHID     int
	MaxWorkersPerHID     int
	ScaleStepThreshold   int
	ProcessingTimeout    time.Duration
}

type effectiveOrderStatusSettings struct {
	SubmittedStatus  string
	SubmittedRemarks string
	SuccessCodes     []int
}

type effectiveRateLimitSettings struct {
	Enabled            bool
	GlobalMaxPerSecond int
	PerHIDMaxPerSecond int
}

var (
	runtimeGlobalOverride *RuntimeConfigPayload
	runtimeHIDOverrides   = map[int]*RuntimeConfigPayload{}
	runtimeConfigMu       sync.RWMutex
	initRuntimeDefaults   RuntimeConfigPayload
)

func ensureHuoyuanRuntimeSchema(ctx context.Context) {
	if pluginDB == nil {
		return
	}
	q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		hid int NOT NULL,
		config_json json NOT NULL,
		remark varchar(512) NOT NULL DEFAULT '',
		updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (hid)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`, pluginTable("huoyuan_runtime"))
	_, _ = pluginDB.ExecContext(ctx, q)
}

// captureInitRuntimeDefaults 在 initConfig 结束时快照代码默认，供恢复/合并 baseline 使用
func captureInitRuntimeDefaults() {
	p := runtimeConfigFromGlobals()
	if p == nil {
		initRuntimeDefaults = RuntimeConfigPayload{}
		return
	}
	initRuntimeDefaults = *cloneRuntimeConfig(p)
}

// defaultRuntimeConfigBase 返回 initConfig 时的内置默认（不受运行时 global override 污染）
func defaultRuntimeConfigBase() RuntimeConfigPayload {
	return *cloneRuntimeConfig(&initRuntimeDefaults)
}

func float64Ptr(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

func intPtr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func boolPtr(v bool) *bool { return &v }

func mergeRuntimeConfig(base, overlay *RuntimeConfigPayload) *RuntimeConfigPayload {
	if overlay == nil {
		return cloneRuntimeConfig(base)
	}
	out := cloneRuntimeConfig(base)
	if overlay.Queue != nil {
		if out.Queue == nil {
			out.Queue = &RuntimeQueueSection{}
		}
		mergeQueueSection(out.Queue, overlay.Queue)
	}
	if overlay.OrderStatus != nil {
		if out.OrderStatus == nil {
			out.OrderStatus = &RuntimeOrderStatusSection{}
		}
		mergeOrderStatusSection(out.OrderStatus, overlay.OrderStatus)
	}
	if overlay.RateLimit != nil {
		if out.RateLimit == nil {
			out.RateLimit = &RuntimeRateLimitSection{}
		}
		mergeRateLimitSection(out.RateLimit, overlay.RateLimit)
	}
	if overlay.Resubmit != nil {
		if out.Resubmit == nil {
			out.Resubmit = &RuntimeResubmitSection{}
		}
		mergeResubmitSection(out.Resubmit, overlay.Resubmit)
	}
	if overlay.Submit != nil {
		if out.Submit == nil {
			out.Submit = &RuntimeSubmitSection{}
		}
		mergeSubmitSection(out.Submit, overlay.Submit)
	}
	return out
}

func mergeQueueSection(dst, src *RuntimeQueueSection) {
	if src.ProducerIntervalMS != nil {
		dst.ProducerIntervalMS = src.ProducerIntervalMS
	}
	if src.MinWorkersPerHID != nil {
		dst.MinWorkersPerHID = src.MinWorkersPerHID
	}
	if src.MaxWorkersPerHID != nil {
		dst.MaxWorkersPerHID = src.MaxWorkersPerHID
	}
	if src.ScaleCheckIntervalMS != nil {
		dst.ScaleCheckIntervalMS = src.ScaleCheckIntervalMS
	}
	if src.ScaleStepThreshold != nil {
		dst.ScaleStepThreshold = src.ScaleStepThreshold
	}
	if src.ProcessingTimeoutMinutes != nil {
		dst.ProcessingTimeoutMinutes = src.ProcessingTimeoutMinutes
	}
	if src.ReaperIntervalMinutes != nil {
		dst.ReaperIntervalMinutes = src.ReaperIntervalMinutes
	}
	if src.TimeoutConfirmWaitSeconds != nil {
		dst.TimeoutConfirmWaitSeconds = src.TimeoutConfirmWaitSeconds
	}
	if src.StatsIntervalMinutes != nil {
		dst.StatsIntervalMinutes = src.StatsIntervalMinutes
	}
	if src.IdleSleepMS != nil {
		dst.IdleSleepMS = src.IdleSleepMS
	}
	if src.SubmitPoolWorkers != nil {
		dst.SubmitPoolWorkers = src.SubmitPoolWorkers
	}
	if src.SubmitPoolQueueCap != nil {
		dst.SubmitPoolQueueCap = src.SubmitPoolQueueCap
	}
	if src.ConfirmPoolWorkers != nil {
		dst.ConfirmPoolWorkers = src.ConfirmPoolWorkers
	}
	if src.ConfirmPoolQueueCap != nil {
		dst.ConfirmPoolQueueCap = src.ConfirmPoolQueueCap
	}
}

func mergeOrderStatusSection(dst, src *RuntimeOrderStatusSection) {
	if src.SubmittedStatus != nil {
		dst.SubmittedStatus = src.SubmittedStatus
	}
	if src.SubmittedRemarks != nil {
		dst.SubmittedRemarks = src.SubmittedRemarks
	}
	if len(src.SuccessCodes) > 0 {
		dst.SuccessCodes = append([]int(nil), src.SuccessCodes...)
	}
}

func mergeRateLimitSection(dst, src *RuntimeRateLimitSection) {
	if src.Enabled != nil {
		dst.Enabled = src.Enabled
	}
	if src.GlobalMaxPerSecond != nil {
		dst.GlobalMaxPerSecond = src.GlobalMaxPerSecond
	}
	if src.PerHIDMaxPerSecond != nil {
		dst.PerHIDMaxPerSecond = src.PerHIDMaxPerSecond
	}
}

func mergeResubmitSection(dst, src *RuntimeResubmitSection) {
	if src.Enabled != nil {
		dst.Enabled = src.Enabled
	}
	if src.MaxAttempts != nil {
		dst.MaxAttempts = src.MaxAttempts
	}
	if src.InitialDelaySeconds != nil {
		dst.InitialDelaySeconds = src.InitialDelaySeconds
	}
	if src.BackoffMultiplier != nil {
		dst.BackoffMultiplier = src.BackoffMultiplier
	}
	if src.MaxDelaySeconds != nil {
		dst.MaxDelaySeconds = src.MaxDelaySeconds
	}
	if src.RetryOnTimeout != nil {
		dst.RetryOnTimeout = src.RetryOnTimeout
	}
	if src.RateLimitCountsAsAttempt != nil {
		dst.RateLimitCountsAsAttempt = src.RateLimitCountsAsAttempt
	}
	if len(src.TerminalKeywords) > 0 {
		dst.TerminalKeywords = append([]string(nil), src.TerminalKeywords...)
	}
	if src.DLQAutoRetry != nil {
		if dst.DLQAutoRetry == nil {
			dst.DLQAutoRetry = &RuntimeDLQAutoRetrySection{}
		}
		if src.DLQAutoRetry.Enabled != nil {
			dst.DLQAutoRetry.Enabled = src.DLQAutoRetry.Enabled
		}
		if src.DLQAutoRetry.ScanIntervalMinutes != nil {
			dst.DLQAutoRetry.ScanIntervalMinutes = src.DLQAutoRetry.ScanIntervalMinutes
		}
		if src.DLQAutoRetry.MaxPerScan != nil {
			dst.DLQAutoRetry.MaxPerScan = src.DLQAutoRetry.MaxPerScan
		}
		if src.DLQAutoRetry.MinAgeMinutes != nil {
			dst.DLQAutoRetry.MinAgeMinutes = src.DLQAutoRetry.MinAgeMinutes
		}
	}
}

func mergeSubmitSection(dst, src *RuntimeSubmitSection) {
	if src.TimeoutSeconds != nil {
		dst.TimeoutSeconds = src.TimeoutSeconds
	}
}

func cloneRuntimeConfig(in *RuntimeConfigPayload) *RuntimeConfigPayload {
	if in == nil {
		return &RuntimeConfigPayload{}
	}
	b, _ := json.Marshal(in)
	var out RuntimeConfigPayload
	_ = json.Unmarshal(b, &out)
	return &out
}

func runtimeConfigFromGlobals() *RuntimeConfigPayload {
	return &RuntimeConfigPayload{
		Queue: &RuntimeQueueSection{
			ProducerIntervalMS:        intPtr(int(producerTick / time.Millisecond)),
			MinWorkersPerHID:          intPtr(minWorkersPerHID),
			MaxWorkersPerHID:          intPtr(maxWorkersPerHID),
			ScaleCheckIntervalMS:      intPtr(int(scaleCheckInterval / time.Millisecond)),
			ScaleStepThreshold:        intPtr(scaleStepThreshold),
			ProcessingTimeoutMinutes:  intPtr(int(processingTimeout / time.Minute)),
			ReaperIntervalMinutes:     intPtr(int(reaperInterval / time.Minute)),
			TimeoutConfirmWaitSeconds: intPtr(int(timeoutConfirmWait / time.Second)),
			StatsIntervalMinutes:      intPtr(int(statsInterval / time.Minute)),
			IdleSleepMS:               intPtr(int(idleSleepDuration / time.Millisecond)),
			SubmitPoolWorkers:         intPtr(submitPoolWorkers),
			SubmitPoolQueueCap:        intPtr(submitPoolQueueCap),
			ConfirmPoolWorkers:        intPtr(confirmPoolWorkers),
			ConfirmPoolQueueCap:       intPtr(confirmPoolQueueCap),
		},
		OrderStatus: &RuntimeOrderStatusSection{
			SubmittedStatus:  strPtr(submittedStatus),
			SubmittedRemarks: strPtr(submittedRemarks),
			SuccessCodes:     append([]int(nil), successCodes...),
		},
		RateLimit: &RuntimeRateLimitSection{
			Enabled:            boolPtr(rateLimitEnabled),
			GlobalMaxPerSecond: intPtr(globalRateLimitMaxPerSecond()),
			PerHIDMaxPerSecond: intPtr(defaultPerHIDMaxPerSecond),
		},
		Resubmit: runtimeResubmitFromProcessDefaults(),
		Submit: &RuntimeSubmitSection{
			TimeoutSeconds: intPtr(int(submitTimeout / time.Second)),
		},
	}
}

func runtimeResubmitFromProcessDefaults() *RuntimeResubmitSection {
	return &RuntimeResubmitSection{
		Enabled:                  boolPtr(defaultResubmitEnabled),
		MaxAttempts:              intPtr(defaultResubmitMaxAttempts),
		InitialDelaySeconds:      intPtr(defaultResubmitInitialDelay),
		BackoffMultiplier:        float64Ptr(defaultResubmitBackoffMultiplier),
		MaxDelaySeconds:          intPtr(defaultResubmitMaxDelay),
		RetryOnTimeout:           boolPtr(defaultResubmitRetryOnTimeout),
		RateLimitCountsAsAttempt: boolPtr(defaultResubmitRateLimitAsAttempt),
		TerminalKeywords:         append([]string(nil), defaultResubmitTerminalKeywords...),
		DLQAutoRetry: &RuntimeDLQAutoRetrySection{
			Enabled:             boolPtr(defaultDLQAutoRetryEnabled),
			ScanIntervalMinutes: intPtr(defaultDLQAutoRetryScanMinutes),
			MaxPerScan:          intPtr(defaultDLQAutoRetryMaxPerScan),
			MinAgeMinutes:       intPtr(defaultDLQAutoRetryMinAgeMinutes),
		},
	}
}

var defaultPerHIDMaxPerSecond int

func globalRateLimitMaxPerSecond() int {
	if globalRateLimiter == nil {
		return 0
	}
	return globalRateLimiter.capacity
}

func reloadRuntimeConfigFromPluginDB(ctx context.Context) error {
	if pluginDB == nil {
		return nil
	}
	ensureHuoyuanRuntimeSchema(ctx)

	var globalRaw sql.NullString
	q := fmt.Sprintf(`SELECT meta_value FROM %s WHERE meta_key=? LIMIT 1`, pluginTable("system_meta"))
	_ = pluginDB.QueryRowContext(ctx, q, runtimeGlobalMetaKey).Scan(&globalRaw)

	var global *RuntimeConfigPayload
	if globalRaw.Valid && strings.TrimSpace(globalRaw.String) != "" {
		var p RuntimeConfigPayload
		if err := json.Unmarshal([]byte(globalRaw.String), &p); err == nil {
			global = &p
		}
	}

	rows, err := pluginDB.QueryContext(ctx, fmt.Sprintf(`SELECT hid, config_json FROM %s`, pluginTable("huoyuan_runtime")))
	if err != nil {
		return err
	}
	defer rows.Close()

	hidMap := map[int]*RuntimeConfigPayload{}
	for rows.Next() {
		var hid int
		var raw []byte
		if err := rows.Scan(&hid, &raw); err != nil {
			continue
		}
		var p RuntimeConfigPayload
		if json.Unmarshal(raw, &p) == nil {
			hidMap[hid] = &p
		}
	}

	runtimeConfigMu.Lock()
	runtimeGlobalOverride = global
	runtimeHIDOverrides = hidMap
	runtimeConfigMu.Unlock()

	applyRuntimeGlobalToProcess()
	initRateLimitersAfterRuntimeLoad()
	return nil
}

func applyRuntimeGlobalToProcess() {
	resetProcessGlobalsFromInitDefaults()

	runtimeConfigMu.RLock()
	global := runtimeGlobalOverride
	runtimeConfigMu.RUnlock()

	base := defaultRuntimeConfigBase()
	effective := mergeRuntimeConfig(&base, global)
	applyQueueGlobals(effective.Queue)
	applyOrderStatusGlobals(effective.OrderStatus)
	applyRateLimitGlobals(effective.RateLimit)
	applySubmitGlobals(effective.Submit)
}

func resetProcessGlobalsFromInitDefaults() {
	p := defaultRuntimeConfigBase()
	applyQueueGlobals(p.Queue)
	applyOrderStatusGlobals(p.OrderStatus)
	rateLimitEnabled = false
	defaultPerHIDMaxPerSecond = 0
	if p.RateLimit != nil {
		if p.RateLimit.Enabled != nil {
			rateLimitEnabled = *p.RateLimit.Enabled
		}
		if p.RateLimit.PerHIDMaxPerSecond != nil {
			defaultPerHIDMaxPerSecond = *p.RateLimit.PerHIDMaxPerSecond
		}
	}
	applySubmitGlobals(p.Submit)
}

func applyQueueGlobals(q *RuntimeQueueSection) {
	if q == nil {
		return
	}
	if q.ProducerIntervalMS != nil && *q.ProducerIntervalMS > 0 {
		producerTick = time.Duration(*q.ProducerIntervalMS) * time.Millisecond
	}
	if q.MinWorkersPerHID != nil && *q.MinWorkersPerHID > 0 {
		minWorkersPerHID = *q.MinWorkersPerHID
	}
	if q.MaxWorkersPerHID != nil && *q.MaxWorkersPerHID > 0 {
		maxWorkersPerHID = *q.MaxWorkersPerHID
	}
	if q.ScaleCheckIntervalMS != nil && *q.ScaleCheckIntervalMS > 0 {
		scaleCheckInterval = time.Duration(*q.ScaleCheckIntervalMS) * time.Millisecond
	}
	if q.ScaleStepThreshold != nil && *q.ScaleStepThreshold > 0 {
		scaleStepThreshold = *q.ScaleStepThreshold
	}
	if q.ProcessingTimeoutMinutes != nil && *q.ProcessingTimeoutMinutes > 0 {
		processingTimeout = time.Duration(*q.ProcessingTimeoutMinutes) * time.Minute
	}
	if q.ReaperIntervalMinutes != nil && *q.ReaperIntervalMinutes > 0 {
		reaperInterval = time.Duration(*q.ReaperIntervalMinutes) * time.Minute
	}
	if q.TimeoutConfirmWaitSeconds != nil && *q.TimeoutConfirmWaitSeconds > 0 {
		timeoutConfirmWait = time.Duration(*q.TimeoutConfirmWaitSeconds) * time.Second
	}
	if q.StatsIntervalMinutes != nil && *q.StatsIntervalMinutes > 0 {
		statsInterval = time.Duration(*q.StatsIntervalMinutes) * time.Minute
	}
	if q.IdleSleepMS != nil && *q.IdleSleepMS > 0 {
		idleSleepDuration = time.Duration(*q.IdleSleepMS) * time.Millisecond
	}
	applyPoolQueueGlobals(q)
}

func applyOrderStatusGlobals(o *RuntimeOrderStatusSection) {
	if o == nil {
		return
	}
	if o.SubmittedStatus != nil && *o.SubmittedStatus != "" {
		submittedStatus = *o.SubmittedStatus
	}
	if o.SubmittedRemarks != nil && *o.SubmittedRemarks != "" {
		submittedRemarks = *o.SubmittedRemarks
	}
	if len(o.SuccessCodes) > 0 {
		successCodes = append([]int(nil), o.SuccessCodes...)
	}
}

func applyRateLimitGlobals(r *RuntimeRateLimitSection) {
	if r == nil {
		return
	}
	if r.Enabled != nil {
		rateLimitEnabled = *r.Enabled
	}
	if r.PerHIDMaxPerSecond != nil {
		defaultPerHIDMaxPerSecond = *r.PerHIDMaxPerSecond
	}
}

func applySubmitGlobals(s *RuntimeSubmitSection) {
	if s == nil || s.TimeoutSeconds == nil || *s.TimeoutSeconds <= 0 {
		return
	}
	submitTimeout = time.Duration(*s.TimeoutSeconds) * time.Second
	if submitTimeout > 24*time.Hour {
		submitTimeout = 24 * time.Hour
	}
	if submitTimeout < 5*time.Second {
		submitTimeout = 5 * time.Second
	}
	httpClient = NewOutboundHTTPClient(submitTimeout)
}

func getSubmitTimeoutForHID(hid int) time.Duration {
	eff := getEffectiveMergedConfig(hid)
	if eff.Submit != nil && eff.Submit.TimeoutSeconds != nil && *eff.Submit.TimeoutSeconds > 0 {
		sec := *eff.Submit.TimeoutSeconds
		if sec < 5 {
			sec = 5
		}
		if sec > 86400 {
			sec = 86400
		}
		return time.Duration(sec) * time.Second
	}
	return submitTimeout
}

func initRateLimitersAfterRuntimeLoad() {
	if !rateLimitEnabled || rdb == nil {
		globalRateLimiter = nil
		return
	}
	eff := getEffectiveGlobalRateLimit()
	if eff.GlobalMaxPerSecond > 0 {
		globalRateLimiter = NewTokenBucket(rdb, "rate_limit:global", eff.GlobalMaxPerSecond, float64(eff.GlobalMaxPerSecond))
	} else {
		globalRateLimiter = nil
	}
	rateLimitMu.Lock()
	perHIDRateLimiters = make(map[int]*TokenBucket)
	rateLimitMu.Unlock()
}

func getEffectiveMergedConfig(hid int) *RuntimeConfigPayload {
	base := defaultRuntimeConfigBase()
	runtimeConfigMu.RLock()
	global := runtimeGlobalOverride
	hidCfg := runtimeHIDOverrides[hid]
	runtimeConfigMu.RUnlock()
	out := mergeRuntimeConfig(&base, global)
	if hid > 0 && hidCfg != nil {
		out = mergeRuntimeConfig(out, hidCfg)
	}
	return out
}

func getEffectiveQueueForHID(hid int) effectiveQueueSettings {
	eff := getEffectiveMergedConfig(hid)
	minW, maxW, step, procMin := minWorkersPerHID, maxWorkersPerHID, scaleStepThreshold, int(processingTimeout/time.Minute)
	if eff.Queue != nil {
		if eff.Queue.MinWorkersPerHID != nil {
			minW = *eff.Queue.MinWorkersPerHID
		}
		if eff.Queue.MaxWorkersPerHID != nil {
			maxW = *eff.Queue.MaxWorkersPerHID
		}
		if eff.Queue.ScaleStepThreshold != nil {
			step = *eff.Queue.ScaleStepThreshold
		}
		if eff.Queue.ProcessingTimeoutMinutes != nil {
			procMin = *eff.Queue.ProcessingTimeoutMinutes
		}
	}
	if minW <= 0 {
		minW = 1
	}
	if maxW <= 0 {
		maxW = 8
	}
	if step <= 0 {
		step = 100
	}
	if procMin <= 0 {
		procMin = 45
	}
	return effectiveQueueSettings{
		MinWorkersPerHID:   minW,
		MaxWorkersPerHID:   maxW,
		ScaleStepThreshold: step,
		ProcessingTimeout:  time.Duration(procMin) * time.Minute,
	}
}

func getEffectiveOrderStatusForHID(hid int) effectiveOrderStatusSettings {
	eff := getEffectiveMergedConfig(hid)
	status, remarks := submittedStatus, submittedRemarks
	codes := append([]int(nil), successCodes...)
	if eff.OrderStatus != nil {
		if eff.OrderStatus.SubmittedStatus != nil && *eff.OrderStatus.SubmittedStatus != "" {
			status = *eff.OrderStatus.SubmittedStatus
		}
		if eff.OrderStatus.SubmittedRemarks != nil && *eff.OrderStatus.SubmittedRemarks != "" {
			remarks = *eff.OrderStatus.SubmittedRemarks
		}
		if len(eff.OrderStatus.SuccessCodes) > 0 {
			codes = append([]int(nil), eff.OrderStatus.SuccessCodes...)
		}
	}
	if status == "" {
		status = "已提交"
	}
	if remarks == "" {
		remarks = "订单已成功提交至处理系统，请耐心等待处理"
	}
	if len(codes) == 0 {
		codes = []int{0, 1}
	}
	return effectiveOrderStatusSettings{SubmittedStatus: status, SubmittedRemarks: remarks, SuccessCodes: codes}
}

func getEffectiveGlobalRateLimit() effectiveRateLimitSettings {
	eff := getEffectiveMergedConfig(0)
	return rateLimitFromPayload(eff)
}

func getEffectiveRateLimitForHID(hid int) effectiveRateLimitSettings {
	eff := getEffectiveMergedConfig(hid)
	return rateLimitFromPayload(eff)
}

func rateLimitFromPayload(eff *RuntimeConfigPayload) effectiveRateLimitSettings {
	out := effectiveRateLimitSettings{Enabled: rateLimitEnabled}
	if eff.RateLimit != nil {
		if eff.RateLimit.Enabled != nil {
			out.Enabled = *eff.RateLimit.Enabled
		}
		if eff.RateLimit.GlobalMaxPerSecond != nil {
			out.GlobalMaxPerSecond = *eff.RateLimit.GlobalMaxPerSecond
		}
		if eff.RateLimit.PerHIDMaxPerSecond != nil {
			out.PerHIDMaxPerSecond = *eff.RateLimit.PerHIDMaxPerSecond
		}
	}
	return out
}

func getSuccessCodesForHID(hid int) []int {
	return getEffectiveOrderStatusForHID(hid).SuccessCodes
}

func saveRuntimeGlobalConfig(ctx context.Context, payload *RuntimeConfigPayload) error {
	if pluginDB == nil {
		return fmt.Errorf("插件数据库未就绪")
	}
	ensureHuoyuanRuntimeSchema(ctx)
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	q := fmt.Sprintf(`INSERT INTO %s (meta_key, meta_value) VALUES (?,?)
		ON DUPLICATE KEY UPDATE meta_value=VALUES(meta_value)`, pluginTable("system_meta"))
	_, err = pluginDB.ExecContext(ctx, q, runtimeGlobalMetaKey, string(b))
	if err != nil {
		return err
	}
	return reloadRuntimeConfigFromPluginDB(ctx)
}

func deleteRuntimeGlobalConfig(ctx context.Context) error {
	if pluginDB == nil {
		return fmt.Errorf("插件数据库未就绪")
	}
	q := fmt.Sprintf(`DELETE FROM %s WHERE meta_key=?`, pluginTable("system_meta"))
	_, err := pluginDB.ExecContext(ctx, q, runtimeGlobalMetaKey)
	if err != nil {
		return err
	}
	return reloadRuntimeConfigFromPluginDB(ctx)
}

func saveRuntimeHIDConfig(ctx context.Context, hid int, payload *RuntimeConfigPayload, remark string) error {
	if pluginDB == nil {
		return fmt.Errorf("插件数据库未就绪")
	}
	if hid <= 0 {
		return fmt.Errorf("hid 无效")
	}
	ensureHuoyuanRuntimeSchema(ctx)
	if runtimePayloadEmpty(payload) {
		if strings.TrimSpace(remark) == "" {
			return deleteRuntimeHIDConfig(ctx, hid)
		}
		payload = &RuntimeConfigPayload{}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	q := fmt.Sprintf(`INSERT INTO %s (hid, config_json, remark) VALUES (?,?,?)
		ON DUPLICATE KEY UPDATE config_json=VALUES(config_json), remark=VALUES(remark)`, pluginTable("huoyuan_runtime"))
	_, err = pluginDB.ExecContext(ctx, q, hid, string(b), remark)
	if err != nil {
		return err
	}
	return reloadRuntimeConfigFromPluginDB(ctx)
}

func deleteRuntimeHIDConfig(ctx context.Context, hid int) error {
	if pluginDB == nil {
		return fmt.Errorf("插件数据库未就绪")
	}
	q := fmt.Sprintf(`DELETE FROM %s WHERE hid=?`, pluginTable("huoyuan_runtime"))
	_, err := pluginDB.ExecContext(ctx, q, hid)
	if err != nil {
		return err
	}
	return reloadRuntimeConfigFromPluginDB(ctx)
}

func getRuntimeGlobalOverride() *RuntimeConfigPayload {
	runtimeConfigMu.RLock()
	defer runtimeConfigMu.RUnlock()
	return cloneRuntimeConfig(runtimeGlobalOverride)
}

func getRuntimeHIDOverride(hid int) *RuntimeConfigPayload {
	runtimeConfigMu.RLock()
	defer runtimeConfigMu.RUnlock()
	return cloneRuntimeConfig(runtimeHIDOverrides[hid])
}

func listRuntimeHIDWithConfig() map[int]bool {
	if pluginDB == nil {
		runtimeConfigMu.RLock()
		out := make(map[int]bool, len(runtimeHIDOverrides))
		for hid := range runtimeHIDOverrides {
			out[hid] = true
		}
		runtimeConfigMu.RUnlock()
		return out
	}
	ctx := context.Background()
	rows, err := pluginDB.QueryContext(ctx, fmt.Sprintf(`SELECT hid FROM %s`, pluginTable("huoyuan_runtime")))
	if err != nil {
		runtimeConfigMu.RLock()
		out := make(map[int]bool, len(runtimeHIDOverrides))
		for hid := range runtimeHIDOverrides {
			out[hid] = true
		}
		runtimeConfigMu.RUnlock()
		return out
	}
	defer rows.Close()
	out := make(map[int]bool)
	for rows.Next() {
		var hid int
		if rows.Scan(&hid) == nil && hid > 0 {
			out[hid] = true
		}
	}
	return out
}

func runtimePayloadEmpty(p *RuntimeConfigPayload) bool {
	if p == nil {
		return true
	}
	b, _ := json.Marshal(p)
	return string(b) == "{}" || string(b) == "null"
}

func parseHIDString(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
