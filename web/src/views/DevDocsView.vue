<script setup lang="ts">
import { ref } from 'vue'
import { Copy, Check, ExternalLink } from '@lucide/vue'
import { toast } from 'vue-sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader } from '@/components/ui/card'
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
import { cn } from '@/lib/utils'
import { DEV_DOC_EXAMPLES, SPECIAL_CASES } from '@/lib/dev-docs-examples'
import {
  CONTENT_TYPE_ROWS,
  FORM_CONFIG_TIPS,
  HZW_FORM_EXAMPLE,
} from '@/lib/dev-docs-content-type'
import {
  ORDER_TEMPLATE_VARS,
  HUOYUAN_TEMPLATE_VARS,
  TEMPLATE_FUNCTIONS,
} from '@/lib/template-vars'
import { ADVANCED_FEATURES, ADVANCED_RULE_EXAMPLES, COVERAGE_SUMMARY, STILL_NEED_GO, TIER_LABELS, XDJK_PLATFORM_COVERAGE } from '@/lib/dev-docs-advanced'
import { defaultRuleConfig } from '@/api/submit-platform'
import { copyToClipboard } from '@/lib/clipboard'

type DocSection = 'guide' | 'vars' | 'examples' | 'advanced'

const navItems: { id: DocSection; label: string }[] = [
  { id: 'guide', label: '快速上手' },
  { id: 'vars', label: '可用字段' },
  { id: 'examples', label: '平台示例' },
  { id: 'advanced', label: '复杂情况' },
]

const tagLabel: Record<string, string> = {
  标准: '常见',
  JSON: 'JSON',
  GET: 'GET',
  Cookie: '带 Cookie',
  特殊: '特殊',
  V2: '新版接口',
}

const activeSection = ref<DocSection>('guide')
const activeExample = ref(DEV_DOC_EXAMPLES[0].id)
const activeAdvanced = ref(ADVANCED_RULE_EXAMPLES[0].id)
const copiedKey = ref('')

const tagColor: Record<string, string> = {
  标准: 'bg-blue-500/15 text-blue-700 dark:text-blue-300',
  JSON: 'bg-violet-500/15 text-violet-700 dark:text-violet-300',
  GET: 'bg-cyan-500/15 text-cyan-700 dark:text-cyan-300',
  Cookie: 'bg-amber-500/15 text-amber-700 dark:text-amber-300',
  特殊: 'bg-orange-500/15 text-orange-700 dark:text-orange-300',
}

const defaultJson = JSON.stringify(defaultRuleConfig(), null, 2)
const hzwFormJson = JSON.stringify(HZW_FORM_EXAMPLE.rule_config, null, 2)
const tplHint = '{{...}}'
const ssrsUrlTpl = '{{huoyuan.url}}/api.php?act=add'

async function copyText(key: string, text: string) {
  if (!text?.trim()) {
    toast.error('没有可复制的内容')
    return
  }
  const ok = await copyToClipboard(text)
  if (!ok) {
    toast.error('复制失败，请手动选中后复制')
    return
  }
  copiedKey.value = key
  toast.success('已复制')
  setTimeout(() => {
    if (copiedKey.value === key) copiedKey.value = ''
  }, 2000)
}

function exampleJson(id: string) {
  const ex = DEV_DOC_EXAMPLES.find((e) => e.id === id)
  return ex ? JSON.stringify(ex.rule_config, null, 2) : ''
}

function advancedJson(id: string) {
  const ex = ADVANCED_RULE_EXAMPLES.find((e) => e.id === id)
  return ex ? JSON.stringify(ex.rule_config, null, 2) : ''
}
</script>

<template>
  <Card class="flex min-h-[calc(100vh-7rem)] flex-col">
    <CardHeader class="flex-row flex-wrap items-center justify-between gap-3 space-y-0 pb-3">
      <div class="flex gap-1 rounded-lg border bg-muted/40 p-1">
        <button
          v-for="item in navItems"
          :key="item.id"
          type="button"
          :class="
            cn(
              'rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
              activeSection === item.id
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground',
            )
          "
          @click="activeSection = item.id"
        >
          {{ item.label }}
        </button>
      </div>
    </CardHeader>

    <CardContent class="min-h-0 flex-1 pb-4">
      <ScrollArea class="h-[calc(100vh-11rem)] rounded-md border">
        <div class="w-full space-y-8 p-4">
          <!-- 入门 -->
          <div v-show="activeSection === 'guide'" class="space-y-8">
            <section class="space-y-3">
              <div>
                <h3 class="text-sm font-medium">从 PHP 转成下单规则</h3>
                <CardDescription class="mt-1">
                  把主站 xdjk.php 里各平台的 PHP 代码，改写成「平台规则」里的 JSON；也可以直接在
                  <RouterLink to="/platforms" class="text-primary underline-offset-4 hover:underline">平台规则</RouterLink>
                  页粘贴 PHP，点「解析」自动生成。
                </CardDescription>
              </div>
              <ol class="list-decimal space-y-2 pl-5 text-sm leading-relaxed text-muted-foreground">
                <li>在 xdjk.php 找到 <code class="rounded bg-muted px-1">if ($type == "xxx")</code> 整段并复制。</li>
                <li>把 <code class="rounded bg-muted px-1">$data = array(...)</code> 里的字段，填到规则的 <code class="rounded bg-muted px-1">body</code> 里。</li>
                <li>配好网址、怎么判断成功、表单还是 JSON 等。</li>
                <li><code class="rounded bg-muted px-1">platform_type</code> 要和货源里的平台类型（pt）一致。</li>
              </ol>
            </section>

            <Separator />

            <section class="space-y-3">
              <h3 class="text-sm font-medium">规则 JSON 长什么样</h3>
              <div class="relative">
                <Button
                  variant="ghost"
                  size="icon"
                  class="absolute right-2 top-2 size-8"
                  type="button"
                  @click="copyText('schema', defaultJson)"
                >
                  <Check v-if="copiedKey === 'schema'" class="size-4 text-emerald-500" />
                  <Copy v-else class="size-4" />
                </Button>
                <pre class="overflow-x-auto rounded-lg border bg-zinc-950 p-4 text-xs leading-relaxed text-zinc-100">{{ defaultJson }}</pre>
              </div>
              <ul class="space-y-1 text-sm text-muted-foreground">
                <li><strong class="text-foreground">method / url</strong> — 怎么请求、请求哪个网址</li>
                <li><strong class="text-foreground">content_type</strong> — <code class="rounded bg-muted px-1">form</code>（表单）或 <code class="rounded bg-muted px-1">json</code>（JSON  body）</li>
                <li><strong class="text-foreground">use_cookie</strong> — PHP 里 get_url 第三个参数传了 $cookie 时设为 true</li>
                <li><strong class="text-foreground">body</strong> — 提交字段，值用 <code class="rounded bg-muted px-1">{{ tplHint }}</code> 模板</li>
                <li><strong class="text-foreground">response</strong> — 怎么算成功（success_codes）、上游单号在哪（yid_field / yid_path）</li>
              </ul>
            </section>

            <Separator />

            <section class="space-y-4">
              <div>
                <h3 class="text-sm font-medium">表单提交（form）</h3>
                <CardDescription class="mt-1">
                  像 hzw、27、29、ssrs 这类 PHP 里 <code class="rounded bg-muted px-1">get_url($url, $data)</code> 的写法，规则里设
                  <code class="rounded bg-muted px-1">"content_type": "form"</code> 即可，不用自己写 Content-Type。
                </CardDescription>
              </div>
              <div class="overflow-x-auto rounded-md border">
                <Table class="w-full">
                  <TableHeader>
                    <TableRow>
                      <TableHead class="w-[14%]">提交格式</TableHead>
                      <TableHead class="w-[28%]">实际请求头</TableHead>
                      <TableHead class="w-[28%]">body 怎么发</TableHead>
                      <TableHead>对应 PHP 写法</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    <TableRow v-for="row in CONTENT_TYPE_ROWS" :key="row.value">
                      <TableCell class="font-mono text-xs">{{ row.value }}</TableCell>
                      <TableCell class="font-mono text-xs text-muted-foreground">{{ row.header }}</TableCell>
                      <TableCell class="text-sm text-muted-foreground">{{ row.bodyDesc }}</TableCell>
                      <TableCell class="font-mono text-xs text-muted-foreground">{{ row.php }}</TableCell>
                    </TableRow>
                  </TableBody>
                </Table>
              </div>
              <ul class="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
                <li v-for="(tip, i) in FORM_CONFIG_TIPS" :key="i">{{ tip }}</li>
              </ul>
              <div class="grid gap-4 lg:grid-cols-2">
                <div>
                  <p class="mb-2 text-xs font-medium text-muted-foreground">hzw PHP（xdjk.php 407–427 行）</p>
                  <pre class="overflow-auto rounded-lg border bg-muted/40 p-3 text-xs leading-relaxed">{{ HZW_FORM_EXAMPLE.php }}</pre>
                </div>
                <div>
                  <div class="mb-2 flex items-center justify-between">
                    <span class="text-xs font-medium text-muted-foreground">hzw 下单规则（form）</span>
                    <Button variant="ghost" size="sm" type="button" @click="copyText('hzw-form', hzwFormJson)">
                      <Copy class="mr-1 size-3.5" /> 复制
                    </Button>
                  </div>
                  <pre class="overflow-auto rounded-lg border bg-zinc-950 p-3 text-xs leading-relaxed text-zinc-100">{{ hzwFormJson }}</pre>
                </div>
              </div>
              <ul class="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
                <li v-for="(note, i) in HZW_FORM_EXAMPLE.notes" :key="i">{{ note }}</li>
              </ul>
            </section>

            <Separator />

            <section class="space-y-4">
              <div>
                <h3 class="text-sm font-medium">ssrs 对照示例</h3>
                <CardDescription class="mt-1">xdjk.php 里 ssrs 段与默认规则一一对应，方便对照学习</CardDescription>
              </div>
              <div class="grid gap-4 lg:grid-cols-2">
                <div>
                  <p class="mb-2 text-xs font-medium text-muted-foreground">PHP 原文</p>
                  <pre class="overflow-auto rounded-lg border bg-muted/40 p-3 text-xs leading-relaxed">{{ DEV_DOC_EXAMPLES[0].php }}</pre>
                </div>
                <div>
                  <p class="mb-2 text-xs font-medium text-muted-foreground">下单规则 JSON</p>
                  <pre class="overflow-auto rounded-lg border bg-zinc-950 p-3 text-xs leading-relaxed text-zinc-100">{{ exampleJson('ssrs') }}</pre>
                </div>
              </div>
              <div class="grid gap-2 text-sm sm:grid-cols-2">
                <div class="rounded-md border p-3">
                  <span class="text-muted-foreground">网址</span>
                  <p class="font-mono text-xs">→ {{ ssrsUrlTpl }}</p>
                </div>
                <div class="rounded-md border p-3">
                  <span class="text-muted-foreground">成功条件</span>
                  <p class="font-mono text-xs">code == "0" → ["0", 0]</p>
                </div>
                <div class="rounded-md border p-3">
                  <span class="text-muted-foreground">请求体</span>
                  <p class="font-mono text-xs">array 键 → body 同名键</p>
                </div>
                <div class="rounded-md border p-3">
                  <span class="text-muted-foreground">提交方式</span>
                  <p class="font-mono text-xs">POST + form</p>
                </div>
              </div>
            </section>
          </div>

          <!-- 模板变量 -->
          <div v-show="activeSection === 'vars'" class="space-y-6">
            <div>
              <h3 class="text-sm font-medium">可用字段与写法</h3>
              <CardDescription class="mt-1">可在 url、headers、body、body_raw 里用 <code class="rounded bg-muted px-1">{{ tplHint }}</code> 引用</CardDescription>
            </div>
            <div>
              <p class="mb-2 text-xs font-medium text-muted-foreground">订单字段 order.*</p>
              <div class="overflow-x-auto rounded-md border">
                <Table class="w-full">
                  <TableHeader>
                    <TableRow>
                      <TableHead class="w-[32%]">写法</TableHead>
                      <TableHead class="w-[28%]">PHP 变量</TableHead>
                      <TableHead>含义</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    <TableRow v-for="row in ORDER_TEMPLATE_VARS" :key="row.var">
                      <TableCell class="font-mono text-xs">{{ row.var }}</TableCell>
                      <TableCell class="font-mono text-xs text-muted-foreground">{{ row.php || '—' }}</TableCell>
                      <TableCell class="text-muted-foreground">{{ row.desc }}</TableCell>
                    </TableRow>
                  </TableBody>
                </Table>
              </div>
            </div>
            <div>
              <p class="mb-2 text-xs font-medium text-muted-foreground">货源字段 huoyuan.*</p>
              <div class="overflow-x-auto rounded-md border">
                <Table class="w-full">
                  <TableHeader>
                    <TableRow>
                      <TableHead class="w-[32%]">写法</TableHead>
                      <TableHead class="w-[28%]">PHP 变量</TableHead>
                      <TableHead>含义</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    <TableRow v-for="row in HUOYUAN_TEMPLATE_VARS" :key="row.var">
                      <TableCell class="font-mono text-xs">{{ row.var }}</TableCell>
                      <TableCell class="font-mono text-xs text-muted-foreground">{{ row.php || '—' }}</TableCell>
                      <TableCell class="text-muted-foreground">{{ row.desc }}</TableCell>
                    </TableRow>
                  </TableBody>
                </Table>
              </div>
            </div>
            <div>
              <p class="mb-2 text-xs font-medium text-muted-foreground">常用函数</p>
              <div class="overflow-x-auto rounded-md border">
                <Table class="w-full">
                  <TableHeader>
                    <TableRow>
                      <TableHead class="w-[45%]">写法</TableHead>
                      <TableHead>含义</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    <TableRow v-for="row in TEMPLATE_FUNCTIONS" :key="row.var">
                      <TableCell class="font-mono text-xs">{{ row.var }}</TableCell>
                      <TableCell class="text-muted-foreground">{{ row.desc }}</TableCell>
                    </TableRow>
                  </TableBody>
                </Table>
              </div>
            </div>
          </div>

          <!-- 示例库 -->
          <div v-show="activeSection === 'examples'" class="space-y-5">
            <div>
              <h3 class="text-sm font-medium">平台示例</h3>
              <CardDescription class="mt-1">点平台名切换示例，可复制 JSON 到「平台规则」编辑器</CardDescription>
            </div>
            <div class="flex flex-wrap gap-2">
              <Button
                v-for="ex in DEV_DOC_EXAMPLES"
                :key="ex.id"
                :variant="activeExample === ex.id ? 'default' : 'outline'"
                size="sm"
                type="button"
                @click="activeExample = ex.id"
              >
                {{ ex.platform_type }}
                <Badge variant="secondary" class="ml-1.5" :class="tagColor[ex.tag]">{{ tagLabel[ex.tag] || ex.tag }}</Badge>
              </Button>
            </div>
            <template v-for="ex in DEV_DOC_EXAMPLES" :key="ex.id">
              <div v-show="activeExample === ex.id" class="space-y-4">
                <div>
                  <h4 class="font-medium">{{ ex.title }}</h4>
                  <p class="mt-1 text-sm text-muted-foreground">{{ ex.summary }}</p>
                </div>
                <div class="grid gap-4 lg:grid-cols-2">
                  <div>
                    <div class="mb-2 flex items-center justify-between">
                      <span class="text-xs font-medium text-muted-foreground">PHP</span>
                      <Button variant="ghost" size="sm" type="button" @click="copyText(`php-${ex.id}`, ex.php)">
                        <Copy class="mr-1 size-3.5" /> 复制
                      </Button>
                    </div>
                    <pre class="overflow-auto rounded-lg border bg-muted/40 p-3 text-xs leading-relaxed">{{ ex.php }}</pre>
                  </div>
                  <div>
                    <div class="mb-2 flex items-center justify-between">
                      <span class="text-xs font-medium text-muted-foreground">下单规则 JSON</span>
                      <Button variant="ghost" size="sm" type="button" @click="copyText(`json-${ex.id}`, exampleJson(ex.id))">
                        <Copy class="mr-1 size-3.5" /> 复制
                      </Button>
                    </div>
                    <pre class="overflow-auto rounded-lg border bg-zinc-950 p-3 text-xs leading-relaxed text-zinc-100">{{ exampleJson(ex.id) }}</pre>
                  </div>
                </div>
                <ul v-if="ex.notes?.length" class="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
                  <li v-for="(note, i) in ex.notes" :key="i">{{ note }}</li>
                </ul>
              </div>
            </template>
          </div>

          <!-- 高级 -->
          <div v-show="activeSection === 'advanced'" class="space-y-8">
            <section class="space-y-4">
              <div>
                <h3 class="text-sm font-medium">复杂平台怎么配</h3>
                <CardDescription class="mt-1">普通写法搞不定时，用下面这些 JSON 字段</CardDescription>
              </div>
              <div class="overflow-x-auto rounded-md border">
                <Table class="w-full">
                  <TableHeader>
                    <TableRow>
                      <TableHead class="w-[24%]">配置项</TableHead>
                      <TableHead class="w-[36%]">干什么用</TableHead>
                      <TableHead>示例写法</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    <TableRow v-for="f in ADVANCED_FEATURES" :key="f.feature">
                      <TableCell class="font-mono text-xs">{{ f.feature }}</TableCell>
                      <TableCell class="text-sm text-muted-foreground">{{ f.desc }}</TableCell>
                      <TableCell class="font-mono text-xs">{{ f.template }}</TableCell>
                    </TableRow>
                  </TableBody>
                </Table>
              </div>
              <div class="flex flex-wrap gap-2">
                <Button
                  v-for="ex in ADVANCED_RULE_EXAMPLES"
                  :key="ex.id"
                  :variant="activeAdvanced === ex.id ? 'default' : 'outline'"
                  size="sm"
                  type="button"
                  @click="activeAdvanced = ex.id"
                >
                  {{ ex.platform }}
                </Button>
              </div>
              <template v-for="ex in ADVANCED_RULE_EXAMPLES" :key="ex.id">
                <div v-show="activeAdvanced === ex.id" class="space-y-3">
                  <div>
                    <h4 class="font-medium">{{ ex.title }}（{{ ex.platform }}）</h4>
                    <p class="mt-1 text-sm text-emerald-600 dark:text-emerald-400">{{ ex.solution }}</p>
                  </div>
                  <div class="relative">
                    <Button
                      variant="ghost"
                      size="sm"
                      class="absolute right-2 top-2 z-10"
                      type="button"
                      @click="copyText(`adv-${ex.id}`, advancedJson(ex.id))"
                    >
                      <Copy class="mr-1 size-3.5" /> 复制规则
                    </Button>
                    <pre class="overflow-auto rounded-lg border bg-zinc-950 p-4 pt-10 text-xs leading-relaxed text-zinc-100">{{ advancedJson(ex.id) }}</pre>
                  </div>
                </div>
              </template>
            </section>

            <Separator />

            <section class="space-y-4">
              <h3 class="text-sm font-medium">不好配的平台速查</h3>
              <div class="overflow-x-auto rounded-md border">
                <Table class="w-full">
                  <TableHeader>
                    <TableRow>
                      <TableHead class="w-[22%]">平台</TableHead>
                      <TableHead class="w-[38%]">原因</TableHead>
                      <TableHead>建议</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    <TableRow v-for="item in SPECIAL_CASES" :key="item.platform">
                      <TableCell class="font-medium">{{ item.platform }}</TableCell>
                      <TableCell class="text-sm text-muted-foreground">{{ item.reason }}</TableCell>
                      <TableCell class="text-sm">{{ item.suggestion }}</TableCell>
                    </TableRow>
                  </TableBody>
                </Table>
              </div>
            </section>

            <Separator />

            <section class="space-y-4">
              <div>
                <h3 class="text-sm font-medium">xdjk.php 里有哪些平台（AI / 本地解析参考）</h3>
                <CardDescription class="mt-1">
                  共 {{ COVERAGE_SUMMARY.total }} 个平台分支，其中 {{ COVERAGE_SUMMARY.ready }} 个可直接写成 JSON 规则（{{ COVERAGE_SUMMARY.coveragePct }}%）。AI 转换和本地解析会优先用普通请求、分支、课代表 kcid、多步流程等方式。
                </CardDescription>
              </div>
              <div class="flex flex-wrap gap-2 text-xs">
                <Badge v-for="(count, tier) in COVERAGE_SUMMARY.byTier" :key="tier" variant="secondary">
                  {{ TIER_LABELS[tier as keyof typeof TIER_LABELS] }}: {{ count }}
                </Badge>
              </div>
              <p v-if="STILL_NEED_GO.length" class="text-sm text-amber-600 dark:text-amber-400">
                还需手写代码：{{ STILL_NEED_GO.join('、') }}（下单后要改主库）
              </p>
              <div class="overflow-x-auto rounded-md border max-h-64">
                <Table class="w-full">
                  <TableHeader>
                    <TableRow>
                      <TableHead class="w-[14%]">平台类型</TableHead>
                      <TableHead class="w-[16%]">名称</TableHead>
                      <TableHead class="w-[14%]">写法分类</TableHead>
                      <TableHead>备注</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    <TableRow v-for="p in XDJK_PLATFORM_COVERAGE" :key="p.type">
                      <TableCell class="font-mono text-xs">{{ p.type }}</TableCell>
                      <TableCell class="text-sm">{{ p.name }}</TableCell>
                      <TableCell class="text-xs">{{ TIER_LABELS[p.tier] }}</TableCell>
                      <TableCell class="text-sm text-muted-foreground">{{ p.note }}</TableCell>
                    </TableRow>
                  </TableBody>
                </Table>
              </div>
            </section>

            <Separator />

            <section class="space-y-3 text-sm text-muted-foreground">
              <h3 class="text-sm font-medium text-foreground">常见问题</h3>
              <div>
                <p class="font-medium text-foreground">成功码是字符串还是数字？</p>
                <p>两种都写上：<code class="rounded bg-muted px-1">["0", 0]</code>，程序都会认。</p>
              </div>
              <div>
                <p class="font-medium text-foreground">上游单号在嵌套 JSON 里怎么取？</p>
                <p>用 yid_path，如白泽写 <code class="rounded bg-muted px-1">data.order_id</code>；龙龙 V2 返回数组时用 <code class="rounded bg-muted px-1">"0"</code> 取第一个。</p>
              </div>
              <div>
                <p class="font-medium text-foreground">上游没有 code 字段怎么办（龙龙 V2）？</p>
                <p>
                  设 <code class="rounded bg-muted px-1">"success_http": true</code>，只要 HTTP 返回 2xx 就算成功，再用
                  yid_path 取单号。详见「平台示例」里的龙龙 longlong。
                </p>
              </div>
              <div>
                <p class="font-medium text-foreground">货源网址末尾要不要加 /？</p>
                <p>
                  都可以。<code class="rounded bg-muted px-1">{{ tplHint }}</code> 会自动去掉末尾
                  <code class="rounded bg-muted px-1">/</code>，拼路径不会出现双斜杠。
                </p>
              </div>
              <div>
                <p class="font-medium text-foreground">form 和 json 怎么选？要自己写 Content-Type 吗？</p>
                <p>
                  PHP 用 <code class="rounded bg-muted px-1">get_url($url, $data)</code> → 设
                  <code class="rounded bg-muted px-1">"content_type": "form"</code>；用
                  <code class="rounded bg-muted px-1">json_encode</code> → 设
                  <code class="rounded bg-muted px-1">"content_type": "json"</code>。hzw、27、29 都是 form，一般不用在 headers 里重复写 Content-Type。
                </p>
              </div>
              <div>
                <p class="font-medium text-foreground">改完规则什么时候生效？</p>
                <p>在「平台规则」里保存、启用/禁用或删除后，会自动生效，不用重启程序。</p>
              </div>
              <div class="flex items-center gap-2 pt-1">
                <ExternalLink class="size-4 shrink-0" />
                <RouterLink to="/platforms" class="text-primary underline-offset-4 hover:underline">前往平台规则</RouterLink>
              </div>
            </section>
          </div>
        </div>
      </ScrollArea>
    </CardContent>
  </Card>
</template>
