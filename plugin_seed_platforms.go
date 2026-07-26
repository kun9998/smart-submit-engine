package main

// pluginSeedPlatforms 安装时写入插件库的默认 JSON 提交规则（INSERT IGNORE，不覆盖已有配置）
// 仅预置 27 / 29 / 2xx / hzw 四种；其余平台请在管理端手动配置 rule_config。
func pluginSeedPlatforms() []SubmitPlatformRow {
	stdBody := map[string]string{
		"uid": "{{huoyuan.user}}", "key": "{{huoyuan.pass}}",
		"platform": "{{order.noun}}", "school": "{{order.school}}",
		"user": "{{order.user}}", "pass": "{{order.pass}}",
		"kcname": "{{order.kcname}}", "kcid": "{{order.kcid}}",
	}
	stdResp := SubmitRuleResp{
		CodeField: "code", SuccessCodes: []interface{}{"0", 0},
		MsgField: "msg", SuccessMsg: "下单成功",
	}

	return []SubmitPlatformRow{
		{
			PlatformType: "27", DisplayName: "27系统", Enabled: true, Remark: "标准API",
			RuleConfig: SubmitRuleConfig{
				Method: "POST", URL: "{{huoyuan.url}}/api.php?act=add",
				ContentType: "form", UseCookie: true, Body: stdBody, Response: stdResp,
			},
		},
		{
			PlatformType: "29", DisplayName: "29系统", Enabled: true, Remark: "标准API",
			RuleConfig: SubmitRuleConfig{
				Method: "POST", URL: "{{huoyuan.url}}/api.php?act=add",
				ContentType: "form", Body: stdBody, Response: stdResp,
			},
		},
		{
			PlatformType: "2xx", DisplayName: "爱学习", Enabled: true, Remark: "爱学习JSON",
			RuleConfig: SubmitRuleConfig{
				Method: "POST", URL: "{{huoyuan.url}}/api/add", ContentType: "json",
				Body: map[string]string{
					"token": "{{huoyuan.pass}}", "platform": "{{order.noun}}",
					"school": "{{order.school}}", "user": "{{order.user}}", "pass": "{{order.pass}}",
					"kcname": "{{order.kcname}}", "kcid": "{{order.kcid}}",
					"time": "{{order.uTime}}", "score": "{{order.uScore}}",
					"speed": "{{order.study_speed}}", "exam_submit": "{{order.is_submit_exam}}",
					"exam_time": "{{order.exam_time}}",
				},
				Response: SubmitRuleResp{
					CodeField: "code", SuccessCodes: []interface{}{"1", 1},
					MsgField: "msg", YIDField: "id", SuccessMsg: "下单成功",
				},
			},
		},
		{
			PlatformType: "hzw", DisplayName: "hzw", Enabled: true, Remark: "标准API success=1",
			RuleConfig: SubmitRuleConfig{
				Method: "POST", URL: "{{huoyuan.url}}/api.php?act=add",
				ContentType: "form", Body: stdBody,
				Response: SubmitRuleResp{
					CodeField: "code", SuccessCodes: []interface{}{"1", 1},
					MsgField: "msg", YIDField: "id", SuccessMsg: "下单成功",
				},
			},
		},
	}
}
