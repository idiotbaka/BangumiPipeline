# Bangumi 元数据、评论与表情

本文按需读取；代码路径相对仓库根目录。修改相关行为时同步更新本文。


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

## 分类、话数与头像

- Bangumi 的 `type=2` 表示动画主体；本地 `anime_type` 另区分 `series` 与 `movie`，不要混用两者。
- 电影分类根据 Subject Tags / Meta Tags 识别；管理员设置 `anime_type_manual=1` 后，元数据刷新保留手动分类。
- Viewer 的正片总话数优先 `eps`，缺失时统计本地 `anime_episodes.type=0`；不要用可能包含 SP 的 `total_episodes` 替代。
- “已完结”标记与用户“已追到最新”含义不同；具体聚合规则见 [viewer.md](viewer.md)。
- 评论用户头像由 `comment_avatars.go` 下载到本地，经受控头像接口读取，不在各客户端分别维护上游头像抓取。
