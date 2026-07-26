package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// ErrHTTPTimeout 标记 HTTP 客户端超时（供队列层识别，不携带上游细节）
var ErrHTTPTimeout = errors.New("E_HTTP_TIMEOUT")


// Order 订单信息
type Order struct {
	OID          string `db:"oid"`
	HID          string `db:"hid"`
	User         string `db:"user"`
	Pass         string `db:"pass"`
	KCName       string `db:"kcname"`
	Status       string `db:"status"`
	Process      string `db:"process"`
	Remarks      string `db:"remarks"`
	DockStatus   string `db:"dockstatus"`
	YID          string `db:"yid"`
	School       string `db:"school"`
	Noun         string `db:"noun"`
	KCID         string `db:"kcid"`
	Name         string `db:"name"`   // 可选：学生姓名等（如 goStudy 的 studentName）
	IsCk         string `db:"isck"`   // 可选："0" 表示不按课程名校验（8090 / jxjyyjy 等）
	UTime        string `db:"uTime"`          // 学习时长（可选字段）
	UScore       string `db:"uScore"`         // 学习分数（可选字段）
	StudySpeed   string `db:"study_speed"`    // 学习速度（可选字段）
	IsSubmitExam string `db:"is_submit_exam"` // 是否提交考试（可选字段）
	ExamTime     string `db:"exam_time"`      // 考试时间（可选字段）
	// simple 平台扩展字段（可选，数据库无列时为空）
	SimpleDayScore   string `db:"simple_day_score"`
	SimpleTotalScore string `db:"simple_total_score"`
	SimpleLearnTime  string `db:"simple_learn_time"`
	IkunStudyIP      string `db:"ikun_study_ip"`
	Expand           string `db:"expand"` // JSON：longlong 的 city/tag/remark/config
}

// ProcessCxResult 处理结果
type ProcessCxResult struct {
	Code       int    `json:"code"`
	Msg        string `json:"msg"`
	YID        string `json:"yid"`
	KCName     string `json:"kcname"`
	User       string `json:"user"`
	Pass       string `json:"pass"`
	KSKS       string `json:"ksks"`
	KSJS       string `json:"ksjs"`
	StatusText string `json:"status_text"`
	Process    string `json:"process"`
	Remarks    string `json:"remarks"`
	Kcks       string `json:"kcks"`
	Kcjs       string `json:"kcjs"`
}

// PlatformPlugin 平台插件接口
type PlatformPlugin interface {
	// GetType 返回平台类型标识（如 "27", "29", "benz" 等）
	GetType() string
	// ProcessOrder 处理订单同步（查询订单状态），返回订单状态信息
	// httpClient: 系统的 HTTP 客户端，用于复用超时等配置
	ProcessOrder(ctx context.Context, order *Order, huoyuan *Huoyuan, httpClient *http.Client) ([]*ProcessCxResult, error)
	// AddOrder 添加订单（下单接口，对应 PHP 的 addWk），返回下单结果
	// httpClient: 系统的 HTTP 客户端，用于复用超时等配置
	AddOrder(ctx context.Context, order *Order, huoyuan *Huoyuan, httpClient *http.Client) (*AddOrderResult, error)
}

// AddOrderResult 下单结果（对应 PHP 的 addWk 返回格式）
type AddOrderResult struct {
	Code int    `json:"code"` // 1=成功, -1=失败
	Msg  string `json:"msg"`  // 消息
	YID  string `json:"yid"`  // 订单ID（如果平台返回）
	// UpstreamBody 上游响应片段（脱敏、截断），仅引擎内部用于规则自动修正
	UpstreamBody string `json:"-"`
}

// Huoyuan 货源信息（对应 PHP 中的 $a）
type Huoyuan struct {
	HID    string
	Type   string // 平台类型（pt）
	URL    string
	User   string
	Pass   string
	Token  string
	Cookie string
}

// 插件注册表（由 registerSubmitRulesFromDB 填充）
var (
	platformPluginsMu sync.RWMutex // 保护 platformPlugins 的并发访问
	platformPlugins   = make(map[string]PlatformPlugin)
)

// 通用 HTTP 请求工具（替代 PHP 的 get_url）
func httpGet(ctx context.Context, httpClient *http.Client, requestURL string, postData map[string]string, headers []string) (string, error) {
	var method string
	var bodyReader io.Reader

	if len(postData) > 0 {
		// POST 请求
		method = "POST"
		formData := url.Values{}
		for k, v := range postData {
			formData.Set(k, v)
		}
		bodyReader = strings.NewReader(formData.Encode())
	} else {
		// GET 请求
		method = "GET"
	}

	return httpRequestCommon(ctx, httpClient, method, requestURL, bodyReader, headers, false)
}

// httpRequestPlatform 通用 HTTP 请求（替代 PHP 的 httpRequest 函数，用于平台插件）
// data 可以是 map[string]interface{} 或 []interface{}（数组格式）
func httpRequestPlatform(ctx context.Context, httpClient *http.Client, method, requestURL string, data interface{}, headers []string, isJSON bool) (string, error) {
	var bodyReader io.Reader

	if data != nil {
		if isJSON {
			jsonData, err := json.Marshal(data)
			if err != nil {
				return "", fmt.Errorf("JSON 序列化失败: %w", err)
			}
			bodyReader = strings.NewReader(string(jsonData))
		} else {
			// 非 JSON 格式，只支持 map 类型
			dataMap, ok := data.(map[string]interface{})
			if !ok {
				return "", fmt.Errorf("非 JSON 格式时，data 必须是 map[string]interface{} 类型")
			}
			formData := url.Values{}
			for k, v := range dataMap {
				formData.Set(k, fmt.Sprintf("%v", v))
			}
			bodyReader = strings.NewReader(formData.Encode())
		}
	}

	return httpRequestCommon(ctx, httpClient, method, requestURL, bodyReader, headers, isJSON)
}

// httpRequestCommon 公共 HTTP 请求处理逻辑
func httpRequestCommon(ctx context.Context, httpClient *http.Client, method, requestURL string, bodyReader io.Reader, headers []string, isJSON bool) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))

	var bodyBytes []byte
	if bodyReader != nil {
		var err error
		bodyBytes, err = io.ReadAll(bodyReader)
		if err != nil {
			return "", fmt.Errorf("E_HTTP_READ_BODY")
		}
	}

	client := cloneHTTPClientNoAutoRedirect(httpClient)
	const maxRedirects = 10

	for attempt := 0; attempt <= maxRedirects; attempt++ {
		reqURL := normalizeOutboundRequestURL(requestURL)
		if err := ValidateOutboundHTTPURL(ctx, reqURL); err != nil {
			return "", err
		}

		var reqBody io.Reader
		if len(bodyBytes) > 0 {
			reqBody = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
		if err != nil {
			log.Printf("E_HTTP_BAD_REQUEST method=%q url=%q err=%v", method, RedactSecrets(reqURL), err)
			return "", fmt.Errorf("E_HTTP_BAD_REQUEST")
		}
		if len(bodyBytes) > 0 {
			req.ContentLength = int64(len(bodyBytes))
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(bodyBytes)), nil
			}
		}

		if len(bodyBytes) > 0 {
			if isJSON {
				req.Header.Set("Content-Type", "application/json")
			} else {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
		}
		for _, h := range headers {
			parts := strings.SplitN(h, ":", 2)
			if len(parts) == 2 {
				req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			}
		}
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/67.0.3396.62 Safari/537.36")
		}

		resp, err := client.Do(req)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return "", fmt.Errorf("%w", ErrHTTPTimeout)
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return "", fmt.Errorf("%w", ErrHTTPTimeout)
			}
			return "", fmt.Errorf("E_HTTP_TRANSPORT")
		}

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := strings.TrimSpace(resp.Header.Get("Location"))
			_ = resp.Body.Close()
			if loc == "" || attempt >= maxRedirects {
				return "", fmt.Errorf("E_HTTP_STATUS_%d", resp.StatusCode)
			}
			next, err := resolveRedirectURL(reqURL, loc)
			if err != nil {
				return "", fmt.Errorf("E_HTTP_STATUS_%d", resp.StatusCode)
			}
			requestURL = next
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return "", fmt.Errorf("E_HTTP_STATUS_%d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("E_HTTP_READ_BODY")
		}
		return string(body), nil
	}
	return "", fmt.Errorf("E_HTTP_REDIRECT")
}

func cloneHTTPClientNoAutoRedirect(c *http.Client) *http.Client {
	if c == nil {
		c = http.DefaultClient
	}
	clone := *c
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &clone
}

func resolveRedirectURL(base, location string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(location)
	if err != nil {
		return "", err
	}
	return baseURL.ResolveReference(ref).String(), nil
}
