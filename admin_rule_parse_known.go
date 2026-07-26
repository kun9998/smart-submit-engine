package main

import "strings"

func parseXdjkKnownPlatform(platformType, code string, specialNotes []string) *xdjkParseResult {
	switch strings.ToLower(strings.TrimSpace(platformType)) {
	case "baize":
		return parseXdjkBaize(platformType, specialNotes)
	case "xiyou":
		return parseXdjkXiyou(platformType, specialNotes)
	case "yyy":
		return parseXdjkYyy(platformType, specialNotes)
	case "8090":
		return parseXdjk8090WithBranches(platformType, code, specialNotes)
	case "kdbxxt":
		return parseXdjkKdbxxt(platformType, specialNotes)
	case "kdbzhs":
		return parseXdjkKdbzhs(platformType, specialNotes)
	case "kdbzhzj":
		return parseXdjkKdbzhzj(platformType, specialNotes)
	case "maliaorun":
		return parseXdjkMaliaorun(platformType, specialNotes)
	case "lyyjxjy":
		return parseXdjkLyyjxjy(platformType, specialNotes)
	case "df1":
		return parseXdjkDf1(platformType, specialNotes)
	case "df2":
		return parseXdjkDf2(platformType, specialNotes)
	case "simple":
		return parseXdjkSimpleWithBranches(platformType, specialNotes)
	case "wuming":
		return parseXdjkWuming(platformType, specialNotes)
	case "dlam":
		return parseXdjkDlam(platformType, specialNotes)
	case "kun":
		return parseXdjkKUN(platformType, specialNotes)
	case "kunba":
		return parseXdjkKunba(platformType, specialNotes)
	case "lotus":
		return parseXdjkLotus(platformType, code, specialNotes)
	case "huoxi":
		return parseXdjkHuoxi(platformType, specialNotes)
	case "longlong":
		return parseXdjkLonglongKnown(platformType, specialNotes)
	case "gostudy":
		return parseXdjkGoStudy(platformType, specialNotes)
	case "jxjyyjy":
		return parseXdjkJxjyyjy(platformType, specialNotes)
	case "langr":
		return parseXdjkLangr(platformType, specialNotes)
	case "yqsl":
		return parseXdjkYqsl(platformType, specialNotes)
	case "algk":
		return parseXdjkAlgk(platformType, specialNotes)
	case "algksy":
		return parseXdjkAlgksy(platformType, specialNotes)
	case "tesla":
		return parseXdjkTesla(platformType, specialNotes)
	case "thoth":
		return parseXdjkTHOTH(platformType, specialNotes)
	case "coco":
		return parseXdjkCoco(platformType, specialNotes)
	case "nx":
		return parseXdjkNx(platformType, specialNotes)
	case "00":
		return parseXdjk00(platformType, specialNotes)
	case "yumeng":
		return parseXdjkYumeng(platformType, specialNotes)
	case "27":
		return parseXdjk27(platformType, specialNotes)
	case "2xx":
		return parseXdjk2xx(platformType, specialNotes)
	case "benz":
		return parseXdjkBenz(platformType, specialNotes)
	case "bld":
		return parseXdjkBld(platformType, specialNotes)
	case "hzw":
		return parseXdjkHzw(platformType, specialNotes)
	case "zfb":
		return parseXdjkZfb(platformType, specialNotes)
	case "duowei":
		return parseXdjkDuowei(platformType, specialNotes)
	case "wanzi":
		return parseXdjkWanzi(platformType, specialNotes)
	case "xm":
		return parseXdjkXm(platformType, specialNotes)
	case "hb":
		return parseXdjkHb(platformType, specialNotes)
	default:
		if std := parseXdjkStandardActAdd(platformType, specialNotes); std != nil {
			return std
		}
		return nil
	}
}

func parseXdjkBaize(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType: platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/v2/docking/add",
			ContentType: "json",
			Body: map[string]string{
				"token":       "{{huoyuan.token}}",
				"platform_id": "{{order.noun}}",
				"school":      "{{order.school}}",
				"account":     "{{order.user}}",
				"pwd":         "{{order.pass}}",
				"course_id":   "{{order.kcid}}",
				"course_name": "{{order.kcname}}",
				"duration":    "{{order.uTime}}",
				"fraction":    "{{order.uScore}}",
			},
			Response: SubmitRuleResp{
				CodeField:    "code",
				SuccessCodes: []interface{}{"0000"},
				MsgField:     "msg",
				YIDPath:      "data.order_id",
				SuccessMsg:   "下单成功",
			},
		},
		Warnings:     []string{"curl POST JSON，已按标准字段映射"},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkXiyou(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType: platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/order/xiadanForPublic",
			ContentType: "form",
			Body: map[string]string{
				"username":   "{{huoyuan.user}}",
				"token":      "{{huoyuan.token}}",
				"classId":    "{{order.noun}}",
				"schoolName": "{{order.school}}",
				"user":       "{{order.user}}",
				"pass":       "{{order.pass}}",
				"courseName": "{{order.kcname}}",
				"courseId":   "{{order.kcid}}",
			},
			Response: SubmitRuleResp{
				CodeField:    "status",
				SuccessCodes: []interface{}{"success"},
				MsgField:     "msg",
				SuccessMsg:   "下单成功",
			},
		},
		SpecialNotes: specialNotes,
	}
}

func parseXdjkYyy(platformType string, specialNotes []string) *xdjkParseResult {
	return &xdjkParseResult{
		PlatformType: platformType,
		TrustedTemplate: true,
		RuleConfig: SubmitRuleConfig{
			Method:      "POST",
			URL:         "{{huoyuan.url}}/api/order",
			ContentType: "form",
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
				SuccessCodes: []interface{}{200, "200"},
				MsgField:     "msg",
				YIDPath:      "data.yid",
				SuccessMsg:   "下单成功",
			},
		},
		SpecialNotes: specialNotes,
	}
}
