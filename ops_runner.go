package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type opsAnalyzeResult struct {
	AuditID       int64             `json:"audit_id"`
	Plan          opsPlanDTO        `json:"plan"`
	Executed      bool              `json:"executed"`
	ActionsResult []opsActionResult `json:"actions_result,omitempty"`
	Events        []string          `json:"events,omitempty"`
	Warnings      []string          `json:"warnings,omitempty"`
}

func runOpsAnalyze(ctx context.Context, trigger string, execute bool, fromWatcher bool, operator string) (*opsAnalyzeResult, error) {
	opsCtx := collectOpsContext(ctx, trigger)
	events := detectOpsEvents(opsCtx)

	var rulePlan, aiPlan *opsPlanDTO
	if getOpsConfig().PlaybooksEnabled {
		rulePlan = matchOpsPlaybooks(opsCtx, events)
	}

	aiErr := ""
	if rulePlan == nil && getOpsConfig().AIEnabled && opsAIReady() {
		plan, err := analyzeOpsWithAI(ctx, opsCtx, events)
		if err != nil {
			aiErr = err.Error()
		} else {
			aiPlan = plan
		}
	}

	plan := mergeOpsPlans(rulePlan, aiPlan)
	if plan.Summary == "" && len(events) == 0 {
		plan.Summary = "未发现明显异常"
	}
	if rulePlan == nil && aiPlan == nil && len(events) > 0 {
		plan = defaultOpsPlan("rule", "unknown", "medium", "检测到异常事件："+strings.Join(events, "、"))
		plan.ManualSuggestions = []string{"请在管理端查看引擎统计与提交日志", "如需自动处置请开启 Playbook 或 AI 运维"}
	}

	plan, warnings := validateOpsPlan(plan)
	if aiErr != "" {
		warnings = append(warnings, "AI 分析失败: "+aiErr)
	}

	status := "planned"
	var execResults []opsActionResult
	var snap *opsExecSnapshot
	executed := false
	errMsg := ""

	doExecute := shouldExecuteOpsPlan(plan, execute, fromWatcher)
	if doExecute {
		actions := filterExecutableActions(plan)
		if len(actions) > 0 {
			var err error
			execResults, snap, err = executeOpsActions(ctx, actions)
			if err != nil {
				errMsg = err.Error()
				status = "failed"
			} else {
				executed = true
				status = "executed"
				for _, r := range execResults {
					if !r.OK {
						status = "partial"
						break
					}
				}
			}
		}
	}

	auditID, err := insertOpsAudit(ctx, opsAuditInsert{
		TriggerType:     trigger,
		Source:          plan.Source,
		Severity:        plan.Severity,
		IncidentType:    plan.IncidentType,
		Summary:         plan.Summary,
		Operator:        operator,
		ContextJSON:     opsContextJSON(opsCtx),
		PlanJSON:        opsPlanJSON(plan),
		Status:          status,
		ExecutedActions: actionResultsJSON(execResults),
		SnapshotJSON:    snapshotJSON(snap),
		ErrorMessage:    errMsg,
	})
	if err != nil {
		return nil, err
	}

	if executed && snap != nil {
		scheduleOpsObservation(auditID, snap, plan)
	}

	return &opsAnalyzeResult{
		AuditID:       auditID,
		Plan:          plan,
		Executed:      executed,
		ActionsResult: execResults,
		Events:        events,
		Warnings:      warnings,
	}, nil
}

func runOpsRollback(ctx context.Context, auditID int64, operator string) ([]opsActionResult, error) {
	row, err := getOpsAuditByID(ctx, auditID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("审计记录不存在")
	}
	if len(row.SnapshotJSON) == 0 {
		return nil, fmt.Errorf("该记录无可回滚快照")
	}
	var snap opsExecSnapshot
	if err := json.Unmarshal(row.SnapshotJSON, &snap); err != nil {
		return nil, fmt.Errorf("快照解析失败")
	}
	results, err := rollbackOpsSnapshot(ctx, &snap)
	if err != nil {
		return nil, err
	}
	_ = updateOpsAudit(ctx, auditID, "rolled_back", actionResultsJSON(results), row.SnapshotJSON, "")
	log.Printf("[AI运维] 手动回滚 audit=%d operator=%s", auditID, operator)
	return results, nil
}
