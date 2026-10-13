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

## 撤销与容器路径

- 下载列表的“撤销”由 `CancelJob` 处理，只接受下载中或失败、仍为 bound、且尚无媒体任务的下载源。
- 先确认 qBittorrent 状态并删除对应任务/文件，再在事务中删除下载记录、将订阅回退待绑定；qBittorrent 操作失败不能静默解除绑定。
- 保留 `activeMu` 与事务中的状态复核，避免撤销和调度并发改变同一任务。
- qBittorrent 与后端处于不同容器挂载时，沿用 `QBitDownloadDir` / `qBitSavePath` 路径映射，不能假定两边绝对路径相同。
