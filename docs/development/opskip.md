# 片头识别与播放器 OP 跳过

代码路径相对仓库根目录。修改检测策略时先读 `backend/internal/opskip/service.go` 与相关测试。

- 独立任务为 `detect-media-openings`，注册在 `backend/cmd/server/main.go`，默认关闭；不在播放请求内运行音频分析。
- 只分析已完成且路径非空的正片成品，按番剧与季数分组；同组至少 3 话才进行交叉识别。
- 使用 ffmpeg Chromaprint 音频指纹，保留串行处理与任务防重，不添加同类重型并发。
- `media_op_fingerprints` 缓存指纹；命中依赖媒体 ID、文件大小、修改时间和指纹版本，源文件变化后重新生成。
- `media_op_group_analysis` 按番剧与季保存已完成分析的输入签名和检测版本。每轮只对成品做文件属性检查；成员和文件未变、版本一致且每个结果均为有效 detected/not_found 时整组复用，不重新匹配、静音分析或更新结果时间。没有需要重算的分组时也不启动 ffmpeg 能力检查。
- 分组签名覆盖具体媒体 ID、话数、路径、大小、纳秒修改时间、媒体完成时间和时长；新增、删除、同数量替换、路径/文件变化都会失效，不能只比较话数。指纹缓存新增 `file_mtime_ns` 校验；旧缓存首次命中仍按原有大小/秒级时间检查后补记纳秒值，避免升级强制重新生成所有指纹。
- 重算前删除旧分组完成记录；全部结果写入后重新读取成员和文件属性，只有输入始终一致、无指纹失败/静音分析失败/取消时才记录完成。失败、缺失或无效结果、检测期间成品变化均留待下轮重试；静音失败仍保留原有指纹结果作为播放回退，但不将该组标为可复用。
- 交叉识别、阈值、分析窗口或静音策略变化时必须递增 `group_cache.go` 的 `groupAnalysisVersion`；指纹版本变化也会使分组失效。重算仍使用整组视频进行交叉匹配，新集可以使此前 not_found 的话重新识别到 OP。
- 升级首次执行需要为已有分组复核一轮以建立可靠的完成记录，可复用旧指纹；之后服务重启也保留分组复用能力。文件检查不读取整段视频内容，若人为同时保留大小和修改时间修改文件，则无法仅凭文件属性识别这一变化。
- 任务完成日志中 `groups_scanned` 为有正片成品的分组数，`groups` / `episodes` 为本轮实际分析数，`groups_reused` / `episodes_reused` 为复用数，`groups_waiting` 为不足最小话数的分组数，`groups_retry` 为本轮未能写入完成记录的组数。`silence_analyzed` 为实际静音分析调用次数，`silence_failed` 为失败次数，`silence_adjusted` 仍只统计修改了结束点的次数；`fingerprint_cached` 仅统计实际重算分组中的指纹缓存命中数。
- `media_op_segments` 保存 pending/detected/not_found/failed、时间范围、置信度和匹配媒体；未知/失败不能当作有效 OP。
- 探测窗口、持续时间阈值、匹配算法和静音结束点校正以服务常量/实现为准，避免文档复制可调参数。
- 管理端状态在 `bangumi/catalog.go` 聚合，Viewer 详情在 `bangumi/viewer_detail.go` 输出可用片段。
- Web `AnimeVideoPlayer.vue` 与 APP `MobileVideoPlayer.vue` 使用服务端时间范围，保持缺失/无效结果回退及手动跳过行为。
- Web 用户是否显示 OP 按钮由 `playerPreferences.ts` 控制；不因关闭按钮删除服务端检测结果。
- 数据库变更仍进入带版本迁移；播放与进度规则见 [viewer.md](viewer.md)，原生控制见 [mobile.md](mobile.md)。

## 管理端进度

- `/openings` 为片头曲识别管理页，查询在 `backend/internal/opskip/management.go`；番剧详情的原有逐话展示仍在 `bangumi/catalog.go`。
- `GET /api/opskip/anime` 支持 `q` 标题/别名搜索、`status` 筛选、`page` 和 `pageSize`。只列有已完成、路径非空正片成品的未软删除番剧，按任务实际分析的视频记录计数；不将标称总话数、未完成产物或 SP 算入分母。
- 统计 detected（已检测到）、not_found（已分析但未找到符合规则的 OP）、pending（待识别）、waiting_episodes（等待足够话数）、failed（失败）。无结果记录视为 pending；仅 pending 且同番剧同季不足任务最小分组数量时派生 waiting_episodes，已有结果保留原状态。
- 总体状态优先显示失败、待识别、等待足够话数，其余为已完成。`status=completed` 表示全部为 detected/not_found；`pending` 筛选所有未完成番剧，包含失败与等待；`failed`、`waiting_episodes`、`not_found` 分别筛选含对应结果的番剧，可与未完成筛选范围重叠。先聚合与筛选，再分页。
- 展开行按需调用 `GET /api/opskip/anime/{bangumiID}`，按季/话数展示成品 ID、状态、有效片头范围、最近分析时间、等待原因与错误。等待条件沿用任务的最小分组常量；错误中的当前成品绝对路径替换为占位文本。
- 页面展示计划任务启用/运行情况、最近错误，采用手动刷新。任务复用未变化的完整分组结果，只重算输入变化或结果不完整的组；页面表示数据库保存的最近结果，不表示永久跳过、当前正在分析该番剧或文件已实时校验。单个视频失败也可能不使整个任务失败，以逐项结果为准。
- 页面只读，不提供跳过、编辑或单项重试操作；列表与展开请求不执行音频分析或逐文件检查，文件检查仅在后台任务进行。
