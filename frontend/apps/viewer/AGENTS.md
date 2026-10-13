# Web 观看端开发

适用于本目录。继承根与 `frontend/AGENTS.md`；路径相对本文件。

- 页面、进度、历史、追番、评论与目录规则读 [观看端说明](../../../docs/development/viewer.md)。
- 涉及鉴权、媒体流或 API 契约再读 [API 说明](../../../docs/development/http-api.md)。
- 桌面页面最小宽度 1200px，保留浅色/深色主题、粉色点缀与现有动效。
- `src/App.vue` 负责登录门禁与站点设置，`HomeScreen.vue` 负责页面壳。
- 详情路径和返回行为沿用 History API，不另行引入 vue-router。
- 站点名称/favicon/注册策略来自 API，公开 `/app/download` 保持可直接访问。
- 从详情返回复用首页状态，避免无条件重载造成骨架屏和轮播闪动。
- 视频、封面与进度使用受控媒体 ID；不接收服务器路径或客户端用户 ID。
- 评论通过 `frontend/shared/commentContent.ts` 解析并结构化渲染，不使用 `v-html`。
- 播放器保留进度上报、真实 buffered range、恢复播放与网页全屏行为。
- 主题与播放器偏好沿用 `theme.ts` / `playerPreferences.ts`；专题、电影与用户设置入口见观看端说明。
