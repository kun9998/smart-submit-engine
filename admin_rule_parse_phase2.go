package main

import "strings"

func kcidPatchBranch(noun string, checkedList []string) SubmitRuleBranch {
	items := make([]interface{}, len(checkedList))
	for i, s := range checkedList {
		items[i] = s
	}
	return SubmitRuleBranch{
		When: &SubmitRuleWhen{Field: "order.noun", Equals: noun},
		KcidJSONPatches: []KcidJSONPatch{{
			Set: map[string]interface{}{
				"task_list.0.config.checked_config_list": items,
			},
		}},
	}
}

func kcidDefaultBranch() SubmitRuleBranch {
	return SubmitRuleBranch{
		When:            &SubmitRuleWhen{Default: true},
		KcidJSONPatches: []KcidJSONPatch{},
	}
}

func kdbRuleBase(apiPath string, branches []SubmitRuleBranch) SubmitRuleConfig {
	return SubmitRuleConfig{
		Method:      "POST",
		URL:         "{{huoyuan.url}}" + apiPath,
		ContentType: "json",
		BodyMode:    "kcid_json",
		Headers: map[string]string{
			"Authorization": "Bearer {{huoyuan.token}}",
			"Content-Type":  "application/json",
		},
		Body: map[string]string{},
		KcidJSONValidate: &KcidJSONValidate{
			Path:  "task_list",
			Exact: 1,
		},
		Branches: branches,
		Response: SubmitRuleResp{
			CodeField:    "code",
			SuccessCodes: []interface{}{"1", 1},
			MsgField:     "msg",
		},
	}
}

func parseXdjkKdbxxt(platformType string, specialNotes []string) *xdjkParseResult {
	branches := []SubmitRuleBranch{
		kcidPatchBranch("秒刷不清", []string{"视频", "作业", "考试", "章测", "直播", "阅读", "秒刷不清"}),
		kcidPatchBranch("单考试", []string{"考试"}),
		kcidPatchBranch("单任务点", []string{"视频", "章测", "直播", "阅读"}),
		kcidPatchBranch("章测收录", []string{"收录"}),
		kcidPatchBranch("考试收录", []string{"考试收录"}),
		kcidPatchBranch("秒刷", []string{"视频", "作业", "考试", "章测", "直播", "阅读", "秒刷"}),
		kcidDefaultBranch(),
	}
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig:      kdbRuleBase("/api/xxt/xxt_task/", branches),
		Warnings:          []string{"kcid_json：order.kcid 为 base64 JSON，按 order.noun 打补丁"},
		SpecialNotes:      specialNotes,
	}
}

func parseXdjkKdbzhs(platformType string, specialNotes []string) *xdjkParseResult {
	branches := []SubmitRuleBranch{
		kcidPatchBranch("秒刷", []string{"视频", "作业", "考试", "习惯", "见面", "互动", "秒刷"}),
		kcidPatchBranch("单补互动", []string{"互动"}),
		kcidPatchBranch("单考试", []string{"考试"}),
		kcidDefaultBranch(),
	}
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig:      kdbRuleBase("/api/zd/zd_task/", branches),
		Warnings:          []string{"kcid_json：智慧树课代表，按 noun 补丁 checked_config_list"},
		SpecialNotes:      specialNotes,
	}
}

func parseXdjkKdbzhzj(platformType string, specialNotes []string) *xdjkParseResult {
	branches := []SubmitRuleBranch{
		kcidPatchBranch("单课件", []string{"视频", "文档", "讨论"}),
		kcidPatchBranch("慢刷", []string{"视频", "文档", "作业", "考试", "讨论", "慢刷"}),
		kcidPatchBranch("单做题", []string{"作业", "考试"}),
		kcidPatchBranch("收录", []string{"收录"}),
		kcidPatchBranch("补时长", []string{"视频", "补时长"}),
		kcidPatchBranch("复习模式", []string{"视频", "文档", "作业", "考试", "讨论", "慢刷", "补时长"}),
		kcidDefaultBranch(),
	}
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig:      kdbRuleBase("/api/zj/zj_task/", branches),
		Warnings:          []string{"kcid_json：智慧职教课代表，按 noun 补丁 checked_config_list"},
		SpecialNotes:      specialNotes,
	}
}

func parseXdjkMaliaorun(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/gk/order",
			ContentType: "json",
			Headers: map[string]string{
				"Authorization": "Bearer {{huoyuan.token}}",
				"Content-Type":  "application/json",
			},
			BodyMode: "raw",
			Body:     map[string]string{},
			Branches: []SubmitRuleBranch{
				{
					When:    &SubmitRuleWhen{Field: "order.noun", Equals: "1"},
					BodyRaw: `{"xh":"{{order.user}}","password":"{{order.pass}}","courses":["{{order.kcname}}"],"bz":"","config":{"openLog":true,"openExam":true,"openHomeWork":true,"openRead":true,"forceRun":false},"learnTimes":"500","learnDays":"5"}`,
				},
				{
					When:    &SubmitRuleWhen{Field: "order.noun", Equals: "2"},
					BodyRaw: `{"xh":"{{order.user}}","password":"{{order.pass}}","courses":["{{order.kcname}}"],"bz":"","config":{"openLog":true,"openExam":true,"openHomeWork":true,"openRead":true,"forceRun":true},"learnTimes":"500","learnDays":"5"}`,
				},
				{
					When:    &SubmitRuleWhen{Default: true},
					BodyRaw: `{"xh":"{{order.user}}","password":"{{order.pass}}","courses":["{{order.kcname}}"],"bz":"","config":{"openLog":true,"openExam":true,"openHomeWork":true,"openRead":true,"forceRun":false},"learnTimes":"800","learnDays":"15"}`,
				},
			},
			Response: SubmitRuleResp{
				CodeField:    "msg",
				SuccessCodes: []interface{}{"所有课程下单成功"},
				MsgField:     "msg",
			},
		},
		Warnings:     []string{"按 order.noun=1/2/其他 三套 body_raw；成功看 msg 而非 code"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkLyyjxjy(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			ContentType: "json",
			UseCookie:   true,
			DelayMS:     5000,
			Body:        map[string]string{},
			Branches: []SubmitRuleBranch{
				{
					When:    &SubmitRuleWhen{Field: "order.kcname", NotContains: "随机课程"},
					URL:     "{{huoyuan.url}}/system/sysOrder/addOrder",
					BodyMode: "raw",
					BodyRaw:  `[{"account":"{{order.user}}","password":"{{order.pass}}","courseId":"{{order.kcid}}","courseName":"{{order.kcname}}","platformId":"{{order.noun}}"}]`,
				},
				{
					When: &SubmitRuleWhen{
						All: []SubmitRuleWhen{
							{Field: "order.kcname", Contains: "随机课程"},
							{Field: "order.school", NotContains: "自动识别"},
						},
					},
					URL:         "{{huoyuan.url}}/lib/add_u.php",
					ContentType: "form",
					BodyMode:    "raw",
					BodyRaw:     "platform={{order.noun}}&c_data={{order.school}}----{{order.user}}----{{order.pass}}%0A&",
				},
				{
					When:        &SubmitRuleWhen{Default: true},
					URL:         "{{huoyuan.url}}/lib/add_u.php",
					ContentType: "form",
					BodyMode:    "raw",
					BodyRaw:     "platform={{order.noun}}&c_data={{order.user}}----{{order.pass}}%0A&",
				},
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{0, "0"},
				MsgField:     "msg",
			},
		},
		Warnings:     []string{"delay_ms=5000 对应 PHP sleep(5)；随机课程走 form raw 分支"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjk8090WithBranches(platformType, code string, specialNotes []string) *xdjkParseResult {
	warnings := []string{"isck=0 时不传课名（selectedCourseKeys 为空数组）"}
	if strings.Contains(code, "$DB") && strings.Contains(code, "->query") {
		warnings = append(warnings, "含 UPDATE 数据库逻辑，规则引擎不会写库")
	}
	bodyBase := `{"websiteId":"{{order.noun}}","accountInfo":"{{order.user}} {{order.pass}}"`
	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/order/submit",
			ContentType: "json",
			Headers: map[string]string{
				"Content-Type":  "application/json; charset=utf-8",
				"Authorization": "Bearer {{huoyuan.token}}",
			},
			BodyMode: "raw",
			Body:     map[string]string{},
			Branches: []SubmitRuleBranch{
				{
					When:     &SubmitRuleWhen{Field: "order.isck", Equals: "0"},
					BodyMode: "raw",
					BodyRaw:  bodyBase + `,"selectedCourseKeys":[]}`,
				},
				{
					When:     &SubmitRuleWhen{Default: true},
					BodyMode: "raw",
					BodyRaw:  bodyBase + `,"selectedCourseKeys":["{{order.kcname}}"]}`,
				},
			},
			Response: SubmitRuleResp{
				CodeField:             "code",
				SuccessCodes:          []interface{}{200, "200"},
				MsgField:              "message",
				YIDField:              "data",
				SuccessUseUpstreamMsg: true,
				FailureMsgOnSuccess:   true,
				FailureMsgRules:       []FailureMsgRule{{Contains: "失败"}},
			},
		},
		Warnings:     warnings,
		SpecialNotes: specialNotes,
	}
}
