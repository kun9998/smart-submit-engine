package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

const (
	adminSessionPrefix = "tj:admin:session:"
	adminSessionCookie = "tj_session"
	adminSessionTTL    = 7 * 24 * time.Hour
)

type memSessionEntry struct {
	username string
	expires  time.Time
}

var (
	memSessions   = map[string]memSessionEntry{}
	memSessionsMu sync.RWMutex
)

type authStatusResponse struct {
	Installed bool   `json:"installed"`
	LoggedIn  bool   `json:"logged_in"`
	Username  string `json:"username,omitempty"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TotpCode string `json:"totp_code"`
}

type installRequest struct {
	MainMySQLDSN      string `json:"main_mysql_dsn"`
	PluginMySQLDSN    string `json:"plugin_mysql_dsn"`
	MySQLDSN          string `json:"mysql_dsn"`
	MainHost          string `json:"main_host"`
	MainPort          int    `json:"main_port"`
	MainUser          string `json:"main_user"`
	MainDBPassword    string `json:"main_db_password"`
	MainDatabase      string `json:"main_database"`
	TablePrefix       string `json:"table_prefix"`
	PluginHost        string `json:"plugin_host"`
	PluginPort        int    `json:"plugin_port"`
	PluginUser          string `json:"plugin_user"`
	PluginDBPassword      string `json:"plugin_db_password"`
	PluginDatabase        string `json:"plugin_database"`
	RedisAddr             string `json:"redis_addr"`
	RedisPass         string `json:"redis_pass"`
	RedisDB           int    `json:"redis_db"`
	Username        string `json:"username"`
	Password          string `json:"password"`
	ConfirmPassword   string `json:"confirm_password"`
	Authcode          string `json:"authcode"`
}

type testDBRequest struct {
	MySQLDSN   string `json:"mysql_dsn"`
	DBType     string `json:"db_type"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	DBPassword string `json:"db_password"`
	Password   string `json:"password"`
	Database   string `json:"database"`
}

func (req testDBRequest) dbPass() string {
	if req.DBPassword != "" {
		return req.DBPassword
	}
	return req.Password
}

func (req testDBRequest) resolveDSN() (string, error) {
	if dsn := strings.TrimSpace(req.MySQLDSN); dsn != "" {
		return dsn, nil
	}
	return mysqlConnParams{
		Host: req.Host, Port: req.Port, User: req.User,
		Password: req.dbPass(), Database: req.Database,
	}.DSN()
}

func (req installRequest) resolveMainDSN() (string, error) {
	if dsn := strings.TrimSpace(req.MainMySQLDSN); dsn != "" {
		return dsn, nil
	}
	db := strings.TrimSpace(req.MainDatabase)
	if db == "" {
		return "", fmt.Errorf("请填写主站数据库名称")
	}
	return mysqlConnParams{
		Host: req.MainHost, Port: req.MainPort, User: req.MainUser,
		Password: req.MainDBPassword, Database: db,
	}.DSN()
}

func (req installRequest) mainConnParams() mysqlConnParams {
	return mysqlConnParams{
		Host: req.MainHost, Port: req.MainPort, User: req.MainUser,
		Password: req.MainDBPassword, Database: strings.TrimSpace(req.MainDatabase),
	}
}

func (req testDBRequest) resolveTestDSN() (string, error) {
	if dsn := strings.TrimSpace(req.MySQLDSN); dsn != "" {
		return dsn, nil
	}
	return mysqlConnParams{
		Host: req.Host, Port: req.Port, User: req.User,
		Password: req.dbPass(), Database: req.Database,
	}.DSN()
}

func (req testDBRequest) connParams() mysqlConnParams {
	return mysqlConnParams{
		Host: req.Host, Port: req.Port, User: req.User,
		Password: req.dbPass(), Database: req.Database,
	}
}

func (req installRequest) pluginConnParams() mysqlConnParams {
	return mysqlConnParams{
		Host: req.PluginHost, Port: req.PluginPort, User: req.PluginUser,
		Password: req.PluginDBPassword, Database: strings.TrimSpace(req.PluginDatabase),
	}
}

func (req installRequest) resolvePluginDSN() (string, error) {
	if dsn := strings.TrimSpace(req.PluginMySQLDSN); dsn != "" {
		return dsn, nil
	}
	if dsn := strings.TrimSpace(req.MySQLDSN); dsn != "" {
		return dsn, nil
	}
	db := strings.TrimSpace(req.PluginDatabase)
	if db == "" {
		return "", fmt.Errorf("请填写插件数据库名称")
	}
	if err := validateMySQLDatabaseName(db); err != nil {
		return "", err
	}
	return req.pluginConnParams().DSN()
}

func adminAuthStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	installed := isPluginInstalled(ctx)
	resp := authStatusResponse{Installed: installed}
	if user := sessionUserFromRequest(ctx, r); user != "" {
		resp.LoggedIn = true
		resp.Username = user
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": resp})
}

func adminLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	if !isPluginInstalled(ctx) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "系统尚未安装，请先完成安装"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
		return
	}
	var req loginRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "用户名和密码不能为空"})
		return
	}
	ok, err := verifyAdminPassword(ctx, req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "用户名或密码错误"})
		return
	}
	secret, totpOn, err := getAdminTotpSecret(ctx, req.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	if totpOn {
		if strings.TrimSpace(req.TotpCode) == "" {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"code": 2, "msg": "请输入验证器动态码", "need_totp": true,
			})
			return
		}
		if !totp.Validate(strings.TrimSpace(req.TotpCode), secret) {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "验证器动态码错误"})
			return
		}
	}
	token, err := createAdminSession(ctx, req.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": "创建会话失败"})
		return
	}
	setSessionCookie(w, r, token)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1, "msg": "登录成功",
		"data": map[string]string{"token": token, "username": req.Username},
	})
}

func adminLogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	token := extractSessionToken(r)
	if token != "" {
		if rdb != nil {
			_ = rdb.Del(r.Context(), adminSessionPrefix+token).Err()
		}
		memSessionsMu.Lock()
		delete(memSessions, token)
		memSessionsMu.Unlock()
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "已退出登录"})
}

func adminInstallTestDBHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
		return
	}
	var req testDBRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	ctx := r.Context()
	if req.DBType == "plugin" {
		dbName := strings.TrimSpace(req.Database)
		if dbName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请填写插件数据库名称"})
			return
		}
		if err := validateMySQLDatabaseName(dbName); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		if err := ensureMySQLDatabase(ctx, req.connParams(), dbName); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
	}
	dsn, err := req.resolveTestDSN()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	conn, err := openPluginDB(dsn)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "连接失败: " + err.Error()})
		return
	}
	conn.Close()
	label := "数据库"
	if req.DBType == "main" {
		label = "主站数据库"
	} else if req.DBType == "plugin" {
		label = "插件数据库"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": label + "连接成功"})
}

func adminInstallHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	ctx := r.Context()
	if isPluginInstalled(ctx) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "系统已安装，请直接登录"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
		return
	}
	var req installRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	pluginDSN, err := req.resolvePluginDSN()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	mainDSN, err := req.resolveMainDSN()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	if mainDSN == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请填写主站数据库名称"})
		return
	}
	if _, err := openPluginDB(mainDSN); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "主站数据库连接失败: " + err.Error()})
		return
	}
	pluginDBName := strings.TrimSpace(req.PluginDatabase)
	if pluginDBName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请填写插件数据库名称"})
		return
	}
	if err := ensureMySQLDatabase(ctx, req.pluginConnParams(), pluginDBName); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	if pluginDSN == "" {
		pluginDSN = resolvePluginDSN(nil)
	}
	if pluginDSN == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "插件数据库配置无效"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "用户名至少 3 个字符"})
		return
	}
	if len(req.Password) < 6 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "密码至少 6 个字符"})
		return
	}
	if req.Password != req.ConfirmPassword {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "两次密码不一致"})
		return
	}

	mainPrefix := normalizeMainTablePrefix(req.TablePrefix)
	setPluginTablePrefix(defaultPluginTablePrefix)
	tablePrefix = mainPrefix

	conn, err := runPluginInstallSQL(ctx, pluginDSN, defaultPluginTablePrefix)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		conn.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": "密码加密失败"})
		return
	}
	q := fmt.Sprintf(`INSERT INTO %s (username, password_hash) VALUES (?,?)`, pluginTable("admin_user"))
	if _, err := conn.ExecContext(ctx, q, req.Username, string(hash)); err != nil {
		conn.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": "创建管理员失败: " + err.Error()})
		return
	}

	setPluginDB(conn, pluginDSN)
	_ = importPluginSeedPlatforms(ctx)

	redisAddr := strings.TrimSpace(req.RedisAddr)
	if redisAddr == "" {
		redisAddr = redisConfig.Addr
	}
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}

	if err := saveInstallConfig(mainDSN, pluginDSN, mainPrefix, redisAddr, req.RedisPass, req.RedisDB); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": "保存配置失败: " + err.Error()})
		return
	}

	mysqlDSN = mainDSN
	reconnectMainDBAfterInstall(mainDSN)
	reconnectRedisAfterInstall(redisAddr, req.RedisPass, req.RedisDB)

	if OrderEngineReady() {
		go startOrderQueueWorkers(context.Background())
		log.Printf("安装完成，订单处理已启动")
	} else {
		log.Printf("安装完成，但订单处理未启动，请检查数据库和 Redis 后重启程序")
	}

	n, _ := reloadSubmitRulesAndRegister(ctx)

	token, err := createAdminSession(ctx, req.Username)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": 1, "msg": "安装成功，请登录",
			"data": map[string]int{"rules_loaded": n},
		})
		return
	}
	setSessionCookie(w, r, token)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1, "msg": "安装成功",
		"data": map[string]interface{}{
			"token": token, "username": req.Username, "rules_loaded": n,
		},
	})
}

func parseInstallDSN(r *http.Request) (string, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	var req testDBRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return "", fmt.Errorf("JSON 无效")
	}
	dsn := strings.TrimSpace(req.MySQLDSN)
	if dsn == "" {
		return "", fmt.Errorf("请填写数据库连接串")
	}
	return dsn, nil
}

func createAdminSession(ctx context.Context, username string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	if rdb != nil {
		if err := rdb.Set(ctx, adminSessionPrefix+token, username, adminSessionTTL).Err(); err == nil {
			return token, nil
		}
	}
	memSessionsMu.Lock()
	memSessions[token] = memSessionEntry{username: username, expires: time.Now().Add(adminSessionTTL)}
	memSessionsMu.Unlock()
	return token, nil
}

// revokeAllAdminSessions 吊销管理员会话；exceptToken 非空时保留该 token（改密后保持当前登录）。
func revokeAllAdminSessions(ctx context.Context, exceptToken string) {
	exceptToken = strings.TrimSpace(exceptToken)
	if rdb != nil {
		iter := rdb.Scan(ctx, 0, adminSessionPrefix+"*", 100).Iterator()
		for iter.Next(ctx) {
			key := iter.Val()
			if exceptToken != "" && key == adminSessionPrefix+exceptToken {
				continue
			}
			_ = rdb.Del(ctx, key).Err()
		}
	}
	memSessionsMu.Lock()
	defer memSessionsMu.Unlock()
	if exceptToken == "" {
		memSessions = make(map[string]memSessionEntry)
		return
	}
	for tok := range memSessions {
		if tok != exceptToken {
			delete(memSessions, tok)
		}
	}
}

func sessionUserFromRequest(ctx context.Context, r *http.Request) string {
	token := extractSessionToken(r)
	if token == "" {
		return ""
	}
	if rdb != nil {
		user, err := rdb.Get(ctx, adminSessionPrefix+token).Result()
		if err == nil && user != "" {
			return user
		}
	}
	memSessionsMu.RLock()
	entry, ok := memSessions[token]
	memSessionsMu.RUnlock()
	if !ok {
		return ""
	}
	if time.Now().After(entry.expires) {
		memSessionsMu.Lock()
		delete(memSessions, token)
		memSessionsMu.Unlock()
		return ""
	}
	return entry.username
}

func verifyAdminPassword(ctx context.Context, username, password string) (bool, error) {
	if pluginDB == nil {
		return false, fmt.Errorf("插件数据库未连接")
	}
	var hash string
	q := fmt.Sprintf(`SELECT password_hash FROM %s WHERE username=? LIMIT 1`, pluginTable("admin_user"))
	err := pluginDB.QueryRowContext(ctx, q, username).Scan(&hash)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil, nil
}

func updateAdminPasswordHash(ctx context.Context, username, newPassword string) error {
	if pluginDB == nil {
		return fmt.Errorf("插件数据库未连接")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("密码加密失败")
	}
	q := fmt.Sprintf(`UPDATE %s SET password_hash=? WHERE username=? LIMIT 1`, pluginTable("admin_user"))
	_, err = pluginDB.ExecContext(ctx, q, string(hash), username)
	return err
}

func adminForgotPasswordSendCodeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	if pluginDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"code": -1, "msg": "插件数据库未连接"})
		return
	}
	ctx := r.Context()
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请填写用户名"})
		return
	}

	genericOK := map[string]interface{}{"code": 1, "msg": "若该账号已绑定 Showdoc，验证码已发送"}
	p, err := getAdminProfile(ctx, username)
	if err != nil || !p.ShowdocBound {
		writeJSON(w, http.StatusOK, genericOK)
		return
	}
	token, code, err := issueVerifyCode(ctx, "password_reset", username, "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	title := "智能提交引擎密码重置验证码"
	content := fmt.Sprintf("重置密码验证码为 **%s**，10 分钟内有效。如非本人操作请忽略。", code)
	if err := pushShowdoc(ctx, p.ShowdocURL, title, content); err != nil {
		deleteVerifyCode(token)
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "验证码推送失败: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  "验证码已推送到 Showdoc",
		"data": map[string]string{"verify_token": token},
	})
}

func adminForgotPasswordResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	if pluginDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"code": -1, "msg": "插件数据库未连接"})
		return
	}
	ctx := r.Context()
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Username        string `json:"username"`
		VerifyCode      string `json:"verify_code"`
		VerifyToken     string `json:"verify_token"`
		NewPassword     string `json:"new_password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	username := strings.TrimSpace(req.Username)
	token := strings.TrimSpace(req.VerifyToken)
	code := strings.TrimSpace(req.VerifyCode)
	if username == "" || token == "" || code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请填写完整信息"})
		return
	}
	if len(req.NewPassword) < 6 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "新密码至少 6 位"})
		return
	}
	if req.NewPassword != req.ConfirmPassword {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "两次密码不一致"})
		return
	}
	p, err := getAdminProfile(ctx, username)
	if err != nil || !p.ShowdocBound {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "该账号未绑定 Showdoc，无法通过此方式重置密码"})
		return
	}
	if !consumeVerifyCode(token, "password_reset", username, code, "") {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "验证码错误或已过期"})
		return
	}
	if err := updateAdminPasswordHash(ctx, username, req.NewPassword); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	revokeAllAdminSessions(ctx, "")
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "密码已重置，请使用新密码登录"})
}

func extractSessionToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if t := r.Header.Get("X-Session-Token"); t != "" {
		return t
	}
	if c, err := r.Cookie(adminSessionCookie); err == nil {
		if t := strings.TrimSpace(c.Value); t != "" {
			return t
		}
	}
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return ""
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(adminSessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func adminRequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if !isPluginInstalled(ctx) {
			writeJSON(w, http.StatusForbidden, map[string]interface{}{"code": -1, "msg": "请先完成安装", "need_install": true})
			return
		}
		if sessionUserFromRequest(ctx, r) == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"code": -1, "msg": "未登录或会话已过期", "need_login": true})
			return
		}
		next(w, r)
	}
}

func adminRequireNotInstalled(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isPluginInstalled(r.Context()) {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "系统已安装"})
			return
		}
		next(w, r)
	}
}
