# 折叠、编辑辅助和标记语言实施记录

2026-10-19；基线 SourceView 1f0ef1b，正式依赖 GOUI v0.2.0。
设计：[折叠](DesignFolding.md)、[编辑辅助](DesignEditorAssistance.md)、[标记语言](DesignMarkup.md)。
用户授权写入设计、修改 SourceView、本机验证后分阶段提交；需要 GOUI 改动时先记录要求并跳过。

## 实施切片

- [x] A1：可见段落映射与原生几何接入。
- [x] A2：折叠范围、后台分析、JSON/XML/HTML 规则、交互、样式、语义和薄 UI。
- [x] B：编辑辅助与可选 GoToBar。
- [x] C1：XML/HTML 标签编辑、正式 Markdown。
- [x] C2：嵌入高亮/编辑和标签名称联动。
- [ ] D：正则/捕获组替换、独立选区查询范围及多行输入。

每个切片记录实际范围和验证，未执行不写成完成；多光标、通用装饰、语言服务及编程语言后置。
查找增强沿用公开 SearchQuery，Regex=false 保留字面搜索。采用 Go regexp 语义；零长度匹配
必须进展，Find/Count/ReplaceAll 使用同一不重叠序列，替换为一次预算受限原子事务。
选区查询范围独立于 FindNext 改变的选区；编辑后映射或清除，换文档清除。非法表达式栏内反馈。

## GOUI 前置要求

暂无。遇到公开契约缺口时在此记录需求、调用证据、受阻场景及跳过范围，不修改 GOUI。

## 验证记录

普通与 GOWORK=off 全仓测试；按风险 race、Windows/macOS 交叉构建和本机原生窗口。
跨平台编译不作为原生交互验收。窗口启动、像素断言和实际操作分别记录。

### A1

新增连续可见行段映射，HeightIndex 只维护可见段落；布局/绘制/命中/滚动/上下导航统一转换。
2000 组固定随机样本与独立行枚举一致；假排版检查隐藏段落不占高度、命中返回真实位置、
未变正文布局复用。Linux amd64 / Go 1.25.8，GOWORK=off 全仓测试通过。
本切片尚未开放折叠，不计为折叠窗口验收；实际隐藏与 IME/快照在 A2 完成。

### A2

- 语言的 FoldingFunc、范围校验、JSON/XML/HTML 规则已落地；Document 单 worker 可取消并
  丢弃过期结果，视图独立保留嵌套状态。侧栏/行末标记、菜单、快捷键、搜索揭示、删除边界、
  只读及真实原文分段快照已接入。声明式只协调开关及信号，菜单文字补齐八语言。
- 新增规则、非法范围、编辑锚点、共享文档、撤销隔离、旧结果/卸载丢弃及通知重入测试。
  旧括号像素测试显式关闭折叠以保持其独立断言；补全定位期望纳入新侧栏宽度，没有删断言或放宽容差。
- Linux amd64 / Go 1.25.8 / Pango：GOWORK=off 全仓及 race 通过；Windows amd64 /
  macOS arm64 全仓 CGO_ENABLED=0 交叉构建通过。
- 新 tests/window/editor/folding 在独立 Xvnc :140 / Xfwm4（合成开启）以 Software 1x、
  Software 2x Dark、OpenGL 1x 通过；实际 imageResource 与 PixelScale 已记录。
  Software 1x 的新窗口另以 -race 构建并运行通过。键盘/指针、隐藏删除、搜索、历史与共享视图均断言通过。
- 原 tests/window/editor -auto 在同桌面 Software 1x 通过，覆盖原编辑、颜色缓存、重挂载、
  reveal、视口锚点与指针。未执行真实物理 IME、混合 DPI 或 Windows/macOS 新窗口。
  所有窗口仅使用内存样本，未改写真实剪贴板/设置，运行后自动关闭。
- XML/HTML 首轮仅折叠确认配对；HTML 省略结束标签及通用容错 DOM 推断未包含。GOUI 无新增要求。

### B

新增空白/行尾、缩进线、标尺、完整词高亮、智能 Home/End 和一级缩进退格；
可选 GoToBar 支持 Control+G、1 起始码点行列、非法输入提示、焦点恢复及八语言。
新增薄 ui.GoToBar 与示例互斥工具栏组合。显示选项不触发正文重排。
Linux / Pango 全仓 GOWORK=off 测试通过；新增纯逻辑/假排版断言与独立
Xvnc :140 / Xfwm4 的原生按键/CommitText 窗口在 Software 环境 1x 及 2x Dark 运行通过。
本窗口未独立探测后端类型，环境设置不作为实际比例/后端证据；未执行物理 IME、RTL 视觉或跨平台运行。
GOUI 无新增要求。

### C1/C2

内置 Markdown、六种现有语言围栏、标题/围栏折叠、列表/引用 Enter、默认配色，
以及 Tags 策略、标签闭合/匹配和名称一次事务联动已接入。ui.LinkedTags 默认协调开启，
主示例增加 guide.md，自定义示例基于内置定义副本。设计文档记录了词法 profile 和尺寸边界，
未宣称完整 CommonMark/DOM 一致性，未增加编程语言。
Linux GOWORK=off 全仓及 race 通过；分片一致性、子定义隔离、保守标签规则、
名称联动/撤销和嵌入补全身份有断言。独立 Xvnc/Xfwm4 的 markup 窗口以
Software 环境 1x、2x Dark 正常 Window.CommitText/DispatchEvent 验证闭合、名称删除与撤销、
HTML 空元素、列表以及围栏内 JSON 配对和换行。比例/后端设置未在该窗口独立探测，
物理 IME/跨平台原生运行未执行；GOUI 无新增要求。
