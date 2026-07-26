package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	adminEnabled bool
	adminAddr    string
)

func initAdminFromConfig(fc *fileConfig) {
	adminEnabled = fc.Admin.Enabled
	adminAddr = normalizeAdminListenAddr(fc.Admin.Addr)
}

func startAdminServer(ctx context.Context) {
	if !adminEnabled {
		return
	}
	mux := http.NewServeMux()

	// 内网入队（主站同机调用，不经过管理端登录）
	mux.HandleFunc("/api/internal/enqueue", internalEnqueueHandler)

	// 公开接口
	mux.HandleFunc("/api/auth/status", adminAuthStatusHandler)
	mux.HandleFunc("/api/auth/login", adminLoginHandler)
	mux.HandleFunc("/api/auth/logout", adminLogoutHandler)
	mux.HandleFunc("/api/auth/forgot-password/send-code", adminForgotPasswordSendCodeHandler)
	mux.HandleFunc("/api/auth/forgot-password/reset", adminForgotPasswordResetHandler)
	mux.HandleFunc("/api/install", adminRequireNotInstalled(adminInstallHandler))
	mux.HandleFunc("/api/install/test-db", adminRequireNotInstalled(adminInstallTestDBHandler))

	// 需登录
	mux.HandleFunc("/api/submit-platforms", adminRequireAuth(adminSubmitPlatformsHandler))
	mux.HandleFunc("/api/submit-platforms/reload", adminRequireAuth(adminReloadCacheHandler))
	mux.HandleFunc("/api/submit-platforms/ai-status", adminRequireAuth(adminRuleAIStatusHandler))
	mux.HandleFunc("/api/submit-platforms/ai-convert", adminRequireAuth(adminRuleAIConvertHandler))
	mux.HandleFunc("/api/submit-platforms/", adminRequireAuth(adminSubmitPlatformByTypeHandler))
	mux.HandleFunc("/api/logs", adminRequireAuth(adminLogsHandler))
	mux.HandleFunc("/api/logs/stream", adminLogsStreamHandler)

	mux.HandleFunc("/api/profile", adminRequireAuth(adminProfileHandler))
	mux.HandleFunc("/api/profile/showdoc/send-code", adminRequireAuth(adminShowdocSendCodeHandler))
	mux.HandleFunc("/api/profile/showdoc/bind", adminRequireAuth(adminShowdocBindHandler))
	mux.HandleFunc("/api/profile/showdoc/unbind", adminRequireAuth(adminShowdocUnbindHandler))
	mux.HandleFunc("/api/profile/showdoc/test", adminRequireAuth(adminShowdocTestHandler))
	mux.HandleFunc("/api/profile/notifications", adminRequireAuth(adminNotificationConfigHandler))
	mux.HandleFunc("/api/profile/password", adminRequireAuth(adminPasswordChangeHandler))
	mux.HandleFunc("/api/profile/totp/send-code", adminRequireAuth(adminTotpSendCodeHandler))
	mux.HandleFunc("/api/profile/totp/setup", adminRequireAuth(adminTotpSetupHandler))
	mux.HandleFunc("/api/profile/totp/enable", adminRequireAuth(adminTotpEnableHandler))
	mux.HandleFunc("/api/profile/totp/disable", adminRequireAuth(adminTotpDisableHandler))

	mux.HandleFunc("/api/huoyuan", adminRequireAuth(adminHuoyuanListHandler))
	mux.HandleFunc("/api/huoyuan-config/global", adminRequireAuth(adminHuoyuanConfigGlobalHandler))
	mux.HandleFunc("/api/huoyuan-config/hid/", adminRequireAuth(adminHuoyuanConfigByHIDHandler))

	mux.HandleFunc("/api/system/info", adminRequireAuth(adminSystemInfoHandler))
	mux.HandleFunc("/api/system/monitor", adminRequireAuth(adminSystemMonitorHandler))
	mux.HandleFunc("/api/system/engine-stats", adminRequireAuth(adminEngineStatsHandler))
	mux.HandleFunc("/api/system/settings", adminRequireAuth(adminSystemSettingsHandler))
	mux.HandleFunc("/api/system/settings/test-redis", adminRequireAuth(adminSystemSettingsTestRedisHandler))
	mux.HandleFunc("/api/system/settings/internal-enqueue", adminRequireAuth(adminInternalEnqueueSaveHandler))
	mux.HandleFunc("/api/system/settings/internal-enqueue/regenerate", adminRequireAuth(adminInternalEnqueueRegenerateHandler))
	mux.HandleFunc("/api/system/upgrade/status", adminRequireAuth(adminUpgradeStatusHandler))
	mux.HandleFunc("/api/system/upgrade/check", adminRequireAuth(adminUpgradeCheckHandler))
	mux.HandleFunc("/api/system/upgrade/apply", adminRequireAuth(adminUpgradeApplyHandler))

	registerOpsRoutes(mux)

	mux.Handle("/", spaFileServer())

	srv := &http.Server{
		Addr:              adminAddr,
		Handler:           corsMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("管理端 HTTP 服务已启动（监听 %s）", adminListenLabel())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("管理前端 HTTP 服务异常: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if getOpsConfig().Enabled {
		bindOpsWatcherLifecycle(ctx)
		startOpsWatcher(ctx)
	}
}

func spaFileServer() http.Handler {
	staticDir := resolveAdminStaticDir()
	fs := http.FileServer(http.Dir("./" + staticDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"code": -1, "msg": "接口不存在"})
			return
		}
		path := "./" + staticDir + r.URL.Path
		if r.URL.Path != "/" {
			if info, err := os.Stat(path); err != nil || info.IsDir() {
				http.ServeFile(w, r, "./"+staticDir+"/index.html")
				return
			}
		}
		fs.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func adminSubmitPlatformsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		list, err := listSubmitPlatformsFromDB(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		dtos := make([]submitPlatformDTO, 0, len(list))
		for _, row := range list {
			dtos = append(dtos, rowToDTO(row))
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": dtos})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
			return
		}
		var req submitPlatformDTO
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
			return
		}
		req.PlatformType = strings.TrimSpace(req.PlatformType)
		if req.PlatformType == "" {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "platform_type 不能为空"})
			return
		}
		row := SubmitPlatformRow{
			PlatformType: req.PlatformType,
			DisplayName:  req.DisplayName,
			Enabled:      req.Enabled,
			RuleConfig:   req.RuleConfig,
			Remark:       req.Remark,
		}
		if err := insertSubmitPlatform(ctx, &row); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		if err := persistSubmitPlatformSourcePHP(ctx, req.PlatformType, req.SourcePHP); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "创建成功"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
	}
}

func adminSubmitPlatformByTypeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := strings.TrimPrefix(r.URL.Path, "/api/submit-platforms/")
	path = strings.Trim(path, "/")
	if path == "" || path == "reload" {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"code": -1, "msg": "未找到"})
		return
	}
	if strings.HasSuffix(path, "/test-submit") {
		platformType := strings.TrimSpace(strings.TrimSuffix(path, "/test-submit"))
		adminRuleTestSubmitHandler(w, r, platformType)
		return
	}
	if strings.HasSuffix(path, "/ai-fix-from-failure") {
		platformType := strings.TrimSpace(strings.TrimSuffix(path, "/ai-fix-from-failure"))
		adminRuleFixFromFailureHandler(w, r, platformType)
		return
	}
	platformType := strings.TrimSpace(path)

	switch r.Method {
	case http.MethodGet:
		row, err := getSubmitRuleCached(ctx, platformType)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		if row == nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"code": -1, "msg": "未找到"})
			return
		}
		dto := rowToDTO(*row)
		enrichSubmitPlatformDTO(ctx, &dto)
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": dto})
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
			return
		}
		var req submitPlatformDTO
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
			return
		}
		row := SubmitPlatformRow{
			DisplayName: req.DisplayName,
			Enabled:     req.Enabled,
			RuleConfig:  req.RuleConfig,
			Remark:      req.Remark,
		}
		if err := updateSubmitPlatform(ctx, platformType, &row); err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"code": -1, "msg": "未找到"})
			return
		} else if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		if err := persistSubmitPlatformSourcePHP(ctx, platformType, req.SourcePHP); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "更新成功"})
	case http.MethodDelete:
		if err := deleteSubmitPlatform(ctx, platformType); err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"code": -1, "msg": "未找到"})
			return
		} else if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "删除成功"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
	}
}

func adminReloadCacheHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	n, err := reloadSubmitRulesAndRegister(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "缓存已刷新", "data": map[string]int{"count": n}})
}
