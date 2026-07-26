import type { SubmitRuleBranch, SubmitRuleConfig } from '@/api/submit-platform'

export interface KnownXdjkParseResult {
  platform_type: string
  rule_config: SubmitRuleConfig
  warnings: string[]
  special_notes: string[]
}

export function parseKnownXdjkPlatform(
  platformType: string,
  code: string,
  specialNotes: string[],
): KnownXdjkParseResult | null {
  switch (platformType.toLowerCase()) {
    case 'baize':
      return parseBaize(platformType, specialNotes)
    case 'xiyou':
      return parseXiyou(platformType, specialNotes)
    case 'yyy':
      return parseYyy(platformType, specialNotes)
    case '8090':
      return parse8090(platformType, code, specialNotes)
    case 'kdbxxt':
      return parseKdbxxt(platformType, specialNotes)
    case 'kdbzhs':
      return parseKdbzhs(platformType, specialNotes)
    case 'kdbzhzj':
      return parseKdbzhzj(platformType, specialNotes)
    case 'maliaorun':
      return parseMaliaorun(platformType, specialNotes)
    case 'lyyjxjy':
      return parseLyyjxjy(platformType, specialNotes)
    case 'df1':
      return parseDf1(platformType, specialNotes)
    case 'df2':
      return parseDf2(platformType, specialNotes)
    case 'simple':
      return parseSimple(platformType, specialNotes)
    case 'wuming':
      return parseWuming(platformType, specialNotes)
    case 'dlam':
      return parseDlam(platformType, specialNotes)
    case 'kun':
      return parseKUN(platformType, specialNotes)
    case 'kunba':
      return parseKunba(platformType, specialNotes)
    case 'lotus':
      return parseLotus(platformType, code, specialNotes)
    case 'huoxi':
      return parseHuoxi(platformType, specialNotes)
    case 'longlong':
      return parseLonglong(platformType, specialNotes)
    case 'gostudy':
      return parseGoStudy(platformType, specialNotes)
    case 'jxjyyjy':
      return parseJxjyyjy(platformType, specialNotes)
    case 'langr':
      return parseLangr(platformType, specialNotes)
    case 'yqsl':
      return parseYqsl(platformType, specialNotes)
    case 'algk':
      return parseAlgk(platformType, specialNotes)
    case 'algksy':
      return parseAlgksy(platformType, specialNotes)
    case 'tesla':
      return parseTesla(platformType, specialNotes)
    case 'thoth':
      return parseTHOTH(platformType, specialNotes)
    case 'coco':
      return parseCoco(platformType, specialNotes)
    case 'nx':
      return parseNx(platformType, specialNotes)
    case '00':
      return parse00(platformType, specialNotes)
    case 'yumeng':
      return parseYumeng(platformType, specialNotes)
    case '27':
      return parse27(platformType, specialNotes)
    case '2xx':
      return parse2xx(platformType, specialNotes)
    case 'benz':
      return parseBenz(platformType, specialNotes)
    case 'bld':
      return parseBld(platformType, specialNotes)
    case 'hzw':
      return parseHzw(platformType, specialNotes)
    case 'zfb':
      return parseZfb(platformType, specialNotes)
    case 'duowei':
      return parseDuowei(platformType, specialNotes)
    case 'wanzi':
      return parseWanzi(platformType, specialNotes)
    case 'xm':
      return parseXm(platformType, specialNotes)
    case 'hb':
      return parseHb(platformType, specialNotes)
    default:
      return parseStandardActAdd(platformType, specialNotes)
  }
}

function kcidBranch(noun: string, list: string[]): SubmitRuleBranch {
  return {
    when: { field: 'order.noun', equals: noun },
    kcid_json_patches: [{ set: { 'task_list.0.config.checked_config_list': list } }],
  }
}

function kdbBase(url: string, branches: SubmitRuleBranch[]): SubmitRuleConfig {
  return {
    method: 'POST',
    url,
    content_type: 'json',
    body_mode: 'kcid_json',
    headers: {
      Authorization: 'Bearer {{huoyuan.token}}',
      'Content-Type': 'application/json',
    },
    body: {},
    kcid_json_validate: { path: 'task_list', exact: 1 },
    branches,
    response: { code_field: 'code', success_codes: ['1', 1], msg_field: 'msg' },
  }
}

function parseBaize(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/v2/docking/add',
      content_type: 'json',
      body: {
        token: '{{huoyuan.token}}',
        platform_id: '{{order.noun}}',
        school: '{{order.school}}',
        account: '{{order.user}}',
        pwd: '{{order.pass}}',
        course_id: '{{order.kcid}}',
        course_name: '{{order.kcname}}',
        duration: '{{order.uTime}}',
        fraction: '{{order.uScore}}',
      },
      response: {
        code_field: 'code',
        success_codes: ['0000'],
        msg_field: 'msg',
        yid_path: 'data.order_id',
        success_msg: '下单成功',
      },
    },
    warnings: ['curl POST JSON，已按标准字段映射'],
    special_notes: specialNotes,
  }
}

function parseXiyou(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/order/xiadanForPublic',
      content_type: 'form',
      body: {
        username: '{{huoyuan.user}}',
        token: '{{huoyuan.token}}',
        classId: '{{order.noun}}',
        schoolName: '{{order.school}}',
        user: '{{order.user}}',
        pass: '{{order.pass}}',
        courseName: '{{order.kcname}}',
        courseId: '{{order.kcid}}',
      },
      response: {
        code_field: 'status',
        success_codes: ['success'],
        msg_field: 'msg',
        success_msg: '下单成功',
      },
    },
    warnings: [],
    special_notes: specialNotes,
  }
}

function parseYyy(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/order',
      content_type: 'form',
      body: {
        uid: '{{huoyuan.user}}',
        key: '{{huoyuan.pass}}',
        platform: '{{order.noun}}',
        school: '{{order.school}}',
        user: '{{order.user}}',
        pass: '{{order.pass}}',
        kcname: '{{order.kcname}}',
        kcid: '{{order.kcid}}',
      },
      response: {
        code_field: 'code',
        success_codes: [200, '200'],
        msg_field: 'msg',
        yid_path: 'data.yid',
        success_msg: '下单成功',
      },
    },
    warnings: [],
    special_notes: specialNotes,
  }
}

function parse8090(platformType: string, code: string, specialNotes: string[]): KnownXdjkParseResult {
  const warnings = ['isck=0 时不传课名（selectedCourseKeys 为空数组）']
  if (/\$DB\s*->\s*query/i.test(code)) {
    warnings.push('含 UPDATE 数据库逻辑，规则引擎不会写库')
  }
  const bodyBase = '{"websiteId":"{{order.noun}}","accountInfo":"{{order.user}} {{order.pass}}"'
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/order/submit',
      content_type: 'json',
      headers: {
        'Content-Type': 'application/json; charset=utf-8',
        Authorization: 'Bearer {{huoyuan.token}}',
      },
      body_mode: 'raw',
      body: {},
      branches: [
        {
          when: { field: 'order.isck', equals: '0' },
          body_mode: 'raw',
          body_raw: `${bodyBase},"selectedCourseKeys":[]}`,
        },
        {
          when: { default: true },
          body_mode: 'raw',
          body_raw: `${bodyBase},"selectedCourseKeys":["{{order.kcname}}"]}`,
        },
      ],
      response: {
        code_field: 'code',
        success_codes: [200, '200'],
        msg_field: 'message',
        yid_field: 'data',
        success_use_upstream_msg: true,
        failure_msg_on_success: true,
        failure_msg_rules: [{ contains: '失败' }],
      },
    },
    warnings,
    special_notes: specialNotes,
  }
}

function parseKdbxxt(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: kdbBase('{{huoyuan.url}}/api/xxt/xxt_task/', [
      kcidBranch('秒刷不清', ['视频', '作业', '考试', '章测', '直播', '阅读', '秒刷不清']),
      kcidBranch('单考试', ['考试']),
      kcidBranch('单任务点', ['视频', '章测', '直播', '阅读']),
      kcidBranch('章测收录', ['收录']),
      kcidBranch('考试收录', ['考试收录']),
      kcidBranch('秒刷', ['视频', '作业', '考试', '章测', '直播', '阅读', '秒刷']),
      { when: { default: true }, kcid_json_patches: [] },
    ]),
    warnings: ['kcid_json：按 order.noun 补丁 checked_config_list'],
    special_notes: specialNotes,
  }
}

function parseKdbzhs(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: kdbBase('{{huoyuan.url}}/api/zd/zd_task/', [
      kcidBranch('秒刷', ['视频', '作业', '考试', '习惯', '见面', '互动', '秒刷']),
      kcidBranch('单补互动', ['互动']),
      kcidBranch('单考试', ['考试']),
      { when: { default: true }, kcid_json_patches: [] },
    ]),
    warnings: ['kcid_json：智慧树课代表'],
    special_notes: specialNotes,
  }
}

function parseKdbzhzj(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: kdbBase('{{huoyuan.url}}/api/zj/zj_task/', [
      kcidBranch('单课件', ['视频', '文档', '讨论']),
      kcidBranch('慢刷', ['视频', '文档', '作业', '考试', '讨论', '慢刷']),
      kcidBranch('单做题', ['作业', '考试']),
      kcidBranch('收录', ['收录']),
      kcidBranch('补时长', ['视频', '补时长']),
      kcidBranch('复习模式', ['视频', '文档', '作业', '考试', '讨论', '慢刷', '补时长']),
      { when: { default: true }, kcid_json_patches: [] },
    ]),
    warnings: ['kcid_json：智慧职教课代表'],
    special_notes: specialNotes,
  }
}

function parseMaliaorun(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/gk/order',
      content_type: 'json',
      headers: { Authorization: 'Bearer {{huoyuan.token}}', 'Content-Type': 'application/json' },
      body_mode: 'raw',
      body: {},
      branches: [
        {
          when: { field: 'order.noun', equals: '1' },
          body_raw:
            '{"xh":"{{order.user}}","password":"{{order.pass}}","courses":["{{order.kcname}}"],"bz":"","config":{"openLog":true,"openExam":true,"openHomeWork":true,"openRead":true,"forceRun":false},"learnTimes":"500","learnDays":"5"}',
        },
        {
          when: { field: 'order.noun', equals: '2' },
          body_raw:
            '{"xh":"{{order.user}}","password":"{{order.pass}}","courses":["{{order.kcname}}"],"bz":"","config":{"openLog":true,"openExam":true,"openHomeWork":true,"openRead":true,"forceRun":true},"learnTimes":"500","learnDays":"5"}',
        },
        {
          when: { default: true },
          body_raw:
            '{"xh":"{{order.user}}","password":"{{order.pass}}","courses":["{{order.kcname}}"],"bz":"","config":{"openLog":true,"openExam":true,"openHomeWork":true,"openRead":true,"forceRun":false},"learnTimes":"800","learnDays":"15"}',
        },
      ],
      response: { code_field: 'msg', success_codes: ['所有课程下单成功'], msg_field: 'msg' },
    },
    warnings: ['按 order.noun=1/2/其他 三套 body_raw'],
    special_notes: specialNotes,
  }
}

function parseLyyjxjy(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      content_type: 'json',
      use_cookie: true,
      delay_ms: 5000,
      body: {},
      branches: [
        {
          when: { field: 'order.kcname', not_contains: '随机课程' },
          url: '{{huoyuan.url}}/system/sysOrder/addOrder',
          body_mode: 'raw',
          body_raw:
            '[{"account":"{{order.user}}","password":"{{order.pass}}","courseId":"{{order.kcid}}","courseName":"{{order.kcname}}","platformId":"{{order.noun}}"}]',
        },
        {
          when: {
            all: [
              { field: 'order.kcname', contains: '随机课程' },
              { field: 'order.school', not_contains: '自动识别' },
            ],
          },
          url: '{{huoyuan.url}}/lib/add_u.php',
          content_type: 'form',
          body_mode: 'raw',
          body_raw: 'platform={{order.noun}}&c_data={{order.school}}----{{order.user}}----{{order.pass}}%0A&',
        },
        {
          when: { default: true },
          url: '{{huoyuan.url}}/lib/add_u.php',
          content_type: 'form',
          body_mode: 'raw',
          body_raw: 'platform={{order.noun}}&c_data={{order.user}}----{{order.pass}}%0A&',
        },
      ],
      response: { code_field: 'code', success_codes: [0, '0'], msg_field: 'msg' },
    },
    warnings: ['delay_ms=5000 对应 PHP sleep(5)'],
    special_notes: specialNotes,
  }
}

function dfBodyRaw(test: number): string {
  return `[{"num":"{{order.user}}","pwd":"{{order.pass}}","name":"","mark":"平台名称","mode":"1","test":${test},"list":[{"code":"{{order.kcid}}","name":"{{order.kcname}}"}]}]`
}

function parseDf1(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/df/api/xd',
      content_type: 'json',
      headers: {
        Authorization: 'DfAi {{huoyuan.token}}',
        'Content-Type': 'application/json;charset=UTF-8',
      },
      body_mode: 'raw',
      body_raw: dfBodyRaw(0),
      body: {},
      response: { code_field: 'code', success_codes: [200, '200'], msg_field: 'msg', success_msg: '下单成功' },
    },
    warnings: ['嵌套 JSON 数组 body_raw；df1 test=0，df2 test=1，需分两条平台配置'],
    special_notes: specialNotes,
  }
}

function parseDf2(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/df/api/xd',
      content_type: 'json',
      headers: {
        Authorization: 'DfAi {{huoyuan.token}}',
        'Content-Type': 'application/json;charset=UTF-8',
      },
      body_mode: 'raw',
      body_raw: dfBodyRaw(1),
      body: {},
      response: { code_field: 'code', success_codes: [200, '200'], msg_field: 'msg', success_msg: '下单成功' },
    },
    warnings: ['嵌套 JSON 数组 body_raw；df1 test=0，df2 test=1，需分两条平台配置'],
    special_notes: specialNotes,
  }
}

function parseSimple(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  const basePlatform =
    '{{concat order.noun "|score=" order.uScore ";sc=" order.uTime "|day_score=" order.simple_day_score ";total_score=" order.simple_total_score ";learntime=" order.simple_learn_time}}'
  const simpleBranchBody = (platform: string, school: string) => ({
    token: '{{huoyuan.token}}',
    platform,
    school,
    user: '{{order.user}}',
    pass: '{{order.pass}}',
    course: '{{order.kcname}}',
    courseid: '{{order.kcid}}',
  })
  const examPipe = '{{concat order.noun "|exam_code=" split_part order.school 1 "|" 2}}'
  const examSemi = '{{concat order.noun ";exam_code=" split_part order.school 1 "|" 2}}'
  const scorePipe =
    '{{concat order.noun "|day_score=" split_part order.school 1 "|" 3 ";total_score=" split_part order.school 2 "|" 3}}'
  const scoreSemi =
    '{{concat order.noun ";day_score=" split_part order.school 1 "|" 3 ";total_score=" split_part order.school 2 "|" 3}}'
  const school2 = '{{split_part order.school 0 "|" 2}}'
  const school3 = '{{split_part order.school 0 "|" 3}}'

  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/Api/Create',
      content_type: 'form',
      body: {},
      branches: [
        {
          when: { all: [{ field: 'order.noun', contains: '437' }, { field: 'order.school', contains: '|' }, { field: 'order.noun', contains: '|' }] },
          body: simpleBranchBody(examSemi, school2),
        },
        {
          when: { all: [{ field: 'order.noun', contains: '437' }, { field: 'order.school', contains: '|' }] },
          body: simpleBranchBody(examPipe, school2),
        },
        {
          when: { all: [{ field: 'order.noun', contains: '392' }, { field: 'order.school', contains: '|' }, { field: 'order.noun', contains: '|' }] },
          body: simpleBranchBody(examSemi, school2),
        },
        {
          when: { all: [{ field: 'order.noun', contains: '392' }, { field: 'order.school', contains: '|' }] },
          body: simpleBranchBody(examPipe, school2),
        },
        {
          when: { all: [{ field: 'order.noun', contains: '385' }, { field: 'order.school', contains: '|' }, { field: 'order.noun', contains: '|' }] },
          body: simpleBranchBody(scoreSemi, school3),
        },
        {
          when: { all: [{ field: 'order.noun', contains: '385' }, { field: 'order.school', contains: '|' }] },
          body: simpleBranchBody(scorePipe, school3),
        },
        { when: { default: true }, body: simpleBranchBody(basePlatform, '{{order.school}}') },
      ],
      response: { code_field: 'code', success_codes: ['1', 1], msg_field: 'msg', success_msg: '添加成功' },
    },
    warnings: [
      'school|考试码(437/392)、school|日上限|累计上限(385) 已用 branches + split_part',
      '385 需 school 含两个 | 分段且日/累计上限非空',
    ],
    special_notes: specialNotes,
  }
}

function parseWuming(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/demo/submitOrder',
      content_type: 'json',
      headers: {
        'Content-Type': 'application/json',
        'X-Requested-With': 'XMLHttpRequest',
        token: '{{huoyuan.token}}',
      },
      body_mode: 'raw',
      body_raw:
        '{"platformId":"{{order.noun}}","school":"{{order.school}}","account":"{{order.user}}","password":"{{order.pass}}","duration":"无","score":"无","courseInfo":[{"courseName":"{{order.kcname}}","courseId":"{{order.kcid}}","unitList":[]}]}',
      body: {},
      response: {
        code_field: 'code',
        success_codes: ['1', 1],
        msg_field: 'msg',
        yid_path: 'data.id',
        success_msg: '下单成功',
      },
    },
    warnings: [],
    special_notes: specialNotes,
  }
}

function dlamBodyRaw(operate: string): string {
  return `{"shopcode":"{{order.noun}}","school":"{{order.school}}","username":"{{order.user}}","password":"{{order.pass}}","operate":"${operate}","course":[{"title":"{{order.kcname}}","id":"{{order.kcid}}"}]}`
}

function parseDlam(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  const op1 = '课件+讨论+作业+文件题+终考+保留答题机会'
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/prod-api/wk/order/xiadan',
      content_type: 'json',
      headers: { 'Content-Type': 'application/json;charset=UTF-8', Authorization: '{{huoyuan.token}}' },
      body_mode: 'raw',
      body: {},
      branches: [
        { when: { field: 'order.noun', equals: 'xgk' }, body_mode: 'raw', body_raw: dlamBodyRaw(op1) },
        { when: { field: 'order.noun', equals: 'xgkplus' }, body_mode: 'raw', body_raw: dlamBodyRaw(op1) },
        { when: { field: 'order.noun', equals: 'yth' }, body_mode: 'raw', body_raw: dlamBodyRaw('视频+测验+作业+考试') },
        {
          when: { field: 'order.noun', equals: 'qsxt' },
          body_mode: 'raw',
          body_raw: dlamBodyRaw('视频+作业+考试+讨论+登录次数+提交简答题'),
        },
      ],
      response: {
        code_field: 'code',
        success_codes: [200, '200'],
        msg_field: 'msg',
        success_use_upstream_msg: true,
      },
    },
    warnings: ['仅支持 xgk/xgkplus/yth/qsxt 四种 noun；其他 noun 需补 branches'],
    special_notes: specialNotes,
  }
}

function parseKUN(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'GET',
      url: '{{huoyuan.url}}:{{random_port}}/getorder/?platform={{urlencode order.noun}}&school={{urlencode order.school}}&account={{order.user}}&password={{order.pass}}&course={{urlencode order.kcname}}&kcid={{order.kcid}}',
      content_type: 'form',
      url_port_pool: [1234, 1235, 1236, 1237, 1238],
      body: {},
      response: {
        code_field: 'code',
        success_codes: ['1', 1],
        msg_field: 'msg',
        yid_field: 'order_token',
        success_msg: '下单成功',
      },
    },
    warnings: ['url_port_pool 替代 PHP array_rand 随机端口'],
    special_notes: specialNotes,
  }
}

function parseKunba(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'GET',
      url: '{{huoyuan.url}}/getorder4/?platform={{urlencode order.noun}}&school={{urlencode order.school}}&account={{order.user}}&password={{order.pass}}&course={{urlencode order.kcname}}&kcid={{order.kcid}}&proxy={{order.ikun_study_ip}}&dtoken={{huoyuan.token}}',
      content_type: 'form',
      body: {},
      response: {
        code_field: 'code',
        success_codes: [1, '1'],
        msg_field: 'msg',
        yid_field: 'order_token',
        success_msg: '已添加至服务器，开始执行刷课！',
      },
    },
    warnings: [],
    special_notes: specialNotes,
  }
}

function parseLotus(platformType: string, code: string, specialNotes: string[]): KnownXdjkParseResult {
  const warnings = [
    '固定第三方域名 text.boox.top，非货源 url',
    '下单成功后 PHP 会 UPDATE 主库 remarks，规则引擎不会写库，请主站侧处理跳转链接',
  ]
  if (/\$DB\s*->\s*query/i.test(code)) {
    warnings.push('已识别写库逻辑，仅转换提交 HTTP 部分')
  }
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: 'http://text.boox.top/api/aipaper_outside_main/ThirdParty/order-create?us={{huoyuan.user}}&pw={{huoyuan.pass}}',
      content_type: 'json',
      headers: {
        Accept: 'application/json, text/plain, */*',
        'Content-Type': 'application/json;charset=utf-8',
        'third-party-identity': '{{huoyuan.token}}',
      },
      body_mode: 'raw',
      body_raw: '{"Prcies":"{{order.noun}}","ThirdPartyId":"{{order.oid}}"}',
      body: {},
      response: { code_field: 'code', success_codes: [0, '0'], msg_field: 'message', success_msg: '下单成功' },
    },
    warnings,
    special_notes: specialNotes,
  }
}

function parseHuoxi(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/addOrder',
      content_type: 'json',
      headers: {
        'Content-Type': 'application/json;charset=utf-8',
        Token: '{{huoyuan.token}}',
      },
      body_mode: 'raw',
      body_raw:
        '[{"account":"{{order.user}}","password":"{{order.pass}}","goodId":"{{order.noun}}","courseName":"{{order.kcname}}","tag":1}]',
      body: {},
      response: { code_field: 'status', success_codes: ['200', 200], msg_field: 'message', success_msg: '下单成功' },
    },
    warnings: [],
    special_notes: specialNotes,
  }
}

function parseLonglong(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
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
      response: { success_http: true, yid_path: '0', success_msg: '下单成功' },
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
    special_notes: specialNotes,
  }
}

function actAddExtra(
  platformType: string,
  actPath: string,
  specialNotes: string[],
  extra: Record<string, string>,
): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: `{{huoyuan.url}}${actPath}`,
      content_type: 'form',
      body: {
        uid: '{{huoyuan.user}}',
        key: '{{huoyuan.pass}}',
        platform: '{{order.noun}}',
        school: '{{order.school}}',
        user: '{{order.user}}',
        pass: '{{order.pass}}',
        kcname: '{{order.kcname}}',
        kcid: '{{order.kcid}}',
        ...extra,
      },
      response: { code_field: 'code', success_codes: ['0', 0], msg_field: 'msg', success_msg: '下单成功' },
    },
    warnings: [],
    special_notes: specialNotes,
  }
}

function parseGoStudy(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/open/submitCourse',
      content_type: 'json',
      headers: { token: '{{huoyuan.token}}', 'Content-Type': 'application/json;charset=UTF-8' },
      body_mode: 'raw',
      body_raw:
        '[{"platformId":"{{order.noun}}","studentName":"{{order.name}}","school":"{{order.school}}","account":"{{order.user}}","password":"{{order.pass}}","code":"{{order.kcid}}","name":"{{order.kcname}}"}]',
      body: {},
      response: {
        code_field: 'code',
        success_codes: [0, '0'],
        msg_field: 'msg',
        yid_path: 'data.0',
        success_msg: '已添加至服务器，开始执行刷课！',
      },
    },
    warnings: ['studentName 取 order.name 字段'],
    special_notes: specialNotes,
  }
}

function parseJxjyyjy(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  const bodyBase = `{"websiteNumber":"{{order.noun}}","data":[{"username":"{{order.user}}","password":"{{order.pass}}","children":`
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/order/buy',
      content_type: 'json',
      headers: {
        'Content-Type': 'application/json; charset=utf-8',
        Authorization: 'Bearer {{huoyuan.token}}',
      },
      body_mode: 'raw',
      body: {},
      branches: [
        {
          when: { field: 'order.isck', equals: '0' },
          body_mode: 'raw',
          body_raw: `${bodyBase}[{"name":""}]}]}`,
        },
        {
          when: { default: true },
          body_mode: 'raw',
          body_raw: `${bodyBase}[{"name":"{{order.kcname}}"}]}]}`,
        },
      ],
      response: {
        code_field: 'code',
        success_codes: [200, '200'],
        msg_field: 'message',
        yid_path: 'data.orderList.0.orderId',
        success_msg: '下单成功',
      },
    },
    warnings: ['isck=0 时 children.name 为空字符串'],
    special_notes: specialNotes,
  }
}

function parseLangr(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return actAddExtra(platformType, '/api1.php?act=add', specialNotes, {
    shichang: '{{order.uTime}}',
    score: '{{order.uScore}}',
  })
}

function parseYqsl(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return actAddExtra(platformType, '/api.php?act=addyqsl', specialNotes, {
    score: '{{order.uScore}}',
    shichang: '{{order.uTime}}',
  })
}

function parseAlgk(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'GET',
      url: '{{huoyuan.url}}/api/Open/AddRenWu?username={{huoyuan.user}}&password={{huoyuan.pass}}&zhanghao={{order.user}}&psd={{urlencode order.pass}}&kcname={{urlencode order.kcname}}',
      content_type: 'form',
      use_cookie: true,
      body: {},
      response: { code_field: 'code', success_codes: ['200', 200], msg_field: 'msg', success_msg: '下单成功' },
    },
    warnings: ['GET 查询串，kcname/pass 需 urlencode'],
    special_notes: specialNotes,
  }
}

function parseAlgksy(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'GET',
      url: '{{huoyuan.url}}/api/ShiYanOpen/AddRenWu?username={{huoyuan.user}}&password={{huoyuan.pass}}&zhanghao={{order.user}}&psd={{urlencode order.pass}}&kcname={{urlencode order.kcname}}',
      content_type: 'form',
      body: {},
      response: { code_field: 'code', success_codes: ['200', 200], msg_field: 'msg', success_msg: '下单成功' },
    },
    warnings: [],
    special_notes: specialNotes,
  }
}

function parseTesla(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/api/external/submit-order',
      content_type: 'form',
      use_cookie: true,
      body: {
        uid: '{{huoyuan.user}}',
        key: '{{huoyuan.pass}}',
        cid: '{{order.noun}}',
        school: '{{order.school}}',
        user: '{{order.user}}',
        pass: '{{order.pass}}',
        kcname: '{{order.kcname}}',
        kcid: '{{order.kcid}}',
      },
      response: { code_field: 'code', success_codes: ['0', 0], msg_field: 'msg', yid_field: 'id', success_msg: '下单成功' },
    },
    warnings: ['platform 字段在 PHP 中为 cid'],
    special_notes: specialNotes,
  }
}

function parseTHOTH(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/open/add',
      content_type: 'form',
      headers: {
        'X-Uid': '{{huoyuan.user}}',
        'X-Api-Key': '{{huoyuan.pass}}',
      },
      body: {
        platform: '{{order.noun}}',
        school: '{{order.school}}',
        user: '{{order.user}}',
        pass: '{{order.pass}}',
        name: '{{order.name}}',
        kcname: '["{{order.kcname}}"]',
        kcid: '["{{order.kcid}}"]',
        score: '{{order.uScore}}',
        shichang: '{{order.uTime}}',
      },
      response: { code_field: 'code', success_codes: ['0', 0], msg_field: 'msg', yid_field: 'id', success_msg: '下单成功' },
    },
    warnings: ['鉴权在 Header X-Uid/X-Api-Key，勿在 body 传 uid/key；kcname/kcid 为 JSON 字符串数组'],
    special_notes: specialNotes,
  }
}

function parseCoco(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/useAPI/addOrder',
      content_type: 'json',
      body: {
        uid: '{{huoyuan.user}}',
        key: '{{huoyuan.pass}}',
        platform: '{{order.noun}}',
        school: '{{order.school}}',
        user: '{{order.user}}',
        pass: '{{order.pass}}',
        kcname: '{{order.kcname}}',
        kcid: '{{order.kcid}}',
      },
      response: { code_field: 'code', success_codes: [1, '1'], msg_field: 'msg', success_msg: '下单成功' },
    },
    warnings: [],
    special_notes: specialNotes,
  }
}

function parseNx(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/v1/order/submit',
      content_type: 'form',
      body: {
        token: '{{huoyuan.token}}',
        school: '{{order.school}}',
        account: '{{order.user}}',
        password: '{{order.pass}}',
        coursename: '{{order.kcname}}',
        value: '{{order.noun}}',
      },
      response: {
        code_field: 'code',
        success_codes: [1, '1'],
        msg_field: 'msg',
        success_msg: '下单成功',
        failure_msg_rules: [
          { contains: '重复提交', msg: '已获取最新订单,等待进度同步' },
          { contains: 'Repeated', msg: '已获取最新订单,等待进度同步' },
          { contains: '积分不足', msg: '学时不足,请联系上级！' },
          { contains: 'Insufficient', msg: '学时不足,请联系上级！' },
          { contains: '不能为空', msg: '参数提交不完整' },
          { contains: 'cannot be empty', msg: '参数提交不完整' },
        ],
      },
    },
    warnings: ['failure_msg_rules 映射 PHP strpos 改写文案'],
    special_notes: specialNotes,
  }
}

function parse00(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'GET',
      url: '{{huoyuan.url}}/submit',
      content_type: 'form',
      body: {
        school: '{{order.school}}',
        account: '{{order.user}}',
        password: '{{order.pass}}',
        coursename: '{{order.kcname}}',
        value: '{{order.noun}}',
        token: '{{huoyuan.token}}',
      },
      response: { code_field: 'code', success_codes: [0, '0'], msg_field: 'message', success_msg: '添加成功' },
    },
    warnings: [],
    special_notes: specialNotes,
  }
}

function parseYumeng(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/addOrder',
      content_type: 'form',
      body: {
        token: '{{huoyuan.token}}',
        platform: '{{order.noun}}',
        school: '{{order.school}}',
        user: '{{order.user}}',
        pass: '{{order.pass}}',
        kcname: '{{order.kcname}}',
        kcid: '{{order.kcid}}',
      },
      response: { code_field: 'code', success_codes: ['1', 1], msg_field: 'msg', yid_field: 'id', success_msg: '下单成功' },
    },
    warnings: [],
    special_notes: specialNotes,
  }
}

function actAddWithSuccess(
  platformType: string,
  actPath: string,
  specialNotes: string[],
  extra: Record<string, string>,
  opts: {
    use_cookie?: boolean
    omit_kcid?: boolean
    success_codes: (string | number)[]
    yid_field?: string
  },
): KnownXdjkParseResult {
  const body: Record<string, string> = {
    uid: '{{huoyuan.user}}',
    key: '{{huoyuan.pass}}',
    platform: '{{order.noun}}',
    school: '{{order.school}}',
    user: '{{order.user}}',
    pass: '{{order.pass}}',
    kcname: '{{order.kcname}}',
    ...extra,
  }
  if (!opts.omit_kcid) {
    body.kcid = '{{order.kcid}}'
  }
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: `{{huoyuan.url}}${actPath}`,
      content_type: 'form',
      use_cookie: opts.use_cookie,
      body,
      response: {
        code_field: 'code',
        success_codes: opts.success_codes,
        msg_field: 'msg',
        yid_field: opts.yid_field,
        success_msg: '下单成功',
      },
    },
    warnings: [],
    special_notes: specialNotes,
  }
}

function parse27(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  const res = actAddWithSuccess(platformType, '/api.php?act=add', specialNotes, {}, {
    use_cookie: true,
    omit_kcid: true,
    success_codes: ['0', 0],
  })
  res.warnings = ['无 kcid 字段；需 use_cookie']
  return res
}

function parse2xx(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/add',
      content_type: 'json',
      body: {
        token: '{{huoyuan.pass}}',
        platform: '{{order.noun}}',
        school: '{{order.school}}',
        user: '{{order.user}}',
        pass: '{{order.pass}}',
        kcname: '{{order.kcname}}',
        kcid: '{{order.kcid}}',
        time: '{{order.uTime}}',
        score: '{{order.uScore}}',
        speed: '{{order.study_speed}}',
        exam_submit: '{{order.is_submit_exam}}',
        exam_time: '{{order.exam_time}}',
      },
      response: {
        code_field: 'code',
        success_codes: ['1', 1],
        msg_field: 'msg',
        yid_field: 'id',
        success_msg: '下单成功',
      },
    },
    warnings: ['token 取 huoyuan.pass；需订单 study_speed/is_submit_exam/exam_time'],
    special_notes: specialNotes,
  }
}

function parseBenz(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return {
    platform_type: platformType,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/add',
      content_type: 'form',
      use_cookie: true,
      body: {
        token: '{{huoyuan.token}}',
        ptid: '{{order.noun}}',
        school: '{{order.school}}',
        user: '{{order.user}}',
        pass: '{{order.pass}}',
        kcname: '{{order.kcname}}',
        kcid: '{{order.kcid}}',
        shichang: '{{order.uTime}}',
      },
      response: { code_field: 'code', success_codes: ['0', 0], msg_field: 'msg', success_msg: '下单成功' },
    },
    warnings: ['platform 在 PHP 中为 ptid；需 use_cookie'],
    special_notes: specialNotes,
  }
}

function parseBld(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return actAddWithSuccess(platformType, '/api.php?act=addcf', specialNotes, {}, { success_codes: ['0', 0] })
}

function parseHzw(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return actAddWithSuccess(platformType, '/api.php?act=add', specialNotes, {}, {
    success_codes: ['1', 1],
    yid_field: 'id',
  })
}

function parseZfb(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return actAddWithSuccess(platformType, '/api.php?act=add', specialNotes, {}, {
    success_codes: ['0', 0],
    yid_field: 'id',
  })
}

function parseDuowei(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return parseZfb(platformType, specialNotes)
}

function parseWanzi(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return parseZfb(platformType, specialNotes)
}

function parseXm(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return actAddWithSuccess(platformType, '/api.php?act=add', specialNotes, {}, {
    use_cookie: true,
    success_codes: ['0', 0],
    yid_field: 'id',
  })
}

function parseHb(platformType: string, specialNotes: string[]): KnownXdjkParseResult {
  return parseZfb(platformType, specialNotes)
}

type StandardActAddSpec = {
  extra?: Record<string, string>
  use_cookie?: boolean
  yid_field?: string
  act_path?: string
}

const STANDARD_ACT_ADD: Record<string, StandardActAddSpec> = {
  '29': {},
  liufu: {},
  ssrs: {},
  daxiong: {},
  bdkj: {},
  ace: {},
  maodou: { extra: { shichang: '{{order.uTime}}', score: '{{order.uScore}}' } },
  bsc: {},
  xuemei: {},
  liunian: { yid_field: 'id' },
  tom: { extra: { type: '29' }, yid_field: 'id' },
  yue29: {},
  '2023': {},
  hh: {},
  huangzu: { use_cookie: true, yid_field: 'id' },
  pup: {},
  ml: {},
  hei: {},
  miaosha: { yid_field: 'id' },
  wufu: {},
}

function parseStandardActAdd(platformType: string, specialNotes: string[]): KnownXdjkParseResult | null {
  const spec = STANDARD_ACT_ADD[platformType.toLowerCase()]
  if (!spec) return null
  return actAddWithSuccess(
    platformType,
    spec.act_path ?? '/api.php?act=add',
    specialNotes,
    spec.extra ?? {},
    {
      use_cookie: spec.use_cookie,
      success_codes: ['0', 0],
      yid_field: spec.yid_field,
    },
  )
}
