import type { SubmitRuleConfig } from '@/api/submit-platform'
import { defaultRuleConfig } from '@/api/submit-platform'
export { FIELD_MAPPING } from '@/lib/template-vars'

export interface DevDocExample {
  id: string
  title: string
  platform_type: string
  tag: '标准' | 'JSON' | 'GET' | 'Cookie' | '特殊' | 'V2'
  summary: string
  php: string
  rule_config: SubmitRuleConfig
  notes?: string[]
}

export const SPECIAL_CASES = [
  { platform: 'KUN / KUN1', reason: '网址端口要随机换', suggestion: '配置 url_port_pool，网址里用 {{random_port}}' },
  { platform: 'kdbxxt / kdbzhs / kdbzhzj', reason: 'kcid 是 base64，还要按 noun 改内容', suggestion: '用 body_mode: kcid_json，再配合 branches 分支' },
  { platform: 'lyyjxjy（懒洋洋）', reason: '不同课程名/学校走不同接口，还要等待', suggestion: '用 branches 分支 + delay_ms 等待 + body_raw' },
  { platform: 'df1 / df2', reason: '请求体是嵌套 JSON 数组', suggestion: '各建一条规则，body_mode 设为 raw' },
  { platform: 'maliaorun（龙猫）', reason: '不同 noun 用不同 body，成功看 msg 文字', suggestion: '用 branches，code_field 设为 msg' },
  { platform: 'simple', reason: '要把多个字段拼进 platform', suggestion: '用 {{concat ...}} 拼接' },
  { platform: 'wuming（无名）', reason: 'body 里有嵌套 courseInfo', suggestion: 'body_mode 设为 raw，整段自己写' },
  { platform: 'lotus', reason: '下单成功后还要写主库备注', suggestion: '提交可用 JSON 规则；备注需主站另外处理' },
  { platform: 'dlam（哆啦A梦）', reason: 'noun 要换算成 operate 等字段', suggestion: '用 branches 或拆成多条规则' },
] as const

export const DEV_DOC_EXAMPLES: DevDocExample[] = [
  {
    id: 'ssrs',
    title: '上善若水 ssrs（最常见标准写法）',
    platform_type: 'ssrs',
    tag: '标准',
    summary: 'get_url + form 表单 + code==0 即成功，与 defaultRuleConfig 完全一致。',
    php: `else if ($type == "ssrs") {
  $data = array(
    "uid" => $a["user"], "key" => $a["pass"],
    "platform" => $noun, "school" => $school,
    "user" => $user, "pass" => $pass,
    "kcname" => $kcname, "kcid" => $kcid
  );
  $ace_url = $a["url"] . "/api.php?act=add";
  $result = get_url($ace_url, $data);
  $result = json_decode($result, true);
  if ($result["code"] == "0") {
    $b = array("code" => 1, "msg" => "下单成功");
  } else {
    $b = array("code" => -1, "msg" => $result["msg"]);
  }
  return $b;
}`,
    rule_config: defaultRuleConfig(),
    notes: [
      'platform_type 填 ssrs，与货源里的平台类型（pt）一致',
      '也可在「平台规则」页直接粘贴整段 PHP，点「解析」自动生成',
    ],
  },
  {
    id: '27',
    title: '27 系统（带 Cookie）',
    platform_type: '27',
    tag: 'Cookie',
    summary: 'get_url 第三个参数传 $cookie，规则里设 use_cookie: true。',
    php: `if ($type == "27") {
  $data = array("uid" => $a["user"], "key" => $a["pass"],
    "platform" => $noun, "school" => $school,
    "user" => $user, "pass" => $pass, "kcname" => $kcname);
  $eq_url = $a["url"] . "/api.php?act=add";
  $result = get_url($eq_url, $data, $cookie);
  ...
}`,
    rule_config: {
      ...defaultRuleConfig(),
      use_cookie: true,
      body: {
        uid: '{{huoyuan.user}}',
        key: '{{huoyuan.pass}}',
        platform: '{{order.noun}}',
        school: '{{order.school}}',
        user: '{{order.user}}',
        pass: '{{order.pass}}',
        kcname: '{{order.kcname}}',
      },
    },
    notes: ['body 无 kcid 时从 array 里删掉对应键即可'],
  },
  {
    id: '2xx',
    title: '爱学习 2xx（JSON + 成功码 1 + 回写 yid）',
    platform_type: '2xx',
    tag: 'JSON',
    summary: 'httpRequest POST JSON；成功 code 为 1；yid 取响应 id 字段。',
    php: `else if ($type == "2xx") {
  $data = array("token" => $a["pass"], "platform" => $noun, ...);
  $ixx_url = $a["url"] . "/api/add";
  $result = httpRequest('POST', $ixx_url, $data, [], true);
  if ($result["code"] == "1") {
    $b = array("code" => 1, "msg" => "下单成功", "yid" => $result["id"]);
  }
}`,
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
  },
  {
    id: 'baize',
    title: '白泽 baize（JSON + 嵌套 yid）',
    platform_type: 'baize',
    tag: 'JSON',
    summary: 'curl POST JSON；成功 code 为 0000；yid 在 data.order_id。',
    php: `else if ($type == "baize") {
  $data = array("token" => $token, "platform_id" => $noun, ...);
  $txt_url = $a["url"] . "/api/v2/docking/add";
  // curl POST json_encode($data)
  if ($result["code"] == "0000") {
    $b = array(..., "yid" => $result["data"]["order_id"]);
  }
}`,
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
    notes: ['嵌套字段用 yid_path，点号分隔：data.order_id'],
  },
  {
    id: 'xiyou',
    title: '西游 xiyou（status=success）',
    platform_type: 'xiyou',
    tag: '特殊',
    summary: '成功字段是 status 而非 code；值为 success。',
    php: `else if ($type == "xiyou") {
  $url = $a["url"] . '/api/order/xiadanForPublic';
  $data = array("username" => $a["user"], "token" => $token, ...);
  if ($result["status"] == "success") { ... }
}`,
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
    notes: ['无 yid 时可省略 yid_field/yid_path'],
  },
  {
    id: 'yyy',
    title: 'YYY（code=200 + data.yid）',
    platform_type: 'yyy',
    tag: '标准',
    summary: 'URL 为 /api/order；成功 code=200；yid 在 data.yid。',
    php: `else if ($type == "yyy") {
  $dx_url = "$dx_rl/api/order";
  if ($result["code"] == "200") {
    $b = array(..., "yid" => $result["data"]["yid"]);
  }
}`,
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
  },
  {
    id: '8090',
    title: '8090 继续教育（200 但 message 可含失败）',
    platform_type: '8090',
    tag: 'JSON',
    summary: 'Bearer + body_raw；code=200 成功，但 message 含「失败」仍算失败；用 failure_msg_on_success。',
    php: `else if ($type == "8090") {
  if ($result['code'] == 200) {
    if (strpos($result['message'], '失败') !== false) { ... }
  }
}`,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api/order/submit',
      content_type: 'json',
      headers: {
        'Content-Type': 'application/json; charset=utf-8',
        Authorization: 'Bearer {{huoyuan.token}}',
      },
      body_mode: 'raw',
      body_raw:
        '{"websiteId":"{{order.noun}}","accountInfo":"{{order.user}} {{order.pass}}","selectedCourseKeys":["{{order.kcname}}"]}',
      body: {},
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
    notes: ['failure_msg_on_success 在 success_codes 命中后仍检查 message 子串'],
  },
  {
    id: 'benz',
    title: 'benz（token 字段 + Cookie）',
    platform_type: 'benz',
    tag: 'Cookie',
    summary: 'body 用 token 而非 uid/key；URL 为 /api/add。',
    php: `else if ($type == "benz") {
  $data = array("token" => $token, "ptid" => $noun, ...);
  $benz_url = $a["url"] . "/api/add";
  $result = get_url($benz_url, $data, $cookie);
}`,
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
      response: {
        code_field: 'code',
        success_codes: ['0', 0],
        msg_field: 'msg',
        success_msg: '下单成功',
      },
    },
  },
  {
    id: 'zfb',
    title: 'zfb（成功回写 yid）',
    platform_type: 'zfb',
    tag: '标准',
    summary: '标准 form 提交，但成功时需把上游 id 写入 yid。',
    php: `else if ($type == "zfb") {
  ...
  if ($result["code"] == "0") {
    return ['code' => 1, 'msg' => '对接成功', 'yid' => $result['id']];
  }
}`,
    rule_config: {
      ...defaultRuleConfig(),
      response: {
        code_field: 'code',
        success_codes: ['0', 0],
        msg_field: 'msg',
        yid_field: 'id',
        success_msg: '下单成功',
      },
    },
  },
  {
    id: 'algk',
    title: '国开 algk（GET 查询参数在 URL）',
    platform_type: 'algk',
    tag: 'GET',
    summary: '无 $data body，参数拼在 URL；成功 code 为 200。',
    php: `else if ($type == "algk") {
  $eq_url = $a["url"]
    . "/api/Open/AddRenWu?username={$a['user']}&password={$a['pass']}"
    . "&zhanghao={$user}&psd={$pass}&kcname={$kcname}";
  $result = get_url($eq_url, $cookie);
  if ($result["code"] == "200") { ... }
}`,
    rule_config: {
      method: 'GET',
      url: '{{huoyuan.url}}/api/Open/AddRenWu?username={{huoyuan.user}}&password={{huoyuan.pass}}&zhanghao={{order.user}}&psd={{urlencode order.pass}}&kcname={{urlencode order.kcname}}',
      content_type: 'form',
      use_cookie: true,
      body: {},
      response: {
        code_field: 'code',
        success_codes: ['200', 200],
        msg_field: 'msg',
        success_msg: '下单成功',
      },
    },
    notes: ['中文或特殊字符用 {{urlencode 字段名}} 包裹'],
  },
  {
    id: 'bld',
    title: '便利店 bld（不同 act）',
    platform_type: 'bld',
    tag: '标准',
    summary: '与 ssrs 相同 body，仅 URL 路径 act=addcf 不同。',
    php: `$dx_url = $a["url"] . "/api.php?act=addcf";`,
    rule_config: {
      ...defaultRuleConfig(),
      url: '{{huoyuan.url}}/api.php?act=addcf',
    },
  },
  {
    id: 'hzw',
    title: 'hzw（form 表单 + 成功码 1）',
    platform_type: 'hzw',
    tag: '标准',
    summary: 'get_url 标准 form-urlencoded POST；成功 code 为 1，yid 取 id。',
    php: `else if ($type == "hzw") {
  $expand = [];
  $data = array(
    "uid" => $a["user"], "key" => $a["pass"],
    "platform" => $noun, "school" => $school,
    "user" => $user, "pass" => $pass,
    "kcid" => $kcid, "kcname" => $kcname,
    "expand" => $expand
  );
  $eq_url = $a["url"] . "/api.php?act=add";
  $result = get_url($eq_url, $data);
  $result = json_decode($result, true);
  if ($result["code"] == "1") {
    $b = array("code" => 1, "msg" => "下单成功", "yid" => $result["id"]);
  } else {
    $b = array("code" => -1, "msg" => $result["msg"]);
  }
  return $b;
}`,
    rule_config: {
      method: 'POST',
      url: '{{huoyuan.url}}/api.php?act=add',
      content_type: 'form',
      body: {
        uid: '{{huoyuan.user}}',
        key: '{{huoyuan.pass}}',
        platform: '{{order.noun}}',
        school: '{{order.school}}',
        user: '{{order.user}}',
        pass: '{{order.pass}}',
        kcid: '{{order.kcid}}',
        kcname: '{{order.kcname}}',
      },
      response: {
        code_field: 'code',
        success_codes: ['1', 1],
        msg_field: 'msg',
        yid_field: 'id',
        success_msg: '下单成功',
      },
    },
    notes: [
      'content_type: "form" 即 Content-Type: application/x-www-form-urlencoded，与 get_url 一致',
      '不必在 headers 里再写 Content-Type',
      'expand 数组基础下单可省略；上游强制要求时可加 "expand": "[]"',
    ],
  },
  {
    id: 'thoth',
    title: 'THOTH OpenAPI（t.thoth8.com）',
    platform_type: 'THOTH',
    tag: 'OpenAPI',
    summary: 'Header X-Uid/X-Api-Key + POST form /api/open/add；下单成功 code=0，yid=id。',
    php: `else if ($type == "THOTH") {
  $headers = array(
    'Content-Type: application/x-www-form-urlencoded',
    'X-Uid: ' . $a["user"],
    'X-Api-Key: ' . $a["pass"],
  );
  $data = array(
    "platform" => (string)$noun,
    "school"   => $school,
    "user"     => $user,
    "pass"     => $pass,
    "kcname"   => json_encode(array($kcname), JSON_UNESCAPED_UNICODE),
    "kcid"     => json_encode(array($kcid), JSON_UNESCAPED_UNICODE),
    "score"    => (string)$uScore,
    "shichang" => (string)$uTime,
  );
  $result = httpRequest('POST', $a["url"] . "/api/open/add", $data, $headers, false);
  $result = json_decode($result, true);
  if ($result["code"] == "0") {
    return array("code" => 1, "msg" => "下单成功", "yid" => $result["id"]);
  }
  return array("code" => -1, "msg" => $result["msg"]);
}`,
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
      response: {
        code_field: 'code',
        success_codes: ['0', 0],
        msg_field: 'msg',
        yid_field: 'id',
        success_msg: '下单成功',
      },
    },
    notes: [
      '货源 URL 填 https://t.thoth8.com（引擎自动拼 /api/open/add）',
      'uid/key 仅放 Header，勿写入 body 或 query',
      '查余额 POST {{huoyuan.url}}/api/open/getmoney，仅 Header 鉴权',
      '查课 POST {{huoyuan.url}}/api/open/get，参数 platform/school/user/pass',
      'kcname/kcid 支持 JSON 数组字符串；单门课也可直接传课名字符串',
    ],
  },
  {
    id: 'longlong',
    title: '龙龙 longlong（Open API V2）',
    platform_type: 'longlong',
    tag: 'V2',
    summary: 'Header X-Uid/X-Api-Key + POST JSON /api/submit/{plat}；HTTP 2xx 成功，yid 为 UUID 数组。',
    php: `else if ($type == "longlong") {
  require_once __DIR__ . '/LongLongV2.php';
  $payload = array(
    'username' => $user,
    'password' => $pass,
    'courses'  => array($kcid),
  );
  $res = llv2_submit($a, $noun, $payload);
  if (!$res['ok']) return array("code" => -1, "msg" => $res['msg']);
  return array("code" => 1, "msg" => "下单成功", "yid" => $res['ids'][0]);
}`,
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
      body_raw: '{"username":"{{order.user}}","password":"{{order.pass}}","courses":["{{order.kcid}}"]}',
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
    notes: [
      '不是 api.php?act=add；认证在 Header，不在 body',
      'response.success_http: true 表示 HTTP 2xx 即成功',
      'courses 必须是 JSON 数组，kcid 为查课 hash',
      '{{huoyuan.url}} 自动去掉末尾 /，填 http://域名 或 http://域名/ 均可',
    ],
  },
]
