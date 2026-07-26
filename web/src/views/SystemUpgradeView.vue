<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ArrowUpCircle, CheckCircle2, Download, Loader2, RefreshCw, Rocket } from '@lucide/vue'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import {
  formatFileSize,
  phaseLabel,
  systemApi,
  type ReleaseInfo,
  type SystemInfo,
  type UpgradeStatus,
} from '@/api/system'

const info = ref<SystemInfo | null>(null)
const release = ref<ReleaseInfo | null>(null)
const status = ref<UpgradeStatus | null>(null)
const loadingInfo = ref(true)
const checking = ref(false)
const applying = ref(false)
const confirmOpen = ref(false)

let pollTimer: ReturnType<typeof setInterval> | null = null
let pollFailCount = 0

const isUpgrading = computed(() => {
  const phase = status.value?.phase
  return !!phase && !['idle', 'failed', 'completed'].includes(phase)
})

const hasUpdate = computed(() => release.value?.has_update === true)

async function loadInfo() {
  loadingInfo.value = true
  try {
    const res = await systemApi.info()
    if (res.code === 1 && res.data) {
      info.value = res.data
    }
  } finally {
    loadingInfo.value = false
  }
}

async function loadStatus() {
  try {
    const res = await systemApi.upgradeStatus()
    if (res.code === 1 && res.data) {
      pollFailCount = 0
      status.value = res.data
      if (res.data.release) {
        release.value = res.data.release
      }
      return true
    }
  } catch {
    // 升级重启期间服务不可用，忽略瞬时网络错误
  }
  pollFailCount++
  return false
}

async function checkUpdate() {
  checking.value = true
  try {
    const res = await systemApi.checkUpgrade()
    if (res.code !== 1 || !res.data) {
      const needAuth = (res as { need_auth?: boolean }).need_auth
      toast.error(needAuth ? `${res.msg || '检查更新失败'}，请先确保在线授权有效` : (res.msg || '检查更新失败'))
      return
    }
    release.value = res.data.release
    if (res.data.release.has_update) {
      toast.success(`发现新版本 ${res.data.release.version}`)
    } else {
      toast.success('当前已是最新版本')
    }
  } finally {
    checking.value = false
  }
}

function startPolling() {
  stopPolling()
  pollFailCount = 0
  pollTimer = setInterval(async () => {
    const ok = await loadStatus()
    const phase = status.value?.phase
    if (phase === 'failed') {
      toast.error(status.value?.error || '升级失败')
      stopPolling()
      return
    }
    if (phase === 'restarting') {
      if (pollFailCount === 0) {
        toast.message('服务正在重启，请稍候刷新页面...')
      }
      return
    }
    if (!ok && pollFailCount >= 120) {
      toast.error('长时间无法连接服务，请手动刷新页面查看是否升级完成')
      stopPolling()
    }
  }, 2000)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

async function applyUpgrade() {
  confirmOpen.value = false
  applying.value = true
  try {
    const res = await systemApi.applyUpgrade(release.value?.version)
    if (res.code !== 1) {
      const needAuth = (res as { need_auth?: boolean }).need_auth
      toast.error(needAuth ? `${res.msg || '启动升级失败'}，请先确保在线授权有效` : (res.msg || '启动升级失败'))
      return
    }
    status.value = res.data || null
    toast.success('升级已开始，服务即将自动重启')
    startPolling()
  } finally {
    applying.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadInfo(), loadStatus()])
  if (isUpgrading.value) {
    startPolling()
  }
})

onUnmounted(stopPolling)
</script>

<template>
  <div class="space-y-6">
    <Card>
      <CardHeader>
        <CardTitle class="flex items-center gap-2">
          <Rocket class="size-5" />
          当前版本
        </CardTitle>
        <CardDescription>程序内置版本与运行环境信息</CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div v-if="loadingInfo" class="space-y-2">
          <Skeleton class="h-5 w-48" />
          <Skeleton class="h-4 w-64" />
        </div>
        <template v-else-if="info">
          <div class="flex flex-wrap items-center gap-3">
            <Badge variant="secondary" class="text-base px-3 py-1">{{ info.product_version }}</Badge>
            <span class="text-sm text-muted-foreground">{{ info.product_name }}</span>
          </div>
          <div class="grid gap-2 text-sm text-muted-foreground sm:grid-cols-2">
            <div>运行平台：{{ info.platform }}</div>
          </div>
        </template>
      </CardContent>
    </Card>

    <Card>
      <CardHeader class="flex flex-row items-start justify-between gap-4 space-y-0">
        <div>
          <CardTitle class="flex items-center gap-2">
            <ArrowUpCircle class="size-5" />
            在线更新
          </CardTitle>
          <CardDescription>从授权站拉取最新发布包</CardDescription>
        </div>
        <Button variant="outline" size="sm" :disabled="checking || isUpgrading" @click="checkUpdate">
          <Loader2 v-if="checking" class="mr-2 size-4 animate-spin" />
          <RefreshCw v-else class="mr-2 size-4" />
          检查更新
        </Button>
      </CardHeader>
      <CardContent class="space-y-4">
        <Alert v-if="release?.has_update">
          <Download class="size-4" />
          <AlertDescription>
            发现新版本 <strong>{{ release.version }}</strong>
            <span v-if="release.size">（{{ formatFileSize(release.size) }}）</span>
            <span v-if="release.force" class="ml-2 text-amber-600">· 建议尽快升级</span>
          </AlertDescription>
        </Alert>

        <Alert v-else-if="release && !release.has_update">
          <CheckCircle2 class="size-4" />
          <AlertDescription>当前已是最新版本，无需升级</AlertDescription>
        </Alert>

        <div v-if="release?.changelog" class="rounded-lg border bg-muted/30 p-4">
          <div class="mb-2 text-sm font-medium">更新说明</div>
          <pre class="whitespace-pre-wrap text-sm text-muted-foreground font-sans">{{ release.changelog }}</pre>
        </div>

        <div v-if="isUpgrading || (status && status.phase !== 'idle')" class="space-y-3 rounded-lg border p-4">
          <div class="flex items-center justify-between text-sm">
            <span>{{ phaseLabel(status?.phase || 'idle') }}</span>
            <span class="text-muted-foreground">{{ status?.progress ?? 0 }}%</span>
          </div>
          <div class="h-2 overflow-hidden rounded-full bg-muted">
            <div
              class="h-full bg-primary transition-all duration-300"
              :style="{ width: `${status?.progress ?? 0}%` }"
            />
          </div>
          <p class="text-sm text-muted-foreground">{{ status?.message }}</p>
          <p v-if="status?.error" class="text-sm text-destructive">{{ status.error }}</p>
        </div>

        <Separator />

        <div class="flex justify-end">
          <Button
            :disabled="!hasUpdate || applying || isUpgrading"
            @click="confirmOpen = true"
          >
            <Loader2 v-if="applying || isUpgrading" class="mr-2 size-4 animate-spin" />
            <Download v-else class="mr-2 size-4" />
            一键升级
          </Button>
        </div>
      </CardContent>
    </Card>

    <Dialog v-model:open="confirmOpen">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>确认升级</DialogTitle>
          <DialogDescription>
            将把系统从 {{ info?.product_version }} 升级到 {{ release?.version }}。升级过程中服务会短暂中断并自动重启，请确认当前没有关键任务正在处理。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" @click="confirmOpen = false">取消</Button>
          <Button @click="applyUpgrade">确认升级</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
