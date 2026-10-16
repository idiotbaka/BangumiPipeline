# LLM 翻译

本文按需读取；代码路径相对仓库根目录。修改相关行为时同步更新本文。


相关代码：

- `backend/internal/translation/service.go`
- `backend/internal/translation/management.go`
- `backend/internal/system/service.go`
- `frontend/apps/admin/src/pages/SettingsPage.vue`
- `frontend/apps/admin/src/pages/TranslationManagementPage.vue`

设置保存到 `llm_settings`：

- `base_url`：OpenAI Chat 兼容接口 v1 根地址。
- `api_key`：Bearer token，可为空以兼容本地模型。
- `model`：模型名称。
- 测试连接要求模型只返回 `OK`。

任务行为：

- 遍历原文非空且中文字段为空的元数据字段。
- 翻译目标包括番剧标题、番剧简介、分集标题、分集简介、角色简介、声优简介。
- 如果原文本身是中文且不需要清理，则直接复制到中文字段。
- 包含日文假名时必须走 LLM。
- 剧情简介和分集简介要清理脚本、演出、作画、制作人员等非剧情信息。
- LLM 请求串行，使用系统网络代理，超时 60 秒。
- 不记录 API KEY。
- 翻译成功日志可以记录原文和译文用于调试。

提示词约定：

- 直接输出结果，不输出说明、引号或 Markdown 代码块。
- 保留或优化段落格式。
- 角色名或专有名词首次出现用括号标注原文。
- 双引号统一使用中文直角引号 `「」`。

## 管理端进度

- `/translations` 每行一部未软删除的番剧；`GET /api/translation/anime` 支持 `q` 标题/别名搜索、`status` 筛选、`page` 和 `pageSize`，先筛选后分页。
- 六类进度与任务选取条件一致：有原文才计入总数，对应中文字段非空计为完成。上游自带中文、中文直拷、LLM 译文都计为完成，无法根据现有字段区分来源；无原文单独计数，不计入待翻译。
- 分集包含所有当前入库类型；角色按当前入库范围统计。声优通过 `character_actors` 按每部番剧的 `actor_id` 去重，中文结果仍全局共享。
- 状态按顺序派生：有待翻译字段为 `pending`；无待翻译但详情/角色/分集任一抓取阶段未完成为 `metadata_pending`；全部阶段完成且无可处理原文为 `no_content`；其余为 `completed`。抓取未完成或失败另显示提示，不能把尚未抓到的资料视作翻译完成。
- 展开行按需调用 `GET /api/translation/anime/{bangumiID}`，只读展示各项目的 `completed` / `pending` / `no_source`，支持按类别和明细状态筛选。
- 页面展示对应计划任务启用情况、最近运行状态及错误，手动刷新。翻译错误没有逐字段持久状态，任务遇错结束本轮，失败项仍是待翻译；不提供逐番剧“翻译失败”或“正在翻译”判断，也不把通用 `updated_at` 当作翻译完成时间。
- 进度针对当前入库内容；新增资料或原文变化导致中文字段清空后，列表自然恢复为待翻译。本期不修改任务执行方式，不新增 schema、编辑、跳过或手动翻译操作。
