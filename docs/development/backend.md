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
- `backend/internal/httpapi/dashboard.go`
- `backend/internal/system/dashboard.go`
- `backend/internal/httpapi/disk.go`
- `backend/internal/httpapi/disk_windows.go`
- `backend/internal/httpapi/disk_unix.go`
- `frontend/apps/admin/src/pages/DashboardPage.vue`

概览页分区并行请求，各自显示加载、错误与重试；单区失败不阻塞其余区域。接口均为管理员只读请求：

- `GET /api/dashboard/pipeline`：当前队列和昨日/今日的绑定、下载完成、媒体处理完成数，返回 `overview`。
- `GET /api/scheduled-tasks`：复用现有任务列表，失败优先展示、保留停用状态和原始错误。
- `GET /api/dashboard/activities`：全站最近 5 条有效观看历史，返回 `items` 和 `generatedAt`。含用户名称、番剧、分集、进度及最后观看时间；不是每次播放的事件流水，详见 [viewer.md](viewer.md)。
- `GET /api/dashboard/storage`：磁盘路径及可用空间，返回 `storage.roots`，单独探测不拖住队列/活动显示。
- 原 `GET /api/dashboard/overview` 保留兼容，返回流水线数据加 `storage`；新页面不使用该合并接口。

队列口径：

- 待绑定订阅数量。
- 下载 pending、downloading、failed 数量。
- 转码 pending、transcoding、failed 数量。
- 默认媒体目录和额外存储根的磁盘空间。

每日口径：

- 后端按 `time.Local`（服务进程实际时区）构造本日零点，用 `AddDate` 获取昨日及次日零点，使用 `[起点, 终点)` 的 Unix 时间范围；不依赖浏览器时区，也不假定一天固定为 86400 秒。响应携带服务器时区、UTC 偏移、两天日期和数据生成时间，页面直接显示。
- 已绑定数基于当前 `binding_status='bound'` 的 `bound_at`；系统生成备注“标题记忆自动绑定”“历史话数同步自动绑定”“本地媒体文件夹同步自动绑定”计自动，“人工确认绑定”“人工手动绑定”“手动同步/替换单话”计手动，其余备注计来源未知并包含在总数，不猜测旧数据来源。以后新增绑定来源需同步此分类。
- 下载完成数使用 `download_jobs.completed_at`，保持下载管理有效绑定条件，排除 `source_type='local'`。媒体完成数使用 `media_jobs.completed_at`，包含 copy、remux、转码和本地导入，与媒体任务的 completed 状态一致。
- 只统计现有记录的当前状态与时间，不是永久审计流水。解除、删除、替换或重试会影响统计；缺失完成/绑定时间不以 `updated_at` 猜测补齐。
- 概览读取不再调用媒体管理的入队计数路径，不触发写入或创建媒体任务；尚未入队的媒体由后台任务创建后显示。

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
