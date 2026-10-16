# HTTP API 与鉴权

代码路径相对仓库根目录。完整路由与请求/响应类型以 Handler 和各端 `api.ts` 为准；本文保留边界与入口。

## 装配与错误处理

- Admin 路由集中在 `backend/internal/httpapi/admin.go` 的 `NewAdminHandler`，Viewer 在 `viewer.go` 的 `NewViewerHandler`。
- 业务逻辑调用现有 Service，错误使用 `writeError`，格式为 `{ "error": { "code": "...", "message": "..." } }`。
- 路径 ID 用 `parsePathID`；前端用 `request<T>` 并抛出 `APIError`。
- Admin 普通 GET 使用 15 秒上下文期限，SSE 流不套用；读请求保持 Admin/Viewer Reader 上下文标记。

## 管理端边界

- 除健康检查、初始化与登录外，管理功能用 `requireAdministrator`，不能复用 Viewer Session。
- “前端管理”的用户、邀请、站点、评论过滤、轮播、筛选、专题页与 APP 发布都属于 Admin API。
- 日志 SSE 与管理员元数据图片同样要求管理员登录。
- 涉及下载撤销、绑定编辑/解除、单话替换、存储移动时保留 Service 的状态检查与文件操作保护。

| 功能 | 路由入口 | 业务文档 |
| --- | --- | --- |
| 设置、概览、任务、日志 | `/api/settings/*`、`/api/dashboard/overview`、`/api/scheduled-tasks`、`/api/system-logs` | [backend.md](backend.md) |
| 番剧设置、刷新 | `/api/anime/{bangumiID}/settings`、`/refresh` | [bangumi.md](bangumi.md) |
| 历史/单话补源、绑定编辑/解除 | `/sync-history`、`/sync-episode`、`/bound-episodes` | [subscription.md](subscription.md) |
| 下载重试、磁力重试、撤销 | `/api/download/jobs/{jobID}/retry`、`/retry-magnet`、`/cancel` | [download.md](download.md) |
| 媒体重试、存储移动 | `/api/media/jobs/{jobID}/retry`、`/api/anime/{bangumiID}/storage` | [media.md](media.md) |
| Viewer 配置与 APP 发布 | `/api/viewer/*`，专题为 `/single-pages` | [admin.md](admin.md) |

管理员 `POST /api/download/jobs/{jobID}/retry-magnet` 接受可选 `magnetUrl`。没有提供且无法从本地订阅信息提取时返回 `422 download_magnet_required`，用于打开填写弹窗；链接无效为 `400 invalid_magnet_url`，任务不允许重试为 `409 download_retry_not_allowed`。成功返回 `{ "result": { "job": ..., "action": "magnet_reset" } }`，任务状态为 `pending`，不会立即绕过调度器下载。

## 观看端鉴权

- 仅需确认登录用 `requireViewer`；需要当前用户 ID 用 `authenticatedViewer`。
- 身份按 `readViewerSessionToken` 解析：Bearer token、Viewer Cookie、媒体 URL 的 `viewer_token`；移动端不使用管理员 Session。
- 历史、追番、邀请、推送订阅和密码修改的用户身份从服务端认证取得。
- CORS / OPTIONS 预检沿用 `viewerCORSMiddleware`，预检放行不等于业务接口免鉴权。
- 公开入口包括健康检查、站点设置、favicon、注册/登录，以及以下 APP 接口：
  - `GET /api/app/releases/latest`
  - `GET /api/app/releases/{releaseID}/download`
- 注册仍在后端检查注册开关与邀请码；退出接口按现有实现清理所提供 token 和 Cookie。
- `/app/download` 在前端入口绕过登录门禁；内置季度专题仍在登录后的页面壳中显示。
- 评论过滤名单只通过 Admin API 管理，不进入公开 site-settings。

## 受控资源与用户业务

- 媒体流、封面、进度和评论同时校验番剧 ID 与媒体 ID，只接受已完成产物。
- 视频由 `http.ServeFile` 提供 Range 播放；图片/视频接口返回内容，不在 JSON 中返回服务器路径。
- 评论用有效媒体在服务端映射 Bangumi episode ID，不允许客户端直接枚举上游剧集。
- 表情、评论头像、角色和声优图片通过各自受控 ID 接口读取。
- Viewer `GET /api/anime/{bangumiID}/cover` 优先返回有效且不早于原图的 `.viewer.webp` 副本（`image/webp`）；缺失、失效或转换失败时回退原图。接口 URL、Viewer 鉴权与私有缓存策略不变，Web/已发布 APP 共用副本，管理端仍读取原图。请求中不触发转码。
- 进度写入由后端校验播放阈值和完播状态，规则见 [viewer.md](viewer.md)。

| 功能 | 入口 |
| --- | --- |
| 首页、季度、分类图书馆 | `/api/home`、`/api/anime-schedule`、`/api/library` |
| 详情、追番、历史 | `/api/anime/{bangumiID}/detail`、`/follow`、`/api/follows`、`/api/watch-history` |
| 流、封面、评论、进度 | `/api/anime/{bangumiID}/media/{mediaID}/*` |
| 自助邀请、改密码 | `/api/invitations`、`PUT /api/auth/password` |
| Web Push | `/api/push/config`、`/api/push/subscriptions` |

Viewer 契约变更同时检查 Web 和 APP 的 `src/api.ts`；调用端分别见 [viewer.md](viewer.md)、[mobile.md](mobile.md)。

- `GET /api/library` 的 `anime_type` 可为 `all`、`series`、`movie`，未传时默认 `series` 保持旧客户端兼容；`all` 仍应用标题/别名搜索、标签组合、软删除过滤、排序与分页。
- `GET /api/watch-history` 每项包含 `animeType` 和首播日期 `airDate`，供客户端区分电影与连载文案、显示电影首播日期；播放进度和媒体标识保持原契约。
