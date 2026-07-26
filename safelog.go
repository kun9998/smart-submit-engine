package main

import (
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
)

// RedactSecrets 对单行或多行文本做敏感信息脱敏（Authorization/Cookie/Token/password 等）
func RedactSecrets(s string) string {
	if s == "" {
		return s
	}
	// HTTP 头样式（禁止依赖完整头日志；若其它代码误打头，此处兜底）
	s = reAuthHeader.ReplaceAllString(s, "$1: [REDACTED]")
	s = reCookieHeader.ReplaceAllString(s, "$1: [REDACTED]")
	s = reTokenLikeHeader.ReplaceAllString(s, "$1: [REDACTED]")
	// URL 查询串常见敏感参数
	s = reQueryPass.ReplaceAllString(s, "$1$2=[REDACTED]")
	s = reQueryToken.ReplaceAllString(s, "$1$2=[REDACTED]")
	// JSON 常见字段
	s = reJSONSecret.ReplaceAllString(s, `$1"[REDACTED]"`)
	return s
}

var (
	reAuthHeader       = regexp.MustCompile(`(?i)\b(Authorization|Proxy-Authorization)\s*:\s*[^\n]+`)
	reCookieHeader     = regexp.MustCompile(`(?i)\b(Cookie|Set-Cookie)\s*:\s*[^\n]+`)
	reTokenLikeHeader  = regexp.MustCompile(`(?i)\b(X-Api-Key|X-Auth-Token|Api-Key|Token)\s*:\s*[^\n]+`)
	reQueryPass        = regexp.MustCompile(`(?i)([?&])(password|pass|pwd)=([^&\s#]+)`)
	reQueryToken       = regexp.MustCompile(`(?i)([?&])(token|access_token|refresh_token|key|secret)=([^&\s#]+)`)
	reJSONSecret       = regexp.MustCompile(`(?i)("(?:password|passwd|token|access_token|refresh_token|secret|cookie|authorization)"\s*:\s*)("[^"]*")`)
)

var globalRedactLogOnce sync.Once

// installGlobalRedactingLogOutput 将标准库 log 输出经脱敏后写入 stderr（强制）
func installGlobalRedactingLogOutput() {
	globalRedactLogOnce.Do(func() {
		log.SetOutput(&redactLogWriter{w: os.Stderr})
	})
}

type redactLogWriter struct {
	w io.Writer
}

func (rw *redactLogWriter) Write(p []byte) (n int, err error) {
	msg := RedactSecrets(string(p))
	trimmed := strings.TrimSpace(msg)
	if trimmed == "" {
		return len(p), nil
	}
	// 订单提交相关：仅管理端「提交日志」页展示，不输出到控制台
	if isSubmitStatusLog(trimmed) {
		appLogHub.appendLine(trimmed)
		return len(p), nil
	}
	// 引擎日常运行日志：静默（不进提交日志页，也不 printf）
	if isEngineRoutineLog(trimmed) {
		return len(p), nil
	}
	// 程序级日志：仅控制台
	_, err = rw.w.Write([]byte(msg))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// SanitizeUserVisibleError 对外/入库可见的简短说明（不含响应体、不含原始异常链）
func SanitizeUserVisibleError(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "E_UNKNOWN"
	}
	s := simplifyErrorMsg(msg)
	s = RedactSecrets(s)
	if len(s) > 120 {
		return s[:120] + "…"
	}
	if s == "" {
		return "E_UNKNOWN"
	}
	return s
}
