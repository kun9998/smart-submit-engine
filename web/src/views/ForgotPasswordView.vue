<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft, KeyRound, Loader2 } from '@lucide/vue'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { authApi } from '@/api/auth'

const router = useRouter()
const username = ref('')
const verifyCode = ref('')
const verifyToken = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const sending = ref(false)
const resetting = ref(false)

async function sendCode() {
  if (!username.value.trim()) {
    toast.error('请填写用户名')
    return
  }
  sending.value = true
  try {
    const res = await authApi.forgotPasswordSendCode(username.value.trim())
    if (res.code === 1) {
      if (res.data?.verify_token) {
        verifyToken.value = res.data.verify_token
        toast.success('验证码已推送到 Showdoc')
      } else {
        verifyToken.value = ''
        toast.message(res.msg || '若该账号已绑定 Showdoc，验证码已发送')
      }
    } else {
      toast.error(res.msg || '发送失败')
    }
  } catch {
    toast.error('网络错误，请确认后端已启动')
  } finally {
    sending.value = false
  }
}

async function resetPassword() {
  if (!username.value.trim() || !verifyCode.value.trim() || !verifyToken.value) {
    toast.error('请先发送并填写 Showdoc 验证码')
    return
  }
  if (newPassword.value.length < 6) {
    toast.error('新密码至少 6 位')
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    toast.error('两次密码不一致')
    return
  }
  resetting.value = true
  try {
    const res = await authApi.forgotPasswordReset({
      username: username.value.trim(),
      verify_code: verifyCode.value.trim(),
      verify_token: verifyToken.value,
      new_password: newPassword.value,
      confirm_password: confirmPassword.value,
    })
    if (res.code === 1) {
      toast.success('密码已重置')
      await router.replace('/login')
    } else {
      toast.error(res.msg || '重置失败')
    }
  } catch {
    toast.error('网络错误，请确认后端已启动')
  } finally {
    resetting.value = false
  }
}
</script>

<template>
  <div class="flex min-h-svh flex-col items-center justify-center bg-muted/40 p-4">
    <Card class="w-full max-w-md">
      <CardHeader class="text-center">
        <CardTitle>忘记密码</CardTitle>
        <CardDescription>通过已绑定的 Showdoc 推送验证码重置密码</CardDescription>
      </CardHeader>
      <CardContent>
        <form class="space-y-4" @submit.prevent="resetPassword">
          <div class="space-y-2">
            <Label for="fp-username">用户名</Label>
            <Input
              id="fp-username"
              v-model="username"
              placeholder="管理员用户名"
              autocomplete="username"
            />
          </div>

          <div class="space-y-2">
            <Label for="fp-code">Showdoc 验证码</Label>
            <div class="flex gap-2">
              <Input
                id="fp-code"
                v-model="verifyCode"
                class="min-w-0 flex-1 tracking-widest"
                placeholder="000000"
                maxlength="6"
                inputmode="numeric"
                autocomplete="one-time-code"
              />
              <Button
                type="button"
                variant="outline"
                class="shrink-0"
                :disabled="sending || !username.trim()"
                @click="sendCode"
              >
                <Loader2 v-if="sending" class="mr-1 size-3.5 animate-spin" />
                发送验证码
              </Button>
            </div>
            <p class="text-xs text-muted-foreground">验证码将推送到该账号绑定的 Showdoc，10 分钟内有效</p>
          </div>

          <div class="space-y-2">
            <Label for="fp-new">新密码</Label>
            <Input
              id="fp-new"
              v-model="newPassword"
              type="password"
              placeholder="至少 6 位"
              autocomplete="new-password"
            />
          </div>

          <div class="space-y-2">
            <Label for="fp-confirm">确认新密码</Label>
            <Input
              id="fp-confirm"
              v-model="confirmPassword"
              type="password"
              autocomplete="new-password"
            />
          </div>

          <Button
            type="submit"
            class="w-full"
            :disabled="resetting || !verifyToken || !verifyCode.trim() || !newPassword || !confirmPassword"
          >
            <KeyRound class="mr-2 size-4" />
            {{ resetting ? '重置中...' : '重置密码' }}
          </Button>

          <Button type="button" variant="ghost" class="w-full" @click="router.push('/login')">
            <ArrowLeft class="mr-2 size-4" />
            返回登录
          </Button>
        </form>
      </CardContent>
    </Card>
  </div>
</template>
