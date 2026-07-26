package main

func parseXdjkLonglongKnown(platformType string, specialNotes []string) *xdjkParseResult {
	res := parseXdjkLongLongV2("llv2_submit", platformType)
	if res != nil {
		res.SpecialNotes = specialNotes
		return res
	}
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig:      buildXdjkLonglongRuleConfig(),
		Warnings:        []string{"expand 字段取 order.expand JSON；school=自动识别 时上游通常忽略空 school"},
		SpecialNotes:    specialNotes,
	}
}

func parseXdjkGoStudy(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/open/submitCourse",
			ContentType: "json",
			Headers: map[string]string{
				"token":        "{{huoyuan.token}}",
				"Content-Type": "application/json;charset=UTF-8",
			},
			BodyMode: "raw",
			BodyRaw:  `[{"platformId":"{{order.noun}}","studentName":"{{order.name}}","school":"{{order.school}}","account":"{{order.user}}","password":"{{order.pass}}","code":"{{order.kcid}}","name":"{{order.kcname}}"}]`,
			Body:     map[string]string{},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{0, "0"},
				MsgField:     "msg",
				YIDPath:      "data.0",
				SuccessMsg:   "已添加至服务器，开始执行刷课！",
			},
		},
		Warnings:     []string{"studentName 取 order.name 字段"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkJxjyyjy(platformType string, specialNotes []string) *xdjkParseResult {
	bodyBase := `{"websiteNumber":"{{order.noun}}","data":[{"username":"{{order.user}}","password":"{{order.pass}}","children":`
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         joinHuoyuanURLTemplate("api/order/buy"),
			ContentType: "json",
			Headers: map[string]string{
				"User-Agent":    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
				"Content-Type":  "application/json; charset=utf-8",
				"Authorization": "Bearer {{huoyuan.token}}",
			},
			BodyMode: "raw",
			Body:     map[string]string{},
			Branches: []SubmitRuleBranch{
				{
					When:     &SubmitRuleWhen{Field: "order.isck", Equals: "0"},
					BodyMode: "raw",
					BodyRaw:  bodyBase + `[{"name":""}]}]}`,
				},
				{
					When:     &SubmitRuleWhen{Default: true},
					BodyMode: "raw",
					BodyRaw:  bodyBase + `[{"name":"{{order.kcname}}"}]}]}`,
				},
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{200, "200"},
				MsgField:     "message",
				YIDPath:      "data.orderList.0.orderId",
				SuccessMsg:   "下单成功",
			},
		},
		Warnings:     []string{"isck=0 时 children.name 为空字符串"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkLangr(platformType string, specialNotes []string) *xdjkParseResult {
	return parseXdjkActAddExtra(platformType, "/api1.php?act=add", specialNotes, map[string]string{
		"shichang": "{{order.uTime}}",
		"score":    "{{order.uScore}}",
	})
}

func parseXdjkYqsl(platformType string, specialNotes []string) *xdjkParseResult {
	return parseXdjkActAddExtra(platformType, "/api.php?act=addyqsl", specialNotes, map[string]string{
		"score":    "{{order.uScore}}",
		"shichang": "{{order.uTime}}",
	})
}

func parseXdjkActAddExtra(platformType, actPath string, specialNotes []string, extra map[string]string) *xdjkParseResult {
	body := map[string]string{
		"uid":      "{{huoyuan.user}}",
		"key":      "{{huoyuan.pass}}",
		"platform": "{{order.noun}}",
		"school":   "{{order.school}}",
		"user":     "{{order.user}}",
		"pass":     "{{order.pass}}",
		"kcname":   "{{order.kcname}}",
		"kcid":     "{{order.kcid}}",
	}
	for k, v := range extra {
		body[k] = v
	}
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}" + actPath,
			ContentType: "form",
			Body:        body,
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"0", 0},
				MsgField:     "msg",
				SuccessMsg:   "下单成功",
			},
		},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkAlgk(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "GET",
			URL:         `{{huoyuan.url}}/api/Open/AddRenWu?username={{huoyuan.user}}&password={{huoyuan.pass}}&zhanghao={{order.user}}&psd={{urlencode order.pass}}&kcname={{urlencode order.kcname}}`,
			ContentType: "form",
			UseCookie:   true,
			Body:        map[string]string{},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"200", 200},
				MsgField:     "msg",
				SuccessMsg:   "下单成功",
			},
		},
		Warnings:     []string{"GET 查询串，kcname/pass 需 urlencode"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkAlgksy(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "GET",
			URL:         `{{huoyuan.url}}/api/ShiYanOpen/AddRenWu?username={{huoyuan.user}}&password={{huoyuan.pass}}&zhanghao={{order.user}}&psd={{urlencode order.pass}}&kcname={{urlencode order.kcname}}`,
			ContentType: "form",
			Body:        map[string]string{},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"200", 200},
				MsgField:     "msg",
				SuccessMsg:   "下单成功",
			},
		},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkTesla(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/api/external/submit-order",
			ContentType: "form",
			UseCookie:   true,
			Body: map[string]string{
				"uid":    "{{huoyuan.user}}",
				"key":    "{{huoyuan.pass}}",
				"cid":    "{{order.noun}}",
				"school": "{{order.school}}",
				"user":   "{{order.user}}",
				"pass":   "{{order.pass}}",
				"kcname": "{{order.kcname}}",
				"kcid":   "{{order.kcid}}",
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"0", 0},
				MsgField:     "msg",
				YIDField:     "id",
				SuccessMsg:   "下单成功",
			},
		},
		Warnings:     []string{"platform 字段在 PHP 中为 cid"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkTHOTH(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/open/add",
			ContentType: "form",
			Headers: map[string]string{
				"X-Uid":     "{{huoyuan.user}}",
				"X-Api-Key": "{{huoyuan.pass}}",
			},
			Body: map[string]string{
				"platform":   "{{order.noun}}",
				"school":     "{{order.school}}",
				"user":       "{{order.user}}",
				"pass":       "{{order.pass}}",
				"name":       "{{order.name}}",
				"kcname":     "[\"{{order.kcname}}\"]",
				"kcid":       "[\"{{order.kcid}}\"]",
				"score":      "{{order.uScore}}",
				"shichang":   "{{order.uTime}}",
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"0", 0},
				MsgField:     "msg",
				YIDField:     "id",
				SuccessMsg:   "下单成功",
			},
		},
		Warnings:     []string{"鉴权在 Header X-Uid/X-Api-Key，勿在 body 传 uid/key；kcname/kcid 为 JSON 字符串数组"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkCoco(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/useAPI/addOrder",
			ContentType: "json",
			Body: map[string]string{
				"uid":      "{{huoyuan.user}}",
				"key":      "{{huoyuan.pass}}",
				"platform": "{{order.noun}}",
				"school":   "{{order.school}}",
				"user":     "{{order.user}}",
				"pass":     "{{order.pass}}",
				"kcname":   "{{order.kcname}}",
				"kcid":     "{{order.kcid}}",
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{1, "1"},
				MsgField:     "msg",
				SuccessMsg:   "下单成功",
			},
		},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkNx(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/v1/order/submit",
			ContentType: "form",
			Body: map[string]string{
				"token":      "{{huoyuan.token}}",
				"school":     "{{order.school}}",
				"account":    "{{order.user}}",
				"password":   "{{order.pass}}",
				"coursename": "{{order.kcname}}",
				"value":      "{{order.noun}}",
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{1, "1"},
				MsgField:     "msg",
				SuccessMsg:   "下单成功",
				FailureMsgRules: []FailureMsgRule{
					{Contains: "重复提交", Msg: "已获取最新订单,等待进度同步"},
					{Contains: "Repeated", Msg: "已获取最新订单,等待进度同步"},
					{Contains: "积分不足", Msg: "学时不足,请联系上级！"},
					{Contains: "Insufficient", Msg: "学时不足,请联系上级！"},
					{Contains: "不能为空", Msg: "参数提交不完整"},
					{Contains: "cannot be empty", Msg: "参数提交不完整"},
				},
			},
		},
		Warnings:     []string{"failure_msg_rules 映射 PHP strpos 改写文案"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjk00(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "GET",
			URL:         "{{huoyuan.url}}/submit",
			ContentType: "form",
			Body: map[string]string{
				"school":     "{{order.school}}",
				"account":    "{{order.user}}",
				"password":   "{{order.pass}}",
				"coursename": "{{order.kcname}}",
				"value":      "{{order.noun}}",
				"token":      "{{huoyuan.token}}",
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{0, "0"},
				MsgField:     "message",
				SuccessMsg:   "添加成功",
			},
		},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkYumeng(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/addOrder",
			ContentType: "form",
			Body: map[string]string{
				"token":    "{{huoyuan.token}}",
				"platform": "{{order.noun}}",
				"school":   "{{order.school}}",
				"user":     "{{order.user}}",
				"pass":     "{{order.pass}}",
				"kcname":   "{{order.kcname}}",
				"kcid":     "{{order.kcid}}",
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"1", 1},
				MsgField:     "msg",
				YIDField:     "id",
				SuccessMsg:   "下单成功",
			},
		},
		SpecialNotes: specialNotes,
	}
}
