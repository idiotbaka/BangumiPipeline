# 片头识别与播放器 OP 跳过

代码路径相对仓库根目录。修改检测策略时先读 `backend/internal/opskip/service.go` 与相关测试。

- 独立任务为 `detect-media-openings`，注册在 `backend/cmd/server/main.go`，默认关闭；不在播放请求内运行音频分析。
- 只分析已完成且路径非空的正片成品，按番剧与季数分组；同组至少 3 话才进行交叉识别。
- 使用 ffmpeg Chromaprint 音频指纹，保留串行处理与任务防重，不添加同类重型并发。
- `media_op_fingerprints` 缓存指纹；命中依赖媒体 ID、文件大小、修改时间和指纹版本，源文件变化后重新生成。
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
- 页面展示计划任务启用/运行情况、最近错误，采用手动刷新。现有任务每轮重新分析满足条件的分组并复用有效指纹；页面表示数据库保存的最近结果，不表示永久跳过、当前正在分析该番剧或文件已实时校验。单个视频失败也可能不使整个任务失败，以逐项结果为准。
- 本期不修改检测策略、不新增 schema，不提供跳过、编辑或单项重试操作；列表与展开请求不执行音频分析或逐文件检查。
