# 媒体处理与存储

本文按需读取；代码路径相对仓库根目录。修改相关行为时同步更新本文。


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
- `download_jobs.source_type='local'` 的任务直接使用 `save_path` 指向的本地视频，并在完成后跳过下载清理。
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
- 字幕由 `selectSubtitle` 统一选择：优先匹配 basename 的中文外挂，其次可用内封字幕，最后外挂回退；保留现有语言与扩展名排序。
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

## 编码与路径处理

- 重新编码时沿用现有码率上限与最大分辨率策略；缩放保持比例、不放大低分辨率源，copy/remux 不应用编码限幅。
- 参数与阈值以 `service.go` 中的常量和 `ffmpegArgs` 为准，改变策略时同时检查规划与执行阶段。
- 内封字幕按 title/language 优先简体、繁体和其他中文轨道，保留 `si` 的字幕序号语义。
- 字幕滤镜路径使用 `ffmpegFilterPath` 的两层转义，覆盖引号、反斜杠、冒号、方括号、逗号、分号；仅 shell 转义无法解决滤镜路径解析。
- 片头识别是独立任务，不属于媒体转码，见 [opskip.md](opskip.md)。
