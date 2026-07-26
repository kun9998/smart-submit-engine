<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { Activity, CheckCircle2, Server, Skull, XCircle } from '@lucide/vue'
import { toast } from 'vue-sonner'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { systemApi, type EngineStats } from '@/api/system'

const loading = ref(true)
const stats = ref<EngineStats | null>(null)

let pollTimer: ReturnType<typeof setInterval> | null = null

function connBadge(ready: boolean) {
  return ready ? 'default' : 'destructive'
}

async function loadStats(silent = false) {
  if (!silent) loading.value = true
  try {
    const res = await systemApi.engineStats()
    if (res.code === 1 && res.data) {
      stats.value = res.data
    } else if (!silent) {
      toast.error(res.msg || '加载失败')
    }
  } finally {
    if (!silent) loading.value = false
  }
}

onMounted(async () => {
  await loadStats()
  pollTimer = setInterval(() => loadStats(true), 10000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div class="space-y-6">
    <div v-if="loading" class="grid gap-4 md:grid-cols-3">
      <Skeleton class="h-28" />
      <Skeleton class="h-28" />
      <Skeleton class="h-28" />
      <Skeleton class="h-64 md:col-span-3" />
    </div>

    <template v-else-if="stats">
      <div class="flex flex-wrap items-center gap-2">
        <Badge :variant="stats.engine_running ? 'default' : 'secondary'">
          <Activity class="mr-1 size-3" />
          {{ stats.engine_running ? '订单处理中' : '未在处理订单' }}
        </Badge>
        <span class="text-xs text-muted-foreground">每 10 秒自动刷新</span>
      </div>

      <div class="grid gap-4 lg:grid-cols-3">
        <Card v-for="(label, key) in { window: '近 30 分钟', today: '今天', lifetime: '累计' }" :key="key">
          <CardHeader class="pb-2">
            <CardTitle class="text-base">{{ label }}</CardTitle>
          </CardHeader>
          <CardContent class="grid grid-cols-3 gap-2 text-center text-sm">
            <div>
              <div class="flex items-center justify-center gap-1 text-emerald-600">
                <CheckCircle2 class="size-4" />
                <span class="font-semibold tabular-nums">{{ stats[key as 'window' | 'today' | 'lifetime'].success }}</span>
              </div>
              <p class="text-xs text-muted-foreground">成功</p>
            </div>
            <div>
              <div class="flex items-center justify-center gap-1 text-amber-600">
                <XCircle class="size-4" />
                <span class="font-semibold tabular-nums">{{ stats[key as 'window' | 'today' | 'lifetime'].fail }}</span>
              </div>
              <p class="text-xs text-muted-foreground">失败</p>
            </div>
            <div>
              <div class="flex items-center justify-center gap-1 text-red-600">
                <Skull class="size-4" />
                <span class="font-semibold tabular-nums">{{ stats[key as 'window' | 'today' | 'lifetime'].dlq }}</span>
              </div>
              <p class="text-xs text-muted-foreground">失败待处理</p>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle class="flex items-center gap-2 text-base">
            <Server class="size-4" />
            服务连接
          </CardTitle>
        </CardHeader>
        <CardContent class="grid gap-3 sm:grid-cols-3">
          <div
            v-for="item in [
              { key: 'redis', label: 'Redis（队列）', conn: stats.connections.redis },
              { key: 'main', label: '主站数据库', conn: stats.connections.main_mysql },
              { key: 'plugin', label: '插件数据库', conn: stats.connections.plugin_mysql },
            ]"
            :key="item.key"
            class="rounded-lg border p-3"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="text-sm font-medium">{{ item.label }}</span>
              <Badge :variant="connBadge(item.conn.ready)">{{ item.conn.ready ? '正常' : '异常' }}</Badge>
            </div>
            <p v-if="item.conn.message && !item.conn.ready" class="mt-1 text-xs text-destructive">{{ item.conn.message }}</p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle class="text-base">各渠道订单情况</CardTitle>
        </CardHeader>
        <CardContent>
          <div v-if="stats.channels.length === 0" class="py-8 text-center text-sm text-muted-foreground">
            还没有渠道数据（有待处理订单后会自动出现）
          </div>
          <Table v-else>
            <TableHeader>
              <TableRow>
                <TableHead>渠道</TableHead>
                <TableHead class="text-right">排队中</TableHead>
                <TableHead class="text-right">正在处理</TableHead>
                <TableHead class="text-right">失败堆积</TableHead>
                <TableHead class="text-right">处理线程</TableHead>
                <TableHead class="text-right">近30分成功</TableHead>
                <TableHead class="text-right">近30分失败</TableHead>
                <TableHead class="text-right">近30分待处理</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="ch in stats.channels" :key="ch.hid">
                <TableCell class="font-medium">
                  <div class="flex flex-wrap items-center gap-2">
                    {{ ch.name }}
                    <Badge v-if="ch.ops_paused" variant="outline" class="text-amber-600">已停用</Badge>
                  </div>
                </TableCell>
                <TableCell class="text-right tabular-nums">{{ ch.queue_depth }}</TableCell>
                <TableCell class="text-right tabular-nums">{{ ch.processing_depth }}</TableCell>
                <TableCell class="text-right tabular-nums">{{ ch.dlq_depth }}</TableCell>
                <TableCell class="text-right tabular-nums">{{ ch.workers }}</TableCell>
                <TableCell class="text-right tabular-nums text-emerald-600">{{ ch.window_success }}</TableCell>
                <TableCell class="text-right tabular-nums text-amber-600">{{ ch.window_fail }}</TableCell>
                <TableCell class="text-right tabular-nums text-red-600">{{ ch.window_dlq }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </template>
  </div>
</template>
