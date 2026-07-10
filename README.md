# BangumiPipeline

BangumiPipeline 是一个面向动画订阅、Bangumi 元数据刮削抓取、下载和媒体转码处理的流水线单体项目。

用于解决传统需要搭建多个项目来碎片化实现番剧订阅、下载、转码和观看的难于维护的问题，提供统一的管理端和观看端。

**半成品项目，目前还在开发中，大部分功能都未实现**

## 环境要求

- Go 1.25 或更高版本
- Node.js 24 或兼容版本
- npm 11 或兼容版本

## 测试与构建

```powershell
npm test
npm run build:server
```

快速验证前端构建：

```powershell
npm run build:ui
```

## Docker

```powershell
docker compose up --build
```

部署到 HTTPS 环境时，应在反向代理后设置 `BP_COOKIE_SECURE=true`。不要直接将管理端暴露到公网，建议使用 Caddy 或 Nginx 提供 TLS 和访问控制。

观看端的追番更新通知采用浏览器标准 Web Push：VAPID 密钥会首次使用时自动生成并保存在 SQLite。该功能需要 HTTPS（`localhost` 开发环境除外）；用户首次登录进入观看端或点击“追番”时会由浏览器请求通知权限。

## 配置

| 环境变量 | 默认值 | 用途 |
| --- | --- | --- |
| `BP_ADMIN_ADDR` | `:8080` | 管理端监听地址 |
| `BP_VIEWER_ADDR` | `:8090` | 观看端监听地址 |
| `BP_DATABASE_PATH` | `./data/bangumi-pipeline.db` | SQLite 文件路径 |
| `BP_ADMIN_WEB_DIR` | `./frontend/apps/admin/dist` | 管理端静态文件目录 |
| `BP_VIEWER_WEB_DIR` | `./frontend/apps/viewer/dist` | 观看端静态文件目录 |
| `BP_COVER_DIR` | `./data/images/bangumi` | Bangumi 大图与角色/声优图片保存目录 |
| `BP_BANGUMI_API_URL` | `https://api.bgm.tv` | Bangumi API 地址 |
| `BP_BANGUMI_USER_AGENT` | 项目默认值 | Bangumi API 请求 User-Agent，部署时应覆盖 |
| `BP_WEB_PUSH_CONTACT_EMAIL` | `noreply@localhost` | Web Push VAPID 联系邮箱，可按部署域名修改 |
| `BP_COOKIE_SECURE` | `false` | 是否仅通过 HTTPS 发送登录 Cookie |
