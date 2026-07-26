package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func pluginHTTPClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	if httpClient != nil {
		return httpClient
	}
	return NewOutboundHTTPClient(60 * time.Second)
}

// normalizeHuoyuanURL 规范化货源根地址：去首尾空格并去掉末尾 /（与 PHP rtrim($url,'/') 一致）
func normalizeHuoyuanURL(raw string) string {
	return strings.TrimSuffix(strings.TrimSpace(raw), "/")
}

// normalizeOutboundRequestURL 出站请求前规范化 URL（与 ValidateOutboundHTTPURL 一致地补 http://）。
// Go 1.20+ 对 host:port/path 无 scheme 的 URL，NewRequest 会报 first path segment cannot contain colon。
func normalizeOutboundRequestURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "\r", "")
	raw = strings.ReplaceAll(raw, "\n", "")
	if raw == "" {
		return raw
	}
	lower := strings.ToLower(raw)
	for strings.HasPrefix(lower, "http://http://") || strings.HasPrefix(lower, "https://https://") {
		if strings.HasPrefix(lower, "http://http://") {
			raw = raw[7:]
		} else {
			raw = raw[8:]
		}
		lower = strings.ToLower(raw)
	}
	if !strings.Contains(lower, "://") {
		raw = "http://" + raw
	}
	return raw
}

// validateHuoyuanURLConfigured 检查货源根地址是否可用。
// 允许只填域名或 host:port（不含 http/https），与 PHP http://{$url} 一致，出站请求时会自动补 http://。
func validateHuoyuanURLConfigured(raw string) error {
	raw = normalizeHuoyuanURL(raw)
	if raw == "" {
		return fmt.Errorf("货源地址为空，请在货源配置中填写地址")
	}
	check := raw
	if !strings.Contains(strings.ToLower(check), "://") {
		check = "http://" + check
	}
	u, err := url.Parse(check)
	if err != nil || u.Host == "" {
		return fmt.Errorf("货源地址格式错误")
	}
	return nil
}

func jsonMapFromString(body string) (map[string]interface{}, error) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func flexInt(v interface{}, def int) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case string:
		if x == "" {
			return def
		}
		if i, err := strconv.Atoi(x); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return int(f)
		}
		return def
	case bool:
		if x {
			return 1
		}
		return 0
	default:
		return def
	}
}

func flexString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(x)
	}
}

func mapGetString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	return flexString(m[key])
}

func firstNonEmpty(parts ...string) string {
	for _, p := range parts {
		if p != "" {
			return p
		}
	}
	return ""
}

func getNestedValueAny(cur interface{}, path string) interface{} {
	for _, p := range strings.Split(path, ".") {
		if p == "" {
			continue
		}
		switch node := cur.(type) {
		case map[string]interface{}:
			cur = node[p]
		case []interface{}:
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil
			}
			cur = node[idx]
		default:
			return nil
		}
	}
	return cur
}

func jsonPathString(rawJSON, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("json 路径为空")
	}
	rawJSON = strings.TrimSpace(rawJSON)
	if rawJSON == "" {
		return "", fmt.Errorf("JSON 为空")
	}
	var anyRoot interface{}
	if err := json.Unmarshal([]byte(rawJSON), &anyRoot); err != nil {
		return "", fmt.Errorf("JSON 无效")
	}
	return flexString(getNestedValueAny(anyRoot, path)), nil
}

func responseMatches(body string, resp SubmitRuleResp) bool {
	if len(resp.SuccessCodes) == 0 {
		return false
	}
	m, err := jsonMapFromString(body)
	if err != nil {
		return false
	}
	codeField := resp.CodeField
	if codeField == "" {
		codeField = "code"
	}
	codeVal := m[codeField]
	for _, sc := range resp.SuccessCodes {
		if codeMatches(codeVal, sc) {
			return true
		}
	}
	return false
}

func responseMsg(body string, resp SubmitRuleResp) string {
	m, err := jsonMapFromString(body)
	if err != nil {
		return "响应解析失败"
	}
	msgField := resp.MsgField
	if msgField == "" {
		msgField = "msg"
	}
	return firstNonEmpty(mapGetString(m, msgField), "请求失败")
}
