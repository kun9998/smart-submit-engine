<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  Activity,
  Bot,
  FileText,
  Loader2,
  PlayCircle,
  RefreshCw,
  RotateCcw,
  Sparkles,
} from '@lucide/vue'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  opsApi,
  opsSeverityVariant,
  opsSeverityLabel,
  opsModeDescription,
  opsModeLabel,
  opsStatusLabel,
  opsSourceLabel,
  opsTriggerTypeLabel,
  opsActionLabel,
  formatOpsPlanSections,
  formatOpsActionResults,
  opsOperatorLabel,
  type OpsAnalyzeResult,
  type OpsAuditRow,
  type OpsConfig,
  type OpsDailyReport,
  type OpsStatus,
} from '@/api/ops'

const initialLoading = ref(true)
const auditRefreshing = ref(false)
const saving = ref(false)
const analyzing = ref(false)
const executing = ref(false)

const status = ref<OpsStatus | null>(null)
const config = ref<OpsConfig | null>(null)
const auditItems = ref<OpsAuditRow[]>([])
const auditTotal = ref(0)
const lastResult = ref<OpsAnalyzeResult | null>(null)
const dailyReport = ref<OpsDailyReport | null>(null)

const detailOpen = ref(false)
const detailRow = ref<OpsAuditRow | null>(null)
const detailLoading = ref(false)
const activeTab = ref<'overview' | 'audit' | 'config'>('overview')

async function loadStatus() {
  const res = await opsApi.status()
  if (res.code === 1 && res.data) status.value = res.data
}

async function loadConfig() {
  const res = await opsApi.config()
  if (res.code === 1 && res.data) config.value = res.data
}

async function loadAudit() {
  const res = await opsApi.auditList(1, 20)
  if (res.code === 1 && res.data) {
    auditItems.value = res.data.items || []
    auditTotal.value = res.data.total || 0
  }
}

async function loadDailyReport() {
  const res = await opsApi.dailyReport()
  if (res.code === 1) dailyReport.value = res.data ?? null
}

async function loadAll(initial = false) {
  if (initial) initialLoading.value = true
  else auditRefreshing.value = true
  try {
    await Promise.all([loadStatus(), loadConfig(), loadAudit(), loadDailyReport()])
  } finally {
    initialLoading.value = false
    auditRefreshing.value = false
  }
}

async function refreshAfterMutation() {
  await Promise.all([loadStatus(), loadAudit()])
}

async function refreshAuditTab() {
  auditRefreshing.value = true
  try {
    await Promise.all([loadAudit(), loadStatus()])
  } finally {
    auditRefreshing.value = false
  }
}

async function saveConfig() {
  if (!config.value) return
  saving.value = true
  try {
    const res = await opsApi.saveConfig(config.value)
    if (res.code !== 1) {
      toast.error(res.msg || '保存失败')
      return
    }
    config.value = res.data || config.value
    await Promise.all([loadStatus(), loadConfig()])
    toast.success('已保存，立即生效')
  } finally {
    saving.value = false
  }
}

async function runAnalyze(execute: boolean) {
  if (execute) executing.value = true
  else analyzing.value = true
  try {
    const res = await opsApi.analyze(execute)
    if (res.code !== 1 || !res.data) {
      toast.error(res.msg || '检查失败')
      return
    }
    lastResult.value = res.data
    if (res.data.warnings?.length) {
      toast.warning(res.data.warnings.join('；'), { duration: 6000 })
    }
    if (execute && res.data.executed) {
      toast.success('已自动处理')
    } else {
      toast.success('检查完成')
    }
    await refreshAfterMutation()
  } finally {
    analyzing.value = false
    executing.value = false
  }
}

async function openDetail(id: number) {
  detailLoading.value = true
  detailOpen.value = true
  try {
    const res = await opsApi.auditDetail(id)
    if (res.code === 1 && res.data) {
      detailRow.value = res.data
    } else {
      toast.error(res.msg || '加载失败')
      detailOpen.value = false
    }
  } finally {
    detailLoading.value = false
  }
}

async function rollback(id: number) {
  const res = await opsApi.rollback(id)
  if (res.code !== 1) {
    toast.error(res.msg || '撤销失败')
    return
  }
  toast.success('已撤销')
  detailOpen.value = false
  await refreshAfterMutation()
}

async function resumeChannel(hid: number) {
  const res = await opsApi.resumeChannel(hid)
  if (res.code !== 1) {
    toast.error(res.msg || '恢复失败')
    return
  }
  toast.success(`渠道 ${hid} 已恢复处理`)
  await loadStatus()
}

function watcherStatusLabel() {
  if (!status.value?.enabled) return '未开启'
  return status.value.watcher_running ? '运行中' : '已停止'
}

function formatTime(iso: string) {
  try {
    return new Date(iso).toLocaleString('zh-CN')
  } catch {
    return iso
  }
}

onMounted(() => loadAll(true))
</script>

<template>
  <div class="space-y-6">
    <div class="flex flex-wrap gap-2">
      <Button
        :variant="activeTab === 'overview' ? 'default' : 'outline'"
        size="sm"
        @click="activeTab = 'overview'"
      >
        概览
      </Button>
      <Button :variant="activeTab === 'audit' ? 'default' : 'outline'" size="sm" @click="activeTab = 'audit'">
        操作记录
      </Button>
      <Button :variant="activeTab === 'config' ? 'default' : 'outline'" size="sm" @click="activeTab = 'config'">
        设置
      </Button>
    </div>

    <div v-show="activeTab === 'overview'" class="space-y-6">
        <div v-if="initialLoading" class="grid gap-4 md:grid-cols-3">
          <Skeleton class="h-28" />
          <Skeleton class="h-28" />
          <Skeleton class="h-28" />
        </div>

        <template v-else>
          <div class="grid gap-4 md:grid-cols-3">
            <Card>
              <CardHeader class="pb-2">
                <CardTitle class="text-sm font-medium text-muted-foreground">运行状态</CardTitle>
              </CardHeader>
              <CardContent>
                <div class="flex items-center gap-2">
                  <Badge :variant="status?.enabled ? 'default' : 'outline'">
                    {{ status?.enabled ? '已开启' : '未开启' }}
                  </Badge>
                  <Badge variant="secondary">{{ opsModeLabel(status?.mode || 'assist') }}</Badge>
                </div>
              </CardContent>
            </Card>
            <Card>
              <CardHeader class="pb-2">
                <CardTitle class="text-sm font-medium text-muted-foreground">AI 与自动检查</CardTitle>
              </CardHeader>
              <CardContent class="space-y-1 text-sm">
                <div class="flex items-center gap-2">
                  <Sparkles class="size-4 text-muted-foreground" />
                  AI {{ status?.ai_ready ? '可用' : '未配置' }}
                </div>
                <div class="flex items-center gap-2">
                  <Activity class="size-4 text-muted-foreground" />
                  自动检查 {{ watcherStatusLabel() }}
                </div>
              </CardContent>
            </Card>
            <Card>
              <CardHeader class="pb-2">
                <CardTitle class="text-sm font-medium text-muted-foreground">已停用的渠道</CardTitle>
              </CardHeader>
              <CardContent>
                <div class="text-2xl font-semibold">{{ status?.paused_channels?.length ?? 0 }}</div>
              </CardContent>
            </Card>
          </div>

          <Card v-if="dailyReport">
            <CardHeader class="pb-2">
              <CardTitle class="flex items-center gap-2 text-base">
                <FileText class="size-4" />
                每日运行情况
              </CardTitle>
              <CardDescription>
                {{ dailyReport.date }} · {{ formatTime(dailyReport.generated_at) }}
                <span v-if="dailyReport.pushed" class="ml-1">· 已推送到消息通知</span>
              </CardDescription>
            </CardHeader>
            <CardContent class="space-y-3">
              <p class="text-sm">{{ dailyReport.summary }}</p>
              <pre class="max-h-64 overflow-auto whitespace-pre-wrap rounded-lg bg-muted/40 p-3 text-xs leading-relaxed">{{ dailyReport.body }}</pre>
            </CardContent>
          </Card>

          <Card>
            <CardHeader class="flex flex-row items-start justify-between gap-4 space-y-0">
              <div>
                <CardTitle class="flex items-center gap-2">
                  <Bot class="size-5" />
                  立即检查
                </CardTitle>
                <CardDescription>看看最近订单处理和报错情况；「只看建议」模式下不会自动改设置</CardDescription>
              </div>
              <div class="flex flex-wrap gap-2">
                <Button variant="outline" size="sm" :disabled="analyzing || executing" @click="runAnalyze(false)">
                  <Loader2 v-if="analyzing" class="mr-2 size-4 animate-spin" />
                  <RefreshCw v-else class="mr-2 size-4" />
                  仅检查
                </Button>
                <Button
                  size="sm"
                  :disabled="analyzing || executing || !config?.enabled"
                  @click="runAnalyze(true)"
                >
                  <Loader2 v-if="executing" class="mr-2 size-4 animate-spin" />
                  检查并处理
                </Button>
              </div>
            </CardHeader>
            <CardContent v-if="lastResult" class="space-y-4">
              <Alert>
                <AlertDescription>
                  <strong>{{ lastResult.plan.summary }}</strong>
                  <span v-if="lastResult.plan.matched_playbook" class="ml-2 text-muted-foreground">
                    · 规则：{{ lastResult.plan.matched_playbook }}
                  </span>
                </AlertDescription>
              </Alert>
              <div v-if="lastResult.plan.manual_suggestions?.length" class="text-sm text-muted-foreground">
                <div class="mb-1 font-medium">建议</div>
                <ul class="list-inside list-disc space-y-1">
                  <li v-for="(s, i) in lastResult.plan.manual_suggestions" :key="i">{{ s }}</li>
                </ul>
              </div>
              <div v-if="lastResult.actions_result?.length" class="rounded-lg border p-3 text-sm">
                <div class="mb-2 font-medium">处理结果</div>
                <div v-for="(r, i) in lastResult.actions_result" :key="i" class="flex justify-between gap-2">
                  <span>{{ opsActionLabel(r.action) }}</span>
                  <span :class="r.ok ? 'text-emerald-600' : 'text-destructive'">{{ r.message }}</span>
                </div>
              </div>
            </CardContent>
          </Card>

          <Card v-if="status?.paused_channels?.length">
            <CardHeader>
              <CardTitle class="text-base">已停用的渠道</CardTitle>
            </CardHeader>
            <CardContent>
              <div class="space-y-2">
                <div
                  v-for="ch in status.paused_channels"
                  :key="ch.hid"
                  class="flex items-center justify-between rounded-lg border px-3 py-2 text-sm"
                >
                  <div>
                    <span class="font-medium">{{ ch.name }}</span>
                    <span class="ml-2 text-muted-foreground">渠道号 {{ ch.hid }}</span>
                  </div>
                  <Button variant="outline" size="sm" @click="resumeChannel(ch.hid)">
                    <PlayCircle class="mr-1 size-4" />
                    恢复处理
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle class="text-base">最近记录</CardTitle>
              <CardDescription>共 {{ auditTotal }} 条</CardDescription>
            </CardHeader>
            <CardContent>
              <div v-if="!auditItems.length" class="text-sm text-muted-foreground">暂无记录</div>
              <div v-else class="space-y-3">
                <div
                  v-for="row in auditItems.slice(0, 5)"
                  :key="row.id"
                  class="flex flex-wrap items-center justify-between gap-2 rounded-lg border px-3 py-2 text-sm"
                >
                  <div class="min-w-0 flex-1">
                    <div class="truncate font-medium">{{ row.summary }}</div>
                    <div class="text-xs text-muted-foreground">{{ formatTime(row.created_at) }} · {{ opsSourceLabel(row.source) }}</div>
                  </div>
                  <div class="flex items-center gap-2">
                    <Badge :variant="opsSeverityVariant(row.severity)">{{ opsSeverityLabel(row.severity) }}</Badge>
                    <Badge variant="outline">{{ opsStatusLabel(row.status) }}</Badge>
                    <Button variant="ghost" size="sm" @click="openDetail(row.id)">详情</Button>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </template>
    </div>

    <div v-show="activeTab === 'audit'">
        <Card>
          <CardHeader class="flex flex-row items-center justify-between">
            <CardTitle class="text-base">操作记录</CardTitle>
            <Button variant="outline" size="sm" :disabled="auditRefreshing" @click="refreshAuditTab">
              <Loader2 v-if="auditRefreshing" class="mr-1 size-4 animate-spin" />
              <RefreshCw v-else class="mr-1 size-4" />
              刷新
            </Button>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>时间</TableHead>
                  <TableHead>摘要</TableHead>
                  <TableHead>来源</TableHead>
                  <TableHead>级别</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead class="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow v-for="row in auditItems" :key="row.id">
                  <TableCell class="whitespace-nowrap text-xs">{{ formatTime(row.created_at) }}</TableCell>
                  <TableCell class="max-w-xs truncate">{{ row.summary }}</TableCell>
                  <TableCell>{{ opsSourceLabel(row.source) }}</TableCell>
                  <TableCell>
                    <Badge :variant="opsSeverityVariant(row.severity)">{{ opsSeverityLabel(row.severity) }}</Badge>
                  </TableCell>
                  <TableCell>{{ opsStatusLabel(row.status) }}</TableCell>
                  <TableCell class="text-right">
                    <Button variant="ghost" size="sm" @click="openDetail(row.id)">详情</Button>
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </CardContent>
        </Card>
    </div>

    <div v-show="activeTab === 'config'">
        <Card v-if="config">
          <CardHeader>
            <CardTitle class="text-base">AI 运维设置</CardTitle>
            <CardDescription>
              常用项可在「货源配置 → 全局配置」里一起保存。这里可改全部参数；默认关闭，「只看建议」不会自动改设置。
            </CardDescription>
          </CardHeader>
          <CardContent class="space-y-6 max-w-2xl">
            <div class="flex items-start gap-3">
              <Checkbox id="ops-enabled" v-model:checked="config.enabled" />
              <div>
                <Label for="ops-enabled">启用 AI 运维</Label>
                <p class="text-xs text-muted-foreground">开启后后台会自动检查，也可在本页手动检查</p>
              </div>
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <div class="space-y-2">
                <Label>运行模式</Label>
                <select
                  v-model="config.mode"
                  class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm"
                >
                  <option value="assist">只看建议</option>
                  <option value="auto">自动处理</option>
                </select>
                <p v-if="config" class="text-xs text-muted-foreground">
                  {{ opsModeDescription(config.mode) }}
                </p>
              </div>
              <div class="flex items-start gap-3 pt-6">
                <Checkbox id="ops-ai" v-model:checked="config.ai_enabled" />
                <Label for="ops-ai">启用 AI 分析（需先在系统设置里配好接口密钥）</Label>
              </div>
            </div>
            <Separator />
            <div class="flex items-start gap-3">
              <Checkbox id="ops-playbooks" v-model:checked="config.playbooks_enabled" />
              <Label for="ops-playbooks">启用自动规则（服务器报错、排队太多、失败订单过多等）</Label>
            </div>
            <Separator />
            <div class="space-y-3">
              <div class="text-sm font-medium">什么时候告警</div>
              <div class="grid gap-4 sm:grid-cols-2">
                <div class="space-y-2">
                  <Label>失败率超过多少就告警（%）</Label>
                  <Input v-model.number="config.thresholds.channel_fail_rate_pct" type="number" />
                  <p class="text-xs text-muted-foreground">最近一段时间里，某渠道失败比例超过此值就提醒</p>
                </div>
                <div class="space-y-2">
                  <Label>失败率突然涨多少就告警（百分点）</Label>
                  <Input v-model.number="config.thresholds.channel_fail_rate_spike_pp" type="number" />
                  <p class="text-xs text-muted-foreground">短时间内失败率上升超过此幅度就提醒</p>
                </div>
                <div class="space-y-2">
                  <Label>失败订单超过多少就告警</Label>
                  <Input v-model.number="config.thresholds.dlq_depth" type="number" />
                  <p class="text-xs text-muted-foreground">堆积的失败订单太多时提醒</p>
                </div>
                <div class="space-y-2">
                  <Label>排队超过多少单就告警</Label>
                  <Input v-model.number="config.thresholds.queue_backlog" type="number" />
                  <p class="text-xs text-muted-foreground">还在排队、没开始处理的订单太多时提醒</p>
                </div>
              </div>
            </div>
            <Separator />
            <div class="space-y-3">
              <div class="text-sm font-medium">什么时候自动恢复渠道</div>
              <div class="grid gap-4 sm:grid-cols-2">
                <div class="space-y-2">
                  <Label>失败率低于多少才恢复（%）</Label>
                  <Input v-model.number="config.thresholds.resume_fail_rate_pct" type="number" />
                  <p class="text-xs text-muted-foreground">被停用的渠道，失败率降到此值以下才可能恢复</p>
                </div>
                <div class="space-y-2">
                  <Label>要稳定多久（分钟）</Label>
                  <Input v-model.number="config.thresholds.resume_stable_minutes" type="number" />
                  <p class="text-xs text-muted-foreground">连续保持低失败率，持续这么久才会恢复</p>
                </div>
                <div class="space-y-2">
                  <Label>至少要有多少笔订单才判断</Label>
                  <Input v-model.number="config.thresholds.resume_min_window_events" type="number" />
                  <p class="text-xs text-muted-foreground">样本太少时不做恢复判断，避免误判</p>
                </div>
                <div class="space-y-2">
                  <Label>多久检查一次（秒）</Label>
                  <Input v-model.number="config.scan_interval_seconds" type="number" />
                  <p class="text-xs text-muted-foreground">后台自动检查订单和报错的间隔</p>
                </div>
              </div>
            </div>
            <Separator />
            <div class="space-y-3">
              <div class="text-sm font-medium">每日运行情况报告</div>
              <div class="flex items-start gap-3">
                <Checkbox id="ops-daily-report" v-model:checked="config.daily_report_enabled" />
                <div>
                  <Label for="ops-daily-report">启用每日报告</Label>
                  <p class="text-xs text-muted-foreground">每天汇总订单处理和运维记录，可推送到绑定的消息通知</p>
                </div>
              </div>
              <div class="grid gap-4 sm:grid-cols-2 max-w-md">
                <div class="space-y-2">
                  <Label>几点生成（0–23 点）</Label>
                  <Input v-model.number="config.daily_report_hour" type="number" min="0" max="23" />
                  <p class="text-xs text-muted-foreground">默认早上 8 点；需开启通知且在个人中心绑定推送地址</p>
                </div>
              </div>
            </div>
            <Button :disabled="saving" @click="saveConfig">
              <Loader2 v-if="saving" class="mr-2 size-4 animate-spin" />
              保存设置
            </Button>
          </CardContent>
        </Card>
    </div>

    <Dialog v-model:open="detailOpen">
      <DialogContent class="max-h-[85vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>记录详情</DialogTitle>
          <DialogDescription v-if="detailRow">{{ detailRow.summary }}</DialogDescription>
        </DialogHeader>
        <div v-if="detailLoading" class="py-8 text-center text-sm text-muted-foreground">加载中…</div>
        <template v-else-if="detailRow">
          <div class="space-y-2 text-sm">
            <div class="flex flex-wrap gap-2">
              <Badge :variant="opsSeverityVariant(detailRow.severity)">{{ opsSeverityLabel(detailRow.severity) }}</Badge>
              <Badge variant="outline">{{ opsStatusLabel(detailRow.status) }}</Badge>
              <Badge variant="secondary">{{ opsSourceLabel(detailRow.source) }}</Badge>
            </div>
            <div class="text-muted-foreground">
              {{ formatTime(detailRow.created_at) }} · {{ opsTriggerTypeLabel(detailRow.trigger_type) }}
              <span v-if="detailRow.operator"> · {{ opsOperatorLabel(detailRow.operator) }}</span>
            </div>
          </div>
          <Separator />
          <div v-if="detailRow.plan_json" class="space-y-3">
            <div
              v-for="(section, si) in formatOpsPlanSections(detailRow.plan_json)"
              :key="si"
              class="space-y-1"
            >
              <div class="text-sm font-medium">{{ section.title }}</div>
              <ul class="space-y-1 text-sm text-muted-foreground">
                <li v-for="(line, li) in section.lines" :key="li" class="whitespace-pre-wrap">{{ line }}</li>
              </ul>
            </div>
          </div>
          <div v-if="detailRow.executed_actions" class="space-y-2">
            <div class="text-sm font-medium">实际做了什么</div>
            <div class="space-y-2 rounded-lg border p-3 text-sm">
              <div
                v-for="(r, i) in formatOpsActionResults(detailRow.executed_actions)"
                :key="i"
                class="flex flex-wrap justify-between gap-2"
              >
                <span>{{ r.action }}</span>
                <span :class="r.ok ? 'text-emerald-600' : 'text-destructive'">{{ r.message || (r.ok ? '成功' : '失败') }}</span>
              </div>
            </div>
          </div>
        </template>
        <DialogFooter v-if="detailRow?.snapshot_json">
          <Button variant="outline" @click="detailOpen = false">关闭</Button>
          <Button variant="destructive" @click="rollback(detailRow!.id)">
            <RotateCcw class="mr-1 size-4" />
            撤销操作
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
