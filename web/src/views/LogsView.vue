<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { Pause, Play, ScrollText, Trash2 } from '@lucide/vue'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { logsApi, type LogEntry } from '@/api/logs'

type DisplayLogEntry = LogEntry & { id: number }

const router = useRouter()
const MAX_ENTRIES = 800
const entries = shallowRef<DisplayLogEntry[]>([])
const autoScroll = ref(true)
const connected = ref(false)
const scrollRef = ref<InstanceType<typeof ScrollArea> | null>(null)

let es: EventSource | null = null
let nextId = 0
const pending: DisplayLogEntry[] = []
let flushRaf = 0
let scrollRaf = 0

function levelClass(level: string) {
  if (level === 'success') return 'text-emerald-400'
  if (level === 'error') return 'text-red-500'
  if (level === 'warn') return 'text-amber-500'
  return 'text-sky-400'
}

function levelLabel(level: string) {
  if (level === 'success') return '成功'
  if (level === 'error') return '失败'
  if (level === 'warn') return '提醒'
  return '信息'
}

function formatTime(iso: string) {
  try {
    return new Date(iso).toLocaleTimeString('zh-CN', { hour12: false })
  } catch {
    return iso
  }
}

function trimEntries(list: DisplayLogEntry[]) {
  if (list.length <= MAX_ENTRIES) return list
  return list.slice(-MAX_ENTRIES)
}

function scheduleFlush() {
  if (flushRaf) return
  flushRaf = requestAnimationFrame(() => {
    flushRaf = 0
    if (!pending.length) return
    const batch = pending.splice(0, pending.length)
    entries.value = trimEntries(entries.value.length ? entries.value.concat(batch) : batch)
    if (autoScroll.value) scheduleScroll()
  })
}

function enqueueEntry(item: LogEntry) {
  pending.push({ ...item, id: ++nextId })
  scheduleFlush()
}

async function loadHistory() {
  const res = await logsApi.list(500)
  if (res.code === 1 && res.data) {
    entries.value = res.data.map((item) => ({ ...item, id: ++nextId }))
    await scrollToBottom()
  } else if (res.need_login) {
    await router.push('/login')
  }
}

function connectStream() {
  disconnectStream()
  es = new EventSource(logsApi.streamUrl())
  es.onopen = () => {
    connected.value = true
  }
  es.onmessage = (ev) => {
    connected.value = true
    try {
      const item = JSON.parse(ev.data) as LogEntry
      enqueueEntry(item)
    } catch {
      /* ignore */
    }
  }
  es.onerror = () => {
    connected.value = false
    es?.close()
    es = null
    setTimeout(connectStream, 3000)
  }
}

function disconnectStream() {
  es?.close()
  es = null
  connected.value = false
}

function scheduleScroll() {
  if (scrollRaf) return
  scrollRaf = requestAnimationFrame(() => {
    scrollRaf = 0
    void scrollToBottom()
  })
}

async function scrollToBottom() {
  await nextTick()
  const root = scrollRef.value?.$el as HTMLElement | undefined
  const viewport = root?.querySelector('[data-slot="scroll-area-viewport"]') as HTMLElement | null
  if (viewport) viewport.scrollTop = viewport.scrollHeight
}

async function clearLogs() {
  const res = await logsApi.clear()
  if (res.code === 1) {
    pending.length = 0
    entries.value = []
    toast.success('已清空')
  } else {
    toast.error(res.msg || '清空失败')
  }
}

onMounted(async () => {
  await loadHistory()
  connectStream()
})

onUnmounted(() => {
  disconnectStream()
  if (flushRaf) cancelAnimationFrame(flushRaf)
  if (scrollRaf) cancelAnimationFrame(scrollRaf)
})
</script>

<template>
  <Card class="flex min-h-[calc(100vh-7rem)] flex-col">
    <CardHeader class="flex-row items-center justify-between space-y-0 pb-3">
      <div class="flex items-center gap-2">
        <ScrollText class="size-5 text-muted-foreground" />
        <span
          class="rounded-full px-2 py-0.5 text-xs"
          :class="connected ? 'bg-emerald-500/15 text-emerald-600' : 'bg-muted text-muted-foreground'"
        >
          {{ connected ? '实时' : '重连中' }}
        </span>
        <span v-if="entries.length" class="text-xs text-muted-foreground">共 {{ entries.length }} 条</span>
      </div>
      <div class="flex gap-2">
        <Button variant="outline" size="sm" type="button" @click="autoScroll = !autoScroll">
          <component :is="autoScroll ? Pause : Play" class="mr-1 size-4" />
          {{ autoScroll ? '暂停滚动' : '自动滚动' }}
        </Button>
        <Button variant="outline" size="sm" type="button" @click="clearLogs">
          <Trash2 class="mr-1 size-4" />
          清空
        </Button>
      </div>
    </CardHeader>
    <CardContent class="min-h-0 flex-1 pb-4">
      <ScrollArea ref="scrollRef" class="h-[calc(100vh-11rem)] rounded-md border bg-zinc-950 p-3">
        <div v-if="!entries.length" class="py-8 text-center text-sm text-muted-foreground">
          暂无提交记录，等待订单处理…
        </div>
        <div v-else class="space-y-0.5 font-mono text-xs leading-relaxed">
          <div v-for="line in entries" :key="line.id" class="flex gap-2">
            <span class="shrink-0 text-muted-foreground">{{ formatTime(line.time) }}</span>
            <span
              class="shrink-0 rounded px-1 py-px text-[10px] font-sans font-medium leading-4"
              :class="{
                'bg-emerald-500/15 text-emerald-500': line.level === 'success',
                'bg-red-500/15 text-red-500': line.level === 'error',
                'bg-amber-500/15 text-amber-500': line.level === 'warn',
                'bg-sky-500/15 text-sky-400': line.level === 'info',
              }"
            >
              {{ levelLabel(line.level) }}
            </span>
            <span class="whitespace-pre-wrap break-all" :class="levelClass(line.level)">{{ line.message }}</span>
          </div>
        </div>
      </ScrollArea>
    </CardContent>
  </Card>
</template>
