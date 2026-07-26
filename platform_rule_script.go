package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func executeScriptSubmitRule(ctx context.Context, order *Order, hy *Huoyuan, httpClient *http.Client, rule SubmitRuleConfig) (*AddOrderResult, error) {
	if rule.Script == nil {
		return &AddOrderResult{Code: -1, Msg: "script 配置为空"}, nil
	}
	src := strings.TrimSpace(rule.Script.Source)
	if src != "" {
		return executeStarlarkSubmitRule(ctx, order, hy, httpClient, rule, src, rule.Script.TimeoutMS)
	}
	if len(rule.Script.Steps) == 0 {
		return &AddOrderResult{Code: -1, Msg: "script.steps 为空"}, nil
	}

	timeout := rule.Script.TimeoutMS
	if timeout <= 0 {
		timeout = 60000
	}
	if timeout > 120000 {
		timeout = 120000
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
		defer cancel()
	}

	return runPipelineSteps(ctx, order, hy, httpClient, rule, rule.Script.Steps, rule.Response)
}
