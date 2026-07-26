import { getSessionToken, parseApiResponse, type ApiResponse } from '@/api/auth'

export interface SystemInfo {
  product_name: string
  product_version: string
  platform: string
}

export interface SystemMonitor {
  system: {
    os: string
    arch: string
    hostname: string
  }
  runtime: {
    pid: number
    uptime_seconds: number
    uptime_text: string
    started_at: string
  }
  process: {
    memory_bytes: number
    memory_text: string
    user: string
  }
  cpu: {
    usage_percent: number
    cores: number
    load_avg_5?: number
    load_avg_15?: number
  }
  memory: {
    usage_percent: number
    total_bytes: number
    used_bytes: number
    free_bytes: number
    total_text: string
    used_text: string
    free_text: string
  }
}

export interface ReleaseInfo {
  has_update: boolean
  version: string
  changelog?: string
  download_url?: string
  sha256?: string
  size?: number
  force?: boolean
  published_at?: string
}

export interface UpgradeStatus {
  phase: string
  message: string
  progress: number
  current_version: string
  target_version?: string
  error?: string
  release?: ReleaseInfo
  started_at?: string
  completed_at?: string
}

export interface EngineCounterSnapshot {
  success: number
  fail: number
  dlq: number
}

export interface EngineConnStatus {
  ready: boolean
  message?: string
  addr?: string
}

export interface EngineConnections {
  redis: EngineConnStatus
  main_mysql: EngineConnStatus
  plugin_mysql: EngineConnStatus
}

export interface EngineChannel {
  hid: number
  name: string
  queue_depth: number
  processing_depth: number
  dlq_depth: number
  workers: number
  window_success: number
  window_fail: number
  window_dlq: number
  ops_paused?: boolean
}

export interface EngineStats {
  window_minutes: number
  window: EngineCounterSnapshot
  today: EngineCounterSnapshot
  lifetime: EngineCounterSnapshot
  engine_running: boolean
  connections: EngineConnections
  channels: EngineChannel[]
}

export interface SettingsRedis {
  addr: string
  addr_configured: boolean
  pass: string
  db: number
  pass_set: boolean
}

export interface SettingsHTTPSecurity {
  host_whitelist: string[]
  block_private_networks: boolean
  allow_insecure_http_to_lan: boolean
}

export interface SettingsAuth {
  authcode: string
  authcode_set: boolean
}

export interface SettingsAI {
  enabled: boolean
  base_url: string
  model: string
  api_key: string
  api_key_set: boolean
}

export interface SettingsInternalEnqueue {
  url: string
  token: string
  ready: boolean
}

export interface SystemSettings {
  redis: SettingsRedis
  http_security: SettingsHTTPSecurity
  auth: SettingsAuth
  ai: SettingsAI
  internal_enqueue: SettingsInternalEnqueue
  need_restart?: boolean
}

export interface SettingsUpdatePayload {
  redis?: Partial<SettingsRedis>
  http_security?: Partial<SettingsHTTPSecurity>
  auth?: Partial<SettingsAuth>
  ai?: Partial<SettingsAI>
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

export const systemApi = {
  info: () => request<SystemInfo>('/api/system/info'),
  monitor: () => request<SystemMonitor>('/api/system/monitor'),
  engineStats: () => request<EngineStats>('/api/system/engine-stats'),
  settings: () => request<SystemSettings>('/api/system/settings'),
  saveSettings: (payload: SettingsUpdatePayload) =>
    request<SystemSettings>('/api/system/settings', {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),
  testRedis: (payload: { addr?: string; pass?: string; db: number }) =>
    request<null>('/api/system/settings/test-redis', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  regenerateInternalEnqueueSecret: () =>
    request<SettingsInternalEnqueue>('/api/system/settings/internal-enqueue/regenerate', {
      method: 'POST',
      body: '{}',
    }),
  saveInternalEnqueueSecret: (token: string) =>
    request<SettingsInternalEnqueue>('/api/system/settings/internal-enqueue', {
      method: 'PUT',
      body: JSON.stringify({ token }),
    }),
  upgradeStatus: () => request<UpgradeStatus>('/api/system/upgrade/status'),
  checkUpgrade: () =>
    request<{ current_version: string; release: ReleaseInfo }>('/api/system/upgrade/check', {
      method: 'POST',
      body: '{}',
    }),
  applyUpgrade: (version?: string) =>
    request<UpgradeStatus>('/api/system/upgrade/apply', {
      method: 'POST',
      body: JSON.stringify(version ? { version } : {}),
    }),
}

export function formatFileSize(bytes?: number): string {
  if (!bytes || bytes <= 0) return '未知'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}

export function phaseLabel(phase: string): string {
  const map: Record<string, string> = {
    idle: '就绪',
    checking: '检查中',
    downloading: '下载中',
    verifying: '校验中',
    applying: '安装中',
    restarting: '重启中',
    failed: '失败',
    completed: '完成',
  }
  return map[phase] || phase
}
