package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SubmitRuleWhen 分支匹配条件（全部非空条件需同时满足；all 不为空时子条件全部满足）
type SubmitRuleWhen struct {
	Field       string           `json:"field"`
	Equals      string           `json:"equals,omitempty"`
	NotEquals   string           `json:"not_equals,omitempty"`
	Contains    string           `json:"contains,omitempty"`
	NotContains string           `json:"not_contains,omitempty"`
	Default     bool             `json:"default,omitempty"`
	All         []SubmitRuleWhen `json:"all,omitempty"`
}

// SubmitRuleBranch 按条件覆盖部分规则字段（首个匹配生效，无匹配则用 default 分支）
type SubmitRuleBranch struct {
	When            *SubmitRuleWhen     `json:"when"`
	Method          string              `json:"method,omitempty"`
	URL             string              `json:"url,omitempty"`
	ContentType     string              `json:"content_type,omitempty"`
	UseCookie       *bool               `json:"use_cookie,omitempty"`
	Headers         map[string]string   `json:"headers,omitempty"`
	Body            map[string]string   `json:"body,omitempty"`
	BodyMode        string              `json:"body_mode,omitempty"`
	BodyRaw         string              `json:"body_raw,omitempty"`
	KcidJSONPatches []KcidJSONPatch     `json:"kcid_json_patches,omitempty"`
	Response        *SubmitRuleResp     `json:"response,omitempty"`
}

// KcidJSONPatch 对 base64 解码后的 kcid JSON 按 noun 等条件打补丁
type KcidJSONPatch struct {
	When *SubmitRuleWhen        `json:"when"`
	Set  map[string]interface{} `json:"set"`
}

// KcidJSONValidate 提交前校验解码后的 JSON
type KcidJSONValidate struct {
	Path   string `json:"path"`
	MinLen int    `json:"min_len,omitempty"`
	MaxLen int    `json:"max_len,omitempty"`
	Exact  int    `json:"exact,omitempty"`
}

type submitTemplateCtx struct {
	randomPort int
	vars       map[string]string
}

func (t *submitTemplateCtx) varGet(key string) string {
	if t == nil || t.vars == nil {
		return ""
	}
	return t.vars[strings.TrimSpace(key)]
}

func (t *submitTemplateCtx) varSet(key, val string) {
	if t == nil {
		return
	}
	if t.vars == nil {
		t.vars = map[string]string{}
	}
	t.vars[strings.TrimSpace(key)] = val
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func resolveEffectiveRule(rule SubmitRuleConfig, order *Order) SubmitRuleConfig {
	if len(rule.Branches) == 0 {
		return rule
	}
	var matched, fallback *SubmitRuleBranch
	for i := range rule.Branches {
		b := &rule.Branches[i]
		if b.When != nil && b.When.Default {
			fallback = b
			continue
		}
		if b.When != nil && matchSubmitWhen(b.When, order) {
			matched = b
			break
		}
	}
	if matched == nil {
		matched = fallback
	}
	if matched == nil {
		return rule
	}
	return mergeSubmitBranch(rule, *matched)
}

func mergeSubmitBranch(base SubmitRuleConfig, br SubmitRuleBranch) SubmitRuleConfig {
	out := base
	if br.Method != "" {
		out.Method = br.Method
	}
	if br.URL != "" {
		out.URL = br.URL
	}
	if br.ContentType != "" {
		out.ContentType = br.ContentType
	}
	if br.UseCookie != nil {
		out.UseCookie = *br.UseCookie
	}
	if len(br.Headers) > 0 {
		if out.Headers == nil {
			out.Headers = map[string]string{}
		}
		for k, v := range br.Headers {
			out.Headers[k] = v
		}
	}
	if len(br.Body) > 0 {
		out.Body = br.Body
	}
	if br.BodyMode != "" {
		out.BodyMode = br.BodyMode
	}
	if br.BodyRaw != "" {
		out.BodyRaw = br.BodyRaw
	}
	if len(br.KcidJSONPatches) > 0 {
		out.KcidJSONPatches = br.KcidJSONPatches
	}
	if br.Response != nil {
		out.Response = *br.Response
	}
	return out
}

func matchSubmitWhen(w *SubmitRuleWhen, order *Order) bool {
	if w == nil || w.Default {
		return false
	}
	if len(w.All) > 0 {
		for i := range w.All {
			sub := w.All[i]
			sub.Default = false
			if !matchSubmitWhen(&sub, order) {
				return false
			}
		}
		return true
	}
	val, err := resolveSubmitField(strings.TrimSpace(w.Field), order, nil)
	if err != nil {
		val = ""
	}
	if w.Equals != "" && val != w.Equals {
		return false
	}
	if w.NotEquals != "" && val == w.NotEquals {
		return false
	}
	if w.Contains != "" && !strings.Contains(val, w.Contains) {
		return false
	}
	if w.NotContains != "" && strings.Contains(val, w.NotContains) {
		return false
	}
	return w.Equals != "" || w.NotEquals != "" || w.Contains != "" || w.NotContains != ""
}

func buildSubmitBody(rule SubmitRuleConfig, order *Order, hy *Huoyuan, tctx *submitTemplateCtx) (body string, isJSON bool, hdrExtra []string, errResult *AddOrderResult) {
	mode := strings.ToLower(strings.TrimSpace(rule.BodyMode))
	isJSON = strings.EqualFold(rule.ContentType, "json")

	switch mode {
	case "raw", "template":
		raw, err := renderSubmitTemplate(rule.BodyRaw, order, hy, tctx)
		if err != nil {
			return "", isJSON, nil, &AddOrderResult{Code: -1, Msg: "Body 模板解析失败: " + err.Error()}
		}
		body = raw
		if !isJSON {
			trim := strings.TrimSpace(raw)
			if strings.HasPrefix(trim, "{") || strings.HasPrefix(trim, "[") {
				isJSON = true
			}
		}
		if isJSON {
			hdrExtra = append(hdrExtra, "Content-Type: application/json")
		} else if body != "" {
			hdrExtra = append(hdrExtra, "Content-Type: application/x-www-form-urlencoded")
		}
		return body, isJSON, hdrExtra, nil

	case "kcid_json":
		b, err := buildKcidJSONBody(rule, order)
		if err != nil {
			return "", true, nil, &AddOrderResult{Code: -1, Msg: err.Error()}
		}
		return string(b), true, []string{"Content-Type: application/json"}, nil
	}

	// 默认 map 模式
	if len(rule.Body) == 0 {
		return "", isJSON, nil, nil
	}
	if isJSON {
		m := make(map[string]interface{})
		for k, tpl := range rule.Body {
			val, err := renderSubmitTemplate(tpl, order, hy, tctx)
			if err != nil {
				return "", isJSON, nil, &AddOrderResult{Code: -1, Msg: "Body 模板解析失败: " + err.Error()}
			}
			if val != "" {
				m[k] = val
			}
		}
		b, err := json.Marshal(m)
		if err != nil {
			return "", isJSON, nil, &AddOrderResult{Code: -1, Msg: "JSON 序列化失败"}
		}
		body = string(b)
		hdrExtra = append(hdrExtra, "Content-Type: application/json")
	} else {
		form := url.Values{}
		for k, tpl := range rule.Body {
			val, err := renderSubmitTemplate(tpl, order, hy, tctx)
			if err != nil {
				return "", isJSON, nil, &AddOrderResult{Code: -1, Msg: "Body 模板解析失败: " + err.Error()}
			}
			form.Set(k, val)
		}
		body = form.Encode()
		hdrExtra = append(hdrExtra, "Content-Type: application/x-www-form-urlencoded")
	}
	return body, isJSON, hdrExtra, nil
}

func buildKcidJSONBody(rule SubmitRuleConfig, order *Order) ([]byte, error) {
	if order == nil || strings.TrimSpace(order.KCID) == "" {
		return nil, fmt.Errorf("kcid 为空")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(order.KCID))
	if err != nil {
		return nil, fmt.Errorf("kcid base64 解码失败")
	}
	var root interface{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("kcid JSON 解析失败")
	}
	patches := rule.KcidJSONPatches
	if len(patches) == 0 && len(rule.Branches) == 0 {
		// 无补丁则原样提交
	} else {
		for _, p := range patches {
			if p.When != nil && !matchSubmitWhen(p.When, order) {
				continue
			}
			for path, val := range p.Set {
				if err := setJSONPath(root, path, val); err != nil {
					return nil, fmt.Errorf("JSON 补丁失败(%s): %w", path, err)
				}
			}
		}
	}
	if v := rule.KcidJSONValidate; v != nil && v.Path != "" {
		if err := validateJSONPath(root, v); err != nil {
			return nil, err
		}
	}
	return json.Marshal(root)
}

func setJSONPath(root interface{}, path string, val interface{}) error {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return fmt.Errorf("空路径")
	}
	cur := root
	for i := 0; i < len(parts)-1; i++ {
		idx, isIndex := parseJSONIndex(parts[i])
		switch node := cur.(type) {
		case map[string]interface{}:
			next, ok := node[parts[i]]
			if !ok {
				child := map[string]interface{}{}
				node[parts[i]] = child
				cur = child
				continue
			}
			cur = next
		case []interface{}:
			if !isIndex || idx < 0 || idx >= len(node) {
				return fmt.Errorf("数组索引无效: %s", parts[i])
			}
			cur = node[idx]
		default:
			return fmt.Errorf("路径无效: %s", parts[i])
		}
	}
	last := parts[len(parts)-1]
	switch node := cur.(type) {
	case map[string]interface{}:
		node[last] = val
		return nil
	case []interface{}:
		idx, ok := parseJSONIndex(last)
		if !ok || idx < 0 || idx >= len(node) {
			return fmt.Errorf("数组索引无效: %s", last)
		}
		node[idx] = val
		return nil
	default:
		return fmt.Errorf("无法写入: %s", path)
	}
}

func parseJSONIndex(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func validateJSONPath(root interface{}, v *KcidJSONValidate) error {
	cur := root
	for _, p := range strings.Split(v.Path, ".") {
		if p == "" {
			continue
		}
		if idx, ok := parseJSONIndex(p); ok {
			arr, ok := cur.([]interface{})
			if !ok {
				return fmt.Errorf("参数异常")
			}
			if v.Exact > 0 && len(arr) != v.Exact {
				return fmt.Errorf("参数异常")
			}
			if v.MinLen > 0 && len(arr) < v.MinLen {
				return fmt.Errorf("参数异常")
			}
			if v.MaxLen > 0 && len(arr) > v.MaxLen {
				return fmt.Errorf("参数异常")
			}
			if idx >= 0 && idx < len(arr) {
				cur = arr[idx]
			}
			continue
		}
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return fmt.Errorf("参数异常")
		}
		cur = obj[p]
	}
	if arr, ok := cur.([]interface{}); ok {
		if v.Exact > 0 && len(arr) != v.Exact {
			return fmt.Errorf("参数异常")
		}
	}
	return nil
}

func renderSubmitTemplate(tpl string, order *Order, hy *Huoyuan, tctx *submitTemplateCtx) (string, error) {
	if tpl == "" {
		return "", nil
	}
	if tctx == nil {
		tctx = &submitTemplateCtx{}
	}
	var errOut error
	out := tplVarRe.ReplaceAllStringFunc(tpl, func(match string) string {
		inner := strings.TrimSpace(match[2 : len(match)-2])
		val, err := evalSubmitTemplateExpr(inner, order, hy, tctx)
		if err != nil {
			errOut = err
			return match
		}
		return val
	})
	return out, errOut
}

func evalSubmitTemplateExpr(inner string, order *Order, hy *Huoyuan, tctx *submitTemplateCtx) (string, error) {
	args := splitTemplateArgs(inner)
	if len(args) == 0 {
		return "", fmt.Errorf("空模板")
	}
	fn := strings.ToLower(args[0])
	switch fn {
	case "urlencode":
		if len(args) < 2 {
			return "", fmt.Errorf("urlencode 缺少参数")
		}
		val, err := evalSubmitTemplateArg(args[1], order, hy, tctx)
		if err != nil {
			return "", err
		}
		return url.QueryEscape(val), nil
	case "base64_decode":
		if len(args) < 2 {
			return "", fmt.Errorf("base64_decode 缺少参数")
		}
		val, err := evalSubmitTemplateArg(args[1], order, hy, tctx)
		if err != nil {
			return "", err
		}
		b, err := base64.StdEncoding.DecodeString(val)
		if err != nil {
			return "", fmt.Errorf("base64 解码失败")
		}
		return string(b), nil
	case "concat":
		return evalConcatTemplateArgs(args, order, hy, tctx)
	case "split_part":
		if len(args) < 4 {
			return "", fmt.Errorf("split_part 需要 split_part 值 索引 分隔符 [limit]")
		}
		val, err := evalSubmitTemplateArgOrExpr(args[1], order, hy, tctx)
		if err != nil {
			return "", err
		}
		idx, err := strconv.Atoi(strings.TrimSpace(args[2]))
		if err != nil {
			return "", fmt.Errorf("split_part 索引无效")
		}
		delim, err := evalSubmitTemplateArgOrExpr(args[3], order, hy, tctx)
		if err != nil {
			return "", err
		}
		limit := 0
		if len(args) >= 5 {
			limit, _ = strconv.Atoi(strings.TrimSpace(args[4]))
		}
		return splitTemplatePart(val, idx, delim, limit), nil
	case "random_port":
		if tctx.randomPort > 0 {
			return strconv.Itoa(tctx.randomPort), nil
		}
		return "", fmt.Errorf("未配置 url_port_pool")
	case "json_path":
		if len(args) < 3 {
			return "", fmt.Errorf("json_path 需要 json_path 变量名 路径")
		}
		fromVar := strings.TrimSpace(args[1])
		fromVar = strings.TrimPrefix(fromVar, "var.")
		raw := tctx.varGet(fromVar)
		if raw == "" {
			return "", fmt.Errorf("变量 %s 为空", fromVar)
		}
		return jsonPathString(raw, strings.TrimSpace(args[2]))
	default:
		// order.xxx / huoyuan.xxx
		if strings.Contains(fn, ".") && len(args) == 1 {
			return evalSubmitTemplateArg(args[0], order, hy, tctx)
		}
		return "", fmt.Errorf("未知模板函数: %s", fn)
	}
}

func evalConcatTemplateArgs(args []string, order *Order, hy *Huoyuan, tctx *submitTemplateCtx) (string, error) {
	var b strings.Builder
	for i := 1; i < len(args); {
		if isSubmitTemplateFn(args[i]) {
			end := i + 1 + templateFnArity(args[i])
			if strings.EqualFold(args[i], "split_part") {
				if i+1+3 > len(args) {
					return "", fmt.Errorf("split_part 参数不足")
				}
				end = i + 1 + 3
				if end < len(args) {
					if _, err := strconv.Atoi(strings.TrimSpace(args[end])); err == nil {
						end++
					}
				}
			}
			if end > len(args) {
				return "", fmt.Errorf("%s 参数不足", args[i])
			}
			subExpr := strings.Join(args[i:end], " ")
			val, err := evalSubmitTemplateExpr(subExpr, order, hy, tctx)
			if err != nil {
				return "", err
			}
			b.WriteString(val)
			i = end
			continue
		}
		val, err := evalSubmitTemplateArgOrExpr(args[i], order, hy, tctx)
		if err != nil {
			return "", err
		}
		b.WriteString(val)
		i++
	}
	return b.String(), nil
}

func templateFnArity(name string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "split_part":
		return 3
	case "urlencode", "base64_decode":
		return 1
	case "json_path":
		return 2
	default:
		return 0
	}
}

func evalSubmitTemplateArg(arg string, order *Order, hy *Huoyuan, tctx *submitTemplateCtx) (string, error) {
	arg = strings.TrimSpace(arg)
	if len(arg) >= 2 {
		if (arg[0] == '"' && arg[len(arg)-1] == '"') || (arg[0] == '\'' && arg[len(arg)-1] == '\'') {
			return arg[1 : len(arg)-1], nil
		}
	}
	if strings.Contains(arg, ".") {
		parts := strings.SplitN(arg, ".", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "var") {
			return tctx.varGet(parts[1]), nil
		}
		return resolveSubmitField(arg, order, hy)
	}
	return arg, nil
}

func evalSubmitTemplateArgOrExpr(arg string, order *Order, hy *Huoyuan, tctx *submitTemplateCtx) (string, error) {
	arg = strings.TrimSpace(arg)
	if strings.Contains(arg, " ") {
		parts := splitTemplateArgs(arg)
		if len(parts) > 0 && isSubmitTemplateFn(parts[0]) {
			return evalSubmitTemplateExpr(arg, order, hy, tctx)
		}
	}
	return evalSubmitTemplateArg(arg, order, hy, tctx)
}

func isSubmitTemplateFn(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "urlencode", "base64_decode", "concat", "split_part", "random_port", "json_path":
		return true
	default:
		return false
	}
}

func splitTemplatePart(val string, index int, delim string, limit int) string {
	var parts []string
	if limit > 0 {
		parts = strings.SplitN(val, delim, limit)
	} else {
		parts = strings.Split(val, delim)
	}
	if index < 0 || index >= len(parts) {
		return ""
	}
	return parts[index]
}

func splitTemplateArgs(s string) []string {
	var args []string
	var cur strings.Builder
	inQuote := rune(0)
	for i, r := range s {
		switch {
		case inQuote != 0:
			cur.WriteRune(r)
			if r == inQuote && (i == 0 || s[i-1] != '\\') {
				inQuote = 0
			}
		case r == '"' || r == '\'':
			inQuote = r
			cur.WriteRune(r)
		case r == ' ' || r == '\t':
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

func newSubmitTemplateCtx(rule SubmitRuleConfig) *submitTemplateCtx {
	tctx := &submitTemplateCtx{vars: map[string]string{}}
	if len(rule.URLPortPool) > 0 {
		tctx.randomPort = rule.URLPortPool[rand.Intn(len(rule.URLPortPool))]
	}
	return tctx
}
