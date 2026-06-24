# BangumiPipeline 开发约定

本仓库是一个 Go 后端与 Vue 3 双前端组成的单体项目。修改代码时优先保持“单进程、双端口、SQLite 单一事实源”的架构，不要重新引入多个服务之间的数据同步。项目当前名称为 BangumiPipeline；代码、文档、前端品牌和部署标识应使用该名称。

## 项目结构

```text
backend/
  cmd/server/             服务入口
  internal/auth/          管理员与 Session
  internal/config/        环境变量配置
  internal/database/      SQLite 初始化和迁移
  internal/httpapi/       HTTP API、双端路由和 SPA 托管
  internal/system/        计划任务状态、调度器和系统设置
  internal/applog/        持久化系统日志与实时订阅
  internal/bangumi/       Bangumi 抓取、存储与番剧查询
  internal/subscription/  RSS 订阅抓取、番剧匹配、绑定和历史话数同步
frontend/apps/
  admin/                  管理端
  viewer/                 观看端
compose.yaml              Linux/本地容器部署
Dockerfile                多阶段生产构建
AGENTS.md                 项目架构、开发约定与验证要求
```

## 核心工作流

“从 bangumi.tv 抓取当季新番元数据”任务默认每 15 分钟执行一次，可在计划任务页面启用、修改间隔或立即执行。任务只处理 `type=2` 的动画，已存在的 Bangumi ID 会跳过，封面默认保存到 `data/images/bangumi`。

任务会通过 Subject 与 Characters API 补全简介、Tags、别名、话数、Infobox、评分集合、角色和配音信息。Infobox、Rating、Collection 等动态结构会保留 JSON；每部番只处理 Characters API 返回的前 10 个角色。详情和角色使用独立完成状态，失败后只重试缺失阶段。Bangumi API 请求严格串行且至少间隔 2 秒，单次 API 或图片请求超时为 20 秒。

演员按 Bangumi actor ID 在 `actors` 表中全局去重，角色通过 `character_actors` 关联。图片状态分为 `downloaded`、`failed` 和 `not_found`：临时失败会让任务失败并在下次重试，HTTP 404 会记录为永久不可用且不再请求。

“抓取订阅和匹配番剧”任务会读取 RSS 番剧订阅，入库新条目，并根据本地番剧名称、中文名和别名解析标题、季数、话数和集数类型。自动匹配成功的条目默认仍是待确认状态；用户确认或手动绑定后才会成为 `bound`。

“番剧管理”会在卡片上显示已绑定的话数标签。只有 `binding_status = 'bound'` 的订阅条目会显示，待确认匹配不会被当作已可用资源。卡片上的“同步历史话数”会使用该番剧最新的已绑定标题作为模板，删除模板中的话数数字后调用 Mikan RSS 搜索接口，自动补齐同标题模式、同季、同话类型且尚未绑定的历史条目。已绑定的话数不会替换，已忽略条目不会覆盖。

“订阅匹配管理”用于查看待确认、已绑定和已忽略的订阅条目，支持确认自动匹配结果、手动选择番剧与话数、忽略误匹配条目。手动绑定会保存标题记忆规则，后续同标题模式的订阅条目可自动绑定。

“系统日志”会从 SQLite 读取最近最多 1000 行，并通过 Server-Sent Events 实时追加新日志，支持 INFO、WARNING、ERROR 等级筛选。计划任务、Bangumi API 请求、RSS 抓取、历史话数同步和图片下载的开始、成功、失败都会写入日志。

Bangumi 要求 API 客户端使用可识别的 User-Agent。请将 `BP_BANGUMI_USER_AGENT` 设置为包含你个人 ID 和项目名的值，例如 `your-id/BangumiPipeline/0.1`。

如果不需要监听前端文件变化，可以使用：

```powershell
npm start
```

## 项目边界

- `backend/cmd/server`：进程入口，同时启动管理端 `:8080` 和观看端 `:8090`。
- `backend/internal/config`：运行配置。新环境变量使用 `BP_*`，旧 `AB_*` 仅作为兼容 fallback。
- `backend/internal/auth`：管理员账号、登录会话。
- `backend/internal/system`：计划任务状态、调度器、网络代理和系统设置。
- `backend/internal/applog`：SQLite 系统日志、日志查询和实时订阅。
- `backend/internal/bangumi`：Bangumi API 客户端、抓取流程、元数据存储和番剧目录查询。
- `backend/internal/subscription`：RSS 订阅抓取、标题解析、番剧匹配、人工绑定、标题记忆和历史话数同步。
- `backend/internal/httpapi`：管理 API、观看端 API、鉴权和静态 SPA 托管。
- `frontend/apps/admin`：Vue 3 + TypeScript + Element Plus 管理端。
- `frontend/apps/viewer`：观看端，目前仍是基础占位页。

## 必须保持的行为

- Bangumi API 请求必须串行，间隔至少 2 秒；API 与图片请求超时为 20 秒。
- 每部番只处理 Characters API 返回的前 10 个角色；角色对应的声优仍按 Bangumi actor ID 全局去重。
- 图片临时错误必须记录为 `failed` 并在后续任务重试；HTTP 404 记录为 `not_found` 且不再重试。
- 详情和角色抓取阶段相互独立，已成功的阶段不得因另一个阶段失败而重复抓取。
- 同一个计划任务不得并发或堆积执行。
- 管理 API、系统日志流和本地元数据图片必须要求管理员登录。
- Infobox、Rating、Collection 等动态字段保存原始 JSON，不要把 Bangumi 的可变 key 写死到数据库列中。
- 数据库使用 SQLite WAL，并保持 `MaxOpenConns(1)`；查询嵌套数据时先关闭外层 rows，避免单连接自锁。
- 默认新数据库路径是 `data/bangumi-pipeline.db`；未显式配置且检测到旧库 `data/autobangumi.db` 时必须继续沿用旧库，避免改名造成数据丢失感。

## 订阅匹配与历史话数

- RSS 订阅抓取使用计划任务 `subscription-rss-match`，名称为 `抓取订阅和匹配番剧`。
- 自动匹配成功的订阅条目默认仍是待确认状态；只有 `binding_status = 'bound'` 的条目可视为已确认绑定。
- 番剧管理卡片只显示已绑定话数，不能显示 `pending` 自动匹配结果。
- 人工确认或手动绑定会写入 `bound_*` 字段，并保存标题记忆规则；后续相同标题模式可以自动绑定。
- 同一番剧、季数、话类型、话数只能有一个已绑定条目；新增绑定不得静默覆盖已有绑定。
- “同步历史话数”只能以该番剧最新已绑定条目为模板，删除模板标题中的话数数字后调用 Mikan `RSS/Search`。
- 历史话数同步只绑定标题记忆 key 一致、同季、同话类型且尚未绑定的条目；已绑定条目跳过，已忽略条目不覆盖。
- Mikan 搜索结果解析复用现有 RSS item 结构和订阅入库逻辑；网络请求使用系统代理配置和 20 秒超时。
- 请求路径上的正则不得使用 `regexp.MustCompile` 处理动态模式；Go `regexp` 不支持 lookahead/lookbehind 等 Perl 语法。

## 数据库变更

所有 schema 变更都放在 `backend/internal/database/database.go`，并增加新的 `schema_migrations` 版本。迁移必须兼容现有数据库，不可要求用户删除数据库重建。涉及清理或重算数据的迁移必须只执行一次。

## 系统日志

业务代码使用 `log/slog`，并提供 `source` 属性，例如 `bangumi`、`subscription`、`scheduler`、`http`。级别只使用 INFO、WARNING、ERROR；Go 中 WARNING 对应 `logger.Warn`。网络抓取、RSS 抓取、历史同步和文件下载至少记录开始、成功、失败三个关键节点。

## 前端约定

- 使用 Composition API、`<script setup lang="ts">` 和明确的 API 类型。
- HTTP 调用统一放在 `frontend/apps/admin/src/api.ts`。
- 管理端路由统一放在 `frontend/apps/admin/src/router.ts`。
- 不直接暴露本地磁盘路径；图片使用 `/api/anime/...` 或 `/api/actors/...` 的受保护接口。
- 新页面或新操作需要处理 loading、空数据、禁用态和 API 错误，并兼顾窄屏布局。
- 番剧卡片按钮区需要保持固定宽度和可读文本；新增操作不要让话数标签或按钮互相遮挡。

## 命名与配置

- 对外项目名使用 `BangumiPipeline`。
- Go module 使用 `bangumipipeline.local/server`。
- npm 包使用 `bangumi-pipeline` 和 `@bangumi-pipeline/*`。
- Docker 二进制使用 `bangumi-pipeline`，容器内用户使用 `bangumipipeline`。
- 新环境变量统一使用 `BP_*`。旧 `AB_*` 可以保留在配置读取中作为兼容层，但不要在新文档或新示例中继续作为首选。
- 不要提交个人代理地址、账号、Cookie 或 Bangumi Token。

## 验证命令

```powershell
gofmt -w <修改过的 Go 文件>
go test ./backend/...
npm run build:ui
```

如果只需要编译后端而不运行测试，可使用：

```powershell
go build -buildvcs=false ./backend/...
```

完整验证也可以运行 `npm test`。Vite 当前可能输出 Element Plus 相关的 chunk 大小和 `INVALID_ANNOTATION` 警告；只要构建退出码为 0，这些属于已知非阻断警告。

## 本地网络

安装依赖遇到网络问题时，可在当前 PowerShell 会话临时设置：

```powershell
$env:HTTP_PROXY='http://localhost:10808'
$env:HTTPS_PROXY='http://localhost:10808'
```

不要把个人代理地址、账号、Cookie 或 Bangumi Token 提交到代码中。