package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func ensureOpsAuditSchema(ctx context.Context) {
	if pluginDB == nil {
		return
	}
	q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		trigger_type VARCHAR(32) NOT NULL,
		source VARCHAR(16) NOT NULL,
		severity VARCHAR(16) NOT NULL,
		incident_type VARCHAR(32) NOT NULL,
		summary TEXT NOT NULL,
		operator VARCHAR(64) NOT NULL DEFAULT '',
		context_json JSON NOT NULL,
		plan_json JSON NOT NULL,
		status VARCHAR(24) NOT NULL,
		executed_actions JSON,
		snapshot_json JSON,
		error_message TEXT,
		INDEX idx_created (created_at),
		INDEX idx_status (status)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`, pluginTable("ops_audit"))
	_, _ = pluginDB.ExecContext(ctx, q)
}

type opsAuditRow struct {
	ID              int64           `json:"id"`
	CreatedAt       time.Time       `json:"created_at"`
	TriggerType     string          `json:"trigger_type"`
	Source          string          `json:"source"`
	Severity        string          `json:"severity"`
	IncidentType    string          `json:"incident_type"`
	Summary         string          `json:"summary"`
	Operator        string          `json:"operator,omitempty"`
	ContextJSON     json.RawMessage `json:"context_json"`
	PlanJSON        json.RawMessage `json:"plan_json"`
	Status          string          `json:"status"`
	ExecutedActions json.RawMessage `json:"executed_actions,omitempty"`
	SnapshotJSON    json.RawMessage `json:"snapshot_json,omitempty"`
	ErrorMessage    string          `json:"error_message,omitempty"`
}

type opsAuditInsert struct {
	TriggerType     string
	Source          string
	Severity        string
	IncidentType    string
	Summary         string
	Operator        string
	ContextJSON     []byte
	PlanJSON        []byte
	Status          string
	ExecutedActions []byte
	SnapshotJSON    []byte
	ErrorMessage    string
}

func insertOpsAudit(ctx context.Context, row opsAuditInsert) (int64, error) {
	if pluginDB == nil {
		return 0, fmt.Errorf("插件数据库未就绪")
	}
	ensureOpsAuditSchema(ctx)
	q := fmt.Sprintf(`INSERT INTO %s
		(trigger_type, source, severity, incident_type, summary, operator, context_json, plan_json, status, executed_actions, snapshot_json, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, pluginTable("ops_audit"))
	res, err := pluginDB.ExecContext(ctx, q,
		row.TriggerType, row.Source, row.Severity, row.IncidentType, row.Summary, row.Operator,
		string(row.ContextJSON), string(row.PlanJSON), row.Status,
		nullJSON(row.ExecutedActions), nullJSON(row.SnapshotJSON), nullString(row.ErrorMessage),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func updateOpsAudit(ctx context.Context, id int64, status string, executedActions, snapshotJSON []byte, errMsg string) error {
	if pluginDB == nil {
		return fmt.Errorf("插件数据库未就绪")
	}
	q := fmt.Sprintf(`UPDATE %s SET status=?, executed_actions=?, snapshot_json=?, error_message=? WHERE id=?`, pluginTable("ops_audit"))
	_, err := pluginDB.ExecContext(ctx, q, status, nullJSON(executedActions), nullJSON(snapshotJSON), nullString(errMsg), id)
	return err
}

func nullJSON(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

func nullString(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func listOpsAudit(ctx context.Context, page, limit int) ([]opsAuditRow, int, error) {
	if pluginDB == nil {
		return nil, 0, fmt.Errorf("插件数据库未就绪")
	}
	ensureOpsAuditSchema(ctx)
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit
	var total int
	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, pluginTable("ops_audit"))
	if err := pluginDB.QueryRowContext(ctx, countQ).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := fmt.Sprintf(`SELECT id, created_at, trigger_type, source, severity, incident_type, summary, operator,
		context_json, plan_json, status, executed_actions, snapshot_json, error_message
		FROM %s ORDER BY id DESC LIMIT ? OFFSET ?`, pluginTable("ops_audit"))
	rows, err := pluginDB.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]opsAuditRow, 0, limit)
	for rows.Next() {
		var r opsAuditRow
		var ctxRaw, planRaw, execRaw, snapRaw sql.NullString
		var errRaw sql.NullString
		if err := rows.Scan(&r.ID, &r.CreatedAt, &r.TriggerType, &r.Source, &r.Severity, &r.IncidentType, &r.Summary, &r.Operator,
			&ctxRaw, &planRaw, &r.Status, &execRaw, &snapRaw, &errRaw); err != nil {
			return nil, 0, err
		}
		if ctxRaw.Valid {
			r.ContextJSON = json.RawMessage(ctxRaw.String)
		}
		if planRaw.Valid {
			r.PlanJSON = json.RawMessage(planRaw.String)
		}
		if execRaw.Valid {
			r.ExecutedActions = json.RawMessage(execRaw.String)
		}
		if snapRaw.Valid {
			r.SnapshotJSON = json.RawMessage(snapRaw.String)
		}
		if errRaw.Valid {
			r.ErrorMessage = errRaw.String
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func getOpsAuditByID(ctx context.Context, id int64) (*opsAuditRow, error) {
	if pluginDB == nil {
		return nil, fmt.Errorf("插件数据库未就绪")
	}
	ensureOpsAuditSchema(ctx)
	q := fmt.Sprintf(`SELECT id, created_at, trigger_type, source, severity, incident_type, summary, operator,
		context_json, plan_json, status, executed_actions, snapshot_json, error_message
		FROM %s WHERE id=?`, pluginTable("ops_audit"))
	var r opsAuditRow
	var ctxRaw, planRaw, execRaw, snapRaw sql.NullString
	var errRaw sql.NullString
	err := pluginDB.QueryRowContext(ctx, q, id).Scan(&r.ID, &r.CreatedAt, &r.TriggerType, &r.Source, &r.Severity, &r.IncidentType, &r.Summary, &r.Operator,
		&ctxRaw, &planRaw, &r.Status, &execRaw, &snapRaw, &errRaw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if ctxRaw.Valid {
		r.ContextJSON = json.RawMessage(ctxRaw.String)
	}
	if planRaw.Valid {
		r.PlanJSON = json.RawMessage(planRaw.String)
	}
	if execRaw.Valid {
		r.ExecutedActions = json.RawMessage(execRaw.String)
	}
	if snapRaw.Valid {
		r.SnapshotJSON = json.RawMessage(snapRaw.String)
	}
	if errRaw.Valid {
		r.ErrorMessage = errRaw.String
	}
	return &r, nil
}

func getLatestOpsAudit(ctx context.Context) (*opsAuditRow, error) {
	rows, _, err := listOpsAudit(ctx, 1, 1)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

type opsAuditDayStats struct {
	Total       int
	Executed    int
	RolledBack  int
	HighOrAbove int
	Highlights  []string
}

func opsAuditStatsSince(ctx context.Context, since time.Time) (opsAuditDayStats, error) {
	var out opsAuditDayStats
	if pluginDB == nil {
		return out, fmt.Errorf("插件数据库未就绪")
	}
	ensureOpsAuditSchema(ctx)
	countQ := fmt.Sprintf(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN status IN ('executed','partial') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status='rolled_back' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN severity IN ('high','critical') THEN 1 ELSE 0 END), 0)
		FROM %s WHERE created_at >= ?`, pluginTable("ops_audit"))
	if err := pluginDB.QueryRowContext(ctx, countQ, since).Scan(
		&out.Total, &out.Executed, &out.RolledBack, &out.HighOrAbove,
	); err != nil {
		return out, err
	}
	listQ := fmt.Sprintf(`SELECT summary FROM %s WHERE created_at >= ? ORDER BY id DESC LIMIT 5`, pluginTable("ops_audit"))
	rows, err := pluginDB.QueryContext(ctx, listQ, since)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return out, err
		}
		out.Highlights = append(out.Highlights, s)
	}
	return out, rows.Err()
}
