# 重构总结

> 更新日期：2026-08-15

## 目标

将 Apple 隐私邮箱管理能力拆分为清晰的协议、服务、存储、HTTP 和前端边界；保留单管理员登录，移除多用户与 `/manage`，并以 SQLite + SSE 作为唯一数据和实时更新方案。

## 已完成

1. Go 单体拆分为配置、领域模型、存储、认证、协议、Apple 服务、邮箱服务、后台监听、调度器、HTTP API 和嵌入式 Web UI。
2. Vue 3 + Vue Router 重建管理页面，完成紧凑侧栏、卡片、选择器、表格、响应式分页、弹窗和亮暗主题。
3. 单管理员首次设置与登录：PBKDF2-SHA256 密码摘要、随机会话 Token、数据库只保存 Token 的 SHA-256 摘要。
4. Apple Account、iCloud Web、2FA、IMAP、登录态检测、后台保活、邮箱创建与远端同步已接入。
5. 邮箱池支持 ID/复选框、点击邮箱复制、状态和备注快捷编辑、详情、取码、远端邮件清理与删除队列。
6. 全量 Apple 邮件清理按账号排队，扫描全部文件夹，按隐私邮箱总数展示进度；内部文件夹名转换为中文。
7. 彻底删除邮箱先清理目标邮箱远端邮件，再删除 Apple 云端邮箱，远端确认后才清理本地记录。
8. 自动创建支持多账号、轮次随机范围、账号随机范围、持久化日志和服务重启恢复；创建失败立即停止。
9. 公共 claim/lookup/取码和租约 `commit/release/renew/note` 已接入幂等状态机。
10. 运行数据、邮件、邮箱和取码 API 导出已接入 JSON、TXT、CSV、TSV、JSONL。
11. GitHub Release、默认分支提交、构建版本和公告中心已接入，自动替换程序继续后置。

## SQLite 数据基座

- `data/app.db` 是唯一业务数据源，不再读取或生成 `state.json`。
- schema 版本为 4，通过 `schema_migrations` 启动时自动升级。
- SQLite 使用 WAL、单连接写入、关系保护、busy timeout 和事务。
- 账号、邮箱、租约、邮件、事件和设置通过 SQL 增量读写，不保留全量内存 State。
- 同账号邮件与 UID 游标使用单事务批量提交。
- Apple Cookie、登录态、IMAP App 专用密码、邮箱 Token 和公共 API Key 使用 AES-GCM 加密。
- 系统设置提供数据库状态、完整性检查、在线备份与空间整理。
- 后台执行邮件保留清理、备份保留清理和 WAL checkpoint。

## SSE 实时更新

- `GET /api/realtime` 提供受登录保护的事件流。
- 数据变化先写入持久化 `change_log`，再通知浏览器。
- 事件使用递增 ID，支持 `Last-Event-ID` 断线回放。
- 无变化时发送心跳，前端按主题刷新相关页面。
- SSE 断开不影响普通 API 读写和 SQLite 持久化。

## 新版页面

- `/login`：首次设置或登录。
- `/`：统计、后台状态与紧凑运行记录。
- `/apple-accounts`：整行打开登录态详情、登录/2FA、检测、IMAP、创建和同步。
- `/mailboxes`：自动分页、多选、详情、取码、邮件清理和邮箱删除。
- `/tasks`：单次/自动创建、随机范围设置、任务概览和调度日志。
- `/exports`：运行数据、邮件、邮箱和取码 API 导出。
- `/settings`：SQLite 维护、公共访问、后台能力、API Key 和版本检查。
- `/verification-code`：公共验证码页面。

新版截图位于 `docs/screenshots/ui/`，使用隔离演示数据采集。

## 已验证

- `go vet ./...` 通过。
- `go test ./...` 通过。
- `go test -race ./...` 通过。
- `go mod verify` 通过。
- `npm audit --json`：98 个依赖，0 个漏洞。
- `npm audit --omit=dev --json`：0 个漏洞。
- 已用浏览器完成登录、控制台、Apple 账号、邮箱池、创建页、导出、设置和公共验证码页面巡检。

## 当前安全结论

本地绑定 `127.0.0.1` 的单管理员使用场景已形成完整闭环。公网部署前仍需完成 Host allowlist、限流、Origin/CSRF、公共验证码访问控制和 Apple 服务 URL allowlist。

本次 `govulncheck` 在 Go 1.26.5 标准库发现 5 条代码可达漏洞，均标记在 Go 1.26.6 修复，因此推荐升级 Go 后重新构建。人工审核还发现远端邮件删除使用邮箱子串匹配，存在相似地址误匹配风险，应优先改成解析收件人后的完整邮箱等值比较。

完整扫描数据与修复优先级见 [`docs/07-代码与安全审计.md`](docs/07-代码与安全审计.md)。
