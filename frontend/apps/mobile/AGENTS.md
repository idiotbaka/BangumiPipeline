# APP WebView 前端开发

适用于本目录。继承根与 `frontend/AGENTS.md`；路径相对本文件。

- APP 布局、返回栈、播放器、鉴权和更新规则读 [移动端说明](../../../docs/development/mobile.md)。
- 历史、追番、评论和进度业务同时读 [观看端说明](../../../docs/development/viewer.md)。
- 涉及原生桥接时读取 [Tauri 指令](../../../src-tauri/AGENTS.md)。
- 修改准备/签名脚本时读取 [脚本指令](../../../scripts/AGENTS.md)。
- API 域名通过 `src/config.ts` 规范化，Viewer token 与媒体 URL 通过 `src/api.ts` 管理。
- `MobileShell.vue` 管理页面与返回栈；主 Tab 滚动位置独立，普通页面预留 Android 安全区。
- `MobileVideoPlayer.vue` 管理 HTML5 播放器，原生能力经 `src/native/player.ts` 调用。
- 原生横屏沉浸式全屏、返回键退出全屏与播放常亮行为必须保持。
- APP 和 TV 共用此 workspace；涉及 `platform.ts`、`tvFocus.ts` 或 TV 布局时检查两种目标。
- 版本更新提示不能阻塞启动/登录；下载页按当前服务器地址生成并用系统浏览器打开。
