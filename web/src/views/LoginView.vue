<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { LogIn } from '@lucide/vue'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { authApi, setSession } from '@/api/auth'
import { adminProductName } from '@/lib/admin-nav'

const router = useRouter()
const username = ref('')
const password = ref('')
const totpCode = ref('')
const needTotp = ref(false)
const loading = ref(false)

async function onSubmit() {
  if (!username.value.trim() || !password.value) {
    toast.error('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    const res = await authApi.login(
      username.value.trim(),
      password.value,
      needTotp.value ? totpCode.value.trim() : undefined,
    )
    if (res.code === 1 && res.data?.token) {
      setSession(res.data.token, res.data.username)
      toast.success('登录成功')
      await router.replace('/')
      return
    }
    if (res.code === 2 || res.need_totp) {
      needTotp.value = true
      toast.message('请输入验证器动态码')
      return
    }
    toast.error(res.msg || '登录失败')
  } catch {
    toast.error('网络错误，请确认 tj 后端已启动')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="flex min-h-svh flex-col items-center justify-center bg-muted/40 p-4">
    <Card class="w-full max-w-md">
      <CardHeader class="text-center">
        <CardTitle>{{ adminProductName }}</CardTitle>
        <CardDescription>请登录管理后台</CardDescription>
      </CardHeader>
      <CardContent>
        <form class="space-y-4" @submit.prevent="onSubmit">
          <div class="space-y-2">
            <Label for="username">用户名</Label>
            <Input id="username" v-model="username" placeholder="管理员用户名" autocomplete="username" />
          </div>
          <div class="space-y-2">
            <Label for="password">密码</Label>
            <Input id="password" v-model="password" type="password" placeholder="密码" autocomplete="current-password" />
          </div>
          <div v-if="needTotp" class="space-y-2">
            <Label for="totp">验证器动态码</Label>
            <Input id="totp" v-model="totpCode" placeholder="6 位动态码" maxlength="8" autocomplete="one-time-code" />
          </div>
          <Button type="submit" class="w-full" :disabled="loading">
            <LogIn class="mr-2 size-4" />
            {{ loading ? '登录中...' : needTotp ? '验证并登录' : '登录' }}
          </Button>
          <div class="text-center">
            <RouterLink to="/forgot-password" class="text-sm text-muted-foreground hover:text-foreground">
              忘记密码？
            </RouterLink>
          </div>
        </form>
      </CardContent>
    </Card>
  </div>
</template>
