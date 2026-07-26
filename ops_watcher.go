package main

import (
	"context"
	"log"
	"sync"
	"time"
)

var (
	opsWatcherMu           sync.Mutex
	opsWatcherRunning      bool
	opsLastAIAnalysis      time.Time
	opsWatcherLifecycleCtx context.Context = context.Background()
)

func bindOpsWatcherLifecycle(ctx context.Context) {
	if ctx != nil {
		opsWatcherLifecycleCtx = ctx
	}
}

func opsWatcherContext() context.Context {
	if opsWatcherLifecycleCtx != nil {
		return opsWatcherLifecycleCtx
	}
	return context.Background()
}

func startOpsWatcher(ctx context.Context) {
	if ctx == nil {
		ctx = opsWatcherContext()
	}
	opsWatcherMu.Lock()
	if opsWatcherRunning {
		opsWatcherMu.Unlock()
		return
	}
	opsWatcherRunning = true
	opsWatcherMu.Unlock()

	go func() {
		defer func() {
			opsWatcherMu.Lock()
			opsWatcherRunning = false
			opsWatcherMu.Unlock()
		}()

		cfg := getOpsConfig()
		interval := time.Duration(cfg.ScanIntervalSeconds) * time.Second
		if interval <= 0 {
			interval = 60 * time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runOpsWatcherCycle(ctx)
			}
		}
	}()
}

func opsWatcherActive() bool {
	opsWatcherMu.Lock()
	defer opsWatcherMu.Unlock()
	return opsWatcherRunning && getOpsConfig().Enabled
}

func runOpsWatcherCycle(ctx context.Context) {
	cfg := getOpsConfig()
	if !cfg.Enabled {
		return
	}

	runOpsAutoResumeCheck(ctx)
	runOpsObservationChecks(ctx)
	maybeRunOpsDailyReport(ctx)

	opsCtx := collectOpsContext(ctx, "scheduled")
	events := detectOpsEvents(opsCtx)
	if len(events) == 0 {
		return
	}

	useAI := cfg.AIEnabled && opsAIReady()
	if useAI {
		if time.Since(opsLastAIAnalysis) < time.Duration(cfg.AIAnalysisIntervalSeconds)*time.Second {
			useAI = false
		}
	}

	trigger := "event:" + events[0]
	execute := cfg.Mode == "auto"
	result, err := runOpsAnalyze(ctx, trigger, execute, true, "watcher")
	if err != nil {
		log.Printf("[AI运维] 检查失败: %v", err)
		return
	}
	if useAI {
		opsLastAIAnalysis = time.Now()
	}
	if result.Executed {
		log.Printf("[AI运维] 已自动处理，记录编号 %d: %s", result.AuditID, result.Plan.Summary)
	}
}
