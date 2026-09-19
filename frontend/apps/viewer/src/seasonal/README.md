# 季度单页面

每个季度专题是观看端内置的静态 Vue 页面，资料和实际使用的图片随前端构建发布。管理端只保存一个启用键；未启用时，观看端导航不会显示入口。

## 2026 年 10 月

- 数据快照：`202610/data.json`，来源 [ACG Secrets.HK 2026 年 10 月新番表](https://acgsecrets.hk/bangumi/202610/)，抓取日期 2026-09-19。
- 封面：`../../public/seasonal/202610/covers/`，全部为本地 WebP。
- 生成脚本：在仓库根目录执行 `python scripts/build-202610-report.py`。脚本需要 Python 3；安装 FFmpeg 后会把封面压缩为 WebP。
- 页面组件：`../components/SeasonPreview202610.vue`。

新增季度页面时，在 `backend/internal/viewer/single_pages.go` 中登记稳定的 key、标题和观看端路径，在 `HomeScreen.vue` 中接入对应组件。管理端的单页面列表会读取这份登记清单。
