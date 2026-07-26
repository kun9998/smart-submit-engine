<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { RefreshCw, RotateCcw, Search, Settings2, X } from '@lucide/vue'
import { toast } from 'vue-sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import {
  huoyuanConfigApi,
  emptyRuntimeConfig,
  opsFromRuntimeView,
  type HuoyuanItem,
  type RuntimeConfigPayload,
  type RuntimeConfigView,
} from '@/api/huoyuan-config'
import { opsModeDescription, type OpsConfig } from '@/api/ops'

type Section = 'global' | 'list'

const router = useRouter()
const loading = ref(false)
const activeSection = ref<Section>('global')
const huoyuanList = ref<HuoyuanItem[]>([])
const globalView = ref<RuntimeConfigView | null>(null)
const globalForm = ref<RuntimeConfigPayload>(emptyRuntimeConfig())
const opsForm = ref<OpsConfig | null>(null)
const successCodesText = ref('')
const terminalKeywordsText = ref('')

const showHIDDialog = ref(false)
const resetGlobalConfirmOpen = ref(false)
const resetHIDConfirmOpen = ref(false)
const editingHID = ref<HuoyuanItem | null>(null)
const hidForm = ref<RuntimeConfigPayload>(emptyRuntimeConfig())
const hidRemark = ref('')
const hidSuccessCodesText = ref('')
const hidTerminalKeywordsText = ref('')
const searchQuery = ref('')

const sortedList = computed(() =>
  [...huoyuanList.value].sort((a, b) => a.hid - b.hid),
)

const filteredList = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return sortedList.value
  return sortedList.value.filter(
    (row) =>
      String(row.hid).includes(q) ||
      row.name.toLowerCase().includes(q) ||
      row.pt.toLowerCase().includes(q),
  )
})

function numVal(v: number | null | undefined): string {
  return v != null && v !== 0 ? String(v) : ''
}

function setNum(obj: Record<string, unknown>, key: string, raw: string) {
  const t = raw.trim()
  if (t === '') {
    delete obj[key]
    return
  }
  const n = Number(t)
  if (!Number.isNaN(n)) obj[key] = n
}

function parseSuccessCodes(text: string): number[] | undefined {
  const t = text.trim()
  if (!t) return undefined
  const parts = t.split(/[,，\s]+/).map((s) => s.trim()).filter(Boolean)
  const nums = parts.map((p) => Number(p)).filter((n) => !Number.isNaN(n))
  return nums.length ? nums : undefined
}

function successCodesToText(codes?: number[] | null) {
  if (!codes?.length) return ''
  return codes.join(', ')
}

function parseKeywords(text: string): string[] | undefined {
  const t = text.trim()
  if (!t) return undefined
  const parts = t.split(/[,，\n]+/).map((s) => s.trim()).filter(Boolean)
  return parts.length ? parts : undefined
}

function keywordsToText(keywords?: string[] | null) {
  if (!keywords?.length) return ''
  return keywords.join(', ')
}

function fillGlobalFormFromView(view: RuntimeConfigView) {
  const eff = view.effective ?? {}
  globalForm.value = {
    queue: { ...(eff.queue ?? {}) },
    order_status: { ...(eff.order_status ?? {}) },
    rate_limit: { ...(eff.rate_limit ?? {}) },
    resubmit: {
      ...(eff.resubmit ?? {}),
      dlq_auto_retry: { ...(eff.resubmit?.dlq_auto_retry ?? {}) },
    },
    submit: { ...(eff.submit ?? {}) },
  }
  successCodesText.value = successCodesToText(eff.order_status?.success_codes)
  terminalKeywordsText.value = keywordsToText(eff.resubmit?.terminal_keywords)
}

async function loadAll() {
  loading.value = true
  try {
    const [listRes, globalRes] = await Promise.all([
      huoyuanConfigApi.listHuoyuan(),
      huoyuanConfigApi.getGlobal(),
    ])
    if (listRes.need_login || globalRes.need_login) {
      await router.push('/login')
      return
    }
    if (listRes.code !== 1) {
      toast.error(listRes.msg || '货源列表加载失败')
    } else if (listRes.data) {
      huoyuanList.value = listRes.data
    }
    if (globalRes.code !== 1) {
      toast.error(globalRes.msg || '全局配置加载失败')
    } else if (globalRes.data) {
      globalView.value = globalRes.data
      fillGlobalFormFromView(globalRes.data)
      opsForm.value = opsFromRuntimeView(globalRes.data)
    }
  } catch (e) {
    const msg = e instanceof Error ? e.message : ''
    toast.error(msg ? `加载失败：${msg}` : '加载失败，请检查后端是否已更新并重启')
  } finally {
    loading.value = false
  }
}

function buildPayloadFromForm(
  form: RuntimeConfigPayload,
  codesText: string,
  keywordsText: string,
): RuntimeConfigPayload {
  const payload: RuntimeConfigPayload = {}
  if (form.queue && Object.keys(form.queue).length) payload.queue = { ...form.queue }
  if (form.rate_limit && Object.keys(form.rate_limit).length) payload.rate_limit = { ...form.rate_limit }
  const os = { ...(form.order_status ?? {}) }
  const codes = parseSuccessCodes(codesText)
  if (codes) os.success_codes = codes
  else delete os.success_codes
  if (os.submitted_status === '') delete os.submitted_status
  if (os.submitted_remarks === '') delete os.submitted_remarks
  if (Object.keys(os).length) payload.order_status = os

  const rs = { ...(form.resubmit ?? {}) }
  const dlq = { ...(rs.dlq_auto_retry ?? {}) }
  const keywords = parseKeywords(keywordsText)
  if (keywords) rs.terminal_keywords = keywords
  else delete rs.terminal_keywords
  if (Object.keys(dlq).length) rs.dlq_auto_retry = dlq
  else delete rs.dlq_auto_retry
  if (Object.keys(rs).length) payload.resubmit = rs

  const sub = { ...(form.submit ?? {}) }
  if (Object.keys(sub).length) payload.submit = sub

  return payload
}

async function saveGlobal() {
  loading.value = true
  try {
    const config = buildPayloadFromForm(globalForm.value, successCodesText.value, terminalKeywordsText.value)
    const res = await huoyuanConfigApi.saveGlobal(config, opsForm.value ?? undefined)
    if (res.code === 1) {
      toast.success('已保存，立即生效')
      if (res.data) {
        globalView.value = res.data
        fillGlobalFormFromView(res.data)
        opsForm.value = opsFromRuntimeView(res.data)
      }
    } else {
      toast.error(res.msg || '保存失败')
    }
  } finally {
    loading.value = false
  }
}

async function confirmResetGlobal() {
  resetGlobalConfirmOpen.value = false
  loading.value = true
  try {
    const res = await huoyuanConfigApi.resetGlobal()
    if (res.code === 1) {
      toast.success('已恢复默认')
      await loadAll()
    } else {
      toast.error(res.msg || '操作失败')
    }
  } finally {
    loading.value = false
  }
}

async function openHIDEdit(row: HuoyuanItem) {
  editingHID.value = row
  hidRemark.value = row.remark ?? ''
  hidForm.value = emptyRuntimeConfig()
  hidSuccessCodesText.value = ''
  hidTerminalKeywordsText.value = ''
  showHIDDialog.value = true
  const res = await huoyuanConfigApi.getHID(row.hid)
  if (res.code === 1 && res.data) {
    const eff = res.data.effective ?? {}
    hidForm.value = {
      queue: { ...(eff.queue ?? {}) },
      order_status: { ...(eff.order_status ?? {}) },
      rate_limit: { ...(eff.rate_limit ?? {}) },
      resubmit: {
        ...(eff.resubmit ?? {}),
        dlq_auto_retry: { ...(eff.resubmit?.dlq_auto_retry ?? {}) },
      },
      submit: { ...(eff.submit ?? {}) },
    }
    hidSuccessCodesText.value = successCodesToText(eff.order_status?.success_codes)
    hidTerminalKeywordsText.value = keywordsToText(eff.resubmit?.terminal_keywords)
  }
}

async function saveHID() {
  if (!editingHID.value) return
  loading.value = true
  try {
    const config = buildPayloadFromForm(hidForm.value, hidSuccessCodesText.value, hidTerminalKeywordsText.value)
    const res = await huoyuanConfigApi.saveHID(editingHID.value.hid, config, hidRemark.value)
    if (res.code === 1) {
      toast.success('货源配置已保存')
      showHIDDialog.value = false
      await loadAll()
    } else {
      toast.error(res.msg || '保存失败')
    }
  } finally {
    loading.value = false
  }
}

async function confirmResetHID() {
  if (!editingHID.value) return
  resetHIDConfirmOpen.value = false
  loading.value = true
  try {
    const res = await huoyuanConfigApi.resetHID(editingHID.value.hid)
    if (res.code === 1) {
      toast.success('已删除单独配置')
      showHIDDialog.value = false
      await loadAll()
    } else {
      toast.error(res.msg || '操作失败')
    }
  } finally {
    loading.value = false
  }
}

onMounted(loadAll)
</script>

<template>
  <Card class="flex min-h-[calc(100vh-7rem)] flex-col">
    <CardHeader class="flex-row flex-wrap items-center justify-between gap-3 space-y-0 pb-3">
      <div v-if="activeSection === 'list'" class="relative w-full sm:w-64 md:w-72">
        <Search class="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          v-model="searchQuery"
          class="h-9 pl-8 pr-8"
          placeholder="搜索货源编号、名称或平台"
        />
        <button
          v-if="searchQuery"
          type="button"
          class="absolute right-2 top-1/2 -translate-y-1/2 rounded p-0.5 text-muted-foreground hover:text-foreground"
          @click="searchQuery = ''"
        >
          <X class="size-3.5" />
        </button>
      </div>
      <div v-else class="hidden sm:block" />
      <div class="flex flex-wrap items-center gap-2">
        <div class="flex gap-1 rounded-lg border bg-muted/40 p-1">
          <button
            type="button"
            :class="cn(
              'rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
              activeSection === 'global' ? 'bg-background shadow-sm' : 'text-muted-foreground hover:text-foreground',
            )"
            @click="activeSection = 'global'"
          >
            全局
          </button>
          <button
            type="button"
            :class="cn(
              'rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
              activeSection === 'list' ? 'bg-background shadow-sm' : 'text-muted-foreground hover:text-foreground',
            )"
            @click="activeSection = 'list'"
          >
            货源列表
          </button>
        </div>
        <Button variant="outline" size="sm" :disabled="loading" @click="loadAll">
          <RefreshCw class="mr-1 size-4" /> 刷新
        </Button>
      </div>
    </CardHeader>

    <CardContent class="min-h-0 flex-1 pb-4">
      <ScrollArea class="h-[calc(100vh-11rem)] rounded-md border">
        <!-- 全局配置：全宽 -->
        <div v-show="activeSection === 'global'" class="w-full space-y-6 p-4">
            <section class="rounded-md border border-border/60 bg-muted/30 p-3 text-xs text-muted-foreground leading-relaxed space-y-2">
              <p><span class="font-medium text-foreground">怎么生效？</span> 点「保存」后，下面绝大部分选项会马上起作用，<span class="text-foreground">不用重启程序</span>。</p>
              <p>只有「提交任务排队上限」「核对任务排队上限」这两项，改完后需要<span class="text-foreground">重启程序</span>才会变。</p>
            </section>
            <section class="space-y-3">
              <h3 class="text-sm font-medium">订单队列</h3>
              <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                <div class="space-y-1">
                  <Label class="text-xs">每隔多久扫新订单（毫秒）</Label>
                  <Input
                    type="number"
                    placeholder="不填则用默认"
                    :model-value="numVal(globalForm.queue?.producer_interval_ms)"
                    @update:model-value="(v) => { if (!globalForm.queue) globalForm.queue = {}; setNum(globalForm.queue as Record<string, unknown>, 'producer_interval_ms', String(v)) }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">每个渠道最少同时处理几单</Label>
                  <Input
                    type="number"
                    placeholder="不填则用默认"
                    :model-value="numVal(globalForm.queue?.min_workers_per_hid)"
                    @update:model-value="(v) => { if (!globalForm.queue) globalForm.queue = {}; setNum(globalForm.queue as Record<string, unknown>, 'min_workers_per_hid', String(v)) }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">每个渠道最多同时处理几单</Label>
                  <Input
                    type="number"
                    placeholder="不填则用默认"
                    :model-value="numVal(globalForm.queue?.max_workers_per_hid)"
                    @update:model-value="(v) => { if (!globalForm.queue) globalForm.queue = {}; setNum(globalForm.queue as Record<string, unknown>, 'max_workers_per_hid', String(v)) }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">排队太多时多加一个处理线程</Label>
                  <Input
                    type="number"
                    placeholder="不填则用默认"
                    :model-value="numVal(globalForm.queue?.scale_step_threshold)"
                    @update:model-value="(v) => { if (!globalForm.queue) globalForm.queue = {}; setNum(globalForm.queue as Record<string, unknown>, 'scale_step_threshold', String(v)) }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">处理多久算超时（分钟）</Label>
                  <Input
                    type="number"
                    placeholder="不填则用默认"
                    :model-value="numVal(globalForm.queue?.processing_timeout_minutes)"
                    @update:model-value="(v) => { if (!globalForm.queue) globalForm.queue = {}; setNum(globalForm.queue as Record<string, unknown>, 'processing_timeout_minutes', String(v)) }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">请求超时后再等几秒查订单是否已成功</Label>
                  <Input
                    type="number"
                    placeholder="不填则用默认"
                    :model-value="numVal(globalForm.queue?.timeout_confirm_wait_seconds)"
                    @update:model-value="(v) => { if (!globalForm.queue) globalForm.queue = {}; setNum(globalForm.queue as Record<string, unknown>, 'timeout_confirm_wait_seconds', String(v)) }"
                  />
                </div>
              </div>
              <p class="text-xs text-muted-foreground">「同时处理几单」保存即生效；「排队上限」改后要重启程序。</p>
              <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <div class="space-y-1">
                  <Label class="text-xs">可同时向上游提交几单</Label>
                  <Input
                    type="number"
                    placeholder="默认 64，保存即生效"
                    :model-value="numVal(globalForm.queue?.submit_pool_workers)"
                    @update:model-value="(v) => { if (!globalForm.queue) globalForm.queue = {}; setNum(globalForm.queue as Record<string, unknown>, 'submit_pool_workers', String(v)) }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">提交任务最多排队几单</Label>
                  <Input
                    type="number"
                    placeholder="默认 512，改后需重启"
                    :model-value="numVal(globalForm.queue?.submit_pool_queue_cap)"
                    @update:model-value="(v) => { if (!globalForm.queue) globalForm.queue = {}; setNum(globalForm.queue as Record<string, unknown>, 'submit_pool_queue_cap', String(v)) }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">超时后同时核对几单</Label>
                  <Input
                    type="number"
                    placeholder="默认 16，保存即生效"
                    :model-value="numVal(globalForm.queue?.confirm_pool_workers)"
                    @update:model-value="(v) => { if (!globalForm.queue) globalForm.queue = {}; setNum(globalForm.queue as Record<string, unknown>, 'confirm_pool_workers', String(v)) }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">核对任务最多排队几单</Label>
                  <Input
                    type="number"
                    placeholder="默认 256，改后需重启"
                    :model-value="numVal(globalForm.queue?.confirm_pool_queue_cap)"
                    @update:model-value="(v) => { if (!globalForm.queue) globalForm.queue = {}; setNum(globalForm.queue as Record<string, unknown>, 'confirm_pool_queue_cap', String(v)) }"
                  />
                </div>
              </div>
            </section>

            <Separator />

            <section class="space-y-3">
              <h3 class="text-sm font-medium">提交成功后的订单展示</h3>
              <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                <div class="space-y-1">
                  <Label class="text-xs">订单状态文案</Label>
                  <Input
                    :model-value="globalForm.order_status?.submitted_status ?? ''"
                    placeholder="如：已提交"
                    @update:model-value="(v) => {
                      if (!globalForm.order_status) globalForm.order_status = {}
                      globalForm.order_status.submitted_status = String(v)
                    }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">订单备注提示</Label>
                  <Input
                    :model-value="globalForm.order_status?.submitted_remarks ?? ''"
                    placeholder="提交成功后的说明文字"
                    @update:model-value="(v) => {
                      if (!globalForm.order_status) globalForm.order_status = {}
                      globalForm.order_status.submitted_remarks = String(v)
                    }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">平台返回这些数字算成功</Label>
                  <Input v-model="successCodesText" placeholder="如：0,1" />
                </div>
              </div>
            </section>

            <Separator />

            <section class="space-y-3">
              <h3 class="text-sm font-medium">提交速度限制</h3>
              <div class="flex items-center gap-2">
                <Checkbox
                  id="rl-enabled"
                  :checked="globalForm.rate_limit?.enabled === true"
                  @update:checked="(v: boolean) => {
                    if (!globalForm.rate_limit) globalForm.rate_limit = {}
                    globalForm.rate_limit.enabled = v
                  }"
                />
                <Label for="rl-enabled" class="cursor-pointer font-normal">限制每秒提交次数</Label>
              </div>
              <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                <div class="space-y-1">
                  <Label class="text-xs">全站每秒最多提交几次</Label>
                  <Input
                    type="number"
                    placeholder="0 表示不限制"
                    :model-value="numVal(globalForm.rate_limit?.global_max_per_second)"
                    @update:model-value="(v) => { if (!globalForm.rate_limit) globalForm.rate_limit = {}; setNum(globalForm.rate_limit as Record<string, unknown>, 'global_max_per_second', String(v)) }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">每个渠道每秒最多提交几次</Label>
                  <Input
                    type="number"
                    placeholder="0 表示不限制"
                    :model-value="numVal(globalForm.rate_limit?.per_hid_max_per_second)"
                    @update:model-value="(v) => { if (!globalForm.rate_limit) globalForm.rate_limit = {}; setNum(globalForm.rate_limit as Record<string, unknown>, 'per_hid_max_per_second', String(v)) }"
                  />
                </div>
              </div>
            </section>

            <Separator />

            <section class="space-y-3">
              <h3 class="text-sm font-medium">平台下单</h3>
              <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                <div class="space-y-1">
                  <Label class="text-xs">等平台回复最久等多少秒</Label>
                  <Input
                    type="number"
                    placeholder="默认 30"
                    :model-value="numVal(globalForm.submit?.timeout_seconds)"
                    @update:model-value="(v) => { if (!globalForm.submit) globalForm.submit = {}; setNum(globalForm.submit as Record<string, unknown>, 'timeout_seconds', String(v)) }"
                  />
                </div>
              </div>
            </section>

            <Separator />

            <section class="space-y-3">
              <h3 class="text-sm font-medium">失败重试</h3>
              <div class="flex flex-wrap items-center gap-4">
                <div class="flex items-center gap-2">
                  <Checkbox
                    id="rs-enabled"
                    :checked="globalForm.resubmit?.enabled === true"
                    @update:checked="(v: boolean) => {
                      if (!globalForm.resubmit) globalForm.resubmit = {}
                      globalForm.resubmit.enabled = v
                    }"
                  />
                  <Label for="rs-enabled" class="cursor-pointer font-normal">启用自动重试</Label>
                </div>
                <div class="flex items-center gap-2">
                  <Checkbox
                    id="rs-timeout"
                    :checked="globalForm.resubmit?.retry_on_timeout === true"
                    @update:checked="(v: boolean) => {
                      if (!globalForm.resubmit) globalForm.resubmit = {}
                      globalForm.resubmit.retry_on_timeout = v
                    }"
                  />
                  <Label for="rs-timeout" class="cursor-pointer font-normal">超时失败也重试</Label>
                </div>
                <div class="flex items-center gap-2">
                  <Checkbox
                    id="rs-rl-attempt"
                    :checked="globalForm.resubmit?.rate_limit_counts_as_attempt === true"
                    @update:checked="(v: boolean) => {
                      if (!globalForm.resubmit) globalForm.resubmit = {}
                      globalForm.resubmit.rate_limit_counts_as_attempt = v
                    }"
                  />
                  <Label for="rs-rl-attempt" class="cursor-pointer font-normal">因限速重试也算一次尝试</Label>
                </div>
              </div>
              <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                <div class="space-y-1">
                  <Label class="text-xs">最大尝试次数（含首次）</Label>
                  <Input
                    type="number"
                    placeholder="默认 3"
                    :model-value="numVal(globalForm.resubmit?.max_attempts)"
                    @update:model-value="(v) => { if (!globalForm.resubmit) globalForm.resubmit = {}; setNum(globalForm.resubmit as Record<string, unknown>, 'max_attempts', String(v)) }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">首次失败后延迟（秒）</Label>
                  <Input
                    type="number"
                    placeholder="默认 30"
                    :model-value="numVal(globalForm.resubmit?.initial_delay_seconds)"
                    @update:model-value="(v) => { if (!globalForm.resubmit) globalForm.resubmit = {}; setNum(globalForm.resubmit as Record<string, unknown>, 'initial_delay_seconds', String(v)) }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">每次重试等待时间乘多少（2 表示翻倍）</Label>
                  <Input
                    type="number"
                    step="0.1"
                    placeholder="默认 2，如 30 秒→60 秒→120 秒"
                    :model-value="numVal(globalForm.resubmit?.backoff_multiplier)"
                    @update:model-value="(v) => { if (!globalForm.resubmit) globalForm.resubmit = {}; setNum(globalForm.resubmit as Record<string, unknown>, 'backoff_multiplier', String(v)) }"
                  />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">单次延迟上限（秒）</Label>
                  <Input
                    type="number"
                    placeholder="默认 600"
                    :model-value="numVal(globalForm.resubmit?.max_delay_seconds)"
                    @update:model-value="(v) => { if (!globalForm.resubmit) globalForm.resubmit = {}; setNum(globalForm.resubmit as Record<string, unknown>, 'max_delay_seconds', String(v)) }"
                  />
                </div>
                <div class="space-y-1 sm:col-span-2 lg:col-span-3">
                  <Label class="text-xs">看到这些词就不再重试（逗号分隔）</Label>
                  <Input v-model="terminalKeywordsText" placeholder="如：余额不足, 课程不存在" />
                </div>
              </div>
              <div class="rounded-md border bg-muted/30 p-3 space-y-3">
                <div class="flex items-center gap-2">
                  <Checkbox
                    id="dlq-auto"
                    :checked="globalForm.resubmit?.dlq_auto_retry?.enabled === true"
                    @update:checked="(v: boolean) => {
                      if (!globalForm.resubmit) globalForm.resubmit = {}
                      if (!globalForm.resubmit.dlq_auto_retry) globalForm.resubmit.dlq_auto_retry = {}
                      globalForm.resubmit.dlq_auto_retry.enabled = v
                    }"
                  />
                  <Label for="dlq-auto" class="cursor-pointer font-normal text-sm">定时自动重试失败订单（默认关）</Label>
                </div>
                <div class="grid gap-3 sm:grid-cols-3">
                  <div class="space-y-1">
                    <Label class="text-xs">多久检查一次失败订单（分钟）</Label>
                    <Input
                      type="number"
                      placeholder="默认 30"
                      :model-value="numVal(globalForm.resubmit?.dlq_auto_retry?.scan_interval_minutes)"
                      @update:model-value="(v) => {
                        if (!globalForm.resubmit) globalForm.resubmit = {}
                        if (!globalForm.resubmit.dlq_auto_retry) globalForm.resubmit.dlq_auto_retry = {}
                        setNum(globalForm.resubmit.dlq_auto_retry as Record<string, unknown>, 'scan_interval_minutes', String(v))
                      }"
                    />
                  </div>
                  <div class="space-y-1">
                    <Label class="text-xs">每次最多重新排队几单</Label>
                    <Input
                      type="number"
                      placeholder="默认 50"
                      :model-value="numVal(globalForm.resubmit?.dlq_auto_retry?.max_per_scan)"
                      @update:model-value="(v) => {
                        if (!globalForm.resubmit) globalForm.resubmit = {}
                        if (!globalForm.resubmit.dlq_auto_retry) globalForm.resubmit.dlq_auto_retry = {}
                        setNum(globalForm.resubmit.dlq_auto_retry as Record<string, unknown>, 'max_per_scan', String(v))
                      }"
                    />
                  </div>
                  <div class="space-y-1">
                    <Label class="text-xs">失败订单躺多久才捞（分钟）</Label>
                    <Input
                      type="number"
                      placeholder="默认 60"
                      :model-value="numVal(globalForm.resubmit?.dlq_auto_retry?.min_age_minutes)"
                      @update:model-value="(v) => {
                        if (!globalForm.resubmit) globalForm.resubmit = {}
                        if (!globalForm.resubmit.dlq_auto_retry) globalForm.resubmit.dlq_auto_retry = {}
                        setNum(globalForm.resubmit.dlq_auto_retry as Record<string, unknown>, 'min_age_minutes', String(v))
                      }"
                    />
                  </div>
                </div>
              </div>
            </section>

            <Separator />

            <section v-if="opsForm" class="space-y-3">
              <h3 class="text-sm font-medium">AI 运维</h3>
              <p class="text-xs text-muted-foreground">保存后立即生效。要看详细记录、手动处理，请去「AI 运维」页。</p>
              <div class="flex flex-wrap items-start gap-4">
                <div class="flex items-center gap-2">
                  <Checkbox id="ops-enabled-global" v-model:checked="opsForm.enabled" />
                  <Label for="ops-enabled-global" class="cursor-pointer font-normal">启用 AI 运维</Label>
                </div>
                <div class="flex items-center gap-2">
                  <Checkbox id="ops-ai-global" v-model:checked="opsForm.ai_enabled" />
                  <Label for="ops-ai-global" class="cursor-pointer font-normal">启用 AI 分析</Label>
                </div>
                <div class="flex items-center gap-2">
                  <Checkbox id="ops-playbooks-global" v-model:checked="opsForm.playbooks_enabled" />
                  <Label for="ops-playbooks-global" class="cursor-pointer font-normal">启用自动规则（服务器报错、排队太多等）</Label>
                </div>
                <div class="flex items-center gap-2">
                  <Checkbox id="ops-daily-global" v-model:checked="opsForm.daily_report_enabled" />
                  <Label for="ops-daily-global" class="cursor-pointer font-normal">每日报告</Label>
                </div>
              </div>
              <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                <div class="space-y-1">
                  <Label class="text-xs">运行模式</Label>
                  <select
                    v-model="opsForm.mode"
                    class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm"
                  >
                    <option value="assist">只看建议</option>
                    <option value="auto">自动处理</option>
                  </select>
                  <p class="text-xs text-muted-foreground">{{ opsModeDescription(opsForm.mode) }}</p>
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">多久检查一次（秒）</Label>
                  <Input v-model.number="opsForm.scan_interval_seconds" type="number" />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">多久用 AI 分析一次（秒）</Label>
                  <Input v-model.number="opsForm.ai_analysis_interval_seconds" type="number" />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">失败率超过多少就告警（%）</Label>
                  <Input v-model.number="opsForm.thresholds.channel_fail_rate_pct" type="number" />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">排队超过多少单就告警</Label>
                  <Input v-model.number="opsForm.thresholds.queue_backlog" type="number" />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">失败订单超过多少就告警</Label>
                  <Input v-model.number="opsForm.thresholds.dlq_depth" type="number" />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">几点发每日报告（0–23 点）</Label>
                  <Input v-model.number="opsForm.daily_report_hour" type="number" min="0" max="23" />
                </div>
                <div class="space-y-1">
                  <Label class="text-xs">一次最多自动改几项</Label>
                  <Input v-model.number="opsForm.policy.max_actions_per_plan" type="number" />
                </div>
              </div>
            </section>

            <div class="flex flex-wrap gap-2 pt-2">
              <Button :disabled="loading" @click="saveGlobal">保存全局配置</Button>
              <Button variant="outline" :disabled="loading" @click="resetGlobalConfirmOpen = true">
                <RotateCcw class="mr-1 size-4" /> 恢复默认
              </Button>
            </div>
        </div>

        <!-- 货源列表：全宽表格 -->
        <div v-show="activeSection === 'list'" class="flex min-h-full flex-col p-4">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-2 text-sm text-muted-foreground">
            <span>
              共 {{ huoyuanList.length }} 个货源
              <template v-if="searchQuery.trim()">，匹配 {{ filteredList.length }} 个</template>
            </span>
          </div>

          <div v-if="loading && !sortedList.length" class="py-12 text-center text-muted-foreground">加载中...</div>
          <div v-else-if="!sortedList.length" class="py-12 text-center text-muted-foreground">暂无货源</div>
          <div v-else-if="!filteredList.length" class="py-12 text-center text-muted-foreground">没有匹配的货源</div>
          <div v-else class="min-h-0 flex-1 overflow-auto rounded-md border">
            <Table class="w-full table-fixed">
              <TableHeader>
                <TableRow class="hover:bg-transparent">
                  <TableHead class="w-[88px]">编号</TableHead>
                  <TableHead class="w-[18%] min-w-[120px]">名称</TableHead>
                  <TableHead class="w-[120px]">平台</TableHead>
                  <TableHead>地址</TableHead>
                  <TableHead class="w-[100px]">配置</TableHead>
                  <TableHead class="w-[96px] text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow v-for="row in filteredList" :key="row.hid">
                  <TableCell class="font-mono tabular-nums">{{ row.hid }}</TableCell>
                  <TableCell class="truncate font-medium" :title="row.name">{{ row.name }}</TableCell>
                  <TableCell class="truncate font-mono text-sm" :title="row.pt">{{ row.pt }}</TableCell>
                  <TableCell class="truncate text-sm text-muted-foreground" :title="row.url">{{ row.url }}</TableCell>
                  <TableCell>
                    <Badge :variant="row.has_config ? 'default' : 'secondary'" class="whitespace-nowrap">
                      {{ row.has_config ? '单独配置' : '继承' }}
                    </Badge>
                  </TableCell>
                  <TableCell class="text-right">
                    <Button variant="outline" size="sm" @click="openHIDEdit(row)">
                      <Settings2 class="mr-1 size-3.5" /> 配置
                    </Button>
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
        </div>
      </ScrollArea>
    </CardContent>

    <Dialog v-model:open="showHIDDialog">
      <DialogContent class="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>货源 {{ editingHID?.name }}（编号 {{ editingHID?.hid }}）</DialogTitle>
        </DialogHeader>
        <div v-if="editingHID" class="space-y-4">
          <div class="space-y-1">
            <Label class="text-xs">备注</Label>
            <Input v-model="hidRemark" placeholder="可选" />
          </div>
          <Separator />
            <div class="grid gap-3 sm:grid-cols-2">
              <div class="space-y-1">
                <Label class="text-xs">等平台回复最久等多少秒</Label>
                <Input
                  type="number"
                  placeholder="继承全局"
                  :model-value="numVal(hidForm.submit?.timeout_seconds)"
                  @update:model-value="(v) => { if (!hidForm.submit) hidForm.submit = {}; setNum(hidForm.submit as Record<string, unknown>, 'timeout_seconds', String(v)) }"
                />
              </div>
              <div class="space-y-1">
                <Label class="text-xs">最少同时处理几单</Label>
              <Input
                type="number"
                placeholder="不填则用全局"
                :model-value="numVal(hidForm.queue?.min_workers_per_hid)"
                @update:model-value="(v) => { if (!hidForm.queue) hidForm.queue = {}; setNum(hidForm.queue as Record<string, unknown>, 'min_workers_per_hid', String(v)) }"
              />
            </div>
            <div class="space-y-1">
              <Label class="text-xs">最多同时处理几单</Label>
              <Input
                type="number"
                placeholder="不填则用全局"
                :model-value="numVal(hidForm.queue?.max_workers_per_hid)"
                @update:model-value="(v) => { if (!hidForm.queue) hidForm.queue = {}; setNum(hidForm.queue as Record<string, unknown>, 'max_workers_per_hid', String(v)) }"
              />
            </div>
            <div class="space-y-1">
              <Label class="text-xs">处理太久算超时（分钟）</Label>
              <Input
                type="number"
                placeholder="不填则用全局"
                :model-value="numVal(hidForm.queue?.processing_timeout_minutes)"
                @update:model-value="(v) => { if (!hidForm.queue) hidForm.queue = {}; setNum(hidForm.queue as Record<string, unknown>, 'processing_timeout_minutes', String(v)) }"
              />
            </div>
            <div class="space-y-1">
              <Label class="text-xs">每秒最多提交几次</Label>
              <Input
                type="number"
                placeholder="0 表示不限制"
                :model-value="numVal(hidForm.rate_limit?.per_hid_max_per_second)"
                @update:model-value="(v) => { if (!hidForm.rate_limit) hidForm.rate_limit = {}; setNum(hidForm.rate_limit as Record<string, unknown>, 'per_hid_max_per_second', String(v)) }"
              />
            </div>
          </div>
          <div class="space-y-1">
            <Label class="text-xs">提交成功后订单状态</Label>
            <Input
              placeholder="不填则用全局"
              :model-value="hidForm.order_status?.submitted_status ?? ''"
              @update:model-value="(v) => {
                if (!hidForm.order_status) hidForm.order_status = {}
                hidForm.order_status.submitted_status = String(v)
              }"
            />
          </div>
          <div class="space-y-1">
            <Label class="text-xs">提交成功备注提示</Label>
            <Input
              placeholder="不填则用全局"
              :model-value="hidForm.order_status?.submitted_remarks ?? ''"
              @update:model-value="(v) => {
                if (!hidForm.order_status) hidForm.order_status = {}
                hidForm.order_status.submitted_remarks = String(v)
              }"
            />
          </div>
          <div class="space-y-1">
            <Label class="text-xs">平台返回这些数字算成功</Label>
            <Input v-model="hidSuccessCodesText" placeholder="如：0,1" />
          </div>
          <Separator />
          <div class="space-y-2">
            <Label class="text-xs font-medium">失败重试（不填则继承全局）</Label>
            <div class="flex flex-wrap gap-3">
              <div class="flex items-center gap-2">
                <Checkbox
                  :id="`hid-rs-${editingHID?.hid}`"
                  :checked="hidForm.resubmit?.enabled === true"
                  @update:checked="(v: boolean) => {
                    if (!hidForm.resubmit) hidForm.resubmit = {}
                    hidForm.resubmit.enabled = v
                  }"
                />
                <Label :for="`hid-rs-${editingHID?.hid}`" class="cursor-pointer font-normal text-xs">启用自动重试</Label>
              </div>
              <div class="flex items-center gap-2">
                <Checkbox
                  :checked="hidForm.resubmit?.retry_on_timeout === true"
                  @update:checked="(v: boolean) => {
                    if (!hidForm.resubmit) hidForm.resubmit = {}
                    hidForm.resubmit.retry_on_timeout = v
                  }"
                />
                <Label class="cursor-pointer font-normal text-xs">超时也重试</Label>
              </div>
            </div>
            <div class="grid gap-3 sm:grid-cols-2">
              <div class="space-y-1">
                <Label class="text-xs">最大尝试次数</Label>
                <Input
                  type="number"
                  placeholder="继承全局"
                  :model-value="numVal(hidForm.resubmit?.max_attempts)"
                  @update:model-value="(v) => { if (!hidForm.resubmit) hidForm.resubmit = {}; setNum(hidForm.resubmit as Record<string, unknown>, 'max_attempts', String(v)) }"
                />
              </div>
              <div class="space-y-1">
                <Label class="text-xs">首次延迟（秒）</Label>
                <Input
                  type="number"
                  placeholder="继承全局"
                  :model-value="numVal(hidForm.resubmit?.initial_delay_seconds)"
                  @update:model-value="(v) => { if (!hidForm.resubmit) hidForm.resubmit = {}; setNum(hidForm.resubmit as Record<string, unknown>, 'initial_delay_seconds', String(v)) }"
                />
              </div>
            </div>
            <div class="space-y-1">
              <Label class="text-xs">看到这些词就不再重试</Label>
              <Input v-model="hidTerminalKeywordsText" placeholder="继承全局" />
            </div>
          </div>
        </div>
        <DialogFooter class="gap-2 sm:justify-between">
          <Button v-if="editingHID?.has_config" variant="outline" type="button" :disabled="loading" @click="resetHIDConfirmOpen = true">
            删除单独配置
          </Button>
          <div class="flex gap-2">
            <Button variant="outline" type="button" @click="showHIDDialog = false">取消</Button>
            <Button type="button" :disabled="loading" @click="saveHID">保存</Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <Dialog v-model:open="resetGlobalConfirmOpen">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>恢复默认配置</DialogTitle>
          <DialogDescription>确定清除全局配置，恢复为默认？此操作不可撤销。</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" type="button" @click="resetGlobalConfirmOpen = false">取消</Button>
          <Button variant="destructive" type="button" :disabled="loading" @click="confirmResetGlobal">确认恢复</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <Dialog v-model:open="resetHIDConfirmOpen">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>删除单独配置</DialogTitle>
          <DialogDescription>
            确定删除货源 {{ editingHID?.hid }}（{{ editingHID?.name }}）的单独配置？删除后将继承全局配置。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" type="button" @click="resetHIDConfirmOpen = false">取消</Button>
          <Button variant="destructive" type="button" :disabled="loading" @click="confirmResetHID">确认删除</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </Card>
</template>
