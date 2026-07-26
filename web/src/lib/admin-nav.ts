/** 管理后台导航文案（侧边栏与路由 meta 共用，避免两处不一致） */

export interface AdminNavItem {
  title: string
  path: string
  description: string
}

export const adminNavGroups: { label: string; items: AdminNavItem[] }[] = [
  {
    label: '订单',
    items: [
      {
        title: '订单概览',
        path: '/',
        description: '实时看各渠道订单成功、失败和排队情况',
      },
      {
        title: '平台规则',
        path: '/platforms',
        description: '配置各平台怎么下单，支持粘贴 PHP 或 AI 转换',
      },
      {
        title: '货源配置',
        path: '/huoyuan',
        description: '全局和每个渠道的并发、重试、成功判断等',
      },
      {
        title: '提交日志',
        path: '/logs',
        description: '查看每笔订单的提交过程和报错',
      },
      {
        title: '开发文档',
        path: '/docs',
        description: '怎么写下单规则、可用字段和平台示例',
      },
    ],
  },
  {
    label: '系统',
    items: [
      {
        title: '系统设置',
        path: '/settings',
        description: '订单队列、访问安全、授权和智能助手',
      },
      {
        title: '系统监控',
        path: '/monitor',
        description: '看服务器和程序是否在正常运行',
      },
      {
        title: 'AI 运维',
        path: '/ops',
        description: '自动检查订单异常、记录操作、可手动处理',
      },
      {
        title: '系统升级',
        path: '/upgrade',
        description: '检查并安装新版本',
      },
    ],
  },
]

export const adminProfileNav: AdminNavItem = {
  title: '个人中心',
  path: '/profile',
  description: '改密码、通知和 Showdoc 绑定',
}

/** path → 导航项，供路由 meta 查找 */
export const adminNavByPath = new Map<string, AdminNavItem>(
  [...adminNavGroups.flatMap((g) => g.items), adminProfileNav].map((item) => [item.path, item]),
)

export const adminProductName = '智能提交引擎'
