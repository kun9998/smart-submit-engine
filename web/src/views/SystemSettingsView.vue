<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Database, Copy, Globe, Loader2, RefreshCw, Save, Shield, Sparkles } from '@lucide/vue'
import { toast } from 'vue-sonner'
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
import { Textarea } from '@/components/ui/textarea'
import { copyToClipboard } from '@/lib/clipboard'
import { systemApi, type SystemSettings } from '@/api/system'

type Section = 'redis' | 'http' | 'ai'

const loading = ref(true)
const saving = ref(false)
const testingRedis = ref(false)
const activeSection = ref<Section>('redis')

const redisAddr = ref('')
const redisDB = ref(0)
const redisAddrConfigured = ref(false)
const redisAddrTouched = ref(false)
const redisPass = ref('')
const redisPassSet = ref(false)
const redisPassTouched = ref(false)

const hostWhitelistText = ref('')
const blockPrivateNetworks = ref(true)
const allowInsecureHTTPToLAN = ref(false)
/** 系统已安装后禁止通过管理端开启 HTTP 内网绕过 */
const httpLANLocked = ref(true)

const aiEnabled = ref(false)
const aiBaseURL = ref('https://api.openai.com/v1')
const aiModel = ref('gpt-4o-mini')
const aiAPIKey = ref('')
const aiAPIKeySet = ref(false)
const aiAPIKeyTouched = ref(false)
const aiSettingsKey = ref(0)

const enqueueUrl = ref('')
const enqueueToken = ref('')
const enqueueReady = ref(false)
const savingEnqueue = ref(false)
const regeneratingEnqueue = ref(false)
const regenerateConfirmOpen = ref(false)

const navItems: { id: Section; label: string; icon: typeof Database }[] = [
  { id: 'redis', label: '订单队列', icon: Database },
  { id: 'http', label: '访问安全', icon: Shield },
  { id: 'ai', label: '智能助手', icon: Sparkles },
]

function applySettings(data: SystemSettings) {
  redisAddr.value = ''
  redisAddrConfigured.value = data.redis.addr_configured
  redisAddrTouched.value = false
  redisPass.value = ''
  redisDB.value = data.redis.db ?? 0
  redisPassSet.value = data.redis.pass_set
  redisPassTouched.value = false

  hostWhitelistText.value = (data.http_security.host_whitelist || []).join('\n')
  blockPrivateNetworks.value = data.http_security.block_private_networks
  allowInsecureHTTPToLAN.value = data.http_security.allow_insecure_http_to_lan

  aiEnabled.value = data.ai?.enabled === true
  aiBaseURL.value = data.ai?.base_url || 'https://api.openai.com/v1'
  aiModel.value = data.ai?.model || 'gpt-4o-mini'
  aiAPIKey.value = ''
  aiAPIKeySet.value = data.ai?.api_key_set === true
  aiAPIKeyTouched.value = false
  aiSettingsKey.value += 1

  enqueueUrl.value = data.internal_enqueue?.url || ''
  enqueueToken.value = data.internal_enqueue?.token || ''
  enqueueReady.value = data.internal_enqueue?.ready === true
}

async function loadSettings() {
  loading.value = true
  try {
    const res = await systemApi.settings()
    if (res.code === 1 && res.data) {
      applySettings(res.data)
    } else {
      toast.error(res.msg || '加载失败')
    }
  } finally {
    loading.value = false
  }
}

async function testRedis() {
  testingRedis.value = true
  try {
    const payload: { addr?: string; pass?: string; db: number } = {
      db: Number(redisDB.value) || 0,
    }
    if (redisAddrTouched.value && redisAddr.value.trim()) {
      payload.addr = redisAddr.value.trim()
    }
    if (redisPassTouched.value && redisPass.value) {
      payload.pass = redisPass.value
    }
    const res = await systemApi.testRedis(payload)
    if (res.code === 1) toast.success(res.msg || '连接成功')
    else toast.error(res.msg || '连接失败')
  } finally {
    testingRedis.value = false
  }
}

async function saveRedis() {
  saving.value = true
  try {
    const payload: Parameters<typeof systemApi.saveSettings>[0] = {
      redis: {
        db: Number(redisDB.value) || 0,
        pass_set: redisPassSet.value || redisPassTouched.value,
      },
    }
    if (redisAddrTouched.value && redisAddr.value.trim()) {
      payload.redis!.addr = redisAddr.value.trim()
    }
    if (redisPassTouched.value) payload.redis!.pass = redisPass.value
    const res = await systemApi.saveSettings(payload)
    if (res.code === 1 && res.data) {
      applySettings(res.data)
      toast.success(res.msg || '保存成功')
    } else {
      toast.error(res.msg || '保存失败')
    }
  } finally {
    saving.value = false
  }
}

async function saveHTTPSecurity() {
  saving.value = true
  try {
    const hosts = hostWhitelistText.value
      .split(/\r?\n/)
      .map((s) => s.trim())
      .filter(Boolean)
    const res = await systemApi.saveSettings({
      http_security: {
        host_whitelist: hosts,
        block_private_networks: blockPrivateNetworks.value,
        allow_insecure_http_to_lan: httpLANLocked.value ? false : allowInsecureHTTPToLAN.value,
      },
    })
    if (res.code === 1 && res.data) {
      applySettings(res.data)
      toast.success(res.msg || '保存成功')
    } else {
      toast.error(res.msg || '保存失败')
    }
  } finally {
    saving.value = false
  }
}


async function saveAI() {
  if (aiEnabled.value && !aiAPIKeySet.value && !aiAPIKeyTouched.value) {
    toast.error('启用智能助手前请先填写接口密钥')
    return
  }
  if (aiEnabled.value && aiAPIKeyTouched.value && !aiAPIKey.value.trim()) {
    toast.error('请填写有效的接口密钥')
    return
  }
  saving.value = true
  try {
    const payload: Parameters<typeof systemApi.saveSettings>[0] = {
      ai: {
        enabled: aiEnabled.value,
        base_url: aiBaseURL.value.trim(),
        model: aiModel.value.trim(),
        api_key_set: aiAPIKeySet.value || aiAPIKeyTouched.value,
      },
    }
    if (aiAPIKeyTouched.value && aiAPIKey.value.trim()) {
      payload.ai!.api_key = aiAPIKey.value.trim()
    }
    const res = await systemApi.saveSettings(payload)
    if (res.code === 1 && res.data) {
      applySettings(res.data)
      toast.success(res.msg || '保存成功')
    } else {
      toast.error(res.msg || '保存失败')
    }
  } finally {
    saving.value = false
  }
}

async function copyEnqueueToken() {
  if (!enqueueToken.value) {
    toast.error('暂无密钥')
    return
  }
  const ok = await copyToClipboard(enqueueToken.value)
  if (ok) toast.success('密钥已复制')
  else toast.error('复制失败，请手动选择复制')
}

async function copyEnqueueUrl() {
  if (!enqueueUrl.value) {
    toast.error('暂无接口地址')
    return
  }
  const ok = await copyToClipboard(enqueueUrl.value)
  if (ok) toast.success('接口地址已复制')
  else toast.error('复制失败，请手动选择复制')
}

async function saveEnqueueToken() {
  const token = enqueueToken.value.trim()
  if (token.length < 4) {
    toast.error('密钥至少 4 个字符')
    return
  }
  savingEnqueue.value = true
  try {
    const res = await systemApi.saveInternalEnqueueSecret(token)
    if (res.code === 1 && res.data) {
      enqueueUrl.value = res.data.url
      enqueueToken.value = res.data.token
      enqueueReady.value = res.data.ready
      toast.success(res.msg || '密钥已保存')
    } else {
      toast.error(res.msg || '保存失败')
    }
  } finally {
    savingEnqueue.value = false
  }
}

async function confirmRegenerateEnqueueToken() {
  regenerateConfirmOpen.value = false
  regeneratingEnqueue.value = true
  try {
    const res = await systemApi.regenerateInternalEnqueueSecret()
    if (res.code === 1 && res.data) {
      enqueueUrl.value = res.data.url
      enqueueToken.value = res.data.token
      enqueueReady.value = res.data.ready
      toast.success(res.msg || '已生成新密钥')
    } else {
      toast.error(res.msg || '生成失败')
    }
  } finally {
    regeneratingEnqueue.value = false
  }
}

onMounted(loadSettings)
</script>

<template>
  <div class="flex flex-col gap-6 lg:flex-row">
      <Card class="lg:w-52 shrink-0">
        <CardContent class="p-2">
          <nav class="flex flex-row flex-wrap gap-1 lg:flex-col">
            <Button
              v-for="item in navItems"
              :key="item.id"
              variant="ghost"
              class="justify-start"
              :class="activeSection === item.id ? 'bg-muted' : ''"
              @click="activeSection = item.id"
            >
              <component :is="item.icon" class="mr-2 size-4" />
              {{ item.label }}
            </Button>
          </nav>
        </CardContent>
      </Card>

      <div class="min-w-0 flex-1 space-y-4">
        <Card v-if="activeSection === 'redis'">
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <Database class="size-4" />
              订单队列连接
            </CardTitle>
            <CardDescription>订单排队和处理依赖队列服务，保存前会自动测试能否连上</CardDescription>
          </CardHeader>
          <CardContent v-if="loading" class="text-sm text-muted-foreground">加载中…</CardContent>
          <CardContent v-else class="space-y-4 max-w-lg">
            <div class="space-y-2">
              <Label for="redis-addr">连接地址</Label>
              <div v-if="redisAddrConfigured && !redisAddrTouched" class="flex items-center gap-2">
                <Badge variant="secondary">已配置</Badge>
                <Button variant="link" class="h-auto p-0 text-xs" @click="redisAddrTouched = true">修改</Button>
              </div>
              <Input
                v-else
                id="redis-addr"
                v-model="redisAddr"
                placeholder="如 127.0.0.1:6379"
                @input="redisAddrTouched = true"
              />
            </div>
            <div class="space-y-2">
              <Label for="redis-pass">密码</Label>
              <Input
                id="redis-pass"
                v-model="redisPass"
                type="password"
                :placeholder="redisPassSet ? '已配置，留空则不修改' : '无密码可留空'"
                @input="redisPassTouched = true"
              />
            </div>
            <div class="space-y-2">
              <Label for="redis-db">库编号（一般填 0）</Label>
              <Input id="redis-db" v-model.number="redisDB" type="number" min="0" max="15" />
            </div>
            <div class="flex flex-wrap gap-2">
              <Button variant="outline" :disabled="testingRedis" @click="testRedis">
                <Loader2 v-if="testingRedis" class="mr-2 size-4 animate-spin" />
                测试连接
              </Button>
              <Button :disabled="saving" @click="saveRedis">
                <Loader2 v-if="saving" class="mr-2 size-4 animate-spin" />
                <Save v-else class="mr-2 size-4" />
                保存
              </Button>
            </div>

            <Separator />

            <div class="space-y-3">
              <div>
                <h4 class="text-sm font-medium">主站入队（内网通知）</h4>
                <p class="text-xs text-muted-foreground mt-1">
                  主站下单后通知程序入队。密钥可随意填写或随机生成，保存后请同步改主站TjEnqueue.php。
                </p>
              </div>
              <div class="space-y-2">
                <Label>接口地址</Label>
                <div class="flex gap-2">
                  <Input :model-value="enqueueUrl" readonly class="font-mono text-xs" />
                  <Button variant="outline" size="icon" type="button" :disabled="!enqueueUrl" @click="copyEnqueueUrl">
                    <Copy class="size-4" />
                  </Button>
                </div>
              </div>
              <div class="space-y-2">
                <Label for="enqueue-token">入队密钥</Label>
                <Input
                  id="enqueue-token"
                  v-model="enqueueToken"
                  class="font-mono text-xs"
                  placeholder="自定义或点随机生成，至少 4 位"
                />
                <p class="text-xs text-muted-foreground">
                  主站 <code class="text-[11px]">includes/TjEnqueue.php</code> 里改
                  <code class="text-[11px]">$tjEnqueueToken</code>，与这里保持一致。
                </p>
              </div>
              <div class="flex flex-wrap gap-2">
                <Button :disabled="savingEnqueue || loading" @click="saveEnqueueToken">
                  <Loader2 v-if="savingEnqueue" class="mr-2 size-4 animate-spin" />
                  <Save v-else class="mr-2 size-4" />
                  保存密钥
                </Button>
                <Button variant="outline" :disabled="regeneratingEnqueue || loading" @click="copyEnqueueToken">
                  <Copy class="mr-2 size-4" />
                  复制密钥
                </Button>
                <Button variant="outline" :disabled="regeneratingEnqueue || loading" @click="regenerateConfirmOpen = true">
                  <Loader2 v-if="regeneratingEnqueue" class="mr-2 size-4 animate-spin" />
                  <RefreshCw v-else class="mr-2 size-4" />
                  随机生成
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card v-else-if="activeSection === 'http'">
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <Globe class="size-4" />
              对外访问安全
            </CardTitle>
            <CardDescription>控制程序向平台下单时能访问哪些网站，以及能否访问内网</CardDescription>
          </CardHeader>
          <CardContent v-if="loading" class="text-sm text-muted-foreground">加载中…</CardContent>
          <CardContent v-else class="space-y-4 max-w-xl">
            <div class="space-y-2">
              <Label for="host-whitelist">允许访问的网站（每行一个，留空表示不限制）</Label>
              <Textarea
                id="host-whitelist"
                v-model="hostWhitelistText"
                rows="6"
                placeholder="example.com&#10;api.example.com"
              />
              <p class="text-xs text-muted-foreground">填主域名即可，子域名也会允许，如 example.com 包含 a.example.com</p>
            </div>
            <Separator />
            <div class="flex items-start gap-3">
              <Checkbox id="block-private" :checked="blockPrivateNetworks" @update:checked="(v: boolean | 'indeterminate') => (blockPrivateNetworks = v === true)" />
              <div class="space-y-1">
                <Label for="block-private">禁止访问内网地址</Label>
                <p class="text-xs text-muted-foreground">防止程序去访问 127.0.0.1、192.168 等内网 IP</p>
              </div>
            </div>
            <div class="flex items-start gap-3">
              <Checkbox
                id="allow-lan-http"
                :checked="allowInsecureHTTPToLAN"
                :disabled="httpLANLocked"
                @update:checked="(v: boolean | 'indeterminate') => (allowInsecureHTTPToLAN = v === true)"
              />
              <div class="space-y-1">
                <Label for="allow-lan-http">调试模式：允许用明文访问内网</Label>
                <p class="text-xs text-muted-foreground">
                  正式环境已锁定关闭；仅安装前可在配置文件里手动开启，供本地调试
                </p>
              </div>
            </div>
            <Button :disabled="saving" @click="saveHTTPSecurity">
              <Loader2 v-if="saving" class="mr-2 size-4 animate-spin" />
              <Save v-else class="mr-2 size-4" />
              保存设置
            </Button>
          </CardContent>
        </Card>

        <Card v-else-if="activeSection === 'ai'">
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <Sparkles class="size-4" />
              智能助手
            </CardTitle>
          </CardHeader>
          <CardContent v-if="loading" class="text-sm text-muted-foreground">加载中…</CardContent>
          <CardContent v-else class="space-y-4 max-w-lg">
            <p class="text-xs text-muted-foreground">
              配置接口密钥后，可用于「平台规则」里的 AI 转换，以及「AI 运维」里的智能分析。具体开关在 AI 运维页或货源全局配置里。
            </p>
            <div class="flex items-start gap-3">
              <Checkbox id="ai-enabled" :key="aiSettingsKey" v-model:checked="aiEnabled" />
              <div class="space-y-1">
                <div class="flex flex-wrap items-center gap-2">
                  <Label for="ai-enabled">启用 AI 规则转换</Label>
                  <Badge v-if="aiEnabled" variant="default">已启用</Badge>
                  <Badge v-else variant="outline">未启用</Badge>
                </div>
                <p class="text-xs text-muted-foreground">关闭后平台规则页只能本地解析 PHP，不能调用 AI</p>
              </div>
            </div>
            <div class="space-y-2">
              <Label for="ai-base-url">AI 服务地址</Label>
              <Input
                id="ai-base-url"
                v-model="aiBaseURL"
                placeholder="https://api.openai.com/v1"
              />
            </div>
            <div class="space-y-2">
              <Label for="ai-model">模型名称</Label>
              <Input id="ai-model" v-model="aiModel" placeholder="如 deepseek-chat、gpt-4o-mini" />
            </div>
            <div class="space-y-2">
              <Label for="ai-api-key">接口密钥</Label>
              <Badge v-if="aiAPIKeySet && !aiAPIKeyTouched" variant="secondary" class="mr-2">已配置</Badge>
              <Input
                id="ai-api-key"
                v-model="aiAPIKey"
                type="password"
                :placeholder="aiAPIKeySet ? '已配置，留空则不修改' : '粘贴密钥，一般以 sk- 开头'"
                autocomplete="off"
                @input="aiAPIKeyTouched = true"
              />
            </div>
            <p v-if="aiEnabled && !aiAPIKeySet && !aiAPIKeyTouched" class="text-xs text-amber-600">
              已勾选启用，请填写接口密钥后点保存。
            </p>
            <Button :disabled="saving" @click="saveAI">
              <Loader2 v-if="saving" class="mr-2 size-4 animate-spin" />
              <Save v-else class="mr-2 size-4" />
              保存
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>

    <Dialog v-model:open="regenerateConfirmOpen">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>重新生成入队密钥</DialogTitle>
          <DialogDescription>
            重新生成后，主站TjEnqueue.php 里的 $tjEnqueueToken 也要一起改，否则主站无法通知入队。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" type="button" @click="regenerateConfirmOpen = false">取消</Button>
          <Button type="button" :disabled="regeneratingEnqueue" @click="confirmRegenerateEnqueueToken">
            <Loader2 v-if="regeneratingEnqueue" class="mr-2 size-4 animate-spin" />
            确认生成
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
</template>
