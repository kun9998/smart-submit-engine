package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

func retryScheduleKey(hid int) string { return fmt.Sprintf("retry_schedule:%d", hid) }

type effectiveResubmitSettings struct {
	Enabled                  bool
	MaxAttempts              int
	InitialDelaySeconds      int
	BackoffMultiplier        float64
	MaxDelaySeconds          int
	RetryOnTimeout           bool
	RateLimitCountsAsAttempt bool
	TerminalKeywords         []string
	DLQAutoRetry             effectiveDLQAutoRetrySettings
}

type effectiveDLQAutoRetrySettings struct {
	Enabled             bool
	ScanIntervalMinutes int
	MaxPerScan          int
	MinAgeMinutes       int
}

var (
	defaultResubmitEnabled            = false
	defaultResubmitMaxAttempts        = 3
	defaultResubmitInitialDelay       = 30
	defaultResubmitBackoffMultiplier  = 2.0
	defaultResubmitMaxDelay           = 600
	defaultResubmitRetryOnTimeout     = false
	defaultResubmitRateLimitAsAttempt = false
	defaultResubmitTerminalKeywords   = []string{
		"余额不足", "余额", "账户余额", "insufficient balance", "no balance",
		"课程不存在", "重复下单", "重复订单", "3天内重复", "请勿下单", "相同课程",
		"参数错误", "不支持的平台",
	}
	defaultDLQAutoRetryEnabled       = false
	defaultDLQAutoRetryScanMinutes   = 30
	defaultDLQAutoRetryMaxPerScan    = 50
	defaultDLQAutoRetryMinAgeMinutes = 60
)

func getEffectiveResubmitForHID(hid int) effectiveResubmitSettings {
	out := effectiveResubmitSettings{
		Enabled:                  defaultResubmitEnabled,
		MaxAttempts:              defaultResubmitMaxAttempts,
		InitialDelaySeconds:      defaultResubmitInitialDelay,
		BackoffMultiplier:        defaultResubmitBackoffMultiplier,
		MaxDelaySeconds:          defaultResubmitMaxDelay,
		RetryOnTimeout:           defaultResubmitRetryOnTimeout,
		RateLimitCountsAsAttempt: defaultResubmitRateLimitAsAttempt,
		TerminalKeywords:         append([]string(nil), defaultResubmitTerminalKeywords...),
		DLQAutoRetry: effectiveDLQAutoRetrySettings{
			Enabled:             defaultDLQAutoRetryEnabled,
			ScanIntervalMinutes: defaultDLQAutoRetryScanMinutes,
			MaxPerScan:          defaultDLQAutoRetryMaxPerScan,
			MinAgeMinutes:       defaultDLQAutoRetryMinAgeMinutes,
		},
	}

	eff := getEffectiveMergedConfig(hid)
	if eff.Resubmit == nil {
		return normalizeResubmitSettings(out)
	}
	applyResubmitPayload(&out, eff.Resubmit)
	return normalizeResubmitSettings(out)
}

func applyResubmitPayload(out *effectiveResubmitSettings, r *RuntimeResubmitSection) {
	if r.Enabled != nil {
		out.Enabled = *r.Enabled
	}
	if r.MaxAttempts != nil {
		out.MaxAttempts = *r.MaxAttempts
	}
	if r.InitialDelaySeconds != nil {
		out.InitialDelaySeconds = *r.InitialDelaySeconds
	}
	if r.BackoffMultiplier != nil {
		out.BackoffMultiplier = *r.BackoffMultiplier
	}
	if r.MaxDelaySeconds != nil {
		out.MaxDelaySeconds = *r.MaxDelaySeconds
	}
	if r.RetryOnTimeout != nil {
		out.RetryOnTimeout = *r.RetryOnTimeout
	}
	if r.RateLimitCountsAsAttempt != nil {
		out.RateLimitCountsAsAttempt = *r.RateLimitCountsAsAttempt
	}
	if len(r.TerminalKeywords) > 0 {
		out.TerminalKeywords = append([]string(nil), r.TerminalKeywords...)
	}
	if r.DLQAutoRetry != nil {
		if r.DLQAutoRetry.Enabled != nil {
			out.DLQAutoRetry.Enabled = *r.DLQAutoRetry.Enabled
		}
		if r.DLQAutoRetry.ScanIntervalMinutes != nil {
			out.DLQAutoRetry.ScanIntervalMinutes = *r.DLQAutoRetry.ScanIntervalMinutes
		}
		if r.DLQAutoRetry.MaxPerScan != nil {
			out.DLQAutoRetry.MaxPerScan = *r.DLQAutoRetry.MaxPerScan
		}
		if r.DLQAutoRetry.MinAgeMinutes != nil {
			out.DLQAutoRetry.MinAgeMinutes = *r.DLQAutoRetry.MinAgeMinutes
		}
	}
}

func normalizeResubmitSettings(s effectiveResubmitSettings) effectiveResubmitSettings {
	if s.MaxAttempts <= 0 {
		s.MaxAttempts = 3
	}
	if s.InitialDelaySeconds <= 0 {
		s.InitialDelaySeconds = 30
	}
	if s.BackoffMultiplier <= 1 {
		s.BackoffMultiplier = 2
	}
	if s.MaxDelaySeconds <= 0 {
		s.MaxDelaySeconds = 600
	}
	if s.DLQAutoRetry.ScanIntervalMinutes <= 0 {
		s.DLQAutoRetry.ScanIntervalMinutes = 30
	}
	if s.DLQAutoRetry.MaxPerScan <= 0 {
		s.DLQAutoRetry.MaxPerScan = 50
	}
	if s.DLQAutoRetry.MinAgeMinutes <= 0 {
		s.DLQAutoRetry.MinAgeMinutes = 60
	}
	return s
}

func computeRetryDelay(cfg effectiveResubmitSettings, attemptAfterFail int) time.Duration {
	if attemptAfterFail <= 0 {
		attemptAfterFail = 1
	}
	sec := float64(cfg.InitialDelaySeconds) * math.Pow(cfg.BackoffMultiplier, float64(attemptAfterFail-1))
	if sec > float64(cfg.MaxDelaySeconds) {
		sec = float64(cfg.MaxDelaySeconds)
	}
	return time.Duration(sec) * time.Second
}

func matchesTerminalKeyword(errmsg string, keywords []string) bool {
	if errmsg == "" {
		return false
	}
	lower := strings.ToLower(errmsg)
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if strings.Contains(errmsg, kw) || strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func isRetryableSubmitError(errmsg string, callErr error) bool {
	if callErr != nil {
		if errors.Is(callErr, ErrHTTPTimeout) {
			return true
		}
		lower := strings.ToLower(errmsg)
		if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded") ||
			strings.Contains(lower, "connection reset") || strings.Contains(lower, "connection refused") ||
			strings.Contains(lower, "network") || strings.Contains(lower, "e_http_transport") ||
			strings.Contains(lower, "e_dns") {
			return true
		}
	}
	if strings.Contains(errmsg, "502") || strings.Contains(errmsg, "503") || strings.Contains(errmsg, "504") ||
		strings.Contains(errmsg, "500") || strings.Contains(errmsg, "408") || strings.Contains(errmsg, "429") ||
		strings.Contains(errmsg, "上游网关") || strings.Contains(errmsg, "上游接口内部错误") ||
		strings.Contains(errmsg, "上游接口暂时不可用") || strings.Contains(errmsg, "网络请求超时") ||
		strings.Contains(errmsg, "网络连接失败") || strings.Contains(errmsg, "域名解析失败") {
		return true
	}
	return false
}

func shouldRetrySubmitFailure(cfg effectiveResubmitSettings, msg orderMsg, errmsg string, callErr error) bool {
	if !cfg.Enabled {
		return false
	}
	if msg.R+1 >= cfg.MaxAttempts {
		return false
	}
	if matchesTerminalKeyword(errmsg, cfg.TerminalKeywords) {
		return false
	}
	return isRetryableSubmitError(errmsg, callErr)
}

func orderAlreadySucceededInDB(ctx context.Context, oid int) (bool, string) {
	if db == nil {
		return false, ""
	}
	var yidDB sql.NullString
	var ds sql.NullString
	orderTable := tableName("order")
	err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT yid, dockstatus FROM %s WHERE oid=? LIMIT 1`, orderTable), oid).Scan(&yidDB, &ds)
	if err != nil {
		return false, ""
	}
	if (yidDB.Valid && yidDB.String != "") || (ds.Valid && ds.String == "1") {
		yid := ""
		if yidDB.Valid {
			yid = yidDB.String
		}
		return true, yid
	}
	return false, ""
}

func scheduleOrderRetry(ctx context.Context, hid int, msg orderMsg, cfg effectiveResubmitSettings) (time.Duration, error) {
	msg.R++
	delay := computeRetryDelay(cfg, msg.R)
	score := float64(time.Now().Add(delay).Unix())
	b, err := json.Marshal(msg)
	if err != nil {
		return 0, err
	}
	if err := rdb.ZAdd(ctx, retryScheduleKey(hid), &redis.Z{Score: score, Member: string(b)}).Err(); err != nil {
		return 0, err
	}
	_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
	return delay, nil
}

func enqueueOrderToHIDQueue(ctx context.Context, hid int, msg orderMsg) error {
	ok, err := rdb.SetNX(ctx, enqKey(msg.OID), 1, enqKeyTTL).Result()
	if err != nil || !ok {
		return fmt.Errorf("enq dedup blocked")
	}
	b, _ := json.Marshal(msg)
	if err := rdb.LPush(ctx, listKey(hid), string(b)).Err(); err != nil {
		_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
		return err
	}
	return nil
}

func logRetryScheduled(hid, oid, attempt int, delay time.Duration, reason string) {
	logSubmitRetryLater(hid, oid, attempt, delay)
}

func markOrderTerminalFailure(ctx context.Context, hid int, msg orderMsg, val, proc, errmsg string, updateStatusRemarks bool) {
	name := submitLogChannel(hid)
	logSubmitFail(hid, msg.OID, errmsg)
	orderTable := tableName("order")
	var execErr error
	if updateStatusRemarks {
		simplifiedErrmsg := simplifyErrorMsg(errmsg)
		_, execErr = db.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET status='提交异常', remarks=?, dockstatus='2' WHERE oid=? LIMIT 1`, orderTable),
			fmt.Sprintf("提交失败：%s", simplifiedErrmsg), msg.OID,
		)
	} else {
		_, execErr = db.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET dockstatus='2' WHERE oid=? LIMIT 1`, orderTable),
			msg.OID,
		)
	}
	if execErr != nil {
		log.Printf("更新订单状态失败，订单号 %d", msg.OID)
	}
	if alertShowdocURL != "" {
		title := fmt.Sprintf("订单提交失败 · %s", name)
		content := fmt.Sprintf("订单号：%d\n渠道：%s\n原因：%s\n已标记为提交异常，请到管理端查看", msg.OID, name, SanitizeUserVisibleError(errmsg))
		go sendNotification(notifySubmitFailure, title, content)
	}
	atomicAddDLQ(hid)
	_ = rdb.LRem(ctx, proc, 1, val).Err()
	b, _ := json.Marshal(msg)
	_ = rdb.LPush(ctx, dlqKey(hid), string(b)).Err()
	_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
}

func atomicAddDLQ(hid int) {
	recordSubmitFail(hid)
	recordSubmitDLQ(hid)
}

func tryHandleSubmitRetry(ctx context.Context, hid int, msg orderMsg, val, proc, errmsg string, callErr error) bool {
	cfg := getEffectiveResubmitForHID(hid)
	if !shouldRetrySubmitFailure(cfg, msg, errmsg, callErr) {
		return false
	}
	delay, err := scheduleOrderRetry(ctx, hid, msg, cfg)
	if err != nil {
		log.Printf("安排重试失败，订单号 %d", msg.OID)
		return false
	}
	logRetryScheduled(hid, msg.OID, msg.R, delay, SanitizeUserVisibleError(errmsg))
	_ = rdb.LRem(ctx, proc, 1, val).Err()
	return true
}

// tryHandleProcessingTimeoutRetry Reaper 回收 processing 超时订单时安排延迟重试。
func tryHandleProcessingTimeoutRetry(ctx context.Context, hid int, msg orderMsg, val, proc string) bool {
	cfg := getEffectiveResubmitForHID(hid)
	if !cfg.Enabled || msg.R+1 >= cfg.MaxAttempts {
		return false
	}
	delay, err := scheduleOrderRetry(ctx, hid, msg, cfg)
	if err != nil {
		log.Printf("自动回收后安排重试失败，订单号 %d", msg.OID)
		return false
	}
	logRetryScheduled(hid, msg.OID, msg.R, delay, "处理时间太长")
	_ = rdb.LRem(ctx, proc, 1, val).Err()
	return true
}

func startRetryScheduleWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				dispatchDueRetries(ctx)
			}
		}
	}()
}

func dispatchDueRetries(ctx context.Context) {
	if rdb == nil {
		return
	}
	hidsMu.RLock()
	snapshot := append([]int(nil), hids...)
	hidsMu.RUnlock()
	now := fmt.Sprintf("%d", time.Now().Unix())
	for _, hid := range snapshot {
		key := retryScheduleKey(hid)
		members, err := rdb.ZRangeByScore(ctx, key, &redis.ZRangeBy{
			Min:   "-inf",
			Max:   now,
			Count: 50,
		}).Result()
		if err != nil || len(members) == 0 {
			continue
		}
		for _, member := range members {
			removed, _ := rdb.ZRem(ctx, key, member).Result()
			if removed == 0 {
				continue
			}
			var msg orderMsg
			if err := json.Unmarshal([]byte(member), &msg); err != nil {
				continue
			}
			if v, exists, _ := getSubmittedOrder(ctx, msg.OID); exists {
				_ = updateOrderStatusSubmitted(ctx, msg.OID, v, hid)
				continue
			}
			if ok, yid := orderAlreadySucceededInDB(ctx, msg.OID); ok {
				_ = setSubmittedOrder(ctx, msg.OID, yid)
				_ = updateOrderStatusSubmitted(ctx, msg.OID, yid, hid)
				continue
			}
			if err := enqueueOrderToHIDQueue(ctx, hid, msg); err != nil {
				// 重新放回延迟队列，短延迟后重试
				_ = rdb.ZAdd(ctx, key, &redis.Z{
					Score:  float64(time.Now().Add(10 * time.Second).Unix()),
					Member: member,
				}).Err()
			}
		}
	}
}

func startDLQAutoRetryWorker(ctx context.Context) {
	go func() {
		for {
			interval := dlqAutoRetryScanInterval()
			if interval < time.Minute {
				interval = time.Minute
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
				scanDLQForAutoRetry(ctx)
			}
		}
	}()
}

func dlqAutoRetryScanInterval() time.Duration {
	globalCfg := getEffectiveResubmitForHID(0)
	minutes := globalCfg.DLQAutoRetry.ScanIntervalMinutes
	if minutes <= 0 {
		minutes = defaultDLQAutoRetryScanMinutes
	}

	hidsMu.RLock()
	snapshot := append([]int(nil), hids...)
	hidsMu.RUnlock()
	for _, hid := range snapshot {
		cfg := getEffectiveResubmitForHID(hid)
		if !cfg.DLQAutoRetry.Enabled {
			continue
		}
		m := cfg.DLQAutoRetry.ScanIntervalMinutes
		if m <= 0 {
			m = defaultDLQAutoRetryScanMinutes
		}
		if m < minutes {
			minutes = m
		}
	}
	if minutes < 1 {
		minutes = 1
	}
	return time.Duration(minutes) * time.Minute
}

func scanDLQForAutoRetry(ctx context.Context) {
	if rdb == nil || db == nil {
		return
	}
	hidsMu.RLock()
	snapshot := append([]int(nil), hids...)
	hidsMu.RUnlock()
	now := time.Now()
	for _, hid := range snapshot {
		cfg := getEffectiveResubmitForHID(hid)
		if !cfg.DLQAutoRetry.Enabled {
			continue
		}
		minAge := time.Duration(cfg.DLQAutoRetry.MinAgeMinutes) * time.Minute
		dlq := dlqKey(hid)
		vals, err := rdb.LRange(ctx, dlq, 0, int64(cfg.DLQAutoRetry.MaxPerScan-1)).Result()
		if err != nil || len(vals) == 0 {
			continue
		}
		for _, val := range vals {
			var msg orderMsg
			if err := json.Unmarshal([]byte(val), &msg); err != nil {
				continue
			}
			if msg.TS > 0 && now.Sub(time.Unix(msg.TS, 0)) < minAge {
				continue
			}
			if v, exists, _ := getSubmittedOrder(ctx, msg.OID); exists {
				_ = rdb.LRem(ctx, dlq, 1, val).Err()
				_ = updateOrderStatusSubmitted(ctx, msg.OID, v, hid)
				continue
			}
			if ok, yid := orderAlreadySucceededInDB(ctx, msg.OID); ok {
				_ = rdb.LRem(ctx, dlq, 1, val).Err()
				_ = setSubmittedOrder(ctx, msg.OID, yid)
				_ = updateOrderStatusSubmitted(ctx, msg.OID, yid, hid)
				continue
			}
			msg.R = 0
			msg.TS = now.Unix()
			orderTable := tableName("order")
			res, err := db.ExecContext(ctx,
				fmt.Sprintf(`UPDATE %s SET dockstatus='0' WHERE oid=? AND dockstatus='2' LIMIT 1`, orderTable),
				msg.OID,
			)
			if err != nil {
				continue
			}
			if rows, _ := res.RowsAffected(); rows == 0 {
				continue
			}
			_ = rdb.LRem(ctx, dlq, 1, val).Err()
			if err := enqueueOrderToHIDQueue(ctx, hid, msg); err != nil {
				log.Printf("失败订单自动重试入队失败，订单号 %d", msg.OID)
			} else {
				logSubmitRequeued(hid, msg.OID)
			}
		}
	}
}
