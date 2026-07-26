package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxAdminLogEntries = 3000

type adminLogEntry struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
	Level   string    `json:"level"`
}

type adminLogHub struct {
	mu      sync.RWMutex
	entries []adminLogEntry
	subs    map[chan adminLogEntry]struct{}
}

var appLogHub = &adminLogHub{
	subs: make(map[chan adminLogEntry]struct{}),
}

func classifyAdminLogLevel(msg string) string {
	if strings.Contains(msg, "订单提交成功") || strings.Contains(msg, "提交成功") {
		return "success"
	}
	if strings.Contains(msg, "提交太快") || strings.Contains(msg, "限流") ||
		strings.Contains(msg, "稍后会自动重试") {
		return "warn"
	}
	if strings.Contains(msg, "订单提交失败") || strings.Contains(msg, "提交失败") ||
		strings.Contains(msg, "订单提交超时") || strings.Contains(msg, "提交超时") ||
		strings.Contains(msg, "更新订单状态失败") || strings.Contains(msg, "写入数据库失败") ||
		strings.Contains(msg, "订单处理太久") || strings.Contains(msg, "处理太久") {
		return "error"
	}
	if strings.Contains(msg, "失败订单重新排队") {
		return "info"
	}
	return "info"
}

// pushSubmitLog 写入管理端「提交日志」页，不输出到控制台。
func pushSubmitLog(level, format string, args ...interface{}) {
	msg := RedactSecrets(strings.TrimSpace(fmt.Sprintf(format, args...)))
	if msg == "" {
		return
	}
	if level == "" {
		level = classifyAdminLogLevel(msg)
	}
	appLogHub.pushEntry(msg, level)
}

func (h *adminLogHub) pushEntry(msg, level string) {
	entry := adminLogEntry{
		Time:    time.Now(),
		Message: msg,
		Level:   level,
	}
	h.mu.Lock()
	h.entries = append(h.entries, entry)
	if len(h.entries) > maxAdminLogEntries {
		h.entries = h.entries[len(h.entries)-maxAdminLogEntries:]
	}
	subs := make([]chan adminLogEntry, 0, len(h.subs))
	for ch := range h.subs {
		subs = append(subs, ch)
	}
	h.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
		}
	}
}

// isSubmitStatusLog 仅保留订单提交相关日志供管理端「提交日志」展示
func isSubmitStatusLog(msg string) bool {
	for _, key := range []string{
		"订单提交成功",
		"订单提交失败",
		"订单提交超时",
		"更新订单状态失败",
		"订单处理太久",
		"提交太快",
		"稍后会自动重试",
		"失败订单重新排队",
		// 兼容旧文案
		"提交成功:",
		"提交失败:",
		"提交超时:",
		"写入数据库失败:",
		"处理太久:",
		"提交太快被限流:",
	} {
		if strings.Contains(msg, key) {
			return true
		}
	}
	return false
}

// isEngineRoutineLog 引擎队列/扩缩容/运维巡检等日常日志，不需要 printf 也不进提交日志页
func isEngineRoutineLog(msg string) bool {
	for _, key := range []string{
		"入队扫描完成",
		"已启动消费者:",
		"已扩容:",
		"已缩容:",
		"发现新渠道:",
		"渠道详情",
		"🎯",
		"消费者异常已恢复:",
		"[AI运维]",
	} {
		if strings.Contains(msg, key) {
			return true
		}
	}
	return false
}

func shouldMirrorConsole(msg string) bool {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "fatal") ||
		strings.Contains(msg, "失败") ||
		strings.Contains(msg, "错误") ||
		strings.Contains(msg, "异常") ||
		strings.Contains(msg, "未连接") {
		return true
	}
	if strings.Contains(msg, "警告") || strings.Contains(msg, "⚠") {
		return true
	}
	for _, key := range []string{
		"正在启动",
		"版本:",
		"管理前端已启动",
		"管理端 HTTP 服务已启动",
		"授权验证失败",
		"程序退出",
		"优雅停机",
		"退出进程",
		"订单队列未启动",
		"请先访问管理端",
		"安装完成",
	} {
		if strings.Contains(msg, key) {
			return true
		}
	}
	return false
}

func (h *adminLogHub) appendLine(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !isSubmitStatusLog(raw) {
		return
	}
	entry := adminLogEntry{
		Time:    time.Now(),
		Message: raw,
		Level:   classifyAdminLogLevel(raw),
	}
	h.mu.Lock()
	h.entries = append(h.entries, entry)
	if len(h.entries) > maxAdminLogEntries {
		h.entries = h.entries[len(h.entries)-maxAdminLogEntries:]
	}
	subs := make([]chan adminLogEntry, 0, len(h.subs))
	for ch := range h.subs {
		subs = append(subs, ch)
	}
	h.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
		}
	}
}

func (h *adminLogHub) recent(limit int) []adminLogEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if limit <= 0 || limit > len(h.entries) {
		limit = len(h.entries)
	}
	start := len(h.entries) - limit
	if start < 0 {
		start = 0
	}
	out := make([]adminLogEntry, limit)
	copy(out, h.entries[start:])
	return out
}

func (h *adminLogHub) subscribe() chan adminLogEntry {
	ch := make(chan adminLogEntry, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *adminLogHub) unsubscribe(ch chan adminLogEntry) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *adminLogHub) clear() {
	h.mu.Lock()
	h.entries = nil
	h.mu.Unlock()
}

func adminLogsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 500
		if q := r.URL.Query().Get("limit"); q != "" {
			if n, err := parsePositiveInt(q, 500); err == nil {
				limit = n
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": 1,
			"msg":  "ok",
			"data": appLogHub.recent(limit),
		})
	case http.MethodDelete:
		appLogHub.clear()
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "已清空"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
	}
}

func adminLogsStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	if sessionUserFromRequest(ctx, r) == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录或会话已过期", "need_login": true})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": "不支持流式输出"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// SSE 注释行：帮助部分反向代理/开发代理尽快建立流式连接
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for _, e := range appLogHub.recent(200) {
		writeLogSSE(w, e)
	}
	flusher.Flush()

	ch := appLogHub.subscribe()
	defer appLogHub.unsubscribe(ch)

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case e, ok := <-ch:
			if !ok {
				return
			}
			writeLogSSE(w, e)
			flusher.Flush()
		}
	}
}

func writeLogSSE(w http.ResponseWriter, e adminLogEntry) {
	b, _ := json.Marshal(e)
	fmt.Fprintf(w, "data: %s\n\n", b)
}

func parsePositiveInt(s string, max int) (int, error) {
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n); err != nil {
		return 0, err
	}
	if n <= 0 {
		n = 100
	}
	if n > max {
		n = max
	}
	return n, nil
}
