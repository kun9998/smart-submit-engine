package main

import (
	"strconv"
	"strings"
)

func parseXdjkDf1(platformType string, specialNotes []string) *xdjkParseResult {
	return parseXdjkDf(platformType, 0, specialNotes)
}

func parseXdjkDf2(platformType string, specialNotes []string) *xdjkParseResult {
	return parseXdjkDf(platformType, 1, specialNotes)
}

func parseXdjkDf(platformType string, test int, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/df/api/xd",
			ContentType: "json",
			Headers: map[string]string{
				"Authorization": "DfAi {{huoyuan.token}}",
				"Content-Type":  "application/json;charset=UTF-8",
			},
			BodyMode: "raw",
			BodyRaw:  `[{"num":"{{order.user}}","pwd":"{{order.pass}}","name":"","mark":"平台名称","mode":"1","test":` + strconv.Itoa(test) + `,"list":[{"code":"{{order.kcid}}","name":"{{order.kcname}}"}]}]`,
			Body:     map[string]string{},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{200, "200"},
				MsgField:     "msg",
				SuccessMsg:   "下单成功",
			},
		},
		Warnings:     []string{"嵌套 JSON 数组 body_raw；df1 test=0，df2 test=1，需分两条平台配置"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkSimple(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/Api/Create",
			ContentType: "form",
			Body: map[string]string{
				"token":    "{{huoyuan.token}}",
				"platform": `{{concat order.noun "|score=" order.uScore ";sc=" order.uTime "|day_score=" order.simple_day_score ";total_score=" order.simple_total_score ";learntime=" order.simple_learn_time}}`,
				"school":   "{{order.school}}",
				"user":     "{{order.user}}",
				"pass":     "{{order.pass}}",
				"course":   "{{order.kcname}}",
				"courseid": "{{order.kcid}}",
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"1", 1},
				MsgField:     "msg",
				SuccessMsg:   "添加成功",
			},
		},
		Warnings: []string{
			"platform 用 {{concat}} 拼接积分/时长字段",
			"PHP 中 school|考试码、school|日上限|累计上限 等改写 noun 的逻辑较复杂，若对接异常请用 branches 拆分",
		},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkWuming(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/demo/submitOrder",
			ContentType: "json",
			Headers: map[string]string{
				"Content-Type":       "application/json",
				"X-Requested-With":   "XMLHttpRequest",
				"token":              "{{huoyuan.token}}",
			},
			BodyMode: "raw",
			BodyRaw:  `{"platformId":"{{order.noun}}","school":"{{order.school}}","account":"{{order.user}}","password":"{{order.pass}}","duration":"无","score":"无","courseInfo":[{"courseName":"{{order.kcname}}","courseId":"{{order.kcid}}","unitList":[]}]}`,
			Body:     map[string]string{},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"1", 1},
				MsgField:     "msg",
				YIDPath:      "data.id",
				SuccessMsg:   "下单成功",
			},
		},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkDlam(platformType string, specialNotes []string) *xdjkParseResult {
	dlamBody := func(operate string) string {
		return `{"shopcode":"{{order.noun}}","school":"{{order.school}}","username":"{{order.user}}","password":"{{order.pass}}","operate":"` + operate + `","course":[{"title":"{{order.kcname}}","id":"{{order.kcid}}"}]}`
	}
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/prod-api/wk/order/xiadan",
			ContentType: "json",
			Headers: map[string]string{
				"Content-Type":  "application/json;charset=UTF-8",
				"Authorization": "{{huoyuan.token}}",
			},
			BodyMode: "raw",
			Body:     map[string]string{},
			Branches: []SubmitRuleBranch{
				{When: &SubmitRuleWhen{Field: "order.noun", Equals: "xgk"}, BodyMode: "raw", BodyRaw: dlamBody("课件+讨论+作业+文件题+终考+保留答题机会")},
				{When: &SubmitRuleWhen{Field: "order.noun", Equals: "xgkplus"}, BodyMode: "raw", BodyRaw: dlamBody("课件+讨论+作业+文件题+终考+保留答题机会")},
				{When: &SubmitRuleWhen{Field: "order.noun", Equals: "yth"}, BodyMode: "raw", BodyRaw: dlamBody("视频+测验+作业+考试")},
				{When: &SubmitRuleWhen{Field: "order.noun", Equals: "qsxt"}, BodyMode: "raw", BodyRaw: dlamBody("视频+作业+考试+讨论+登录次数+提交简答题")},
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{200, "200"},
				MsgField:     "msg",
				SuccessUseUpstreamMsg: true,
			},
		},
		Warnings:     []string{"仅支持 xgk/xgkplus/yth/qsxt 四种 noun；其他 noun 需补 branches"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkKUN(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "GET",
			URL:         `{{huoyuan.url}}:{{random_port}}/getorder/?platform={{urlencode order.noun}}&school={{urlencode order.school}}&account={{order.user}}&password={{order.pass}}&course={{urlencode order.kcname}}&kcid={{order.kcid}}`,
			ContentType: "form",
			URLPortPool: []int{1234, 1235, 1236, 1237, 1238},
			Body:        map[string]string{},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"1", 1},
				MsgField:     "msg",
				YIDField:     "order_token",
				SuccessMsg:   "下单成功",
			},
		},
		Warnings:     []string{"url_port_pool 替代 PHP array_rand 随机端口"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkKunba(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method: "GET",
			URL:    `{{huoyuan.url}}/getorder4/?platform={{urlencode order.noun}}&school={{urlencode order.school}}&account={{order.user}}&password={{order.pass}}&course={{urlencode order.kcname}}&kcid={{order.kcid}}&proxy={{order.ikun_study_ip}}&dtoken={{huoyuan.token}}`,
			ContentType: "form",
			Body:        map[string]string{},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{1, "1"},
				MsgField:     "msg",
				YIDField:     "order_token",
				SuccessMsg:   "已添加至服务器，开始执行刷课！",
			},
		},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkLotus(platformType, code string, specialNotes []string) *xdjkParseResult {
	warnings := []string{
		"固定第三方域名 text.boox.top，非货源 url",
		"下单成功后 PHP 会 UPDATE 主库 remarks，规则引擎不会写库，请主站侧处理跳转链接",
	}
	if strings.Contains(code, "$DB") && strings.Contains(code, "->query") {
		warnings = append(warnings, "已识别写库逻辑，仅转换提交 HTTP 部分")
	}
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "http://text.boox.top/api/aipaper_outside_main/ThirdParty/order-create?us={{huoyuan.user}}&pw={{huoyuan.pass}}",
			ContentType: "json",
			Headers: map[string]string{
				"Accept":               "application/json, text/plain, */*",
				"Content-Type":         "application/json;charset=utf-8",
				"third-party-identity": "{{huoyuan.token}}",
			},
			BodyMode: "raw",
			BodyRaw:  `{"Prcies":"{{order.noun}}","ThirdPartyId":"{{order.oid}}"}`,
			Body:     map[string]string{},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{0, "0"},
				MsgField:     "message",
				SuccessMsg:   "下单成功",
			},
		},
		Warnings:     warnings,
		SpecialNotes: specialNotes,
	}
}

func parseXdjkHuoxi(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/addOrder",
			ContentType: "json",
			Headers: map[string]string{
				"Content-Type": "application/json;charset=utf-8",
				"Token":        "{{huoyuan.token}}",
			},
			BodyMode: "raw",
			BodyRaw:  `[{"account":"{{order.user}}","password":"{{order.pass}}","goodId":"{{order.noun}}","courseName":"{{order.kcname}}","tag":1}]`,
			Body:     map[string]string{},
			Response: SubmitRuleResp{
				CodeField:    "status",
				SuccessCodes: []interface{}{"200", 200},
				MsgField:     "message",
				SuccessMsg:   "下单成功",
			},
		},
		SpecialNotes: specialNotes,
	}
}
