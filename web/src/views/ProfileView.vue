<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  Bell,
  CheckCircle2,
  Copy,
  KeyRound,
  Loader2,
  Megaphone,
  ShieldCheck,
  ShieldOff,
} from '@lucide/vue'
import { Checkbox } from '@/components/ui/checkbox'
import { toast } from 'vue-sonner'
import { copyToClipboard } from '@/lib/clipboard'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader } from '@/components/ui/card'
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
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import QRCode from 'qrcode'
import { cn } from '@/lib/utils'
import { profileApi, type AdminProfile, type NotificationConfig } from '@/api/profile'

type Section = 'showdoc' | 'notify' | 'password' | 'totp'

const router = useRouter()
const profile = ref<AdminProfile | null>(null)
const loading = ref(true)
const activeSection = ref<Section>('showdoc')

const showdocURL = ref('')
const showdocCode = ref('')
const showdocVerifyToken = ref('')
const showdocSending = ref(false)
const unbindConfirmOpen = ref(false)

const notifyForm = ref<NotificationConfig>({
  enabled: false,
  notify_submit_failure: true,
  notify_submit_timeout: true,
  notify_db_write_failure: true,
  notify_processing_timeout: true,
})
const notifySaving = ref(false)

const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const pwdTotpCode = ref('')
const pwdSaving = ref(false)

const totpSecret = ref('')
const totpQrDataUrl = ref('')
const totpVerifyCode = ref('')
const totpVerifyToken = ref('')
const totpAppCode = ref('')
const disablePassword = ref('')
const disableTotpCode = ref('')

const initials = computed(() => (profile.value?.username || 'AD').slice(0, 2).toUpperCase())

const navItems: { id: Section; label: string; icon: typeof Bell }[] = [
  { id: 'showdoc', label: 'Showdoc', icon: Bell },
  { id: 'notify', label: '通知', icon: Megaphone },
  { id: 'password', label: '密码', icon: KeyRound },
  { id: 'totp', label: '二步验证', icon: ShieldCheck },
]

async function loadNotifications() {
  const res = await profileApi.getNotifications()
  if (res.code === 1 && res.data) {
    notifyForm.value = { ...res.data.config }
  }
}

async function loadProfile() {
  loading.value = true
  try {
    const res = await profileApi.get()
    if (res.code === 1 && res.data) {
      profile.value = res.data
      showdocURL.value = res.data.showdoc_url || ''
      await loadNotifications()
    } else if (res.need_login) {
      await router.push('/login')
    }
  } finally {
    loading.value = false
  }
}

async function saveNotifications(options?: { silent?: boolean }) {
  if (!profile.value?.showdoc_bound) {
    if (!options?.silent) toast.error('请先绑定 Showdoc')
    return false
  }
  notifySaving.value = true
  try {
    const res = await profileApi.saveNotifications({ ...notifyForm.value })
    if (res.code === 1 && res.data) {
      notifyForm.value = { ...res.data }
      if (!options?.silent) toast.success('通知配置已保存')
      return true
    }
    toast.error(res.msg || '保存失败')
    await loadNotifications()
    return false
  } finally {
    notifySaving.value = false
  }
}

function onNotifyChecked(field: keyof NotificationConfig, value: boolean | 'indeterminate') {
  notifyForm.value = { ...notifyForm.value, [field]: value === true }
  void saveNotifications({ silent: field !== 'enabled' })
}

async function sendShowdocCode() {
  if (!showdocURL.value.trim()) {
    toast.error('请填写 Showdoc 推送地址')
    return
  }
  showdocSending.value = true
  try {
    const res = await profileApi.showdocSendCode(showdocURL.value.trim())
    if (res.code === 1 && res.data?.verify_token) {
      showdocVerifyToken.value = res.data.verify_token
      toast.success('验证码已推送，请查收 Showdoc')
    } else {
      toast.error(res.msg || '发送失败')
    }
  } finally {
    showdocSending.value = false
  }
}

async function bindShowdoc() {
  const res = await profileApi.showdocBind({
    url: showdocURL.value.trim(),
    code: showdocCode.value.trim(),
    verify_token: showdocVerifyToken.value,
  })
  if (res.code === 1) {
    toast.success('Showdoc 绑定成功')
    showdocCode.value = ''
    showdocVerifyToken.value = ''
    await loadProfile()
  } else {
    toast.error(res.msg || '绑定失败')
  }
}

async function confirmUnbindShowdoc() {
  unbindConfirmOpen.value = false
  const res = await profileApi.showdocUnbind()
  if (res.code === 1) {
    toast.success('已解绑')
    await loadProfile()
  } else {
    toast.error(res.msg || '操作失败')
  }
}

async function testShowdoc() {
  const res = await profileApi.showdocTest()
  if (res.code === 1) toast.success('测试推送已发送')
  else toast.error(res.msg || '发送失败')
}

async function changePassword() {
  pwdSaving.value = true
  try {
    const res = await profileApi.changePassword({
      old_password: oldPassword.value,
      new_password: newPassword.value,
      confirm_password: confirmPassword.value,
      totp_code: pwdTotpCode.value || undefined,
    })
    if (res.code === 1) {
      toast.success('密码已修改')
      oldPassword.value = ''
      newPassword.value = ''
      confirmPassword.value = ''
      pwdTotpCode.value = ''
    } else {
      toast.error(res.msg || '修改失败')
    }
  } finally {
    pwdSaving.value = false
  }
}

async function sendTotpVerifyCode() {
  const res = await profileApi.totpSendCode()
  if (res.code === 1 && res.data?.verify_token) {
    totpVerifyToken.value = res.data.verify_token
    toast.success('验证码已推送到 Showdoc')
  } else {
    toast.error(res.msg || '发送失败')
  }
}

async function startTotpSetup() {
  const res = await profileApi.totpSetup()
  if (res.code === 1 && res.data) {
    totpSecret.value = res.data.secret
    totpQrDataUrl.value = ''
    if (res.data.otpauth_url) {
      try {
        totpQrDataUrl.value = await QRCode.toDataURL(res.data.otpauth_url, {
          width: 200,
          margin: 2,
        })
      } catch {
        toast.error('二维码生成失败，请使用下方密钥手动添加')
      }
    }
    toast.success('请使用验证器 App 扫描二维码')
  } else {
    toast.error(res.msg || '生成失败')
  }
}

async function enableTotp() {
  const res = await profileApi.totpEnable({
    secret: totpSecret.value,
    totp_code: totpAppCode.value.trim(),
    verify_code: totpVerifyCode.value.trim(),
    verify_token: totpVerifyToken.value,
  })
  if (res.code === 1) {
    toast.success('二步验证已开启')
    totpSecret.value = ''
    totpQrDataUrl.value = ''
    totpVerifyCode.value = ''
    totpVerifyToken.value = ''
    totpAppCode.value = ''
    await loadProfile()
  } else {
    toast.error(res.msg || '开启失败')
  }
}

async function disableTotp() {
  const res = await profileApi.totpDisable({
    password: disablePassword.value,
    totp_code: disableTotpCode.value.trim(),
  })
  if (res.code === 1) {
    toast.success('二步验证已关闭')
    disablePassword.value = ''
    disableTotpCode.value = ''
    await loadProfile()
  } else {
    toast.error(res.msg || '关闭失败')
  }
}

async function copySecret() {
  if (!totpSecret.value) return
  const ok = await copyToClipboard(totpSecret.value)
  if (ok) toast.success('密钥已复制')
  else toast.error('复制失败，请手动选中后复制')
}

onMounted(loadProfile)
</script>

<template>
  <Card class="flex min-h-[calc(100vh-7rem)] flex-col">
    <CardHeader class="flex flex-col gap-3 space-y-0 pb-3">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex min-w-0 flex-wrap items-center gap-3">
          <template v-if="loading">
            <Skeleton class="h-6 w-24 rounded-full" />
            <Skeleton class="h-6 w-24 rounded-full" />
          </template>
          <template v-else-if="profile">
            <Badge variant="outline" class="gap-1 font-normal">
              <Avatar class="size-4 rounded-sm">
                <AvatarFallback class="rounded-sm text-[10px]">{{ initials }}</AvatarFallback>
              </Avatar>
              {{ profile.username }}
            </Badge>
            <Badge :variant="profile.showdoc_bound ? 'default' : 'secondary'" class="gap-1 font-normal">
              Showdoc {{ profile.showdoc_bound ? '已绑定' : '未绑定' }}
            </Badge>
            <Badge :variant="profile.totp_enabled ? 'default' : 'secondary'" class="gap-1 font-normal">
              二步验证 {{ profile.totp_enabled ? '已开启' : '未开启' }}
            </Badge>
          </template>
        </div>
      </div>
      <div class="flex w-full gap-1 rounded-lg border bg-muted/40 p-1">
        <button
          v-for="item in navItems"
          :key="item.id"
          type="button"
          :class="
            cn(
              'flex flex-1 items-center justify-center gap-1.5 rounded-md px-3 py-2 text-sm font-medium transition-colors',
              activeSection === item.id
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground',
            )
          "
          @click="activeSection = item.id"
        >
          <component :is="item.icon" class="size-3.5 shrink-0" />
          <span class="truncate">{{ item.label }}</span>
        </button>
      </div>
    </CardHeader>

    <CardContent class="min-h-0 flex-1 pb-4">
      <ScrollArea class="h-[calc(100vh-11rem)] rounded-md border">
        <div class="w-full space-y-6 p-4">
          <!-- Showdoc -->
          <div v-show="activeSection === 'showdoc'" class="flex w-full flex-col space-y-5">
            <div>
              <h3 class="text-sm font-medium">Showdoc 推送</h3>
            </div>

            <div v-if="profile?.showdoc_bound" class="flex w-full items-start gap-3 rounded-lg border bg-muted/30 p-4">
              <CheckCircle2 class="mt-0.5 size-5 shrink-0 text-emerald-600" />
              <div class="min-w-0 flex-1 space-y-1">
                <p class="text-sm font-medium">已绑定</p>
                <p class="break-all font-mono text-xs text-muted-foreground">{{ profile.showdoc_url }}</p>
              </div>
            </div>

            <div v-if="!profile?.showdoc_bound" class="grid w-full gap-4">
              <div class="space-y-2">
                <Label for="showdoc-url">推送地址</Label>
                <Input
                  id="showdoc-url"
                  v-model="showdocURL"
                  class="w-full font-mono text-sm"
                  placeholder="https://push.showdoc.com.cn/server/api/push/..."
                />
              </div>

              <div class="space-y-2">
                <Label for="showdoc-code">验证码</Label>
                <div class="flex w-full gap-2">
                  <Input
                    id="showdoc-code"
                    v-model="showdocCode"
                    class="min-w-0 flex-1 tracking-widest"
                    placeholder="000000"
                    maxlength="6"
                    inputmode="numeric"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    class="shrink-0"
                    :disabled="showdocSending || !showdocURL.trim()"
                    @click="sendShowdocCode"
                  >
                    <Loader2 v-if="showdocSending" class="mr-1 size-3.5 animate-spin" />
                    发送验证码
                  </Button>
                </div>
                <p class="text-xs text-muted-foreground">验证码将推送到上方地址，10 分钟内有效</p>
              </div>
            </div>

            <template v-if="!profile?.showdoc_bound">
              <Button
                type="button"
                class="w-full"
                :disabled="!showdocCode || !showdocVerifyToken"
                @click="bindShowdoc"
              >
                确认绑定
              </Button>
            </template>

            <div v-else class="grid w-full grid-cols-2 gap-2">
              <Button type="button" variant="outline" @click="testShowdoc">测试推送</Button>
              <Button type="button" variant="outline" @click="unbindConfirmOpen = true">解绑</Button>
            </div>
          </div>

          <!-- 通知配置 -->
          <div v-show="activeSection === 'notify'" class="flex w-full flex-col space-y-5">
            <h3 class="text-sm font-medium">订单通知</h3>

            <Alert v-if="!profile?.showdoc_bound">
              <AlertDescription>请先在 Showdoc 标签页完成绑定，绑定成功后将自动开启通知。</AlertDescription>
            </Alert>

            <template v-else>
              <div class="flex w-full items-center gap-2 rounded-lg border p-3">
                <Checkbox
                  id="notify-enabled"
                  :checked="notifyForm.enabled"
                  :disabled="notifySaving"
                  @update:checked="(v: boolean | 'indeterminate') => onNotifyChecked('enabled', v)"
                />
                <div class="flex flex-1 flex-wrap items-center gap-2">
                  <Label for="notify-enabled" class="cursor-pointer font-normal">启用 Showdoc 通知</Label>
                  <Badge :variant="notifyForm.enabled ? 'default' : 'secondary'">
                    {{ notifyForm.enabled ? '已启用' : '未启用' }}
                  </Badge>
                </div>
              </div>

              <div class="grid w-full gap-3 sm:grid-cols-2">
                <label
                  class="flex w-full items-start gap-2 rounded-lg border p-3 text-sm transition-colors hover:bg-muted/40"
                  :class="{ 'opacity-50': !notifyForm.enabled }"
                >
                  <Checkbox
                    class="mt-0.5"
                    :checked="notifyForm.notify_submit_failure"
                    :disabled="!notifyForm.enabled || notifySaving"
                    @update:checked="(v: boolean | 'indeterminate') => onNotifyChecked('notify_submit_failure', v)"
                  />
                  <span>提交失败（上游拒绝或业务终态失败）</span>
                </label>
                <label
                  class="flex w-full items-start gap-2 rounded-lg border p-3 text-sm transition-colors hover:bg-muted/40"
                  :class="{ 'opacity-50': !notifyForm.enabled }"
                >
                  <Checkbox
                    class="mt-0.5"
                    :checked="notifyForm.notify_submit_timeout"
                    :disabled="!notifyForm.enabled || notifySaving"
                    @update:checked="(v: boolean | 'indeterminate') => onNotifyChecked('notify_submit_timeout', v)"
                  />
                  <span>提交超时（网络超时且重试仍失败）</span>
                </label>
                <label
                  class="flex w-full items-start gap-2 rounded-lg border p-3 text-sm transition-colors hover:bg-muted/40"
                  :class="{ 'opacity-50': !notifyForm.enabled }"
                >
                  <Checkbox
                    class="mt-0.5"
                    :checked="notifyForm.notify_db_write_failure"
                    :disabled="!notifyForm.enabled || notifySaving"
                    @update:checked="(v: boolean | 'indeterminate') => onNotifyChecked('notify_db_write_failure', v)"
                  />
                  <span>写库失败（上游已成功但本地未写入）</span>
                </label>
                <label
                  class="flex w-full items-start gap-2 rounded-lg border p-3 text-sm transition-colors hover:bg-muted/40"
                  :class="{ 'opacity-50': !notifyForm.enabled }"
                >
                  <Checkbox
                    class="mt-0.5"
                    :checked="notifyForm.notify_processing_timeout"
                    :disabled="!notifyForm.enabled || notifySaving"
                    @update:checked="(v: boolean | 'indeterminate') => onNotifyChecked('notify_processing_timeout', v)"
                  />
                  <span>处理超时（在队列中停留过久）</span>
                </label>
              </div>

              <p v-if="notifySaving" class="text-xs text-muted-foreground">保存中…</p>
            </template>
          </div>

          <!-- 密码 -->
          <div v-show="activeSection === 'password'" class="space-y-5">
            <div>
              <h3 class="text-sm font-medium">登录密码</h3>
              <CardDescription class="mt-1">定期更换密码可提高账户安全性</CardDescription>
            </div>

            <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              <div class="space-y-2">
                <Label for="old-pwd">原密码</Label>
                <Input id="old-pwd" v-model="oldPassword" type="password" autocomplete="current-password" />
              </div>
              <div class="space-y-2">
                <Label for="new-pwd">新密码</Label>
                <Input id="new-pwd" v-model="newPassword" type="password" autocomplete="new-password" />
                <p class="text-xs text-muted-foreground">至少 6 位字符</p>
              </div>
              <div class="space-y-2">
                <Label for="confirm-pwd">确认新密码</Label>
                <Input id="confirm-pwd" v-model="confirmPassword" type="password" autocomplete="new-password" />
              </div>
              <div v-if="profile?.totp_enabled" class="space-y-2 sm:col-span-2 lg:col-span-1">
                <Label for="pwd-totp">验证器动态码</Label>
                <Input
                  id="pwd-totp"
                  v-model="pwdTotpCode"
                  class="max-w-xs tracking-widest"
                  placeholder="000000"
                  maxlength="8"
                  inputmode="numeric"
                />
              </div>
            </div>

            <div class="flex justify-end pt-2">
              <Button type="button" :disabled="pwdSaving" @click="changePassword">
                <Loader2 v-if="pwdSaving" class="mr-2 size-4 animate-spin" />
                保存密码
              </Button>
            </div>
          </div>

          <!-- TOTP -->
          <div v-show="activeSection === 'totp'" class="space-y-5">
            <div>
              <h3 class="text-sm font-medium">二步验证 (TOTP)</h3>
              <CardDescription class="mt-1">登录时除密码外，还需输入验证器 App 动态码</CardDescription>
            </div>

            <div
              v-if="profile?.totp_enabled"
              class="flex items-start gap-3 rounded-lg border border-emerald-500/20 bg-emerald-500/5 p-4"
            >
              <ShieldCheck class="mt-0.5 size-5 shrink-0 text-emerald-600" />
              <div>
                <p class="text-sm font-medium text-emerald-800 dark:text-emerald-300">二步验证已开启</p>
                <p class="mt-1 text-sm text-muted-foreground">每次登录需输入验证器 App 中的 6 位动态码。</p>
              </div>
            </div>

            <Alert v-if="!profile?.totp_enabled && !profile?.showdoc_bound">
              <AlertDescription>请先在 Showdoc 中完成绑定，才能接收验证码并开启二步验证。</AlertDescription>
            </Alert>

            <template v-if="!profile?.totp_enabled && profile?.showdoc_bound">
              <ol class="grid gap-5 text-sm lg:grid-cols-3">
                <li class="flex gap-3 rounded-lg border p-4">
                  <span class="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">1</span>
                  <div class="min-w-0 flex-1 space-y-2">
                    <p class="font-medium">扫码或手动添加验证器</p>
                    <Button type="button" variant="outline" size="sm" @click="startTotpSetup">生成二维码</Button>
                    <div v-if="totpSecret" class="space-y-3 rounded-lg border bg-muted/40 p-3">
                      <div v-if="totpQrDataUrl" class="flex flex-col items-center gap-2">
                        <img
                          :src="totpQrDataUrl"
                          alt="二步验证二维码"
                          class="size-[200px] rounded-md border bg-white p-1"
                        />
                        <p class="text-center text-xs text-muted-foreground">使用 Google Authenticator、Microsoft Authenticator 等 App 扫码</p>
                      </div>
                      <div>
                        <div class="mb-2 flex items-center justify-between">
                          <span class="text-xs font-medium text-muted-foreground">无法扫码？手动输入密钥</span>
                          <Button type="button" variant="ghost" size="sm" class="h-7 px-2" @click="copySecret">
                            <Copy class="mr-1 size-3.5" /> 复制
                          </Button>
                        </div>
                        <code class="block break-all font-mono text-sm">{{ totpSecret }}</code>
                      </div>
                    </div>
                  </div>
                </li>
                <li class="flex gap-3 rounded-lg border p-4">
                  <span class="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">2</span>
                  <div class="min-w-0 flex-1 space-y-2">
                    <p class="font-medium">获取 Showdoc 验证码</p>
                    <div class="flex gap-2">
                      <Input
                        v-model="totpVerifyCode"
                        class="min-w-0 flex-1 tracking-widest"
                        placeholder="000000"
                        maxlength="6"
                        inputmode="numeric"
                      />
                      <Button type="button" variant="outline" class="shrink-0" @click="sendTotpVerifyCode">
                        发送验证码
                      </Button>
                    </div>
                  </div>
                </li>
                <li class="flex gap-3 rounded-lg border p-4">
                  <span class="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">3</span>
                  <div class="min-w-0 flex-1 space-y-2">
                    <p class="font-medium">输入验证器动态码完成绑定</p>
                    <Input v-model="totpAppCode" class="tracking-widest" placeholder="000000" maxlength="8" inputmode="numeric" />
                  </div>
                </li>
              </ol>
              <div class="flex justify-end pt-2">
                <Button
                  type="button"
                  :disabled="!totpSecret || !totpVerifyToken || !totpVerifyCode.trim() || !totpAppCode.trim()"
                  @click="enableTotp"
                >
                  开启二步验证
                </Button>
              </div>
            </template>

            <template v-if="profile?.totp_enabled">
              <Separator />
              <p class="text-sm font-medium text-muted-foreground">关闭二步验证</p>
              <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                <div class="space-y-2">
                  <Label>当前密码</Label>
                  <Input v-model="disablePassword" type="password" autocomplete="current-password" />
                </div>
                <div class="space-y-2">
                  <Label>验证器动态码</Label>
                  <Input v-model="disableTotpCode" class="tracking-widest" maxlength="8" inputmode="numeric" />
                </div>
              </div>
              <div class="flex justify-end pt-2">
                <Button type="button" variant="outline" @click="disableTotp">
                  <ShieldOff class="mr-2 size-4" />
                  关闭二步验证
                </Button>
              </div>
            </template>
          </div>
        </div>
      </ScrollArea>
    </CardContent>
  </Card>

  <Dialog v-model:open="unbindConfirmOpen">
    <DialogContent class="max-w-md">
      <DialogHeader>
        <DialogTitle>解绑 Showdoc</DialogTitle>
        <DialogDescription>确定解绑 Showdoc？解绑后订单异常通知将无法推送。</DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button variant="outline" type="button" @click="unbindConfirmOpen = false">取消</Button>
        <Button variant="destructive" type="button" @click="confirmUnbindShowdoc">确认解绑</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
