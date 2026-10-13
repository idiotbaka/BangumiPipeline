# BangumiPipeline AI 开发指南

本文件只保留全仓库共识、代码入口和按需阅读索引。
先读本次任务涉及的代码与文档，再沿用现有分层、命名和错误处理方式。

## 项目与架构

BangumiPipeline 是 Go 后端、Vue 3 双 Web 前端与 Tauri Android 客户端组成的单体项目。
它集合番剧元数据刮削、RSS 磁力订阅下载、视频转码与产物管理，并提供 Web 和 APP 观看入口。

- 后端保持单进程、双端口：管理端默认 `:8080`，观看端默认 `:8090`。
- SQLite 是唯一事实源；不引入独立移动端后端、数据源或跨服务数据同步。
- 移动端复用 Viewer API、用户、观看历史与追番数据。
- 管理员认证与 Viewer 认证独立；观看端用户数据始终按当前登录用户隔离。
- 只有 `subscription_items.binding_status='bound'` 的资源进入下载和媒体链路。
- 时间表、图书馆和管理端目录允许展示尚无绑定或产物的元数据番剧。

## 目录地图

```text
backend/
  cmd/server/             依赖装配、计划任务注册、两个 HTTP server
  internal/auth/          管理员账号与 Session
  internal/config/        环境变量（新配置 BP_*，AB_* 仅旧配置兼容）
  internal/database/      SQLite、迁移、Writer/Reader 路由
  internal/httpapi/       Admin/Viewer API、鉴权、图片/媒体、SPA
  internal/system/        调度器、计划任务状态、系统设置
  internal/applog/        SQLite 日志与 SSE
  internal/bangumi/       元数据、分集、评论/表情与目录查询
  internal/subscription/  RSS、标题解析、匹配、绑定、补源
  internal/download/      qBittorrent 创建、同步、重试、清理
  internal/media/         产物识别、复制/remux/转码、封面、存储移动
  internal/opskip/        片头识别
  internal/translation/   OpenAI Chat 兼容 LLM 翻译
  internal/viewer/        Viewer 用户、站点、历史、追番、APP 发布、推送
  internal/imageutil/     图片处理工具
  internal/mediafile/     媒体文件工具
frontend/
  apps/admin/             Vue 3 + TypeScript + Element Plus 管理端
  apps/viewer/            Vue 3 + TypeScript 桌面 Web 观看端
  apps/mobile/            Tauri WebView 前端（含 TV 目标）
  shared/                 Web/APP 共用逻辑，如评论内容解析
src-tauri/                Rust 壳、Android 覆盖、原生插件、资源
scripts/                  运维修复、Android 准备与 APK 构建
docs/development/         按业务边界读取的开发说明
compose.yaml / Dockerfile 容器部署与生产构建
```

## 指令与阅读方式

- 根文件适用于全仓库；各目录的 `AGENTS.md` 只补充该目录的开发约定。
- 修改某个目录前，读取从仓库根到目标目录路径上尚未读取的 `AGENTS.md`。
- 从根目录启动时也执行上述步骤，不假定子目录指令已自动加载。
- 按下表阅读本次任务对应的文档；跨边界改动读取所有相关项，无需通读全部文档。
- 本文链接相对仓库根目录；子目录指令中的链接相对各自文件。
- 文档组织与维护方式见 [开发文档索引](docs/development/README.md)。
- 文档说明现有行为与关键约束；用户明确要求改变行为时，同步修改实现和对应文档。

| 涉及边界 | 阅读文档 | 首先查看 |
| --- | --- | --- |
| 服务装配、配置、调度、日志、系统概览 | [backend.md](docs/development/backend.md) | `backend/cmd/server/main.go`、`backend/internal/system/` |
| SQL、连接池、迁移、查询性能 | [database.md](docs/development/database.md) | `backend/internal/database/` |
| Admin/Viewer API、鉴权、媒体接口 | [http-api.md](docs/development/http-api.md) | `backend/internal/httpapi/admin.go`、`viewer.go` |
| Bangumi 抓取、刷新、分集、评论同步、表情 | [bangumi.md](docs/development/bangumi.md) | `backend/internal/bangumi/` |
| LLM 翻译、翻译设置与提示词 | [translation.md](docs/development/translation.md) | `backend/internal/translation/service.go` |
| RSS 匹配、绑定、历史补源、单话替换 | [subscription.md](docs/development/subscription.md) | `backend/internal/subscription/service.go` |
| qBittorrent 下载与清理 | [download.md](docs/development/download.md) | `backend/internal/download/` |
| 复制/remux/转码、封面、存储移动 | [media.md](docs/development/media.md) | `backend/internal/media/service.go` |
| 片头识别、指纹缓存、OP 跳过 | [opskip.md](docs/development/opskip.md) | `backend/internal/opskip/`、Web/APP 播放器 |
| 管理端页面与交互 | [admin.md](docs/development/admin.md) | `frontend/apps/admin/src/` |
| Web 观看端或 Viewer 业务、进度、追番、评论展示 | [viewer.md](docs/development/viewer.md) | `frontend/apps/viewer/src/`、`backend/internal/viewer/` |
| APP 页面、播放器、原生桥接、Android 资源 | [mobile.md](docs/development/mobile.md) | `frontend/apps/mobile/`、`src-tauri/` |
| 运维脚本、容器部署、构建与签名 | [operations.md](docs/development/operations.md) | `scripts/`、根 `package.json`、`Dockerfile` |

## 数据库底线

- 保持 WAL；唯一 Writer 为 `MaxOpenConns(1)`、`MaxIdleConns(1)`。
- 所有事务与 INSERT/UPDATE/DELETE 走 Writer，不在 Reader 上写入或开启事务。
- Viewer、Admin、Worker Reader 独立，默认连接数为 4、1、1。
- Reader 使用 `mode=ro` 和 `query_only=ON`；HTTP 读请求按上下文路由，后台默认 Worker。
- 连接级 PRAGMA 写入 DSN 或逐连接初始化，不能假定连接池中的新连接继承一次执行的设置。
- schema 变更集中在 `backend/internal/database/database.go`，新增 `schema_migrations` 版本。
- 迁移兼容已有数据库；清理、重算、补默认值只执行一次，不要求删库重建。
- 嵌套查询前关闭外层 `rows`，避免占用 Reader 或阻止 WAL checkpoint。
- 表结构与索引以迁移代码为准；表职责地图见数据库文档。

## 后端与任务底线

- 路由在 `NewAdminHandler` / `NewViewerHandler` 装配，业务逻辑沿用现有 Service 分层。
- 错误响应沿用 `writeError`；路径 ID 用 `parsePathID`。
- Admin 普通 GET 使用 15 秒上下文期限，系统日志 SSE 不使用该期限。
- 同一计划任务不得并发或堆积执行，保留 Scheduler 内存与数据库双重保护。
- Bangumi API 请求串行，间隔至少 2 秒；API 与图片请求超时为 20 秒。
- ffmpeg 重型任务保持单任务执行；现有 copy 与一个 ffmpeg 并行策略见媒体文档。
- 业务日志使用 `log/slog` 并带 `source`，级别为 INFO、WARNING、ERROR。
- 外部请求沿用系统网络代理配置；动态正则用 `regexp.Compile` 处理错误。

## 各端开发入口

- Vue 使用 Composition API 与 `<script setup lang="ts">`。
- 请求函数和 API 类型集中在各端 `src/api.ts`，保留 loading、空状态、禁用态和错误处理。
- Admin 导航由 `src/router.ts` 与 `src/components/AdminLayout.vue` 管理。
- Web 观看端按桌面最小宽度 1200px 设计；页面切换沿用 `HomeScreen.vue` 和 History API。
- APP 按手机交互设计；页面栈在 `MobileShell.vue`，原生能力通过 `src/native/` 桥接。
- Viewer API 变更同时检查 Web 与 APP 的 `api.ts`、媒体鉴权和用户数据兼容。
- 共用评论解析在 `frontend/shared/commentContent.ts`，表情代码与清单在后端 `bangumi/smiles.go`。
- 站点标题、favicon、注册策略来自 API；第三方 CSS、JS、字体等静态资源本地化。
- Web/APP 保留浅色与深色主题，沿用各端 `theme.ts` / `theme.css` 和首屏初始化。
- 持久 Android 改动写入 `src-tauri/android-src/`、`android-res*/` 或 `plugins/`，再由准备脚本同步。
- `src-tauri/gen/android/` 是生成目录，不能作为持久功能修改的唯一来源。

## 鉴权与数据安全

- 管理 API、日志流和本地元数据图片要求管理员登录，保留现有公开初始化/登录/健康检查例外。
- Viewer 公开接口例外见 API 文档，其余目录、详情、图片、媒体、历史与追番要求 Viewer 登录。
- 历史、追番等用户 ID 从服务端认证取得，不信任客户端传入的用户 ID。
- 媒体接口同时校验番剧 ID、媒体 ID 和已完成状态。
- 观看端与普通媒体接口不暴露服务器绝对路径；管理端系统设置、概览、存储管理可显示路径。
- 不提交个人代理地址、账号、Cookie、Token、qBittorrent 凭据、LLM API KEY 或 Android 签名密码。
- `.codex/`、`.codex-tmp/`、运行数据、keystore、`signing.env`、APK 和打包输出保持忽略。

## 修改与验证

- 搜索优先使用 `rg` / `rg --files`，手工编辑优先使用补丁。
- 不顺手重写无关内容，不回滚用户或其他 Agent 已做出的无关改动。
- Go 文件修改后执行 `gofmt -w <修改过的 Go 文件>`。
- 沿用项目偏好：除非任务明确要求，不主动执行 `go test`、`npm run build:ui` 或 `npm test`。
- 需要验证时选择受影响范围的命令；完整命令与 Android 环境要求见运维文档。
- 仅文档改动检查链接、路径、内容覆盖与 diff，无需运行应用构建。
- 业务规则更新到对应开发文档；只有全仓库共识进入根文件，局部入口进入子目录指令。
- 根文件目标 100–200 行，子目录指令保持简短，详细规则不重复粘贴到多份指令中。
