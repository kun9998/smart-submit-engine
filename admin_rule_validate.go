package main

import "strings"

// validateRuleConfig 静态检查 rule_config，返回可读提示（非致命项也列出，便于保存前核对）
func validateRuleConfig(rule SubmitRuleConfig) []string {
	var hints []string

	if !aiRuleHasActionableURL(rule) {
		hints = append(hints, "缺少 url（顶层、branches、pipeline 或 script 步骤中至少一处）")
	}
	if len(rule.Response.SuccessCodes) == 0 && !rule.Response.SuccessHTTP {
		hints = append(hints, "缺少 response.success_codes 或 success_http")
	}
	if strings.TrimSpace(rule.Method) == "" {
		hints = append(hints, "缺少 method，建议 POST 或 GET")
	}
	if rule.ContentType == "" && len(rule.Branches) == 0 {
		hints = append(hints, "缺少 content_type，form 或 json")
	}

	if rule.BodyMode == "raw" && strings.TrimSpace(rule.BodyRaw) == "" && !branchHasBodyRaw(rule.Branches) {
		hints = append(hints, "body_mode=raw 但未配置 body_raw（含 branches 内）")
	}
	if rule.BodyMode == "kcid_json" {
		if len(rule.KcidJSONPatches) == 0 && !branchHasKcidPatches(rule.Branches) {
			hints = append(hints, "kcid_json 未配置 kcid_json_patches 或 branches 补丁")
		}
		if rule.KcidJSONValidate == nil {
			hints = append(hints, "kcid_json 建议配置 kcid_json_validate（如 task_list exact:1）")
		}
	}
	if len(rule.Branches) > 0 && !branchesHaveDefault(rule.Branches) {
		hints = append(hints, "branches 建议包含 when.default 兜底分支")
	}
	if rule.Handler == "pipeline" && len(rule.Pipeline) == 0 {
		hints = append(hints, "handler=pipeline 但 pipeline 为空")
	}
	if rule.DelayMS < 0 {
		hints = append(hints, "delay_ms 不能为负数")
	}

	return hints
}

func branchHasBodyRaw(branches []SubmitRuleBranch) bool {
	for _, b := range branches {
		if strings.TrimSpace(b.BodyRaw) != "" {
			return true
		}
	}
	return false
}

func branchHasKcidPatches(branches []SubmitRuleBranch) bool {
	for _, b := range branches {
		if len(b.KcidJSONPatches) > 0 {
			return true
		}
	}
	return false
}

func branchesHaveDefault(branches []SubmitRuleBranch) bool {
	for i := range branches {
		if branches[i].When != nil && branches[i].When.Default {
			return true
		}
	}
	return false
}

func attachRuleValidationHints(out *aiConvertResponse) {
	if out == nil {
		return
	}
	hints := validateRuleConfig(out.RuleConfig)
	out.ValidationHints = hints
	if len(hints) > 0 {
		out.Warnings = appendUniqueStrings(out.Warnings, hints...)
	}
}
