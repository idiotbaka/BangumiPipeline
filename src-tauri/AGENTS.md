# Tauri / Android 原生开发

适用于 `src-tauri/`。继承根指令；路径相对本文件。

- 修改原生桥接、资源或配置前读 [移动端说明](../docs/development/mobile.md)。
- 涉及前端调用时读取 [APP 指令](../frontend/apps/mobile/AGENTS.md)。
- 涉及资源准备和 APK 签名时读取 [脚本指令](../scripts/AGENTS.md) 与 [运维说明](../docs/development/operations.md)。
- `src/lib.rs` 是插件注册入口，`plugins/player/` 管理原生播放器能力。
- 持久修改放在 `android-src/`、`android-res/`、`android-res-tv/` 或 `plugins/`。
- `gen/android/` 为生成工程，用准备脚本同步；不能只改生成文件。
- 普通 Activity 与播放器全屏使用不同的系统栏/安全区策略，退出全屏恢复方向和系统栏。
- 手机与 TV 配置分别在 `tauri.conf.json`、`tauri.tv.conf.json`；根据目标使用对应准备与构建命令。
- 图标/启动资源同时检查 Tauri bundle 和 Android 覆盖资源。
- 版本值以配置和包清单为准；签名 keystore、`signing.env` 与 APK 不提交。
- 原生验证使用对应 Gradle 子任务，命令及生成工程前提见运维文档。
