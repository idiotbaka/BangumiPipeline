# 脚本与运维开发

适用于 `scripts/`。继承根指令；路径相对本文件。

- 修改前读 [运维说明](../docs/development/operations.md)；视频修复另读 [媒体说明](../docs/development/media.md)。
- Android 准备与构建另读 [移动端说明](../docs/development/mobile.md) 和 [Tauri 指令](../src-tauri/AGENTS.md)。
- `fix-mp4-remux.sh` 默认 dry-run，必须显式 `--apply` 才替换文件。
- 保持同目录临时文件、校验后替换的修复流程，产物路径不变。
- `prepare-mobile-android.mjs` 负责持久覆盖同步，生成工程未初始化时安全跳过。
- 手机/TV 构建目标沿用现有参数；签名密码使用环境变量或现有本地交互流程。
- 不提交 keystore、`signing.env`、个人网络配置、APK 或构建输出。
