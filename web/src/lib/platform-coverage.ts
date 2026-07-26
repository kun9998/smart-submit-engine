/** xdjk.php 平台覆盖扫描（Checkorder/xdjk.php，addWk 共 65 个 pt） */

export type PlatformCoverageTier =
  | 'http'
  | 'body_raw'
  | 'branches'
  | 'kcid_json'
  | 'pipeline'
  | 'special'
  | 'blocked'

export interface PlatformCoverageEntry {
  type: string
  name: string
  tier: PlatformCoverageTier
  /** 引擎内可直接 rule_config 表达 */
  ready: boolean
  note: string
}

export const XDJK_PLATFORM_COVERAGE: PlatformCoverageEntry[] = [
  { type: '27', name: '27 系统', tier: 'http', ready: true, note: 'api.php?act=add + cookie' },
  { type: '2xx', name: '爱学习', tier: 'http', ready: true, note: 'POST JSON，含 uTime/uScore 等扩展字段' },
  { type: 'zfb', name: 'zfb', tier: 'http', ready: true, note: '标准 uid/key act=add，带 yid' },
  { type: 'duowei', name: '多维', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'benz', name: 'benz', tier: 'http', ready: true, note: 'token + cookie，code=0' },
  { type: 'daxiong', name: '大雄', tier: 'http', ready: true, note: '标准 act=add' },
  { type: '29', name: '29 系统', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'liufu', name: 'liufu', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'KUN', name: 'KUN', tier: 'special', ready: true, note: 'url_port_pool + {{random_port}}' },
  { type: 'ssrs', name: 'SSRS', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'bld', name: 'bld', tier: 'http', ready: true, note: 'act=addcf' },
  { type: 'algk', name: 'algk', tier: 'http', ready: true, note: 'GET 查询串 + urlencode kcname/pass' },
  { type: 'baize', name: '白泽', tier: 'body_raw', ready: true, note: 'JSON body_raw，success_codes 0000，yid_path data.order_id' },
  { type: 'algksy', name: 'algksy', tier: 'http', ready: true, note: '同 algk，不同 API 路径' },
  { type: 'lotus', name: '荷花论文', tier: 'body_raw', ready: true, note: '固定第三方域名提交；remarks 写库需主站侧处理' },
  { type: 'kunba', name: 'kunba', tier: 'special', ready: true, note: 'GET + order.ikun_study_ip 代理参数' },
  { type: 'bdkj', name: '暗网', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'xiyou', name: '西游', tier: 'http', ready: true, note: 'code_field=status，success=success' },
  { type: 'ACE', name: 'ACE', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'maodou', name: 'maodou', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'hzw', name: 'hzw', tier: 'http', ready: true, note: 'code success=1' },
  { type: 'simple', name: 'simple', tier: 'special', ready: true, note: '{{concat}} 拼接 noun + simple_* 字段' },
  { type: 'langr', name: '初见', tier: 'http', ready: true, note: '含 shichang/score' },
  { type: 'wuming', name: 'wuming', tier: 'body_raw', ready: true, note: '嵌套 courseInfo 数组 body_raw' },
  { type: 'dlam', name: '哆啦A梦', tier: 'branches', ready: true, note: 'branches 按 noun 设 operate + body_raw course 数组' },
  { type: '00', name: '00', tier: 'http', ready: true, note: 'token 在 body' },
  { type: 'Bsc', name: 'Bsc', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'xm', name: 'xm', tier: 'http', ready: true, note: '标准 act=add + yid' },
  { type: 'maliaorun', name: '龙猫', tier: 'branches', ready: true, note: 'branches 按 noun 改 learnTimes/learnDays/forceRun + body_raw' },
  { type: 'yqsl', name: '元气森林', tier: 'http', ready: true, note: 'act=addyqsl + score/shichang' },
  { type: 'df1', name: '黑森林 df1', tier: 'body_raw', ready: true, note: '嵌套 JSON 数组，test=0' },
  { type: 'df2', name: '黑森林 df2', tier: 'body_raw', ready: true, note: '同 df1，test=1，建议单独平台条目' },
  { type: 'longlong', name: 'longlong', tier: 'body_raw', ready: true, note: 'V2 /api/submit + X-Uid/X-Api-Key，success_http + body_raw courses 数组' },
  { type: 'kdbxxt', name: '课代表学习通', tier: 'kcid_json', ready: true, note: 'kcid_json + noun 补丁 checked_config_list' },
  { type: 'kdbzhs', name: '课代表智慧树', tier: 'kcid_json', ready: true, note: 'kcid_json 补丁' },
  { type: 'kdbzhzj', name: '课代表智慧职教', tier: 'kcid_json', ready: true, note: 'kcid_json 补丁' },
  { type: 'xuemei', name: '学妹', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'liunian', name: 'liunian', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'Tom', name: 'Tom', tier: 'http', ready: true, note: '标准 act=add + yid' },
  { type: 'yue29', name: 'yue29', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'huoxi', name: 'huoxi', tier: 'body_raw', ready: true, note: 'JSON 数组 body_raw，code_field=status' },
  { type: 'yyy', name: 'YYY', tier: 'http', ready: true, note: 'api/order，success_codes 200，yid_path' },
  { type: 'lyyjxjy', name: '懒洋洋', tier: 'pipeline', ready: true, note: 'branches + delay_ms/poll；随机课程走 form raw' },
  { type: 'hb', name: '黑白', tier: 'http', ready: true, note: '标准 act=add + yid' },
  { type: '2023', name: '2023', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'hh', name: '花花', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'goStudy', name: 'goStudy', tier: 'body_raw', ready: true, note: 'JSON 数组 + order.name' },
  { type: 'huangzu', name: 'huangzu', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'pup', name: 'pup', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'yumeng', name: 'yumeng', tier: 'http', ready: true, note: '标准 act=add' },
  { type: '8090', name: '8090', tier: 'branches', ready: true, note: 'Bearer JSON submit；isck=0 不传课名；yid=data' },
  { type: 'jxjyyjy', name: '易教育', tier: 'branches', ready: true, note: 'Bearer JSON api/order/buy；isck=0 空课名' },
  { type: 'ml', name: '茉莉', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'tesla', name: '特斯拉', tier: 'http', ready: true, note: 'cid 字段 + cookie，/api/api/external/submit-order' },
  { type: 'HEI', name: '嘿嘿', tier: 'http', ready: true, note: '标准 act=add' },
  { type: 'wanzi', name: '丸子', tier: 'http', ready: true, note: '标准 act=add + yid' },
  { type: 'THOTH', name: 'THOTH', tier: 'http', ready: true, note: 'Header X-Uid/X-Api-Key + form /api/open/add，success=0' },
  { type: 'coco', name: 'coco', tier: 'http', ready: true, note: 'JSON /api/useAPI/addOrder，success=1' },
  { type: 'nx', name: '奶昔', tier: 'http', ready: true, note: 'JSON submit + failure_msg_rules' },
  { type: 'miaosha', name: '秒杀', tier: 'http', ready: true, note: '标准 act=add + yid' },
  { type: 'wufu', name: '五福', tier: 'http', ready: true, note: '标准 act=add' },
]

export const COVERAGE_SUMMARY = (() => {
  const total = XDJK_PLATFORM_COVERAGE.length
  const ready = XDJK_PLATFORM_COVERAGE.filter((p) => p.ready).length
  const blocked = XDJK_PLATFORM_COVERAGE.filter((p) => p.tier === 'blocked').length
  const byTier = XDJK_PLATFORM_COVERAGE.reduce<Record<PlatformCoverageTier, number>>(
    (acc, p) => {
      acc[p.tier] = (acc[p.tier] || 0) + 1
      return acc
    },
    { http: 0, body_raw: 0, branches: 0, kcid_json: 0, pipeline: 0, special: 0, blocked: 0 },
  )
  return {
    total,
    ready,
    blocked,
    coveragePct: Math.round((ready / total) * 1000) / 10,
    byTier,
  }
})()

export const TIER_LABELS: Record<PlatformCoverageTier, string> = {
  http: '普通一次请求',
  body_raw: '自定义请求体',
  branches: '按条件分情况',
  kcid_json: '课代表 kcid',
  pipeline: '多步流程',
  special: '特殊字段/端口',
  blocked: '暂不支持',
}
