package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

const internalEnqueueMetaKey = "internal_enqueue_secret"

var (
	internalEnqueueSecret   string
	internalEnqueueSecretMu sync.RWMutex
)

func internalEnqueueURL() string {
	addr := normalizeAdminListenAddr(adminAddr)
	port := "8090"
	if strings.HasPrefix(addr, ":") {
		port = strings.TrimPrefix(addr, ":")
	} else if i := strings.LastIndex(addr, ":"); i >= 0 && i < len(addr)-1 {
		port = addr[i+1:]
	}
	return fmt.Sprintf("http://127.0.0.1:%s/api/internal/enqueue", port)
}

func getInternalEnqueueSecret() string {
	internalEnqueueSecretMu.RLock()
	defer internalEnqueueSecretMu.RUnlock()
	return internalEnqueueSecret
}

func setInternalEnqueueSecret(secret string) {
	internalEnqueueSecretMu.Lock()
	internalEnqueueSecret = strings.TrimSpace(secret)
	internalEnqueueSecretMu.Unlock()
}

func generateInternalEnqueueSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func loadInternalEnqueueSecretFromPluginDB(ctx context.Context) {
	if pluginDB == nil {
		setInternalEnqueueSecret("")
		return
	}
	raw, _ := getSystemMeta(ctx, internalEnqueueMetaKey)
	setInternalEnqueueSecret(raw)
}

func validateInternalEnqueueSecret(secret string) error {
	secret = strings.TrimSpace(secret)
	if len(secret) < 4 {
		return fmt.Errorf("密钥至少 4 个字符")
	}
	if len(secret) > 128 {
		return fmt.Errorf("密钥最多 128 个字符")
	}
	return nil
}

func saveInternalEnqueueSecret(ctx context.Context, secret string) (string, error) {
	if pluginDB == nil {
		return "", fmt.Errorf("插件数据库未就绪")
	}
	secret = strings.TrimSpace(secret)
	if err := validateInternalEnqueueSecret(secret); err != nil {
		return "", err
	}
	if err := setSystemMeta(ctx, internalEnqueueMetaKey, secret); err != nil {
		return "", err
	}
	setInternalEnqueueSecret(secret)
	return secret, nil
}

func ensureInternalEnqueueSecret(ctx context.Context) (string, error) {
	if pluginDB == nil {
		return "", fmt.Errorf("插件数据库未就绪")
	}
	raw, err := getSystemMeta(ctx, internalEnqueueMetaKey)
	if err == nil && strings.TrimSpace(raw) != "" {
		setInternalEnqueueSecret(raw)
		return raw, nil
	}
	secret, err := generateInternalEnqueueSecret()
	if err != nil {
		return "", err
	}
	if err := setSystemMeta(ctx, internalEnqueueMetaKey, secret); err != nil {
		return "", err
	}
	setInternalEnqueueSecret(secret)
	return secret, nil
}

func regenerateInternalEnqueueSecret(ctx context.Context) (string, error) {
	if pluginDB == nil {
		return "", fmt.Errorf("插件数据库未就绪")
	}
	secret, err := generateInternalEnqueueSecret()
	if err != nil {
		return "", err
	}
	if err := setSystemMeta(ctx, internalEnqueueMetaKey, secret); err != nil {
		return "", err
	}
	setInternalEnqueueSecret(secret)
	return secret, nil
}

type settingsInternalEnqueueDTO struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	Ready bool   `json:"ready"`
}

func loadInternalEnqueueSettingsDTO(ctx context.Context) settingsInternalEnqueueDTO {
	out := settingsInternalEnqueueDTO{
		URL: internalEnqueueURL(),
	}
	if !pluginDBReady() {
		return out
	}
	token := strings.TrimSpace(getInternalEnqueueSecret())
	if token == "" {
		raw, _ := getSystemMeta(ctx, internalEnqueueMetaKey)
		token = strings.TrimSpace(raw)
		setInternalEnqueueSecret(token)
	}
	out.Token = token
	out.Ready = token != ""
	return out
}

func adminInternalEnqueueSaveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	if !pluginDBReady() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"code": -1, "msg": "插件数据库未就绪"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	token, err := saveInternalEnqueueSecret(r.Context(), req.Token)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  "密钥已保存，请同步更新主站配置",
		"data": settingsInternalEnqueueDTO{
			URL:   internalEnqueueURL(),
			Token: token,
			Ready: true,
		},
	})
}

func adminInternalEnqueueRegenerateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	if !pluginDBReady() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"code": -1, "msg": "插件数据库未就绪"})
		return
	}
	ctx := r.Context()
	token, err := regenerateInternalEnqueueSecret(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  "已生成新密钥，请同步更新主站配置",
		"data": settingsInternalEnqueueDTO{
			URL:   internalEnqueueURL(),
			Token: token,
			Ready: true,
		},
	})
}
