package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	opsDailyReportMetaKey     = "ops_daily_report_latest"
	opsDailyReportLastDateKey = "ops_daily_report_last_date"
)

type opsDailyReportDTO struct {
	Date        string    `json:"date"`
	GeneratedAt time.Time `json:"generated_at"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Body        string    `json:"body"`
	Pushed      bool      `json:"pushed"`
}

var (
	opsReportMu        sync.Mutex
	opsDailyReportLast opsDailyReportDTO
)

func getLatestOpsDailyReport() opsDailyReportDTO {
	opsReportMu.Lock()
	defer opsReportMu.Unlock()
	return opsDailyReportLast
}

func loadOpsDailyReportFromMeta(ctx context.Context) {
	raw, err := getSystemMeta(ctx, opsDailyReportMetaKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return
	}
	var r opsDailyReportDTO
	if json.Unmarshal([]byte(raw), &r) != nil {
		return
	}
	opsReportMu.Lock()
	opsDailyReportLast = r
	opsReportMu.Unlock()
}

func saveOpsDailyReport(ctx context.Context, r opsDailyReportDTO) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := setSystemMeta(ctx, opsDailyReportMetaKey, string(b)); err != nil {
		return err
	}
	opsReportMu.Lock()
	opsDailyReportLast = r
	opsReportMu.Unlock()
	return nil
}

func maybeRunOpsDailyReport(ctx context.Context) {
	cfg := getOpsConfig()
	if !cfg.Enabled || !cfg.DailyReportEnabled {
		return
	}
	now := time.Now()
	if now.Hour() < cfg.DailyReportHour {
		return
	}
	today := now.Format("2006-01-02")
	lastDate, _ := getSystemMeta(ctx, opsDailyReportLastDateKey)
	if lastDate == today {
		return
	}

	report, err := buildOpsDailyReport(ctx, today)
	if err != nil {
		return
	}
	if cfg.NotifyOnAutoAction {
		url := strings.TrimSpace(alertShowdocURL)
		if url == "" {
			loadAlertShowdocFromPluginDB(ctx)
			url = strings.TrimSpace(alertShowdocURL)
		}
		if url != "" {
			if pushShowdoc(ctx, url, report.Title, report.Body) == nil {
				report.Pushed = true
			}
		}
	}
	_ = saveOpsDailyReport(ctx, report)
	_ = setSystemMeta(ctx, opsDailyReportLastDateKey, today)
}

func buildOpsDailyReport(ctx context.Context, date string) (opsDailyReportDTO, error) {
	cfg := getOpsConfig()
	since := time.Now().Add(-24 * time.Hour)
	opsCtx := collectOpsContext(ctx, "daily_report")
	channels := enrichOpsChannels(opsCtx)
	auditStats, _ := opsAuditStatsSince(ctx, since)

	var b strings.Builder
	title := fmt.Sprintf("AI运维 每日巡检 %s", date)
	fmt.Fprintf(&b, "【%s】\n\n", title)

	engineState := "未运行"
	if opsCtx.Engine.EngineRunning {
		engineState = "运行中"
	}
	fmt.Fprintf(&b, "运行模式：%s\n", opsModeText(cfg.Mode))
	fmt.Fprintf(&b, "引擎状态：%s\n", engineState)
	fmt.Fprintf(&b, "连接：Redis %s · 主库 MySQL %s\n\n",
		connMark(opsCtx.Engine.Connections.Redis.Ready),
		connMark(opsCtx.Engine.Connections.MainMySQL.Ready))

	fmt.Fprintf(&b, "今日提交：成功 %d · 失败 %d · DLQ %d\n",
		opsCtx.Engine.Today.Success, opsCtx.Engine.Today.Fail, opsCtx.Engine.Today.DLQ)
	fmt.Fprintf(&b, "近 %d 分钟：成功 %d · 失败 %d · DLQ %d\n\n",
		opsCtx.Engine.WindowMinutes,
		opsCtx.Engine.Window.Success, opsCtx.Engine.Window.Fail, opsCtx.Engine.Window.DLQ)

	active := 0
	paused := 0
	var warnLines []string
	for _, ch := range channels {
		if ch.Paused {
			paused++
		} else {
			active++
		}
		if ch.FailRatePct >= cfg.Thresholds.ChannelFailRatePct && ch.WindowFail >= 10 {
			warnLines = append(warnLines, fmt.Sprintf("· HID %d %s 失败率 %.1f%%", ch.HID, ch.Name, ch.FailRatePct))
		}
		if ch.DLQDepth >= cfg.Thresholds.DLQDepth {
			warnLines = append(warnLines, fmt.Sprintf("· HID %d %s DLQ %d", ch.HID, ch.Name, ch.DLQDepth))
		}
	}
	fmt.Fprintf(&b, "渠道：活跃 %d · 暂停 %d\n\n", active, paused)

	fmt.Fprintf(&b, "过去 24 小时运维事件：共 %d 条\n", auditStats.Total)
	fmt.Fprintf(&b, "· 已执行 %d · 已回滚 %d · 高/严重 %d\n",
		auditStats.Executed, auditStats.RolledBack, auditStats.HighOrAbove)
	if len(auditStats.Highlights) > 0 {
		b.WriteString("\n近期事件：\n")
		for _, h := range auditStats.Highlights {
			fmt.Fprintf(&b, "· %s\n", h)
		}
	}

	if len(warnLines) > 0 {
		b.WriteString("\n需关注：\n")
		for _, line := range warnLines {
			b.WriteString(line + "\n")
		}
	} else {
		b.WriteString("\n需关注：暂无超阈渠道\n")
	}

	summary := fmt.Sprintf("今日成功 %d 失败 %d；24h 运维 %d 条；暂停渠道 %d",
		opsCtx.Engine.Today.Success, opsCtx.Engine.Today.Fail, auditStats.Total, paused)

	return opsDailyReportDTO{
		Date:        date,
		GeneratedAt: time.Now(),
		Title:       title,
		Summary:     summary,
		Body:        strings.TrimSpace(b.String()),
	}, nil
}

func connMark(ok bool) string {
	if ok {
		return "正常"
	}
	return "异常"
}

func opsModeText(mode string) string {
	if mode == "auto" {
		return "自动处置"
	}
	return "观察模式"
}
