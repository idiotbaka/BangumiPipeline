# 观看端 Web 与 Viewer 业务

本文按需读取；代码路径相对仓库根目录。修改相关行为时同步更新本文。


观看端使用 Vue 3、TypeScript，当前不适配移动端，页面最小宽度按 1200px 设计。

番剧竖版封面沿用 Viewer `/api/anime/{bangumiID}/cover`，由后端优先提供 560px、至多 80,000 字节的 WebP 副本；缺失时回退原图，前端无需自行拼接格式后缀。APP 共用此接口，生成与存量补齐规则见 [bangumi.md](bangumi.md)。

关键文件：

- `frontend/apps/viewer/src/App.vue`：观看端入口、登录/注册门禁、站点设置应用和登录后首页壳。
- `frontend/apps/viewer/src/api.ts`：观看端 HTTP 调用和 API 类型。
- `frontend/apps/viewer/src/components/HomeScreen.vue`：顶部导航、首页轮播、热播、最近更新、我的追番以及页面/详情切换壳。
- `frontend/apps/viewer/src/components/ScheduleScreen.vue`：季度番剧时间表。
- `frontend/apps/viewer/src/components/LibraryScreen.vue`：番剧图书馆标签筛选与搜索。
- `frontend/apps/viewer/src/components/AnimeDetailScreen.vue`：详情、选集、播放、元数据、角色声优及追番入口。
- `frontend/apps/viewer/src/components/AnimeVideoPlayer.vue`：自定义 HTML5 播放器、网页全屏和播放进度上报。
- `frontend/apps/viewer/src/components/HistoryScreen.vue`：按话展示观看历史。
- `frontend/apps/viewer/src/components/FollowScreen.vue`、`FollowCard.vue`：我的追番列表和复用卡片。
- `frontend/apps/viewer/src/components/AppDownloadPage.vue`：公开的移动端 APP 最新版本下载落地页。
- `frontend/apps/viewer/src/assets/`：观看端本地图片、字体、样式依赖等静态资源。
- `backend/internal/httpapi/viewer.go`：观看端 API、用户认证、受控媒体接口和 SPA 托管。
- `backend/internal/viewer/service.go`：观看端用户、Session、站点设置和邀请码注册逻辑。
- `backend/internal/viewer/carousel.go`、`filter.go`：轮播图与图书馆筛选配置。
- `backend/internal/viewer/history.go`、`follow.go`：观看进度、历史和追番聚合。
- `backend/internal/bangumi/viewer_schedule.go`、`viewer_library.go`、`viewer_detail.go`：观看端时间表、图书馆和详情查询。

前端约定：

- 默认访问必须登录；未登录展示左侧登录/注册表单、右侧 `chara.png` 视觉区。
- 浅色主题保持白底、粉色点缀与轻量动效；同时保留深色主题，通过现有主题变量适配，第三方 CSS、JS、字体等资源必须本地化。
- 网站名称、favicon、注册开关和邀请码要求来自 `GET /api/site-settings`，不要在观看端硬编码站点标题。
- 注册接口必须尊重后端注册开关和邀请码策略；邀请码只在后端事务中校验并消费。
- 观看端认证与管理端认证相互独立，使用 viewer 用户和 viewer session。
- 首页、目录、详情、图片、媒体、历史与追番接口要求 Viewer 登录；公开接口和鉴权边界见 [http-api.md](http-api.md)。
- APP 最新版本信息和 APK 下载接口为公开接口；H5 下载页路径固定为 `/app/download`，显示版本发布时间且不进入观看端登录门禁。
- 观看端页面当前由 `HomeScreen.vue` 内部状态切换；详情页使用 `/anime/{bangumiID}` 和 History API，未引入 viewer 端 vue-router。
- 吐槽过滤名单只在管理端“网站设置”中维护，观看端评论列表与分集评论数量都必须排除 `username` 或 `nickname` 完全匹配 `viewer_comment_username_filters.username` 的记录；不要把过滤名单暴露给公开站点设置接口。
- 观看端和普通媒体接口不得向浏览器暴露服务器本地绝对路径；视频、分集封面、番剧封面、角色和声优图片都通过受控 ID 接口读取。
- 首页数据首次加载后应复用已有状态；从详情返回时不要无条件完整刷新首页，避免骨架屏和轮播重置闪动。

### 首页、时间表与图书馆

- 首页 `GET /api/home` 返回热播推荐、最近更新、动画电影、已配置轮播和当前用户追番聚合。
- 首页“最近更新”默认展示 24 个番剧卡片，对应 3 排。
- 轮播图片保存在 SQLite，绑定有效番剧；观看端轮播可进入对应详情页。
- 番剧时间表按日本动画季度 `1/4/7/10` 月切换，并用 `YYYY年M月` 精确匹配 `anime_tags`；API 返回该季度所有类型，客户端按 `animeType` 与星期分组。
- 时间表周一到周日仅显示多话连载，星期未定的连载归入“开播未定”；动画电影不论星期是否已定均归入“电影/剧场版”。Web/APP 共用 `frontend/shared/animeSchedule.ts` 的 Tab 与分组规则。
- 时间表和图书馆卡片都可展示没有产物的番剧，并按首播日期区分“尚未开播”和“尚未放流”。
- 图书馆标题搜索匹配原名、中文名和别名；同一筛选维度内为 OR，不同维度之间为 AND，标签匹配 `anime_tags.name`。
- 图书馆默认将存在 `media_jobs.status='completed'` 且产物路径非空的番剧排在前面。
- 顶部“搜索番剧”回车后切换到图书馆并应用关键词。
- Web/APP 图书馆第一行固定为“类型”，可选 `TV/连载` 或 `动画电影/剧场版`；默认不限类型，重复点击当前类型或清除筛选恢复不限。该维度不依赖管理员标签配置，与其余维度按 AND 组合。
- `/api/library` 支持 `anime_type=all|series|movie`，未传时兼容旧客户端默认连载。双端搜索显式查询 `all`；Web 顶部搜索同时清除之前的筛选，避免电影被旧条件排除。
- 图书馆电影卡片不显示“完结”或更新话数，右上角显示“动画电影”，右下角显示评分；混合结果按每项 `animeType` 判断。

### 详情、播放与进度

- 详情接口组合番剧基础信息、分集元数据、已完成媒体、最后观看进度和追番状态；角色按主角、配角、其他顺序展示。
- 分集只有关联到 `media_jobs.status='completed'` 且 `output_path` 非空时才可播放；未产出的分集不可点击。
- 播放器右侧面板使用“选集 / 评论”双 Tab；评论接口按当前已完成 `mediaID` 在服务端映射 Bangumi episode ID，不接受客户端直接枚举任意 episode ID。
- 评论及同级楼中楼回复按源评论时间倒序；切换到评论 Tab 或切换当前选集时重新加载，并取消/忽略已经过期的请求。
- 评论正文不得使用 `v-html`。只以结构化节点渲染 `[s]`、`[mask]`、`[img]`、`[img=宽,高]` 和可用的 Bangumi 表情；未知或残缺标签、非法外链协议、缺失表情需要安全过滤。
- Viewer 番剧详情按 `bangumi_episode_comments` 中的本地主楼快照分组统计每话 `commentCount`，不使用可能存在时间差的元数据计数；评论 Tab 在尚未请求正文时显示该统计值，并在正文加载后使用评论接口的最新计数。
- 评论外链图片仅接受无账号凭据的 HTTP/HTTPS URL，并限制显示尺寸；遮罩内容支持 hover 和键盘 focus 后显示。
- 带尺寸参数的评论图片允许在同一行排列并自动换行，图片间距为 20px；无尺寸参数的评论图片保持独占一行。
- 评论用户签名为空或仅含空白时不渲染占位文案。
- 视频和分集封面必须同时校验 `bangumiID` 与 `mediaID`；视频由 `http.ServeFile` 提供 Range 播放，不在 JSON 中返回路径。
- 默认选择请求指定的媒体，否则恢复最后观看媒体，再否则选择第一个可播放分集。已完播记录从 0 秒开始，未完播记录恢复位置。
- 播放器每 10 秒以及暂停、结束、组件卸载时上报进度；实际播放不超过 15 秒不落库，达到时长 90% 记为完播。
- 自定义播放器支持普通全屏和网页全屏；网页全屏期间隐藏顶部导航。播放中 3 秒无交互会隐藏 UI，暂停、缓冲或报错时保持显示，隐藏态底部保留粉色进度线。
- 播放器进度条要展示浏览器实际 `video.buffered` 范围；活动 UI 的进度条和隐藏态底部进度条都要显示缓冲范围，不额外伪造或限制为固定 5 分钟。
- 详情标签先显示蓝色 Meta Tags；粉色普通 Tags 中与 Meta Tags 重复的项不再显示。

### 观看历史与追番

- 观看历史按话记录，同一用户和 `media_job_id` 唯一；列表按最后观看时间倒序并显示该番最新产物话数。
- 管理员概览通过 `viewer.RecentActivities` 读取跨用户最近 5 条观看快照，复用用户管理的分集命名、中文回退与进度判定，按 `last_watched_at DESC, id DESC` 稳定排序；排除已删除番剧、非 completed/无路径的媒体。不是完整播放事件审计，同一用户同一话再次观看会更新该条记录。不修改 Viewer API 或用户隔离规则。
- 从历史进入详情时恢复到该话和进度；完播记录从该话起始位置播放。
- 历史接口返回元数据中的 `animeType` 与 `airDate`；双端电影历史显示“看到 N%”或“已看完”，标题下依次显示“动画电影”和“于 YYYY.MM.DD 首播”（日期缺失时“首播日期未定”），与连载卡片的分集标题及话数/更新信息保持相同行数。保留媒体 ID、进度条与恢复行为，不通过总话数猜测电影类型。
- 详情页“追番”按钮写入 `viewer_anime_follows`；Web 详情/播放器页面仅对连载显示该按钮，电影无论可播放话数多少均隐藏按钮及其错误提示。独立“我的追番”页展示全部追番，首页只展示尚未追到最新产物的项目。
- 追番恢复目标依次为：未完播的最后观看话、已有更新时的最新产物、未观看时的最早产物；没有产物时不生成可播放目标。
- “已追到最新”仅指最后观看话已完播且与当前最新产物为同一季、同类型、同话数。

## 共用客户端与服务端边界

- Web 与 APP 的评论解析共用 `frontend/shared/commentContent.ts`，各端入口只负责导出；表情识别与清单仍由后端 `bangumi/smiles.go` 维护。
- 移动端同时遵循本文的用户隔离、历史、追番、评论和进度契约；原生/布局规则见 [mobile.md](mobile.md)。
- 播放进度阈值与完播判断在服务端校验，不得只由前端决定完播状态。

## 分类与状态

- Web 电影独立进入首页电影区域，并从剧集热播/更新区域排除；季度时间表通过“电影/剧场版”独立显示，图书馆支持 `animeType` 分类筛选。
- Viewer 正片总话数使用 `eps`，缺失时数本地正片元数据；SP 等不计入该总数。
- `isCompleted` 由后端判断是否存在话数等于正片总数的已完成正片产物，不代表用户已看完或逐话检查全部产物。
- 单话电影采用现有简化详情布局；选定有效媒体后再加载评论，切换时重置/取消旧请求，避免提前请求或重复加载竞态。

## 主题与播放器设置

- 主题逻辑集中在 `src/theme.ts` / `theme.css`，与 `index.html` 首次绘制初始化一致，避免首屏主题闪烁。
- Web 主题支持 light/dark 本地持久化与跨标签页同步；公开 APP 下载页保持现有浅色策略。
- `src/playerPreferences.ts` 保存播完暂停/自动下一话、OP 按钮、音量和静音设置；读取校验与存储失败回退不可遗漏。
- 播放器已支持快捷键、画中画、截图、选集和统计菜单；新控制项遵循浏览器能力检测、输入焦点和卸载清理逻辑。
- OP 跳过使用详情接口的识别结果，处理结果缺失或无效范围，见 [opskip.md](opskip.md)。

## 用户设置、邀请与 Web Push

- 改密码在 `viewer.Service.ChangePassword`，前端入口为 `SettingsScreen.vue`；配套会话行为以服务实现为准。
- 自助邀请码在 `viewer/invitation.go`：注册满 7 天起可生成，随后按注册日起的自然月累计，上限 10 个；资格和数量在 Writer 事务中复核。
- 邀请码来自当前登录用户，注册时在事务中校验并消费，不能由客户端控制创建者/使用者。
- Web Push 使用 `viewer/push.go`、`src/pushNotifications.ts`、`public/push-sw.js`，需要 HTTPS（localhost 例外）和浏览器授权。
- VAPID 密钥持久化到 SQLite；媒体完成回调为追番用户入队，唯一约束避免同一订阅/产物重复入队，调度任务负责投递与重试。
- 订阅/退出操作保持当前用户边界，权限拒绝或浏览器能力缺失不应阻塞观看。

## 内置季度专题

- 专题由内置 Vue 组件、本地数据快照与图片组成，管理端只选择启用 key。
- 清单在 `backend/internal/viewer/single_pages.go`，启用项经 site-settings 提供；新专题同时登记后端和 `HomeScreen.vue` 组件/路径。
- 数据与生成流程见 [季度专题说明](../../frontend/apps/viewer/src/seasonal/README.md)，资料随前端构建发布。
