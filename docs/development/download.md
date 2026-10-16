# 下载与 qBittorrent 清理

本文按需读取；代码路径相对仓库根目录。修改相关行为时同步更新本文。


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
- 仅对选择了磁力重试的任务，优先使用 `download_jobs.retry_magnet_url`；其他任务保留上述优先级。该字段随任务持久保存，服务重启或再次普通重试均不会切回 torrent URL，原订阅链接不修改。
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

磁力链接重试：

- 管理端“下载失败”列表的 `重试(改用磁力链接)` 调用 `POST /api/download/jobs/{jobID}/retry-magnet`，可传 `{ "magnetUrl": "magnet:?..." }`；未传地址时先尝试本地信息，找不到返回 `422 download_magnet_required`，前端再弹窗填写，取消弹窗不会修改任务。
- 自动查找先检查该任务保存的磁力重试地址、实际提交地址及订阅 enclosure/torrent/link/GUID 中的有效 magnet，再检查描述中的 magnet，随后尝试从受支持的 Mikan 详情链接生成 BTIH，最后可从此前记录的有效 qBittorrent hash 生成。已有完整 magnet 保留 trackers 等参数，不依赖在线抓取种子文件。
- 服务端校验 magnet 结构及 BTIH（40 位十六进制或 32 位 Base32），统一 hash 为十六进制便于匹配；非法地址返回 `400 invalid_magnet_url`，不会清理 qBittorrent 或重置任务。
- 仅允许仍为 bound、来源为 download、尚无媒体任务的失败任务。有效地址取得后核对 qBittorrent：失败任务删除但保留文件；同 hash 正常任务保留，下一轮调度核对复用。成功后只将选中任务重置为 `pending`，清除旧进度、hash、错误和时间，保存磁力地址，由下载计划任务按原并发和磁盘限制创建任务。
- 磁力提交前先按 hash 检查已存在任务，提交后最多轮询约 20 秒确认；HTTP 源仍沿用原确认时间。相同 hash 任务只有在保存目录匹配后才复用，目录不一致时明确报错，避免把服务器其他目录的文件误当作本任务产物。
- 提交日志记录下载方式，未能匹配时记录 hash 和任务 ID。重试、撤销、状态同步及开始下载共用 `activeMu`，避免同一进程内交错操作。

完成清理：

- 媒体产物完成后，`media.Service` 调用 `download.Service.CleanupCompletedQBitTask`。
- 媒体任务也会兜底重试已完成产物但下载清理失败的历史任务，避免 qBittorrent 中长期堆积已完成 torrent。
- 删除 qBittorrent torrent，`deleteFiles=true`。
- 尽量删除 `bp-item-<subscription_item_id>` 标签。
- 清理失败不回滚已完成媒体产物，只记录 warning。

## 撤销与容器路径

- 下载列表的“撤销”由 `CancelJob` 处理，只接受下载中或失败、仍为 bound、且尚无媒体任务的下载源。
- 先确认 qBittorrent 状态并删除对应任务/文件，再在事务中删除下载记录、将订阅回退待绑定；qBittorrent 操作失败不能静默解除绑定。
- 保留 `activeMu` 与事务中的状态复核，避免撤销和调度并发改变同一任务。
- qBittorrent 与后端处于不同容器挂载时，沿用 `QBitDownloadDir` / `qBitSavePath` 路径映射，不能假定两边绝对路径相同。
