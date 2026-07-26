package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	submitRuleCachePrefix = "tj:submit:rule:"
	submitRuleCacheAll    = "tj:submit:rules:all"
)

// SubmitRuleConfig 平台提交规则（存储于 rule_config JSON 列）
// handler: 空/http=单次 HTTP；pipeline=多步流程；script=script.steps 或未来 Starlark
type SubmitRuleConfig struct {
	Handler           string               `json:"handler,omitempty"`
	Method            string               `json:"method"`
	URL               string               `json:"url"`
	ContentType       string               `json:"content_type"` // form | json
	UseCookie         bool                 `json:"use_cookie"`
	Headers           map[string]string    `json:"headers"`
	Body              map[string]string    `json:"body"`
	BodyMode          string               `json:"body_mode"` // 空/map | raw | kcid_json
	BodyRaw           string               `json:"body_raw"`
	URLPortPool       []int                `json:"url_port_pool"`
	Branches          []SubmitRuleBranch   `json:"branches"`
	KcidJSONPatches   []KcidJSONPatch      `json:"kcid_json_patches"`
	KcidJSONValidate  *KcidJSONValidate    `json:"kcid_json_validate"`
	DelayMS           int                  `json:"delay_ms"`
	Pipeline          []SubmitPipelineStep `json:"pipeline,omitempty"`
	Script            *SubmitScriptConfig  `json:"script,omitempty"`
	Process           *ProcessRuleConfig   `json:"process,omitempty"`
	Response          SubmitRuleResp       `json:"response"`
}

// SubmitPipelineStep 多步流程中的一步（handler=pipeline 或 script.steps）
type SubmitPipelineStep struct {
	Name       string              `json:"name,omitempty"`
	When       *SubmitRuleWhen     `json:"when,omitempty"`
	Action     string              `json:"action,omitempty"` // set|delay|http|finish|extract|return|poll|process_finish
	DelayMS    int                 `json:"delay_ms,omitempty"`
	Set        map[string]string   `json:"set,omitempty"`
	Extract    *PipelineExtract    `json:"extract,omitempty"`
	Method     string              `json:"method,omitempty"`
	URL        string              `json:"url,omitempty"`
	ContentType string             `json:"content_type,omitempty"`
	UseCookie  bool                `json:"use_cookie,omitempty"`
	Headers    map[string]string   `json:"headers,omitempty"`
	Body       map[string]string   `json:"body,omitempty"`
	BodyMode   string              `json:"body_mode,omitempty"`
	BodyRaw    string              `json:"body_raw,omitempty"`
	KcidJSONPatches []KcidJSONPatch `json:"kcid_json_patches,omitempty"`
	SaveBodyAs string              `json:"save_body_as,omitempty"`
	Response   *SubmitRuleResp     `json:"response,omitempty"`
	ReturnCode int                 `json:"return_code,omitempty"`
	ReturnMsg  string              `json:"return_msg,omitempty"`
	ReturnYID  string              `json:"return_yid,omitempty"`
	Poll       *PipelinePollConfig `json:"poll,omitempty"`
	ProcessMap *ProcessResultMap   `json:"process_map,omitempty"`
}

// PipelinePollConfig 轮询 HTTP 直到响应满足 until（或达到 max_attempts）
type PipelinePollConfig struct {
	IntervalMS  int             `json:"interval_ms,omitempty"`
	MaxAttempts int             `json:"max_attempts,omitempty"`
	Until       SubmitRuleResp  `json:"until"`
	Fail        *SubmitRuleResp `json:"fail,omitempty"`
}

// ProcessRuleConfig 查课/同步进度（ProcessOrder）
type ProcessRuleConfig struct {
	Handler     string               `json:"handler,omitempty"` // http | pipeline | script
	Method      string               `json:"method,omitempty"`
	URL         string               `json:"url,omitempty"`
	ContentType string               `json:"content_type,omitempty"`
	UseCookie   bool                 `json:"use_cookie,omitempty"`
	Headers     map[string]string    `json:"headers,omitempty"`
	Body        map[string]string    `json:"body,omitempty"`
	BodyMode    string               `json:"body_mode,omitempty"`
	BodyRaw     string               `json:"body_raw,omitempty"`
	Pipeline    []SubmitPipelineStep `json:"pipeline,omitempty"`
	Script      *SubmitScriptConfig  `json:"script,omitempty"`
	Map         ProcessResultMap     `json:"map"`
}

// ProcessResultMap 将上游 JSON 映射为 ProcessCxResult 列表
type ProcessResultMap struct {
	ItemsPath    string            `json:"items_path,omitempty"`
	CodeField    string            `json:"code_field,omitempty"`
	SuccessCodes []interface{}     `json:"success_codes,omitempty"`
	MsgField     string            `json:"msg_field,omitempty"`
	Fields       map[string]string `json:"fields"`
}

type PipelineExtract struct {
	From string `json:"from"` // 变量名，存的是 JSON 响应体
	Path string `json:"path"` // 如 data.token
	To   string `json:"to"`   // 写入 var
}

// SubmitScriptConfig 内嵌脚本（steps 与 pipeline 同引擎；source 预留 Starlark）
type SubmitScriptConfig struct {
	Steps     []SubmitPipelineStep `json:"steps,omitempty"`
	Source    string               `json:"source,omitempty"`
	TimeoutMS int                  `json:"timeout_ms,omitempty"`
}

// FailureMsgRule 失败时按上游 msg 子串映射返回文案（对应 PHP strpos 分支）
type FailureMsgRule struct {
	Contains string `json:"contains"`
	Msg      string `json:"msg"`
	Code     int    `json:"code,omitempty"` // 默认 -1
}

type SubmitRuleResp struct {
	CodeField              string           `json:"code_field"`
	SuccessCodes           []interface{}    `json:"success_codes"`
	SuccessHTTP            bool             `json:"success_http,omitempty"` // 龙龙 V2 等：HTTP 2xx 即成功，响应可无 code
	MsgField               string           `json:"msg_field"`
	YIDField               string           `json:"yid_field"`
	YIDPath                string           `json:"yid_path"`
	SuccessMsg             string           `json:"success_msg"`
	FailureMsgRules        []FailureMsgRule `json:"failure_msg_rules,omitempty"`
	FailureMsgOnSuccess    bool             `json:"failure_msg_on_success,omitempty"`    // 8090 等：success_codes 命中后仍按 msg 子串判失败
	SuccessUseUpstreamMsg  bool             `json:"success_use_upstream_msg,omitempty"` // 成功时返回上游 msg_field 内容
}

// SubmitPlatformRow 数据库行
type SubmitPlatformRow struct {
	ID           int
	PlatformType string
	DisplayName  string
	Enabled      bool
	RuleConfig   SubmitRuleConfig
	Version      int
	Remark       string
}

var tplVarRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// dbRulePlatformPlugin 基于数据库规则的动态插件
type dbRulePlatformPlugin struct {
	typ  string
	rule SubmitRuleConfig
}

func (p *dbRulePlatformPlugin) GetType() string { return p.typ }

func (p *dbRulePlatformPlugin) ProcessOrder(ctx context.Context, order *Order, huoyuan *Huoyuan, httpClient *http.Client) ([]*ProcessCxResult, error) {
	if p.rule.Process == nil {
		return nil, nil
	}
	return executeProcessRule(ctx, order, huoyuan, httpClient, *p.rule.Process)
}

func (p *dbRulePlatformPlugin) AddOrder(ctx context.Context, order *Order, huoyuan *Huoyuan, httpClient *http.Client) (*AddOrderResult, error) {
	return executeSubmitRule(ctx, order, huoyuan, httpClient, p.rule)
}

func executeSubmitRule(ctx context.Context, order *Order, hy *Huoyuan, httpClient *http.Client, rule SubmitRuleConfig) (*AddOrderResult, error) {
	rule = resolveEffectiveRule(rule, order)
	handler := strings.ToLower(strings.TrimSpace(rule.Handler))
	switch handler {
	case "pipeline":
		return executePipelineSubmitRule(ctx, order, hy, httpClient, rule)
	case "script":
		return executeScriptSubmitRule(ctx, order, hy, httpClient, rule)
	default:
		return executeHTTPSubmitRule(ctx, order, hy, httpClient, rule)
	}
}

func executeHTTPSubmitRule(ctx context.Context, order *Order, hy *Huoyuan, httpClient *http.Client, rule SubmitRuleConfig) (*AddOrderResult, error) {
	client := pluginHTTPClient(httpClient)

	if rule.DelayMS > 0 {
		select {
		case <-ctx.Done():
			return &AddOrderResult{Code: -1, Msg: "已取消"}, nil
		case <-time.After(time.Duration(rule.DelayMS) * time.Millisecond):
		}
	}

	tctx := newSubmitTemplateCtx(rule)

	reqURL, err := renderSubmitTemplate(rule.URL, order, hy, tctx)
	if err != nil {
		return &AddOrderResult{Code: -1, Msg: "URL 模板解析失败: " + err.Error()}, nil
	}
	reqURL = strings.TrimSpace(reqURL)
	if reqURL == "" {
		return &AddOrderResult{Code: -1, Msg: "请求 URL 为空"}, nil
	}

	method := strings.ToUpper(strings.TrimSpace(rule.Method))
	if method == "" {
		method = "POST"
	}

	var hdr []string
	for k, v := range rule.Headers {
		rendered, err := renderSubmitTemplate(v, order, hy, tctx)
		if err != nil {
			return &AddOrderResult{Code: -1, Msg: "Header 模板解析失败: " + err.Error()}, nil
		}
		hdr = append(hdr, k+": "+rendered)
	}
	if rule.UseCookie && hy.Cookie != "" {
		hdr = append(hdr, "Cookie: "+hy.Cookie)
	}

	isJSON := strings.EqualFold(rule.ContentType, "json")
	body, isJSON, bodyHdr, errResult := buildSubmitBody(rule, order, hy, tctx)
	if errResult != nil {
		return errResult, nil
	}
	hdr = append(hdr, bodyHdr...)

	var respBody string
	if method == "GET" {
		if body != "" {
			sep := "?"
			if strings.Contains(reqURL, "?") {
				sep = "&"
			}
			reqURL = reqURL + sep + body
		}
		respBody, err = httpRequestCommon(ctx, client, "GET", reqURL, nil, hdr, false)
	} else {
		respBody, err = httpRequestCommon(ctx, client, method, reqURL, strings.NewReader(body), hdr, isJSON)
	}
	if err != nil {
		return &AddOrderResult{Code: -1, Msg: "请求失败: " + err.Error()}, nil
	}

	return parseSubmitResponse(respBody, rule.Response)
}

func parseSubmitResponse(body string, resp SubmitRuleResp) (*AddOrderResult, error) {
	rawBody := body
	body = strings.TrimSpace(body)
	if body == "" {
		if resp.SuccessHTTP {
			return &AddOrderResult{Code: -1, Msg: "下单成功但未返回订单UUID", UpstreamBody: truncateUpstreamForAutoFix(rawBody)}, nil
		}
		return &AddOrderResult{Code: -1, Msg: "响应解析失败", UpstreamBody: truncateUpstreamForAutoFix(rawBody)}, nil
	}

	var root interface{}
	if err := json.Unmarshal([]byte(body), &root); err != nil {
		if resp.SuccessHTTP {
			return &AddOrderResult{Code: 1, Msg: submitSuccessMsg(resp), YID: body}, nil
		}
		return &AddOrderResult{Code: -1, Msg: "响应解析失败", UpstreamBody: truncateUpstreamForAutoFix(rawBody)}, nil
	}

	ok := resp.SuccessHTTP
	msg := ""
	if !ok {
		m, okMap := root.(map[string]interface{})
		if !okMap {
			return &AddOrderResult{Code: -1, Msg: "响应格式错误", UpstreamBody: truncateUpstreamForAutoFix(rawBody)}, nil
		}
		codeField := resp.CodeField
		if codeField == "" {
			codeField = "code"
		}
		codeVal := m[codeField]
		for _, sc := range resp.SuccessCodes {
			if codeMatches(codeVal, sc) {
				ok = true
				break
			}
		}
		msgField := resp.MsgField
		if msgField == "" {
			msgField = "msg"
		}
		msg = mapGetString(m, msgField)
	}

	if !ok {
		if msg == "" {
			if m, okMap := root.(map[string]interface{}); okMap {
				msgField := resp.MsgField
				if msgField == "" {
					msgField = "msg"
				}
				msg = mapGetString(m, msgField)
			}
		}
		if mapped := matchFailureMsgRule(msg, resp.FailureMsgRules); mapped != nil {
			mapped.UpstreamBody = truncateUpstreamForAutoFix(rawBody)
			return mapped, nil
		}
		return &AddOrderResult{Code: -1, Msg: firstNonEmpty(msg, "下单失败"), UpstreamBody: truncateUpstreamForAutoFix(rawBody)}, nil
	}

	if resp.FailureMsgOnSuccess {
		if msg == "" {
			if m, okMap := root.(map[string]interface{}); okMap {
				msgField := resp.MsgField
				if msgField == "" {
					msgField = "msg"
				}
				msg = mapGetString(m, msgField)
			}
		}
		if mapped := matchFailureMsgRule(msg, resp.FailureMsgRules); mapped != nil {
			mapped.UpstreamBody = truncateUpstreamForAutoFix(rawBody)
			return mapped, nil
		}
	}

	yid := extractSubmitYID(root, resp)
	if yid == "" && (resp.YIDField != "" || resp.YIDPath != "") {
		return &AddOrderResult{Code: -1, Msg: "下单成功但未返回订单UUID", UpstreamBody: truncateUpstreamForAutoFix(rawBody)}, nil
	}
	outMsg := submitSuccessMsg(resp)
	if resp.SuccessUseUpstreamMsg && msg != "" {
		outMsg = msg
	}
	return &AddOrderResult{Code: 1, Msg: outMsg, YID: yid}, nil
}

func submitSuccessMsg(resp SubmitRuleResp) string {
	if strings.TrimSpace(resp.SuccessMsg) != "" {
		return resp.SuccessMsg
	}
	return "下单成功"
}

func extractSubmitYID(root interface{}, resp SubmitRuleResp) string {
	if resp.YIDPath != "" {
		if yid := strings.TrimSpace(flexString(getNestedValueAny(root, resp.YIDPath))); yid != "" {
			return yid
		}
	}
	if resp.YIDField != "" {
		if m, ok := root.(map[string]interface{}); ok {
			if yid := strings.TrimSpace(mapGetString(m, resp.YIDField)); yid != "" {
				return yid
			}
		}
	}
	if arr, ok := root.([]interface{}); ok {
		for _, item := range arr {
			if yid := strings.TrimSpace(flexString(item)); yid != "" {
				return yid
			}
		}
	}
	if s, ok := root.(string); ok {
		return strings.TrimSpace(s)
	}
	if m, ok := root.(map[string]interface{}); ok {
		for _, key := range []string{"uuid", "id", "orderId", "yid"} {
			if yid := strings.TrimSpace(mapGetString(m, key)); yid != "" {
				return yid
			}
		}
	}
	return ""
}

func codeMatches(actual, expected interface{}) bool {
	return flexString(actual) == flexString(expected)
}

func matchFailureMsgRule(msg string, rules []FailureMsgRule) *AddOrderResult {
	if msg == "" || len(rules) == 0 {
		return nil
	}
	for _, rule := range rules {
		needle := strings.TrimSpace(rule.Contains)
		if needle == "" {
			continue
		}
		if !strings.Contains(msg, needle) {
			continue
		}
		outCode := rule.Code
		if outCode == 0 {
			outCode = -1
		}
		return &AddOrderResult{
			Code: outCode,
			Msg:  firstNonEmpty(strings.TrimSpace(rule.Msg), msg),
		}
	}
	return nil
}

func getNestedValue(m map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	var cur interface{} = m
	for _, p := range parts {
		if p == "" {
			continue
		}
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return nil
		}
		cur = obj[p]
	}
	return cur
}

func resolveSubmitField(field string, order *Order, hy *Huoyuan) (string, error) {
	dot := strings.Index(field, ".")
	if dot < 0 {
		return "", fmt.Errorf("未知字段: %s", field)
	}
	src, key := field[:dot], field[dot+1:]
	switch src {
	case "order":
		return orderField(order, key), nil
	case "huoyuan":
		return huoyuanField(hy, key), nil
	default:
		return "", fmt.Errorf("未知来源: %s", src)
	}
}

func orderExpandField(o *Order, subkey string) string {
	if o == nil || strings.TrimSpace(o.Expand) == "" {
		return ""
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(o.Expand), &root); err != nil {
		return ""
	}
	subkey = strings.ToLower(strings.TrimSpace(subkey))
	switch subkey {
	case "city", "tag", "config":
		return flexString(root[subkey])
	case "remark":
		v := root["remark"]
		if arr, ok := v.([]interface{}); ok {
			parts := make([]string, 0, len(arr))
			for _, item := range arr {
				if s := strings.TrimSpace(flexString(item)); s != "" {
					parts = append(parts, s)
				}
			}
			return strings.Join(parts, ",")
		}
		return flexString(v)
	default:
		return flexString(getNestedValueAny(root, subkey))
	}
}

func orderField(o *Order, key string) string {
	if o == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "oid":
		return o.OID
	case "hid":
		return o.HID
	case "user":
		return o.User
	case "pass":
		return o.Pass
	case "school":
		return o.School
	case "noun":
		return o.Noun
	case "kcid":
		return o.KCID
	case "kcname":
		return o.KCName
	case "utime", "u_time":
		return o.UTime
	case "uscore", "u_score":
		return o.UScore
	case "study_speed":
		return o.StudySpeed
	case "is_submit_exam":
		return o.IsSubmitExam
	case "exam_time":
		return o.ExamTime
	case "name":
		return o.Name
	case "status":
		return o.Status
	case "process":
		return o.Process
	case "remarks":
		return o.Remarks
	case "dockstatus", "dock_status":
		return o.DockStatus
	case "yid":
		return o.YID
	case "isck", "is_ck":
		return o.IsCk
	case "simple_day_score":
		return o.SimpleDayScore
	case "simple_total_score":
		return o.SimpleTotalScore
	case "simple_learn_time":
		return o.SimpleLearnTime
	case "ikun_study_ip":
		return o.IkunStudyIP
	case "expand":
		return o.Expand
	default:
		if strings.HasPrefix(strings.ToLower(key), "expand.") {
			return orderExpandField(o, key[7:])
		}
		return ""
	}
}

func huoyuanField(h *Huoyuan, key string) string {
	if h == nil {
		return ""
	}
	switch key {
	case "url", "URL":
		return normalizeHuoyuanURL(h.URL)
	case "user":
		return h.User
	case "pass":
		return h.Pass
	case "token":
		return h.Token
	case "cookie":
		return h.Cookie
	default:
		return ""
	}
}

func submitRuleCacheKey(platformType string) string {
	return submitRuleCachePrefix + platformType
}

func loadSubmitRuleFromDB(ctx context.Context, platformType string) (*SubmitPlatformRow, error) {
	if pluginDB == nil {
		return nil, nil
	}
	q := fmt.Sprintf(`SELECT id, platform_type, display_name, enabled, rule_config, version, IFNULL(remark,'')
		FROM %s WHERE platform_type=? LIMIT 1`, pluginTable("submit_platform"))
	var row SubmitPlatformRow
	var ruleJSON []byte
	var enabled int
	err := pluginDB.QueryRowContext(ctx, q, platformType).Scan(
		&row.ID, &row.PlatformType, &row.DisplayName, &enabled, &ruleJSON, &row.Version, &row.Remark,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.Enabled = enabled == 1
	if err := json.Unmarshal(ruleJSON, &row.RuleConfig); err != nil {
		return nil, fmt.Errorf("规则 JSON 无效: %w", err)
	}
	return &row, nil
}

func loadAllEnabledSubmitRulesFromDB(ctx context.Context) ([]SubmitPlatformRow, error) {
	if pluginDB == nil {
		return nil, nil
	}
	q := fmt.Sprintf(`SELECT id, platform_type, display_name, enabled, rule_config, version, IFNULL(remark,'')
		FROM %s WHERE enabled=1 ORDER BY id ASC`, pluginTable("submit_platform"))
	rows, err := pluginDB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []SubmitPlatformRow
	for rows.Next() {
		var row SubmitPlatformRow
		var ruleJSON []byte
		var enabled int
		if err := rows.Scan(&row.ID, &row.PlatformType, &row.DisplayName, &enabled, &ruleJSON, &row.Version, &row.Remark); err != nil {
			return list, err
		}
		row.Enabled = enabled == 1
		if err := json.Unmarshal(ruleJSON, &row.RuleConfig); err != nil {
			continue
		}
		list = append(list, row)
	}
	return list, rows.Err()
}

// getSubmitRuleCached 从 Redis 读取，未命中则查库并写入（无 TTL，永久有效）
func getSubmitRuleCached(ctx context.Context, platformType string) (*SubmitPlatformRow, error) {
	if rdb == nil {
		return loadSubmitRuleFromDB(ctx, platformType)
	}
	key := submitRuleCacheKey(platformType)
	val, err := rdb.Get(ctx, key).Result()
	if err == nil && val != "" {
		var row SubmitPlatformRow
		if json.Unmarshal([]byte(val), &row) == nil {
			return &row, nil
		}
	}
	row, err := loadSubmitRuleFromDB(ctx, platformType)
	if err != nil || row == nil {
		return row, err
	}
	if b, err := json.Marshal(row); err == nil {
		_ = rdb.Set(ctx, key, string(b), 0).Err()
	}
	return row, nil
}

func invalidateSubmitRuleCache(ctx context.Context, platformType string) {
	if rdb == nil {
		return
	}
	_ = rdb.Del(ctx, submitRuleCacheKey(platformType)).Err()
	_ = rdb.Del(ctx, submitRuleCacheAll).Err()
}

// refreshSubmitRuleCache 将单条规则写入 Redis（供管理端读取）
func refreshSubmitRuleCache(ctx context.Context, row *SubmitPlatformRow) {
	if rdb == nil || row == nil {
		return
	}
	if b, err := json.Marshal(row); err == nil {
		_ = rdb.Set(ctx, submitRuleCacheKey(row.PlatformType), string(b), 0).Err()
	}
}

func registerSubmitRuleInMemory(row *SubmitPlatformRow) {
	platformPluginsMu.Lock()
	defer platformPluginsMu.Unlock()
	if row != nil && row.Enabled {
		platformPlugins[row.PlatformType] = &dbRulePlatformPlugin{
			typ:  row.PlatformType,
			rule: row.RuleConfig,
		}
		return
	}
	if row != nil {
		delete(platformPlugins, row.PlatformType)
	}
}

func unregisterSubmitRuleInMemory(platformType string) {
	platformPluginsMu.Lock()
	delete(platformPlugins, platformType)
	platformPluginsMu.Unlock()
}

// syncSubmitRuleAfterChange 从库加载最新规则并同步 Redis + 内存下单插件表
func syncSubmitRuleAfterChange(ctx context.Context, platformType string) error {
	row, err := loadSubmitRuleFromDB(ctx, platformType)
	if err != nil {
		return err
	}
	if row == nil {
		removeSubmitRuleFromRuntime(ctx, platformType)
		return nil
	}
	refreshSubmitRuleCache(ctx, row)
	registerSubmitRuleInMemory(row)
	return nil
}

// removeSubmitRuleFromRuntime 删除规则的 Redis 缓存与内存注册
func removeSubmitRuleFromRuntime(ctx context.Context, platformType string) {
	invalidateSubmitRuleCache(ctx, platformType)
	unregisterSubmitRuleInMemory(platformType)
}

func invalidateAllSubmitRuleCache(ctx context.Context) {
	if rdb == nil {
		return
	}
	iter := rdb.Scan(ctx, 0, submitRuleCachePrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		_ = rdb.Del(ctx, iter.Val()).Err()
	}
	_ = rdb.Del(ctx, submitRuleCacheAll).Err()
}

func registerSubmitRulesFromDB(ctx context.Context) int {
	list, err := loadAllEnabledSubmitRulesFromDB(ctx)
	if err != nil {
		log.Printf("加载数据库提交规则失败: %v", err)
		return 0
	}
	count := 0
	platformPluginsMu.Lock()
	defer platformPluginsMu.Unlock()
	for _, row := range list {
		platformPlugins[row.PlatformType] = &dbRulePlatformPlugin{
			typ:  row.PlatformType,
			rule: row.RuleConfig,
		}
		count++
		if rdb != nil {
			if b, err := json.Marshal(row); err == nil {
				_ = rdb.Set(ctx, submitRuleCacheKey(row.PlatformType), string(b), 0).Err()
			}
		}
	}
	return count
}

func listSubmitPlatformsFromDB(ctx context.Context) ([]SubmitPlatformRow, error) {
	if pluginDB == nil {
		return nil, fmt.Errorf("插件数据库未连接")
	}
	q := fmt.Sprintf(`SELECT id, platform_type, display_name, enabled, rule_config, version, IFNULL(remark,'')
		FROM %s ORDER BY id ASC`, pluginTable("submit_platform"))
	rows, err := pluginDB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []SubmitPlatformRow
	for rows.Next() {
		var row SubmitPlatformRow
		var ruleJSON []byte
		var enabled int
		if err := rows.Scan(&row.ID, &row.PlatformType, &row.DisplayName, &enabled, &ruleJSON, &row.Version, &row.Remark); err != nil {
			return list, err
		}
		row.Enabled = enabled == 1
		_ = json.Unmarshal(ruleJSON, &row.RuleConfig)
		list = append(list, row)
	}
	return list, rows.Err()
}

func insertSubmitPlatform(ctx context.Context, row *SubmitPlatformRow) error {
	if pluginDB == nil {
		return fmt.Errorf("插件数据库未连接")
	}
	ruleJSON, err := json.Marshal(row.RuleConfig)
	if err != nil {
		return err
	}
	enabled := 0
	if row.Enabled {
		enabled = 1
	}
	q := fmt.Sprintf(`INSERT INTO %s (platform_type, display_name, enabled, rule_config, version, remark)
		VALUES (?,?,?,?,1,?)`, pluginTable("submit_platform"))
	_, err = pluginDB.ExecContext(ctx, q, row.PlatformType, row.DisplayName, enabled, ruleJSON, row.Remark)
	if err != nil {
		return err
	}
	return syncSubmitRuleAfterChange(ctx, row.PlatformType)
}

func updateSubmitPlatform(ctx context.Context, platformType string, row *SubmitPlatformRow) error {
	ruleJSON, err := json.Marshal(row.RuleConfig)
	if err != nil {
		return err
	}
	enabled := 0
	if row.Enabled {
		enabled = 1
	}
	q := fmt.Sprintf(`UPDATE %s SET display_name=?, enabled=?, rule_config=?, version=version+1, remark=?
		WHERE platform_type=? LIMIT 1`, pluginTable("submit_platform"))
	res, err := pluginDB.ExecContext(ctx, q, row.DisplayName, enabled, ruleJSON, row.Remark, platformType)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return syncSubmitRuleAfterChange(ctx, platformType)
}

func deleteSubmitPlatform(ctx context.Context, platformType string) error {
	q := fmt.Sprintf(`DELETE FROM %s WHERE platform_type=? LIMIT 1`, pluginTable("submit_platform"))
	res, err := pluginDB.ExecContext(ctx, q, platformType)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	removeSubmitRuleFromRuntime(ctx, platformType)
	return nil
}

func reloadSubmitRulesAndRegister(ctx context.Context) (int, error) {
	invalidateAllSubmitRuleCache(ctx)
	platformPluginsMu.Lock()
	platformPlugins = make(map[string]PlatformPlugin)
	platformPluginsMu.Unlock()
	n := registerSubmitRulesFromDB(ctx)
	return n, nil
}

// 供 API 序列化
type submitPlatformDTO struct {
	ID           int              `json:"id"`
	PlatformType string           `json:"platform_type"`
	DisplayName  string           `json:"display_name"`
	Enabled      bool             `json:"enabled"`
	RuleConfig   SubmitRuleConfig `json:"rule_config"`
	Version      int              `json:"version"`
	Remark       string           `json:"remark"`
	SourcePHP    *string          `json:"source_php,omitempty"`
}

func enrichSubmitPlatformDTO(ctx context.Context, dto *submitPlatformDTO) {
	if dto == nil || strings.TrimSpace(dto.PlatformType) == "" {
		return
	}
	php := loadPlatformSourcePHP(ctx, dto.PlatformType)
	dto.SourcePHP = &php
}

func persistSubmitPlatformSourcePHP(ctx context.Context, platformType string, sourcePHP *string) error {
	if sourcePHP == nil {
		return nil
	}
	return savePlatformSourcePHP(ctx, platformType, *sourcePHP)
}

func rowToDTO(row SubmitPlatformRow) submitPlatformDTO {
	return submitPlatformDTO{
		ID:           row.ID,
		PlatformType: row.PlatformType,
		DisplayName:  row.DisplayName,
		Enabled:      row.Enabled,
		RuleConfig:   row.RuleConfig,
		Version:      row.Version,
		Remark:       row.Remark,
	}
}

func parseEnabledBool(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return int(x) != 0
	case string:
		return x == "1" || strings.EqualFold(x, "true")
	default:
		return false
	}
}

func parseSubmitPlatformID(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}
