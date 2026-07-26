<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { Activity, Cpu, HardDrive, Server } from '@lucide/vue'
import { toast } from 'vue-sonner'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { systemApi, type SystemMonitor } from '@/api/system'

const loading = ref(true)
const monitor = ref<SystemMonitor | null>(null)

let pollTimer: ReturnType<typeof setInterval> | null = null

function clampPercent(n: number) {
  if (!Number.isFinite(n)) return 0
  return Math.min(100, Math.max(0, n))
}

function formatPercent(n: number) {
  return `${Math.round(clampPercent(n))}%`
}

async function loadMonitor(silent = false) {
  if (!silent) loading.value = true
  try {
    const res = await systemApi.monitor()
    if (res.code === 1 && res.data) {
      monitor.value = res.data
    } else if (!silent) {
      toast.error(res.msg || '加载监控数据失败')
    }
  } finally {
    if (!silent) loading.value = false
  }
}

onMounted(async () => {
  await loadMonitor()
  pollTimer = setInterval(() => loadMonitor(true), 10000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div class="space-y-6">
    <div v-if="loading" class="grid gap-4 md:grid-cols-2">
      <Skeleton class="h-40" />
      <Skeleton class="h-40" />
      <Skeleton class="h-48 md:col-span-2" />
    </div>

    <template v-else-if="monitor">
      <div class="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <Server class="size-4" />
              系统信息
            </CardTitle>
          </CardHeader>
          <CardContent class="space-y-3 text-sm">
            <div class="flex justify-between gap-4">
              <span class="text-muted-foreground">操作系统</span>
              <span class="font-medium">{{ monitor.system.os }}</span>
            </div>
            <div class="flex justify-between gap-4">
              <span class="text-muted-foreground">系统架构</span>
              <span class="font-medium">{{ monitor.system.arch }}</span>
            </div>
            <div class="flex justify-between gap-4">
              <span class="text-muted-foreground">服务器名称</span>
              <span class="font-medium truncate max-w-[60%] text-right">{{ monitor.system.hostname || '-' }}</span>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <Activity class="size-4" />
              运行信息
            </CardTitle>
          </CardHeader>
          <CardContent class="space-y-3 text-sm">
            <div class="flex justify-between gap-4">
              <span class="text-muted-foreground">进程 ID</span>
              <span class="font-medium">{{ monitor.runtime.pid }}</span>
            </div>
            <div class="flex justify-between gap-4">
              <span class="text-muted-foreground">运行时长</span>
              <span class="font-medium text-right">{{ monitor.runtime.uptime_text }}</span>
            </div>
            <div class="flex justify-between gap-4">
              <span class="text-muted-foreground">启动时间</span>
              <span class="font-medium">{{ monitor.runtime.started_at }}</span>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <HardDrive class="size-4" />
              资源信息
            </CardTitle>
            <CardDescription>当前进程占用</CardDescription>
          </CardHeader>
          <CardContent class="space-y-3 text-sm">
            <div class="flex justify-between gap-4">
              <span class="text-muted-foreground">运行内存</span>
              <span class="font-medium">{{ monitor.process.memory_text }}</span>
            </div>
            <div class="flex justify-between gap-4">
              <span class="text-muted-foreground">运行用户</span>
              <span class="font-medium">{{ monitor.process.user }}</span>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <Cpu class="size-4" />
              CPU 使用情况
            </CardTitle>
          </CardHeader>
          <CardContent class="space-y-4 text-sm">
            <div class="space-y-2">
              <div class="flex justify-between">
                <span class="text-muted-foreground">CPU 使用率</span>
                <span class="font-medium">{{ formatPercent(monitor.cpu.usage_percent) }}</span>
              </div>
              <div class="h-2 overflow-hidden rounded-full bg-muted">
                <div
                  class="h-full bg-primary transition-all duration-500"
                  :style="{ width: `${clampPercent(monitor.cpu.usage_percent)}%` }"
                />
              </div>
            </div>
            <div class="flex justify-between gap-4">
              <span class="text-muted-foreground">核心数</span>
              <span class="font-medium">{{ monitor.cpu.cores }}</span>
            </div>
            <div class="grid grid-cols-2 gap-3 pt-1">
              <div class="rounded-md border bg-muted/30 px-3 py-2">
                <div class="text-xs text-muted-foreground">Avg 5</div>
                <div class="font-medium">{{ monitor.cpu.load_avg_5?.toFixed(2) ?? '-' }}</div>
              </div>
              <div class="rounded-md border bg-muted/30 px-3 py-2">
                <div class="text-xs text-muted-foreground">Avg 15</div>
                <div class="font-medium">{{ monitor.cpu.load_avg_15?.toFixed(2) ?? '-' }}</div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card class="md:col-span-2">
          <CardHeader>
            <CardTitle class="text-base">内存使用情况</CardTitle>
          </CardHeader>
          <CardContent class="space-y-4 text-sm">
            <div class="space-y-2">
              <div class="flex justify-between">
                <span class="text-muted-foreground">内存使用率</span>
                <span class="font-medium">{{ formatPercent(monitor.memory.usage_percent) }}</span>
              </div>
              <div class="h-2 overflow-hidden rounded-full bg-muted">
                <div
                  class="h-full bg-emerald-500 transition-all duration-500"
                  :style="{ width: `${clampPercent(monitor.memory.usage_percent)}%` }"
                />
              </div>
            </div>
            <div class="grid gap-3 sm:grid-cols-3">
              <div class="rounded-md border px-3 py-2">
                <div class="text-xs text-muted-foreground">总量</div>
                <div class="font-medium">{{ monitor.memory.total_text || '-' }}</div>
              </div>
              <div class="rounded-md border px-3 py-2">
                <div class="text-xs text-muted-foreground">已使用</div>
                <div class="font-medium">{{ monitor.memory.used_text || '-' }}</div>
              </div>
              <div class="rounded-md border px-3 py-2">
                <div class="text-xs text-muted-foreground">剩余</div>
                <div class="font-medium">{{ monitor.memory.free_text || '-' }}</div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </template>
  </div>
</template>
