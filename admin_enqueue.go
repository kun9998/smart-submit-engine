package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func requestClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

func isInternalRequestIP(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func internalEnqueueAllowed(r *http.Request) bool {
	if !isInternalRequestIP(requestClientIP(r)) {
		return false
	}
	if getInternalEnqueueSecret() == "" {
		return false
	}
	token := strings.TrimSpace(r.Header.Get("X-Tj-Enqueue-Token"))
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("Authorization"))
		token = strings.TrimPrefix(token, "Bearer ")
	}
	return token == getInternalEnqueueSecret()
}

type internalEnqueueRequest struct {
	OID int `json:"oid"`
	HID int `json:"hid,omitempty"`
}

func internalEnqueueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	if !internalEnqueueAllowed(r) {
		writeJSON(w, http.StatusForbidden, map[string]interface{}{"code": -1, "msg": "仅允许本机或内网调用，且需提供正确的入队密钥"})
		return
	}
	if !OrderEngineReady() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"code": -1, "msg": "订单引擎未就绪（主库或 Redis 未连接）"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
		return
	}
	var req internalEnqueueRequest
	if err := json.Unmarshal(body, &req); err != nil || req.OID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请提供有效的 oid"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	hid, queued, errMsg := pushOrderToQueue(ctx, req.OID, req.HID)
	if errMsg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": errMsg})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  queued,
		"data": map[string]interface{}{"oid": req.OID, "hid": hid},
	})
}

// pushOrderToQueue 校验订单并入 Redis 队列；返回 hid、提示文案、错误信息。
func pushOrderToQueue(ctx context.Context, oid, reqHID int) (hid int, queued string, errMsg string) {
	if db == nil {
		return 0, "", "主站数据库未连接"
	}
	if rdb == nil {
		return 0, "", "Redis 未连接"
	}

	orderTable := tableName("order")
	var dbHID int
	var dockStatus string
	err := db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT hid, dockstatus FROM %s WHERE oid=? AND status!='已取消' LIMIT 1`, orderTable),
		oid,
	).Scan(&dbHID, &dockStatus)
	if err == sql.ErrNoRows {
		return 0, "", "订单不存在或已取消"
	}
	if err != nil {
		return 0, "", err.Error()
	}
	if dockStatus != "0" {
		return 0, "", "订单不在待提交状态"
	}
	if dbHID <= 0 {
		return 0, "", "订单未绑定货源"
	}
	if reqHID > 0 && reqHID != dbHID {
		return 0, "", fmt.Sprintf("hid 与订单不一致（订单为 %d）", dbHID)
	}
	hid = dbHID

	if opsIsChannelPaused(hid) {
		return hid, "", "该货源渠道已暂停，暂不入队"
	}

	msg := orderMsg{OID: oid, HID: hid, R: 0, TS: time.Now().Unix()}
	if err := enqueueOrderToHIDQueue(ctx, hid, msg); err != nil {
		if strings.Contains(err.Error(), "enq dedup blocked") {
			return hid, "已在队列", ""
		}
		return hid, "", err.Error()
	}

	ensureOrderConsumerForHID(hid)
	return hid, "已入队", ""
}

// ensureOrderConsumerForHID 新货源首次入队时立即启动 consumer，避免等扫库周期。
func ensureOrderConsumerForHID(hid int) {
	if hid <= 0 || orderQueueRootCtx == nil || !orderQueueStarted {
		return
	}
	if opsIsChannelPaused(hid) {
		return
	}

	concurrencyMu.RLock()
	hasWorkers := currWorkers[hid] > 0
	concurrencyMu.RUnlock()
	if hasWorkers {
		return
	}

	if rateLimitEnabled {
		ensureRateLimiterForHID(hid)
	}

	qStart := getEffectiveQueueForHID(hid)
	concurrencyMu.Lock()
	defer concurrencyMu.Unlock()
	if currWorkers[hid] > 0 {
		return
	}
	for i := 0; i < qStart.MinWorkersPerHID; i++ {
		wctx, cancel := context.WithCancel(orderQueueRootCtx)
		orderQueueWorkerWG.Add(1)
		go consumer(wctx, hid, &orderQueueWorkerWG)
		workerCancels[hid] = append(workerCancels[hid], cancel)
		currWorkers[hid]++
	}

	hidsMu.Lock()
	for _, h := range hids {
		if h == hid {
			hidsMu.Unlock()
			return
		}
	}
	hids = append(hids, hid)
	hidsMu.Unlock()
}
