import { getSessionToken, parseApiResponse, type ApiResponse } from '@/api/auth'

export interface OpsConfig {
  enabled: boolean
  mode: 'assist' | 'auto'
  ai_enabled: boolean
  scan_interval_seconds: number
  ai_analysis_interval_seconds: number
  ai_rate_limit_per_hour: number
  auto_execute_min_severity: string
  playbooks_enabled: boolean
  notify_on_auto_action: boolean
  notify_on_rollback: boolean
  observe_duration_minutes: number
  daily_report_enabled: boolean
  daily_report_hour: number
  ops_model?: string
  ops_max_tokens?: number
  thresholds: {
    channel_fail_rate_pct: number
    channel_fail_rate_spike_pp: number
    dlq_depth: number
    queue_backlog: number
    resume_fail_rate_pct: number
    resume_stable_minutes: number
    resume_min_window_events: number
  }
  policy: {
    max_actions_per_plan: number
    action_cooldown_seconds: number
    hid_cooldown_seconds: number
  }
}

export interface OpsPausedChannel {
  hid: number
  name: string
  since: string
}

export interface OpsStatus {
  enabled: boolean
  mode: string
  ai_ready: boolean
  watcher_running: boolean
  paused_channels: OpsPausedChannel[]
  last_incident?: {
    id: number
    summary: string
    status: string
  }
}

export interface OpsActionSpec {
  action: string
  params?: Record<string, unknown>
  risk?: string
  auto_execute: boolean
  reason?: string
}

export interface OpsPlan {
  incident_type: string
  severity: string
  summary: string
  root_cause_hypothesis?: string
  confidence?: number
  recommended_actions: OpsActionSpec[]
  manual_suggestions?: string[]
  matched_playbook?: string
  source: string
}

export interface OpsActionResult {
  action: string
  ok: boolean
  message: string
}

export interface OpsAnalyzeResult {
  audit_id: number
  plan: OpsPlan
  executed: boolean
  actions_result?: OpsActionResult[]
  events?: string[]
  warnings?: string[]
}

export interface OpsAuditRow {
  id: number
  created_at: string
  trigger_type: string
  source: string
  severity: string
  incident_type: string
  summary: string
  operator?: string
  status: string
  error_message?: string
  context_json?: unknown
  plan_json?: unknown
  executed_actions?: unknown
  snapshot_json?: unknown
}

export interface OpsDailyReport {
  date: string
  generated_at: string
  title: string
  summary: string
  body: string
  pushed: boolean
}

function authHeaders(): Record<string, string> {
  const token = getSessionToken()
  const h: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) {
    h['Authorization'] = `Bearer ${token}`
    h['X-Session-Token'] = token
  }
  return h
}

async function request<T>(path: string, options: RequestInit = {}): Promise<ApiResponse<T>> {
  const res = await fetch(path, {
    ...options,
    headers: { ...authHeaders(), ...(options.headers as Record<string, string>) },
  })
  return parseApiResponse<T>(res)
}

export const opsApi = {
  config: () => request<OpsConfig>('/api/ops/config'),
  saveConfig: (payload: Partial<OpsConfig>) =>
    request<OpsConfig>('/api/ops/config', { method: 'PUT', body: JSON.stringify(payload) }),
  status: () => request<OpsStatus>('/api/ops/status'),
  analyze: (execute = false) =>
    request<OpsAnalyzeResult>('/api/ops/analyze', {
      method: 'POST',
      body: JSON.stringify({ execute }),
    }),
  auditList: (page = 1, limit = 20) =>
    request<{ items: OpsAuditRow[]; total: number; page: number; limit: number }>(
      `/api/ops/audit?page=${page}&limit=${limit}`,
    ),
  auditDetail: (id: number) => request<OpsAuditRow>(`/api/ops/audit/${id}`),
  rollback: (id: number) =>
    request<OpsActionResult[]>(`/api/ops/rollback/${id}`, { method: 'POST', body: '{}' }),
  dailyReport: () => request<OpsDailyReport | null>('/api/ops/report/daily'),
  pauseChannel: (hid: number) =>
    request<null>(`/api/ops/channels/${hid}/pause`, { method: 'POST', body: '{}' }),
  resumeChannel: (hid: number) =>
    request<null>(`/api/ops/channels/${hid}/resume`, { method: 'POST', body: '{}' }),
}

export function opsModeLabel(mode: string): string {
  if (mode === 'auto') return '自动处理'
  return '只看建议'
}

export function opsModeDescription(mode: string): string {
  if (mode === 'auto') {
    return '发现问题后，会自动暂停渠道、调整并发等（在安全范围内）'
  }
  return '只提示问题和建议，不会自动改设置；需要时可手动执行'
}

export function opsSeverityLabel(severity: string): string {
  const map: Record<string, string> = {
    critical: '严重',
    high: '高',
    medium: '中',
    low: '低',
  }
  return map[severity.toLowerCase()] || severity
}

export function opsStatusLabel(status: string): string {
  const map: Record<string, string> = {
    planned: '待处理',
    executed: '已完成',
    partial: '部分成功',
    rolled_back: '已撤销',
    rejected: '已拒绝',
    failed: '失败',
  }
  return map[status] || status
}

export function opsSeverityVariant(severity: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (severity) {
    case 'critical':
    case 'high':
      return 'destructive'
    case 'medium':
      return 'default'
    default:
      return 'secondary'
  }
}

/** 操作人显示 */
export function opsOperatorLabel(operator?: string): string {
  if (!operator) return ''
  if (operator === 'watcher') return '系统自动'
  return operator
}

/** 操作记录来源 */
export function opsSourceLabel(source: string): string {
  const map: Record<string, string> = {
    rule: '自动规则',
    ai: 'AI 分析',
    manual: '手动检查',
  }
  return map[source.toLowerCase()] || source
}

/** 触发方式 */
export function opsTriggerTypeLabel(trigger: string): string {
  const t = trigger.toLowerCase()
  if (t === 'manual') return '手动触发'
  if (t === 'watcher') return '定时自动检查'
  if (t.startsWith('auto_resume:')) return '渠道自动恢复'
  return trigger
}

/** 运维动作名称 */
export function opsActionLabel(action: string): string {
  const map: Record<string, string> = {
    pause_channel: '停用渠道',
    resume_channel: '恢复渠道',
    adjust_workers: '调整同时处理数量',
    enable_dlq_auto_retry: '开启失败订单定时重试',
    reload_rules: '重新加载平台规则',
    notify: '发送通知',
    noop: '仅记录，不改动',
  }
  return map[action] || action
}

interface OpsPlanJsonShape {
  summary?: string
  root_cause_hypothesis?: string
  confidence?: number
  matched_playbook?: string
  recommended_actions?: Array<{ action: string; reason?: string; auto_execute?: boolean }>
  manual_suggestions?: string[]
}

/** 把处理方案 JSON 转成可读条目 */
export function formatOpsPlanSections(plan: unknown): { title: string; lines: string[] }[] {
  if (!plan || typeof plan !== 'object') return []
  const p = plan as OpsPlanJsonShape
  const sections: { title: string; lines: string[] }[] = []
  if (p.summary) sections.push({ title: '情况说明', lines: [p.summary] })
  if (p.root_cause_hypothesis) sections.push({ title: '可能原因', lines: [p.root_cause_hypothesis] })
  if (p.matched_playbook) sections.push({ title: '匹配到的规则', lines: [p.matched_playbook] })
  if (p.recommended_actions?.length) {
    sections.push({
      title: '建议怎么处理',
      lines: p.recommended_actions.map((a) => {
        const who = a.auto_execute ? '可自动执行' : '需您确认'
        const reason = a.reason ? `：${a.reason}` : ''
        return `· ${opsActionLabel(a.action)}（${who}）${reason}`
      }),
    })
  }
  if (p.manual_suggestions?.length) {
    sections.push({ title: '其他建议', lines: p.manual_suggestions.map((s) => `· ${s}`) })
  }
  return sections
}

/** 把处理结果 JSON 转成可读列表 */
export function formatOpsActionResults(
  actions: unknown,
): { action: string; ok: boolean; message: string }[] {
  if (!Array.isArray(actions)) return []
  return actions.map((raw) => {
    const a = raw as OpsActionResult
    return {
      action: opsActionLabel(String(a.action || '')),
      ok: !!a.ok,
      message: String(a.message || ''),
    }
  })
}
