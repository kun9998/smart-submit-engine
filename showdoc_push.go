package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type showdocPushResult struct {
	ErrorCode    int    `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

// pushShowdoc 调用 Showdoc API 推送（title + content，支持 GET/POST 表单）
func pushShowdoc(ctx context.Context, pushURL, title, content string) error {
	pushURL = strings.TrimSpace(pushURL)
	if pushURL == "" {
		return fmt.Errorf("推送地址为空")
	}
	if err := ValidateOutboundHTTPURL(ctx, pushURL); err != nil {
		return fmt.Errorf("推送地址无效")
	}
	title = RedactSecrets(strings.TrimSpace(title))
	content = RedactSecrets(strings.TrimSpace(content))
	if title == "" || content == "" {
		return fmt.Errorf("标题和内容不能为空")
	}

	client := NewOutboundHTTPClient(15 * time.Second)

	// 优先 POST 表单（Showdoc 文档：title、content 必选）
	form := url.Values{}
	form.Set("title", title)
	form.Set("content", content)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pushURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		// 回退 GET
		return pushShowdocGET(ctx, client, pushURL, title, content)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err := parseShowdocResponse(body); err != nil {
		if resp.StatusCode >= 400 {
			return pushShowdocGET(ctx, client, pushURL, title, content)
		}
		return err
	}
	return nil
}

func pushShowdocGET(ctx context.Context, client *http.Client, pushURL, title, content string) error {
	u, err := url.Parse(pushURL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("title", title)
	q.Set("content", content)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("推送失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return parseShowdocResponse(body)
}

func parseShowdocResponse(body []byte) error {
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) == 0 {
		return nil
	}
	var r showdocPushResult
	if err := json.Unmarshal(body, &r); err != nil {
		return nil
	}
	if r.ErrorCode != 0 {
		msg := strings.TrimSpace(r.ErrorMessage)
		if msg == "" {
			msg = "推送被拒绝"
		}
		return fmt.Errorf("%s (error_code=%d)", msg, r.ErrorCode)
	}
	return nil
}

func syncAlertShowdocURL(ctx context.Context, pushURL string) {
	pushURL = strings.TrimSpace(pushURL)
	_ = setSystemMeta(ctx, "showdoc_push_url", pushURL)
	alertShowdocURL = pushURL
}

func loadAlertShowdocFromPluginDB(ctx context.Context) {
	if pluginDB == nil {
		return
	}
	if u, err := getSystemMeta(ctx, "showdoc_push_url"); err == nil && strings.TrimSpace(u) != "" {
		alertShowdocURL = strings.TrimSpace(u)
		return
	}
	var u sql.NullString
	q := fmt.Sprintf(`SELECT showdoc_url FROM %s WHERE showdoc_url IS NOT NULL AND showdoc_url != '' ORDER BY id ASC LIMIT 1`, pluginTable("admin_user"))
	if err := pluginDB.QueryRowContext(ctx, q).Scan(&u); err == nil && u.Valid {
		alertShowdocURL = strings.TrimSpace(u.String)
	}
}

func ensureSystemMetaSchema(ctx context.Context) {
	if pluginDB == nil {
		return
	}
	q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		meta_key varchar(64) NOT NULL,
		meta_value text NOT NULL,
		updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (meta_key)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`, pluginTable("system_meta"))
	_, _ = pluginDB.ExecContext(ctx, q)
}

func setSystemMeta(ctx context.Context, key, value string) error {
	if pluginDB == nil {
		return fmt.Errorf("插件数据库未连接")
	}
	ensureSystemMetaSchema(ctx)
	q := fmt.Sprintf(`INSERT INTO %s (meta_key, meta_value) VALUES (?,?)
		ON DUPLICATE KEY UPDATE meta_value=VALUES(meta_value)`, pluginTable("system_meta"))
	_, err := pluginDB.ExecContext(ctx, q, key, value)
	return err
}

func getSystemMeta(ctx context.Context, key string) (string, error) {
	if pluginDB == nil {
		return "", fmt.Errorf("插件数据库未连接")
	}
	ensureSystemMetaSchema(ctx)
	var val string
	q := fmt.Sprintf(`SELECT meta_value FROM %s WHERE meta_key=? LIMIT 1`, pluginTable("system_meta"))
	err := pluginDB.QueryRowContext(ctx, q, key).Scan(&val)
	return val, err
}
