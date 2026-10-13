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
