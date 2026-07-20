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
| `BP_BANGUMI_NEXT_API_URL` | `https://next.bgm.tv` | Bangumi 新版站点 API 地址，用于抓取剧集吐槽 |
| `BP_BANGUMI_USER_AGENT` | 项目默认值 | Bangumi API 请求 User-Agent，部署时应覆盖 |
| `BP_WEB_PUSH_CONTACT_EMAIL` | `noreply@localhost` | Web Push VAPID 联系邮箱，可按部署域名修改 |
| `BP_COOKIE_SECURE` | `false` | 是否仅通过 HTTPS 发送登录 Cookie |

## Bangumi 评论表情资源

首次执行“同步 Bangumi 剧集吐槽”计划任务时，后端会检查并下载评论表情到
`BP_COVER_DIR/smiles`（默认 `./data/images/bangumi/smiles`）。图片保留上游 GIF/PNG
原格式，`manifest.json` 负责把 `(bgm24)`、`(musume_06)` 等评论代码绑定到确定的
本地文件名和 Content-Type。运行数据目录已被 Git 忽略，不会提交这些图片。

当前目录包含 428 个上游实际存在的资源；Bangumi 官方富文本编辑器未实现且资源返回
404 的 `(musume_97)`、`(musume_98)` 会固定跳过。完整清单在后续任务中只做本地校验，
文件缺失时仅补抓缺失项。计划任务自动下载时会使用系统设置中保存的 HTTP/HTTPS 代理。

也可以在仓库根目录手动执行同步脚本：

```powershell
.\scripts\sync-bangumi-smiles.ps1 `
  -HttpProxy "http://localhost:10808" `
  -HttpsProxy "http://localhost:10808"
```

观看端番剧播放页右侧支持在“选集 / 评论”间切换。评论按当前成品媒体安全映射到
Bangumi 剧集 ID，展示头像、昵称、签名、评论时间和楼中楼回复；详情接口按本地已抓取的主楼评论分组统计每话评论数，因此加载和切换话数时会立即显示，切换话数也会重新加载正文。
评论图片支持 `[img]URL[/img]` 和带尺寸提示的 `[img=宽,高]URL[/img]`，并统一受评论区最大显示尺寸约束。
正文以结构化节点渲染删除线、剧透遮罩、外链图片及本地 Bangumi 表情，不使用
`v-html`。未知标签、缺失表情与非 HTTP/HTTPS 图片地址会被过滤。
