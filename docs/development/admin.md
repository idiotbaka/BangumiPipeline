# 管理端 Web

本文按需读取；代码路径相对仓库根目录。修改相关行为时同步更新本文。


管理端使用 Vue 3、TypeScript、Element Plus。

关键文件：

- `frontend/apps/admin/src/api.ts`：所有 HTTP 调用和 API 类型。
- `frontend/apps/admin/src/router.ts`：路由。
- `frontend/apps/admin/src/components/AdminLayout.vue`：侧边栏。
- `frontend/apps/admin/src/pages/DashboardPage.vue`：系统概览。
- `frontend/apps/admin/src/pages/SettingsPage.vue`：网络、LLM、下载、订阅、额外存储。
- `frontend/apps/admin/src/pages/ScheduledTasksPage.vue`：计划任务、自定义 Bangumi 标签抓取设置。
- `frontend/apps/admin/src/pages/AnimeListPage.vue`：番剧管理、搜索、排序、刷新、历史同步、单话同步/替换、存储移动。
- `frontend/apps/admin/src/components/AnimeDetailPanel.vue`：番剧详情、角色/声优、分集展示。
- `frontend/apps/admin/src/pages/SubscriptionMatchesPage.vue`：订阅匹配。
- `frontend/apps/admin/src/pages/DownloadManagementPage.vue`：下载管理。
- `frontend/apps/admin/src/pages/TranscodeManagementPage.vue`：媒体处理管理。
- `frontend/apps/admin/src/pages/TranslationManagementPage.vue`：按番剧展示六类元数据中文内容进度、元数据抓取提示与只读明细，规则见 [translation.md](translation.md)。
- `frontend/apps/admin/src/pages/OpeningManagementPage.vue`：按番剧汇总 OP 检测结果，展开显示每季每话的等待原因、时间范围和错误，规则见 [opskip.md](opskip.md)。
- `frontend/apps/admin/src/pages/SystemLogsPage.vue`：系统日志。
- `frontend/apps/admin/src/pages/ViewerUserManagementPage.vue`：观看端用户启停与密码重置。
- `frontend/apps/admin/src/pages/ViewerInviteManagementPage.vue`：观看端邀请码生成与使用状态。
- `frontend/apps/admin/src/pages/ViewerSiteSettingsPage.vue`：观看端站点名称、注册策略和 favicon。
- `frontend/apps/admin/src/pages/ViewerCarouselManagementPage.vue`：首页轮播图新增、编辑、删除、图片上传、番剧绑定和排序。
- `frontend/apps/admin/src/pages/ViewerFilterManagementPage.vue`：番剧图书馆筛选维度、标签列表和排序。
- `frontend/apps/admin/src/pages/AppReleaseManagementPage.vue`：移动端 APP 版本列表、发布、修改、删除，以及 arm64-v8a APK 与更新日志管理。

前端约定：

- 使用 Composition API 和 `<script setup lang="ts">`。
- 新 API 类型必须写在 `api.ts`。
- 新页面必须处理 loading、空状态、禁用态和 API 错误。
- 图片使用受保护接口 `/api/anime/...`、`/api/actors/...`。
- 观看端和普通媒体接口不要暴露服务器本地绝对路径。
- 管理端系统设置、系统概览、番剧存储管理可以展示服务器路径。
- 番剧卡片按钮区保持固定宽度和可读文本，避免话数 tag 或按钮遮挡。
- “前端管理”下的观看端配置仍通过 Admin API 管理，必须使用管理员鉴权，不得复用观看端 Session。
- 翻译与 OP 管理页支持搜索、进度筛选、分页与手动刷新；`ProgressTaskStatus.vue` 展示对应计划任务状态，明细按展开行加载。第一期只读，不提供编辑、跳过或手动执行操作。

## 后续功能入口

- `ViewerSinglePageManagementPage.vue` 管理内置专题页启用 key，清单在后端 `viewer/single_pages.go`，不接受任意 HTML 或 URL。
- 番剧设置包含资源集数偏移与 `series/movie` 分类，API 在 `catalog.UpdateSettings`。
- 已绑定话数可编辑/解除，下载任务可撤销；UI 禁用态与错误提示遵守服务端对任务状态的限制。
- 下载失败列表同时保留“重试”“重试(改用磁力链接)”和“撤销下载”。磁力重试先由后端查找已有链接或从 Mikan 详情链接生成；收到 `download_magnet_required` 后才弹出填写框，确认时校验有效 BTIH，错误保留在弹窗中，取消不改任务。成功只重置所选任务为使用磁力链接的待下载状态，不影响其他任务；请求期间禁用重复操作。
- 邀请码列表区分管理员与用户生成来源；用户自助配额在后端校验，见 [viewer.md](viewer.md)。
