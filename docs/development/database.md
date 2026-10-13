# 数据库架构与迁移

本文按需读取；代码路径相对仓库根目录。修改相关行为时同步更新本文。


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
- `viewer_comment_username_filters`：观看端吐槽过滤名单；同时按 Bangumi `username` 或 `nickname` 全匹配。
- `viewer_carousel_items`：首页轮播图二进制图片、绑定番剧和排序。
- `viewer_filter_dimensions`、`viewer_filter_tags`：番剧图书馆可配置的筛选维度与标签。
- `viewer_watch_history`：按用户和成品媒体记录播放位置、时长、完播状态及最后观看时间。
- `viewer_anime_follows`：用户追番关系；同一用户和番剧唯一。
- `viewer_app_releases`：移动端 APP 版本、更新日志、arm64-v8a APK、文件大小与 SHA-256。

## 其他已实现的数据边界

- `media_op_fingerprints`、`media_op_segments`：成品音频指纹缓存与片头识别结果。
- `viewer_web_push_settings`、`viewer_web_push_subscriptions`、`viewer_web_push_deliveries`：VAPID 密钥、按用户订阅与待投递队列。
- `viewer_invitation_codes.created_by_user_id` 记录用户自助邀请码来源，与管理员生成的码共用同一表。
- `viewer_site_settings.active_single_page_key` 保存启用的内置专题页 key；专题内容随前端构建发布。
- `anime_metadata.anime_type` 区分 `series` / `movie`，`anime_type_manual` 保留手动分类；订阅集数偏移在 `subscription_episode_offset`。
- 目录查询需遵守现有 `deleted_at` 软删除边界；完整字段、约束、索引与迁移版本以 `database.go` 为准。
