package main

import "strings"

const xdjkLonglongBodyRaw = `{"username":"{{order.user}}","password":"{{order.pass}}","courses":["{{order.kcid}}"],"school":"{{order.school}}","name":"{{order.name}}","city":"{{order.expand.city}}","tag":"{{order.expand.tag}}","remark":"{{order.expand.remark}}","config":"{{order.expand.config}}"}`

func xdjkLonglongProcessConfig() *ProcessRuleConfig {
	return &ProcessRuleConfig{
		Handler: "http",
		Method:  "GET",
		URL:     "{{huoyuan.url}}/api/order/uuid/{{order.yid}}",
		Headers: map[string]string{
			"X-Uid":     "{{huoyuan.user}}",
			"X-Api-Key": "{{huoyuan.pass}}",
			"Accept":    "application/json",
		},
		Map: ProcessResultMap{
			Fields: map[string]string{
				"yid":         "uuid",
				"kcname":      "courseName",
				"status_text": "status",
				"process":     "finish",
				"remarks":     "result",
				"kcks":        "courseStartTime",
				"kcjs":        "courseEndTime",
				"ksks":        "examStartTime",
				"ksjs":        "examEndTime",
			},
		},
	}
}

func buildXdjkLonglongRuleConfig() SubmitRuleConfig {
	return SubmitRuleConfig{
		Method:      "POST",
		URL:         "{{huoyuan.url}}/api/submit/{{order.noun}}",
		ContentType: "json",
		Headers: map[string]string{
			"X-Uid":     "{{huoyuan.user}}",
			"X-Api-Key": "{{huoyuan.pass}}",
			"Accept":    "application/json",
		},
		BodyMode: "raw",
		BodyRaw:  xdjkLonglongBodyRaw,
		Body:     map[string]string{},
		Response: SubmitRuleResp{
			SuccessHTTP: true,
			YIDPath:     "0",
			SuccessMsg:  "下单成功",
		},
		Process: xdjkLonglongProcessConfig(),
	}
}

func parseXdjkSimpleWithBranches(platformType string, specialNotes []string) *xdjkParseResult {
	basePlatform := `{{concat order.noun "|score=" order.uScore ";sc=" order.uTime "|day_score=" order.simple_day_score ";total_score=" order.simple_total_score ";learntime=" order.simple_learn_time}}`
	simpleBranchBody := func(platform, school string) map[string]string {
		return map[string]string{
			"token":    "{{huoyuan.token}}",
			"platform": platform,
			"school":   school,
			"user":     "{{order.user}}",
			"pass":     "{{order.pass}}",
			"course":   "{{order.kcname}}",
			"courseid": "{{order.kcid}}",
		}
	}
	examPipe := `{{concat order.noun "|exam_code=" split_part order.school 1 "|" 2}}`
	examSemi := `{{concat order.noun ";exam_code=" split_part order.school 1 "|" 2}}`
	scorePipe := `{{concat order.noun "|day_score=" split_part order.school 1 "|" 3 ";total_score=" split_part order.school 2 "|" 3}}`
	scoreSemi := `{{concat order.noun ";day_score=" split_part order.school 1 "|" 3 ";total_score=" split_part order.school 2 "|" 3}}`
	school2 := `{{split_part order.school 0 "|" 2}}`
	school3 := `{{split_part order.school 0 "|" 3}}`

	return &xdjkParseResult{
		PlatformType:    platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/Api/Create",
			ContentType: "form",
			Body:        map[string]string{},
			Branches: []SubmitRuleBranch{
				{
					When: &SubmitRuleWhen{All: []SubmitRuleWhen{
						{Field: "order.noun", Contains: "437"},
						{Field: "order.school", Contains: "|"},
						{Field: "order.noun", Contains: "|"},
					}},
					Body: simpleBranchBody(examSemi, school2),
				},
				{
					When: &SubmitRuleWhen{All: []SubmitRuleWhen{
						{Field: "order.noun", Contains: "437"},
						{Field: "order.school", Contains: "|"},
					}},
					Body: simpleBranchBody(examPipe, school2),
				},
				{
					When: &SubmitRuleWhen{All: []SubmitRuleWhen{
						{Field: "order.noun", Contains: "392"},
						{Field: "order.school", Contains: "|"},
						{Field: "order.noun", Contains: "|"},
					}},
					Body: simpleBranchBody(examSemi, school2),
				},
				{
					When: &SubmitRuleWhen{All: []SubmitRuleWhen{
						{Field: "order.noun", Contains: "392"},
						{Field: "order.school", Contains: "|"},
					}},
					Body: simpleBranchBody(examPipe, school2),
				},
				{
					When: &SubmitRuleWhen{All: []SubmitRuleWhen{
						{Field: "order.noun", Contains: "385"},
						{Field: "order.school", Contains: "|"},
						{Field: "order.noun", Contains: "|"},
					}},
					Body: simpleBranchBody(scoreSemi, school3),
				},
				{
					When: &SubmitRuleWhen{All: []SubmitRuleWhen{
						{Field: "order.noun", Contains: "385"},
						{Field: "order.school", Contains: "|"},
					}},
					Body: simpleBranchBody(scorePipe, school3),
				},
				{
					When: &SubmitRuleWhen{Default: true},
					Body: simpleBranchBody(basePlatform, "{{order.school}}"),
				},
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"1", 1},
				MsgField:     "msg",
				SuccessMsg:   "添加成功",
			},
		},
		Warnings: []string{
			"school|考试码(437/392)、school|日上限|累计上限(385) 已用 branches + split_part",
			"385 需 school 含两个 | 分段且日/累计上限非空",
		},
		SpecialNotes: specialNotes,
	}
}

type standardActAddSpec struct {
	extra     map[string]string
	useCookie bool
	yidField  string
	actPath   string
}

var xdjkStandardActAdd = map[string]standardActAddSpec{
	"29":      {},
	"liufu":   {},
	"ssrs":    {},
	"daxiong": {},
	"bdkj":    {},
	"ace":     {},
	"maodou":  {extra: map[string]string{"shichang": "{{order.uTime}}", "score": "{{order.uScore}}"}},
	"bsc":     {},
	"xuemei":  {},
	"liunian": {yidField: "id"},
	"tom":     {extra: map[string]string{"type": "29"}, yidField: "id"},
	"yue29":   {},
	"2023":    {},
	"hh":      {},
	"huangzu": {useCookie: true, yidField: "id"},
	"pup":     {},
	"ml":      {},
	"hei":     {},
	"miaosha": {yidField: "id"},
	"wufu":    {},
}

func parseXdjkStandardActAdd(platformType string, specialNotes []string) *xdjkParseResult {
	spec, ok := xdjkStandardActAdd[toLowerASCII(platformType)]
	if !ok {
		return nil
	}
	actPath := spec.actPath
	if actPath == "" {
		actPath = "/api.php?act=add"
	}
	res := parseXdjkActAddWithSuccess(platformType, actPath, specialNotes, spec.extra, spec.useCookie,
		[]interface{}{"0", 0}, spec.yidField)
	return res
}

func toLowerASCII(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
