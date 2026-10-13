# LLM 翻译

本文按需读取；代码路径相对仓库根目录。修改相关行为时同步更新本文。


相关代码：

- `backend/internal/translation/service.go`
- `backend/internal/system/service.go`
- `frontend/apps/admin/src/pages/SettingsPage.vue`

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
