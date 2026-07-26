import { getSessionToken, parseApiResponse, type ApiResponse } from '@/api/auth'

export type { ApiResponse }

export interface RuntimeQueueSection {
  producer_interval_ms?: number | null
  min_workers_per_hid?: number | null
  max_workers_per_hid?: number | null
  scale_check_interval_ms?: number | null
  scale_step_threshold?: number | null
  processing_timeout_minutes?: number | null
  reaper_interval_minutes?: number | null
  timeout_confirm_wait_seconds?: number | null
  stats_interval_minutes?: number | null
  idle_sleep_ms?: number | null
  submit_pool_workers?: number | null
  submit_pool_queue_cap?: number | null
  confirm_pool_workers?: number | null
  confirm_pool_queue_cap?: number | null
}

export interface RuntimeOrderStatusSection {
  submitted_status?: string | null
  submitted_remarks?: string | null
  success_codes?: number[] | null
}

export interface RuntimeRateLimitSection {
  enabled?: boolean | null
  global_max_per_second?: number | null
  per_hid_max_per_second?: number | null
}

export interface RuntimeDLQAutoRetrySection {
  enabled?: boolean | null
  scan_interval_minutes?: number | null
  max_per_scan?: number | null
  min_age_minutes?: number | null
}

export interface RuntimeResubmitSection {
  enabled?: boolean | null
  max_attempts?: number | null
  initial_delay_seconds?: number | null
  backoff_multiplier?: number | null
  max_delay_seconds?: number | null
  retry_on_timeout?: boolean | null
  rate_limit_counts_as_attempt?: boolean | null
  terminal_keywords?: string[] | null
  dlq_auto_retry?: RuntimeDLQAutoRetrySection
}

export interface RuntimeSubmitSection {
  timeout_seconds?: number | null
}

export interface RuntimeConfigPayload {
  queue?: RuntimeQueueSection
  order_status?: RuntimeOrderStatusSection
  rate_limit?: RuntimeRateLimitSection
  resubmit?: RuntimeResubmitSection
  submit?: RuntimeSubmitSection
}

export interface RuntimeConfigView {
  defaults: RuntimeConfigPayload
  override?: RuntimeConfigPayload
  effective: RuntimeConfigPayload
  ops_defaults?: import('@/api/ops').OpsConfig
  ops?: import('@/api/ops').OpsConfig
}

export interface HuoyuanItem {
  hid: number
  name: string
  pt: string
  url: string
  has_config: boolean
  remark?: string
}

async function request<T>(path: string, options: RequestInit = {}): Promise<ApiResponse<T>> {
  const token = getSessionToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  }
  if (token) {
    headers.Authorization = `Bearer ${token}`
    headers['X-Session-Token'] = token
  }
  const res = await fetch(path, { ...options, headers })
  return parseApiResponse<T>(res)
}

export const huoyuanConfigApi = {
  listHuoyuan: () => request<HuoyuanItem[]>('/api/huoyuan'),
  getGlobal: () => request<RuntimeConfigView>('/api/huoyuan-config/global'),
  saveGlobal: (config: RuntimeConfigPayload, ops?: import('@/api/ops').OpsConfig) =>
    request<RuntimeConfigView>('/api/huoyuan-config/global', {
      method: 'PUT',
      body: JSON.stringify({ config, ops }),
    }),
  resetGlobal: () =>
    request('/api/huoyuan-config/global', { method: 'DELETE' }),
  getHID: (hid: number) =>
    request<{ hid: number; defaults: RuntimeConfigPayload; override: RuntimeConfigPayload; effective: RuntimeConfigPayload }>(
      `/api/huoyuan-config/hid/${hid}`,
    ),
  saveHID: (hid: number, config: RuntimeConfigPayload, remark = '') =>
    request(`/api/huoyuan-config/hid/${hid}`, {
      method: 'PUT',
      body: JSON.stringify({ config, remark }),
    }),
  resetHID: (hid: number) =>
    request(`/api/huoyuan-config/hid/${hid}`, { method: 'DELETE' }),
}

export function emptyRuntimeConfig(): RuntimeConfigPayload {
  return { queue: {}, order_status: {}, rate_limit: {}, resubmit: {}, submit: {} }
}

/** 从全局配置视图取运维配置，兼容旧后端未返回 ops 字段的情况 */
export function opsFromRuntimeView(view: RuntimeConfigView): import('@/api/ops').OpsConfig | null {
  const src = view.ops ?? view.ops_defaults
  if (!src) return null
  return JSON.parse(JSON.stringify(src)) as import('@/api/ops').OpsConfig
}
