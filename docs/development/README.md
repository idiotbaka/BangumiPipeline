# 开发文档组织

这里存放按业务边界读取的说明。所有代码路径相对仓库根目录；文件之间的 Markdown 链接相对当前文档。

## 三层分工

1. 根 [AGENTS.md](../../AGENTS.md)：项目定位、目录地图、全局架构与鉴权底线、阅读索引，目标 100–200 行。
2. backend、frontend（含各端）、src-tauri、scripts 的局部 AGENTS.md：短的目录约定与文档入口。
3. 本目录的普通 Markdown：按本次任务读取相关业务规则、数据职责、容易回归的边界。

Codex 启动时按根目录到当前工作目录收集指令，项目指令合计默认限制为 32 KiB。
从仓库根启动不会因此自动遍历加载所有子目录；根指令明确要求修改前补读目标路径的局部指令。
普通 Markdown 链接不会自动内联，按索引主动读取；分目录工作时也不必通读全部业务说明。
加载机制见 [Codex 官方说明](https://learn.chatgpt.com/docs/agent-configuration/agents-md)。

## 文档索引

| 文档 | 边界 |
| --- | --- |
| [backend.md](backend.md) | 装配、配置、任务、日志、概览 |
| [database.md](database.md) | WAL、连接池、迁移、表职责 |
| [http-api.md](http-api.md) | 路由入口、错误响应、鉴权与受控资源 |
| [bangumi.md](bangumi.md) | 元数据、分类、话数、评论同步、头像/表情 |
| [translation.md](translation.md) | 翻译、原文变化与提示词 |
| [subscription.md](subscription.md) | RSS、本地采集、绑定、偏移、电影补源 |
| [download.md](download.md) | qBittorrent、重试、撤销、路径映射、清理 |
| [media.md](media.md) | 识别、字幕、编码、MP4、封面、存储移动 |
| [opskip.md](opskip.md) | Chromaprint 检测、缓存与播放器跳过 |
| [admin.md](admin.md) | 管理页面与操作边界 |
| [viewer.md](viewer.md) | Web、Viewer 业务、评论、主题、邀请、推送、专题 |
| [mobile.md](mobile.md) | APP/TV、页面栈、播放器、原生与主题 |
| [operations.md](operations.md) | 脚本、部署、命名、构建与验证 |

## 如何维护

- 先看代码；文档保留稳定边界、入口与曾修复的回归风险，不成为所有实现细节的副本。
- 全局规则只放根指令，局部约定只放局部指令，详细业务规则在对应文档维护。
- 跨端共用业务链接到同一说明，避免 Web/APP/后端分别维护三套规则。
- 新功能按现有边界补入口与必要约束；只有新增业务边界才增加文档。
- 版本、完整路由、可调算法参数和字段定义以配置/代码为准；历史提交用于发现遗漏，当前实现用于核实。
- 不把一次性样式调整、发布版本号或所有历史修复逐条写成长期限制。
- 修改文档后检查链接和路径；新增局部指令时检查对应根到叶路径合计大小，保持低于默认上限并留余量。
