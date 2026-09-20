<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="brand/logos/lockup-white.svg">
    <img src="brand/logos/lockup-black.svg" alt="SubLane" width="420">
  </picture>
</h1>

<p align="center">
  面向内部团队的极简、自托管订阅网关，优先支持 Codex。
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0--only-171717?style=flat-square&amp;labelColor=555555" alt="License: AGPL-3.0-only" height="20"></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.26%2B-171717?style=flat-square&amp;logo=go&amp;logoColor=white&amp;labelColor=555555" alt="Go 1.26+" height="20"></a>
  <a href="web/package.json"><img src="https://img.shields.io/badge/React-19-171717?style=flat-square&amp;logo=react&amp;logoColor=white&amp;labelColor=555555" alt="React 19" height="20"></a>
  <a href="internal/storage/storage.go"><img src="https://img.shields.io/badge/SQLite-embedded-171717?style=flat-square&amp;logo=sqlite&amp;logoColor=white&amp;labelColor=555555" alt="SQLite embedded" height="20"></a>
  <a href="docs/roadmap.md"><img src="https://img.shields.io/badge/stage-foundation-92400E?style=flat-square&amp;labelColor=555555" alt="Stage: foundation" height="20"></a>
</p>

<p align="center">
  <a href="README.md">English</a> · <a href="https://github.com/murongg/SubLane">项目仓库</a> · <a href="brand/README.zh-CN.md">品牌物料</a>
</p>

**当前处于开发初期。** 已提供管理员初始化、成员账号创建与启停、角色权限控制、个人 API 密钥、登录退出、SQLite 持久化、内嵌前端、中英文切换和明暗主题。已实现 Codex、Claude、Antigravity OAuth／凭据 JSON 导入、凭据加密和 HTTP/SSE/WebSocket 转发，并通过模拟上游测试。真实订阅和桌面端仍待验收。已提供密码管理、成员请求限制和个人／团队用量汇总。

后端使用 Go、chi 路由、sqlc 生成的类型安全查询和纯 Go SQLite 驱动。sqlc 只用于开发时生成代码，生产部署仍是单个进程。

## 开始开发

需要 Go 1.26+、Node.js 24、pnpm 9.12.2。

```sh
git clone git@github.com:murongg/SubLane.git
cd SubLane
make setup
make dev
```

打开 http://127.0.0.1:5173，后端位于 http://127.0.0.1:8080。前端通过 Vite 代理调用后端。Ctrl+C 同时停止两个进程；修改 Go 代码后需重新启动。

如果 5173 端口已被占用，可使用 `make dev WEB_PORT=5174`。

首次启动后，在欢迎页点击「开始设置」，填写用户名、密码和确认密码，创建管理员后自动进入工作空间。用户名为 3–32 个英文字母、数字、下划线或连字符；密码为 8–20 个字符。请先在本机完成初始化，再开放给其他人访问。初始化完成后入口关闭，重启不会重新开放。

## 构建运行

```sh
make build
./bin/sublane
```

打开 http://127.0.0.1:8080。构建后为包含前端的单个二进制文件，运行时不需要 Node.js。数据库默认保存在 `./data/sublane.db`，订阅凭据加密密钥为 `./data/credentials.key`，备份时需要一起保存。

Docker：`docker compose up --build -d`。容器使用命名卷保存数据库，宿主端口仅绑定本机。

Docker 部署同样通过页面创建首个管理员。网络部署应使用 HTTPS 反向代理，并在容器环境中设置 `SUBLANE_PUBLIC_URL` 为外部访问地址。会话有效期为 12 小时；退出会撤销当前会话。详见[认证说明](docs/authentication.md)。

## 接入订阅账号

管理员在「订阅账号」中选择 Codex、Claude 或 Antigravity，再选择浏览器授权或导入凭据 JSON，验证连接后，成员即可使用个人 API 密钥接入客户端。OAuth 授权后需将浏览器地址栏的 localhost 回调地址粘贴回页面；此时 localhost 页面无法打开是预期行为。密钥页面提供客户端配置。模型列表直接返回原始模型名，无需添加服务商前缀；系统按分组权限和模型支持情况选择可用账号，同名模型只显示一次。历史带前缀的调用仍然兼容。订阅额度目前仅支持 Codex。详细边界见 [多服务商说明](docs/providers.md) 和 [Codex 接入说明](docs/codex.md)。

## 检查

管理员可在「成员管理」中创建、启用或停用成员。成员登录后只显示个人工作空间，不能访问订阅账号、成员管理或系统状态；外观和语言设置对两种角色均可用。详见[成员与权限说明](docs/members.md)。

运行 `make check`，执行 sqlc 生成代码一致性检查、后端检查与竞态测试、前端类型与格式检查、单元测试及生产构建。修改 SQL 查询或迁移文件后运行 `make generate`。CI 还会构建 Docker 镜像。

界面默认英文，在顶部或偏好设置中切换中英文、明暗主题，偏好保存在当前浏览器。`PRODUCT.md` 和主开发文档使用英文。

仓库仅保留必要的品牌资源。运行 `make brand` 可将完整物料导出到 Git 忽略的 `dist/brand/`，详见[品牌指南](brand/README.zh-CN.md)。

配置变量、目录结构、接口和当前边界见 [英文 README](README.md)。配置通过进程环境变量传入，服务不会自动读取 `.env`。

## 协议

使用 [AGPL-3.0-only](LICENSE)，保留 [Shadcn Admin 等第三方的许可声明](THIRD_PARTY_NOTICES.md)。

## 账号分组

管理员可以创建账号池分组，并在成员管理中分配可用分组。个人 API 密钥绑定一个分组，请求只使用组内账号。已有账号和密钥自动迁移到默认组。账号可跨组共享；如需独占使用，应从默认组和其他分组中移出。详见 [分组说明](docs/groups.md)。

## 账号池运行状态

账号支持单账号并发限制、持久化冷却和请求驱动的恢复。每位用户可查看自己的请求记录，管理员还可查看全站请求的结果、耗时和已报告的 Token 用量，不记录提示词、响应正文或凭据。详见 [账号池运行说明](docs/pool-runtime.md) 与 [团队管理说明](docs/team-controls.md)。

## 备份与恢复

管理员可以在「系统设置 → 备份与恢复」下载备份、上传校验并生成独立恢复目录，再重启切换。命令行提供 `backup`、`backup verify` 和 `restore`；备份包含数据库和凭据加密密钥，请保存在私有位置。用法及 Docker 操作见 [备份与恢复](docs/backup.md)。
