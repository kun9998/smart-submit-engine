package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HTTPSecurity 出站 HTTP 策略（config.yaml http_security）
type HTTPSecurity struct {
	HostWhitelist          []string `yaml:"host_whitelist,omitempty"` // 非空时：仅允许这些域名及其子域（条目写主域，如 example.com）
	BlockPrivateNetworks   *bool    `yaml:"block_private_networks,omitempty"`
	AllowInsecureHTTPToLAN bool     `yaml:"allow_insecure_http_to_lan,omitempty"` // 仅开发：true 且为 http 时跳过「解析到私网」拦截（仍受 host_whitelist 约束）
}

var (
	httpSecMu          sync.RWMutex
	httpHostWhitelist  []string // 小写、已去空格
	httpBlockPrivate   = true
	httpAllowHTTPToLAN = false
)

// productionHTTPSecurityLocked 系统已安装后视为生产环境，禁止 HTTP 内网绕过。
func productionHTTPSecurityLocked() bool {
	if !pluginDBReady() {
		return false
	}
	return isPluginInstalled(context.Background())
}

func initHTTPSecurityFromConfig(fc *fileConfig) {
	httpSecMu.Lock()
	defer httpSecMu.Unlock()
	httpHostWhitelist = nil
	httpBlockPrivate = true
	httpAllowHTTPToLAN = false
	if fc == nil {
		return
	}
	hs := fc.HTTPSecurity
	if hs.BlockPrivateNetworks != nil {
		httpBlockPrivate = *hs.BlockPrivateNetworks
	}
	httpAllowHTTPToLAN = hs.AllowInsecureHTTPToLAN
	if productionHTTPSecurityLocked() {
		httpAllowHTTPToLAN = false
	}
	for _, h := range hs.HostWhitelist {
		h = strings.ToLower(strings.TrimSpace(h))
		h = strings.TrimPrefix(h, ".")
		if h != "" {
			httpHostWhitelist = append(httpHostWhitelist, h)
		}
	}
}

func hostMatchesHTTPWhitelist(host string) bool {
	httpSecMu.RLock()
	defer httpSecMu.RUnlock()
	if len(httpHostWhitelist) == 0 {
		return true
	}
	h := strings.ToLower(strings.TrimSpace(host))
	for _, p := range httpHostWhitelist {
		if h == p || strings.HasSuffix(h, "."+p) {
			return true
		}
	}
	return false
}

func isBlockedOutboundIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 127 || (ip4[0] == 0 && ip4[1] == 0 && ip4[2] == 0 && ip4[3] == 0) {
			return true
		}
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
	}
	if ip.IsMulticast() {
		return true
	}
	return false
}

// 出站前 DNS 校验：首包/冷解析偶发超过 2s 会误判为 E_DNS；略放宽 + 重试 + 成功结果短缓存
const (
	outboundDNSLookupTimeout = 5 * time.Second
	outboundDNSMaxAttempts   = 3
	outboundDNSRetryDelay    = 200 * time.Millisecond
	outboundDNSPositiveTTL   = 5 * time.Minute
)

type dnsPositiveEntry struct {
	until time.Time
	ips   []net.IP // 拷贝，避免外部修改
}

var (
	dnsPositiveMu sync.RWMutex
	dnsPositive   = make(map[string]dnsPositiveEntry) // host 小写 -> 已通过「非公网拦截」的解析结果
)

func dnsPositiveCacheGet(host string) ([]net.IP, bool) {
	h := strings.ToLower(strings.TrimSpace(host))
	dnsPositiveMu.RLock()
	e, ok := dnsPositive[h]
	dnsPositiveMu.RUnlock()
	if !ok || time.Now().After(e.until) {
		return nil, false
	}
	out := make([]net.IP, len(e.ips))
	for i, ip := range e.ips {
		out[i] = append(net.IP(nil), ip...)
	}
	return out, true
}

func dnsPositiveCacheSet(host string, addrs []net.IPAddr) {
	h := strings.ToLower(strings.TrimSpace(host))
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		if a.IP != nil {
			ips = append(ips, append(net.IP(nil), a.IP...))
		}
	}
	dnsPositiveMu.Lock()
	dnsPositive[h] = dnsPositiveEntry{until: time.Now().Add(outboundDNSPositiveTTL), ips: ips}
	dnsPositiveMu.Unlock()
}

func lookupHostAddrsWithRetry(ctx context.Context, host string) ([]net.IPAddr, error) {
	var lastErr error
	for attempt := 0; attempt < outboundDNSMaxAttempts; attempt++ {
		if attempt > 0 {
			t := time.NewTimer(outboundDNSRetryDelay)
			select {
			case <-ctx.Done():
				t.Stop()
				return nil, ctx.Err()
			case <-t.C:
			}
		}
		perTry := outboundDNSLookupTimeout
		if d, ok := ctx.Deadline(); ok {
			if rem := time.Until(d); rem > 0 && rem < perTry {
				perTry = rem
			}
		}
		if perTry <= 0 {
			lastErr = context.DeadlineExceeded
			break
		}
		lookupCtx, cancel := context.WithTimeout(ctx, perTry)
		addrs, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
		cancel()
		if err == nil && len(addrs) > 0 {
			return addrs, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = errors.New("empty DNS answer")
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("E_DNS")
	}
	return nil, lastErr
}

// ValidateOutboundHTTPURL 校验出站请求 URL：白名单 +（可选）私网拦截
func ValidateOutboundHTTPURL(ctx context.Context, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("E_URL_INVALID")
	}
	// 仅填 host:port 或 IP:port（无 scheme）时，Go 的 url.Parse 会得到空 Host，误判为 E_URL_INVALID
	if !strings.Contains(strings.ToLower(raw), "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Scheme == "" {
		return fmt.Errorf("E_URL_INVALID")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("E_URL_SCHEME")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("E_URL_HOST")
	}
	if !hostMatchesHTTPWhitelist(host) {
		return fmt.Errorf("E_HOST_NOT_WHITELIST")
	}
	httpSecMu.RLock()
	block := httpBlockPrivate
	allowLANHTTP := httpAllowHTTPToLAN
	httpSecMu.RUnlock()
	if !block {
		return nil
	}
	if allowLANHTTP && scheme == "http" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedOutboundIP(ip) {
			return fmt.Errorf("E_BLOCKED_ADDRESS")
		}
		return nil
	}
	var addrs []net.IPAddr
	if cached, ok := dnsPositiveCacheGet(host); ok {
		addrs = make([]net.IPAddr, len(cached))
		for i, ip := range cached {
			addrs[i] = net.IPAddr{IP: ip}
		}
	} else {
		var err error
		addrs, err = lookupHostAddrsWithRetry(ctx, host)
		if err != nil {
			return fmt.Errorf("E_DNS")
		}
	}
	// 改为「至少一个公网可用 IP 即放行」：
	// 避免混合解析（同时返回公网和私网）被一票否决导致误伤。
	publicAddrs := make([]net.IPAddr, 0, len(addrs))
	for _, a := range addrs {
		if a.IP == nil {
			continue
		}
		if !isBlockedOutboundIP(a.IP) {
			publicAddrs = append(publicAddrs, a)
		}
	}
	if len(publicAddrs) == 0 {
		return fmt.Errorf("E_BLOCKED_ADDRESS")
	}
	// 仅缓存通过当前策略的公网解析结果，减少重复解析与抖动影响。
	dnsPositiveCacheSet(host, publicAddrs)
	return nil
}

type guardingRoundTripper struct {
	base http.RoundTripper
}

func (g *guardingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return nil, fmt.Errorf("E_URL_MISSING")
	}
	if err := ValidateOutboundHTTPURL(req.Context(), req.URL.String()); err != nil {
		return nil, err
	}
	base := g.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func newSecureTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		if t, ok := http.DefaultTransport.(*http.Transport); ok {
			base = t.Clone()
		} else {
			base = http.DefaultTransport
		}
	}
	return &guardingRoundTripper{base: base}
}

// NewOutboundHTTPClient 全局出站 HTTP 客户端（带白名单与私网拦截）
func NewOutboundHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: newSecureTransport(nil),
	}
}
