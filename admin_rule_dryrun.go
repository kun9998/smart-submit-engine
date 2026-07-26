package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ruleTestSubmitRequest struct {
	RuleConfig SubmitRuleConfig `json:"rule_config"`
	OID        string           `json:"oid,omitempty"`
}

type ruleTestSubmitResponse struct {
	HasOrders     bool   `json:"has_orders"`
	OrderCount    int    `json:"order_count"`
	OID           string `json:"oid,omitempty"`
	HID           int    `json:"hid,omitempty"`
	HuoyuanName   string `json:"huoyuan_name,omitempty"`
	PlatformMatch bool   `json:"platform_match"`
	Success       bool   `json:"success"`
	YID           string `json:"yid,omitempty"`
	ErrMsg        string `json:"err_msg,omitempty"`
	UpstreamBody  string `json:"upstream_body,omitempty"`
	Warning       string `json:"warning,omitempty"`
}

type ruleFixFromFailureRequest struct {
	RuleConfig   SubmitRuleConfig `json:"rule_config"`
	ErrMsg       string           `json:"err_msg"`
	UpstreamBody string           `json:"upstream_body,omitempty"`
	PHP          string           `json:"php,omitempty"`
}

func countOrdersForPlatform(ctx context.Context, platformType string) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("主库未连接")
	}
	platformType = strings.TrimSpace(platformType)
	if platformType == "" {
		return 0, fmt.Errorf("platform_type 为空")
	}
	q := fmt.Sprintf(`SELECT COUNT(*) FROM %s o INNER JOIN %s h ON o.hid = h.hid
		WHERE h.pt = ? AND IFNULL(o.status,'') != '已取消'`, tableName("order"), tableName("huoyuan"))
	var n int
	if err := db.QueryRowContext(ctx, q, platformType).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func findTestOrderForPlatform(ctx context.Context, platformType, oidHint string) (oid string, hid int, huoyuanName string, err error) {
	if db == nil {
		return "", 0, "", fmt.Errorf("主库未连接")
	}
	platformType = strings.TrimSpace(platformType)
	oidHint = strings.TrimSpace(oidHint)
	orderTable := tableName("order")
	huoyuanTable := tableName("huoyuan")

	if oidHint != "" {
		q := fmt.Sprintf(`SELECT CAST(o.oid AS CHAR), o.hid, IFNULL(h.name,''), h.pt
			FROM %s o INNER JOIN %s h ON o.hid = h.hid
			WHERE o.oid = ? AND IFNULL(o.status,'') != '已取消' LIMIT 1`, orderTable, huoyuanTable)
		var pt string
		if err := db.QueryRowContext(ctx, q, oidHint).Scan(&oid, &hid, &huoyuanName, &pt); err != nil {
			if err == sql.ErrNoRows {
				return "", 0, "", fmt.Errorf("订单 %s 不存在或已取消", oidHint)
			}
			return "", 0, "", err
		}
		if strings.TrimSpace(pt) != platformType {
			return oid, hid, huoyuanName, fmt.Errorf("订单 %s 所属平台为 %s，与当前规则 %s 不一致", oid, pt, platformType)
		}
		return oid, hid, huoyuanName, nil
	}

	q := fmt.Sprintf(`SELECT CAST(o.oid AS CHAR), o.hid, IFNULL(h.name,'')
		FROM %s o INNER JOIN %s h ON o.hid = h.hid
		WHERE h.pt = ? AND IFNULL(o.status,'') != '已取消'
		ORDER BY (o.dockstatus = '0') DESC, o.oid DESC
		LIMIT 1`, orderTable, huoyuanTable)
	if err := db.QueryRowContext(ctx, q, platformType).Scan(&oid, &hid, &huoyuanName); err != nil {
		if err == sql.ErrNoRows {
			return "", 0, "", nil
		}
		return "", 0, "", err
	}
	return oid, hid, huoyuanName, nil
}

func loadOrderAndHuoyuanForSubmit(ctx context.Context, oid int) (*Order, *Huoyuan, int, error) {
	if oid <= 0 {
		return nil, nil, 0, fmt.Errorf("无效的 oid: %d", oid)
	}
	normalizedOID, err := normalizeOID(strconv.Itoa(oid))
	if err != nil {
		return nil, nil, 0, fmt.Errorf("无效的订单ID: %v", err)
	}

	orderQuery := fmt.Sprintf(`SELECT oid, hid, user, pass, kcname, status, process, remarks, dockstatus, yid, school, noun, kcid FROM %s WHERE oid=? LIMIT 1`, tableName("order"))
	var order Order
	var noun, kcid sql.NullString
	err = db.QueryRowContext(ctx, orderQuery, normalizedOID).Scan(
		&order.OID, &order.HID, &order.User, &order.Pass, &order.KCName,
		&order.Status, &order.Process, &order.Remarks, &order.DockStatus,
		&order.YID, &order.School, &noun, &kcid,
	)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("查询订单失败: %v", err)
	}
	if noun.Valid {
		order.Noun = noun.String
	}
	if kcid.Valid {
		order.KCID = kcid.String
	}

	optionalFieldsQuery := fmt.Sprintf(`SELECT
		IFNULL(uTime, ''), IFNULL(uScore, ''), IFNULL(study_speed, ''),
		IFNULL(is_submit_exam, ''), IFNULL(exam_time, '')
		FROM %s WHERE oid=? LIMIT 1`, tableName("order"))
	var uTime, uScore, studySpeed, isSubmitExam, examTime sql.NullString
	if err := db.QueryRowContext(ctx, optionalFieldsQuery, normalizedOID).Scan(
		&uTime, &uScore, &studySpeed, &isSubmitExam, &examTime,
	); err == nil {
		if uTime.Valid {
			order.UTime = uTime.String
		}
		if uScore.Valid {
			order.UScore = uScore.String
		}
		if studySpeed.Valid {
			order.StudySpeed = studySpeed.String
		}
		if isSubmitExam.Valid {
			order.IsSubmitExam = isSubmitExam.String
		}
		if examTime.Valid {
			order.ExamTime = examTime.String
		}
	}

	var orderName sql.NullString
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT `name` FROM %s WHERE oid=? LIMIT 1", tableName("order")), normalizedOID).Scan(&orderName); err == nil && orderName.Valid {
		order.Name = strings.TrimSpace(orderName.String)
	}
	var orderIsCk sql.NullString
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT `isck` FROM %s WHERE oid=? LIMIT 1", tableName("order")), normalizedOID).Scan(&orderIsCk); err == nil && orderIsCk.Valid {
		order.IsCk = strings.TrimSpace(orderIsCk.String)
	}
	var simpleDay, simpleTotal, simpleLearn, ikunStudyIP sql.NullString
	optionalExtraQuery := fmt.Sprintf(`SELECT
		IFNULL(simple_day_score, ''), IFNULL(simple_total_score, ''),
		IFNULL(simple_learn_time, ''), IFNULL(ikun_study_ip, '')
		FROM %s WHERE oid=? LIMIT 1`, tableName("order"))
	if err := db.QueryRowContext(ctx, optionalExtraQuery, normalizedOID).Scan(
		&simpleDay, &simpleTotal, &simpleLearn, &ikunStudyIP,
	); err == nil {
		if simpleDay.Valid {
			order.SimpleDayScore = simpleDay.String
		}
		if simpleTotal.Valid {
			order.SimpleTotalScore = simpleTotal.String
		}
		if simpleLearn.Valid {
			order.SimpleLearnTime = simpleLearn.String
		}
		if ikunStudyIP.Valid {
			order.IkunStudyIP = ikunStudyIP.String
		}
	}
	var orderExpand sql.NullString
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT IFNULL(`expand`, '') FROM %s WHERE oid=? LIMIT 1", tableName("order")), normalizedOID).Scan(&orderExpand); err == nil && orderExpand.Valid {
		order.Expand = strings.TrimSpace(orderExpand.String)
	}

	hidNum := parseHIDString(order.HID)
	huoyuanQuery := fmt.Sprintf(`SELECT hid, pt, url, user, pass, token, cookie FROM %s WHERE hid=? LIMIT 1`, tableName("huoyuan"))
	var huoyuan Huoyuan
	var cookie sql.NullString
	err = db.QueryRowContext(ctx, huoyuanQuery, order.HID).Scan(
		&huoyuan.HID, &huoyuan.Type, &huoyuan.URL,
		&huoyuan.User, &huoyuan.Pass, &huoyuan.Token, &cookie,
	)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("查询货源信息失败: %v", err)
	}
	if cookie.Valid {
		huoyuan.Cookie = cookie.String
	}
	huoyuan.URL = normalizeHuoyuanURL(huoyuan.URL)
	if err := validateHuoyuanURLConfigured(huoyuan.URL); err != nil {
		return nil, nil, 0, err
	}
	return &order, &huoyuan, hidNum, nil
}

func dryRunSubmitWithRule(ctx context.Context, oid int, rule SubmitRuleConfig) (*AddOrderResult, *Huoyuan, error) {
	order, huoyuan, hidNum, err := loadOrderAndHuoyuanForSubmit(ctx, oid)
	if err != nil {
		return nil, nil, err
	}
	timeout := getSubmitTimeoutForHID(hidNum)
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	submitClient := NewOutboundHTTPClient(timeout)
	result, err := executeSubmitRule(ctx, order, huoyuan, submitClient, rule)
	if err != nil {
		return nil, huoyuan, err
	}
	return result, huoyuan, nil
}

func adminRuleTestSubmitHandler(w http.ResponseWriter, r *http.Request, platformType string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	platformType = strings.TrimSpace(platformType)
	if platformType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "platform_type 不能为空"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
		return
	}
	var req ruleTestSubmitRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	normalizeParsedRule(&req.RuleConfig)
	if !aiRuleHasActionableURL(req.RuleConfig) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "规则缺少 url"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	out := ruleTestSubmitResponse{}
	count, err := countOrdersForPlatform(ctx, platformType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	out.OrderCount = count
	out.HasOrders = count > 0
	if !out.HasOrders {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": 1,
			"msg":  "数据库中没有该平台的订单，无法试单",
			"data": out,
		})
		return
	}

	oidStr, hid, huoyuanName, err := findTestOrderForPlatform(ctx, platformType, req.OID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	if oidStr == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": 1,
			"msg":  "未找到可用试单订单",
			"data": out,
		})
		return
	}
	oidInt, err := strconv.Atoi(strings.TrimSpace(oidStr))
	if err != nil || oidInt <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "订单号无效"})
		return
	}

	out.OID = oidStr
	out.HID = hid
	out.HuoyuanName = huoyuanName
	out.Warning = "试单会真实请求上游接口，成功时可能在货源侧产生订单；不会修改本地订单状态"

	result, huoyuan, err := dryRunSubmitWithRule(ctx, oidInt, req.RuleConfig)
	if err != nil {
		out.Success = false
		out.ErrMsg = SanitizeUserVisibleError(err.Error())
		if huoyuan != nil {
			out.PlatformMatch = strings.TrimSpace(huoyuan.Type) == platformType
		}
		log.Printf("[规则试单] platform=%s oid=%s success=false pre_submit_err=%q", platformType, oidStr, out.ErrMsg)
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": out})
		return
	}
	out.PlatformMatch = huoyuan != nil && strings.TrimSpace(huoyuan.Type) == platformType
	if huoyuan != nil && !out.PlatformMatch {
		out.ErrMsg = fmt.Sprintf("货源平台类型为 %s，与规则 %s 不一致", huoyuan.Type, platformType)
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": out})
		return
	}

	out.Success = result != nil && result.Code == 1
	if out.Success {
		out.YID = result.YID
		out.ErrMsg = result.Msg
	} else if result != nil {
		out.ErrMsg = SanitizeUserVisibleError(result.Msg)
		out.UpstreamBody = truncateUpstreamForAutoFix(result.UpstreamBody)
	}
	log.Printf("[规则试单] platform=%s oid=%s success=%v msg=%q", platformType, oidStr, out.Success, out.ErrMsg)
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": out})
}

func adminRuleFixFromFailureHandler(w http.ResponseWriter, r *http.Request, platformType string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	if !aiConfigReady() {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"code": -1,
			"msg":  "请先在「系统设置 → AI 能力」中启用并配置 API Key",
		})
		return
	}
	platformType = strings.TrimSpace(platformType)
	if platformType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "platform_type 不能为空"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
		return
	}
	var req ruleFixFromFailureRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
		return
	}
	if strings.TrimSpace(req.ErrMsg) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "请提供试单失败信息 err_msg"})
		return
	}
	normalizeParsedRule(&req.RuleConfig)
	if !aiRuleHasActionableURL(req.RuleConfig) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "规则缺少 url"})
		return
	}

	rateKey := sessionUserFromRequest(r.Context(), r)
	if rateKey == "" {
		rateKey = strings.TrimSpace(r.RemoteAddr)
	}
	if !allowAIConvert(rateKey) {
		writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"code": -1,
			"msg":  "AI 请求过于频繁，请稍后再试",
		})
		return
	}

	php := strings.TrimSpace(req.PHP)
	if php == "" {
		php = loadPlatformSourcePHP(r.Context(), platformType)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	out, err := fixRuleFromSubmitFailure(ctx, platformType, req.RuleConfig, req.ErrMsg, req.UpstreamBody, php)
	if err != nil {
		log.Printf("[规则试单修复] platform=%s err=%v", platformType, err)
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": out})
}
