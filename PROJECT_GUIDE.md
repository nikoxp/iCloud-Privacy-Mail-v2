# iCloud Privacy Mail 项目指南

> 文档版本：2026-08-10
>
> 项目目录：`iCloud-Privacy-Mail-v2`
>
> 运行定位：单管理员、本地优先的 Apple 隐私邮箱管理工具

本项目参考开源项目 [q1953258942/iCloud-Privacy-Mail](https://github.com/q1953258942/iCloud-Privacy-Mail) 的 Apple/iCloud 协议实现、隐私邮箱数据模型和邮件处理思路，在独立目录中重新组织后端边界，并使用 Vue 3 重建管理页面。参考项目保留原版权归属；本项目的页面、模块划分和本地单管理员流程属于重新实现。

文档中的页面截图来自独立的空数据实例，统一为 1264 × 832，只包含浏览器页面，没有真实 Apple 账号、邮箱、Cookie、密码或邮件内容。

## 1. 项目能力

- 首次运行创建唯一的本地管理员，后续访问管理页面需要登录。
- 支持 Apple Account 新接口与 iCloud Web 旧接口登录、两步验证和登录态检测。
- 支持保存 iCloud IMAP App 专用密码、实时监听邮件和手动取码。
- 支持创建、同步、导入、筛选、快捷编辑、停用和删除隐私邮箱。
- 支持清理 Apple 远端邮件，并同步更新本地邮件数量。
- 支持立即创建一个隐私邮箱，或按账号和时间间隔自动创建。
- 支持公共取号 API、邮箱独立取码 API 和面向外部用户的公共验证码页面。
- 支持导出运行数据、邮件、邮箱地址和邮箱取码 API。
- 使用本地 JSON 保存状态，前端构建产物嵌入 Go 服务，可由单一二进制运行。

项目不包含多用户注册、用户管理后台和 `/manage` 页面。访问 `/manage` 或 `/manage/` 会返回 404。

## 2. 快速开始

### 2.1 环境要求

| 工具 | 建议版本 | 用途 |
| --- | --- | --- |
| Go | 1.25 或更高 | 编译和运行后端、迁移工具 |
| Node.js | 满足 Vite 8 要求的版本 | 构建 Vue 前端 |
| npm | 与 Node.js 配套 | 安装前端依赖 |

### 2.2 首次构建

在项目根目录执行：

```bash
cp config.example.json config.json
./scripts/build.sh
go run main.go
```

无参数执行 `go run main.go` 时会打开中文启动菜单。选择“使用当前配置启动服务”后访问：

```text
http://127.0.0.1:8788/
```

项目没有预设密码。首次打开会要求创建管理员账号和至少 8 位密码；管理员创建后，后续访问进入登录页。

### 2.3 常用启动方式

```bash
# 使用默认 config.json 启动并显示菜单
go run main.go

# 跳过菜单，直接按配置启动
go run main.go -config config.json

# 显式打开菜单
go run main.go -menu

# 临时覆盖地址、端口和状态文件
go run main.go -config config.json -host 127.0.0.1 -port 8788 -data data/state.json

# 运行已构建的二进制
./bin/ipm-server -config config.json
```

### 2.4 开发模式

```bash
./scripts/dev.sh
```

- Vue 开发地址：`http://127.0.0.1:5174/`
- Go API 地址：`http://127.0.0.1:8788/`
- Vite 会把 `/api` 请求代理到 Go 服务。

修改前端后用于正式运行时，应再次执行 `./scripts/build.sh`。脚本会安装锁定依赖、构建 Vue、同步静态资源并生成 `bin/ipm-server` 和 `bin/ipm-migrate`。

`go run main.go` 提供的是 `internal/webui/dist` 中的 Go 内嵌前端资源，不会直接读取 `frontend/src`。只需要刷新内嵌资源而不重新构建 Go 二进制时，可在项目根目录执行：

```bash
npm --prefix frontend run build
./scripts/sync-web.sh
go run main.go
```

使用 `./scripts/dev.sh` 时，浏览器应访问 Vite 的 `http://127.0.0.1:5174/`；此模式会直接反映前端源码变更，并把 `/api` 请求代理到 Go 服务。

## 3. 页面与使用方法

### 3.1 登录页 `/login`

![登录页](./docs/screenshots/empty/00-login.jpg)

登录页承担两种状态：

1. 首次运行：创建唯一的本地管理员。
2. 已初始化：使用管理员账号和密码进入控制台。

密码输入框带显示/隐藏按钮。登录成功后，服务通过 HttpOnly、SameSite Strict Cookie 保存控制台会话；默认会话有效期为 168 小时。公网 HTTPS 部署时应在配置中开启 `secure_cookie`。

### 3.2 控制台 `/`

![控制台](./docs/screenshots/empty/01-dashboard.jpg)

控制台用于快速查看整体状态：

- Apple 账号总数和正常账号数量。
- 隐私邮箱总数和当前可用数量。
- 本地邮件缓存数量。
- 最近运行事件，可通过右上角清空按钮清除。
- Apple 登录态每次实际发起保活请求时记录开始和结果；成功、临时失败与登录态失效使用不同级别展示，控制台会定时刷新。
- IMAP 监听、Apple 登录态保活、自动创建和公共取号 API 的运行状态。

控制台只展示汇总信息。具体账号操作进入“Apple 账号”，邮箱和验证码操作进入“邮箱池”，创建任务进入“创建隐私邮箱”。

### 3.3 Apple 账号 `/apple-accounts`

![Apple 账号](./docs/screenshots/empty/02-apple-accounts.jpg)

该页面集中管理一个 Apple 账号的三类登录态：

| 登录态 | 主要用途 |
| --- | --- |
| Apple Account 新接口 | 优先创建隐私邮箱、管理 Apple Account 会话 |
| iCloud Web 旧接口 | 同步远端隐私邮箱、兼容创建和远端邮件管理 |
| iCloud IMAP | 接收邮件、监听新邮件和提取验证码 |

推荐操作顺序：

1. 点击“添加 Apple 账号”。
2. 选择 Apple Account 或 iCloud Web 登录方式，输入 Apple ID 和密码。
3. Apple 要求两步验证时，在弹窗中提交可信设备验证码或短信验证码。
4. 选择账号，点击顶部“IMAP 取码”，保存 Apple ID 邮箱和 App 专用密码。
5. 点击“检测”分别验证已保存的登录态。
6. 点击“创建隐私邮箱”创建一个邮箱，或点击同步操作导入 Apple 服务器上的已有邮箱。

账号列表按创建时间从旧到新排列。删除账号只清理本地账号、登录态、关联邮箱和本地邮件，Apple 服务器上的隐私邮箱仍会保留；删除弹窗会再次说明影响范围。

### 3.4 邮箱池 `/mailboxes`

![邮箱池](./docs/screenshots/empty/03-mailboxes.jpg)

邮箱池是隐私邮箱的主要操作页面：

- 按邮箱地址、标签或备注搜索。
- 按 Apple 账号筛选；默认显示全部账号创建的邮箱。
- 按使用状态筛选。
- 同步一个账号或全部账号在 Apple 服务器上已有的隐私邮箱。
- 手动导入一个已有邮箱并绑定 Apple 账号。
- 点击邮箱文字自动复制完整邮箱地址，无需额外复制图标。
- “标签/备注”列将标签显示在上方、备注显示在下方并居中；点击备注可在小弹窗中修改或清空。
- 点击状态标签可在公共表单弹窗中选择并保存新状态。
- 查看邮箱详情、API/iCloud 开关和本地邮件。
- 手动同步邮件、获取验证码、清理远端邮件。
- 从 Apple 服务器彻底删除邮箱，或只删除本地记录。

列表操作按钮使用不同颜色区分同步、取码、详情和删除，并保留固定宽度。彻底删除任务执行时，当前邮箱显示“删除中”，后续已确认的邮箱显示“排队中”，两种状态均显示旋转进度图标且不会改变列表布局。

点击“详情”后会显示遮罩弹窗。弹窗中的本地邮件使用固定高度列表；点击邮件可在第二层弹窗查看完整主题、发件人、时间和正文，不会挤压详情页底部操作区。

“获取验证码”会先查看最近 5 分钟的本地缓存，再按需触发账号级邮件同步。后台 IMAP 监听与手动取码共用本地消息缓存和账号级同步锁，监听到的邮件会写入邮箱池，手动取码仍可从缓存读取验证码。

远端删除与本地删除的区别：

- 彻底删除：查询 Apple 列表、调用远端删除、再次确认远端不存在，然后删除本地记录。
- 只删除本地：保留 Apple 服务器上的邮箱，只清除本地邮箱和本地邮件。

列表中的每次彻底删除都需要单独二次确认，并明确显示目标邮箱。连续确认多个邮箱时，前端会加入串行队列，避免同时请求 Apple；顶部中央提示在队列结束前持续显示当前邮箱、等待数量和完成统计，结束后再显示成功/失败结果。删除失败会保留对应本地记录，并在最终提示中显示最近一次错误。

### 3.5 创建隐私邮箱 `/tasks`

![创建隐私邮箱](./docs/screenshots/empty/04-tasks.jpg)

页面支持“创建一个”和“自动创建”两种执行方式：

- 创建一个：默认选择第一个可用 Apple 账号，点击后立即创建一次。
- 自动创建：选择一个或多个账号，按下一轮间隔持续创建；账号之间按账号间隔依次执行。

创建通道默认是“自动接口：新接口优先，失败用旧接口”。也可以固定使用 Apple Account 新接口或 iCloud Web 旧接口。

标签输入的是前缀。留空时默认使用 `x`，服务会检查本地已有标签并生成连续编号，例如 `x_1`、`x_2`、`x_3`。

右上角设置按钮用于保存创建默认值：

- 默认标签前缀和备注。
- 默认参与账号。
- 单次创建与自动创建的通道。
- 下一轮间隔，默认 60 分钟。
- 账号间隔，默认 5 秒。
- Apple Account 与 iCloud Web 的两步验证方式。

任务概览显示执行方式、参与账号、成功/失败数量、创建通道和执行时间。调度日志固定显示两条高度，更多记录在区域内部滚动。

### 3.6 本地导出 `/exports`

![本地导出](./docs/screenshots/empty/05-exports.jpg)

导出页面提供四种常用下载：

| 导出项 | 格式 | 内容 |
| --- | --- | --- |
| 运行数据 | JSON | Apple 账号、邮箱、登录态和设置，不含邮件正文 |
| 运行数据与邮件 | JSON | 完整运行数据和所有本地邮件 |
| 邮箱地址 | TXT | 每行一个隐私邮箱地址 |
| 取码 API | TXT | 邮箱地址及其独立取码链接 |

邮箱和 API 导出接口还支持 `csv`、`tsv`、`jsonl`。运行数据含 Apple Cookie、登录态、IMAP App 专用密码和邮箱 API Token，应保存在可信位置。

### 3.7 系统设置 `/settings`

![系统设置](./docs/screenshots/empty/06-settings.jpg)

系统设置包含以下区域：

1. 本地数据：邮箱池每页数量、当前状态文件路径。
2. 后台能力：IMAP 实时邮件监听、Apple 登录态保活。
3. 公共访问：公共取号 API、公共验证码页面、公共 API Key。
4. 版本与更新：当前版本、构建提交、运行平台、检查时间和 GitHub 最新内容。

“生成新 Key”使用浏览器加密随机数生成公共 API Key。系统设置中的 Key 优先于 `config.json` 的备用 `api_key`。公共取号、批量查询和带全局 Key 的邮箱取码需要该 Key；公共验证码页面有独立开关，不读取这个 Key。

页面中的开关只在底层能力由 `config.json` 启用时生效。例如 `mail_watcher_enabled=false` 时，IMAP 后台监听能力保持停用。

左侧导航底部的版本号可以直接跳到“版本与更新”。顶部铃铛是公告中心，默认显示最新 5 条并在内部滚动；点击公告使用带遮罩的详情弹窗。项目公告来自 `internal/updatecheck/announcements.json`，GitHub Release 或新的默认分支提交也会自动生成版本公告。所有远端正文均按纯文本显示。

### 3.8 公共验证码 `/verification-code`

![公共验证码页面](./docs/screenshots/empty/07-verification-code.jpg)

公共验证码页面向外部用户，使用独立布局，没有后台导航和后台入口。开启方式：

1. 管理员进入“系统设置”。
2. 打开“公共验证码页面”。
3. 保存系统设置。
4. 外部用户访问 `/verification-code`，输入完整邮箱地址并获取验证码。

页面只显示当前邮箱的最新匹配验证码、发件人、邮件主题和接收时间。当前实现默认匹配 OpenAI 验证码，并限制在最近 5 分钟窗口内。

## 4. 常用操作流程

### 4.1 从零开始建立邮箱池

```text
首次创建管理员
  → 添加 Apple 账号并完成两步验证
  → 保存 IMAP App 专用密码
  → 检测三类登录态
  → 同步 Apple 已有隐私邮箱或创建新邮箱
  → 在邮箱池查看邮件和验证码
```

### 4.2 自动创建邮箱

```text
进入“创建隐私邮箱”
  → 选择“自动创建”
  → 选择参与账号
  → 保持自动接口或指定接口
  → 打开右上角设置并确认间隔
  → 启动自动创建
  → 在任务概览和调度日志中查看结果
```

### 4.3 开放公共取号 API

1. 在系统设置生成并保存公共 API Key。
2. 打开“公共取号 API”。
3. 公网部署时配置 `public_base_url`、HTTPS、`secure_cookie=true` 和反向代理。
4. 调用方通过 `X-API-Key` 或 `Authorization: Bearer <KEY>` 提交 Key。

领取一个可用邮箱：

```bash
curl -X POST 'https://HOST/api/v1/mailboxes/claim' \
  -H 'Content-Type: application/json' \
  -H 'X-API-Key: TOKEN' \
  -d '{"project":"PROJECT","purpose":"PURPOSE"}'
```

按邮箱独立 Token 取码：

```bash
curl 'https://HOST/api/v1/mailboxes/EMAIL/code' \
  -H 'Authorization: Bearer TOKEN'
```

生产调用推荐把 Key 放在请求头中，减少浏览器历史、访问日志和代理日志记录 URL 参数的机会。

## 5. HTTP 接口

### 5.1 登录与控制台

```text
GET  /api/health
GET  /api/auth/status
POST /api/auth/setup
POST /api/auth/login
POST /api/auth/logout
GET  /api/dashboard
GET  /api/events
POST /api/events/clear
```

### 5.2 Apple 账号

```text
GET    /api/apple-accounts
GET    /api/apple-accounts/{id}
DELETE /api/apple-accounts/{id}
POST   /api/apple-accounts/login/start
POST   /api/apple-accounts/login/2fa
POST   /api/apple-accounts/{id}/check
POST   /api/apple-accounts/{id}/imap
POST   /api/apple-accounts/{id}/mailboxes
POST   /api/apple-accounts/{id}/mailboxes/sync
```

### 5.3 邮箱池

```text
GET    /api/mailboxes?q=&status=&account_id=&page=&page_size=
POST   /api/mailboxes
POST   /api/mailboxes/remote-clean
GET    /api/mailboxes/{id}
POST   /api/mailboxes/{id}/status
POST   /api/mailboxes/{id}/sync
POST   /api/mailboxes/{id}/remote-clean
DELETE /api/mailboxes/{id}
DELETE /api/mailboxes/{id}?local_only=1
GET    /api/mailboxes/{id}/messages
GET    /api/mailboxes/{id}/code
```

### 5.4 创建、设置和导出

```text
GET  /api/tasks
GET  /api/settings
PUT  /api/settings
GET  /api/update/status
GET  /api/create-settings
PUT  /api/create-settings
GET  /api/scheduler/status
POST /api/scheduler/start
POST /api/scheduler/stop
POST /api/scheduler/logs/clear
GET  /api/runtime/export
GET  /api/runtime/export?include_messages=1
GET  /api/runtime/export-mailbox-apis?format=txt
GET  /api/runtime/export-mailbox-emails?format=txt
```

### 5.5 公共接口

```text
GET  /api/v1/health
POST /api/v1/mailboxes/claim
POST /api/v1/mailboxes/lookup
GET  /api/v1/mailboxes/{email}/code
GET  /api/v1/public-code/status
GET  /api/v1/public-code?email={email}
```

邮箱取码支持以下查询参数：

| 参数 | 说明 |
| --- | --- |
| `after` | RFC3339 起始时间 |
| `keyword` | 邮件匹配关键词，默认 `OpenAI` |
| `wait_ms` | 等待新邮件的毫秒数，最大 30000 |
| `allow_stale` | 同步失败时允许读取本地缓存 |
| `cache` | 只读本地缓存 |
| `peek` / `preview` | 预览验证码，不标记为已发放 |

## 6. 配置文件

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `host` | `127.0.0.1` | HTTP 监听地址 |
| `port` | `8788` | HTTP 监听端口 |
| `data_path` | `data/state.json` | 本地状态文件 |
| `session_ttl_hours` | `168` | 管理员会话有效小时数 |
| `secure_cookie` | `false` | HTTPS 部署时设为 `true` |
| `api_key` | 空 | 公共接口备用全局 Key |
| `public_base_url` | 空 | 对外生成取码链接时使用的基础地址 |
| `icloud_default_host` | `www.icloud.com.cn` | 默认 iCloud 区域主机 |
| `icloud_client_id` | 内置默认值 | iCloud Web 协议客户端 ID |
| `apple_account_api_key` | 空 | Apple Account 管理接口备用 API Key |
| `apple_account_keep_alive_enabled` | `true` | 是否加载登录态保活能力 |
| `apple_account_keep_alive_ms` | `180000` | 保活基础间隔；后台每 30 秒扫描，每轮重新生成随机间隔 |
| `apple_account_keep_alive_jitter_percent` | `15` | 保活随机浮动比例 |
| `mail_watcher_enabled` | `true` | 是否加载 IMAP 监听能力 |
| `mail_watcher_poll_ms` | `3000` | 监听分组重检间隔 |
| `mail_watcher_fetch_limit` | `8` | 常规同步每账号最多拉取邮件数 |
| `mail_watcher_initial_fetch_limit` | `20` | 首次监听最多拉取邮件数 |
| `mail_watcher_lookback_hours` | `24` | 首次监听回看小时数 |
| `public_fast_sync_wait_ms` | `600` | 公共取码快速同步等待时间 |
| `public_sync_min_interval_ms` | `3000` | 同一邮箱公共同步最短间隔 |
| `update_enabled` | `true` | 是否启用 GitHub 版本与项目公告检查 |
| `update_repository` | `xiuxiu56/iCloud-Privacy-Mail-v2` | 检查 Release、默认分支提交和公告文件的仓库 |

`config.json` 和 `data/` 已加入 `.gitignore`。如果配置文件中写入 API Key，应把文件权限收紧为仅当前用户可读：

```bash
chmod 600 config.json
```

## 7. 数据存储与迁移

状态默认保存在 `data/state.json`，schema 版本为 3，主要内容包括：

- 管理员密码摘要和控制台会话摘要。
- Apple 账号、iCloud Web Cookie、Apple Account 登录态。
- IMAP 邮箱、App 专用密码和同步游标。
- 隐私邮箱、邮箱 API Token、使用状态和备注。
- 本地邮件、最近验证码、系统事件和设置。

保存时使用同目录临时文件、`fsync` 和原子重命名；状态文件权限设为 `0600`，目录权限设为 `0700`。

从参考项目状态迁移：

```bash
./bin/ipm-migrate \
  -source /PATH/TO/OLD/data/state.json \
  -target data/state.json
```

目标文件已经存在时，需要显式增加 `-force`。迁移前建议同时备份旧状态文件和目标状态文件。旧控制台 Web 会话不会迁移，迁移后需要重新登录。

## 8. 项目结构

```text
iCloud-Privacy-Mail-v2/
├── main.go                         # 服务主入口、启动菜单、HTTP 生命周期
├── go.mod
├── config.example.json             # 配置模板
├── cmd/
│   └── migrate/main.go             # 旧状态迁移命令
├── internal/
│   ├── auth/service.go             # 单管理员、密码摘要、控制台会话
│   ├── buildinfo/buildinfo.go      # 二进制版本、提交、构建时间和平台
│   ├── config/config.go            # 默认配置与 JSON 加载
│   ├── domain/model.go             # schema 3 领域模型
│   ├── apple/service.go            # Apple 登录、2FA、检测、IMAP 和保活
│   ├── mailbox/service.go          # 邮箱创建、同步、取码、清理和删除
│   ├── mailwatcher/service.go      # IMAP IDLE、批量同步和重连
│   ├── scheduler/service.go        # 自动创建调度与内存日志
│   ├── protocol/                   # Apple SRP、iCloud、IMAP 协议客户端
│   ├── store/                      # 并发安全的 JSON 状态存储
│   ├── updatecheck/                # GitHub Release、提交和公告检查
│   ├── httpapi/                    # 管理接口、公共接口和导出接口
│   └── webui/                      # Go embed 前端构建产物
├── frontend/
│   ├── src/components/             # 选择器、确认弹窗、侧栏、提示等
│   ├── src/layouts/                # 后台主布局
│   ├── src/views/                  # 登录及各业务页面
│   └── src/router/                 # Vue Router 与登录守卫
├── scripts/
│   ├── build.sh                    # 完整构建
│   ├── dev.sh                      # 前后端开发模式
│   └── sync-web.sh                 # 同步前端产物到 Go embed
├── docs/
│   ├── screenshots/empty/          # 空数据页面截图
│   └── 07-代码与安全审计.md        # 最新审计报告
├── PROJECT_GUIDE.md                # 本文档
├── README.md
└── REFACTOR_SUMMARY.md
```

## 9. 技术架构

### 9.1 技术栈

| 层级 | 技术 | 作用 |
| --- | --- | --- |
| 后端 | Go 1.25+、标准库 `net/http` | API、Cookie、超时、后台任务和静态资源服务 |
| 存储 | 本地 JSON、原子替换 | 单机状态保存和迁移 |
| Apple 协议 | Go 自研客户端 | SRP、2FA、Apple Account、iCloud Web、HME |
| 邮件 | IMAP over TLS、iCloud Web 邮件接口 | 邮件同步、IDLE 监听和远端清理 |
| 前端 | Vue 3、Vue Router | 页面组件、路由守卫和响应式状态 |
| 样式 | Tailwind CSS 4 | 亮色/暗色、响应式布局和统一组件样式 |
| 图标 | Lucide Vue | 页面图标 |
| 构建 | Vite 8、Go embed | 生产前端构建和单服务运行 |

Go 模块当前只使用标准库。前端生产依赖为 Vue、Vue Router 和 Lucide Vue。

### 9.2 运行关系

```text
浏览器 Vue 页面
  │
  ├── 管理 API ── Cookie ── auth.Service
  ├── 公共 API ── API Key / 邮箱 Token
  │
  ▼
httpapi.Server
  ├── apple.Service ───── protocol.AuthFacade / ICloudClient
  ├── mailbox.Service ─── protocol.IMAP / ICloudClient
  ├── mailwatcher.Service ── IMAP IDLE / 批量同步
  ├── scheduler.Service ──── 周期创建
  └── store.Store ────────── data/state.json
```

同一个 Apple 账号的邮件同步会合并为一次执行，然后把邮件按收件地址分发到该账号的隐私邮箱，减少逐邮箱重复登录和 UID 游标互相覆盖。

## 10. 本地运行与公网部署

本地使用建议保持：

- `host=127.0.0.1`
- `secure_cookie=false`
- 公共取号和公共验证码开关保持关闭，按需临时开启。
- 定期备份 `data/state.json`。

公网部署前应完成以下工作：

1. 使用 HTTPS 反向代理并设置 `secure_cookie=true`。
2. 固定允许的 Host 和对外 `public_base_url`。
3. 为登录、公共取号和公共验证码加入速率限制。
4. 为管理写接口加入 Origin/CSRF 校验。
5. 为公共验证码页面增加访问密钥或一次性授权，并统一错误响应。
6. 使用系统密钥环或主密钥加密 Apple Cookie、IMAP App 专用密码和 API Key。
7. 给敏感导出响应增加 `Cache-Control: no-store`，并限制代理缓存。
8. 配置状态备份、日志保留、并发上限和服务监控。

完整问题分级、代码证据和修复顺序见 [代码与安全审计](docs/07-代码与安全审计.md)。

## 11. 相关文档

- [代码与安全审计](docs/07-代码与安全审计.md)
- [现状分析](docs/01-现状分析.md)
- [架构说明](docs/02-重构架构.md)
- [功能取舍](docs/03-功能取舍.md)
- [迁移指南](docs/04-迁移指南.md)
- [项目结构](docs/05-项目结构.md)
- [历史缺口审计](docs/06-缺口审计.md)
- [参考项目](https://github.com/q1953258942/iCloud-Privacy-Mail)
