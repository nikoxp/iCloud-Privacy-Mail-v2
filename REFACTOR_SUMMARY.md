# 重构总结

## 目标

在独立目录中重新组织 iCloud 隐私邮箱项目，保留本地登录保护和已有 JSON 数据的迁移能力，去掉多用户管理及 `/manage`，并完成 P0、P1、P2 的核心迁移。

## 已完成

1. 创建全新目录 `iCloud-Privacy-Mail-v2`，原项目未作为重构目标修改。
2. 把 Go 单体拆成配置、领域模型、存储、认证、协议、Apple 服务、邮箱服务、后台监听、调度器、HTTP API 和嵌入式 Web UI。
3. 用 Vue 3 + Vue Router 重建页面，借鉴 `vue_demo` 的布局语言，补上登录守卫、移动端侧栏、暗色模式和实际操作反馈。
4. 单管理员首次设置：管理员密码使用 PBKDF2-SHA256 保存，登录态使用随机 Token 的 SHA-256 摘要，Cookie 为 HttpOnly + SameSite Strict。
5. 状态迁移升级到 schema 3，保存 `create_settings`、owner 字段、同步 UID、验证码去重字段和 typed Apple 登录态；旧 Web 会话不迁移。
6. `/manage`、`/manage/` 固定返回 404；前端所有入口均位于默认控制台。
7. Apple Account / iCloud Web 登录、2FA、登录态检查、IMAP App 专用密码验证、隐私邮箱创建/同步/删除、手动导入和远端邮件清理已接入。
8. 根目录 `main.go` 是统一主入口；无参数运行时提供中文启动菜单，也支持配置、地址、端口和数据文件参数。`mailwatcher` 使用按 Apple 账号分组的批量同步、IMAP IDLE、UID 起点、主动取码唤醒和断线退避重连。
9. 公共健康、claim、lookup、邮箱取码、运行数据和邮箱文本导出已接入 API Key 校验；取码支持新鲜窗口、缓存读取、预览和并发同步合并。

## 页面

- `/login`：首次设置或登录。
- `/`：控制台统计、最近事件和模块入口。
- `/apple-accounts`：Apple 登录/2FA 弹窗、登录态检测、账号选择、顶部 IMAP 保存、创建和同步弹窗。
- `/mailboxes`：按单个或全部 Apple 账号同步已有隐私邮箱、搜索、详情、邮件、取码、状态、远端清理、远程删除和本地删除。
- `/tasks`：创建一个隐私邮箱、自动创建、创建默认值、任务概览和调度日志。
- `/exports`：运行数据、邮件、邮箱地址和取码 API 导出。
- `/settings`：邮箱分页、邮件监听、Apple 保活、公共访问开关和公共 API Key。
- `/verification-code`：面向外部用户的独立公共验证码页面。

## P0 / P1 / P2

### P0 数据基座

- `domain.SchemaVersion = 3`。
- `cmd/migrate` 会输出旧用户、登录态和孤立邮箱数量。
- 创建配置、owner、同步游标、验证码去重和 typed `ICloudSession` / `LoginState` 均保留。

### P1 个人使用闭环

- 协议层位于 `internal/protocol`，HTTP 层只调用 `internal/apple` 和 `internal/mailbox` 服务。
- 删除 Apple 隐私邮箱时按远端列表确认、停用后重试删除、再次列表确认，再清理本地记录。
- 邮件同步支持 iCloud Web 与 IMAP 增量游标；同一账号一次拉取后分发到全部隐私邮箱，避免逐邮箱推进 UID 造成漏信。
- 验证码读取使用 5 分钟新鲜窗口并保存已服务消息 ID，支持 `cache`、`peek/preview` 和主动唤醒后台监听。

### P2 运营能力

- `internal/mailwatcher` 按 Apple 账号分组，使用 IMAP IDLE 监听、周期重检、UID 起点和指数退避重连。
- `internal/scheduler` 提供周期创建、启动/停止和日志。
- `/api/v1` 公共接口支持全局 API Key；邮箱自身 API token 可直接取码。
- JSON、TXT、CSV、TSV、JSONL 导出已接入；更新功能保留配置但默认关闭。

## 关键决策

### 继续使用 Go

旧项目协议客户端、IMAP、调度器和 JSON 状态都已经是 Go。重构的主要问题是边界和页面耦合，不是语言性能，因此继续使用 Go 可以直接复用网络超时、并发和单二进制部署能力。

### 前端使用 Vue

`vue_demo` 已提供目标布局方向，Vue 3 的组件、路由和响应式状态足以覆盖本地控制台。前端使用 Vite 本地构建、Tailwind 插件和 Lucide 组件，离线运行更稳定。

### 不保留 `/manage`

原 `/manage` 同时承担用户管理和管理数据展示，与个人单管理员场景冲突。统计、邮箱和 Apple 入口已整合到默认控制台，公网部署时只增加登录策略和部署配置。

## 后续建议

1. 公网部署前增加状态文件加密、CSRF、登录失败限制、反向代理和备份说明。
2. 如需要再实现更新检查；当前 `update_enabled=false`。
3. 根据公网并发和实际邮箱数量继续观测 IMAP 限流，再决定是否拆分多进程任务队列。

## 验收状态

- Vue 生产构建、`go vet ./...`、`go build ./...` 均已通过。
- 已同步前端静态资源到 `internal/webui/dist`。
- 已用独立临时实例完成首次设置、登录保护、API Key 健康检查、参数校验、导出和 `/manage` 404 验收。
- 已使用旧项目状态迁移验证：Apple 账号 1 个、邮箱 88 个、邮件 22 封，原文件未修改。
- 输入框统一使用标签、图标、输入区域和帮助文字，图标与底部文字保持独立间距。
- 按要求没有新增测试代码。

## 最新文档

- 完整使用、页面截图、接口、配置和技术结构见 `PROJECT_GUIDE.md`。
- 2026-08-10 功能缺口、安全风险、扫描结果和修复顺序见 `docs/07-代码与安全审计.md`。
