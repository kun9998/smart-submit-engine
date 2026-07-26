export interface AuthStatus {
  installed: boolean
  logged_in: boolean
  username?: string
}

export interface ApiResponse<T = unknown> {
  code: number
  msg: string
  data?: T
  need_login?: boolean
  need_install?: boolean
  need_totp?: boolean
}

export interface DbConnectionParams {
  host: string
  port: number
  user: string
  db_password: string
  database: string
}

export interface InstallParams {
  main_host: string
  main_port: number
  main_user: string
  main_db_password: string
  main_database: string
  table_prefix: string
  plugin_host: string
  plugin_port: number
  plugin_user: string
  plugin_db_password: string
  plugin_database: string
  username: string
  password: string
  confirm_password: string
  authcode: string
}

const TOKEN_KEY = 'tj_session_token'
const USER_KEY = 'tj_username'

export function getSessionToken(): string {
  return localStorage.getItem(TOKEN_KEY) || ''
}

export function setSession(token: string, username: string) {
  localStorage.setItem(TOKEN_KEY, token)
  localStorage.setItem(USER_KEY, username)
}

export function clearSession() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
}

export function getUsername(): string {
  return localStorage.getItem(USER_KEY) || ''
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

export async function parseApiResponse<T>(res: Response): Promise<ApiResponse<T>> {
  const ct = res.headers.get('content-type') || ''
  if (!ct.includes('application/json')) {
    const text = (await res.text()).trim()
    if (text.startsWith('<')) {
      return {
        code: -1,
        msg: res.ok
          ? '服务返回异常页面，请确认反向代理已放行 /api 且后端已更新'
          : `请求失败（HTTP ${res.status}），服务可能正在重启`,
      }
    }
    return {
      code: -1,
      msg: text ? text.slice(0, 200) : `请求失败（HTTP ${res.status}）`,
    }
  }
  return res.json()
}

async function request<T>(path: string, options: RequestInit = {}): Promise<ApiResponse<T>> {
  const res = await fetch(path, {
    ...options,
    headers: { ...authHeaders(), ...(options.headers as Record<string, string>) },
  })
  return parseApiResponse<T>(res)
}

export const authApi = {
  status: () => request<AuthStatus>('/api/auth/status'),
  login: (username: string, password: string, totp_code?: string) =>
    request<{ token: string; username: string }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password, totp_code: totp_code || '' }),
    }),
  logout: () => request('/api/auth/logout', { method: 'POST' }),
  forgotPasswordSendCode: (username: string) =>
    request<{ verify_token?: string }>('/api/auth/forgot-password/send-code', {
      method: 'POST',
      body: JSON.stringify({ username }),
    }),
  forgotPasswordReset: (body: {
    username: string
    verify_code: string
    verify_token: string
    new_password: string
    confirm_password: string
  }) =>
    request('/api/auth/forgot-password/reset', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  testDB: (params: DbConnectionParams, db_type: 'main' | 'plugin' = 'plugin') =>
    request('/api/install/test-db', {
      method: 'POST',
      body: JSON.stringify({ ...params, db_type }),
    }),
  install: (body: InstallParams) =>
    request<{ token?: string; username?: string; rules_loaded?: number }>('/api/install', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
}
