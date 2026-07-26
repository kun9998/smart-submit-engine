import type { SubmitRuleConfig } from '@/api/submit-platform'
import { parseKnownXdjkPlatform } from './parse-xdjk-known'

export interface ParseXdjkResult {
  platform_type: string
  rule_config: SubmitRuleConfig
  warnings: string[]
  /** 无法完全用规则引擎表达，需手写 Go 或手动改 JSON */
  special_notes: string[]
}

export interface ParseXdjkError {
  error: string
  special_notes?: string[]
}

const TYPE_RE = /\$type\s*==\s*["']([^"']+)["']/i

/** xdjk.php 里无法靠通用 rule_config 表达的模式 */
const SPECIAL_BLOCKERS: { pattern: RegExp; note: string }[] = [
  { pattern: /base64_decode\s*\(\s*\$kcid\s*\)/, note: '课代表系列 (kdbxxt/kdbzhs/kdbzhzj)：请求体来自 base64 解码 kcid，且随 noun 改 JSON，需单独实现' },
  { pattern: /array_rand\s*\(/, note: 'KUN：随机端口无法写入静态规则，需固定端口或 Go 侧实现' },
  { pattern: /\$DB\s*->\s*query/, note: 'lotus 等：含写主库 remarks 的 SQL，超出提交规则范围' },
  { pattern: /\bsleep\s*\(/, note: '含 sleep 延时，规则引擎不支持' },
  { pattern: /if\s*\(\s*strpos\s*\(\s*\$kcname/, note: '懒洋洋等：按课程名分支不同 URL/Body，需拆成多条规则或 Go 实现' },
  { pattern: /\$type\s*==\s*["']df1["']\s*\|\|\s*\$type/, note: 'df1/df2：同一分支含 test 参数差异，请分别粘贴 df1 或 df2 块' },
  { pattern: /\$noun\s*==\s*1\s*\)\s*\{/, note: '龙猫 (maliaorun)：按 noun 数值分支多套 data，请粘贴具体分支' },
  { pattern: /\$noun\s*=\s*\$noun\s*\.\s*["']\|/, note: 'simple：会改写 noun 再提交，需手动处理' },
  { pattern: /if\s*\(\s*\$noun\s*==\s*['"]xgk['"]/, note: '哆啦A梦 (dlam)：按 noun 计算 operate 字段，需手动配置 body' },
  { pattern: /http:\/\/text\.boox\.top/, note: 'lotus：固定第三方域名，非货源 url' },
  { pattern: /["']courseInfo["']\s*=>\s*array/, note: '无名 (wuming) 等：含嵌套 JSON 数组 body，请解析后手动补全' },
  { pattern: /\$data\s*=\s*["']platform=/, note: '懒洋洋随机课：body 为 form 字符串而非 array，需手动编写' },
  { pattern: /ikun_study_ip/, note: 'kunba：已支持 {{order.ikun_study_ip}}，请确认货源 URL 模板' },
]

function detectSpecialBlockers(code: string): string[] {
  const notes: string[] = []
  for (const { pattern, note } of SPECIAL_BLOCKERS) {
    if (pattern.test(code)) notes.push(note)
  }
  return notes
}

function mapPhpValue(raw: string): { tpl: string; ok: boolean } {
  const v = raw.trim().replace(/\s+/g, ' ')
  const huoyuan = v.match(/^\$a\[["'](\w+)["']\]$/)
  if (huoyuan) {
    const key = huoyuan[1]
    if (['user', 'pass', 'url', 'token', 'cookie'].includes(key)) {
      return { tpl: `{{huoyuan.${key}}}`, ok: true }
    }
    return { tpl: `{{huoyuan.${key}}}`, ok: false }
  }
  const orderD = v.match(/^\$d\[["'](\w+)["']]$/)
  if (orderD) {
    return { tpl: `{{order.${orderD[1]}}}`, ok: true }
  }
  const orderVars: Record<string, string> = {
    $noun: '{{order.noun}}',
    $school: '{{order.school}}',
    $user: '{{order.user}}',
    $pass: '{{order.pass}}',
    $kcname: '{{order.kcname}}',
    $kcid: '{{order.kcid}}',
    $uTime: '{{order.uTime}}',
    $uScore: '{{order.uScore}}',
    $oid: '{{order.oid}}',
    $token: '{{huoyuan.token}}',
    $cookie: '{{huoyuan.cookie}}',
    $expand: '[]',
    $operate: '',
    $course: '[]',
  }
  if (orderVars[v] !== undefined) {
    if (v === '$expand' || v === '$operate' || v === '$course') {
      return { tpl: orderVars[v], ok: false }
    }
    return { tpl: orderVars[v], ok: true }
  }
  return { tpl: v, ok: false }
}

function parseArrayBody(inner: string): { body: Record<string, string>; warnings: string[] } {
  const body: Record<string, string> = {}
  const warnings: string[] = []
  const pairRe = /["']([^"']+)["']\s*=>\s*([^,]+?)(?=,\s*["']|\s*$)/g
  let m: RegExpExecArray | null
  while ((m = pairRe.exec(inner)) !== null) {
    const key = m[1]
    const raw = m[2].trim()
    if (/^array\s*\(/i.test(raw)) {
      warnings.push(`字段 ${key} 为嵌套 array，已跳过，请手动补全`)
      continue
    }
    const mapped = mapPhpValue(raw)
    body[key] = mapped.tpl
    if (!mapped.ok) warnings.push(`字段 ${key} 的值「${raw}」未能识别，请手动检查`)
  }
  return { body, warnings }
}

/** 取最后一次 $data / $postData = array(...) 或 [...] */
function findDataArrayInner(code: string): string {
  const patterns = [
    /\$data\s*=\s*array\s*\(([\s\S]*?)\)\s*;/gi,
    /\$data\s*=\s*\[([\s\S]*?)\]\s*;/gi,
    /\$postData\s*=\s*array\s*\(([\s\S]*?)\)\s*;/gi,
    /\$postData\s*=\s*\[([\s\S]*?)\]\s*;/gi,
  ]
  for (const re of patterns) {
    const matches = [...code.matchAll(re)]
    if (matches.length) return matches[matches.length - 1][1]
  }
  return ''
}

function phpVarToTemplate(expr: string): string {
  return expr
    .replace(/\{\$a\[['"](\w+)['"]\]\}/g, (_, k) => `{{huoyuan.${k}}}`)
    .replace(/\$a\[['"](\w+)['"]\]/g, (_, k) => `{{huoyuan.${k}}}`)
    .replace(/urlencode\s*\(\s*\$(\w+)\s*\)/g, (_, v) => {
      const m = mapPhpValue(`$${v}`)
      return m.tpl.startsWith('{{') ? `{{urlencode ${m.tpl.slice(2, -2)}}}` : m.tpl
    })
    .replace(/\$(\w+)/g, (full) => {
      const m = mapPhpValue(full)
      return m.ok ? m.tpl : full
    })
}

/** kunba / KUN：URL 拼接查询参数，无 $data */
function parseConcatGetUrl(code: string): { url: string; method: string; warnings: string[] } | null {
  const m = code.match(
    /\$(\w+)\s*=\s*\$(\w+)\s*\.\s*['"]([^'"]*\?[^'"]*)['"]\s*\.([^;]+);/i,
  )
  if (!m) return null
  let qs = m[3] + m[4]
  qs = qs.replace(/\s*\.\s*urlencode\s*\(\s*\$(\w+)\s*\)/gi, (_, v) => {
    const t = mapPhpValue(`$${v}`)
    return t.tpl.startsWith('{{') ? `{{urlencode ${t.tpl.slice(2, -2)}}}` : t.tpl
  })
  qs = qs.replace(/\s*\.\s*['"]&([^'"]*)['"]/g, '&$1')
  qs = phpVarToTemplate(qs.replace(/\s*\.\s*/g, ''))
  const warnings: string[] = []
  if (/array_rand|randomPort/i.test(code)) {
    warnings.push('含随机端口，URL 中端口需手动指定')
  }
  return {
    url: `{{huoyuan.url}}${qs.startsWith('/') ? '' : '/'}${qs.replace(/^\//, '')}`,
    method: 'GET',
    warnings,
  }
}

function joinHuoyuanURLTemplate(path: string): string {
  path = path.trim()
  if (!path) return '{{huoyuan.url}}'
  if (/^https?:\/\//i.test(path)) return path
  if (!path.startsWith('/') && !path.startsWith('?') && !path.startsWith(':')) {
    path = `/${path}`
  }
  return `{{huoyuan.url}}${path}`
}

function resolveUrl(code: string): { url: string; method: string; warnings: string[] } {
  const warnings: string[] = []
  const urlVars = new Map<string, string>()

  for (const m of code.matchAll(/\$(\w+)\s*=\s*\$a\[["']url["']\]\s*;/gi)) {
    urlVars.set(m[1], '{{huoyuan.url}}')
  }

  const directConcat = code.match(/\$(\w+)\s*=\s*\$a\[["']url["']\]\s*\.\s*["']([^"']+)["']/i)
  if (directConcat) {
    urlVars.set(directConcat[1], joinHuoyuanURLTemplate(directConcat[2]))
  }

  const httpConcat = code.match(
    /\$(\w+)\s*=\s*["']https?:\/\/["']\s*\.\s*\$a\[["']url["']\]\s*\.\s*["']([^"']+)["']/i,
  )
  if (httpConcat) {
    urlVars.set(httpConcat[1], joinHuoyuanURLTemplate(httpConcat[2]))
  }

  for (const m of code.matchAll(/\$(\w+)\s*=\s*["']([^"']+)["']\s*;/g)) {
    const varName = m[1]
    let path = m[2]
    path = path.replace(/\$(\w+)/g, (_, n) => urlVars.get(n) || '{{huoyuan.url}}')
    path = phpVarToTemplate(path)
    if (!path.includes('{{huoyuan.url}}') && path.startsWith('/')) {
      path = joinHuoyuanURLTemplate(path)
    } else if (path.includes('{{huoyuan.url}}')) {
      path = path.replace(/https?:\/\/\{\{huoyuan\.url\}\}/gi, '{{huoyuan.url}}')
    }
    urlVars.set(varName, path)
  }

  const queryUrl = [...urlVars.values()].find((u) => u.includes('?') && /username=|platform=|zhanghao=/i.test(u))
  if (queryUrl && /get_url\s*\(\s*\$(\w+)/i.test(code)) {
    const hasBody = /get_url\s*\(\s*\$(\w+)\s*,\s*\$(data|jsonData)\b/i.test(code)
    return { url: queryUrl, method: hasBody ? 'POST' : 'GET', warnings }
  }

  const concatGet = parseConcatGetUrl(code)
  if (concatGet) return concatGet

  const getUrl = code.match(/get_url\s*\(\s*\$(\w+)(?:\s*,\s*\$(\w+))?(?:\s*,\s*\$(\w+))?\s*\)/i)
  if (getUrl) {
    const resolved = urlVars.get(getUrl[1])
    if (resolved) {
      return { url: resolved, method: getUrl[2] && getUrl[2] !== 'cookie' ? 'POST' : 'GET', warnings }
    }
  }

  const httpGet = code.match(/httpRequest\s*\(\s*["']GET["']\s*,\s*\$(\w+)/i)
  if (httpGet) {
    const resolved = urlVars.get(httpGet[1])
    if (resolved) return { url: resolved, method: 'GET', warnings }
  }

  const httpPost = code.match(
    /httpRequest\s*\(\s*["'](\w+)["']\s*,\s*\$(\w+)\s*,\s*\$(\w+)\s*,\s*\[[^\]]*\]\s*,\s*(true|false)\s*\)/i,
  )
  if (httpPost) {
    const resolved = urlVars.get(httpPost[2])
    if (resolved) return { url: resolved, method: httpPost[1].toUpperCase(), warnings }
  }

  const curlURL = code.match(/curl_setopt\s*\(\s*\$(\w+)\s*,\s*CURLOPT_URL\s*,\s*\$(\w+)\s*\)/i)
  if (curlURL) {
    const resolved = urlVars.get(curlURL[2])
    if (resolved) return { url: resolved, method: 'POST', warnings }
  }

  for (const [name, url] of urlVars) {
    if (
      code.includes(`get_url($${name}`) ||
      code.includes('httpRequest(') ||
      code.includes('post(') ||
      code.includes('curl_exec') ||
      code.includes('curl_setopt')
    ) {
      return { url, method: 'POST', warnings }
    }
  }

  warnings.push('未能自动识别 URL，已使用默认 /api.php?act=add')
  return { url: '{{huoyuan.url}}/api.php?act=add', method: 'POST', warnings }
}

function parseHeaders(code: string): Record<string, string> | undefined {
  const headers: Record<string, string> = {}
  const authBearer = code.match(/["']Authorization:\s*Bearer\s*["']\s*\.\s*\$(\w+)/i)
  if (authBearer) headers.Authorization = `Bearer {{huoyuan.token}}`
  const authDf = code.match(/["']Authorization:\s*DfAi\s*\$(\w+)/i)
  if (authDf) headers.Authorization = `DfAi {{huoyuan.token}}`
  const tokenHdr = code.match(/["']Token:\s*["']\s*\.\s*\$a\[["']token["']\]/i)
  if (tokenHdr) headers.Token = '{{huoyuan.token}}'
  const tokenPlain = code.match(/["']token:\s*["']\s*\.\s*\$a\[["']token["']\]/i)
  if (tokenPlain) headers.token = '{{huoyuan.token}}'
  return Object.keys(headers).length ? headers : undefined
}

function detectTransport(code: string): {
  content_type: 'form' | 'json'
  use_cookie: boolean
} {
  let use_cookie =
    /get_url\s*\([^)]+,\s*\$(\w+)\s*,\s*\$cookie\s*\)/i.test(code) ||
    /get_url\s*\(\s*\$(\w+)\s*,\s*\$cookie\s*\)/i.test(code) ||
    /["']cookie:\s*\{\$cookie\}/i.test(code)

  const isJson =
    /\$data\s*=\s*json_encode/i.test(code) ||
    /\$data\s*=\s*\[/i.test(code) ||
    /json_encode\s*\(\s*\$data/i.test(code) ||
    /Content-Type:\s*application\/json/i.test(code) ||
    /httpRequest\s*\([^)]+,\s*true\s*\)/i.test(code) ||
    /\bpost\s*\(\s*\$(\w+)\s*,\s*\$(jsonData|data)\s*,/i.test(code) ||
    /\bpost\s*\(\s*\$(\w+)\s*,\s*\$data\s*\)/i.test(code)

  return { content_type: isJson ? 'json' : 'form', use_cookie }
}

/** 解析 PHP 失败分支 strpos($result["msg"], "x") → $msg = "..." */
function parseFailureMsgRules(code: string): NonNullable<SubmitRuleConfig['response']['failure_msg_rules']> {
  const rules: NonNullable<SubmitRuleConfig['response']['failure_msg_rules']> = []
  const seen = new Set<string>()
  const re =
    /strpos\s*\(\s*\$result\[["'](\w+)["']\]\s*,\s*["']([^"']+)["']\s*\)\s*!==\s*false[\s\S]*?\$msg\s*=\s*["']([^"']+)["']/gi
  let m: RegExpExecArray | null
  while ((m = re.exec(code)) !== null) {
    const msgField = m[1]
    if (msgField.toLowerCase() !== 'msg' && msgField.toLowerCase() !== 'message') {
      continue
    }
    const contains = m[2]
    const msg = m[3]
    const key = `${contains}\0${msg}`
    if (seen.has(key)) continue
    seen.add(key)
    rules.push({ contains, msg })
  }
  return rules
}

function parseResponse(code: string): SubmitRuleConfig['response'] {
  const resp: SubmitRuleConfig['response'] = {
    code_field: 'code',
    success_codes: ['0', 0],
    msg_field: 'msg',
    success_msg: '下单成功',
  }

  const msgSuccess = code.match(/if\s*\(\s*\$result\[["']msg["']\]\s*==\s*["']([^"']+)["']\s*\)/i)
  if (msgSuccess) {
    resp.code_field = 'msg'
    resp.success_codes = [msgSuccess[1]]
    resp.msg_field = 'msg'
  } else if (/if\s*\(\s*\$result\[["']status["']\]\s*==\s*["']success["']\s*\)/i.test(code)) {
    resp.code_field = 'status'
    resp.success_codes = ['success']
    resp.msg_field = 'msg'
  }

  const successIfQuoted = code.match(/if\s*\(\s*\$result\[["'](\w+)["']\]\s*==\s*["']([^"']+)["']\s*\)/i)
  if (successIfQuoted && !msgSuccess && resp.code_field === 'code') {
    resp.code_field = successIfQuoted[1]
    const val = successIfQuoted[2]
    const num = Number(val)
    resp.success_codes = Number.isNaN(num) ? [val] : [val, num]
  }

  const successIfNum = code.match(/if\s*\(\s*\$result\[["'](\w+)["']\]\s*==\s*(\d+)\s*\)/i)
  if (successIfNum && !msgSuccess) {
    resp.code_field = successIfNum[1]
    const n = Number(successIfNum[2])
    resp.success_codes = [n, String(n)]
  }

  if (/result\[["']message["']\]/i.test(code) && !/result\[["']msg["']\]/i.test(code)) {
    resp.msg_field = 'message'
  }

  const yidNested = code.match(/["']yid["']\s*=>\s*\$result\[["'](\w+)["']\]\[["'](\w+)["']\]/i)
  if (yidNested) resp.yid_path = `${yidNested[1]}.${yidNested[2]}`

  const yidFlat = code.match(/["']yid["']\s*=>\s*\$result\[["'](\w+)["']\]/i)
  if (yidFlat && !yidNested) resp.yid_field = yidFlat[1]

  const yidToken = code.match(/["']yid["']\s*=>\s*\$result\[["']order_token["']\]/i)
  if (yidToken) resp.yid_field = 'order_token'

  const yidData0 = code.match(/["']yid["']\s*=>\s*\$result\[["']data["']\]\[0\]/i)
  if (yidData0) resp.yid_path = 'data.0'

  const msg = code.match(/["']msg["']\s*=>\s*["']([^"']+)["']/i)
  if (msg && !msgSuccess) resp.success_msg = msg[1]

  const failureRules = parseFailureMsgRules(code)
  if (failureRules.length) resp.failure_msg_rules = failureRules

  return resp
}

function parseLongLongV2(code: string, platform_type: string): ParseXdjkResult | null {
  if (!/llv2_submit\s*\(/i.test(code) && !/LongLongV2\.php/i.test(code)) {
    return null
  }
  return {
    platform_type,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/submit/{{order.noun}}',
      content_type: 'json',
      headers: {
        'X-Uid': '{{huoyuan.user}}',
        'X-Api-Key': '{{huoyuan.pass}}',
        Accept: 'application/json',
      },
      body_mode: 'raw',
      body_raw:
        '{"username":"{{order.user}}","password":"{{order.pass}}","courses":["{{order.kcid}}"],"school":"{{order.school}}","name":"{{order.name}}","city":"{{order.expand.city}}","tag":"{{order.expand.tag}}","remark":"{{order.expand.remark}}","config":"{{order.expand.config}}"}',
      body: {},
      response: {
        success_http: true,
        yid_path: '0',
        success_msg: '下单成功',
      },
      process: {
        handler: 'http',
        method: 'GET',
        url: '{{huoyuan.url}}/api/order/uuid/{{order.yid}}',
        headers: {
          'X-Uid': '{{huoyuan.user}}',
          'X-Api-Key': '{{huoyuan.pass}}',
          Accept: 'application/json',
        },
        map: {
          fields: {
            yid: 'uuid',
            kcname: 'courseName',
            status_text: 'status',
            process: 'finish',
            remarks: 'result',
            kcks: 'courseStartTime',
            kcjs: 'courseEndTime',
            ksks: 'examStartTime',
            ksjs: 'examEndTime',
          },
        },
      },
    },
    warnings: ['expand 字段取 order.expand JSON；school=自动识别 时上游通常忽略空 school'],
    special_notes: [],
  }
}

export function parseXdjkPhp(code: string): ParseXdjkResult | ParseXdjkError {
  const trimmed = code.trim()
  if (!trimmed) return { error: '请粘贴 PHP 代码' }

  const special_notes = detectSpecialBlockers(trimmed)

  const typeMatch = trimmed.match(TYPE_RE)
  if (!typeMatch) {
    return { error: '未找到平台类型，请包含 if ($type == "xxx") 片段', special_notes }
  }

  const platform_type = typeMatch[1]

  const longlong = parseLongLongV2(trimmed, platform_type)
  if (longlong) return longlong

  const known = parseKnownXdjkPlatform(platform_type, trimmed, special_notes)
  if (known) return known

  const hardBlock = special_notes.find((n) =>
    /课代表|随机端口|写主库|sleep|按课程名分支|龙猫.*分支|simple：|哆啦A梦|固定第三方|form 字符串/i.test(n),
  )
  if (hardBlock) {
    return {
      error: '该段 PHP 含特殊逻辑，无法完整自动转换',
      special_notes,
    }
  }

  const warnings: string[] = []

  const dataInner = findDataArrayInner(trimmed)
  let body: Record<string, string> = {}

  if (dataInner) {
    const parsed = parseArrayBody(dataInner)
    body = parsed.body
    warnings.push(...parsed.warnings)
  } else {
    const hasUrlOnlyGet =
      /get_url\s*\(\s*\$(\w+)\s*(?:,\s*\$cookie)?\s*\)/i.test(trimmed) ||
      parseConcatGetUrl(trimmed) !== null
    if (!hasUrlOnlyGet) {
      return {
        error: '未找到 $data = array(...) 或 $data = [...]，且不是 GET 拼接 URL 写法',
        special_notes,
      }
    }
    warnings.push('无 $data 请求体（GET 参数在 URL 中）')
  }

  const { url, method, warnings: urlWarnings } = resolveUrl(trimmed)
  warnings.push(...urlWarnings)

  const transport = detectTransport(trimmed)
  const response = parseResponse(trimmed)
  const headers = parseHeaders(trimmed)

  const rule_config: SubmitRuleConfig = {
    method: method || 'POST',
    url,
    content_type: transport.content_type,
    use_cookie: transport.use_cookie,
    body,
    response,
  }
  if (headers) rule_config.headers = headers

  if (Object.keys(body).length === 0 && method === 'GET') {
    rule_config.content_type = 'form'
  }

  return { platform_type, rule_config, warnings, special_notes }
}
