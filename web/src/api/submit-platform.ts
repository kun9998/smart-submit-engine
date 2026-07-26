export interface SubmitRuleWhen {
  /** 单条件匹配字段；default 分支或 all 组合条件时可省略 */
  field?: string
  equals?: string
  not_equals?: string
  contains?: string
  not_contains?: string
  default?: boolean
  /** 多条件 AND，设置后忽略顶层 field */
  all?: Omit<SubmitRuleWhen, 'all' | 'default'>[]
}

export interface SubmitRuleBranch {
  when?: SubmitRuleWhen
  method?: string
  url?: string
  content_type?: 'form' | 'json'
  use_cookie?: boolean
  headers?: Record<string, string>
  body?: Record<string, string>
  body_mode?: 'raw' | 'kcid_json' | 'map'
  body_raw?: string
  kcid_json_patches?: { when?: SubmitRuleWhen; set: Record<string, unknown> }[]
  response?: SubmitRuleConfig['response']
}

export interface PipelineExtract {
  from: string
  path: string
  to: string
}

export interface SubmitPipelineStep {
  name?: string
  when?: SubmitRuleWhen
  /** set | delay | http | finish | extract | return | poll | process_finish */
  action?: string
  delay_ms?: number
  set?: Record<string, string>
  extract?: PipelineExtract
  method?: string
  url?: string
  content_type?: 'form' | 'json'
  use_cookie?: boolean
  headers?: Record<string, string>
  body?: Record<string, string>
  body_mode?: 'raw' | 'kcid_json' | 'map'
  body_raw?: string
  save_body_as?: string
  response?: SubmitRuleConfig['response']
  return_code?: number
  return_msg?: string
  return_yid?: string
  /** action=poll 时：循环 HTTP 直到 until 命中 */
  poll?: {
    interval_ms?: number
    max_attempts?: number
    until: SubmitRuleConfig['response']
    fail?: SubmitRuleConfig['response']
  }
  process_map?: ProcessResultMap
}

export interface ProcessResultMap {
  items_path?: string
  code_field?: string
  success_codes?: (string | number)[]
  msg_field?: string
  fields: Record<string, string>
}

export interface ProcessRuleConfig {
  handler?: 'http' | 'pipeline' | 'script'
  method?: string
  url?: string
  content_type?: 'form' | 'json'
  use_cookie?: boolean
  headers?: Record<string, string>
  body?: Record<string, string>
  body_mode?: 'raw' | 'kcid_json' | 'map'
  body_raw?: string
  pipeline?: SubmitPipelineStep[]
  script?: SubmitScriptConfig
  map: ProcessResultMap
}

export interface SubmitScriptConfig {
  steps?: SubmitPipelineStep[]
  source?: string
  timeout_ms?: number
}

export interface SubmitRuleConfig {
  /** 空/http=单次请求；pipeline=多步；script=script.steps */
  handler?: '' | 'http' | 'pipeline' | 'script'
  method: string
  /** 顶层 URL；branches/pipeline 可覆盖时可省略 */
  url?: string
  content_type: 'form' | 'json'
  use_cookie?: boolean
  headers?: Record<string, string>
  body: Record<string, string>
  /** 空/map=键值 body；raw=整段模板；kcid_json=解码 kcid 后打补丁 */
  body_mode?: 'raw' | 'kcid_json' | 'map'
  body_raw?: string
  /** KUN 等：URL 中 {{random_port}} 从此池随机 */
  url_port_pool?: number[]
  /** 按 order 字段分支，覆盖 url/body 等 */
  branches?: SubmitRuleBranch[]
  kcid_json_patches?: { when?: SubmitRuleWhen; set: Record<string, unknown> }[]
  kcid_json_validate?: { path: string; exact?: number; min_len?: number; max_len?: number }
  delay_ms?: number
  pipeline?: SubmitPipelineStep[]
  script?: SubmitScriptConfig
  /** 查课/进度同步（ProcessOrder） */
  process?: ProcessRuleConfig
  response: {
    code_field?: string
    success_codes?: (string | number)[]
    success_http?: boolean
    msg_field?: string
    yid_field?: string
    yid_path?: string
    success_msg?: string
    failure_msg_rules?: { contains: string; msg?: string; code?: number }[]
    failure_msg_on_success?: boolean
    success_use_upstream_msg?: boolean
  }
}

export interface SubmitPlatform {
  id?: number
  platform_type: string
  display_name: string
  enabled: boolean
  rule_config: SubmitRuleConfig
  version?: number
  remark?: string
  source_php?: string
}

export interface ApiResponse<T = unknown> {
  code: number
  msg: string
  data?: T
  need_login?: boolean
  need_install?: boolean
}

const TOKEN_KEY = 'tj_session_token'

function getSessionToken(): string {
  return localStorage.getItem(TOKEN_KEY) || ''
}

async function request<T>(path: string, options: RequestInit = {}): Promise<ApiResponse<T>> {
  const token = getSessionToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
    headers['X-Session-Token'] = token
  }
  const res = await fetch(path, { ...options, headers })
  return res.json()
}

export interface RuleTestSubmitResult {
  has_orders: boolean
  order_count: number
  oid?: string
  hid?: number
  huoyuan_name?: string
  platform_match?: boolean
  success: boolean
  yid?: string
  err_msg?: string
  upstream_body?: string
  warning?: string
}

export const api = {
  list: () => request<SubmitPlatform[]>('/api/submit-platforms'),
  get: (type: string) => request<SubmitPlatform>(`/api/submit-platforms/${encodeURIComponent(type)}`),
  create: (body: SubmitPlatform) =>
    request('/api/submit-platforms', { method: 'POST', body: JSON.stringify(body) }),
  update: (type: string, body: Partial<SubmitPlatform>) =>
    request(`/api/submit-platforms/${encodeURIComponent(type)}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
  remove: (type: string) =>
    request(`/api/submit-platforms/${encodeURIComponent(type)}`, { method: 'DELETE' }),
  reloadCache: () =>
    request<{ count: number }>('/api/submit-platforms/reload', { method: 'POST' }),
  aiStatus: () =>
    request<{ configured: boolean; enabled: boolean; model: string }>('/api/submit-platforms/ai-status'),
  aiConvert: (php: string, platform_type_hint?: string) =>
    request<{
      platform_type: string
      rule_config: SubmitRuleConfig
      warnings?: string[]
      notes?: string
      parse_source?: 'local' | 'hybrid' | 'ai'
      validation_hints?: string[]
    }>('/api/submit-platforms/ai-convert', {
      method: 'POST',
      body: JSON.stringify({ php, platform_type_hint }),
    }),
  testSubmit: (platform_type: string, rule_config: SubmitRuleConfig, oid?: string) =>
    request<RuleTestSubmitResult>(
      `/api/submit-platforms/${encodeURIComponent(platform_type)}/test-submit`,
      {
        method: 'POST',
        body: JSON.stringify({ rule_config, oid }),
      },
    ),
  aiFixFromFailure: (
    platform_type: string,
    body: {
      rule_config: SubmitRuleConfig
      err_msg: string
      upstream_body?: string
      php?: string
    },
  ) =>
    request<{
      platform_type: string
      rule_config: SubmitRuleConfig
      warnings?: string[]
      notes?: string
      parse_source?: string
      validation_hints?: string[]
    }>(`/api/submit-platforms/${encodeURIComponent(platform_type)}/ai-fix-from-failure`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
}

export function defaultRuleConfig(): SubmitRuleConfig {
  return {
    method: 'POST',
    url: '{{huoyuan.url}}/api.php?act=add',
    content_type: 'form',
    use_cookie: false,
    body: {
      uid: '{{huoyuan.user}}',
      key: '{{huoyuan.pass}}',
      platform: '{{order.noun}}',
      school: '{{order.school}}',
      user: '{{order.user}}',
      pass: '{{order.pass}}',
      kcname: '{{order.kcname}}',
      kcid: '{{order.kcid}}',
    },
    response: {
      code_field: 'code',
      success_codes: ['0', 0],
      msg_field: 'msg',
      success_msg: '下单成功',
    },
  }
}
