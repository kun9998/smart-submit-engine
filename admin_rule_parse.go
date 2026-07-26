package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type xdjkParseResult struct {
	PlatformType    string
	RuleConfig      SubmitRuleConfig
	Warnings        []string
	SpecialNotes    []string
	Error           string
	TrustedTemplate bool // 内置完整模板，可跳过嵌套 body / 特殊逻辑拦截
}

var (
	xdjkTypeRE          = regexp.MustCompile(`(?i)\$type\s*==\s*["']([^"']+)["']`)
	xdjkDataArrayRE     = regexp.MustCompile(`(?is)\$data\s*=\s*array\s*\(([\s\S]*?)\)\s*;`)
	xdjkDataShortRE     = regexp.MustCompile(`(?is)\$data\s*=\s*\[([\s\S]*?)\]\s*;`)
	xdjkPostDataArrayRE = regexp.MustCompile(`(?is)\$postData\s*=\s*array\s*\(([\s\S]*?)\)\s*;`)
	xdjkPostDataShortRE = regexp.MustCompile(`(?is)\$postData\s*=\s*\[([\s\S]*?)\]\s*;`)
	xdjkBodyPairRE      = regexp.MustCompile(`(?s)["']([^"']+)["']\s*=>\s*([^,]+)`)
	xdjkHuoyuanVarRE    = regexp.MustCompile(`(?i)^\$a\[["'](\w+)["']\]$`)
	xdjkOrderDVarRE     = regexp.MustCompile(`(?i)^\$d\[["'](\w+)["']\]$`)
	xdjkUrlAssignRE     = regexp.MustCompile(`(?i)\$(\w+)\s*=\s*\$a\[["']url["']\]\s*;`)
	xdjkUrlConcatRE     = regexp.MustCompile(`(?i)\$(\w+)\s*=\s*\$a\[["']url["']\]\s*\.\s*["']([^"']+)["']`)
	xdjkUrlHttpConcatRE = regexp.MustCompile(`(?i)\$(\w+)\s*=\s*["']https?://["']\s*\.\s*\$a\[["']url["']\]\s*\.\s*["']([^"']+)["']`)
	xdjkHuoyuanURLDoubleSchemeRE = regexp.MustCompile(`(?i)https?://(\{\{huoyuan\.url\}\})`)
	xdjkStrAssignRE     = regexp.MustCompile(`(?i)\$(\w+)\s*=\s*["']([^"']+)["']\s*;`)
	xdjkGetUrlRE        = regexp.MustCompile(`(?i)get_url\s*\(\s*\$(\w+)(?:\s*,\s*\$(\w+))?(?:\s*,\s*\$(\w+))?\s*\)`)
	xdjkHttpGetRE       = regexp.MustCompile(`(?i)httpRequest\s*\(\s*["']GET["']\s*,\s*\$(\w+)`)
	xdjkHttpPostRE      = regexp.MustCompile(`(?i)httpRequest\s*\(\s*["'](\w+)["']\s*,\s*\$(\w+)\s*,\s*\$(\w+)\s*,\s*\[[^\]]*\]\s*,\s*(true|false)\s*\)`)
	xdjkConcatGetRE     = regexp.MustCompile(`(?i)\$(\w+)\s*=\s*\$(\w+)\s*\.\s*["']([^'"]*\?[^'"]*)["']\s*\.([^;]+);`)
	xdjkFailureMsgRE    = regexp.MustCompile(`(?is)strpos\s*\(\s*\$result\[["'](\w+)["']\]\s*,\s*["']([^"']+)["']\s*\)\s*!==\s*false[\s\S]*?\$msg\s*=\s*["']([^"']+)["']`)
	xdjkMsgSuccessRE    = regexp.MustCompile(`(?i)if\s*\(\s*\$result\[["']msg["']\]\s*==\s*["']([^"']+)["']\s*\)`)
	xdjkSuccessQuotedRE = regexp.MustCompile(`(?i)if\s*\(\s*\$result\[["'](\w+)["']\]\s*==\s*["']([^"']+)["']\s*\)`)
	xdjkSuccessNumRE    = regexp.MustCompile(`(?i)if\s*\(\s*\$result\[["'](\w+)["']\]\s*==\s*(\d+)\s*\)`)
	xdjkYidNestedRE     = regexp.MustCompile(`(?i)["']yid["']\s*=>\s*\$result\[["'](\w+)["']\]\[["'](\w+)["']\]`)
	xdjkYidFlatRE       = regexp.MustCompile(`(?i)["']yid["']\s*=>\s*\$result\[["'](\w+)["']\]`)
	xdjkYidTokenRE      = regexp.MustCompile(`(?i)["']yid["']\s*=>\s*\$result\[["']order_token["']\]`)
	xdjkYidData0RE      = regexp.MustCompile(`(?i)["']yid["']\s*=>\s*\$result\[["']data["']\]\[0\]`)
	xdjkReturnMsgRE         = regexp.MustCompile(`(?i)["']msg["']\s*=>\s*["']([^"']+)["']`)
	xdjkNestedArrayRE       = regexp.MustCompile(`(?i)^array\s*\(`)
	xdjkConcatUrlencodeRE   = regexp.MustCompile(`(?i)\s*\.\s*urlencode\s*\(\s*\$(\w+)\s*\)`)
	xdjkConcatAmpRE         = regexp.MustCompile(`\s*\.\s*['"]&([^'"]*)['"]`)
	xdjkArrayRandRE         = regexp.MustCompile(`(?i)array_rand|randomPort`)
	xdjkQueryParamRE        = regexp.MustCompile(`(?i)username=|platform=|zhanghao=`)
	xdjkGetUrlBodyRE        = regexp.MustCompile(`(?i)get_url\s*\(\s*\$(\w+)\s*,\s*\$(data|jsonData)\b`)
	xdjkAuthBearerRE        = regexp.MustCompile(`(?i)["']Authorization:\s*Bearer\s*["']\s*\.\s*\$(\w+)`)
	xdjkAuthDfAiRE          = regexp.MustCompile(`(?i)["']Authorization:\s*DfAi\s*\$(\w+)`)
	xdjkTokenHdrRE          = regexp.MustCompile(`(?i)["']Token:\s*["']\s*\.\s*\$a\[["']token["']\]`)
	xdjkTokenPlainRE        = regexp.MustCompile(`(?i)["']token:\s*["']\s*\.\s*\$a\[["']token["']\]`)
	xdjkCookieGetRE         = regexp.MustCompile(`(?i)get_url\s*\([^)]+,\s*\$(\w+)\s*,\s*\$cookie\s*\)`)
	xdjkCookieGetShortRE    = regexp.MustCompile(`(?i)get_url\s*\(\s*\$(\w+)\s*,\s*\$cookie\s*\)`)
	xdjkCookieHdrRE         = regexp.MustCompile(`(?i)["']cookie:\s*\{\$cookie\}`)
	xdjkDataJsonEncodeRE    = regexp.MustCompile(`(?i)\$data\s*=\s*json_encode`)
	xdjkDataBracketRE       = regexp.MustCompile(`(?i)\$data\s*=\s*\[`)
	xdjkJsonEncodeDataRE    = regexp.MustCompile(`(?i)json_encode\s*\(\s*\$data`)
	xdjkContentTypeJSONRE   = regexp.MustCompile(`(?i)Content-Type:\s*application/json`)
	xdjkHttpRequestTrueRE   = regexp.MustCompile(`(?i)httpRequest\s*\([^)]+,\s*true\s*\)`)
	xdjkPostJsonRE          = regexp.MustCompile(`(?i)\bpost\s*\(\s*\$(\w+)\s*,\s*\$(jsonData|data)\s*,`)
	xdjkPostDataRE          = regexp.MustCompile(`(?i)\bpost\s*\(\s*\$(\w+)\s*,\s*\$data\s*\)`)
	xdjkResultMessageRE     = regexp.MustCompile(`(?i)result\[["']message["']\]`)
	xdjkResultMsgRE         = regexp.MustCompile(`(?i)result\[["']msg["']\]`)
	xdjkLLv2SubmitRE        = regexp.MustCompile(`(?i)llv2_submit\s*\(`)
	xdjkLongLongV2FileRE    = regexp.MustCompile(`(?i)LongLongV2\.php`)
	xdjkGetUrlOnlyRE        = regexp.MustCompile(`(?i)get_url\s*\(\s*\$(\w+)\s*(?:,\s*\$cookie)?\s*\)`)
	xdjkJsonEncodeArrayRE   = regexp.MustCompile(`(?i)json_encode\s*\(\s*\[`)
	xdjkDataNestedArrayRE   = regexp.MustCompile(`(?i)\$data\s*=\s*array\s*\(\s*array\s*\(`)
	xdjkRequestDataRE       = regexp.MustCompile(`(?i)\$requestData\s*=\s*array`)
	xdjkPostDataAssignRE    = regexp.MustCompile(`(?i)\$postData\s*=\s*\[`)
	xdjkJsonEncodeRequestRE = regexp.MustCompile(`(?i)json_encode\s*\(\s*\$requestData`)
	xdjkJsonEncodePostRE    = regexp.MustCompile(`(?i)json_encode\s*\(\s*\$postData`)
	xdjkNestedFieldArrayRE  = regexp.MustCompile(`(?i)["']\w+["']\s*=>\s*array\s*\(`)
	xdjkSpecialNoteBlockRE  = regexp.MustCompile(`(?i)课代表|随机端口|写主库|sleep|按课程名|龙猫|simple|哆啦|固定第三方|form 字符串`)
	xdjkTplHuoyuanBraceRE   = regexp.MustCompile(`(?i)\{\$a\[['"](\w+)['"]\]\}`)
	xdjkTplHuoyuanRE        = regexp.MustCompile(`(?i)\$a\[['"](\w+)['"]\]`)
	xdjkTplUrlencodeRE      = regexp.MustCompile(`(?i)urlencode\s*\(\s*\$(\w+)\s*\)`)
	xdjkTplVarRE            = regexp.MustCompile(`(?i)\$(\w+)`)
	xdjkCurlSetOptURLRE     = regexp.MustCompile(`(?i)curl_setopt\s*\(\s*\$(\w+)\s*,\s*CURLOPT_URL\s*,\s*\$(\w+)\s*\)`)
	xdjkStatusSuccessRE     = regexp.MustCompile(`(?i)\$result\[["']status["']\]\s*==\s*["']success["']`)
)

type xdjkSpecialBlocker struct {
	pattern *regexp.Regexp
	note    string
}

var xdjkSpecialBlockers = []xdjkSpecialBlocker{
	{regexp.MustCompile(`(?i)base64_decode\s*\(\s*\$kcid\s*\)`), "课代表系列 (kdbxxt/kdbzhs/kdbzhzj)：请求体来自 base64 解码 kcid，且随 noun 改 JSON，需单独实现"},
	{regexp.MustCompile(`(?i)array_rand\s*\(`), "KUN：随机端口无法写入静态规则，需固定端口或 Go 侧实现"},
	{regexp.MustCompile(`(?i)\$DB\s*->\s*query`), "lotus 等：含写主库 remarks 的 SQL，超出提交规则范围"},
	{regexp.MustCompile(`(?i)\bsleep\s*\(`), "含 sleep 延时，规则引擎不支持"},
	{regexp.MustCompile(`(?i)if\s*\(\s*strpos\s*\(\s*\$kcname`), "懒洋洋等：按课程名分支不同 URL/Body，需拆成多条规则或 Go 实现"},
	{regexp.MustCompile(`(?i)\$type\s*==\s*["']df1["']\s*\|\|\s*\$type`), "df1/df2：同一分支含 test 参数差异，请分别粘贴 df1 或 df2 块"},
	{regexp.MustCompile(`(?i)\$noun\s*==\s*1\s*\)\s*\{`), "龙猫 (maliaorun)：按 noun 数值分支多套 data，请粘贴具体分支"},
	{regexp.MustCompile(`(?i)\$noun\s*=\s*\$noun\s*\.\s*["']\|`), "simple：会改写 noun 再提交，需手动处理"},
	{regexp.MustCompile(`(?i)if\s*\(\s*\$noun\s*==\s*['"]xgk['"]`), "哆啦A梦 (dlam)：按 noun 计算 operate 字段，需手动配置 body"},
	{regexp.MustCompile(`(?i)http://text\.boox\.top`), "lotus：固定第三方域名，非货源 url"},
	{regexp.MustCompile(`(?i)["']courseInfo["']\s*=>\s*array`), "无名 (wuming) 等：含嵌套 JSON 数组 body，请解析后手动补全"},
	{regexp.MustCompile(`(?i)\$data\s*=\s*["']platform=`), "懒洋洋随机课：body 为 form 字符串而非 array，需手动编写"},
	{regexp.MustCompile(`(?i)ikun_study_ip`), "kunba：已支持 {{order.ikun_study_ip}}，请确认货源 URL 模板"},
}

func detectXdjkSpecialBlockers(code string) []string {
	var notes []string
	for _, b := range xdjkSpecialBlockers {
		if b.pattern.MatchString(code) {
			notes = append(notes, b.note)
		}
	}
	return notes
}

func mapXdjkPhpValue(raw string) (tpl string, ok bool) {
	v := strings.TrimSpace(strings.Join(strings.Fields(raw), " "))
	if m := xdjkHuoyuanVarRE.FindStringSubmatch(v); len(m) > 1 {
		key := m[1]
		if key == "user" || key == "pass" || key == "url" || key == "token" || key == "cookie" {
			return "{{huoyuan." + key + "}}", true
		}
		return "{{huoyuan." + key + "}}", false
	}
	if m := xdjkOrderDVarRE.FindStringSubmatch(v); len(m) > 1 {
		return "{{order." + m[1] + "}}", true
	}
	orderVars := map[string]struct {
		tpl string
		ok  bool
	}{
		"$noun":   {"{{order.noun}}", true},
		"$school": {"{{order.school}}", true},
		"$user":   {"{{order.user}}", true},
		"$pass":   {"{{order.pass}}", true},
		"$kcname": {"{{order.kcname}}", true},
		"$kcid":   {"{{order.kcid}}", true},
		"$uTime":  {"{{order.uTime}}", true},
		"$uScore": {"{{order.uScore}}", true},
		"$oid":    {"{{order.oid}}", true},
		"$token":  {"{{huoyuan.token}}", true},
		"$cookie": {"{{huoyuan.cookie}}", true},
		"$expand": {"[]", false},
		"$operate": {"", false},
		"$course":  {"[]", false},
	}
	if mapped, exists := orderVars[v]; exists {
		return mapped.tpl, mapped.ok
	}
	return v, false
}

func parseXdjkArrayBody(inner string) (map[string]string, []string) {
	body := map[string]string{}
	var warnings []string
	for _, m := range xdjkBodyPairRE.FindAllStringSubmatch(inner, -1) {
		key := m[1]
		raw := strings.TrimSpace(m[2])
		if xdjkNestedArrayRE.MatchString(raw) {
			warnings = append(warnings, "字段 "+key+" 为嵌套 array，已跳过，请手动补全")
			continue
		}
		tpl, ok := mapXdjkPhpValue(raw)
		body[key] = tpl
		if !ok {
			warnings = append(warnings, "字段 "+key+" 的值「"+raw+"」未能识别，请手动检查")
		}
	}
	return body, warnings
}

func findXdjkDataArrayInner(code string) string {
	if arrays := xdjkDataArrayRE.FindAllStringSubmatch(code, -1); len(arrays) > 0 {
		return arrays[len(arrays)-1][1]
	}
	if short := xdjkDataShortRE.FindAllStringSubmatch(code, -1); len(short) > 0 {
		return short[len(short)-1][1]
	}
	if post := xdjkPostDataArrayRE.FindAllStringSubmatch(code, -1); len(post) > 0 {
		return post[len(post)-1][1]
	}
	if postShort := xdjkPostDataShortRE.FindAllStringSubmatch(code, -1); len(postShort) > 0 {
		return postShort[len(postShort)-1][1]
	}
	return ""
}

func xdjkPhpVarToTemplate(expr string) string {
	expr = xdjkTplHuoyuanBraceRE.ReplaceAllStringFunc(expr, func(s string) string {
		m := xdjkTplHuoyuanBraceRE.FindStringSubmatch(s)
		if len(m) > 1 {
			return "{{huoyuan." + m[1] + "}}"
		}
		return s
	})
	expr = xdjkTplHuoyuanRE.ReplaceAllStringFunc(expr, func(s string) string {
		m := xdjkTplHuoyuanRE.FindStringSubmatch(s)
		if len(m) > 1 {
			return "{{huoyuan." + m[1] + "}}"
		}
		return s
	})
	expr = xdjkTplUrlencodeRE.ReplaceAllStringFunc(expr, func(s string) string {
		m := xdjkTplUrlencodeRE.FindStringSubmatch(s)
		if len(m) > 1 {
			tpl, ok := mapXdjkPhpValue("$" + m[1])
			if ok && strings.HasPrefix(tpl, "{{") && strings.HasSuffix(tpl, "}}") {
				return "{{urlencode " + tpl[2:len(tpl)-2] + "}}"
			}
			return tpl
		}
		return s
	})
	expr = xdjkTplVarRE.ReplaceAllStringFunc(expr, func(s string) string {
		tpl, ok := mapXdjkPhpValue(s)
		if ok {
			return tpl
		}
		return s
	})
	return expr
}

// normalizeHuoyuanURLTemplate 修正模板 URL 中的双协议、双斜杠等问题。
func normalizeHuoyuanURLTemplate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "{{huoyuan.url}}"
	}
	s = xdjkHuoyuanURLDoubleSchemeRE.ReplaceAllString(s, "$1")
	for strings.Contains(s, "{{huoyuan.url}}//") {
		s = strings.ReplaceAll(s, "{{huoyuan.url}}//", "{{huoyuan.url}}/")
	}
	return s
}

// joinHuoyuanURLTemplate 拼接 {{huoyuan.url}} 与 PHP 中的路径段。
// tj 渲染时会去掉货源 url 末尾 /，因此 path 无前导 / 时需补上（对应 PHP $a["url"]."api/..." 且 url 带尾斜杠的写法）。
func joinHuoyuanURLTemplate(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "{{huoyuan.url}}"
	}
	if strings.Contains(path, "{{huoyuan.url}}") {
		return normalizeHuoyuanURLTemplate(path)
	}
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "?") && !strings.HasPrefix(path, ":") {
		path = "/" + path
	}
	return "{{huoyuan.url}}" + path
}

func parseXdjkConcatGetURL(code string) (url, method string, warnings []string, ok bool) {
	m := xdjkConcatGetRE.FindStringSubmatch(code)
	if len(m) < 5 {
		return "", "", nil, false
	}
	qs := m[3] + m[4]
	qs = xdjkConcatUrlencodeRE.ReplaceAllStringFunc(qs, func(s string) string {
		sub := xdjkTplVarRE.FindStringSubmatch(s)
		if len(sub) > 1 {
			tpl, ok := mapXdjkPhpValue("$" + sub[1])
			if ok && strings.HasPrefix(tpl, "{{") {
				return "{{urlencode " + tpl[2:len(tpl)-2] + "}}"
			}
		}
		return s
	})
	qs = xdjkConcatAmpRE.ReplaceAllString(qs, "&$1")
	qs = xdjkPhpVarToTemplate(strings.ReplaceAll(qs, " ", ""))
	qs = strings.ReplaceAll(qs, ".", "")
	if xdjkArrayRandRE.MatchString(code) {
		warnings = append(warnings, "含随机端口，URL 中端口需手动指定")
	}
	return joinHuoyuanURLTemplate(qs), "GET", warnings, true
}

func resolveXdjkURL(code string) (url, method string, warnings []string) {
	urlVars := map[string]string{}

	for _, m := range xdjkUrlAssignRE.FindAllStringSubmatch(code, -1) {
		urlVars[m[1]] = "{{huoyuan.url}}"
	}
	for _, m := range xdjkUrlHttpConcatRE.FindAllStringSubmatch(code, -1) {
		if len(m) > 2 {
			urlVars[m[1]] = joinHuoyuanURLTemplate(m[2])
		}
	}
	for _, m := range xdjkUrlConcatRE.FindAllStringSubmatch(code, -1) {
		if len(m) > 2 {
			urlVars[m[1]] = joinHuoyuanURLTemplate(m[2])
		}
	}
	for _, m := range xdjkStrAssignRE.FindAllStringSubmatch(code, -1) {
		varName := m[1]
		path := m[2]
		path = xdjkTplVarRE.ReplaceAllStringFunc(path, func(s string) string {
			sub := xdjkTplVarRE.FindStringSubmatch(s)
			if len(sub) > 1 {
				if v, ok := urlVars[sub[1]]; ok {
					return v
				}
			}
			return "{{huoyuan.url}}"
		})
		path = xdjkPhpVarToTemplate(path)
		if !strings.Contains(path, "{{huoyuan.url}}") && strings.HasPrefix(path, "/") {
			path = joinHuoyuanURLTemplate(path)
		} else if strings.Contains(path, "{{huoyuan.url}}") {
			path = normalizeHuoyuanURLTemplate(path)
		}
		urlVars[varName] = path
	}

	for _, u := range urlVars {
		if strings.Contains(u, "?") && xdjkQueryParamRE.MatchString(u) {
			if xdjkGetUrlRE.MatchString(code) {
				hasBody := xdjkGetUrlBodyRE.MatchString(code)
				if hasBody {
					return u, "POST", warnings
				}
				return u, "GET", warnings
			}
		}
	}

	if u, m, w, ok := parseXdjkConcatGetURL(code); ok {
		return u, m, w
	}

	if m := xdjkGetUrlRE.FindStringSubmatch(code); len(m) > 1 {
		if resolved, ok := urlVars[m[1]]; ok {
			method := "GET"
			if len(m) > 2 && m[2] != "" && m[2] != "cookie" {
				method = "POST"
			}
			return resolved, method, warnings
		}
	}
	if m := xdjkHttpGetRE.FindStringSubmatch(code); len(m) > 1 {
		if resolved, ok := urlVars[m[1]]; ok {
			return resolved, "GET", warnings
		}
	}
	if m := xdjkHttpPostRE.FindStringSubmatch(code); len(m) > 2 {
		if resolved, ok := urlVars[m[2]]; ok {
			return resolved, strings.ToUpper(m[1]), warnings
		}
	}
	if m := xdjkCurlSetOptURLRE.FindStringSubmatch(code); len(m) > 2 {
		if resolved, ok := urlVars[m[2]]; ok {
			return resolved, "POST", warnings
		}
	}
	for name, u := range urlVars {
		if strings.Contains(code, "get_url($"+name) ||
			strings.Contains(code, "httpRequest(") ||
			strings.Contains(code, "post(") ||
			strings.Contains(code, "curl_exec") ||
			strings.Contains(code, "curl_setopt") {
			return u, "POST", warnings
		}
	}

	warnings = append(warnings, "未能自动识别 URL，已使用默认 /api.php?act=add")
	return "{{huoyuan.url}}/api.php?act=add", "POST", warnings
}

func parseXdjkHeaders(code string) map[string]string {
	headers := map[string]string{}
	if xdjkAuthBearerRE.MatchString(code) {
		headers["Authorization"] = "Bearer {{huoyuan.token}}"
	}
	if xdjkAuthDfAiRE.MatchString(code) {
		headers["Authorization"] = "DfAi {{huoyuan.token}}"
	}
	if xdjkTokenHdrRE.MatchString(code) {
		headers["Token"] = "{{huoyuan.token}}"
	}
	if xdjkTokenPlainRE.MatchString(code) {
		headers["token"] = "{{huoyuan.token}}"
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func detectXdjkTransport(code string) (contentType string, useCookie bool) {
	useCookie = xdjkCookieGetRE.MatchString(code) ||
		xdjkCookieGetShortRE.MatchString(code) ||
		xdjkCookieHdrRE.MatchString(code)

	isJSON := xdjkDataJsonEncodeRE.MatchString(code) ||
		xdjkDataBracketRE.MatchString(code) ||
		xdjkJsonEncodeDataRE.MatchString(code) ||
		xdjkContentTypeJSONRE.MatchString(code) ||
		xdjkHttpRequestTrueRE.MatchString(code) ||
		xdjkPostJsonRE.MatchString(code) ||
		xdjkPostDataRE.MatchString(code)

	if isJSON {
		return "json", useCookie
	}
	return "form", useCookie
}

func parseXdjkFailureMsgRules(code string) []FailureMsgRule {
	var rules []FailureMsgRule
	seen := map[string]struct{}{}
	for _, m := range xdjkFailureMsgRE.FindAllStringSubmatch(code, -1) {
		msgField := strings.ToLower(m[1])
		if msgField != "msg" && msgField != "message" {
			continue
		}
		key := m[2] + "\x00" + m[3]
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		rules = append(rules, FailureMsgRule{Contains: m[2], Msg: m[3]})
	}
	return rules
}

func parseXdjkResponse(code string) SubmitRuleResp {
	resp := SubmitRuleResp{
		CodeField:    "code",
		SuccessCodes: []interface{}{"0", 0},
		MsgField:     "msg",
		SuccessMsg:   "下单成功",
	}

	if m := xdjkMsgSuccessRE.FindStringSubmatch(code); len(m) > 1 {
		resp.CodeField = "msg"
		resp.SuccessCodes = []interface{}{m[1]}
		resp.MsgField = "msg"
		return resp
	}
	if xdjkStatusSuccessRE.MatchString(code) {
		resp.CodeField = "status"
		resp.SuccessCodes = []interface{}{"success"}
		resp.MsgField = "msg"
	}
	if m := xdjkSuccessQuotedRE.FindStringSubmatch(code); len(m) > 2 {
		resp.CodeField = m[1]
		resp.SuccessCodes = []interface{}{m[2], m[2]}
	}
	if m := xdjkSuccessNumRE.FindStringSubmatch(code); len(m) > 2 {
		resp.CodeField = m[1]
		resp.SuccessCodes = []interface{}{mustAtoi(m[2]), m[2]}
	}
	if xdjkResultMessageRE.MatchString(code) && !xdjkResultMsgRE.MatchString(code) {
		resp.MsgField = "message"
	}
	if m := xdjkYidNestedRE.FindStringSubmatch(code); len(m) > 2 {
		resp.YIDPath = m[1] + "." + m[2]
	} else if m := xdjkYidFlatRE.FindStringSubmatch(code); len(m) > 1 {
		resp.YIDField = m[1]
	}
	if xdjkYidTokenRE.MatchString(code) {
		resp.YIDField = "order_token"
	}
	if xdjkYidData0RE.MatchString(code) {
		resp.YIDPath = "data.0"
	}
	if m := xdjkReturnMsgRE.FindStringSubmatch(code); len(m) > 1 && !xdjkMsgSuccessRE.MatchString(code) {
		resp.SuccessMsg = m[1]
	}
	if rules := parseXdjkFailureMsgRules(code); len(rules) > 0 {
		resp.FailureMsgRules = rules
	}
	return resp
}

func mustAtoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func parseXdjkLongLongV2(code, platformType string) *xdjkParseResult {
	if !xdjkLLv2SubmitRE.MatchString(code) && !xdjkLongLongV2FileRE.MatchString(code) {
		return nil
	}
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig:      buildXdjkLonglongRuleConfig(),
		Warnings:        []string{"expand 字段取 order.expand JSON；school=自动识别 时上游通常忽略空 school"},
	}
}

func parseXdjkPhpFromGo(code string) *xdjkParseResult {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return &xdjkParseResult{Error: "请粘贴 PHP 代码"}
	}

	specialNotes := detectXdjkSpecialBlockers(trimmed)
	typeMatch := xdjkTypeRE.FindStringSubmatch(trimmed)
	if len(typeMatch) < 2 {
		return &xdjkParseResult{
			Error:        "未找到平台类型，请包含 if ($type == \"xxx\") 片段",
			SpecialNotes: specialNotes,
		}
	}
	platformType := typeMatch[1]

	if ll := parseXdjkLongLongV2(trimmed, platformType); ll != nil {
		ll.SpecialNotes = specialNotes
		return ll
	}

	if known := parseXdjkKnownPlatform(platformType, trimmed, specialNotes); known != nil {
		return known
	}

	var warnings []string
	dataInner := findXdjkDataArrayInner(trimmed)
	body := map[string]string{}

	if dataInner != "" {
		var bodyWarnings []string
		body, bodyWarnings = parseXdjkArrayBody(dataInner)
		warnings = append(warnings, bodyWarnings...)
	} else {
		hasURLOnlyGet := xdjkGetUrlRE.MatchString(trimmed) || xdjkGetUrlOnlyRE.MatchString(trimmed)
		if _, _, _, ok := parseXdjkConcatGetURL(trimmed); ok {
			hasURLOnlyGet = true
		}
		if !hasURLOnlyGet {
			return &xdjkParseResult{
				Error:        "未找到 $data = array(...) 或 $data = [...]，且不是 GET 拼接 URL 写法",
				SpecialNotes: specialNotes,
				PlatformType: platformType,
			}
		}
		warnings = append(warnings, "无 $data 请求体（GET 参数在 URL 中）")
	}

	url, method, urlWarnings := resolveXdjkURL(trimmed)
	warnings = append(warnings, urlWarnings...)

	contentType, useCookie := detectXdjkTransport(trimmed)
	response := parseXdjkResponse(trimmed)
	headers := parseXdjkHeaders(trimmed)

	rule := SubmitRuleConfig{
		Method:      method,
		URL:         url,
		ContentType: contentType,
		UseCookie:   useCookie,
		Body:        body,
		Response:    response,
	}
	if headers != nil {
		rule.Headers = headers
	}
	if len(body) == 0 && method == "GET" {
		rule.ContentType = "form"
	}

	return &xdjkParseResult{
		PlatformType: platformType,
		RuleConfig:   rule,
		Warnings:     warnings,
		SpecialNotes: specialNotes,
	}
}

func xdjkLocalParseComplete(res *xdjkParseResult) bool {
	if res == nil || res.Error != "" {
		return false
	}
	rule := res.RuleConfig
	if !aiRuleHasActionableURL(rule) {
		return false
	}
	if len(rule.Response.SuccessCodes) == 0 && !rule.Response.SuccessHTTP {
		return false
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "嵌套 array") ||
			strings.Contains(w, "未能识别 URL") ||
			strings.Contains(w, "未能识别") ||
			strings.Contains(w, "请手动") {
			return false
		}
	}
	return true
}

func xdjkNeedsNestedBody(code string) bool {
	return xdjkJsonEncodeArrayRE.MatchString(code) ||
		xdjkDataNestedArrayRE.MatchString(code) ||
		xdjkRequestDataRE.MatchString(code) ||
		xdjkPostDataAssignRE.MatchString(code) ||
		xdjkJsonEncodeRequestRE.MatchString(code) ||
		xdjkJsonEncodePostRE.MatchString(code) ||
		(xdjkJsonEncodeDataRE.MatchString(code) && xdjkNestedFieldArrayRE.MatchString(code))
}

func xdjkLocalParseCompleteWithCode(res *xdjkParseResult, php string) bool {
	if !xdjkLocalParseComplete(res) {
		return false
	}
	if res.TrustedTemplate {
		return true
	}
	if xdjkNeedsNestedBody(php) {
		return false
	}
	for _, note := range res.SpecialNotes {
		if xdjkSpecialNoteBlockRE.MatchString(note) {
			return false
		}
	}
	return true
}

func normalizeParsedRule(rule *SubmitRuleConfig) {
	if strings.TrimSpace(rule.Method) == "" {
		rule.Method = "POST"
	}
	if rule.ContentType == "" {
		rule.ContentType = "form"
	}
	if rule.Body == nil {
		rule.Body = map[string]string{}
	}
	if rule.Response.CodeField == "" {
		rule.Response.CodeField = "code"
	}
}

func mergeRuleWithLocalDraft(local, ai SubmitRuleConfig, trustedLocal bool) SubmitRuleConfig {
	if trustedLocal {
		return mergeRulePreferLocalDraft(local, ai)
	}
	return mergeRuleGapFillLocal(local, ai)
}

// mergeRulePreferLocalDraft：内置可信模板，本地 url/method/response 等优先保留。
func mergeRulePreferLocalDraft(local, ai SubmitRuleConfig) SubmitRuleConfig {
	out := ai
	if strings.TrimSpace(local.URL) != "" {
		out.URL = local.URL
	}
	if strings.TrimSpace(local.Method) != "" {
		out.Method = local.Method
	}
	if local.ContentType != "" {
		out.ContentType = local.ContentType
	}
	out.UseCookie = local.UseCookie || ai.UseCookie

	if len(local.Headers) > 0 {
		if out.Headers == nil {
			out.Headers = map[string]string{}
		}
		for k, v := range local.Headers {
			out.Headers[k] = v
		}
	}

	if len(local.Body) > 0 && strings.TrimSpace(local.BodyMode) == "" && strings.TrimSpace(ai.BodyMode) == "" {
		out.Body = local.Body
	}

	if len(local.Response.SuccessCodes) > 0 {
		out.Response.CodeField = local.Response.CodeField
		out.Response.SuccessCodes = local.Response.SuccessCodes
		if local.Response.MsgField != "" {
			out.Response.MsgField = local.Response.MsgField
		}
		if local.Response.YIDField != "" {
			out.Response.YIDField = local.Response.YIDField
		}
		if local.Response.YIDPath != "" {
			out.Response.YIDPath = local.Response.YIDPath
		}
		if local.Response.SuccessMsg != "" {
			out.Response.SuccessMsg = local.Response.SuccessMsg
		}
		if len(local.Response.FailureMsgRules) > 0 {
			out.Response.FailureMsgRules = local.Response.FailureMsgRules
		}
	}
	if local.Response.SuccessHTTP {
		out.Response.SuccessHTTP = true
	}

	if local.Process != nil && out.Process == nil {
		out.Process = local.Process
	}

	normalizeParsedRule(&out)
	return out
}

// mergeRuleGapFillLocal：非可信草稿只补 AI 未填字段，AI 已输出内容不被本地覆盖。
func mergeRuleGapFillLocal(local, ai SubmitRuleConfig) SubmitRuleConfig {
	out := ai
	if strings.TrimSpace(out.URL) == "" && strings.TrimSpace(local.URL) != "" {
		out.URL = local.URL
	}
	if strings.TrimSpace(out.Method) == "" && strings.TrimSpace(local.Method) != "" {
		out.Method = local.Method
	}
	if out.ContentType == "" && local.ContentType != "" {
		out.ContentType = local.ContentType
	}
	out.UseCookie = out.UseCookie || local.UseCookie

	if len(local.Headers) > 0 {
		if out.Headers == nil {
			out.Headers = map[string]string{}
		}
		for k, v := range local.Headers {
			if strings.TrimSpace(out.Headers[k]) == "" {
				out.Headers[k] = v
			}
		}
	}

	if len(local.Body) > 0 && len(out.Body) == 0 &&
		strings.TrimSpace(out.BodyMode) == "" && strings.TrimSpace(local.BodyMode) == "" {
		out.Body = local.Body
	}

	if len(out.Response.SuccessCodes) == 0 && len(local.Response.SuccessCodes) > 0 {
		if local.Response.CodeField != "" {
			out.Response.CodeField = local.Response.CodeField
		}
		out.Response.SuccessCodes = local.Response.SuccessCodes
		if local.Response.MsgField != "" && out.Response.MsgField == "" {
			out.Response.MsgField = local.Response.MsgField
		}
		if local.Response.YIDField != "" && out.Response.YIDField == "" {
			out.Response.YIDField = local.Response.YIDField
		}
		if local.Response.YIDPath != "" && out.Response.YIDPath == "" {
			out.Response.YIDPath = local.Response.YIDPath
		}
		if local.Response.SuccessMsg != "" && out.Response.SuccessMsg == "" {
			out.Response.SuccessMsg = local.Response.SuccessMsg
		}
		if len(local.Response.FailureMsgRules) > 0 && len(out.Response.FailureMsgRules) == 0 {
			out.Response.FailureMsgRules = local.Response.FailureMsgRules
		}
	}
	if !out.Response.SuccessHTTP && local.Response.SuccessHTTP {
		out.Response.SuccessHTTP = true
	}

	if local.Process != nil && out.Process == nil {
		out.Process = local.Process
	}

	normalizeParsedRule(&out)
	return out
}

func buildLocalDraftPrompt(draft *xdjkParseResult, php, hint string) string {
	var b strings.Builder
	b.WriteString(`请将以下 xdjk.php 平台片段转换为 rule_config JSON。

`)
	if draft != nil && draft.TrustedTemplate {
		b.WriteString(`## 本地预解析草稿（内置可信模板）
以下草稿的 url、method、response.success_codes 通常已正确。
你的任务是在此基础上**补全**缺失部分，尤其是：
- body_mode:"raw" + body_raw（嵌套 JSON / json_encode 整段）
- headers、branches、handler/pipeline、process、failure_msg_rules
- 保留草稿中已正确的 url、method、content_type、response，不要随意改掉

`)
	} else {
		b.WriteString(`## 本地预解析草稿（仅供参考，可能有误）
以下草稿由正则/启发式提取，**可能不完整或错误**。
必须以 PHP 源码为准：若草稿与 PHP 不一致，以 PHP 为准并输出修正后的 rule_config。
最终会以你的输出为准，草稿不会强制覆盖你修正过的字段。

`)
	}
	if draft != nil {
		if draft.Error != "" {
			fmt.Fprintf(&b, "本地解析提示: %s\n", draft.Error)
		}
		if draft.PlatformType != "" {
			fmt.Fprintf(&b, "platform_type: %s\n", draft.PlatformType)
		}
		if len(draft.Warnings) > 0 {
			fmt.Fprintf(&b, "本地 warnings: %s\n", strings.Join(draft.Warnings, "；"))
		}
		if len(draft.SpecialNotes) > 0 {
			fmt.Fprintf(&b, "特殊逻辑: %s\n", strings.Join(draft.SpecialNotes, "；"))
		}
		draftJSON, err := jsonMarshalCompact(map[string]interface{}{
			"platform_type": draft.PlatformType,
			"rule_config":   draft.RuleConfig,
		})
		if err == nil {
			b.WriteString("\n本地草稿 JSON:\n")
			b.WriteString(draftJSON)
			b.WriteString("\n\n")
		}
	}
	if hint != "" {
		fmt.Fprintf(&b, "平台类型提示: %s\n\n", hint)
	}
	b.WriteString("## PHP 源码\n```php\n")
	b.WriteString(php)
	b.WriteString("\n```")
	return b.String()
}

func jsonMarshalCompact(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
