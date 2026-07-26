package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

const (
	verifyCodeTTL    = 10 * time.Minute
	verifyCodePrefix = "tj:admin:verify:"
)

type verifyEntry struct {
	code     string
	purpose  string
	username string
	extra    string
	expires  time.Time
}

type verifyEntryStored struct {
	Code     string    `json:"code"`
	Purpose  string    `json:"purpose"`
	Username string    `json:"username"`
	Extra    string    `json:"extra"`
	Expires  time.Time `json:"expires"`
}

func (e verifyEntry) toStored() verifyEntryStored {
	return verifyEntryStored{
		Code: e.code, Purpose: e.purpose, Username: e.username,
		Extra: e.extra, Expires: e.expires,
	}
}

func storedToVerifyEntry(s verifyEntryStored) verifyEntry {
	return verifyEntry{
		code: s.Code, purpose: s.Purpose, username: s.Username,
		extra: s.Extra, expires: s.Expires,
	}
}

func normalizeVerifyCodeInput(code string) string {
	code = strings.TrimSpace(code)
	var digits strings.Builder
	for _, r := range code {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	return digits.String()
}

func normalizeVerifyExtra(extra string) string {
	return strings.TrimSpace(extra)
}

var (
	verifyMem   = map[string]verifyEntry{}
	verifyMemMu sync.RWMutex
)

type adminProfileDTO struct {
	Username     string `json:"username"`
	ShowdocURL   string `json:"showdoc_url,omitempty"`
	ShowdocBound bool   `json:"showdoc_bound"`
	TotpEnabled  bool   `json:"totp_enabled"`
}

func ensureAdminUserProfileSchema(ctx context.Context) {
	if pluginDB == nil {
		return
	}
	alters := []string{
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN showdoc_url varchar(512) DEFAULT NULL`, pluginTable("admin_user")),
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN totp_secret varchar(128) DEFAULT NULL`, pluginTable("admin_user")),
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN totp_enabled tinyint(1) NOT NULL DEFAULT 0`, pluginTable("admin_user")),
	}
	for _, q := range alters {
		_, _ = pluginDB.ExecContext(ctx, q)
	}
	loadAlertShowdocFromPluginDB(ctx)
	loadNotificationConfigFromPluginDB(ctx)
}

func adminUsernameFromRequest(ctx context.Context, r *http.Request) string {
	return sessionUserFromRequest(ctx, r)
}

func getAdminProfile(ctx context.Context, username string) (*adminProfileDTO, error) {
	var showdoc sql.NullString
	var totpEnabled int
	q := fmt.Sprintf(`SELECT showdoc_url, totp_enabled FROM %s WHERE username=? LIMIT 1`, pluginTable("admin_user"))
	err := pluginDB.QueryRowContext(ctx, q, username).Scan(&showdoc, &totpEnabled)
	if err != nil {
		return nil, err
	}
	p := &adminProfileDTO{Username: username, TotpEnabled: totpEnabled == 1}
	if showdoc.Valid && strings.TrimSpace(showdoc.String) != "" {
		p.ShowdocURL = strings.TrimSpace(showdoc.String)
		p.ShowdocBound = true
	}
	return p, nil
}

func adminProfileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	user := adminUsernameFromRequest(ctx, r)
	if user == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录", "need_login": true})
		return
	}
	p, err := getAdminProfile(ctx, user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": p})
}

type showdocSendCodeReq struct {
	URL string `json:"url"`
}

func adminShowdocSendCodeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	user := adminUsernameFromRequest(ctx, r)
	if user == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录", "need_login": true})
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req showdocSendCodeReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	pushURL := strings.TrimSpace(req.URL)
	if pushURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请填写 Showdoc 推送地址"})
		return
	}
	token, code, err := issueVerifyCode(ctx, "showdoc_bind", user, pushURL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	title := "智能提交引擎验证码"
	content := fmt.Sprintf("您的验证码为 **%s**，10 分钟内有效。如非本人操作请忽略。", code)
	if err := pushShowdoc(ctx, pushURL, title, content); err != nil {
		deleteVerifyCode(token)
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "推送失败: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1, "msg": "验证码已推送，请查收 Showdoc",
		"data": map[string]string{"verify_token": token},
	})
}

type showdocBindReq struct {
	URL         string `json:"url"`
	Code        string `json:"code"`
	VerifyToken string `json:"verify_token"`
}

func adminShowdocBindHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	user := adminUsernameFromRequest(ctx, r)
	if user == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录", "need_login": true})
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req showdocBindReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	pushURL := strings.TrimSpace(req.URL)
	code := strings.TrimSpace(req.Code)
	token := strings.TrimSpace(req.VerifyToken)
	if pushURL == "" || code == "" || token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请填写推送地址、验证码和 verify_token"})
		return
	}
	if !consumeVerifyCode(token, "showdoc_bind", user, code, pushURL) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "验证码错误或已过期"})
		return
	}
	q := fmt.Sprintf(`UPDATE %s SET showdoc_url=? WHERE username=? LIMIT 1`, pluginTable("admin_user"))
	if _, err := pluginDB.ExecContext(ctx, q, pushURL, user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	syncAlertShowdocURL(ctx, pushURL)
	enableNotificationOnShowdocBind(ctx)
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "Showdoc 绑定成功"})
}

func adminShowdocUnbindHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	user := adminUsernameFromRequest(ctx, r)
	if user == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录", "need_login": true})
		return
	}
	q := fmt.Sprintf(`UPDATE %s SET showdoc_url=NULL WHERE username=? LIMIT 1`, pluginTable("admin_user"))
	_, _ = pluginDB.ExecContext(ctx, q, user)
	syncAlertShowdocURL(ctx, "")
	disableNotificationOnShowdocUnbind(ctx)
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "已解绑 Showdoc"})
}

type passwordChangeReq struct {
	OldPassword     string `json:"old_password"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
	TotpCode        string `json:"totp_code"`
}

func adminPasswordChangeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	user := adminUsernameFromRequest(ctx, r)
	if user == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录", "need_login": true})
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req passwordChangeReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	if len(req.NewPassword) < 6 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "新密码至少 6 位"})
		return
	}
	if req.NewPassword != req.ConfirmPassword {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "两次新密码不一致"})
		return
	}
	ok, err := verifyAdminPassword(ctx, user, req.OldPassword)
	if err != nil || !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "原密码错误"})
		return
	}
	if err := requireAdminTotpIfEnabled(ctx, user, req.TotpCode); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": "密码加密失败"})
		return
	}
	q := fmt.Sprintf(`UPDATE %s SET password_hash=? WHERE username=? LIMIT 1`, pluginTable("admin_user"))
	if _, err := pluginDB.ExecContext(ctx, q, string(hash), user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	revokeAllAdminSessions(ctx, extractSessionToken(r))
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "密码已修改，其他设备上的登录已失效"})
}

func adminTotpSendCodeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	user := adminUsernameFromRequest(ctx, r)
	if user == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录", "need_login": true})
		return
	}
	p, err := getAdminProfile(ctx, user)
	if err != nil || !p.ShowdocBound {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请先绑定 Showdoc 后再开启二步验证"})
		return
	}
	token, code, err := issueVerifyCode(ctx, "totp_enable", user, "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	title := "智能提交引擎 TOTP 绑定验证码"
	content := fmt.Sprintf("开启二步验证的验证码为 **%s**，10 分钟内有效。", code)
	if err := pushShowdoc(ctx, p.ShowdocURL, title, content); err != nil {
		deleteVerifyCode(token)
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "验证码推送失败: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1, "msg": "验证码已推送到 Showdoc",
		"data": map[string]string{"verify_token": token},
	})
}

func adminTotpSetupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	user := adminUsernameFromRequest(ctx, r)
	if user == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录", "need_login": true})
		return
	}
	p, err := getAdminProfile(ctx, user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	if p.TotpEnabled {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "已开启二步验证，请先关闭后再重新绑定"})
		return
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      ProductName,
		AccountName: user,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": "生成密钥失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1, "msg": "ok",
		"data": map[string]string{
			"secret":      key.Secret(),
			"otpauth_url": key.URL(),
		},
	})
}

type totpEnableReq struct {
	Secret      string `json:"secret"`
	TotpCode    string `json:"totp_code"`
	VerifyCode  string `json:"verify_code"`
	VerifyToken string `json:"verify_token"`
}

func adminTotpEnableHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	user := adminUsernameFromRequest(ctx, r)
	if user == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录", "need_login": true})
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req totpEnableReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	secret := strings.TrimSpace(req.Secret)
	totpCode := strings.TrimSpace(req.TotpCode)
	verifyCode := strings.TrimSpace(req.VerifyCode)
	verifyToken := strings.TrimSpace(req.VerifyToken)
	if secret == "" || totpCode == "" || verifyCode == "" || verifyToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请填写完整信息"})
		return
	}
	if !consumeVerifyCode(verifyToken, "totp_enable", user, verifyCode, "") {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "Showdoc 验证码错误或已过期"})
		return
	}
	if !totp.Validate(totpCode, secret) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "验证器动态码错误"})
		return
	}
	q := fmt.Sprintf(`UPDATE %s SET totp_secret=?, totp_enabled=1 WHERE username=? LIMIT 1`, pluginTable("admin_user"))
	if _, err := pluginDB.ExecContext(ctx, q, secret, user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "二步验证已开启"})
}

type totpDisableReq struct {
	Password string `json:"password"`
	TotpCode string `json:"totp_code"`
}

func adminTotpDisableHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	user := adminUsernameFromRequest(ctx, r)
	if user == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录", "need_login": true})
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req totpDisableReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	ok, err := verifyAdminPassword(ctx, user, req.Password)
	if err != nil || !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "密码错误"})
		return
	}
	if err := requireAdminTotpIfEnabled(ctx, user, req.TotpCode); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	q := fmt.Sprintf(`UPDATE %s SET totp_secret=NULL, totp_enabled=0 WHERE username=? LIMIT 1`, pluginTable("admin_user"))
	_, _ = pluginDB.ExecContext(ctx, q, user)
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "二步验证已关闭"})
}

func requireAdminTotpIfEnabled(ctx context.Context, username, totpCode string) error {
	secret, enabled, err := getAdminTotpSecret(ctx, username)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	if strings.TrimSpace(totpCode) == "" {
		return fmt.Errorf("请输入验证器动态码")
	}
	if !totp.Validate(strings.TrimSpace(totpCode), secret) {
		return fmt.Errorf("验证器动态码错误")
	}
	return nil
}

func getAdminTotpSecret(ctx context.Context, username string) (secret string, enabled bool, err error) {
	var sec sql.NullString
	var en int
	q := fmt.Sprintf(`SELECT totp_secret, totp_enabled FROM %s WHERE username=? LIMIT 1`, pluginTable("admin_user"))
	err = pluginDB.QueryRowContext(ctx, q, username).Scan(&sec, &en)
	if err != nil {
		return "", false, err
	}
	if sec.Valid {
		secret = sec.String
	}
	return secret, en == 1, nil
}

func issueVerifyCode(ctx context.Context, purpose, username, extra string) (token, code string, err error) {
	code = randomDigits(6)
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b)
	entry := verifyEntry{
		code: code, purpose: purpose, username: username,
		extra: normalizeVerifyExtra(extra), expires: time.Now().Add(verifyCodeTTL),
	}
	if rdb != nil {
		payload, err := json.Marshal(entry.toStored())
		if err == nil {
			key := verifyCodePrefix + token
			if err := rdb.Set(ctx, key, payload, verifyCodeTTL).Err(); err == nil {
				return token, code, nil
			}
		}
	}
	verifyMemMu.Lock()
	verifyMem[token] = entry
	verifyMemMu.Unlock()
	return token, code, nil
}

func consumeVerifyCode(token, purpose, username, code, extra string) bool {
	entry, ok := loadVerifyEntry(token)
	if !ok {
		return false
	}
	if time.Now().After(entry.expires) {
		deleteVerifyCode(token)
		return false
	}
	if entry.purpose != purpose || entry.username != username {
		deleteVerifyCode(token)
		return false
	}
	extra = normalizeVerifyExtra(extra)
	if extra != "" && entry.extra != extra {
		deleteVerifyCode(token)
		return false
	}
	inputCode := normalizeVerifyCodeInput(code)
	if inputCode == "" || entry.code != inputCode {
		deleteVerifyCode(token)
		return false
	}
	deleteVerifyCode(token)
	return true
}

func loadVerifyEntry(token string) (verifyEntry, bool) {
	if rdb != nil {
		val, err := rdb.Get(context.Background(), verifyCodePrefix+token).Result()
		if err == nil && val != "" {
			var stored verifyEntryStored
			if json.Unmarshal([]byte(val), &stored) == nil && stored.Code != "" {
				return storedToVerifyEntry(stored), true
			}
		}
	}
	verifyMemMu.RLock()
	e, ok := verifyMem[token]
	verifyMemMu.RUnlock()
	return e, ok
}

func deleteVerifyCode(token string) {
	if rdb != nil {
		_ = rdb.Del(context.Background(), verifyCodePrefix+token).Err()
	}
	verifyMemMu.Lock()
	delete(verifyMem, token)
	verifyMemMu.Unlock()
}

func randomDigits(n int) string {
	var s strings.Builder
	for i := 0; i < n; i++ {
		v, _ := rand.Int(rand.Reader, big.NewInt(10))
		s.WriteByte(byte('0' + v.Int64()))
	}
	return s.String()
}

func adminShowdocTestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	user := adminUsernameFromRequest(ctx, r)
	if user == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录", "need_login": true})
		return
	}
	p, err := getAdminProfile(ctx, user)
	if err != nil || !p.ShowdocBound {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "尚未绑定 Showdoc"})
		return
	}
	if err := pushShowdoc(ctx, p.ShowdocURL, "智能提交引擎测试", "这是一条测试推送，绑定正常。"); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "测试推送已发送"})
}

func adminNotificationConfigHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := adminUsernameFromRequest(ctx, r)
	if user == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录", "need_login": true})
		return
	}
	switch r.Method {
	case http.MethodGet:
		p, err := getAdminProfile(ctx, user)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		cfg := getNotificationConfig()
		defaults := defaultNotificationConfig()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": 1,
			"msg":  "ok",
			"data": map[string]interface{}{
				"showdoc_bound": p.ShowdocBound,
				"config":        cfg,
				"defaults":      defaults,
			},
		})
	case http.MethodPut:
		p, err := getAdminProfile(ctx, user)
		if err != nil || !p.ShowdocBound {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请先绑定 Showdoc"})
			return
		}
		body, _ := io.ReadAll(r.Body)
		var cfg NotificationConfig
		if err := json.Unmarshal(body, &cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
			return
		}
		if err := saveNotificationConfig(ctx, cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": 1,
			"msg":  "通知配置已保存",
			"data": getNotificationConfig(),
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
	}
}
