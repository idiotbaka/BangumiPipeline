# RSS、绑定与补源

本文按需读取；代码路径相对仓库根目录。修改相关行为时同步更新本文。


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

- 支持 `rss` 和 `local` 两种同步方式；未传同步方式时兼容为 `rss`。
- 使用该番剧最新已绑定标题作为模板。
- 删除模板标题中的话数数字后调用 Mikan `RSS/Search`。
- 只绑定标题记忆 key 一致、同季、同话类型且尚未绑定的条目。
- 已绑定跳过，已忽略不覆盖。
- 网络请求使用系统代理配置，超时 20 秒。
- 本地媒体方式要求服务器绝对文件夹路径，递归扫描支持的视频扩展名，并按文件名应用包含/过滤字段。
- 本地媒体文件名复用订阅标题的季数、集类型和集数识别以及番剧集数偏移；同批次重复集数和已有绑定必须跳过，不静默覆盖。
- 本地媒体使用 `download_jobs.source_type='local'` 记录已完成采集，并直接创建 `media_jobs(status='pending')`，不进入 qBittorrent 下载或下载管理列表。
- 本地媒体处理完成后必须保留原始文件，不得调用 qBittorrent 文件清理。

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

## 集数偏移与电影补源

- 番剧设置中的 `subscription_episode_offset` 用于将资源集数转换为本地话数；自动匹配、历史补源、绑定与产物命名要保持一致。
- 电影历史补源在应用包含/过滤条件后，只有候选恰好一个时才按 `Season 1 / episode / 01` 绑定。
- 该单候选规则同时适用于 RSS 与本地文件，允许电影文件名没有集数，并跳过集数偏移。
- 多候选电影继续使用正常话数识别与重复检查，不能把全部候选强制绑定为同一话。
- 已有绑定、忽略项、季数/类型和标题记忆校验仍适用；不要扩大单候选电影的特例范围。
- 已绑定话数支持编辑与解除，入口在 `httpapi/admin.go` 的 `updateAnimeEpisodeBinding` / `deleteAnimeEpisodeBinding`；改动前核对既有任务和产物的处理逻辑。
