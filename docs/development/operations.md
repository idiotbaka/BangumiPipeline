# 运维、命名与验证

本文按需读取；代码路径相对仓库根目录。修改相关行为时同步更新本文。

## 运维脚本

相关代码：

- `scripts/fix-mp4-remux.sh`
- `scripts/prepare-mobile-android.mjs`
- `scripts/build-android-apks.ps1`

视频修复脚本：

- 默认扫描 `/opt/downloads/BangumiPipeline/data/bangumi`，可用 `--root` 覆盖。
- 默认 dry-run，只输出候选文件；必须显式加 `--apply` 才会替换文件。
- 只处理 MP4，检测 ffprobe edit-list/index warning、`moov` 在 `mdat` 后面、多个顶层 `mdat` box 三类问题。
- 修复方式为同目录生成临时文件，执行 `ffmpeg -map 0 -c copy -movflags +faststart`，ffprobe 校验成功后替换原文件。
- 默认不保留备份；`--keep-backup` 会保留隐藏 `.remux.bak` 文件。
- 脚本保持成品视频路径不变，不需要同步修改 SQLite 中的 `media_jobs.output_path`。

移动端脚本：

- `prepare-mobile-android.mjs` 按 mobile/tv 目标同步代码、manifest 和资源覆盖，并准备生成工程的 namespace/application ID；切换目标时处理残留资源。
- `prepare-mobile-android.mjs` 可以在 Android 工程未初始化时安全跳过。
- `build-android-apks.ps1` 默认构建 `aarch64`、`armv7`、`i686`、`x86_64` 多架构 APK。
- 首次签名时脚本会读取或要求 `ANDROID_KEYSTORE_PASSWORD`、`ANDROID_KEY_PASSWORD`，并在本地生成 keystore 与 `signing.env`。
- APK 产物输出到 `apk/{AppName}-{Version}-{yyyyMMdd-HHmmss}/`。

## 命名约定

- 项目名：`BangumiPipeline`
- Go module：`bangumipipeline.local/server`
- npm workspace/package：`bangumi-pipeline`、`@bangumi-pipeline/*`
- 移动端 npm workspace：`@bangumi-pipeline/mobile`
- 移动端 APP 名称：`BakaVip2`
- 移动端版本：以 `src-tauri/tauri.conf.json`、对应 TV 配置和 `frontend/apps/mobile/package.json` 为准。
- 移动端 Android package identifier：`vip.baka.bangumipipeline.mobile`
- 移动端 Rust crate：`bangumi-pipeline-mobile`
- Docker 二进制：`bangumi-pipeline`
- 容器用户：`bangumipipeline`
- 新环境变量：`BP_*`

## 验证命令

仅在任务明确要求或需要完整验证时使用，遵守根指令的验证范围约定。
所有以下命令默认从仓库根目录运行；Gradle 示例需已初始化的 Android 工程、SDK/JDK 和对应原生修改的覆盖同步。

常规完整验证：

```powershell
gofmt -w <修改过的 Go 文件>
go test ./backend/...
npm run build:ui
```

只编译后端：

```powershell
go build -buildvcs=false ./backend/...
```

只构建移动端 Web 资源：

```powershell
npm run build --workspace @bangumi-pipeline/mobile
```

同步 Tauri Android 覆盖资源：

```powershell
npm run prepare:mobile-android
```

验证 Android 原生播放器插件：

```powershell
cd src-tauri/gen/android
.\gradlew.bat :tauri-plugin-player:compileReleaseKotlin
```

验证 Android APP 资源：

```powershell
cd src-tauri/gen/android
.\gradlew.bat :app:processArm64ReleaseResources
```

构建并签名多架构 APK：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build-android-apks.ps1
```

完整验证也可以运行：

```powershell
npm test
```

Vite 可能输出 Element Plus chunk size 和 `INVALID_ANNOTATION` 警告；只要退出码为 0，属于已知非阻断警告。

## 本地网络与部署

- 安装依赖需要代理时，只在当前 PowerShell 会话设置 `HTTP_PROXY` / `HTTPS_PROXY`，地址取自用户本地配置。
- 不提交个人代理地址、账号、Cookie、Token、qBittorrent 凭据、LLM API KEY 或签名密码。
- 环境要求、容器启动与部署配置见根 [README.md](../../README.md)、`Dockerfile` 与 `compose.yaml`。

## 按端验证与 TV 构建

| 范围 | 仓库根目录命令 |
| --- | --- |
| 管理端类型检查与构建 | `npm run build --workspace @bangumi-pipeline/admin` |
| Web 观看端类型检查与构建 | `npm run build --workspace @bangumi-pipeline/viewer` |
| Viewer 前端测试（任务要求时） | `npm run test --workspace @bangumi-pipeline/viewer` |
| TV Web 资源 | `npm run build:tv` |
| TV 原生覆盖准备 | `npm run prepare:tv-android` |
| TV APK 构建与签名 | `npm run build:tv:apk` |

- 手机与 TV 的 Tauri 配置、manifest、application ID 和资源独立，按所选目标检查。
- 只修改文档时检查链接、路径、规则覆盖和差异；不需要安装依赖、初始化 Android 或构建 APK。
