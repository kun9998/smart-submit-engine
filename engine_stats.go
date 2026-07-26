package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const engineStatsWindowMinutes = 30

type engineCounterSnapshot struct {
	Success uint64 `json:"success"`
	Fail    uint64 `json:"fail"`
	DLQ     uint64 `json:"dlq"`
}

type engineConnStatus struct {
	Ready   bool   `json:"ready"`
	Message string `json:"message,omitempty"`
	Addr    string `json:"addr,omitempty"`
}

type engineConnectionsDTO struct {
	Redis       engineConnStatus `json:"redis"`
	MainMySQL   engineConnStatus `json:"main_mysql"`
	PluginMySQL engineConnStatus `json:"plugin_mysql"`
}

type engineChannelDTO struct {
	HID               int    `json:"hid"`
	Name              string `json:"name"`
	QueueDepth        int64  `json:"queue_depth"`
	ProcessingDepth   int64  `json:"processing_depth"`
	DLQDepth          int64  `json:"dlq_depth"`
	Workers           int    `json:"workers"`
	WindowSuccess     uint64 `json:"window_success"`
	WindowFail        uint64 `json:"window_fail"`
	WindowDLQ         uint64 `json:"window_dlq"`
	OpsPaused         bool   `json:"ops_paused"`
}

type engineStatsDTO struct {
	WindowMinutes int                   `json:"window_minutes"`
	Window        engineCounterSnapshot `json:"window"`
	Today         engineCounterSnapshot `json:"today"`
	Lifetime      engineCounterSnapshot `json:"lifetime"`
	EngineRunning bool                  `json:"engine_running"`
	Connections   engineConnectionsDTO  `json:"connections"`
	Channels      []engineChannelDTO    `json:"channels"`
}

var (
	failCount uint64

	engineStatsMu sync.Mutex
	engineToday   engineCounterSnapshot
	engineWindow  engineCounterSnapshot
	engineDay     string
	failWin       = map[int]uint64{}
)

func engineLocalDate() string {
	return time.Now().Format("2006-01-02")
}

func ensureEngineDayLocked() {
	today := engineLocalDate()
	if engineDay == today {
		return
	}
	engineDay = today
	engineToday = engineCounterSnapshot{}
}

func addEngineCountersLocked(kind string, n uint64) {
	ensureEngineDayLocked()
	switch kind {
	case "success":
		engineToday.Success += n
		engineWindow.Success += n
	case "fail":
		engineToday.Fail += n
		engineWindow.Fail += n
	case "dlq":
		engineToday.DLQ += n
		engineWindow.DLQ += n
	}
}

func recordSubmitSuccess(hid int) {
	atomic.AddUint64(&successCount, 1)
	perMu.Lock()
	succPer[hid]++
	succWin[hid]++
	perMu.Unlock()

	engineStatsMu.Lock()
	addEngineCountersLocked("success", 1)
	engineStatsMu.Unlock()
}

func recordSubmitFail(hid int) {
	atomic.AddUint64(&failCount, 1)
	perMu.Lock()
	failWin[hid]++
	perMu.Unlock()

	engineStatsMu.Lock()
	addEngineCountersLocked("fail", 1)
	engineStatsMu.Unlock()
}

func recordSubmitDLQ(hid int) {
	atomic.AddUint64(&dlqCount, 1)
	perMu.Lock()
	dlqPer[hid]++
	dlqWin[hid]++
	perMu.Unlock()

	engineStatsMu.Lock()
	addEngineCountersLocked("dlq", 1)
	engineStatsMu.Unlock()
}

func resetEngineWindowStats() {
	engineStatsMu.Lock()
	engineWindow = engineCounterSnapshot{}
	engineStatsMu.Unlock()

	perMu.Lock()
	succWin = map[int]uint64{}
	failWin = map[int]uint64{}
	dlqWin = map[int]uint64{}
	perMu.Unlock()
}

func startEngineStatsWindowReset(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(engineStatsWindowMinutes * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				resetEngineWindowStats()
			}
		}
	}()
}

func pingConnStatus(ping func(context.Context) error, addr string) engineConnStatus {
	if ping == nil {
		return engineConnStatus{Ready: false, Message: "未配置", Addr: addr}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ping(ctx); err != nil {
		if addr != "" {
			log.Printf("[连接检测] %s: %v", addr, err)
		} else {
			log.Printf("[连接检测] %v", err)
		}
		return engineConnStatus{Ready: false, Message: "连接失败", Addr: addr}
	}
	return engineConnStatus{Ready: true, Addr: addr}
}

func collectEngineStats(ctx context.Context) engineStatsDTO {
	out := engineStatsDTO{
		WindowMinutes: engineStatsWindowMinutes,
		EngineRunning: OrderEngineReady() && orderQueueStarted,
	}

	engineStatsMu.Lock()
	ensureEngineDayLocked()
	out.Window = engineWindow
	out.Today = engineToday
	engineStatsMu.Unlock()

	out.Lifetime = engineCounterSnapshot{
		Success: atomic.LoadUint64(&successCount),
		Fail:    atomic.LoadUint64(&failCount),
		DLQ:     atomic.LoadUint64(&dlqCount),
	}

	out.Connections.Redis = pingConnStatus(func(c context.Context) error {
		if rdb == nil {
			return fmt.Errorf("未连接")
		}
		return rdb.Ping(c).Err()
	}, "")

	out.Connections.MainMySQL = pingConnStatus(func(c context.Context) error {
		if db == nil {
			return fmt.Errorf("未连接")
		}
		return db.PingContext(c)
	}, "")

	out.Connections.PluginMySQL = pingConnStatus(func(c context.Context) error {
		if pluginDB == nil {
			return fmt.Errorf("未连接")
		}
		return pluginDB.PingContext(c)
	}, "")

	hidsMu.RLock()
	snapshot := append([]int(nil), hids...)
	hidsMu.RUnlock()

	concurrencyMu.RLock()
	workerSnap := make(map[int]int, len(currWorkers))
	for k, v := range currWorkers {
		workerSnap[k] = v
	}
	concurrencyMu.RUnlock()

	nameByHID := huoyuanDisplayNames(ctx, snapshot)

	perMu.Lock()
	windowByHID := make(map[int]struct {
		success, fail, dlq uint64
	}, len(snapshot))
	for _, hid := range snapshot {
		windowByHID[hid] = struct {
			success, fail, dlq uint64
		}{succWin[hid], failWin[hid], dlqWin[hid]}
	}
	perMu.Unlock()

	channels := make([]engineChannelDTO, 0, len(snapshot))
	for _, hid := range snapshot {
		win := windowByHID[hid]
		ch := engineChannelDTO{
			HID:           hid,
			Name:          nameByHID[hid],
			Workers:       workerSnap[hid],
			WindowSuccess: win.success,
			WindowFail:    win.fail,
			WindowDLQ:     win.dlq,
			OpsPaused:     opsIsChannelPaused(hid),
		}
		if rdb != nil {
			ch.QueueDepth, _ = rdb.LLen(ctx, listKey(hid)).Result()
			ch.ProcessingDepth, _ = rdb.LLen(ctx, procKey(hid)).Result()
			ch.DLQDepth, _ = rdb.LLen(ctx, dlqKey(hid)).Result()
		}
		channels = append(channels, ch)
	}
	out.Channels = channels
	return out
}

func huoyuanDisplayNames(ctx context.Context, hids []int) map[int]string {
	out := make(map[int]string, len(hids))
	if len(hids) == 0 {
		return out
	}
	for _, hid := range hids {
		out[hid] = fmt.Sprintf("hid%d", hid)
	}
	if db == nil {
		return out
	}
	placeholders := make([]string, 0, len(hids))
	args := make([]interface{}, 0, len(hids))
	for _, hid := range hids {
		placeholders = append(placeholders, "?")
		args = append(args, hid)
	}
	q := fmt.Sprintf(`SELECT hid, name FROM %s WHERE hid IN (%s)`, tableName("huoyuan"), strings.Join(placeholders, ","))
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var hid int
		var name string
		if rows.Scan(&hid, &name) == nil && strings.TrimSpace(name) != "" {
			out[hid] = strings.TrimSpace(name)
			nameMu.Lock()
			hidToName[hid] = out[hid]
			nameMu.Unlock()
		}
	}
	return out
}
