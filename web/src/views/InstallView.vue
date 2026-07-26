<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Check, Circle, Database, Dot, UserPlus } from '@lucide/vue'
import { notifyError, notifySuccess } from '@/lib/toast'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Separator } from '@/components/ui/separator'
import {
  Stepper,
  StepperDescription,
  StepperItem,
  StepperSeparator,
  StepperTitle,
  StepperTrigger,
} from '@/components/ui/stepper'
import { authApi, setSession, type DbConnectionParams } from '@/api/auth'

const router = useRouter()
const stepIndex = ref(1)
const loading = ref(false)

const mainHost = ref('127.0.0.1')
const mainPort = ref(3306)
const mainUser = ref('root')
const mainPassword = ref('')
const mainDatabase = ref('www')
const mainTablePrefix = ref('love_learn')

const pluginHost = ref('127.0.0.1')
const pluginPort = ref(3306)
const pluginUser = ref('root')
const pluginPassword = ref('')
const pluginDatabase = ref('tj_plugin')

const adminUsername = ref('admin')
const adminPassword = ref('')
const confirmPassword = ref('')

const steps = [
  { step: 1, title: '数据库', description: '两套独立连接' },
  { step: 2, title: '管理员账号', description: '设置登录账号' },
]

function mainConn(): DbConnectionParams {
  return {
    host: mainHost.value.trim(),
    port: Number(mainPort.value) || 3306,
    user: mainUser.value.trim(),
    db_password: mainPassword.value,
    database: mainDatabase.value.trim(),
  }
}

function pluginConn(): DbConnectionParams {
  return {
    host: pluginHost.value.trim(),
    port: Number(pluginPort.value) || 3306,
    user: pluginUser.value.trim(),
    db_password: pluginPassword.value,
    database: pluginDatabase.value.trim(),
  }
}

function validateConn(c: DbConnectionParams, label: string) {
  if (!c.host) {
    notifyError(`请填写${label}主机地址`)
    return false
  }
  if (!c.user) {
    notifyError(`请填写${label}用户名`)
    return false
  }
  if (!c.database) {
    notifyError(`请填写${label}数据库名称`)
    return false
  }
  return true
}

async function testMainDB() {
  const c = mainConn()
  if (!validateConn(c, '主站库')) return false
  loading.value = true
  try {
    const res = await authApi.testDB(c, 'main')
    if (res.code === 1) {
      notifySuccess('主站数据库连接成功')
      return true
    }
    notifyError(res.msg || '主站库连接失败')
    return false
  } catch {
    notifyError('网络错误，请确认后端已启动')
    return false
  } finally {
    loading.value = false
  }
}

async function testPluginDB() {
  const c = pluginConn()
  if (!validateConn(c, '插件库')) return false
  loading.value = true
  try {
    const res = await authApi.testDB(c, 'plugin')
    if (res.code === 1) {
      notifySuccess('插件数据库连接成功')
      return true
    }
    notifyError(res.msg || '插件库连接失败')
    return false
  } catch {
    notifyError('网络错误')
    return false
  } finally {
    loading.value = false
  }
}

async function goToAdminStep() {
  const mainOk = await testMainDB()
  if (!mainOk) return
  const pluginOk = await testPluginDB()
  if (pluginOk) stepIndex.value = 2
}

async function runInstall() {
  const main = mainConn()
  const plugin = pluginConn()
  if (!validateConn(main, '主站库') || !validateConn(plugin, '插件库')) return
  if (adminPassword.value.length < 6) {
    notifyError('管理员密码至少 6 位')
    return
  }
  if (adminPassword.value !== confirmPassword.value) {
    notifyError('两次密码不一致')
    return
  }
  loading.value = true
  try {
    const res = await authApi.install({
      main_host: main.host,
      main_port: main.port,
      main_user: main.user,
      main_db_password: main.db_password,
      main_database: main.database,
      table_prefix: mainTablePrefix.value.trim() || 'love_learn',
      plugin_host: plugin.host,
      plugin_port: plugin.port,
      plugin_user: plugin.user,
      plugin_db_password: plugin.db_password,
      plugin_database: plugin.database,
      username: adminUsername.value.trim(),
      password: adminPassword.value,
      confirm_password: confirmPassword.value,
      authcode: '',
    })
    if (res.code === 1) {
      notifySuccess('安装成功')
      if (res.data?.token) {
        setSession(res.data.token, res.data.username || adminUsername.value)
        await router.replace('/')
      } else {
        await router.replace('/login')
      }
    } else {
      notifyError(res.msg || '安装失败')
    }
  } catch {
    notifyError('安装请求失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="flex min-h-svh flex-col items-center justify-center bg-muted/40 p-4 py-8">
    <Card class="w-full max-w-2xl">
      <CardHeader>
        <CardTitle>首次安装</CardTitle>
      </CardHeader>
      <CardContent>
        <Stepper v-model="stepIndex" class="mb-6 block w-full">
          <div class="flex w-full items-start gap-2">
            <StepperItem
              v-for="item in steps"
              :key="item.step"
              v-slot="{ state }"
              class="relative flex w-full flex-col items-center justify-center"
              :step="item.step"
            >
              <StepperSeparator
                v-if="item.step !== steps[steps.length - 1]?.step"
                class="absolute left-[calc(50%+20px)] right-[calc(-50%+10px)] top-5 block h-0.5 shrink-0 rounded-full bg-muted group-data-[state=completed]:bg-primary"
              />
              <StepperTrigger as-child>
                <Button
                  :variant="state === 'completed' || state === 'active' ? 'default' : 'outline'"
                  size="icon"
                  class="z-10 shrink-0 rounded-full"
                  :class="[state === 'active' && 'ring-2 ring-ring ring-offset-2 ring-offset-background']"
                  type="button"
                  :disabled="item.step === 2 && stepIndex < 2"
                >
                  <Check v-if="state === 'completed'" class="size-5" />
                  <Circle v-else-if="state === 'active'" class="size-5" />
                  <Dot v-else class="size-5" />
                </Button>
              </StepperTrigger>
              <div class="mt-5 flex flex-col items-center text-center">
                <StepperTitle :class="[state === 'active' && 'text-primary']" class="text-sm font-semibold">
                  {{ item.title }}
                </StepperTitle>
                <StepperDescription class="text-xs text-muted-foreground">
                  {{ item.description }}
                </StepperDescription>
              </div>
            </StepperItem>
          </div>
        </Stepper>

        <Separator class="mb-6" />

        <div v-show="stepIndex === 1" class="space-y-6">
          <div class="space-y-4 rounded-xl border p-4">
            <div class="flex items-center gap-2 text-sm font-medium">
              <Database class="size-4" />
              你的站点数据库
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <div class="space-y-2">
                <Label for="main-host">主机地址</Label>
                <Input id="main-host" v-model="mainHost" placeholder="127.0.0.1" />
              </div>
              <div class="space-y-2">
                <Label for="main-port">端口</Label>
                <Input id="main-port" v-model.number="mainPort" type="number" placeholder="3306" />
              </div>
              <div class="space-y-2">
                <Label for="main-user">用户名</Label>
                <Input id="main-user" v-model="mainUser" placeholder="root" />
              </div>
              <div class="space-y-2">
                <Label for="main-pass">数据库密码</Label>
                <Input id="main-pass" v-model="mainPassword" type="password" placeholder="主站库密码" />
              </div>
              <div class="space-y-2">
                <Label for="main-db">数据库名称</Label>
                <Input id="main-db" v-model="mainDatabase" placeholder="www" />
              </div>
              <div class="space-y-2">
                <Label for="main-prefix">表前缀</Label>
                <Input id="main-prefix" v-model="mainTablePrefix" placeholder="love_learn" />
              </div>
            </div>
            <Button variant="outline" size="sm" type="button" :disabled="loading" @click="testMainDB">
              测试主站库连接
            </Button>
          </div>

          <div class="space-y-4 rounded-xl border p-4">
            <div class="flex items-center gap-2 text-sm font-medium">
              <Database class="size-4" />
              插件数据库
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <div class="space-y-2">
                <Label for="plugin-host">主机地址</Label>
                <Input id="plugin-host" v-model="pluginHost" placeholder="127.0.0.1" />
              </div>
              <div class="space-y-2">
                <Label for="plugin-port">端口</Label>
                <Input id="plugin-port" v-model.number="pluginPort" type="number" placeholder="3306" />
              </div>
              <div class="space-y-2">
                <Label for="plugin-user">用户名</Label>
                <Input id="plugin-user" v-model="pluginUser" placeholder="root" />
              </div>
              <div class="space-y-2">
                <Label for="plugin-pass">数据库密码</Label>
                <Input id="plugin-pass" v-model="pluginPassword" type="password" placeholder="插件库密码" />
              </div>
              <div class="space-y-2">
                <Label for="plugin-db">数据库名称</Label>
                <Input id="plugin-db" v-model="pluginDatabase" placeholder="tj_plugin" />
              </div>
            </div>
            <Button variant="outline" size="sm" type="button" :disabled="loading" @click="testPluginDB">
              测试插件库连接
            </Button>
          </div>

          <Button class="w-full" type="button" :disabled="loading" @click="goToAdminStep">
            测试全部连接并继续
          </Button>
        </div>

        <div v-show="stepIndex === 2" class="space-y-4">
          <div class="space-y-2">
            <Label for="admin-user">管理员用户名</Label>
            <Input id="admin-user" v-model="adminUsername" placeholder="admin" />
          </div>
          <div class="space-y-2">
            <Label for="admin-pass">登录密码</Label>
            <Input id="admin-pass" v-model="adminPassword" type="password" placeholder="至少 6 位" />
          </div>
          <div class="space-y-2">
            <Label for="admin-pass2">确认密码</Label>
            <Input id="admin-pass2" v-model="confirmPassword" type="password" />
          </div>
          <div class="flex gap-2">
            <Button variant="outline" type="button" @click="stepIndex = 1">上一步</Button>
            <Button class="flex-1" type="button" :disabled="loading" @click="runInstall">
              <UserPlus class="mr-2 size-4" />
              {{ loading ? '安装中...' : '完成安装' }}
            </Button>
          </div>
          <Alert>
            <AlertTitle>安装提示</AlertTitle>
            <AlertDescription>
              若订单队列未启动，请重启程序。
            </AlertDescription>
          </Alert>
        </div>
      </CardContent>
    </Card>
  </div>
</template>
