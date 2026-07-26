<script setup lang="ts">

import { ref, onMounted, computed } from 'vue'

import { RouterLink, useRouter } from 'vue-router'

import { Plus, Pencil, Trash2, Database, Sparkles, Loader2, BookOpen, Braces, FileCode2, PlayCircle } from '@lucide/vue'

import { toast } from 'vue-sonner'

import { Button } from '@/components/ui/button'

import { Card, CardContent, CardHeader } from '@/components/ui/card'

import { Input } from '@/components/ui/input'

import { Textarea } from '@/components/ui/textarea'

import { Label } from '@/components/ui/label'

import { Badge } from '@/components/ui/badge'

import { Checkbox } from '@/components/ui/checkbox'

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

import { api, defaultRuleConfig, type SubmitPlatform, type SubmitRuleConfig, type RuleTestSubmitResult } from '@/api/submit-platform'

import { parseXdjkPhp } from '@/lib/parse-xdjk-php'



type EditorTab = 'json' | 'php'



const router = useRouter()

const platforms = ref<SubmitPlatform[]>([])

const loading = ref(false)

const showEditor = ref(false)

const editing = ref(false)

const ruleJsonText = ref('')

const phpPasteText = ref('')

const aiConverting = ref(false)

const aiConfigured = ref(false)

const ruleTesting = ref(false)

const ruleFixing = ref(false)

/** 最近一次试单失败上下文，供 AI 修正使用（不再弹窗） */
const testFailPayload = ref<Pick<RuleTestSubmitResult, 'err_msg' | 'upstream_body' | 'oid' | 'huoyuan_name'> | null>(null)

/** 失败说明：试单失败自动填入，也可手填后点 AI 修正 */
const aiFixErrMsg = ref('')

function recordTestFailure(
  payload: Pick<RuleTestSubmitResult, 'err_msg' | 'upstream_body' | 'oid' | 'huoyuan_name'>,
) {
  testPassed.value = false
  testFailPayload.value = payload
  aiFixErrMsg.value = payload.err_msg || ''
}

const aiFixNeedsHuoyuanHint = computed(() => {
  const msg = aiFixErrMsg.value || testFailPayload.value?.err_msg || ''
  return msg.includes('货源地址') || msg.includes('货源配置')
})

const canAiFix = computed(() => {
  if (!aiConfigured.value || ruleFixing.value) return false
  if (!form.value.platform_type.trim()) return false
  return !!(aiFixErrMsg.value.trim() || testFailPayload.value?.err_msg?.trim())
})

const showSaveWithoutTestDialog = ref(false)

const deleteConfirmOpen = ref(false)

const deletingRow = ref<SubmitPlatform | null>(null)

/** 打开编辑时的 rule_config 快照；有变更则须试单通过后再保存 */
const initialRuleSnapshot = ref('')

const testPassed = ref(false)

const editorTab = ref<EditorTab>('json')

const jsonError = ref('')



const form = ref<SubmitPlatform>({

  platform_type: '',

  display_name: '',

  enabled: true,

  remark: '',

  rule_config: defaultRuleConfig(),

})



const sortedPlatforms = computed(() =>

  [...platforms.value].sort((a, b) => a.platform_type.localeCompare(b.platform_type)),

)



const enabledCount = computed(() => platforms.value.filter((p) => p.enabled).length)

function ruleConfigSnapshot(): string {
  return JSON.stringify(form.value.rule_config)
}

const ruleNeedsTest = computed(() => {
  if (initialRuleSnapshot.value === '') return true
  return ruleConfigSnapshot() !== initialRuleSnapshot.value
})

const saveRequiresTest = computed(
  () =>
    !testPassed.value &&
    (initialRuleSnapshot.value === '' || ruleConfigSnapshot() !== initialRuleSnapshot.value),
)

const workflowStep = computed(() => {
  if (!phpPasteText.value.trim() && !form.value.platform_type.trim()) return 1
  if (ruleNeedsTest.value && !testPassed.value) return 2
  return 3
})



async function loadList() {

  loading.value = true

  try {

    const res = await api.list()

    if (res.code === 1 && res.data) {

      platforms.value = res.data

    } else if (res.need_login) {

      await router.push('/login')

    } else {

      toast.error(res.msg || '加载失败')

    }

  } catch {

    toast.error('网络错误')

  } finally {

    loading.value = false

  }

}



function resetEditorState() {

  phpPasteText.value = ''

  editorTab.value = 'json'

  jsonError.value = ''

  initialRuleSnapshot.value = ''

  testPassed.value = false

  testFailPayload.value = null

  aiFixErrMsg.value = ''

}

function markRuleChanged() {

  testPassed.value = false

}

function setInitialRuleBaseline() {

  initialRuleSnapshot.value = ruleConfigSnapshot()

  testPassed.value = true

}



function openCreate() {

  editing.value = false

  resetEditorState()

  form.value = {

    platform_type: '',

    display_name: '',

    enabled: true,

    remark: '',

    rule_config: defaultRuleConfig(),

  }

  ruleJsonText.value = JSON.stringify(form.value.rule_config, null, 2)

  showEditor.value = true

}



async function openEdit(row: SubmitPlatform) {

  editing.value = true

  resetEditorState()

  try {

    const res = await api.get(row.platform_type)

    if (res.code === 1 && res.data) {

      form.value = { ...res.data, rule_config: { ...res.data.rule_config } }

      if (res.data.source_php) {

        phpPasteText.value = res.data.source_php

      }

    } else {

      form.value = { ...row, rule_config: { ...row.rule_config } }

    }

  } catch {

    form.value = { ...row, rule_config: { ...row.rule_config } }

  }

  ruleJsonText.value = JSON.stringify(form.value.rule_config, null, 2)

  setInitialRuleBaseline()

  showEditor.value = true

}



function validateJson(): boolean {

  try {

    JSON.parse(ruleJsonText.value)

    jsonError.value = ''

    return true

  } catch (e) {

    jsonError.value = e instanceof Error ? e.message : 'JSON 格式错误'

    return false

  }

}



function syncRuleFromJson(options?: { silent?: boolean }): boolean {

  if (!validateJson()) {

    if (!options?.silent) toast.error('规则 JSON 格式错误')

    return false

  }

  form.value.rule_config = JSON.parse(ruleJsonText.value) as SubmitRuleConfig

  if (!options?.silent) toast.success('规则 JSON 已解析')

  return true

}



function formatJson() {

  if (!validateJson()) {

    toast.error('无法格式化：JSON 无效')

    return

  }

  const parsed = JSON.parse(ruleJsonText.value) as SubmitRuleConfig

  ruleJsonText.value = JSON.stringify(parsed, null, 2)

  markRuleChanged()

  toast.success('已格式化')

}



function importFromPhp() {

  const result = parseXdjkPhp(phpPasteText.value)

  if ('error' in result) {

    toast.error(result.error)

    if (result.special_notes?.length) {

      toast.warning(result.special_notes.join('\n'), { duration: 8000 })

    }

    return

  }

  applyParsedRule(result.platform_type, result.rule_config, [...result.special_notes, ...result.warnings])

}



function applyParsedRule(platformType: string, ruleConfig: SubmitRuleConfig, msgs: string[], source?: string) {

  if (!editing.value) {

    form.value.platform_type = platformType

    if (!form.value.display_name.trim()) {

      form.value.display_name = platformType

    }

  } else if (form.value.platform_type !== platformType) {

    toast.warning(`识别到平台类型 ${platformType}，与当前不一致，仅更新规则配置`)

  }

  form.value.rule_config = ruleConfig

  ruleJsonText.value = JSON.stringify(ruleConfig, null, 2)

  jsonError.value = ''

  editorTab.value = 'json'

  markRuleChanged()

  const sourceLabel =
    source === 'local'
      ? '本地解析'
      : source === 'hybrid'
        ? '本地+AI'
        : source === 'ai'
          ? 'AI'
          : source === 'submit_fail_fix'
            ? '试单修复'
            : ''

  const prefix = sourceLabel ? `[${sourceLabel}] ` : ''

  if (msgs.length) {

    toast.warning(`${prefix}已填充，请注意：${msgs.join('；')}`, { duration: 8000 })

  } else {

    toast.success(`${prefix}已填充规则配置`)

  }

}



async function importFromAi() {

  const php = phpPasteText.value.trim()

  if (!php) {

    toast.error('请先粘贴 PHP 代码')

    editorTab.value = 'php'

    return

  }

  if (!aiConfigured.value) {

    toast.error('请先在「系统设置 → AI 转换」中配置 API Key')

    return

  }

  aiConverting.value = true

  try {

    const res = await api.aiConvert(php, form.value.platform_type.trim() || undefined)

    if (res.code !== 1 || !res.data) {

      toast.error(res.msg || 'AI 转换失败')

      return

    }

    const msgs = [...(res.data.warnings || [])]

    if (res.data.notes) msgs.unshift(res.data.notes)

    applyParsedRule(res.data.platform_type, res.data.rule_config, msgs, res.data.parse_source)

    toast.info('建议：试单测试通过后再保存', { duration: 5000 })

  } catch {

    toast.error('AI 转换请求失败')

  } finally {

    aiConverting.value = false

  }

}



async function loadAiStatus() {

  try {

    const res = await api.aiStatus()

    aiConfigured.value = res.code === 1 && !!res.data?.configured

  } catch {

    aiConfigured.value = false

  }

}



async function testSubmitRule() {

  if (!syncRuleFromJson({ silent: true })) {

    toast.error('规则 JSON 格式错误')

    return

  }

  if (!form.value.platform_type.trim()) {

    toast.error('请先填写平台类型')

    return

  }

  ruleTesting.value = true

  try {

    const res = await api.testSubmit(form.value.platform_type.trim(), form.value.rule_config)

    if (res.code !== 1 || !res.data) {
      const msg = res.msg || '试单失败'
      recordTestFailure({ err_msg: msg })
      toast.error(`${msg}。可核对失败说明后点「AI 修正」`, { duration: 8000 })
      return
    }

    const data = res.data

    if (!data.has_orders) {

      toast.error(`数据库中没有平台「${form.value.platform_type}」的订单，请先在主站下单后再试`)

      return

    }

    if (data.warning) {

      toast.info(data.warning, { duration: 6000 })

    }

    if (data.success) {

      testPassed.value = true

      toast.success(

        `试单成功，可以保存。订单 ${data.oid}${data.yid ? `，yid=${data.yid}` : ''}${data.err_msg ? `（${data.err_msg}）` : ''}`,

        { duration: 8000 },

      )

      return

    }

    testPassed.value = false

    recordTestFailure({
      err_msg: data.err_msg || '试单失败',
      upstream_body: data.upstream_body,
      oid: data.oid,
      huoyuan_name: data.huoyuan_name,
    })
    toast.error(`${data.err_msg || '试单失败'}。可核对失败说明后点「AI 修正」`, { duration: 8000 })

  } catch {

    toast.error('试单请求失败')

  } finally {

    ruleTesting.value = false

  }

}



async function aiFixRule(andRetest = false) {
  if (!syncRuleFromJson({ silent: true })) {
    toast.error('规则 JSON 格式错误')
    return
  }
  if (!form.value.platform_type.trim()) {
    toast.error('请先填写平台类型')
    return
  }
  const errMsg = aiFixErrMsg.value.trim() || testFailPayload.value?.err_msg?.trim() || ''
  if (!errMsg) {
    toast.error('请填写失败说明，或先执行试单以自动填入')
    return
  }
  if (!aiConfigured.value) {
    toast.error('请先在「系统设置 → AI 转换」中配置 API Key')
    return
  }

  ruleFixing.value = true

  try {
    const php = phpPasteText.value.trim() || undefined
    const res = await api.aiFixFromFailure(form.value.platform_type.trim(), {
      rule_config: form.value.rule_config,
      err_msg: errMsg,
      upstream_body: testFailPayload.value?.upstream_body,
      php,
    })

    if (res.code !== 1 || !res.data) {
      toast.error(res.msg || 'AI 修正失败')
      return
    }

    const msgs = [...(res.data.warnings || [])]
    if (res.data.notes) msgs.unshift(res.data.notes)
    applyParsedRule(res.data.platform_type, res.data.rule_config, msgs, res.data.parse_source || 'submit_fail_fix')

    if (andRetest) {
      toast.success('已填充 AI 修正结果，正在再次试单…')
      await testSubmitRule()
    } else {
      toast.success('已填充 AI 修正结果，请核对后再次试单，通过后再保存')
    }
  } catch {
    toast.error('AI 修正请求失败')
  } finally {
    ruleFixing.value = false
  }
}



async function saveForm(force = false) {

  if (!syncRuleFromJson({ silent: true })) {

    toast.error('规则 JSON 格式错误')

    return

  }

  if (!form.value.platform_type.trim()) {

    toast.error('平台类型不能为空')

    editorTab.value = 'json'

    return

  }

  if (!force && saveRequiresTest.value) {

    showSaveWithoutTestDialog.value = true

    return

  }

  await persistPlatformRule()

}

async function persistPlatformRule() {

  loading.value = true

  try {

    const payload: Partial<SubmitPlatform> = {

      display_name: form.value.display_name,

      enabled: form.value.enabled,

      rule_config: form.value.rule_config,

      remark: form.value.remark,

    }

    const res = editing.value

      ? await api.update(form.value.platform_type, payload)

      : await api.create({ ...form.value, ...payload })

    if (res.code === 1) {

      toast.success(editing.value ? '更新成功' : '创建成功')

      showEditor.value = false

      showSaveWithoutTestDialog.value = false

      await loadList()

    } else {

      toast.error(res.msg || '保存失败')

    }

  } catch {

    toast.error('保存失败')

  } finally {

    loading.value = false

  }

}



function openDeleteConfirm(row: SubmitPlatform) {

  deletingRow.value = row

  deleteConfirmOpen.value = true

}



async function confirmDeleteRow() {

  if (!deletingRow.value) return

  deleteConfirmOpen.value = false

  const row = deletingRow.value

  deletingRow.value = null

  loading.value = true

  try {

    const res = await api.remove(row.platform_type)

    if (res.code === 1) {

      toast.success('已删除')

      await loadList()

    } else {

      toast.error(res.msg || '删除失败')

    }

  } finally {

    loading.value = false

  }

}



onMounted(() => {

  loadList()

  loadAiStatus()

})

</script>



<template>

  <Card>

    <CardHeader class="flex-row flex-wrap items-center justify-between gap-3 space-y-0 pb-3">

      <p v-if="sortedPlatforms.length" class="text-sm text-muted-foreground">

        共 {{ sortedPlatforms.length }} 条，启用 {{ enabledCount }} 条

      </p>

      <div v-else class="text-sm text-muted-foreground">暂无规则</div>

      <div class="flex flex-wrap gap-2">

        <Button variant="outline" size="sm" :disabled="loading" @click="loadList">

          <Database class="mr-1 size-4" /> 重新加载

        </Button>

        <Button size="sm" @click="openCreate">

          <Plus class="mr-1 size-4" /> 新增平台

        </Button>

      </div>

    </CardHeader>

    <CardContent>

      <div v-if="loading && !platforms.length" class="py-8 text-center text-muted-foreground">加载中...</div>

      <div v-else-if="!sortedPlatforms.length" class="py-8 text-center text-muted-foreground">

        点击「新增平台」添加第一条规则

      </div>

      <Table v-else>

        <TableHeader>

          <TableRow>

            <TableHead>平台类型</TableHead>

            <TableHead>名称</TableHead>

            <TableHead>状态</TableHead>

            <TableHead>版本</TableHead>

            <TableHead>备注</TableHead>

            <TableHead class="text-right">操作</TableHead>

          </TableRow>

        </TableHeader>

        <TableBody>

          <TableRow v-for="row in sortedPlatforms" :key="row.platform_type">

            <TableCell class="font-mono font-medium">{{ row.platform_type }}</TableCell>

            <TableCell>{{ row.display_name }}</TableCell>

            <TableCell>

              <Badge :variant="row.enabled ? 'default' : 'secondary'">

                {{ row.enabled ? '启用' : '禁用' }}

              </Badge>

            </TableCell>

            <TableCell>{{ row.version ?? 1 }}</TableCell>

            <TableCell class="max-w-xs truncate text-muted-foreground">{{ row.remark }}</TableCell>

            <TableCell class="text-right">

              <div class="flex justify-end gap-1">

                <Button variant="ghost" size="icon-sm" @click="openEdit(row)">

                  <Pencil class="size-4" />

                </Button>

                <Button variant="ghost" size="icon-sm" @click="openDeleteConfirm(row)">

                  <Trash2 class="size-4 text-destructive" />

                </Button>

              </div>

            </TableCell>

          </TableRow>

        </TableBody>

      </Table>

    </CardContent>

  </Card>



  <Dialog v-model:open="showEditor">

    <DialogContent class="flex max-h-[90vh] max-w-4xl flex-col gap-0 overflow-hidden p-0 sm:max-w-4xl">

      <DialogHeader class="space-y-1 border-b px-6 py-4 text-left">

        <DialogTitle>{{ editing ? '编辑平台规则' : '新增平台规则' }}</DialogTitle>

        <DialogDescription>

          推荐流程：① PHP 导入 / AI 转换 → ② 试单测试 → ③ 试单通过后再保存

        </DialogDescription>

        <div class="flex flex-wrap gap-2 pt-2">

          <Badge :variant="workflowStep >= 1 ? 'default' : 'outline'">① 转换</Badge>

          <Badge :variant="workflowStep >= 2 ? 'default' : 'outline'">② 试单</Badge>

          <Badge :variant="workflowStep >= 3 ? 'default' : 'outline'">③ 保存</Badge>

          <Badge v-if="testPassed" variant="secondary" class="bg-green-600/15 text-green-700 dark:text-green-400">

            试单已通过

          </Badge>

          <Badge v-else-if="saveRequiresTest" variant="outline" class="text-amber-700 dark:text-amber-400">

            须试单通过后再保存

          </Badge>

        </div>

      </DialogHeader>



      <div class="min-h-0 flex-1 overflow-y-auto px-6 py-5">

        <section class="space-y-4 rounded-lg border bg-muted/20 p-4">

          <p class="text-xs font-medium uppercase tracking-wide text-muted-foreground">基本信息</p>

          <div class="grid gap-4 sm:grid-cols-2">

            <div class="space-y-2">

              <Label for="platform-type">平台类型</Label>

              <Input

                id="platform-type"

                v-model="form.platform_type"

                :disabled="editing"

                class="font-mono"

                placeholder="如 27、2xx、benz"

              />

            </div>

            <div class="space-y-2">

              <Label for="display-name">显示名称</Label>

              <Input id="display-name" v-model="form.display_name" placeholder="如 27系统" />

            </div>

          </div>

          <div class="space-y-2">

            <Label for="remark">备注</Label>

            <Input id="remark" v-model="form.remark" placeholder="可选" />

          </div>

          <div class="flex items-center gap-2">

            <Checkbox id="enabled" v-model:checked="form.enabled" />

            <Label for="enabled" class="cursor-pointer font-normal">启用该平台规则</Label>

          </div>

        </section>



        <div class="mt-5 flex flex-wrap items-center gap-2 border-b pb-2">

          <button

            type="button"

            :class="

              cn(

                'inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',

                editorTab === 'json'

                  ? 'bg-primary text-primary-foreground shadow-sm'

                  : 'text-muted-foreground hover:bg-muted hover:text-foreground',

              )

            "

            @click="editorTab = 'json'"

          >

            <Braces class="size-4" />

            规则 JSON

          </button>

          <button

            type="button"

            :class="

              cn(

                'inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',

                editorTab === 'php'

                  ? 'bg-primary text-primary-foreground shadow-sm'

                  : 'text-muted-foreground hover:bg-muted hover:text-foreground',

              )

            "

            @click="editorTab = 'php'"

          >

            <FileCode2 class="size-4" />

            PHP 导入

          </button>

          <Button variant="ghost" size="sm" class="ml-auto" as-child>

            <RouterLink to="/docs" target="_blank">

              <BookOpen class="mr-1 size-4" />

              开发文档

            </RouterLink>

          </Button>

        </div>



        <div class="mt-4 space-y-2 rounded-lg border bg-muted/30 p-3">

          <Label for="ai-fix-err" class="text-xs font-medium">失败说明（AI 修正依据）</Label>

          <Textarea

            id="ai-fix-err"

            v-model="aiFixErrMsg"

            :rows="2"

            class="text-xs"

            placeholder="试单失败会自动填入；也可手填，如：404、成功码不匹配、缺参数字段名…"

          />

          <p v-if="testFailPayload?.oid" class="text-xs text-muted-foreground">

            最近试单：订单 {{ testFailPayload.oid }}

            <template v-if="testFailPayload.huoyuan_name">（{{ testFailPayload.huoyuan_name }}）</template>

          </p>

          <p v-if="aiFixNeedsHuoyuanHint" class="text-xs text-amber-600 dark:text-amber-400">

            若规则 URL 已修正仍报货源地址错误，请到「货源配置」检查该渠道的 url（可只填域名或 IP:端口，无需写 http://）。

          </p>

        </div>



        <div v-show="editorTab === 'php'" class="mt-4 space-y-3">

          <p class="text-sm text-muted-foreground">

            粘贴 xdjk.php 中单个

            <code class="rounded bg-muted px-1 text-xs">if ($type == &quot;xxx&quot;)</code>

            分支。复杂平台优先

            <strong class="font-medium text-foreground">AI 智能转换</strong>（本地草稿 + AI 补全）；试单失败或手填说明后点

            <strong class="font-medium text-foreground">AI 修正</strong>；简单 form 下单可点「本地解析」。

          </p>

          <div class="flex flex-wrap gap-2">

            <Button variant="secondary" size="sm" type="button" @click="importFromPhp">

              本地解析

            </Button>

            <Button

              variant="default"

              size="sm"

              type="button"

              :disabled="aiConverting || !aiConfigured"

              @click="importFromAi"

            >

              <Loader2 v-if="aiConverting" class="mr-1 size-4 animate-spin" />

              <Sparkles v-else class="mr-1 size-4" />

              AI 智能转换

            </Button>

            <Button

              variant="outline"

              size="sm"

              type="button"

              :disabled="!canAiFix"

              :title="canAiFix ? '根据失败说明修正当前规则 JSON' : '需填写失败说明或先试单失败'"

              @click="aiFixRule(false)"

            >

              <Loader2 v-if="ruleFixing" class="mr-1 size-4 animate-spin" />

              <Sparkles v-else class="mr-1 size-4" />

              AI 修正

            </Button>

          </div>

          <p v-if="!aiConfigured" class="text-xs text-amber-600 dark:text-amber-400">

            未配置 AI：请先在「系统设置 → AI 转换」填写 API Key

          </p>

          <Textarea

            v-model="phpPasteText"

            :rows="12"

            class="font-mono text-xs leading-relaxed"

            placeholder="粘贴 PHP 代码…"

          />

        </div>



        <div v-show="editorTab === 'json'" class="mt-4 space-y-3">

          <div class="flex flex-wrap items-center justify-between gap-2">

            <p class="text-sm text-muted-foreground">

              保存前会自动校验 JSON。支持 handler: pipeline / script / process 等高级字段。

            </p>

            <div class="flex gap-2">

              <Button variant="outline" size="sm" type="button" @click="formatJson">格式化</Button>

              <Button
                variant="secondary"
                size="sm"
                type="button"
                @click="() => { if (syncRuleFromJson()) markRuleChanged() }"
              >
                校验并应用
              </Button>

            </div>

          </div>

          <Textarea

            v-model="ruleJsonText"

            :rows="18"

            class="font-mono text-xs leading-relaxed"

            :class="jsonError ? 'border-destructive focus-visible:ring-destructive' : ''"

            @blur="validateJson"

          />

          <p v-if="jsonError" class="text-xs text-destructive">{{ jsonError }}</p>

        </div>

      </div>



      <DialogFooter class="border-t px-6 py-4 sm:justify-between">

        <Button

          variant="secondary"

          type="button"

          :disabled="ruleTesting || loading || !form.platform_type.trim()"

          @click="testSubmitRule"

        >

          <Loader2 v-if="ruleTesting" class="mr-2 size-4 animate-spin" />

          <PlayCircle v-else class="mr-2 size-4" />

          试单测试

        </Button>

        <Button

          v-if="canAiFix"

          variant="outline"

          type="button"

          :disabled="ruleFixing || ruleTesting"

          @click="aiFixRule(true)"

        >

          <Loader2 v-if="ruleFixing" class="mr-2 size-4 animate-spin" />

          <Sparkles v-else class="mr-2 size-4" />

          AI 修正并试单

        </Button>

        <div class="flex gap-2">

          <Button variant="outline" type="button" @click="showEditor = false">取消</Button>

          <Button type="button" :disabled="loading" @click="saveForm()">

            {{ saveRequiresTest ? '保存（须先试单）' : '保存' }}

          </Button>

        </div>

      </DialogFooter>

    </DialogContent>

  </Dialog>



  <Dialog v-model:open="showSaveWithoutTestDialog">

    <DialogContent class="max-w-md">

      <DialogHeader>

        <DialogTitle>尚未试单通过</DialogTitle>

        <DialogDescription>

          当前规则未通过试单或转换后还未试单。建议先点「试单测试」，成功后再保存，避免错误规则上线。

        </DialogDescription>

      </DialogHeader>

      <DialogFooter>

        <Button variant="outline" type="button" @click="showSaveWithoutTestDialog = false">返回试单</Button>

        <Button variant="destructive" type="button" :disabled="loading" @click="persistPlatformRule()">

          仍要保存

        </Button>

      </DialogFooter>

    </DialogContent>

  </Dialog>



  <Dialog v-model:open="deleteConfirmOpen">

    <DialogContent class="max-w-md">

      <DialogHeader>

        <DialogTitle>删除平台规则</DialogTitle>

        <DialogDescription>

          确定删除平台规则「{{ deletingRow?.platform_type }}」？此操作不可撤销。

        </DialogDescription>

      </DialogHeader>

      <DialogFooter>

        <Button variant="outline" type="button" @click="deleteConfirmOpen = false">取消</Button>

        <Button variant="destructive" type="button" :disabled="loading" @click="confirmDeleteRow">确认删除</Button>

      </DialogFooter>

    </DialogContent>

  </Dialog>

</template>


