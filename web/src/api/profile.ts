import { getSessionToken, type ApiResponse } from '@/api/auth'

export interface AdminProfile {
  username: string
  showdoc_url?: string
  showdoc_bound: boolean
  totp_enabled: boolean
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
  const res = await fetch(path, { ...options, headers: { ...authHeaders(), ...(options.headers as Record<string, string>) } })
  return res.json()
}

export interface NotificationConfig {
  enabled: boolean
  notify_submit_failure: boolean
  notify_submit_timeout: boolean
  notify_db_write_failure: boolean
  notify_processing_timeout: boolean
}

export interface NotificationConfigView {
  showdoc_bound: boolean
  config: NotificationConfig
  defaults: NotificationConfig
}

export const profileApi = {
  get: () => request<AdminProfile>('/api/profile'),
  showdocSendCode: (url: string) =>
    request<{ verify_token: string }>('/api/profile/showdoc/send-code', {
      method: 'POST',
      body: JSON.stringify({ url }),
    }),
  showdocBind: (body: { url: string; code: string; verify_token: string }) =>
    request('/api/profile/showdoc/bind', { method: 'POST', body: JSON.stringify(body) }),
  showdocUnbind: () => request('/api/profile/showdoc/unbind', { method: 'POST' }),
  showdocTest: () => request('/api/profile/showdoc/test', { method: 'POST' }),
  getNotifications: () => request<NotificationConfigView>('/api/profile/notifications'),
  saveNotifications: (config: NotificationConfig) =>
    request<NotificationConfig>('/api/profile/notifications', {
      method: 'PUT',
      body: JSON.stringify(config),
    }),
  changePassword: (body: {
    old_password: string
    new_password: string
    confirm_password: string
    totp_code?: string
  }) => request('/api/profile/password', { method: 'POST', body: JSON.stringify(body) }),
  totpSendCode: () => request<{ verify_token: string }>('/api/profile/totp/send-code', { method: 'POST' }),
  totpSetup: () => request<{ secret: string; otpauth_url: string }>('/api/profile/totp/setup', { method: 'POST' }),
  totpEnable: (body: {
    secret: string
    totp_code: string
    verify_code: string
    verify_token: string
  }) => request('/api/profile/totp/enable', { method: 'POST', body: JSON.stringify(body) }),
  totpDisable: (body: { password: string; totp_code: string }) =>
    request('/api/profile/totp/disable', { method: 'POST', body: JSON.stringify(body) }),
}
