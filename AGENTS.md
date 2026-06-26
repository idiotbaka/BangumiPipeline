# BangumiPipeline AI 开发指南

本仓库是一个 Go 后端与 Vue 3 双前端组成的单体项目。核心架构是“单进程、双端口、SQLite 单一事实源”：后端进程同时服务管理端与观看端，不要重新引入多个服务之间的数据同步。项目对外名称统一为 `BangumiPipeline`。

## 项目结构

```text
backend/
  cmd/server/             服务入口；初始化依赖、注册计划任务、启动双 HTTP 端口
  internal/auth/          管理员账号、密码哈希、登录 Session
  internal/config/        环境变量配置，统一优先 BP_*，AB_* 仅兼容旧配置
  internal/database/      SQLite 打开、WAL、schema_migrations 和所有迁移
  internal/httpapi/       管理端 API、观看端 API、鉴权、受保护图片与 SPA 托管
  internal/system/        计划任务状态、调度器、网络/订阅/下载/存储/LLM 系统设置
  internal/applog/        SQLite 系统日志与 SSE 实时日志
  internal/bangumi/       Bangumi 元数据抓取、分集元数据、存储、番剧目录查询
  internal/subscription/  RSS 订阅抓取、标题解析、番剧匹配、人工绑定、历史话数同步
  internal/download/      qBittorrent 对接、下载任务创建/同步/失败重试/清理
  internal/media/         下载产物识别、ffprobe/ffmpeg 处理、最终产物/封面图入库、多磁盘移动
  internal/translation/   OpenAI Chat 兼容 LLM 元数据翻译计划任务
frontend/apps/
  admin/                  Vue 3 + TypeScript + Element Plus 管理端
  viewer/                 观看端，目前仍是基础占位
compose.yaml              Linux/本地容器部署
Dockerfile                多阶段生产构建
AGENTS.md                 本文件，面向 AI Agent 的项目说明
```

## 运行入口与依赖装配

- 后端入口在 `backend/cmd/server/main.go`。
- 管理端默认监听 `:8080`，观看端默认监听 `:8090`。
- 服务初始化顺序：SQLite -> 日志 handler -> auth/system/bangumi/subscription/download/media/translation -> scheduler -> HTTP servers。
- 计划任务注册点在 `main.go`：
  - `bangumi-season-metadata`
  - `subscription-rss-match`
  - `download-bound-episodes`
  - `process-downloaded-media`
  - `translate-anime-metadata`
- Admin API 路由集中在 `backend/internal/httpapi/admin.go` 的 `NewAdminHandler`。
- 前端 API 类型和请求函数统一在 `frontend/apps/admin/src/api.ts`。
- 管理端路由在 `frontend/apps/admin/src/router.ts`，侧边栏在 `frontend/apps/admin/src/components/AdminLayout.vue`。

## 配置与目录

主要环境变量在 `backend/internal/config/config.go`：

- `BP_ADMIN_ADDR`、`BP_VIEWER_ADDR`
- `BP_DATABASE_PATH`
- `BP_ADMIN_WEB_DIR`、`BP_VIEWER_WEB_DIR`
- `BP_COVER_DIR`，默认 `./data/images/bangumi`
- `BP_DOWNLOAD_DIR`，默认 `./data/downloads`
- `BP_MEDIA_DIR`，默认 `./data/bangumi`
- `BP_FFMPEG_PATH`，默认 `ffmpeg`
- `BP_FFPROBE_PATH`，默认 `ffprobe`
- `BP_BANGUMI_API_URL`
- `BP_BANGUMI_USER_AGENT`
- `BP_COOKIE_SECURE`

旧 `AB_*` 变量只作为兼容 fallback，新代码和文档不要把 `AB_*` 作为首选。不要提交个人代理地址、账号、Cookie、Bangumi Token、qBittorrent 密码或 LLM API KEY。

Bangumi 要求可识别 User-Agent。部署时应设置 `BP_BANGUMI_USER_AGENT`，例如 `your-id/BangumiPipeline/0.1`。

额外媒体存储根目录和 LLM 配置属于数据库中的系统设置，不通过环境变量配置。默认媒体目录仍由 `BP_MEDIA_DIR` 控制。

## 数据库

SQLite 是唯一事实源，打开逻辑在 `backend/internal/database/database.go`：

- 使用 WAL。
- `MaxOpenConns(1)` 和 `MaxIdleConns(1)` 必须保持。
- 默认新库是 `data/bangumi-pipeline.db`。
- 如果未显式配置且存在旧库 `data/autobangumi.db`，必须继续沿用旧库，避免用户误以为数据丢失。

所有 schema 变更都放在 `backend/internal/database/database.go`，并增加新的 `schema_migrations` 版本。迁移必须兼容已有数据库，不允许要求用户删库重建。涉及清理、重算、补默认值的迁移必须只执行一次。

关键表：

- `scheduled_tasks`：计划任务配置与运行状态。
- `system_logs`：持久化系统日志。
- `anime_metadata`、`anime_aliases`、`anime_tags`、`anime_characters`、`actors`、`character_actors`、`anime_episodes`：Bangumi 元数据、角色/声优、分集元数据和中文翻译字段。
- `subscription_settings`、`subscription_items`、`subscription_title_rules`：RSS 订阅、匹配、绑定和标题记忆。
- `download_settings`、`download_jobs`：qBittorrent 配置与下载状态。
- `media_jobs`：下载完成后的视频识别、转码/压制、最终产物路径、封面图路径和处理状态。
- `media_storage_settings`：额外成品视频存储根目录配置。
- `llm_settings`：OpenAI Chat 兼容 LLM 的 Base URL、API KEY、模型名称。

查询嵌套数据时要先关闭外层 `rows`，避免单连接 SQLite 自锁。

## 计划任务

计划任务由 `backend/internal/system/scheduler.go` 调度，同一任务不可并发。`Scheduler.Trigger` 会检查内存 `running` map，`system.Service.MarkTaskStarted` 也会阻止数据库层面重复标记运行。不要在单个任务内部再并发执行同一类重型工作，除非同时更新并发保护策略。

当前任务：

- `bangumi-season-metadata`
  - 名称：`从 bangumi.tv 抓取当季新番元数据`
  - 默认间隔 15 分钟，默认关闭。
  - 执行器：`backend/internal/bangumi.Syncer`
- `subscription-rss-match`
  - 名称：`抓取订阅和匹配番剧`
  - 默认间隔 15 分钟，默认关闭。
  - 执行器：`backend/internal/subscription.Service`
- `download-bound-episodes`
  - 名称：`下载番剧`
  - 默认间隔 1 分钟，默认关闭。
  - 执行器：`backend/internal/download.Service`
- `process-downloaded-media`
  - 名称：`处理和转码已下载完成的视频`
  - 默认间隔 1 分钟，默认关闭。
  - 执行器：`backend/internal/media.Service`
- `translate-anime-metadata`
  - 名称：`翻译新番元数据`
  - 默认间隔 1 分钟，默认关闭。
  - 执行器：`backend/internal/translation.Service`
  - 需要系统设置中配置有效 LLM Base URL 和模型名称，否则执行失败。

## Bangumi 元数据流程

相关代码：`backend/internal/bangumi/`。

- `Syncer` 只处理 `type=2` 动画。
- 已存在 Bangumi ID 会跳过基础创建，但失败阶段可重试。
- Subject API 与 Characters API 用于补全简介、Tags、别名、话数、Infobox、Rating、Collection、角色和声优。
- Episodes API `GET /v0/episodes?subject_id={bangumi_id}` 用于补全每一话元数据，保存到 `anime_episodes`。
- 分集元数据只在缺失或分集阶段未完成时抓取；已存在且完整的分集元数据不应在常规计划任务中重复更新。
- Infobox、Rating、Collection、Meta Tags 等动态结构保存原始 JSON，不要把 Bangumi 可变 key 写死进数据库列。
- 每部番只处理 Characters API 返回的前 10 个角色。
- 详情、角色、分集阶段相互独立：一个阶段成功后，不应因另一个阶段失败而重复抓取。
- 演员按 Bangumi actor ID 在 `actors` 表全局去重，通过 `character_actors` 关联角色。
- 分集元数据保存日文/原文标题 `anime_episodes.name`、中文标题 `name_cn`、简介原文 `description`、简介中文 `description_cn`、话数 `ep_number`、排序 `sort_number`、类型 `type`、时长和评论数。
- Bangumi 的特殊话、SP、中间话可能 `ep_number=0` 且 `sort_number` 为 `9.5` 等小数；展示和排序必须使用 `sort_number`，不要假设话数只能是整数。
- Bangumi API 请求必须串行，间隔至少 2 秒；API 与图片请求超时为 20 秒。
- 图片状态：
  - `downloaded`：已下载。
  - `failed`：临时失败，后续任务可重试。
  - `not_found`：HTTP 404 或无 URL，永久不可用，不再请求。

番剧目录查询在 `backend/internal/bangumi/catalog.go`。番剧管理卡片的已绑定话数来自 `subscription_items`，如果对应 `media_jobs.status = completed`，该话数 `status` 返回 `completed`，前端显示绿色 tag；否则返回 `matched`，显示黄色 tag。详情展示剧情简介、分集简介、角色简介和声优简介时，优先读取对应中文翻译字段，缺失时回退原文字段。

## LLM 元数据翻译

相关代码：

- `backend/internal/translation/service.go`
- `backend/internal/system/service.go`
- 设置面板在 `frontend/apps/admin/src/pages/SettingsPage.vue`

系统设置中的 LLM 配置保存到 `llm_settings`：

- `base_url`：OpenAI Chat 兼容接口的 v1 根地址，后端会调用 `/chat/completions`。
- `api_key`：Bearer token，可为空以兼容本地模型服务。
- `model`：模型名称；计划任务至少要求 Base URL 和模型名称有效。
- “测试连接”会调用当前配置的模型并要求模型只返回 `OK`；返回非 `OK` 视为测试失败。

翻译计划任务 `translate-anime-metadata` 的行为：

- 遍历原文非空且中文字段为空的元数据字段。
- 翻译目标包括：番剧标题 `anime_metadata.name -> name_cn`、番剧简介 `summary -> summary_cn`、分集标题 `anime_episodes.name -> name_cn`、分集简介 `description -> description_cn`、角色简介 `anime_characters.summary -> summary_cn`、声优简介 `actors.short_summary -> short_summary_cn`。
- 如果原文本身是中文且不需要清理，则直接复制到中文字段，不调用 LLM。
- 日文判断主要看是否包含平假名或片假名；包含日文假名时必须走 LLM。
- 剧情简介、分集简介即使是中文，如果包含脚本、演出、作画、制作人员等非剧情信息，也要走 LLM 清理。
- LLM 请求串行处理，使用系统网络代理配置，超时 60 秒。
- 翻译成功后写 INFO 日志，`source=translation`，包含文本类型、番剧名、原文和译文，便于在控制台和系统日志中调试质量；不得记录 API KEY。

翻译提示词要求：

- 直接输出翻译内容，不输出“好的”、说明、注释、引号或 Markdown 代码块。
- 明确番剧标题和文本类型。
- 保留或优化段落格式。
- 剧情简介和分集简介中去除非剧情简介内容，例如脚本、分镜、演出、作画监督、制作人员信息。
- 剧情简介、分集简介、角色简介涉及角色名或专有名词时，首次出现用括号标注原文，例如 `好实祈（好実いのり）`。
- 所有双引号统一使用中文直角引号 `「」`。

## RSS 订阅、匹配和历史话数

相关代码：`backend/internal/subscription/service.go`。

- RSS 配置在 `subscription_settings`。
- 抓取后写入 `subscription_items`。
- 自动匹配根据本地番剧名称、中文名和别名解析标题、季数、话数和集数类型。
- 自动匹配成功的条目仍是待确认，不可视为已绑定资源。
- 只有 `binding_status = 'bound'` 的条目才可进入番剧管理卡片、下载任务和媒体处理链路。
- 手动确认或手动绑定会写入 `bound_*` 字段，并保存标题记忆规则到 `subscription_title_rules`。
- 同一番剧、季数、话类型、话数只能有一个已绑定条目，新增绑定不得静默覆盖已有绑定。
- “同步历史话数”使用该番剧最新已绑定标题作为模板，删除模板标题中的话数数字后调用 Mikan `RSS/Search`。
- 历史同步只绑定标题记忆 key 一致、同季、同话类型且尚未绑定的条目；已绑定跳过，已忽略不覆盖。
- Mikan 搜索结果复用现有 RSS item 入库和解析逻辑；网络请求使用系统代理配置和 20 秒超时。
- 动态正则不要用 `regexp.MustCompile` 处理用户/网络输入；Go regexp 不支持 lookahead/lookbehind 等 Perl 语法。

管理端页面：`frontend/apps/admin/src/pages/SubscriptionMatchesPage.vue`。

## 下载流程和 qBittorrent

相关代码：

- `backend/internal/download/service.go`
- `backend/internal/download/qbit.go`
- `frontend/apps/admin/src/pages/DownloadManagementPage.vue`
- 设置面板在 `frontend/apps/admin/src/pages/SettingsPage.vue`

系统设置中的下载配置保存到 `download_settings`：

- qBittorrent Host、Port、Username、Password
- 最大并发下载数 `max_concurrent_downloads`

下载计划任务 `download-bound-episodes` 的行为：

- 从已绑定的 `subscription_items` 中查找尚未下载或 `pending` 的话数。
- 下载源优先级：`enclosure_url` -> `torrent_url` -> `link`。
- 对 Mikan `https://mikanani.me/Home/Episode/<40位hash>` 链接，可转成 `magnet:?xt=urn:btih:<hash>`。
- 每个任务下载到 `BP_DOWNLOAD_DIR` 下清晰命名的单独文件夹。
- qBittorrent tag：
  - 全局 tag：`bangumi-pipeline`
  - 单集 tag：`bp-item-<subscription_item_id>`
- 并发上限来自 `download_settings.max_concurrent_downloads`。
- 剩余磁盘空间小于 10GB 时不创建新任务，并写 WARNING 日志。
- 下载失败不自动重试；管理端“下载失败”列表提供“重试”按钮。

`download_jobs.status`：

- `pending`：待下载。
- `downloading`：已提交 qBittorrent，持续同步进度。
- `completed`：qBittorrent 报告完成。
- `failed`：创建失败、qBittorrent 状态异常或任务丢失。

状态同步不要只依赖 qBittorrent tag。现有逻辑会通过 hash、tag、save_path 多条件匹配，兼容 tag 未写入或重复 torrent 的情况。

下载失败重试逻辑：

- 如果 qBittorrent 实际仍在下载或已完成，纠正本地状态。
- 如果 qBittorrent 没有该任务，重置为 `pending`。
- 如果 qBittorrent 上该任务确实失败，删除 qBittorrent 任务但不删文件，然后重置为 `pending`。

最终媒体产物完成后，`media.Service` 会调用 `download.Service.CleanupCompletedQBitTask`：

- 删除 qBittorrent torrent。
- `deleteFiles=true`，清理 downloads 中原始下载文件。
- 尽量删除 `bp-item-<subscription_item_id>` 标签，避免标签堆积。
- 清理失败不回滚已完成媒体产物；只在 `media_jobs.error_message` 和系统日志中记录 warning。

## 媒体处理、转码和最终产物

相关代码：

- `backend/internal/media/service.go`
- 管理端页面：`frontend/apps/admin/src/pages/TranscodeManagementPage.vue`

最终产物目录默认 `data/bangumi`，可用 `BP_MEDIA_DIR` 修改。ffmpeg/ffprobe 默认使用 PATH 中的 `ffmpeg`、`ffprobe`，可用 `BP_FFMPEG_PATH`、`BP_FFPROBE_PATH` 指定完整路径。

系统设置支持配置多个额外成品视频存储根目录，保存到 `media_storage_settings.extra_roots_json`。每部番当前成品视频存储根目录保存在 `anime_metadata.media_storage_root`；空值表示使用默认媒体目录。番剧管理会展示当前存储路径，并可通过 `POST /api/anime/{bangumiID}/storage` 移动到默认媒体目录或已配置的额外存储根目录。

移动番剧存储路径时：

- 目标根目录必须是服务器上的绝对路径，且必须已在系统设置中配置；默认媒体目录永远可选。
- 如果该番剧存在 `media_jobs.status = 'transcoding'` 的任务，禁止移动。
- 移动会搬迁该番剧成品视频目录，并同步更新 `media_jobs.output_path` 和 `media_jobs.cover_path`。
- `media.Service.storageMu` 用于避免路径移动、媒体任务占用和封面补齐同时修改同一批文件。
- 连载番剧移动后，后续新增绑定话数进入媒体处理时会读取最新的 `anime_metadata.media_storage_root`，输出到新的存储根目录。

媒体计划任务 `process-downloaded-media` 的行为：

1. 将 `download_jobs.status = completed` 且尚无媒体任务的记录写入 `media_jobs(status='pending')`。
2. 恢复服务重启中断的 `transcoding` 任务为 `pending`。
3. 对已完成但封面图缺失、失败或文件被删除的历史产物补齐封面图。
4. 循环处理待处理任务：
   - 如果只需复制最终产物，继续处理下一个任务。
   - 如果需要 ffmpeg remux、转码或字幕压制，本轮只处理这一个重型任务。
   - 如果探测阶段失败，本轮停止，避免环境错误时一次性把所有任务打失败。

视频识别：

- 下载产物可能是单视频文件，也可能是文件夹。
- 文件夹内递归选择体积最大的视频文件作为主视频。
- 支持识别常见视频扩展名：mp4、m4v、mkv、mov、avi、wmv、flv、ts、m2ts、webm。
- 外挂字幕优先选择与视频同 basename 的 ass/ssa/srt/vtt，否则选择目录中第一个字幕文件。
- 内封字幕通过 ffprobe streams 判断。

处理策略：

- 无字幕且浏览器可直接播放：复制到最终产物目录。
- 无字幕但容器不适合网页播放、视频音频已是 H.264/AAC：ffmpeg remux。
- 编码不适合网页播放：ffmpeg 转码为 H.264 + AAC + MP4。
- 有外挂字幕或内封字幕：ffmpeg 字幕压制并转码为 H.264 + AAC + MP4。

浏览器可直接播放的判断：

- 扩展名 mp4 或 m4v。
- 视频编码 `h264`。
- 音频为空或 `aac`。
- 像素格式为空、`yuv420p` 或 `yuvj420p`。

最终产物命名：

```text
{番剧存储根目录}/{番剧名称}/{Season 1|OVA|OAD|SP}/{番剧名称 S01E01.mp4}
```

路径片段会过滤 Windows 不允许的字符，例如 `/:?*"<>\|` 和控制字符。正片目录使用 `Season N`；特殊集目录使用 `OVA`、`OAD`、`SP` 等。

封面图生成：

- 最终产物生成后，使用 ffprobe 获取视频总时长，并尝试在总时长 1/2 处截取一帧。
- 使用 ffmpeg 先截取临时 PNG，再编码为 JPG；最长边不超过 480px，JPEG 质量为 80。
- 封面图与产物视频同目录、同 basename，例如 `番剧 S01E01.mp4` 对应 `番剧 S01E01.jpg`。
- `media_jobs.cover_path` 保存封面图路径，`cover_status` 只使用 `pending`、`completed`、`failed`，`cover_error` 保存失败原因。
- 封面图生成失败只写 WARNING 日志并更新封面状态，不应导致已生成的视频产物失败。
- 转码失败重试会清空旧 `output_path`、`cover_path`、封面状态和处理状态，重新进入 `pending`。

`media_jobs.status`：

- `pending`：待处理。
- `transcoding`：处理中。包含复制、探测、remux、转码、字幕压制过程。
- `completed`：最终可播放产物已生成。
- `failed`：探测、复制、ffmpeg 或文件操作失败。

转码失败重试：

- 管理端“转码管理”的“处理失败”列表有“重试”按钮。
- 后端 `POST /api/media/jobs/{jobID}/retry` 仅允许 failed 任务重试。
- 重试会清空上次 source/output/codec/action/error/timestamps，并重置为 `pending`，等待后续计划任务处理。

## 系统概览

相关代码：

- `backend/internal/httpapi/admin.go`
- `backend/internal/httpapi/disk.go`
- `backend/internal/httpapi/disk_windows.go`
- `backend/internal/httpapi/disk_unix.go`
- `frontend/apps/admin/src/pages/DashboardPage.vue`

系统概览通过 `GET /api/dashboard/overview` 实时读取关键状态：

- 订阅匹配管理：`binding_status = 'pending'` 的待绑定数量。
- 下载管理：`pending`、`downloading`、`failed` 数量。
- 转码管理：`pending`、`transcoding`、`failed` 数量。
- 存储空间：默认媒体目录和已配置额外存储根目录的剩余空间、总空间、已用空间、已用百分比和可用状态。

磁盘空间每次打开系统概览或刷新时实时获取，不缓存到数据库。路径不存在时，会向上查找最近存在的父目录用于探测挂载磁盘；探测失败时该路径仍返回，但 `available=false` 且包含错误信息。Windows 使用 `GetDiskFreeSpaceEx`，非 Windows 使用 `statfs`。

## 系统日志

相关代码：`backend/internal/applog/`。

- 业务代码使用 `log/slog`。
- 日志必须带 `source` 属性，例如 `bangumi`、`subscription`、`download`、`media`、`translation`、`scheduler`、`http`。
- 级别只使用 INFO、WARNING、ERROR；Go 中 WARNING 对应 `logger.Warn`。
- 系统日志页面读取最近最多 1000 行，并通过 SSE 实时追加。
- 管理 API、系统日志流、本地元数据图片都必须要求管理员登录。

## 管理端前端

管理端使用 Vue 3、TypeScript、Element Plus。

关键文件：

- `frontend/apps/admin/src/api.ts`：所有 HTTP 调用和 API 类型。
- `frontend/apps/admin/src/router.ts`：路由。
- `frontend/apps/admin/src/components/AdminLayout.vue`：侧边栏。
- `frontend/apps/admin/src/pages/DashboardPage.vue`：系统概览、关键状态统计和实时磁盘空间。
- `frontend/apps/admin/src/pages/SettingsPage.vue`：网络、LLM、下载、订阅和额外存储路径设置。
- `frontend/apps/admin/src/pages/ScheduledTasksPage.vue`：计划任务。
- `frontend/apps/admin/src/pages/AnimeListPage.vue`：番剧管理卡片、历史话数同步入口、存储路径移动入口。
- `frontend/apps/admin/src/components/AnimeDetailPanel.vue`：番剧详情、角色/声优信息、分集标题和分集简介展示。
- `frontend/apps/admin/src/pages/SubscriptionMatchesPage.vue`：订阅匹配管理。
- `frontend/apps/admin/src/pages/DownloadManagementPage.vue`：下载状态、失败重试。
- `frontend/apps/admin/src/pages/TranscodeManagementPage.vue`：媒体处理/转码状态、失败重试。
- `frontend/apps/admin/src/pages/SystemLogsPage.vue`：系统日志。

前端约定：

- 使用 Composition API 和 `<script setup lang="ts">`。
- 新 API 类型必须写在 `api.ts`。
- 新页面必须处理 loading、空状态、禁用态和 API 错误。
- 观看端和普通媒体接口不要暴露服务器本地绝对路径；管理端的系统设置、系统概览和番剧存储管理可以展示服务器路径，用于管理员配置和移动存储。
- 图片使用受保护接口 `/api/anime/...`、`/api/actors/...`。
- 番剧卡片按钮区保持固定宽度和可读文本；新增按钮不要让话数 tag 或按钮互相遮挡。

## HTTP API 约定

- Admin API 都在 `backend/internal/httpapi/admin.go`。
- 使用 `requireAdministrator` 保护管理接口。
- 错误响应使用 `writeError`，格式为 `{ "error": { "code": "...", "message": "..." } }`。
- 路径 ID 使用 `parsePathID`。
- 前端请求通过 `api.ts` 的 `request<T>`，错误会抛出 `APIError`。

重要接口：

- `GET /api/dashboard/overview`
- `GET/PUT /api/settings/network`
- `GET/PUT /api/settings/subscription`
- `GET/PUT /api/settings/download`
- `POST /api/settings/download/test`
- `GET/PUT /api/settings/llm`
- `POST /api/settings/llm/test`
- `GET/PUT /api/settings/media-storage`
- `GET /api/scheduled-tasks`
- `PATCH /api/scheduled-tasks/{taskKey}`
- `POST /api/scheduled-tasks/{taskKey}/run`
- `GET /api/anime`
- `POST /api/anime`
- `GET /api/anime/search`
- `GET /api/anime/{bangumiID}`
- `DELETE /api/anime/{bangumiID}`
- `POST /api/anime/{bangumiID}/refresh`
- `POST /api/anime/{bangumiID}/sync-history`
- `POST /api/anime/{bangumiID}/storage`
- `GET /api/anime/{bangumiID}/cover`
- `GET /api/anime/{bangumiID}/characters/{characterID}/image`
- `GET /api/actors/{actorID}/image`
- `GET /api/subscription/items`
- `POST /api/subscription/items/{itemID}/confirm`
- `PUT /api/subscription/items/{itemID}/binding`
- `POST /api/subscription/items/{itemID}/ignore`
- `GET /api/download/jobs`
- `POST /api/download/jobs/{jobID}/retry`
- `GET /api/media/jobs`
- `POST /api/media/jobs/{jobID}/retry`
- `GET /api/system-logs`
- `GET /api/system-logs/stream`

## 必须保持的行为

- 单进程、双端口、SQLite 单一事实源。
- 同一个计划任务不得并发或堆积执行。
- Bangumi API 请求串行且至少间隔 2 秒。
- Bangumi API 与图片请求超时为 20 秒。
- 每部番只处理 Characters API 返回的前 10 个角色。
- 图片 404 记录为 `not_found` 且不再重试；临时错误记录为 `failed`。
- 自动匹配的订阅条目不能当作已绑定资源，只有 `binding_status='bound'` 才能下载和显示在番剧卡片。
- 下载任务创建前检查并发上限和 10GB 剩余磁盘空间。
- 下载失败不自动重试，只能通过管理端操作重置。
- 媒体处理最终产物完成后再清理 qBittorrent 原下载任务和文件。
- ffmpeg 转码/压制保持单任务执行；轻量 copy 任务可以在同一计划任务内连续处理。
- 最终媒体产物必须使用番剧当前 `anime_metadata.media_storage_root`，为空时才回退默认媒体目录。
- 移动番剧存储路径必须拒绝转码中的番剧，并同步移动视频和封面图路径。
- 封面图生成失败不得让已完成的视频产物失败，必须记录 `cover_status='failed'`、`cover_error` 和 WARNING 日志。
- 系统概览的磁盘空间必须实时探测默认媒体目录和额外存储根目录，不写入数据库缓存。
- LLM 翻译任务必须串行调用模型，不得记录 API KEY；翻译成功日志可以记录原文和译文用于调试。
- 观看端和普通媒体接口不向前端暴露本地绝对路径；管理端存储配置和系统概览是例外。

## 命名约定

- 项目名：`BangumiPipeline`
- Go module：`bangumipipeline.local/server`
- npm workspace/package：`bangumi-pipeline`、`@bangumi-pipeline/*`
- Docker 二进制：`bangumi-pipeline`
- 容器用户：`bangumipipeline`
- 新环境变量：`BP_*`

## AI 开发约定

- 修改代码前先阅读相关模块，优先沿用现有分层、命名和错误处理方式。
- 搜索文件或文本优先使用 `rg` / `rg --files`。
- 手工编辑文件优先使用补丁方式，避免顺手重写无关内容。
- 不要回滚用户或其他 Agent 已经做出的无关改动。
- PowerShell 命令在当前 Windows 开发环境中按当前工具权限策略执行。
- 默认只运行必要验证：Go 文件变更后执行 `gofmt -w <修改过的 Go 文件>`；除非当前任务明确要求，不要主动执行 `go test`、`npm run build:ui` 或 `npm test`。

## 验证命令

常规完整验证：

```powershell
gofmt -w <修改过的 Go 文件>
go test ./backend/...
npm run build:ui
```

只编译后端：

```powershell
go build -buildvcs=false ./backend/...
```

完整验证也可以运行：

```powershell
npm test
```

Vite 可能输出 Element Plus chunk size 和 `INVALID_ANNOTATION` 警告；只要退出码为 0，属于已知非阻断警告。

## 本地网络

安装依赖遇到网络问题时，可在当前 PowerShell 会话临时设置代理：

```powershell
$env:HTTP_PROXY='http://localhost:10808'
$env:HTTPS_PROXY='http://localhost:10808'
```

不要把个人代理地址、账号、Cookie、Bangumi Token、qBittorrent 凭据或 LLM API KEY 提交到仓库。
