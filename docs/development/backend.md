# 后端装配、配置与运行

本文按需读取；代码路径相对仓库根目录。修改相关行为时同步更新本文。

## 入口与装配

- 入口为 `backend/cmd/server/main.go`，先打开 SQLite 并迁移，再装配日志、各 Service、Scheduler 与 HTTP server。
- 管理端默认 `:8080`，观看端默认 `:8090`，同一个进程服务两个端口。
- 路由在 `NewAdminHandler` / `NewViewerHandler` 注册，细节见 [http-api.md](http-api.md)。
- 计划任务在 `main.go` 注册，默认配置由数据库迁移初始化；新增任务同时检查两处。
- 媒体 Service 装配下载清理、元数据刷新、评论入队与新集推送回调，不能遗漏完成后的链路。

## 配置

主要环境变量在 `backend/internal/config/config.go`：

- `BP_ADMIN_ADDR`
- `BP_VIEWER_ADDR`
- `BP_DATABASE_PATH`
- `BP_ADMIN_WEB_DIR`
- `BP_VIEWER_WEB_DIR`
- `BP_COVER_DIR`，默认 `./data/images/bangumi`
- `BP_DOWNLOAD_DIR`，默认 `./data/downloads`
- `BP_MEDIA_DIR`，默认 `./data/bangumi`
- `BP_FFMPEG_PATH`，默认 `ffmpeg`
- `BP_FFPROBE_PATH`，默认 `ffprobe`
- `BP_BANGUMI_API_URL`
- `BP_BANGUMI_NEXT_API_URL`，默认 `https://next.bgm.tv`
- `BP_BANGUMI_USER_AGENT`
- `BP_COOKIE_SECURE`
- `BP_WEB_PUSH_CONTACT_EMAIL`：Web Push VAPID 联系邮箱，配置入口见 `backend/internal/config/config.go`。

规则：

- 不提交代理地址、账号、Cookie、Bangumi Token、qBittorrent 密码或 LLM API KEY。
- Bangumi 请求需要可识别 User-Agent，例如 `your-id/BangumiPipeline/0.1`。
- 额外媒体存储根目录、网络代理、下载设置、LLM 设置保存在数据库系统设置中。

## 调度器

计划任务由 `backend/internal/system/scheduler.go` 调度。

必须保持：

- 同一个任务不可并发执行。
- `Scheduler.Trigger` 检查内存 `running` map。
- `system.Service.MarkTaskStarted` 阻止数据库层面的重复运行。
- 不要在单个任务内部并发执行同类重型工作，除非同步更新并发保护策略。

当前任务（默认值用于首次初始化，运行设置以数据库为准）：

- `bangumi-season-metadata`：从 bangumi.tv 抓取新番元数据，默认 15 分钟，默认关闭。
- `subscription-rss-match`：抓取订阅和匹配番剧，默认 15 分钟，默认关闭。
- `download-bound-episodes`：下载番剧，默认 1 分钟，默认关闭。
- `process-downloaded-media`：处理和转码已下载完成的视频，默认 1 分钟，默认关闭。
- `translate-anime-metadata`：翻译新番元数据，默认 1 分钟，默认关闭。
- `sync-bangumi-episode-comments`：同步已有成品话数的 Bangumi 吐槽，默认 1 分钟，默认开启。
- `detect-media-openings`：Chromaprint 片头识别，默认 1440 分钟，默认关闭，见 [opskip.md](opskip.md)。
- `deliver-viewer-push-notifications`：投递追番新集 Web Push，默认 5 分钟，默认开启，见 [viewer.md](viewer.md)。

## 系统概览和日志

系统概览相关代码：

- `backend/internal/httpapi/admin.go`
- `backend/internal/httpapi/disk.go`
- `backend/internal/httpapi/disk_windows.go`
- `backend/internal/httpapi/disk_unix.go`
- `frontend/apps/admin/src/pages/DashboardPage.vue`

概览接口 `GET /api/dashboard/overview` 实时读取：

- 待绑定订阅数量。
- 下载 pending、downloading、failed 数量。
- 转码 pending、transcoding、failed 数量。
- 默认媒体目录和额外存储根的磁盘空间。

磁盘空间规则：

- 每次打开概览或刷新时实时探测。
- 不写入数据库缓存。
- 路径不存在时向上查找最近存在的父目录用于探测。
- Windows 使用 `GetDiskFreeSpaceEx`。
- 非 Windows 使用 `statfs`。

日志相关代码：`backend/internal/applog/`。

- 业务代码使用 `log/slog`。
- 日志必须带 `source`，例如 `bangumi`、`subscription`、`download`、`media`、`translation`、`scheduler`、`http`。
- 级别只使用 INFO、WARNING、ERROR。
- Go 中 WARNING 对应 `logger.Warn`。
- 系统日志页面读取最近最多 1000 行，并通过 SSE 实时追加。
- 管理 API、日志流、本地元数据图片都必须要求管理员登录。
