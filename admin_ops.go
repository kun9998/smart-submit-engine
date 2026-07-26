package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func registerOpsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/ops/config", adminRequireAuth(adminOpsConfigHandler))
	mux.HandleFunc("/api/ops/status", adminRequireAuth(adminOpsStatusHandler))
	mux.HandleFunc("/api/ops/analyze", adminRequireAuth(adminOpsAnalyzeHandler))
	mux.HandleFunc("/api/ops/audit", adminRequireAuth(adminOpsAuditListHandler))
	mux.HandleFunc("/api/ops/", adminRequireAuth(adminOpsSubHandler))
}

func adminOpsConfigHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": getOpsConfig()})
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
			return
		}
		var req OpsConfig
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
			return
		}
		if err := saveOpsConfig(ctx, req); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		if req.Enabled {
			startOpsWatcher(opsWatcherContext())
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "保存成功", "data": getOpsConfig()})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
	}
}

func adminOpsStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	cfg := getOpsConfig()
	var lastIncident interface{}
	if row, err := getLatestOpsAudit(r.Context()); err == nil && row != nil {
		lastIncident = map[string]interface{}{
			"id":      row.ID,
			"summary": row.Summary,
			"status":  row.Status,
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  "ok",
		"data": map[string]interface{}{
			"enabled":          cfg.Enabled,
			"mode":             cfg.Mode,
			"ai_ready":         opsAIReady(),
			"watcher_running":  opsWatcherActive(),
			"paused_channels":  pausedChannelDTOs(),
			"last_incident":    lastIncident,
		},
	})
}

func adminOpsAnalyzeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Execute bool `json:"execute"`
	}
	_ = json.Unmarshal(body, &req)

	operator := sessionUserFromRequest(r.Context(), r)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	result, err := runOpsAnalyze(ctx, "manual", req.Execute, false, operator)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": result})
}

func adminOpsAuditListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, total, err := listOpsAudit(r.Context(), page, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  "ok",
		"data": map[string]interface{}{
			"items": rows,
			"total": total,
			"page":  page,
			"limit": limit,
		},
	})
}

func adminOpsSubHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/ops/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"code": -1, "msg": "未找到"})
		return
	}

	switch parts[0] {
	case "audit":
		if len(parts) == 2 {
			adminOpsAuditDetailHandler(w, r, parts[1])
			return
		}
	case "rollback":
		if len(parts) == 2 && r.Method == http.MethodPost {
			adminOpsRollbackHandler(w, r, parts[1])
			return
		}
	case "channels":
		if len(parts) == 3 {
			hid, err := strconv.Atoi(parts[1])
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "hid 无效"})
				return
			}
			switch parts[2] {
			case "pause":
				adminOpsChannelPauseHandler(w, r, hid)
				return
			case "resume":
				adminOpsChannelResumeHandler(w, r, hid)
				return
			}
		}
	case "report":
		if len(parts) == 2 && parts[1] == "daily" && r.Method == http.MethodGet {
			adminOpsDailyReportHandler(w, r)
			return
		}
	}

	writeJSON(w, http.StatusNotFound, map[string]interface{}{"code": -1, "msg": "未找到"})
}

func adminOpsAuditDetailHandler(w http.ResponseWriter, r *http.Request, idStr string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "id 无效"})
		return
	}
	row, err := getOpsAuditByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	if row == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"code": -1, "msg": "未找到"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": row})
}

func adminOpsRollbackHandler(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "id 无效"})
		return
	}
	operator := sessionUserFromRequest(r.Context(), r)
	results, err := runOpsRollback(r.Context(), id, operator)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "回滚完成", "data": results})
}

func adminOpsChannelPauseHandler(w http.ResponseWriter, r *http.Request, hid int) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	if err := opsPauseChannel(hid); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "已暂停"})
}

func adminOpsChannelResumeHandler(w http.ResponseWriter, r *http.Request, hid int) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	if err := opsResumeChannel(hid); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "已恢复"})
}

func adminOpsDailyReportHandler(w http.ResponseWriter, r *http.Request) {
	loadOpsDailyReportFromMeta(r.Context())
	report := getLatestOpsDailyReport()
	if report.Date == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": 1,
			"msg":  "ok",
			"data": nil,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  "ok",
		"data": report,
	})
}
