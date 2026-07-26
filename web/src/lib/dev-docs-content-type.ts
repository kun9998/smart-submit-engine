import type { SubmitRuleConfig } from '@/api/submit-platform'

/** content_type 与 PHP 请求方式对照 */
export const CONTENT_TYPE_ROWS = [
  {
    value: 'form',
    header: 'application/x-www-form-urlencoded',
    bodyDesc: 'body 对象各键值经 urlencode 后拼成 uid=xx&key=yy&...',
    php: 'get_url($url, $data) 或 httpRequest(..., false)',
    when: 'xdjk 里 $data = array(...) 后直接 POST，无 json_encode',
  },
  {
    value: 'json',
    header: 'application/json',
    bodyDesc: 'body 对象序列化为 JSON 字符串',
    php: "httpRequest('POST', $url, $data, [], true) 或 curl + json_encode",
    when: 'PHP 里 Content-Type: application/json 或第五个参数 true',
  },
] as const

export const FORM_CONFIG_TIPS = [
  '规则里写 "content_type": "form" 即可，程序会自动按表单方式提交，一般不用在 headers 里再写 Content-Type。',
  'body 里每个键对应 PHP 里 $data 数组的一个字段，值写模板，如 "uid": "{{huoyuan.user}}"。',
  '值为空的字段会自动省略，不需要的键直接从 body 里删掉。',
  '如果是 GET 且参数在网址上，method 设 GET、body 留 {} 即可。',
] as const

/** hzw 完整 form 示例（与安装种子、xdjk.php 407–427 行一致） */
export const HZW_FORM_EXAMPLE = {
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
  $result = get_url($eq_url, $data);   // POST form-urlencoded
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
    use_cookie: false,
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
  } satisfies SubmitRuleConfig,
  notes: [
    'content_type 设为 form 就等价于 PHP 里 get_url 的表单提交，不用自己写 Content-Type。',
    'hzw 成功码是 1（不是 0），response.success_codes 写 ["1", 1]。',
    'PHP 里 expand 是数组；普通下单可省略。上游强制要求时，可在 body 加 "expand": "[]"。',
    'expand 里结构很复杂时，改用 body_mode: "raw" 自己写整段内容。',
  ],
} as const
