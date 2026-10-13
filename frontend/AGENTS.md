# 前端共用开发约定

适用于 `frontend/`。继承根指令；各端另有局部指令。

- 使用 Vue 3、TypeScript、Composition API 与 `<script setup lang="ts">`。
- 请求封装和 API 类型集中在各端 `src/api.ts`，错误沿用 `APIError`。
- 页面处理 loading、空状态、禁用态和请求失败；保留现有视觉风格。
- 修改前读取目标端的 `AGENTS.md` 和根阅读索引对应文档。
- 修改 Viewer 接口契约同时检查 `apps/viewer/src/api.ts` 与 `apps/mobile/src/api.ts`。
- `shared/commentContent.ts` 是 Web/APP 评论解析共用实现，修改时检查两端渲染器。
- 评论结构化渲染、外链过滤、表情和计数规则见 [观看端说明](../docs/development/viewer.md)。
- 涉及共用解析时读取两端局部指令；不要复制维护两套内容解析或表情映射。
- Web 观看端与 APP 使用各自布局与导航，APP 含 TV 构建目标。
- 包管理与构建入口以根和各端 `package.json` 为准。
