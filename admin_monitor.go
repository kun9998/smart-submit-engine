package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type systemMonitorDTO struct {
	System  monitorSystemInfo  `json:"system"`
	Runtime monitorRuntimeInfo `json:"runtime"`
	Process monitorProcessInfo `json:"process"`
	CPU     monitorCPUInfo     `json:"cpu"`
	Memory  monitorMemoryInfo  `json:"memory"`
}

type monitorSystemInfo struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Hostname string `json:"hostname"`
}

type monitorRuntimeInfo struct {
	PID           int    `json:"pid"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	UptimeText    string `json:"uptime_text"`
	StartedAt     string `json:"started_at"`
}

type monitorProcessInfo struct {
	MemoryBytes uint64 `json:"memory_bytes"`
	MemoryText  string `json:"memory_text"`
	User        string `json:"user"`
}

type monitorCPUInfo struct {
	UsagePercent float64  `json:"usage_percent"`
	Cores        int      `json:"cores"`
	LoadAvg5     *float64 `json:"load_avg_5,omitempty"`
	LoadAvg15    *float64 `json:"load_avg_15,omitempty"`
}

type monitorMemoryInfo struct {
	UsagePercent float64 `json:"usage_percent"`
	TotalBytes   uint64  `json:"total_bytes"`
	UsedBytes    uint64  `json:"used_bytes"`
	FreeBytes    uint64  `json:"free_bytes"`
	TotalText    string  `json:"total_text"`
	UsedText     string  `json:"used_text"`
	FreeText     string  `json:"free_text"`
}

func markAppStarted() {
	if appStartedAt.IsZero() {
		appStartedAt = time.Now()
	}
}

func collectSystemMonitor() systemMonitorDTO {
	now := time.Now()
	uptime := now.Sub(appStartedAt)
	if appStartedAt.IsZero() {
		uptime = 0
	}

	hostname, _ := os.Hostname()
	runUser := monitorRunUser()
	procMem := monitorProcessMemoryBytes()

	cpuUsage, load5, load15 := monitorCPUStats()
	memTotal, memUsed, memFree := monitorSystemMemory()

	memUsagePct := 0.0
	if memTotal > 0 {
		memUsagePct = float64(memUsed) / float64(memTotal) * 100
	}

	return systemMonitorDTO{
		System: monitorSystemInfo{
			OS:       monitorOSName(),
			Arch:     monitorArchName(),
			Hostname: hostname,
		},
		Runtime: monitorRuntimeInfo{
			PID:           os.Getpid(),
			UptimeSeconds: int64(uptime.Seconds()),
			UptimeText:    formatDurationCN(uptime),
			StartedAt:     appStartedAt.Local().Format("2006-01-02 15:04:05"),
		},
		Process: monitorProcessInfo{
			MemoryBytes: procMem,
			MemoryText:  formatBytesHuman(procMem),
			User:        runUser,
		},
		CPU: monitorCPUInfo{
			UsagePercent: cpuUsage,
			Cores:        runtime.NumCPU(),
			LoadAvg5:     load5,
			LoadAvg15:    load15,
		},
		Memory: monitorMemoryInfo{
			UsagePercent: memUsagePct,
			TotalBytes:   memTotal,
			UsedBytes:    memUsed,
			FreeBytes:    memFree,
			TotalText:    formatBytesHuman(memTotal),
			UsedText:     formatBytesHuman(memUsed),
			FreeText:     formatBytesHuman(memFree),
		},
	}
}

func monitorOSName() string {
	switch runtime.GOOS {
	case "linux":
		return "linux"
	case "windows":
		return "windows"
	case "darwin":
		return "macOS"
	default:
		return runtime.GOOS
	}
}

func monitorArchName() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "386":
		return "x86"
	case "arm64":
		return "aarch64"
	case "arm":
		return "arm"
	default:
		return runtime.GOARCH
	}
}

func monitorRunUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		name := u.Username
		if i := strings.LastIndex(name, `\`); i >= 0 {
			name = name[i+1:]
		}
		return name
	}
	return "-"
}

func monitorProcessMemoryBytes() uint64 {
	if rss, ok := readProcessRSS(); ok {
		return rss
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Alloc
}

func formatDurationCN(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSec := int64(d.Seconds())
	days := totalSec / 86400
	totalSec %= 86400
	hours := totalSec / 3600
	totalSec %= 3600
	minutes := totalSec / 60
	seconds := totalSec % 60
	return fmt.Sprintf("%d 天 %d 小时 %d 分钟 %d 秒", days, hours, minutes, seconds)
}

func formatBytesHuman(n uint64) string {
	if n == 0 {
		return "0 B"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit && exp < 3; v /= unit {
		div *= unit
		exp++
	}
	value := float64(n) / float64(div)
	suffix := []string{"KB", "MB", "GB", "TB"}[exp]
	if value >= 100 || exp == 0 {
		return fmt.Sprintf("%.0f %s", value, suffix)
	}
	if value >= 10 {
		return fmt.Sprintf("%.1f %s", value, suffix)
	}
	return fmt.Sprintf("%.2f %s", value, suffix)
}

func readProcessRSSLinux() (uint64, bool) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseUint(fields[1], 10, 64)
				if err == nil {
					return kb * 1024, true
				}
			}
		}
	}
	return 0, false
}

func adminSystemMonitorHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  "ok",
		"data": collectSystemMonitor(),
	})
}
