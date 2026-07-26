/** 规则里可用的模板变量（与后端字段一致） */

export interface TemplateVarRow {
  var: string
  php?: string
  desc: string
}

export const ORDER_TEMPLATE_VARS: TemplateVarRow[] = [
  { var: '{{order.oid}}', php: '$oid', desc: '订单号' },
  { var: '{{order.noun}}', php: '$noun', desc: '平台对接参数（如 ptid、platform）' },
  { var: '{{order.school}}', php: '$school', desc: '学校' },
  { var: '{{order.user}}', php: '$user', desc: '下单账号' },
  { var: '{{order.pass}}', php: '$pass', desc: '下单密码' },
  { var: '{{order.kcname}}', php: '$kcname', desc: '课程名称' },
  { var: '{{order.kcid}}', php: '$kcid', desc: '课程 ID（课代表等可能是 base64 JSON）' },
  { var: '{{order.name}}', php: '$name', desc: '学生姓名等（部分平台）' },
  { var: '{{order.uTime}}', php: '$uTime', desc: '学习时长' },
  { var: '{{order.uScore}}', php: '$uScore', desc: '学习分数' },
  { var: '{{order.study_speed}}', php: '$d["study_speed"]', desc: '学习速度（爱学习等）' },
  { var: '{{order.is_submit_exam}}', php: '$d["is_submit_exam"]', desc: '是否提交考试' },
  { var: '{{order.exam_time}}', php: '$d["exam_time"]', desc: '考试时间' },
  { var: '{{order.simple_day_score}}', php: '$d["simple_day_score"]', desc: 'simple 平台：日分数' },
  { var: '{{order.simple_total_score}}', php: '$d["simple_total_score"]', desc: 'simple 平台：总分数' },
  { var: '{{order.simple_learn_time}}', php: '$d["simple_learn_time"]', desc: 'simple 平台：学习时长' },
  { var: '{{order.ikun_study_ip}}', php: '$d["ikun_study_ip"]', desc: 'kunba：学习 IP / 代理' },
  { var: '{{order.hid}}', php: '$hid', desc: '货源编号' },
  { var: '{{order.yid}}', php: '$yid', desc: '订单已有的上游单号' },
  { var: '{{order.isck}}', php: '$isck / $d["isck"]', desc: '是否按课程名校验（8090 等）' },
  { var: '{{order.expand.city}}', php: '$d["expand"].city', desc: 'longlong 扩展字段：城市' },
  { var: '{{order.expand.tag}}', php: '$d["expand"].tag', desc: 'longlong 扩展字段：标签' },
  { var: '{{order.expand.remark}}', php: '$d["expand"].remark', desc: 'longlong 扩展字段：备注' },
  { var: '{{order.expand.config}}', php: '$d["expand"].config', desc: 'longlong 扩展字段：配置 JSON' },
  { var: '{{order.status}}', php: '$status', desc: '订单状态' },
  { var: '{{order.process}}', php: '$process', desc: '学习进度' },
  { var: '{{order.remarks}}', php: '$remarks', desc: '订单备注' },
  { var: '{{order.dockstatus}}', php: '$dockstatus', desc: '对接状态' },
]

export const HUOYUAN_TEMPLATE_VARS: TemplateVarRow[] = [
  { var: '{{huoyuan.url}}', php: '$a["url"]', desc: '货源网址（会自动去掉末尾 /）' },
  { var: '{{huoyuan.user}}', php: '$a["user"]', desc: '货源账号（uid）' },
  { var: '{{huoyuan.pass}}', php: '$a["pass"]', desc: '货源密码（key；部分平台当 token 用）' },
  { var: '{{huoyuan.token}}', php: '$a["token"] / $token', desc: '货源 Token' },
  { var: '{{huoyuan.cookie}}', php: '$cookie', desc: '货源 Cookie（一般配合 use_cookie）' },
]

export const TEMPLATE_FUNCTIONS: TemplateVarRow[] = [
  { var: '{{urlencode order.pass}}', desc: '网址编码（GET 参数、中文密码等）' },
  { var: '{{concat order.noun "|score=" order.uScore}}', desc: '把多段文字拼在一起（simple 改 platform）' },
  { var: '{{base64_decode order.kcid}}', desc: '解码 kcid（整段 body 时用；推荐 body_mode: kcid_json）' },
  { var: '{{random_port}}', desc: '从端口池随机选一个（KUN；须配置 url_port_pool）' },
  { var: '{{var.token}}', desc: '多步流程里的中间变量（如前一步登录拿到的 token）' },
  { var: '{{json_path var.login_body data.token}}', desc: '从上一段 JSON 里按路径取值' },
]

/** 兼容旧 FIELD_MAPPING 表格（PHP → JSON 对照） */
export const FIELD_MAPPING = [
  ...HUOYUAN_TEMPLATE_VARS.filter((r) => r.var !== '{{huoyuan.cookie}}').map((r) => ({
    php: r.php!,
    json: r.var,
    desc: r.desc,
  })),
  { php: '$cookie', json: 'use_cookie: true', desc: '请求时带上货源 Cookie（不是 body 字段）' },
  ...ORDER_TEMPLATE_VARS.map((r) => ({
    php: r.php || '—',
    json: r.var,
    desc: r.desc,
  })),
]
