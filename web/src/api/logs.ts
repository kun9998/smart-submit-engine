import { getSessionToken, type ApiResponse } from '@/api/auth'

export interface LogEntry {
  time: string
  message: string
  level: 'info' | 'warn' | 'error' | 'success'
}

export const logsApi = {
  list: async (limit = 500): Promise<ApiResponse<LogEntry[]>> => {
    const res = await fetch(`/api/logs?limit=${limit}`, {
      headers: authHeaders(),
    })
    return res.json()
  },
  clear: async (): Promise<ApiResponse> => {
    const res = await fetch('/api/logs', {
      method: 'DELETE',
      headers: authHeaders(),
    })
    return res.json()
  },
  streamUrl(): string {
    const token = getSessionToken()
    // 始终走当前访问域名/端口（生产与 go run 均适用；Vite 开发模式由 proxy 转发 /api）
    const q = token ? `?token=${encodeURIComponent(token)}` : ''
    return `/api/logs/stream${q}`
  },
}

function authHeaders(): Record<string, string> {
  const token = getSessionToken()
  const h: Record<string, string> = {}
  if (token) {
    h['Authorization'] = `Bearer ${token}`
    h['X-Session-Token'] = token
  }
  return h
}
