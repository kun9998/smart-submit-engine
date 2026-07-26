# 智能提交引擎

一句话说明：**帮你把订单自动提交到各个平台的小引擎**。  

当前版本：`V3.5.1`

---

## 它能干什么？

- 从你的主站数据库里捞「待提交」订单
- 按货源 / 平台规则自动提交
- 提供管理后台：看队列、改配置、管平台规则、看日志
- 支持限流、重试、异常标记等常见运维能力

适合：自己有订单系统，想接多个上游平台自动下单的朋友。

---

## 你需要准备什么？

| 东西 | 说明 |
|------|------|
| 一台电脑或服务器 | Windows / Linux 都行 |
| MySQL | 两套库：主站订单库 + 插件库（可同一台机器） |
| Redis | 用来做订单队列 |
| Go 1.22+ | 编译后端 |
| Node.js 18+ | 编译前端管理台（可选，也可用本仓库构建好的页面） |

---

## 5 分钟上手

### 1. 下载代码

```bash
git clone https://github.com/kun9998/smart-submit-engine.git
cd smart-submit-engine
```

### 2. 准备配置

```bash
copy config.example.yaml config.yaml
```

Linux / macOS：

```bash
cp config.example.yaml config.yaml
```

`config.yaml` 里先可以不填数据库，后面用网页安装向导。

### 3. 编译并启动后端

```bash
go build -o tj.exe .
.\tj.exe
```

Linux：

```bash
go build -o tj .
./tj
```

默认管理后台地址：`http://127.0.0.1:8090`

### 4. 打开网页完成安装

浏览器打开管理后台 → 按向导填：

1. **主站库**：你的订单所在数据库  
2. **插件库**：引擎自己用的库（会自动建表）  
3. **管理员账号密码**：以后登录用  

装完即可登录使用。

### 5.（可选）自己编译前端

如果页面显示不正常，或你改了前端代码：

```bash
cd web
npm install
npm run build
```

把 `web/dist` 内容拷到项目根目录的 `admin-dist`（也可用仓库里的 `build-frontend.bat`）。

---

## 目录大概长什么样？

```
├── *.go              后端源码（Go）
├── web/              前端源码（Vue）
├── admin-dist/       已打包的管理后台静态页
├── migrations/       数据库相关
├── plugin_install.sql
├── config.example.yaml
└── docs/             更多说明
```


---

## 文档

- [安装说明](docs/安装说明.md) — 环境、数据库、常见坑
- [开发说明](docs/开发说明.md) — 本地改代码怎么跑
- [功能介绍](docs/功能介绍.md) — 管理台里各菜单是干什么的

---

## 许可证

本仓库以开源方式发布，方便学习与自用。  
**请勿把真实数据库密码、密钥提交到 Git。** 只用 `config.example.yaml` 当模板。

---

## 声明

- 请遵守你所对接平台的服务条款与当地法律法规。
- 开源不等于免责：线上环境请自己做好备份、权限与安全防护。
