package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

type settingsRedisDTO struct {
	Addr           string `json:"addr"`
	AddrConfigured bool   `json:"addr_configured"`
	Pass           string `json:"pass"`
	DB             int    `json:"db"`
	PassSet        bool   `json:"pass_set"`
}

type settingsHTTPSecurityDTO struct {
	HostWhitelist          []string `json:"host_whitelist"`
	BlockPrivateNetworks   bool     `json:"block_private_networks"`
	AllowInsecureHTTPToLAN bool     `json:"allow_insecure_http_to_lan"`
}

type settingsAuthDTO struct {
	Authcode    string `json:"authcode"`
	AuthcodeSet bool   `json:"authcode_set"`
}

type settingsAIDTO struct {
	Enabled   bool   `json:"enabled"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	APIKey    string `json:"api_key"`
	APIKeySet bool   `json:"api_key_set"`
}

type systemSettingsDTO struct {
	Redis           settingsRedisDTO           `json:"redis"`
	HTTPSecurity    settingsHTTPSecurityDTO    `json:"http_security"`
	Auth            settingsAuthDTO            `json:"auth"`
	AI              settingsAIDTO              `json:"ai"`
	InternalEnqueue settingsInternalEnqueueDTO `json:"internal_enqueue"`
	NeedRestart     bool                       `json:"need_restart"`
}

type settingsUpdateRequest struct {
	Redis        *settingsRedisDTO        `json:"redis,omitempty"`
	HTTPSecurity *settingsHTTPSecurityDTO `json:"http_security,omitempty"`
	Auth         *settingsAuthDTO         `json:"auth,omitempty"`
	AI           *settingsAIDTO           `json:"ai,omitempty"`
}

type testRedisRequest struct {
	Addr string `json:"addr"`
	Pass string `json:"pass"`
	DB   int    `json:"db"`
}

func loadSystemSettingsDTO() (systemSettingsDTO, error) {
	if pluginDBReady() {
		loadAIConfigFromPluginDB(context.Background())
	}
	fc, err := loadConfigFile()
	if err != nil {
		return systemSettingsDTO{}, err
	}
	out := systemSettingsDTO{
		Redis: settingsRedisDTO{
			Addr:           "",
			AddrConfigured: strings.TrimSpace(fc.Redis.Addr) != "",
			Pass:           "",
			DB:             fc.Redis.DB,
			PassSet:        strings.TrimSpace(fc.Redis.Pass) != "",
		},
		Auth: settingsAuthDTO{
			Authcode:    "",
			AuthcodeSet: false,
		},
	}
	hs := fc.HTTPSecurity
	allowLAN := hs.AllowInsecureHTTPToLAN
	if productionHTTPSecurityLocked() {
		allowLAN = false
	}
	out.HTTPSecurity = settingsHTTPSecurityDTO{
		HostWhitelist:          append([]string(nil), hs.HostWhitelist...),
		BlockPrivateNetworks:   hs.BlockPrivateNetworks == nil || *hs.BlockPrivateNetworks,
		AllowInsecureHTTPToLAN: allowLAN,
	}
	aiCfg := getAIConfig()
	out.AI = settingsAIDTO{
		Enabled:   aiCfg.Enabled,
		BaseURL:   aiCfg.BaseURL,
		Model:     aiCfg.Model,
		APIKey:    "",
		APIKeySet: strings.TrimSpace(aiCfg.APIKey) != "",
	}
	out.InternalEnqueue = loadInternalEnqueueSettingsDTO(context.Background())
	return out, nil
}

func testRedisConnection(addr, pass string, dbNum int) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return fmt.Errorf("Redis 地址不能为空")
	}
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     pass,
		DB:           dbNum,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return client.Ping(ctx).Err()
}

func adminEngineStatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  "ok",
		"data": collectEngineStats(ctx),
	})
}

func adminSystemSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data, err := loadSystemSettingsDTO()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": data})
	case http.MethodPut:
		adminSystemSettingsUpdateHandler(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
	}
}

func adminSystemSettingsUpdateHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
		return
	}
	var req settingsUpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}

	fc, err := loadConfigFile()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}

	needRestart := false
	applied := []string{}

	if req.Redis != nil {
		addr := strings.TrimSpace(fc.Redis.Addr)
		if s := strings.TrimSpace(req.Redis.Addr); s != "" {
			addr = s
		}
		if addr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "Redis 地址不能为空"})
			return
		}
		pass := fc.Redis.Pass
		if strings.TrimSpace(req.Redis.Pass) != "" {
			pass = req.Redis.Pass
		}
		if err := testRedisConnection(addr, pass, req.Redis.DB); err != nil {
			log.Printf("[设置] Redis 连接测试失败: %v", err)
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "Redis 连接测试失败"})
			return
		}
		fc.Redis.Addr = addr
		fc.Redis.Pass = pass
		fc.Redis.DB = req.Redis.DB
		if err := saveConfigFile(fc); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		applyFileConfigToRuntime(&fc)
		reconnectRedisAfterInstall(fc.Redis.Addr, fc.Redis.Pass, fc.Redis.DB)
		if OrderEngineReady() && !orderQueueStarted {
			go startOrderQueueWorkers(context.Background())
		}
		applied = append(applied, "Redis")
	}

	if req.HTTPSecurity != nil {
		blockPrivate := req.HTTPSecurity.BlockPrivateNetworks
		allowLAN := req.HTTPSecurity.AllowInsecureHTTPToLAN
		if productionHTTPSecurityLocked() {
			if allowLAN {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{
					"code": -1,
					"msg":  "系统已安装，生产环境不允许开启「HTTP 访问内网」",
				})
				return
			}
			allowLAN = false
		}
		fc.HTTPSecurity = HTTPSecurity{
			HostWhitelist:          normalizeHostWhitelist(req.HTTPSecurity.HostWhitelist),
			BlockPrivateNetworks:   &blockPrivate,
			AllowInsecureHTTPToLAN: allowLAN,
		}
		if err := saveConfigFile(fc); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		initHTTPSecurityFromConfig(&fc)
		applied = append(applied, "HTTP 安全策略")
	}


	if req.AI != nil {
		if !pluginDBReady() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"code": -1, "msg": "插件数据库未就绪"})
			return
		}
		cfg, err := applyAISettingsUpdate(getAIConfig(), req.AI)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		if err := saveAIConfig(r.Context(), cfg); err != nil {
			log.Printf("[设置] 保存 AI 配置失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": "保存 AI 配置失败"})
			return
		}
		applied = append(applied, "AI 转换")
	}

	data, _ := loadSystemSettingsDTO()
	data.NeedRestart = needRestart
	msg := "保存成功"
	if len(applied) > 0 {
		msg = "已更新：" + strings.Join(applied, "、")
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": msg, "data": data})
}

func normalizeHostWhitelist(list []string) []string {
	out := make([]string, 0, len(list))
	seen := map[string]struct{}{}
	for _, h := range list {
		h = strings.ToLower(strings.TrimSpace(h))
		h = strings.TrimPrefix(h, ".")
		if h == "" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	return out
}

func adminSystemSettingsTestRedisHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
		return
	}
	var req testRedisRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	addr := strings.TrimSpace(req.Addr)
	if addr == "" {
		fc, err := loadConfigFile()
		if err == nil {
			addr = strings.TrimSpace(fc.Redis.Addr)
		}
	}
	pass := req.Pass
	if strings.TrimSpace(pass) == "" {
		fc, err := loadConfigFile()
		if err == nil {
			pass = fc.Redis.Pass
		}
	}
	if err := testRedisConnection(addr, pass, req.DB); err != nil {
		log.Printf("[设置] Redis 连接测试失败: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "连接失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "连接成功"})
}
