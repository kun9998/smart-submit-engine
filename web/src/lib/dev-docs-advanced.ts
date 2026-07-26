/** 高级 rule_config 能力说明与完整 JSON 示例 */

export const ADVANCED_FEATURES = [
  {
    feature: 'body_mode: "raw"',
    desc: '自己写整段请求体，适合嵌套 JSON 或特殊 form 字符串',
    template: '{{concat ...}} / {{order.user}}',
  },
  {
    feature: 'body_mode: "kcid_json"',
    desc: '把 order.kcid 解码成 JSON，再按 noun 打补丁后提交（课代表系列）',
    template: 'kcid_json_patches + kcid_json_validate',
  },
  {
    feature: 'branches',
    desc: '按课程名、学校、noun 等条件，走不同的网址和 body',
    template: 'when.contains / when.equals / when.default',
  },
  {
    feature: 'url_port_pool',
    desc: '网址里写 {{random_port}}，从端口列表随机（KUN）',
    template: '[1234,1235,1236]',
  },
  {
    feature: '{{split_part ...}}',
    desc: '按分隔符截取一段文字（如学校|考试码）',
    template: '{{split_part order.school 1 "|" 2}}',
  },
  {
    feature: '{{concat ...}}',
    desc: '拼接多段文字（simple 改 platform）',
    template: '{{concat order.noun "|score=" order.uScore}}',
  },
  {
    feature: 'handler: "pipeline"',
    desc: '多步流程：先登录再下单、轮询查状态等',
    template: 'pipeline[].action: http|finish|extract|set|delay|poll',
  },
  {
    feature: '{{var.xxx}}',
    desc: '多步流程里传递上一步的结果（如 token）',
    template: '{{var.token}} / {{json_path var.login_body data.token}}',
  },
  {
    feature: 'delay_ms',
    desc: '提交前等待若干毫秒（PHP sleep(5) → 5000）',
    template: '5000',
  },
  {
    feature: 'action: "poll"',
    desc: '反复请求直到成功或超时（查课、查状态）',
    template: 'poll.interval_ms / max_attempts / until',
  },
  {
    feature: 'process 查课',
    desc: '查学习进度：process.handler + process.map 映射字段',
    template: 'process.map.items_path + fields',
  },
  {
    feature: 'response.success_http',
    desc: '只要 HTTP 返回 2xx 就算成功（龙龙 V2 等无 code 字段）',
    template: '"success_http": true, "yid_path": "0"',
  },
  {
    feature: 'script.source',
    desc: '用 Starlark 脚本写复杂逻辑（高级用法）',
    template: 'result = {"code":1,"msg":"ok","yid":"..."}',
  },
] as const

export interface AdvancedRuleExample {
  id: string
  platform: string
  title: string
  solution: string
  rule_config: Record<string, unknown>
}

export const ADVANCED_RULE_EXAMPLES: AdvancedRuleExample[] = [
  {
    id: 'kun',
    platform: 'KUN',
    title: '随机端口',
    solution: 'url_port_pool + URL 模板 {{random_port}}，无需 Go 硬编码',
    rule_config: {
      method: 'GET',
      url: '{{huoyuan.url}}:{{random_port}}/getorder/?platform={{urlencode order.noun}}&school={{urlencode order.school}}&account={{order.user}}&password={{order.pass}}&course={{urlencode order.kcname}}&kcid={{order.kcid}}',
      url_port_pool: [1234, 1235, 1236, 1237, 1238],
      content_type: 'form',
      body: {},
      response: {
        code_field: 'code',
        success_codes: ['1', 1],
        msg_field: 'msg',
        yid_field: 'order_token',
      },
    },
  },
  {
    id: 'df1',
    platform: 'df1',
    title: '嵌套 JSON 数组 body',
    solution: 'body_mode: raw + body_raw 写完整 JSON 模板；df2 另建一条，test 改为 1',
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/df/api/xd',
      content_type: 'json',
      body_mode: 'raw',
      body_raw:
        '[{"num":"{{order.user}}","pwd":"{{order.pass}}","name":"","mark":"平台名称","mode":"1","test":0,"list":[{"code":"{{order.kcid}}","name":"{{order.kcname}}"}]}]',
      headers: {
        Authorization: 'DfAi {{huoyuan.token}}',
        'Content-Type': 'application/json;charset=UTF-8',
      },
      body: {},
      response: { code_field: 'code', success_codes: [200, '200'], msg_field: 'msg' },
    },
  },
  {
    id: 'simple',
    platform: 'simple',
    title: '提交前拼接 noun',
    solution: '{{concat}} 在 body.platform 里拼 score/sc；扩展字段用 order.simple_*',
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/Api/Create',
      content_type: 'form',
      body: {
        token: '{{huoyuan.token}}',
        platform:
          '{{concat order.noun "|score=" order.uScore ";sc=" order.uTime "|day_score=" order.simple_day_score ";total_score=" order.simple_total_score ";learntime=" order.simple_learn_time}}',
        school: '{{order.school}}',
        user: '{{order.user}}',
        pass: '{{order.pass}}',
        course: '{{order.kcname}}',
        courseid: '{{order.kcid}}',
      },
      response: { code_field: 'code', success_codes: ['1', 1], msg_field: 'msg' },
    },
  },
  {
    id: 'wuming',
    platform: 'wuming',
    title: '嵌套 courseInfo 数组',
    solution: 'body_mode: raw，整段 JSON 用模板占位',
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/demo/submitOrder',
      content_type: 'json',
      headers: { token: '{{huoyuan.token}}' },
      body_mode: 'raw',
      body_raw:
        '{"platformId":"{{order.noun}}","school":"{{order.school}}","account":"{{order.user}}","password":"{{order.pass}}","duration":"无","score":"无","courseInfo":[{"courseName":"{{order.kcname}}","courseId":"{{order.kcid}}","unitList":[]}]}',
      body: {},
      response: { code_field: 'code', success_codes: ['1', 1], msg_field: 'msg', yid_path: 'data.id' },
    },
  },
  {
    id: 'pipeline-login',
    platform: 'demo',
    title: '多步：登录拿 token 再下单',
    solution: 'handler: pipeline；http + extract + finish，全在 Go worker 内',
    rule_config: {
      handler: 'pipeline',
      method: 'POST',
      url: '',
      content_type: 'form',
      body: {},
      response: {
        code_field: 'code',
        success_codes: ['1', 1],
        msg_field: 'msg',
        yid_field: 'id',
        success_msg: '下单成功',
      },
      pipeline: [
        {
          action: 'http',
          method: 'POST',
          url: '{{huoyuan.url}}/login',
          content_type: 'json',
          body: { user: '{{huoyuan.user}}', pass: '{{huoyuan.pass}}' },
          save_body_as: 'login_body',
        },
        {
          action: 'extract',
          extract: { from: 'login_body', path: 'data.token', to: 'token' },
        },
        {
          action: 'finish',
          method: 'POST',
          url: '{{huoyuan.url}}/submit',
          content_type: 'json',
          body: {
            token: '{{var.token}}',
            user: '{{order.user}}',
            pass: '{{order.pass}}',
            platform: '{{order.noun}}',
          },
        },
      ],
    },
  },
  {
    id: 'maliaorun',
    platform: 'maliaorun',
    title: '按 noun 三套 body + msg 判成功',
    solution: 'branches 按 order.noun 分支；response.code_field 设为 msg',
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/gk/order',
      content_type: 'json',
      headers: { Authorization: 'Bearer {{huoyuan.token}}' },
      body_mode: 'raw',
      body: {},
      branches: [
        {
          when: { field: 'order.noun', equals: '1' },
          body_raw:
            '{"xh":"{{order.user}}","password":"{{order.pass}}","courses":["{{order.kcname}}"],"config":{"openLog":true,"openExam":true,"openHomeWork":true,"openRead":true,"forceRun":false},"learnTimes":"500","learnDays":"5"}',
        },
        {
          when: { field: 'order.noun', equals: '2' },
          body_raw:
            '{"xh":"{{order.user}}","password":"{{order.pass}}","courses":["{{order.kcname}}"],"config":{"forceRun":true},"learnTimes":"500","learnDays":"5"}',
        },
        {
          when: { default: true },
          body_raw:
            '{"xh":"{{order.user}}","password":"{{order.pass}}","courses":["{{order.kcname}}"],"learnTimes":"800","learnDays":"15"}',
        },
      ],
      response: {
        code_field: 'msg',
        success_codes: ['所有课程下单成功'],
        msg_field: 'msg',
      },
    },
  },
  {
    id: 'lyyjxjy',
    platform: 'lyyjxjy',
    title: '按课程名 / 学校分支',
    solution: 'branches + when.contains；delay_ms 代替 sleep(5)',
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
  },
  {
    id: 'kdbxxt',
    platform: 'kdbxxt',
    title: 'kcid base64 JSON + noun 补丁',
    solution: 'body_mode: kcid_json + branches 里 kcid_json_patches',
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/xxt/xxt_task/',
      content_type: 'json',
      body_mode: 'kcid_json',
      headers: {
        Authorization: 'Bearer {{huoyuan.token}}',
        'Content-Type': 'application/json',
      },
      body: {},
      kcid_json_validate: { path: 'task_list', exact: 1 },
      branches: [
        {
          when: { field: 'order.noun', equals: '秒刷不清' },
          kcid_json_patches: [
            {
              set: {
                'task_list.0.config.checked_config_list': [
                  '视频', '作业', '考试', '章测', '直播', '阅读', '秒刷不清',
                ],
              },
            },
          ],
        },
        {
          when: { field: 'order.noun', equals: '单考试' },
          kcid_json_patches: [{ set: { 'task_list.0.config.checked_config_list': ['考试'] } }],
        },
        {
          when: { field: 'order.noun', equals: '单任务点' },
          kcid_json_patches: [
            { set: { 'task_list.0.config.checked_config_list': ['视频', '章测', '直播', '阅读'] } },
          ],
        },
        {
          when: { field: 'order.noun', equals: '章测收录' },
          kcid_json_patches: [{ set: { 'task_list.0.config.checked_config_list': ['收录'] } }],
        },
        {
          when: { field: 'order.noun', equals: '考试收录' },
          kcid_json_patches: [{ set: { 'task_list.0.config.checked_config_list': ['考试收录'] } }],
        },
        {
          when: { field: 'order.noun', equals: '秒刷' },
          kcid_json_patches: [
            {
              set: {
                'task_list.0.config.checked_config_list': [
                  '视频', '作业', '考试', '章测', '直播', '阅读', '秒刷',
                ],
              },
            },
          ],
        },
        {
          when: { default: true },
          kcid_json_patches: [],
        },
      ],
      response: {
        code_field: 'code',
        success_codes: ['1', 1],
        msg_field: 'msg',
      },
    },
  },
  {
    id: 'poll',
    platform: 'demo',
    title: 'poll 轮询查状态',
    solution: 'action=poll + poll.until.success_codes，interval_ms / max_attempts 控制频率',
    rule_config: {
      handler: 'pipeline',
      method: 'POST',
      url: '',
      content_type: 'form',
      body: {},
      response: { code_field: 'code', success_codes: [1, '1'], msg_field: 'msg' },
      pipeline: [
        {
          action: 'poll',
          method: 'GET',
          url: '{{huoyuan.url}}/status?yid={{order.yid}}',
          poll: {
            interval_ms: 2000,
            max_attempts: 15,
            until: { code_field: 'status', success_codes: ['done', 1] },
          },
        },
        {
          action: 'finish',
          method: 'GET',
          url: '{{huoyuan.url}}/status?yid={{order.yid}}',
          response: { code_field: 'code', success_codes: [1, '1'], msg_field: 'msg' },
        },
      ],
    },
  },
  {
    id: 'process',
    platform: 'demo_cx',
    title: 'ProcessOrder 查课',
    solution: 'process.handler=http + process.map.fields 映射 progress/kcname',
    rule_config: {
      method: 'POST',
      url: '',
      content_type: 'form',
      body: {},
      response: { code_field: 'code', success_codes: [1], msg_field: 'msg' },
      process: {
        handler: 'http',
        method: 'GET',
        url: '{{huoyuan.url}}/query?yid={{order.yid}}',
        map: {
          code_field: 'code',
          success_codes: [0, '0'],
          fields: {
            kcname: 'data.name',
            process: 'data.progress',
            status_text: 'data.status',
          },
        },
      },
    },
  },
]

export { COVERAGE_SUMMARY, TIER_LABELS, XDJK_PLATFORM_COVERAGE } from './platform-coverage'

/** 全部平台均可在管理端用 JSON 规则配置 */
export const STILL_NEED_GO = ['lotus'] as const
