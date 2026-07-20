# BangumiPipeline AI 开发指南

本文件用于让 AI Agent 快速理解项目边界、关键流程和不可破坏的行为。先读相关模块，再改代码；优先沿用现有分层、命名和错误处理方式。

## 项目定位

`BangumiPipeline` 是一个 Go 后端、Vue 3 双 Web 前端与 Tauri 移动端组成的单体项目。

核心架构必须保持：

- 单进程。
- 双端口：管理端和观看端由同一个后端进程服务。
- SQLite 单一事实源。
- 移动端 APP 只作为观看端 API 的客户端，不引入新的后端服务或独立数据源。
- 不重新引入多个服务之间的数据同步。

## 目录地图

```text
backend/
  cmd/server/             服务入口，初始化依赖，注册计划任务，启动 HTTP 端口
  internal/auth/          管理员账号、密码哈希、Session
  internal/config/        环境变量配置，优先 BP_*，AB_* 仅兼容旧配置
  internal/database/      SQLite 打开、WAL、schema_migrations 和迁移
  internal/httpapi/       管理端 API、观看端 API、鉴权、图片接口、SPA 托管
  internal/system/        计划任务状态、调度器、网络/下载/媒体/LLM 设置
  internal/applog/        SQLite 系统日志与 SSE 实时日志
  internal/bangumi/       Bangumi 元数据抓取、图片下载、分集、目录查询
  internal/subscription/  RSS 抓取、标题解析、番剧匹配、手动绑定和补源
  internal/download/      qBittorrent 对接、下载任务创建/同步/重试/清理
  internal/media/         下载产物识别、ffmpeg/ffprobe、最终产物、封面、存储移动
  internal/translation/   OpenAI Chat 兼容 LLM 元数据翻译
  internal/viewer/        观看端用户、Session、站点配置、轮播、筛选、历史与追番
frontend/apps/
  admin/                  Vue 3 + TypeScript + Element Plus 管理端
  viewer/                 观看端
  mobile/                 Tauri + Vue 3 移动端 APP WebView 前端
src-tauri/                Tauri v2 移动端壳、Android 原生桥接、图标和资源覆盖
scripts/                  运维和一次性维护脚本
compose.yaml              Linux/本地容器部署
Dockerfile                多阶段生产构建
```

## 入口与装配

- 后端入口：`backend/cmd/server/main.go`。
- 管理端默认监听 `:8080`。
- 观看端默认监听 `:8090`。
- 初始化顺序：SQLite -> 日志 handler -> admin auth/viewer auth/system/bangumi/subscription/download/media/translation -> scheduler -> HTTP servers。
- Admin API 路由集中在 `backend/internal/httpapi/admin.go` 的 `NewAdminHandler`。
- Viewer API 路由集中在 `backend/internal/httpapi/viewer.go` 的 `NewViewerHandler`。
- Web 前端的 API 类型和请求函数分别集中在 `frontend/apps/admin/src/api.ts`、`frontend/apps/viewer/src/api.ts`。
- 移动端 API 类型和请求函数集中在 `frontend/apps/mobile/src/api.ts`，复用观看端 Viewer API。
- 管理端路由在 `frontend/apps/admin/src/router.ts`。
- 侧边栏在 `frontend/apps/admin/src/components/AdminLayout.vue`。
- 移动端 Tauri 配置在 `src-tauri/tauri.conf.json`。
- 移动端 Android Activity 覆盖在 `src-tauri/android-src/main/java/vip/baka/bangumipipeline/mobile/MainActivity.kt`。
- 移动端原生播放器桥接在 `src-tauri/plugins/player/`。

计划任务注册点在 `main.go`：

- `bangumi-season-metadata`
- `subscription-rss-match`
- `download-bound-episodes`
- `process-downloaded-media`
- `translate-anime-metadata`
- `sync-bangumi-episode-comments`

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

规则：

- 新代码和文档优先使用 `BP_*`。
- `AB_*` 只作为旧配置兼容 fallback。
- 不提交代理地址、账号、Cookie、Bangumi Token、qBittorrent 密码或 LLM API KEY。
- Bangumi 请求需要可识别 User-Agent，例如 `your-id/BangumiPipeline/0.1`。
- 额外媒体存储根目录、网络代理、下载设置、LLM 设置保存在数据库系统设置中。

## 数据库

SQLite 是唯一事实源。Writer 打开和迁移在 `backend/internal/database/database.go`，分池与读写路由在 `backend/internal/database/connections.go`。

必须保持：

- WAL。
- 唯一 Writer：`MaxOpenConns(1)`、`MaxIdleConns(1)`，所有事务和 INSERT/UPDATE/DELETE 必须走 Writer。
- Viewer、Admin、Worker 使用互相隔离的只读连接池；默认连接数分别为 4、1、1。
- Reader 必须以 SQLite `mode=ro` 和 `query_only=ON` 打开，不能用于事务或业务写入。
- HTTP 请求通过上下文路由到 Viewer/Admin Reader；Scheduler 及后台任务路由到 Worker Reader。未标记的后台上下文默认使用 Worker Reader。
- `foreign_keys`、`busy_timeout`、`synchronous`、`query_only` 等连接级 PRAGMA 必须写入 DSN 或逐连接初始化，不能只对连接池执行一次后假定新物理连接会继承。
- 默认新库 `data/bangumi-pipeline.db`。
- 如果未显式配置且存在旧库 `data/autobangumi.db`，继续沿用旧库。

迁移规则：

- 所有 schema 变更都放在 `backend/internal/database/database.go`。
- 新 schema 变更必须增加 `schema_migrations` 版本。
- 迁移必须兼容已有数据库，不允许要求用户删库重建。
- 清理、重算、补默认值等迁移必须只执行一次。
- 查询嵌套数据时仍应先关闭外层 `rows`，避免长时间持有 Reader 快照、阻止 WAL checkpoint 或无谓占用连接。
- Admin 普通 GET API 使用 15 秒上下文期限；系统日志 SSE 流不使用该期限。

关键表：

- `scheduled_tasks`：计划任务配置与运行状态。
- `system_logs`：持久化系统日志。
- `anime_metadata`：番剧主体元数据、阶段状态、存储根、媒体触发刷新时间。
- `anime_aliases`、`anime_tags`：别名与标签。
- `anime_characters`、`actors`、`character_actors`：角色、声优、关联。
- `anime_episodes`：分集元数据。
- `bangumi_episode_comment_sync`：按 Bangumi episode ID 记录吐槽抓取里程碑、重试和历史回填状态。
- `bangumi_episode_comment_task_state`：记录一次性历史评论回填是否已经耗尽，避免每分钟重复扫描成品媒体。
- `bangumi_episode_comments`：剧集吐槽及楼中楼回复快照，保留用户、头像、签名、reactions 和原始 JSON。
- `bangumi_custom_search_settings`：Bangumi 自定义标签抓取设置。
- `subscription_settings`、`subscription_items`、`subscription_title_rules`：RSS、匹配、绑定、标题记忆。
- `download_settings`、`download_jobs`：qBittorrent 设置与下载状态。
- `media_jobs`：媒体处理状态、最终视频、封面图、转码信息，以及评论链路入队完成标记。
- `media_storage_settings`：额外成品视频存储根目录。
- `llm_settings`：OpenAI Chat 兼容 LLM 配置。
- `viewer_users`、`viewer_sessions`：观看端独立用户和 Session。
- `viewer_site_settings`、`viewer_invitation_codes`：观看端站点配置、favicon、注册开关和邀请码。
- `viewer_carousel_items`：首页轮播图二进制图片、绑定番剧和排序。
- `viewer_filter_dimensions`、`viewer_filter_tags`：番剧图书馆可配置的筛选维度与标签。
- `viewer_watch_history`：按用户和成品媒体记录播放位置、时长、完播状态及最后观看时间。
- `viewer_anime_follows`：用户追番关系；同一用户和番剧唯一。
- `viewer_app_releases`：移动端 APP 版本、更新日志、arm64-v8a APK、文件大小与 SHA-256。

## 调度器

计划任务由 `backend/internal/system/scheduler.go` 调度。

必须保持：

- 同一个任务不可并发执行。
- `Scheduler.Trigger` 检查内存 `running` map。
- `system.Service.MarkTaskStarted` 阻止数据库层面的重复运行。
- 不要在单个任务内部并发执行同类重型工作，除非同步更新并发保护策略。

当前任务：

- `bangumi-season-metadata`：从 bangumi.tv 抓取新番元数据，默认 15 分钟，默认关闭。
- `subscription-rss-match`：抓取订阅和匹配番剧，默认 15 分钟，默认关闭。
- `download-bound-episodes`：下载番剧，默认 1 分钟，默认关闭。
- `process-downloaded-media`：处理和转码已下载完成的视频，默认 1 分钟，默认关闭。
- `translate-anime-metadata`：翻译新番元数据，默认 1 分钟，默认关闭。
- `sync-bangumi-episode-comments`：同步已有成品话数的 Bangumi 吐槽，默认 1 分钟，默认开启。

## Bangumi 元数据

相关代码：`backend/internal/bangumi/`。

基础行为：

- `Syncer` 只处理 `type=2` 动画。
- 常规任务先抓 `api.bgm.tv/calendar`，再按自定义标签额外搜索。
- 自定义标签接口：`POST /v0/search/subjects?limit=20&offset=N`。
- 自定义搜索 body 固定 `filter.type=[2]`，`filter.tag` 来自用户保存的标签列表。
- 自定义搜索分页每页 20 条，按 `limit/offset` 拉完整结果。
- 已存在 Bangumi ID 会跳过基础创建，但失败阶段可重试。
- Subject API 补全简介、Tags、别名、话数、Infobox、Rating、Collection、Meta Tags。
- Characters API 补全角色和声优。
- Episodes API `GET /v0/episodes?subject_id={bangumi_id}` 补全分集。
- 剧集吐槽使用 `GET https://next.bgm.tv/p1/episodes/{episode_id}/comments`，由独立计划任务串行同步。
- 某话成品首次完成后立即入队，并在成品完成后的 2 小时、12 小时、24 小时、3 天和 7 天更新；超过 7 天且从未抓取的历史成品只兜底抓取一次。
- 吐槽正文按接口原文保存，不直接替换 `(bgm38)`、`[s]`、`[mask]`、`[img]` 等内容标记。
- 首次执行吐槽任务时检查 `BP_COVER_DIR/smiles`；表情资源不完整时使用系统代理补抓，并通过 `manifest.json` 将代码绑定到本地 GIF/PNG 文件。
- 完整的 `manifest.json` 是表情资源的持久完成标记；自动任务成功后不再逐个打开 428 个文件，失败重试至少间隔 1 小时。需要重新校验或修复文件时使用手动同步脚本。
- 历史成品兜底每轮最多发现 10 话；耗尽后写入 `bangumi_episode_comment_task_state.historical_backfill_completed` 并跳过每分钟扫描，新成品仍由媒体完成回调立即入队。`media_jobs.comment_sync_enqueued` 记录每个成品是否已进入评论链路；新成品入队失败时必须清除全局完成标记，服务重启时也要通过未入队成品部分索引兜住中断窗口。
- 媒体与 Bangumi 分集匹配必须同时使用 `bangumi_id` 和 `episode_id` 命中复合索引，禁止仅按 `episode_id` 对 `anime_episodes` 做外层关联而触发全表扫描。
- 评论到期查询使用 `(status, is_backfill, next_fetch_at, episode_id)` 索引；历史发现使用已完成媒体的部分索引，避免计划任务长期占用 Worker Reader 和 CPU。
- 表情保留原始 GIF/PNG，不能统一转为 JPG；格式不固定的资源先尝试 PNG，404 后尝试 GIF。
- 表情目录共定义 428 个实际资源；官网未实现且上游 404 的 `(musume_97)`、`(musume_98)` 必须固定跳过。
- 表情代码识别和清单解析集中在 `backend/internal/bangumi/smiles.go`；不要在 Viewer、Web 或 Mobile 端各自维护重复映射表。

刷新行为：

- 管理端“刷新元数据”调用 `Syncer.RefreshSubject`。
- 手动刷新会重新抓详情、角色和分集，并更新简介、分集、标签、评分、收藏、Infobox、Meta Tags 等。
- 剧情简介、角色简介、声优简介、分集标题或简介原文变动时，清空对应翻译字段。
- 原文未变时保留已翻译字段，避免 LLM 浪费。

阶段行为：

- 详情、角色、分集阶段相互独立。
- 一个阶段成功后，不应因另一个阶段失败而重复抓取。
- 常规任务只补缺失或未完成阶段；完整分集通常不重复更新。
- 如果正片分集数量少于 `eps` 或 `total_episodes`，允许重新补分集。

数据规则：

- Infobox、Rating、Collection、Meta Tags 保存原始 JSON，不把 Bangumi 可变 key 写死进列。
- 每部番只处理 Characters API 返回的前 10 个角色。
- 演员按 Bangumi actor ID 在 `actors` 表全局去重。
- 分集排序使用 `sort_number`，不要假设话数只能是整数。
- 特殊话、SP、中间话可能 `ep_number=0`，`sort_number=9.5`。

图片规则：

- 番剧封面、角色图、声优图下载后保存为 JPG。
- 图片保存前会压缩：最大宽度 680px，JPEG quality 80。
- 缩放依赖 ffmpeg；如果 ffmpeg 不可用，则跳过缩放但仍尝试 JPEG 重新编码。
- 图片请求超时 20 秒。
- Bangumi API 请求串行，间隔至少 2 秒。
- 图片状态只使用：
  - `downloaded`：已下载。
  - `failed`：临时失败，可重试。
  - `not_found`：空 URL 或 404，永久不可用，不再请求。

番剧目录：

- 查询代码在 `backend/internal/bangumi/catalog.go`。
- 番剧管理列表支持按添加时间、首播时间降序排序。
- 番剧管理列表支持按标题或别名模糊搜索。
- 卡片已绑定话数来自 `subscription_items`。
- 如果对应 `media_jobs.status='completed'`，话数状态返回 `completed`，否则返回 `matched`。
- 详情展示优先读取中文翻译字段，缺失时回退原文字段。

## LLM 翻译

相关代码：

- `backend/internal/translation/service.go`
- `backend/internal/system/service.go`
- `frontend/apps/admin/src/pages/SettingsPage.vue`

设置保存到 `llm_settings`：

- `base_url`：OpenAI Chat 兼容接口 v1 根地址。
- `api_key`：Bearer token，可为空以兼容本地模型。
- `model`：模型名称。
- 测试连接要求模型只返回 `OK`。

任务行为：

- 遍历原文非空且中文字段为空的元数据字段。
- 翻译目标包括番剧标题、番剧简介、分集标题、分集简介、角色简介、声优简介。
- 如果原文本身是中文且不需要清理，则直接复制到中文字段。
- 包含日文假名时必须走 LLM。
- 剧情简介和分集简介要清理脚本、演出、作画、制作人员等非剧情信息。
- LLM 请求串行，使用系统网络代理，超时 60 秒。
- 不记录 API KEY。
- 翻译成功日志可以记录原文和译文用于调试。

提示词约定：

- 直接输出结果，不输出说明、引号或 Markdown 代码块。
- 保留或优化段落格式。
- 角色名或专有名词首次出现用括号标注原文。
- 双引号统一使用中文直角引号 `「」`。

## RSS、匹配和手动补源

相关代码：`backend/internal/subscription/service.go`。

RSS 流程：

- RSS 配置保存在 `subscription_settings`。
- 抓取结果写入 `subscription_items`。
- 自动匹配根据本地番剧名称、中文名和别名解析标题、季数、话数和集数类型。
- 自动匹配成功的条目仍是待确认，不可视为已绑定资源。
- 只有 `binding_status='bound'` 的订阅条目才能进入下载和媒体处理链路；管理端元数据目录和观看端无产物卡片不受此限制。
- 手动确认或手动绑定会写入 `bound_*` 字段，并保存标题记忆规则到 `subscription_title_rules`。
- 同一番剧、季数、话类型、话数只能有一个已绑定条目，新增绑定不得静默覆盖。

同步历史话数：

- 使用该番剧最新已绑定标题作为模板。
- 删除模板标题中的话数数字后调用 Mikan `RSS/Search`。
- 只绑定标题记忆 key 一致、同季、同话类型且尚未绑定的条目。
- 已绑定跳过，已忽略不覆盖。
- 网络请求使用系统代理配置，超时 20 秒。

同步/替换单话：

- 管理端番剧卡片有“同步/替换单话”。
- API：`POST /api/anime/{bangumiID}/sync-episode`。
- 输入为磁力链接、季数、集数类型、集数。
- 磁力链接必须包含 BTIH。
- 该功能不走 RSS 匹配，直接创建或更新已绑定 `subscription_items`。
- 同时创建或重置对应 `download_jobs(status='pending')`。
- 替换前会清理同一番剧、季数、话类型、话数的旧绑定和旧下载任务。
- 如果已有成品视频，会删除旧成品视频和旧封面图，并删除旧 `media_jobs`。
- 如果对应话存在 `media_jobs.status='transcoding'`，必须拒绝操作。
- 动态正则不要用 `regexp.MustCompile` 处理用户或网络输入。
- Go regexp 不支持 lookahead/lookbehind 等 Perl 语法。

## 下载流程

相关代码：

- `backend/internal/download/service.go`
- `backend/internal/download/qbit.go`
- `frontend/apps/admin/src/pages/DownloadManagementPage.vue`

下载设置保存到 `download_settings`：

- qBittorrent Host、Port、Username、Password。
- `max_concurrent_downloads`。

任务行为：

- 从已绑定 `subscription_items` 查找尚未下载或 pending 的话数。
- 下载源优先级：`enclosure_url` -> `torrent_url` -> `link`。
- Mikan `https://mikanani.me/Home/Episode/<40位hash>` 可转成 magnet。
- 每个任务下载到 `BP_DOWNLOAD_DIR` 下独立文件夹。
- qBittorrent 全局 tag：`bangumi-pipeline`。
- 单集 tag：`bp-item-<subscription_item_id>`。
- 创建新下载前检查并发上限和 10GB 剩余磁盘空间。
- 下载失败不自动重试，只能通过管理端操作重置。

状态：

- `pending`：待下载。
- `downloading`：已提交 qBittorrent，持续同步进度。
- `completed`：qBittorrent 报告完成。
- `failed`：创建失败、qBittorrent 状态异常或任务丢失。

状态同步不要只依赖 qBittorrent tag。现有逻辑会通过 hash、tag、save_path 多条件匹配。
qBittorrent `torrents/info` 响应可能很大；读取、JSON 解析和错误日志必须保留足够诊断信息，避免大队列时只报 `unexpected end of JSON input` 而无法定位响应截断、空响应或代理错误。

完成清理：

- 媒体产物完成后，`media.Service` 调用 `download.Service.CleanupCompletedQBitTask`。
- 媒体任务也会兜底重试已完成产物但下载清理失败的历史任务，避免 qBittorrent 中长期堆积已完成 torrent。
- 删除 qBittorrent torrent，`deleteFiles=true`。
- 尽量删除 `bp-item-<subscription_item_id>` 标签。
- 清理失败不回滚已完成媒体产物，只记录 warning。

## 媒体处理

相关代码：

- `backend/internal/media/service.go`
- `frontend/apps/admin/src/pages/TranscodeManagementPage.vue`

目录与工具：

- 最终产物目录默认 `data/bangumi`，可由 `BP_MEDIA_DIR` 修改。
- ffmpeg/ffprobe 默认来自 PATH，可由 `BP_FFMPEG_PATH`、`BP_FFPROBE_PATH` 指定。
- 额外成品视频存储根目录保存到 `media_storage_settings.extra_roots_json`。
- 每部番当前存储根保存在 `anime_metadata.media_storage_root`。
- 空 `media_storage_root` 表示使用默认媒体目录。

媒体任务行为：

- 将 `download_jobs.status='completed'` 且尚无媒体任务的记录写入 `media_jobs(status='pending')`。
- 恢复服务重启中断的 `transcoding` 为 `pending`。
- 补齐已完成但封面图缺失、失败或文件被删除的历史产物封面。
- 每轮先规划 pending 任务的处理方式，再拆成 copy 线路和 ffmpeg 线路。
- copy 任务可在同一轮连续处理，并可与一个 ffmpeg 任务并行执行，避免长时间转码阻塞直接复制任务。
- ffmpeg remux、转码、字幕压制等重型任务每轮只处理一个。
- 探测阶段失败时本轮停止，避免环境错误批量打失败。

识别规则：

- 下载产物可以是单视频文件或文件夹。
- 文件夹中递归选择体积最大的视频作为主视频。
- 支持 mp4、m4v、mkv、mov、avi、wmv、flv、ts、m2ts、webm。
- 外挂字幕优先同 basename 的 ass/ssa/srt/vtt，否则取第一个字幕文件。
- 内封字幕通过 ffprobe streams 判断。

处理策略：

- 无字幕且浏览器可直接播放：复制。
- 无字幕但容器不适合网页播放、编码为 H.264/AAC：remux。
- 编码不适合网页播放：转码为 H.264 + AAC + MP4。
- 有外挂或内封字幕：压制字幕并转码为 H.264 + AAC + MP4。

浏览器可直接播放判断：

- 扩展名 mp4 或 m4v。
- 视频编码 `h264`。
- 音频为空或 `aac`。
- 像素格式为空、`yuv420p` 或 `yuvj420p`。
- 仅满足 MP4/H.264/AAC 不一定足够。MP4 还要检查顶层 box 结构和 ffprobe warning：
  - `moov` 必须在 `mdat` 之前，否则 remux。
  - 顶层存在多个 `mdat` box 时 remux；这类文件常见于 HLS/fMP4 片段拼装，可能导致浏览器无限 Range 请求。
  - ffprobe warning 包含 edit list 缺关键帧或找不到索引项时 remux。
- remux 使用 `ffmpeg -c copy -movflags +faststart`，不重新编码，不改变画质。

最终命名：

```text
{番剧存储根目录}/{番剧名称}/{Season 1|OVA|OAD|SP}/{番剧名称 S01E01.mp4}
```

路径片段会过滤 Windows 不允许字符和控制字符。正片目录使用 `Season N`，特殊集目录使用 `OVA`、`OAD`、`SP` 等。

封面图：

- 最终视频生成后，用 ffprobe 获取总时长。
- 用 ffmpeg 在总时长 1/2 处截取临时 PNG，再编码为 JPG。
- 最长边不超过 480px，JPEG quality 80。
- 封面图与视频同目录、同 basename。
- 封面图失败不导致视频产物失败。
- 失败时记录 `cover_status='failed'`、`cover_error` 和 warning。

媒体完成后刷新元数据：

- 某话产物完成后会触发一次该番剧 `RefreshSubject`。
- 每部番由媒体任务触发的自动刷新每天最多一次。
- 节流字段为 `anime_metadata.last_media_refresh_at`。
- 这个节流不影响用户手动刷新。

存储移动：

- API：`POST /api/anime/{bangumiID}/storage`。
- 目标根目录必须是服务器绝对路径。
- 目标根目录必须已在系统设置中配置；默认媒体目录永远可选。
- 如果该番剧存在转码中的任务，禁止移动。
- 移动会搬迁番剧成品目录，并同步 `media_jobs.output_path` 与 `media_jobs.cover_path`。
- `media.Service.storageMu` 用于避免移动、媒体任务、封面补齐同时改文件。

媒体状态：

- `pending`：待处理。
- `transcoding`：处理中，包含复制、探测、remux、转码、字幕压制。
- `completed`：最终可播放产物已生成。
- `failed`：探测、复制、ffmpeg 或文件操作失败。

## 运维脚本

相关代码：

- `scripts/fix-mp4-remux.sh`
- `scripts/prepare-mobile-android.mjs`
- `scripts/build-android-apks.ps1`

视频修复脚本：

- 默认扫描 `/opt/downloads/BangumiPipeline/data/bangumi`，可用 `--root` 覆盖。
- 默认 dry-run，只输出候选文件；必须显式加 `--apply` 才会替换文件。
- 只处理 MP4，检测 ffprobe edit-list/index warning、`moov` 在 `mdat` 后面、多个顶层 `mdat` box 三类问题。
- 修复方式为同目录生成临时文件，执行 `ffmpeg -map 0 -c copy -movflags +faststart`，ffprobe 校验成功后替换原文件。
- 默认不保留备份；`--keep-backup` 会保留隐藏 `.remux.bak` 文件。
- 脚本保持成品视频路径不变，不需要同步修改 SQLite 中的 `media_jobs.output_path`。

移动端脚本：

- `prepare-mobile-android.mjs` 只负责把 `src-tauri/android-src/` 与 `src-tauri/android-res/` 覆盖到 Tauri 生成的 Android 工程。
- `prepare-mobile-android.mjs` 可以在 Android 工程未初始化时安全跳过。
- `build-android-apks.ps1` 默认构建 `aarch64`、`armv7`、`i686`、`x86_64` 多架构 APK。
- 首次签名时脚本会读取或要求 `ANDROID_KEYSTORE_PASSWORD`、`ANDROID_KEY_PASSWORD`，并在本地生成 keystore 与 `signing.env`。
- APK 产物输出到 `apk/{AppName}-{Version}-{yyyyMMdd-HHmmss}/`。

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

## 管理端前端

管理端使用 Vue 3、TypeScript、Element Plus。

关键文件：

- `frontend/apps/admin/src/api.ts`：所有 HTTP 调用和 API 类型。
- `frontend/apps/admin/src/router.ts`：路由。
- `frontend/apps/admin/src/components/AdminLayout.vue`：侧边栏。
- `frontend/apps/admin/src/pages/DashboardPage.vue`：系统概览。
- `frontend/apps/admin/src/pages/SettingsPage.vue`：网络、LLM、下载、订阅、额外存储。
- `frontend/apps/admin/src/pages/ScheduledTasksPage.vue`：计划任务、自定义 Bangumi 标签抓取设置。
- `frontend/apps/admin/src/pages/AnimeListPage.vue`：番剧管理、搜索、排序、刷新、历史同步、单话同步/替换、存储移动。
- `frontend/apps/admin/src/components/AnimeDetailPanel.vue`：番剧详情、角色/声优、分集展示。
- `frontend/apps/admin/src/pages/SubscriptionMatchesPage.vue`：订阅匹配。
- `frontend/apps/admin/src/pages/DownloadManagementPage.vue`：下载管理。
- `frontend/apps/admin/src/pages/TranscodeManagementPage.vue`：媒体处理管理。
- `frontend/apps/admin/src/pages/SystemLogsPage.vue`：系统日志。
- `frontend/apps/admin/src/pages/ViewerUserManagementPage.vue`：观看端用户启停与密码重置。
- `frontend/apps/admin/src/pages/ViewerInviteManagementPage.vue`：观看端邀请码生成与使用状态。
- `frontend/apps/admin/src/pages/ViewerSiteSettingsPage.vue`：观看端站点名称、注册策略和 favicon。
- `frontend/apps/admin/src/pages/ViewerCarouselManagementPage.vue`：首页轮播图新增、编辑、删除、图片上传、番剧绑定和排序。
- `frontend/apps/admin/src/pages/ViewerFilterManagementPage.vue`：番剧图书馆筛选维度、标签列表和排序。
- `frontend/apps/admin/src/pages/AppReleaseManagementPage.vue`：移动端 APP 版本列表、发布、修改、删除，以及 arm64-v8a APK 与更新日志管理。

前端约定：

- 使用 Composition API 和 `<script setup lang="ts">`。
- 新 API 类型必须写在 `api.ts`。
- 新页面必须处理 loading、空状态、禁用态和 API 错误。
- 图片使用受保护接口 `/api/anime/...`、`/api/actors/...`。
- 观看端和普通媒体接口不要暴露服务器本地绝对路径。
- 管理端系统设置、系统概览、番剧存储管理可以展示服务器路径。
- 番剧卡片按钮区保持固定宽度和可读文本，避免话数 tag 或按钮遮挡。
- “前端管理”下的观看端配置仍通过 Admin API 管理，必须使用管理员鉴权，不得复用观看端 Session。

## 观看端前端

观看端使用 Vue 3、TypeScript，当前不适配移动端，页面最小宽度按 1200px 设计。

关键文件：

- `frontend/apps/viewer/src/App.vue`：观看端入口、登录/注册门禁、站点设置应用和登录后首页壳。
- `frontend/apps/viewer/src/api.ts`：观看端 HTTP 调用和 API 类型。
- `frontend/apps/viewer/src/components/HomeScreen.vue`：顶部导航、首页轮播、热播、最近更新、我的追番以及页面/详情切换壳。
- `frontend/apps/viewer/src/components/ScheduleScreen.vue`：季度番剧时间表。
- `frontend/apps/viewer/src/components/LibraryScreen.vue`：番剧图书馆标签筛选与搜索。
- `frontend/apps/viewer/src/components/AnimeDetailScreen.vue`：详情、选集、播放、元数据、角色声优及追番入口。
- `frontend/apps/viewer/src/components/AnimeVideoPlayer.vue`：自定义 HTML5 播放器、网页全屏和播放进度上报。
- `frontend/apps/viewer/src/components/HistoryScreen.vue`：按话展示观看历史。
- `frontend/apps/viewer/src/components/FollowScreen.vue`、`FollowCard.vue`：我的追番列表和复用卡片。
- `frontend/apps/viewer/src/components/AppDownloadPage.vue`：公开的移动端 APP 最新版本下载落地页。
- `frontend/apps/viewer/src/assets/`：观看端本地图片、字体、样式依赖等静态资源。
- `backend/internal/httpapi/viewer.go`：观看端 API、用户认证、受控媒体接口和 SPA 托管。
- `backend/internal/viewer/service.go`：观看端用户、Session、站点设置和邀请码注册逻辑。
- `backend/internal/viewer/carousel.go`、`filter.go`：轮播图与图书馆筛选配置。
- `backend/internal/viewer/history.go`、`follow.go`：观看进度、历史和追番聚合。
- `backend/internal/bangumi/viewer_schedule.go`、`viewer_library.go`、`viewer_detail.go`：观看端时间表、图书馆和详情查询。

前端约定：

- 默认访问必须登录；未登录展示左侧登录/注册表单、右侧 `chara.png` 视觉区。
- 视觉基调保持白底、粉色主题、清透几何 UI 和轻量动效；第三方 CSS、JS、字体等资源必须本地化。
- 网站名称、favicon、注册开关和邀请码要求来自 `GET /api/site-settings`，不要在观看端硬编码站点标题。
- 注册接口必须尊重后端注册开关和邀请码策略；邀请码只在后端事务中校验并消费。
- 观看端认证与管理端认证相互独立，使用 viewer 用户和 viewer session。
- 除健康检查、站点设置、favicon、注册和登录外，首页、目录、详情、图片、媒体、历史与追番接口都要求观看端登录。
- APP 最新版本信息和 APK 下载接口为公开接口；H5 下载页路径固定为 `/app/download`，显示版本发布时间且不进入观看端登录门禁。
- 观看端页面当前由 `HomeScreen.vue` 内部状态切换；详情页使用 `/anime/{bangumiID}` 和 History API，未引入 viewer 端 vue-router。
- 观看端和普通媒体接口不得向浏览器暴露服务器本地绝对路径；视频、分集封面、番剧封面、角色和声优图片都通过受控 ID 接口读取。
- 首页数据首次加载后应复用已有状态；从详情返回时不要无条件完整刷新首页，避免骨架屏和轮播重置闪动。

### 首页、时间表与图书馆

- 首页 `GET /api/home` 返回热播推荐、最近更新、已配置轮播和当前用户追番聚合。
- 首页“最近更新”默认展示 24 个番剧卡片，对应 3 排。
- 轮播图片保存在 SQLite，绑定有效番剧；观看端轮播可进入对应详情页。
- 番剧时间表按日本动画季度 `1/4/7/10` 月切换，并用 `YYYY年M月` 精确匹配 `anime_tags`；周一到周日之外归入“其他”。
- 时间表和图书馆卡片都可展示没有产物的番剧，并按首播日期区分“尚未开播”和“尚未放流”。
- 图书馆标题搜索匹配原名、中文名和别名；同一筛选维度内为 OR，不同维度之间为 AND，标签匹配 `anime_tags.name`。
- 图书馆默认将存在 `media_jobs.status='completed'` 且产物路径非空的番剧排在前面。
- 顶部“搜索番剧”回车后切换到图书馆并应用关键词。

### 详情、播放与进度

- 详情接口组合番剧基础信息、分集元数据、已完成媒体、最后观看进度和追番状态；角色按主角、配角、其他顺序展示。
- 分集只有关联到 `media_jobs.status='completed'` 且 `output_path` 非空时才可播放；未产出的分集不可点击。
- 播放器右侧面板使用“选集 / 评论”双 Tab；评论接口按当前已完成 `mediaID` 在服务端映射 Bangumi episode ID，不接受客户端直接枚举任意 episode ID。
- 评论及同级楼中楼回复按源评论时间倒序；切换到评论 Tab 或切换当前选集时重新加载，并取消/忽略已经过期的请求。
- 评论正文不得使用 `v-html`。只以结构化节点渲染 `[s]`、`[mask]`、`[img]`、`[img=宽,高]` 和可用的 Bangumi 表情；未知或残缺标签、非法外链协议、缺失表情需要安全过滤。
- Viewer 番剧详情按 `bangumi_episode_comments` 中的本地主楼快照分组统计每话 `commentCount`，不使用可能存在时间差的元数据计数；评论 Tab 在尚未请求正文时显示该统计值，并在正文加载后使用评论接口的最新计数。
- 评论外链图片仅接受无账号凭据的 HTTP/HTTPS URL，并限制显示尺寸；遮罩内容支持 hover 和键盘 focus 后显示。
- 带尺寸参数的评论图片允许在同一行排列并自动换行，图片间距为 20px；无尺寸参数的评论图片保持独占一行。
- 评论用户签名为空或仅含空白时不渲染占位文案。
- 视频和分集封面必须同时校验 `bangumiID` 与 `mediaID`；视频由 `http.ServeFile` 提供 Range 播放，不在 JSON 中返回路径。
- 默认选择请求指定的媒体，否则恢复最后观看媒体，再否则选择第一个可播放分集。已完播记录从 0 秒开始，未完播记录恢复位置。
- 播放器每 10 秒以及暂停、结束、组件卸载时上报进度；实际播放不超过 15 秒不落库，达到时长 90% 记为完播。
- 自定义播放器支持普通全屏和网页全屏；网页全屏期间隐藏顶部导航。播放中 3 秒无交互会隐藏 UI，暂停、缓冲或报错时保持显示，隐藏态底部保留粉色进度线。
- 播放器进度条要展示浏览器实际 `video.buffered` 范围；活动 UI 的进度条和隐藏态底部进度条都要显示缓冲范围，不额外伪造或限制为固定 5 分钟。
- 详情标签先显示蓝色 Meta Tags；粉色普通 Tags 中与 Meta Tags 重复的项不再显示。

### 观看历史与追番

- 观看历史按话记录，同一用户和 `media_job_id` 唯一；列表按最后观看时间倒序并显示该番最新产物话数。
- 从历史进入详情时恢复到该话和进度；完播记录从该话起始位置播放。
- 详情页“追番”按钮写入 `viewer_anime_follows`。独立“我的追番”页展示全部追番，首页只展示尚未追到最新产物的项目。
- 追番恢复目标依次为：未完播的最后观看话、已有更新时的最新产物、未观看时的最早产物；没有产物时不生成可播放目标。
- “已追到最新”仅指最后观看话已完播且与当前最新产物为同一季、同类型、同话数。

## APP 端 / Tauri 移动端

移动端使用 Tauri v2 + Vue 3 + Android WebView。它是观看端的移动客户端，账号、追番、观看历史、番剧详情、媒体流和进度都必须复用 Viewer API 与同一套 SQLite 数据。

关键文件：

- `frontend/apps/mobile/src/App.vue`：移动端入口、站点初始化、登录门禁。
- `frontend/apps/mobile/src/config.ts`：API 域名配置，默认 `https://baka.vip/`，并支持 `public/app-config.json` 和本地覆盖。
- `frontend/apps/mobile/src/api.ts`：移动端请求封装、Viewer API 类型、Bearer token 保存和鉴权 URL 构造。
- `frontend/apps/mobile/src/components/MobileShell.vue`：登录后的主壳，底部导航、首页、时间表、图书馆、我的、搜索、追番、历史和详情页状态切换。
- `frontend/apps/mobile/src/components/MobileAnimeDetailScreen.vue`：移动端番剧详情页、选集、简介、元数据、角色声优与追番入口。
- `frontend/apps/mobile/src/components/MobileVideoPlayer.vue`：移动端自定义播放器、播放进度上报、手势和原生全屏桥接。
- `frontend/apps/mobile/src/components/AppUpdateDialog.vue`：移动端新版本提示、更新日志和下载/忽略操作。
- `frontend/apps/mobile/src/appUpdate.ts`：移动端语义版本比较、忽略版本持久化和下载页地址生成。
- `frontend/apps/mobile/src/native/player.ts`：调用 Tauri player 插件的前端桥接。
- `frontend/apps/mobile/src/native/opener.ts`：通过 Tauri opener 插件唤起系统默认浏览器。
- `frontend/apps/mobile/src/assets/`：移动端本地 SVG 图标和样式资源。
- `src-tauri/tauri.conf.json`：Tauri v2 配置，APP 名称、版本、bundle、Android minSdk 和 build hook。
- `src-tauri/src/lib.rs`：Tauri 插件注册和运行入口。
- `src-tauri/plugins/player/`：自定义 Tauri player 插件，Android 原生全屏、横屏、沉浸式和常亮屏幕能力在这里实现。
- `src-tauri/android-src/main/java/vip/baka/bangumipipeline/mobile/MainActivity.kt`：Android Activity 覆盖，安全区、WebView 自动播放、返回键和退出确认逻辑在这里实现。
- `src-tauri/android-res/`：Android 资源覆盖，包括应用名、图标、主题和启动窗口资源。
- `scripts/prepare-mobile-android.mjs`：构建前把 Android Java/Kotlin 与资源覆盖复制到 Tauri 生成工程。
- `scripts/build-android-apks.ps1`：构建多架构 APK、创建或读取本地签名文件、zipalign、apksigner，并输出到 `apk/`。

运行和打包：

- 本地开发运行 `npm run tauri:dev`，实际执行 `cargo tauri android dev`。
- Tauri dev/build 前会执行 `npm run prepare:mobile-android`，不要绕过该步骤修改生成目录。
- 只构建移动端 Web 资源运行 `npm run build:mobile` 或 `npm run build --workspace @bangumi-pipeline/mobile`。
- 打包签名 APK 运行 `npm run build:mobile:apk` 或 `powershell -ExecutionPolicy Bypass -File scripts/build-android-apks.ps1`。
- 生成的 `src-tauri/gen/android/` 是 Tauri 生成目录，原则上不要把功能改在这里；需要持久化的 Android 代码和资源放回 `src-tauri/android-src/`、`src-tauri/android-res/` 或插件目录。

移动端 API 和鉴权：

- 默认 API 基址为 `https://baka.vip/`，配置文件为 `frontend/apps/mobile/public/app-config.json`。
- 用户可在 APP 内保存 API 域名覆盖值；保存逻辑必须走 `normalizeAPIBaseURL`，保证尾部有 `/`。
- 移动端登录、注册、用户信息、首页、时间表、图书馆、详情、媒体、追番和历史都复用 Viewer API。
- 移动端认证使用 Viewer token，不使用管理端 Session，不使用客户端传入用户 ID。
- 受控媒体 URL 必须通过 `buildAuthenticatedMediaURL` 追加移动端 token；不得返回或暴露服务器本地路径。
- Web 观看端与移动端用户数据共享，后端接口改动必须同时考虑 `frontend/apps/viewer/src/api.ts` 和 `frontend/apps/mobile/src/api.ts` 的类型兼容。

移动端交互和布局约定：

- 移动端不是 1200px Web 观看端的缩放版，必须按手机 APP 交互重新设计。
- 主导航固定为底部 Tab：首页、时间表、图书馆、我的。
- 顶部栏、搜索栏、时间表季度/星期栏等固定元素滚动时应保持固定，并考虑 Android safe area。
- 四个主 Tab 的滚动高度互相独立，切换 Tab 不得串用上一页滚动位置。
- 详情、搜索、我的追番、观看历史等子页面使用内部 History API 管理返回栈；Android 返回键应先返回上一页。
- 根页面按返回键时先显示 “再按一次退出 BakaVip2”，3 秒内再次返回才退出。
- 进入新页面使用右侧滑入，返回上一页使用反向滑动；过渡期间不要让顶部栏消失或让旧页面滚动到顶部。
- APP 内隐藏浏览器滚动条，但不能禁用滚动能力。
- 视觉上保持白底、粉色点缀、轻量卡片和低厚重感；尽量少用粗体、少用 `h1`/`h2`。
- 番剧列表在手机端通常按 3 列展示；大量数据页面需要下滑加载或懒渲染，避免一次性渲染 1000+ 项导致卡顿。
- Loading 和骨架屏应优先使用移动端已有卡片骨架效果，不要回退成大段文字 loading。

移动端播放器约定：

- 播放器仍使用 HTML5 `<video>` 承载媒体流，视觉和控制层在 `MobileVideoPlayer.vue`。
- 点击播放器只显示/隐藏 UI；双击才切换播放/暂停。
- 媒体拿到有效 duration 前显示 “播放器加载中”，且不允许用户点击播放。
- 视频可控制后应尝试自动播放；Android WebView 的用户手势限制由 `MainActivity.kt` 关闭。
- 播放中要调用原生 `setKeepScreenOn(true)` 常亮屏幕，暂停、结束、卸载时关闭。
- 全屏必须通过 Tauri player 插件进入 Android 原生横屏沉浸式，而不是只依赖 WebView 默认全屏。
- 全屏时隐藏状态栏和导航栏，横屏方向允许随设备方向旋转，退出时恢复原方向和系统栏。
- 全屏时 Android 返回键应优先退出播放器全屏，而不是退出 APP。
- Android 可能因系统手势短暂显示导航栏；player 插件需要在全屏期间重复应用 immersive flags。
- 左右滑动播放器用于快进/快退，最长 10 分钟，极短滑动不生效；松手后才执行 seek。
- 进度上报规则与 Web 观看端保持一致：定时、暂停、结束、卸载时上报，短播放不落库，接近结尾才完播。
- 页面与顶栏的前进/返回动画统一由 `MobileShell.vue` 的共享过渡控制；进入和离开必须使用方向相反、距离相等的全宽位移，避免透明页面层重叠产生旧页面残留。
- APP 启动后异步读取公开的最新版本接口；只有服务端版本高于当前版本且未被忽略时才提示，不得阻塞启动和登录。
- 忽略版本只针对精确版本号；“关于 APP”中的手动检查必须绕过忽略状态。
- APP 下载页使用当前服务器地址拼接 `/app/download`，并通过 Tauri opener 插件交给系统默认浏览器打开。

Android 原生和资源约定：

- APP 名称默认为 `BakaVip2`，版本默认为 `1.1.0`，包名为 `vip.baka.bangumipipeline.mobile`。
- Tauri bundle 图标来自 `src-tauri/icons/icon.png` 和 `src-tauri/icons/icon.ico`。
- Android launcher 图标覆盖在 `src-tauri/android-res/mipmap-*`，不要只改 Tauri 默认图标而忘记 Android 资源。
- Android 主题、应用名、颜色、启动窗口资源覆盖在 `src-tauri/android-res/values*` 和 `drawable/`。
- 当前原生启动窗口只保留纯背景色，角色开屏由 WebView 内部 Loading 页负责；如需恢复原生角色开屏，必须同时更新 `splash_background.xml` 和准备脚本。
- `MainActivity.kt` 必须保持 `WindowCompat.setDecorFitsSystemWindows(window, true)`，避免普通页面与 Android 状态栏打架。
- 进入播放器原生全屏时，player 插件会临时切换为 `setDecorFitsSystemWindows(false)`；退出全屏必须恢复。
- Android 原生代码变更后，优先在 `src-tauri/gen/android` 下运行对应 Gradle 子任务验证，例如 `.\gradlew.bat :tauri-plugin-player:compileReleaseKotlin` 或 `.\gradlew.bat :app:processArm64ReleaseResources`。
- 签名文件和密码在 `src-tauri/android-signing/` 下本地生成，必须保持 gitignore，不得提交 keystore 或密码。

## HTTP API

Admin API 都在 `backend/internal/httpapi/admin.go`。

约定：

- 除健康检查、初始化和登录接口外，使用 `requireAdministrator` 保护管理接口。
- 错误响应使用 `writeError`。
- 错误格式：`{ "error": { "code": "...", "message": "..." } }`。
- 路径 ID 使用 `parsePathID`。
- 前端通过 `api.ts` 的 `request<T>` 请求，错误抛出 `APIError`。

关键接口：

- `GET /api/dashboard/overview`
- `GET/PUT /api/settings/network`
- `GET/PUT /api/settings/subscription`
- `GET/PUT /api/settings/download`
- `POST /api/settings/download/test`
- `GET/PUT /api/settings/llm`
- `POST /api/settings/llm/test`
- `GET/PUT /api/settings/media-storage`
- `GET/PUT /api/settings/bangumi-custom-search`
- `GET /api/viewer/users`
- `PATCH /api/viewer/users/{userID}`
- `POST /api/viewer/users/{userID}/password`
- `GET/POST /api/viewer/invites`
- `GET/PUT /api/viewer/site-settings`
- `GET/PUT /api/viewer/site-settings/favicon`
- `GET/POST /api/viewer/carousels`
- `PUT/DELETE /api/viewer/carousels/{carouselID}`
- `GET /api/viewer/carousels/{carouselID}/image`
- `GET/POST /api/viewer/filter-dimensions`
- `PUT/DELETE /api/viewer/filter-dimensions/{dimensionID}`
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
- `POST /api/anime/{bangumiID}/sync-episode`
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

Viewer API 在 `backend/internal/httpapi/viewer.go`。

约定：

- 仅需确认登录的接口使用 `requireViewer`；需要当前用户 ID 的历史、追番、详情与首页接口使用 `authenticatedViewer`。
- 错误响应沿用 `{ "error": { "code": "...", "message": "..." } }`，路径 ID 继续使用 `parsePathID`。
- 任何媒体、封面或人物图片接口只能返回文件内容，不得返回服务器路径。

关键接口：

- `GET /api/health`
- `GET /api/site-settings`
- `GET /api/auth/me`
- `POST /api/auth/register`
- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/home`
- `GET /api/anime-schedule?season=YYYY-MM`
- `GET /api/library/filters`
- `GET /api/library?q=...&filter={dimensionID}:{tag}`
- `GET /api/watch-history`
- `GET /api/follows`
- `GET /api/carousels/{carouselID}/image`
- `GET /api/anime/{bangumiID}/detail`
- `GET /api/anime/{bangumiID}/cover`
- `PUT /api/anime/{bangumiID}/follow`
- `GET /api/anime/{bangumiID}/media/{mediaID}/stream`
- `GET /api/anime/{bangumiID}/media/{mediaID}/cover`
- `GET /api/anime/{bangumiID}/media/{mediaID}/comments`
- `PUT /api/anime/{bangumiID}/media/{mediaID}/progress`
- `GET /api/bangumi-smiles/{code}`
- `GET /api/anime/{bangumiID}/characters/{characterID}/image`
- `GET /api/actors/{actorID}/image`
- `GET /favicon.png`

## 必须保持的行为

- 单进程、双端口、SQLite 单一事实源。
- 同一个计划任务不得并发或堆积执行。
- Bangumi API 请求串行且至少间隔 2 秒。
- Bangumi API 与图片请求超时为 20 秒。
- 每部番只处理 Characters API 返回的前 10 个角色。
- Bangumi 图片下载后压缩为 JPG，最大宽度 680px，quality 80。
- 图片 404 或空 URL 记录为 `not_found` 且不再重试。
- 自动匹配的订阅条目不能当作已绑定资源。
- 只有 `binding_status='bound'` 的订阅条目才能进入下载和媒体处理链路；时间表和图书馆允许展示尚无绑定或成品的元数据番剧。
- 下载任务创建前检查并发上限和 10GB 剩余磁盘空间。
- 下载失败不自动重试。
- qBittorrent 大队列状态同步必须保留完整读取上限和可诊断 JSON 错误信息。
- 媒体最终产物完成后再清理 qBittorrent 原下载任务和文件。
- ffmpeg 转码/压制保持单任务执行。
- 轻量 copy 任务可以在同一计划任务内连续处理，并允许与一个 ffmpeg 任务并行。
- MP4/H.264/AAC 不能直接等同于浏览器可播；slowstart、多个顶层 `mdat`、problematic edit list 必须 remux。
- 最终媒体产物必须使用番剧当前 `anime_metadata.media_storage_root`。
- 移动番剧存储路径必须拒绝转码中的番剧。
- 封面图生成失败不得让已完成的视频产物失败。
- 媒体任务触发的番剧元数据刷新每部番每天最多一次。
- LLM 翻译任务必须串行调用模型，不得记录 API KEY。
- 原文元数据变化时才清空对应译文字段；未变化不清空。
- 系统概览的磁盘空间必须实时探测，不写数据库缓存。
- 观看端和普通媒体接口不向前端暴露本地绝对路径。
- 观看端媒体流、媒体封面和进度写入必须同时校验番剧 ID 与媒体 ID，且只接受已完成产物。
- 播放进度不超过 15 秒不记录，达到 90% 才标记完播；不得仅由前端决定完播状态。
- 播放器缓冲条必须基于浏览器实际 buffered range，同时覆盖活动 UI 和隐藏态底部进度条。
- 观看历史和追番数据必须按当前 viewer 用户隔离，不能使用客户端传入的用户 ID。
- 移动端必须复用 Viewer API 和 Viewer 用户数据，不引入独立移动端账号、历史或追番表。
- 移动端默认 API 基址为 `https://baka.vip/`，本地覆盖必须经过规范化，不能硬编码到组件里。
- 移动端主 Tab 滚动位置必须互相独立；子页面返回必须回到上一页，而不是直接退出 APP。
- 移动端根页面返回必须二次确认退出，提示文本为 “再按一次退出 BakaVip2”。
- 移动端普通页面必须预留 Android 状态栏安全区；播放器原生全屏期间才允许隐藏系统栏。
- 移动端播放器全屏必须走 Android 原生桥接，保持横屏、沉浸式、返回键优先退出全屏和播放常亮屏幕。
- 移动端播放器点击只显示/隐藏控制 UI，双击才播放/暂停；视频 duration 未就绪时不得允许播放。
- 移动端 Android 持久化原生修改必须写入 `src-tauri/android-src/`、`src-tauri/android-res/` 或 `src-tauri/plugins/`，不要只改 `src-tauri/gen/android/`。
- 移动端签名 keystore、`signing.env`、APK 产物和本地打包输出不得提交。

## 命名约定

- 项目名：`BangumiPipeline`
- Go module：`bangumipipeline.local/server`
- npm workspace/package：`bangumi-pipeline`、`@bangumi-pipeline/*`
- 移动端 npm workspace：`@bangumi-pipeline/mobile`
- 移动端 APP 名称：`BakaVip2`
- 移动端默认版本：`1.1.0`
- 移动端 Android package identifier：`vip.baka.bangumipipeline.mobile`
- 移动端 Rust crate：`bangumi-pipeline-mobile`
- Docker 二进制：`bangumi-pipeline`
- 容器用户：`bangumipipeline`
- 新环境变量：`BP_*`

## AI 开发约定

- 修改代码前先读相关模块。
- 优先沿用现有分层、命名和错误处理方式。
- 搜索文件或文本优先使用 `rg` 或 `rg --files`。
- 手工编辑文件优先使用补丁方式。
- 不要顺手重写无关内容。
- 不要回滚用户或其他 Agent 已做出的无关改动。
- `.codex/` 和 `.codex-tmp/` 是本地 Codex 配置/临时文件目录，保持在 `.gitignore` 中，不提交。
- PowerShell 命令按当前 Windows 开发环境和工具权限策略执行。
- Go 文件变更后执行 `gofmt -w <修改过的 Go 文件>`。
- 除非当前任务明确要求，不要主动执行 `go test`、`npm run build:ui` 或 `npm test`。

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

只构建移动端 Web 资源：

```powershell
npm run build --workspace @bangumi-pipeline/mobile
```

同步 Tauri Android 覆盖资源：

```powershell
npm run prepare:mobile-android
```

验证 Android 原生播放器插件：

```powershell
cd src-tauri/gen/android
.\gradlew.bat :tauri-plugin-player:compileReleaseKotlin
```

验证 Android APP 资源：

```powershell
cd src-tauri/gen/android
.\gradlew.bat :app:processArm64ReleaseResources
```

构建并签名多架构 APK：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-android-apks.ps1
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
