package main

func parseXdjk27(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api.php?act=add",
			ContentType: "form",
			UseCookie:   true,
			Body: map[string]string{
				"uid":      "{{huoyuan.user}}",
				"key":      "{{huoyuan.pass}}",
				"platform": "{{order.noun}}",
				"school":   "{{order.school}}",
				"user":     "{{order.user}}",
				"pass":     "{{order.pass}}",
				"kcname":   "{{order.kcname}}",
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"0", 0},
				MsgField:     "msg",
				SuccessMsg:   "下单成功",
			},
		},
		Warnings:     []string{"无 kcid 字段；需 use_cookie"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjk2xx(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/add",
			ContentType: "json",
			Body: map[string]string{
				"token":        "{{huoyuan.pass}}",
				"platform":     "{{order.noun}}",
				"school":       "{{order.school}}",
				"user":         "{{order.user}}",
				"pass":         "{{order.pass}}",
				"kcname":       "{{order.kcname}}",
				"kcid":         "{{order.kcid}}",
				"time":         "{{order.uTime}}",
				"score":        "{{order.uScore}}",
				"speed":        "{{order.study_speed}}",
				"exam_submit":  "{{order.is_submit_exam}}",
				"exam_time":    "{{order.exam_time}}",
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"1", 1},
				MsgField:     "msg",
				YIDField:     "id",
				SuccessMsg:   "下单成功",
			},
		},
		Warnings:     []string{"token 取 huoyuan.pass；需订单 study_speed/is_submit_exam/exam_time"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkBenz(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/add",
			ContentType: "form",
			UseCookie:   true,
			Body: map[string]string{
				"token":    "{{huoyuan.token}}",
				"ptid":     "{{order.noun}}",
				"school":   "{{order.school}}",
				"user":     "{{order.user}}",
				"pass":     "{{order.pass}}",
				"kcname":   "{{order.kcname}}",
				"kcid":     "{{order.kcid}}",
				"shichang": "{{order.uTime}}",
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"0", 0},
				MsgField:     "msg",
				SuccessMsg:   "下单成功",
			},
		},
		Warnings:     []string{"platform 在 PHP 中为 ptid；需 use_cookie"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkBld(platformType string, specialNotes []string) *xdjkParseResult {
	return parseXdjkActAddExtra(platformType, "/api.php?act=addcf", specialNotes, nil)
}

func parseXdjkHzw(platformType string, specialNotes []string) *xdjkParseResult {
	return parseXdjkActAddWithSuccess(platformType, "/api.php?act=add", specialNotes, nil, false,
		[]interface{}{"1", 1}, "id")
}

func parseXdjkZfb(platformType string, specialNotes []string) *xdjkParseResult {
	return parseXdjkActAddWithSuccess(platformType, "/api.php?act=add", specialNotes, nil, false,
		[]interface{}{"0", 0}, "id")
}

func parseXdjkDuowei(platformType string, specialNotes []string) *xdjkParseResult {
	return parseXdjkActAddWithSuccess(platformType, "/api.php?act=add", specialNotes, nil, false,
		[]interface{}{"0", 0}, "id")
}

func parseXdjkWanzi(platformType string, specialNotes []string) *xdjkParseResult {
	return parseXdjkActAddWithSuccess(platformType, "/api.php?act=add", specialNotes, nil, false,
		[]interface{}{"0", 0}, "id")
}

func parseXdjkXm(platformType string, specialNotes []string) *xdjkParseResult {
	return parseXdjkActAddWithSuccess(platformType, "/api.php?act=add", specialNotes, nil, true,
		[]interface{}{"0", 0}, "id")
}

func parseXdjkHb(platformType string, specialNotes []string) *xdjkParseResult {
	return parseXdjkActAddWithSuccess(platformType, "/api.php?act=add", specialNotes, nil, false,
		[]interface{}{"0", 0}, "id")
}

func parseXdjkActAddWithSuccess(platformType, actPath string, specialNotes []string, extra map[string]string, useCookie bool, successCodes []interface{}, yidField string) *xdjkParseResult {
	res := parseXdjkActAddExtra(platformType, actPath, specialNotes, extra)
	res.RuleConfig.UseCookie = useCookie
	res.RuleConfig.Response.SuccessCodes = successCodes
	if yidField != "" {
		res.RuleConfig.Response.YIDField = yidField
	}
	return res
}
