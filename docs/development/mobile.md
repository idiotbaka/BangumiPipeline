# APP / Tauri Android

本文按需读取；代码路径相对仓库根目录。修改相关行为时同步更新本文。


移动端使用 Tauri v2 + Vue 3 + Android WebView。它是观看端的移动客户端，账号、追番、观看历史、番剧详情、媒体流和进度都必须复用 Viewer API 与同一套 SQLite 数据。

关键文件：

- `frontend/apps/mobile/src/App.vue`：移动端入口、站点初始化、登录门禁。
- `frontend/apps/mobile/src/config.ts`：API 域名配置，默认 `https://baka.vip/`，并支持 `public/app-config.json` 和本地覆盖。
- `frontend/apps/mobile/src/api.ts`：移动端请求封装、Viewer API 类型、Bearer token 保存和鉴权 URL 构造。
- `frontend/apps/mobile/src/components/MobileShell.vue`：登录后的主壳，底部导航、首页、时间表、图书馆、我的、搜索、追番、历史和详情页状态切换。
- `frontend/apps/mobile/src/components/MobileAnimeDetailScreen.vue`：移动端番剧详情页、选集、简介、元数据、角色声优与追番入口。
- `frontend/apps/mobile/src/components/MobileVideoPlayer.vue`：移动端自定义播放器、播放进度上报、手势和原生全屏桥接。
- `frontend/apps/mobile/src/components/AppUpdateDialog.vue`：移动端新版本提示、更新日志和下载/忽略操作。
- `frontend/apps/mobile/src/appUpdate.ts`：移动端语义版本比较、忽略版本持久化和下载页地址生成。
- `frontend/apps/mobile/src/native/player.ts`：调用 Tauri player 插件的前端桥接。
- `frontend/apps/mobile/src/native/opener.ts`：通过 Tauri opener 插件唤起系统默认浏览器。
- `frontend/apps/mobile/src/assets/`：移动端本地 SVG 图标和样式资源。
- `src-tauri/tauri.conf.json`：Tauri v2 配置，APP 名称、版本、bundle、Android minSdk 和 build hook。
- `src-tauri/src/lib.rs`：Tauri 插件注册和运行入口。
- `src-tauri/plugins/player/`：自定义 Tauri player 插件，Android 原生全屏、横屏、沉浸式和常亮屏幕能力在这里实现。
- `src-tauri/android-src/main/java/vip/baka/bangumipipeline/mobile/MainActivity.kt`：Android Activity 覆盖，安全区、WebView 自动播放、返回键和退出确认逻辑在这里实现。
- `src-tauri/android-res/`：Android 资源覆盖，包括应用名、图标、主题和启动窗口资源。
- `scripts/prepare-mobile-android.mjs`：构建前把 Android Java/Kotlin 与资源覆盖复制到 Tauri 生成工程。
- `scripts/build-android-apks.ps1`：构建多架构 APK、创建或读取本地签名文件、zipalign、apksigner，并输出到 `apk/`。

运行和打包：

- 本地开发运行 `npm run tauri:dev`，实际执行 `cargo tauri android dev`。
- Tauri dev/build 前会执行 `npm run prepare:mobile-android`，不要绕过该步骤修改生成目录。
- 只构建移动端 Web 资源运行 `npm run build:mobile` 或 `npm run build --workspace @bangumi-pipeline/mobile`。
- 打包签名 APK 运行 `npm run build:mobile:apk` 或 `powershell -ExecutionPolicy Bypass -File scripts/build-android-apks.ps1`。
- 生成的 `src-tauri/gen/android/` 是 Tauri 生成目录，原则上不要把功能改在这里；需要持久化的 Android 代码和资源放回 `src-tauri/android-src/`、`src-tauri/android-res/` 或插件目录。

移动端 API 和鉴权：

- 默认 API 基址为 `https://baka.vip/`，配置文件为 `frontend/apps/mobile/public/app-config.json`。
- 用户可在 APP 内保存 API 域名覆盖值；保存逻辑必须走 `normalizeAPIBaseURL`，保证尾部有 `/`。
- 移动端登录、注册、用户信息、首页、时间表、图书馆、详情、媒体、追番和历史都复用 Viewer API。
- 移动端认证使用 Viewer token，不使用管理端 Session，不使用客户端传入用户 ID。
- 受控媒体 URL 必须通过 `buildAuthenticatedMediaURL` 追加移动端 token；不得返回或暴露服务器本地路径。
- Web 观看端与移动端用户数据共享，后端接口改动必须同时考虑 `frontend/apps/viewer/src/api.ts` 和 `frontend/apps/mobile/src/api.ts` 的类型兼容。

移动端交互和布局约定：

- 移动端不是 1200px Web 观看端的缩放版，必须按手机 APP 交互重新设计。
- 主导航固定为底部 Tab：首页、时间表、图书馆、我的。
- 顶部栏、搜索栏、时间表季度/星期栏等固定元素滚动时应保持固定，并考虑 Android safe area。
- 四个主 Tab 的滚动高度互相独立，切换 Tab 不得串用上一页滚动位置。
- 详情、搜索、我的追番、观看历史等子页面使用内部 History API 管理返回栈；Android 返回键应先返回上一页。
- 根页面按返回键时先显示 “再按一次退出 BakaVip2”，3 秒内再次返回才退出。
- 进入新页面使用右侧滑入，返回上一页使用反向滑动；过渡期间不要让顶部栏消失或让旧页面滚动到顶部。
- APP 内隐藏浏览器滚动条，但不能禁用滚动能力。
- 浅色主题保持白底、粉色点缀与轻量卡片，同时通过既有主题变量适配深色主题；尽量少用粗体、少用 `h1`/`h2`。
- 番剧列表在手机端通常按 3 列展示；大量数据页面需要下滑加载或懒渲染，避免一次性渲染 1000+ 项导致卡顿。
- Loading 和骨架屏应优先使用移动端已有卡片骨架效果，不要回退成大段文字 loading。

移动端播放器约定：

- 播放器仍使用 HTML5 `<video>` 承载媒体流，视觉和控制层在 `MobileVideoPlayer.vue`。
- 点击播放器只显示/隐藏 UI；双击才切换播放/暂停。
- 媒体拿到有效 duration 前显示 “播放器加载中”，且不允许用户点击播放。
- 视频可控制后应尝试自动播放；Android WebView 的用户手势限制由 `MainActivity.kt` 关闭。
- 播放中要调用原生 `setKeepScreenOn(true)` 常亮屏幕，暂停、结束、卸载时关闭。
- 全屏必须通过 Tauri player 插件进入 Android 原生横屏沉浸式，而不是只依赖 WebView 默认全屏。
- 全屏时隐藏状态栏和导航栏，横屏方向允许随设备方向旋转，退出时恢复原方向和系统栏。
- 全屏时 Android 返回键应优先退出播放器全屏，而不是退出 APP。
- Android 可能因系统手势短暂显示导航栏；player 插件需要在全屏期间重复应用 immersive flags。
- 左右滑动播放器用于快进/快退，最长 10 分钟，极短滑动不生效；松手后才执行 seek。
- 进度上报规则与 Web 观看端保持一致：定时、暂停、结束、卸载时上报，短播放不落库，接近结尾才完播。
- 页面与顶栏的前进/返回动画统一由 `MobileShell.vue` 的共享过渡控制；进入和离开必须使用方向相反、距离相等的全宽位移，避免透明页面层重叠产生旧页面残留。
- APP 启动后异步读取公开的最新版本接口；只有服务端版本高于当前版本且未被忽略时才提示，不得阻塞启动和登录。
- 忽略版本只针对精确版本号；“关于 APP”中的手动检查必须绕过忽略状态。
- APP 下载页使用当前服务器地址拼接 `/app/download`，并通过 Tauri opener 插件交给系统默认浏览器打开。

Android 原生和资源约定：

- 手机 APP 名称为 `BakaVip2`，包名为 `vip.baka.bangumipipeline.mobile`；版本读取 Tauri 配置和包清单，发布时检查一致性。
- Tauri bundle 图标来自 `src-tauri/icons/icon.png` 和 `src-tauri/icons/icon.ico`。
- Android launcher 图标覆盖在 `src-tauri/android-res/mipmap-*`，不要只改 Tauri 默认图标而忘记 Android 资源。
- Android 主题、应用名、颜色、启动窗口资源覆盖在 `src-tauri/android-res/values*` 和 `drawable/`。
- 当前原生启动窗口只保留纯背景色，角色开屏由 WebView 内部 Loading 页负责；如需恢复原生角色开屏，必须同时更新 `splash_background.xml` 和准备脚本。
- `MainActivity.kt` 必须保持 `WindowCompat.setDecorFitsSystemWindows(window, true)`，避免普通页面与 Android 状态栏打架。
- 进入播放器原生全屏时，player 插件会临时切换为 `setDecorFitsSystemWindows(false)`；退出全屏必须恢复。
- Android 原生代码变更后，优先在 `src-tauri/gen/android` 下运行对应 Gradle 子任务验证，例如 `.\gradlew.bat :tauri-plugin-player:compileReleaseKotlin` 或 `.\gradlew.bat :app:processArm64ReleaseResources`。
- 签名文件和密码在 `src-tauri/android-signing/` 下本地生成，必须保持 gitignore，不得提交 keystore 或密码。

## 主题、共用业务与原生工具

- 主题在 `src/theme.ts` / `theme.css`，首屏初始化与 `index.html` 一致；主题不随账号或服务器切换清除。
- 手机系统栏主题通过 player 插件串行同步，避免旧调用覆盖新选择；TV 按现有分支处理。
- 评论、历史、追番、完结状态与 OP 跳过复用服务端契约，见 [viewer.md](viewer.md) 与 [opskip.md](opskip.md)。
- 图片原图保存、系统浏览器与剪贴板走 `src/native/` 既有桥接；原生注册与权限随插件同步检查。

## Android TV 目标

- TV 复用 mobile workspace，由 `src/platform.ts` 与 Vite tv mode 区分，原生配置在 `src-tauri/tauri.tv.conf.json`。
- 遥控器焦点逻辑集中于 `src/tvFocus.ts`；交互元素要能聚焦，弹窗限制焦点范围，关闭后恢复原位置。
- 遥控器按键、播放器控制、返回键与手机触摸手势按既有目标分支处理，不将手机三列布局和手势规则直接套给 TV。
- TV 原生覆盖在 `src-tauri/android-src/tv-main/` 和 `src-tauri/android-res-tv/`；准备脚本选择 manifest、资源和 application ID。
- 两种目标共用生成工程，切换目标时重新运行对应准备脚本，避免残留 TV manifest/资源。
- TV 构建与签名命令见 [operations.md](operations.md)。
