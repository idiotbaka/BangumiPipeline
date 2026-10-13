# Go 后端开发

适用于 `backend/`。继承根指令；以下路径相对本文件。

- 修改装配、配置、任务或日志前读 [后端说明](../docs/development/backend.md)。
- 修改 SQL、连接池、迁移或查询性能前读 [数据库说明](../docs/development/database.md)。
- 修改路由、鉴权或媒体接口前读 [API 说明](../docs/development/http-api.md)。
- 按业务读取 [Bangumi](../docs/development/bangumi.md)、[翻译](../docs/development/translation.md)、[订阅](../docs/development/subscription.md)、[下载](../docs/development/download.md)、[媒体](../docs/development/media.md)。
- Viewer 业务、目录查询、评论展示与计数、播放进度读 [观看端说明](../docs/development/viewer.md)，同时检查 Web/APP 调用端。
- 片头检测与 OP 跳过读 [片头识别说明](../docs/development/opskip.md)，不要在播放请求中执行分析。

## 实现约定

- `cmd/server/main.go` 负责依赖装配和任务注册；`internal/httpapi/` 负责 HTTP 边界。
- 沿用 Service 分层与 `database.Executor`，事务和业务写入走唯一 Writer。
- 保留按上下文选择 Reader 的方式，不绕过连接路由。
- schema 与一次性修复进入带版本的迁移，不在请求或每轮任务中反复执行。
- 保留任务内存/数据库防重、外部请求限速与媒体文件锁。
- 下载完成、媒体完成、清理与评论/推送回调的顺序按各业务文档处理。
- 修改后格式化对应 Go 文件；构建/测试命令见 [运维说明](../docs/development/operations.md)。
