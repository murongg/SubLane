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
  <a href="https://sublane.dev/docs/zh/roadmap"><img src="https://img.shields.io/badge/stage-internal%20testing-92400E?style=flat-square&amp;labelColor=555555" alt="Stage: internal testing" height="20"></a>
</p>

<p align="center">
  <a href="README.md">English</a> · <a href="https://github.com/murongg/SubLane">项目仓库</a> · <a href="brand/README.zh-CN.md">品牌物料</a>
</p>

SubLane 将团队的 AI 订阅账号整合为统一、自托管的网关，采用 Go + SQLite，内置 Web 管理界面，单进程运行。

## 功能

- 支持 Codex、Claude、Antigravity 账号，通过 OAuth 授权或导入凭据接入。
- 提供 OpenAI 兼容 API、[Claude Messages 和 Gemini 原生生成接口](https://sublane.dev/docs/zh/protocols)，支持 HTTP/SSE 及 Responses WebSocket。
- 成员管理、个人 API 密钥和账号分组权限。
- 账号池调度、并发限制、冷却恢复和 Codex 额度感知。
- 请求诊断、用量图表及备份恢复。
- 中英文界面，支持明暗主题。

## 快速开始

安装 Docker、Compose、curl 和 jq 后，运行：

```sh
curl -fsSL https://raw.githubusercontent.com/murongg/SubLane/main/scripts/install.sh | bash
```

打开 http://127.0.0.1:8080，创建管理员、接入订阅账号，并将账号加入账号池，再创建个人密钥接入客户端。先按[第一次使用](https://sublane.dev/docs/zh/quickstart)跑通一次调用；默认自由使用，团队共享和额度限制之后再按需设置。

脚本会在 `./sublane` 安装最新正式版，没有正式版时使用最新预发布版。数据保存在 Docker 数据卷中，不会覆盖已有目录。自定义参数、手动 Compose 部署、HTTPS 和升级步骤见[部署文档](https://sublane.dev/docs/zh/deployment)。

## 本地开发

需要 Go 1.26+、Node.js 24、pnpm 9.12.2。

```sh
git clone https://github.com/murongg/SubLane.git
cd SubLane
make setup
make dev
```

打开 http://127.0.0.1:5173，运行 `make check` 执行检查、测试和构建。

## 文档

从[分步入门教程](https://sublane.dev/docs/zh/guide/installation)开始。使用文档统一维护在[文档仓库](https://github.com/murongg/sublane-website)。

- [部署与升级](https://sublane.dev/docs/zh/deployment) · [备份恢复](https://sublane.dev/docs/zh/backup)
- [服务商接入](https://sublane.dev/docs/zh/providers) · [API 密钥](https://sublane.dev/docs/zh/api-keys) · [账号分组](https://sublane.dev/docs/zh/groups)
- [开发指南](https://sublane.dev/docs/zh/development) · [系统架构](https://sublane.dev/docs/zh/architecture)
- [发版说明](https://sublane.dev/docs/zh/releases) · [更新日志](CHANGELOG.md)
- [参与贡献](CONTRIBUTING.md) · [安全说明](SECURITY.md)

## 协议

使用 [AGPL-3.0-only](LICENSE)。依赖和改编组件的许可见[第三方声明](THIRD_PARTY_NOTICES.md)。
