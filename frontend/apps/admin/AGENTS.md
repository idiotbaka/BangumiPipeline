# 管理端开发

适用于本目录。继承根与 `frontend/AGENTS.md`；路径相对本文件。

- 页面入口和交互规则读 [管理端说明](../../../docs/development/admin.md)。
- API 契约与管理员鉴权读 [API 说明](../../../docs/development/http-api.md)。
- 涉及订阅、下载、媒体、翻译、任务等操作时，再按根索引读取业务文档。
- 使用 Vue 3 + TypeScript + Element Plus；API 类型/请求放在 `src/api.ts`。
- 新页面同时接入 `src/router.ts` 与 `src/components/AdminLayout.vue`。
- “前端管理”的 Viewer 配置使用 Admin API 和管理员认证。
- 番剧卡片按钮保持固定宽度和可读文本，处理 loading、禁用态和操作错误。
- 服务器路径仅在系统设置、系统概览和存储管理等管理功能中展示。
