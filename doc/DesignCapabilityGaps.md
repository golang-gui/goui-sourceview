# 编辑器能力差距与优先级

状态：2026-11-04 实施前的能力盘点与路线建议。P0 系列正在实施；以下代码事实保留为盘点基线。
基线为当前开发分支，包含行标记、色块地图、独立外部高亮和补全刷新；不能把这些全部
写成已发布 v0.3.0 的能力。具体已实现契约以各专题和代码为准，旧计划中的“首版”不代表当前全貌。

## 判断依据

当前语言高亮覆盖已经较广。下一阶段重点是折叠和编辑辅助的覆盖、日常操作完整性，
然后改善使用方接入外部语法/语义结果的体验。多光标、通用装饰及语言服务呈现继续后置。
优先级结合实际编辑频率、当前已有设施、实现风险和应用需求，不机械追平参考框架的功能数。

| 参照 | 重点参考 | 当前差距 |
| --- | --- | --- |
| Monaco | 结构编辑、补全内容、语言服务结果呈现、视图状态 | 代码片段、结构选择、诊断、参数提示等尚未形成完整体验 |
| Scintilla | 文本操作、列编辑、范围指示器、保存点 | 单光标命令仍需补齐，缺少保存点、多选区及范围标注 |
| GtkSourceView | 缩进定制、Snippet、Hover、搜索上下文 | 基础职责划分接近，但专项能力的覆盖与深度不足 |

Monaco 控件与 VS Code 工作台分别判断；项目树、终端、Git、LSP/DAP 会话不是本文要求
SourceView 内置的功能。文档有某个接口也不证明所有语言和平台均具备同等体验。

## P0：已有编辑能力的覆盖与深度

| 项目 | 当前代码事实 | 目标与顺序 | 相对成本 |
| --- | --- | --- | --- |
| 折叠覆盖 | 内置 Folding 主要在 JSON、XML、HTML、Markdown；JSONC 目前没有绑定 JSON 折叠 | JSONC/JSON5、YAML、TOML优先，再扩展常用括号块、注释块和 region | 中 |
| 外部折叠结果 | FoldingFunc 绑定不可变语言定义，没有独立提交结果的接口 | 应用复用已有分析结果，更新候选时不更换语言或清理补全 | 中 |
| 缩进与输入辅助 | 已有换行回调、配对、标签联动和智能退格，各语言深度不一致 | 补配置/标记语言换行，明确重新缩进与粘贴缩进的安全范围，再补常用代码语言 | 中 |
| 单光标操作 | 已有行操作、按词移动、注释、括号跳转 | 子词导航、结构选区优先；随后合并行、大小写转换、清理尾部空白 | 小到中 |

详细设计与实施切片见 [P0 设计](DesignP0Editing.md)。这些目标不包括完整格式化器、
代码片段会话、多光标或公共 AST 框架。

## P1：日常编辑器应用的完整体验

| 项目 | 缺口 | 边界 |
| --- | --- | --- |
| 代码片段 | Completion 只有文本编辑与最终 Caret，没有占位符跳转/镜像会话 | 需处理 Tab/Shift+Tab、取消、Undo/Redo、IME、外部编辑；不能只识别 `$1` |
| 补全呈现 | 缺少种类、独立文档说明、弃用标识、按需加载详情 | 先做种类和说明；Provider 继续拥有过滤排序，不照搬全部字段 |
| 保存点 | 有 Revision 和历史，没有直接的保存点契约 | 撤销回保存状态也应正确；普通历史能力优先评估 GOUI textedit，文件保存仍归应用 |
| 视图状态 | 没有统一导出/恢复选区、滚动锚点、折叠意图的接口 | 用于标签页切换和恢复；不保存原生布局，不负责标签页或持久化存储 |
| 特殊字符 | 空白与行尾已有，零宽/双向控制符/其他控制字符未覆盖 | 让不可见内容可辨识，不把所有非 ASCII 文本视作异常 |

保存点不能用 `Revision != savedRevision` 代替：撤销也会产生新修订。输出换行格式、
应用编码等非正文配置是否导致“未保存”需要单独组合，不能混进文本历史的修订含义。

## 横向质量检查：大文件查询响应性

`internal/editor/search.go` 的正则首次查询仍同步复制全文、执行
FindAllStringSubmatchIndex；多行字面查询也缓存规范结果序列。计数界面分帧并不等于
首次匹配已分帧。这是源码可确认的同步路径，本次未重新测量真实阻塞时间。

应以正常窗口输入验证查询切换、取消、替换和编辑并发的体验。后续执行策略必须保留
跨行锚点、跨段匹配、捕获组和规范非重叠顺序，不能任意切片调用正则。可取消、过期丢弃、
进度反馈与工作预算是目标；是否使用 worker 或事件循环分步按算法性质决定，不预定新接口。
若复现明显冻结，作为质量问题优先修复，不因功能排期而延后。

## P2：语言服务呈现与高级编辑

| 项目 | 当前基础 | 尚缺 |
| --- | --- | --- |
| 诊断 | 外部前景高亮、行标记 | 范围、严重程度、波浪线、消息、问题导航 |
| Hover / 参数提示 | Popover、异步请求与生命周期经验 | 独立触发规则、内容、签名/参数强调及失效处理 |
| 结构导航 | 行列、括号、标签导航 | 文档符号、定义/引用目标、语义引用、重命名请求 |
| 多光标/多选区/矩形选择 | 单选区与批量编辑事务 | 多点输入、列复制粘贴、选中下一匹配、IME 和撤销语义 |
| 通用范围装饰 | 搜索背景、行标记、Highlights | 自定义背景/边框/下划线和命中、叠加规则、编辑后映射 |

Highlights 只负责前景，不能顺手扩成诊断、可执行补全或任意装饰集合。各功能分别明确
数据所有者和失效规则。若应用重点转为外部语言分析驱动的代码编辑器，可把诊断、Hover、
参数提示上调 P1，仍不要求控件管理 LSP 连接。

格式化和重命名的文本应用已有 ApplyEdits(document, revision, edits, selection)，能形成
一次撤销事务。先用实际适配器验证缺口，再决定是否新增 Provider，避免平行编辑体系。

## P3：按具体场景推进

- Diff/Merge：Diff 文件高亮不等于差异编辑器；更适合作为组合多个 SourceView 的配套控件。
- Inlay hints、CodeLens、行内补全、固定结构标题：涉及行内/行间布局，按应用价值评估。
- token 字重/斜体/字号：会改变测量身份，需要独立设计，不能当作纯颜色扩展。
- 新的长尾语言、完整语法定义生态：优先维护现有扫描器、混合语言边界和实际文件问题。

建议批次：折叠与结构编辑 → 输入与补全体验 → 编辑器应用状态与按需的语言服务呈现。
此为路线，不是对 P1/P2/P3 的实现授权。

## 证据与适用条件

2026-11-04 核对。Monaco 在线 typedoc 未在页面固定发行号，以下为查阅时接口承诺，
不据此断言内部算法或性能。GtkSourceView 页面为 API 5，View 为库 5.22.1，Snippet
页面显示 5.21.1；Scintilla 文档标注 2026-10-01。此次没有重跑参考编辑器或跨平台性能比较。

- [Monaco languages](https://microsoft.github.io/monaco-editor/typedoc/modules/editor_editor_api.languages.html)：各语言能力的接入边界。
- [CompletionItem](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.CompletionItem.html)：项目种类、文档、插入规则和附加编辑。
- [FoldingRangeProvider](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.FoldingRangeProvider.html)：独立折叠请求和变化通知。
- [Marker](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.IMarkerData.html)、[Decoration](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.IModelDecorationOptions.html)：诊断与范围装饰分别建模。
- [视图状态](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.IStandaloneCodeEditor.html#saveViewState)、[DiffEditor](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.IStandaloneDiffEditor.html)：视图快照和独立差异控件。
- [Scintilla](https://www.scintilla.org/ScintillaDoc.html)：子词操作、保存点、多选区、字符表示、指示器与 call tip。
- [GtkSourceView](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.View.html)、[Snippet](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.Snippet.html)、[SearchContext](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.SearchContext.html)：可组合的专项编辑能力。
