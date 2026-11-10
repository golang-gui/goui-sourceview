# 编辑器能力差距与优先级

状态：2026-11-10，基于 v0.5.0（`ee2d146`）代码和官方框架接口复核。
本文维护当前差距、职责边界和推进顺序；提案不是已实现 API，也不授权继续实现。
[P0 设计](DesignP0Editing.md) 与各 Plan 保留历史范围和验收；不能从旧“首版”清单判断当前缺口。

## 定位与已有基础

SourceView 是供应用嵌入的基础代码编辑器。优先完整、稳定的文本编辑与可接入能力，
不以穷尽编程语言语法、构建完整 AST/语义引擎或复制 IDE 工作台为目标。

当前已经具备：

- 单光标 Unicode 编辑、选区、历史、行操作、子词导航与结构扩选、缩进和整理命令。
- 多语言词法高亮、混合语言、常见结构/关键字/章节/记录/多行值折叠，独立外部折叠结果。
- 基于实际显示行数的长行折叠、统一 gutter 与结构单行预览；小于三个显示行不提供长行折叠入口。
- 查找替换、正则、跨行查询、选区范围、行标记、Minimap、外部前景高亮及补全刷新。
- 普通补全列表与默认关闭的宿主提供型单行/多行影子建议；Tab 整段接受、Esc 清除、单次撤销。
- Basic/Modern、亮暗主题和内置界面本地化；补全输入、水平滚动和全文替换的专项修正。

折叠按当前已确认的通用规则维护实际样本问题，不再按语言名称扩展深层语法解析。
内联建议已随 v0.5.0 发布，不能继续把它整体列为未来功能。

## 官方框架对照

核对的是公开契约，不是三种框架的桌面、性能或所有平台实测。它们提供的层次不同，
不能从“有相关 API”推断功能完全等价，也不把每个框架的全部能力作为 SourceView 的最低门槛。

| 能力 | 官方接口事实 | SourceView 的取舍与当前差距 |
| --- | --- | --- |
| 保存状态 | Scintilla 有保存点和进入/离开通知；Monaco alternative version 可在 Undo/Redo 时恢复相同值；GTK TextBuffer 有 modified 标记 | 缺可靠的文档保存点；GTK modified 页面本身不能证明与前两者完全相同的历史身份语义 |
| 视图状态 | Monaco 有 saveViewState/restoreViewState，快照区分光标、视口与贡献状态 | 缺统一捕获/恢复接口；不据此声称 Scintilla/GTK 也提供同等的一体化快照 |
| 补全呈现 | Monaco 区分 kind/detail/documentation/deprecated；GtkSourceView 有图标、正文、注释、详情列；Scintilla 可显示候选类型图像 | 当前只有 Label/Text/Detail 等编辑信息；先补类型、完整说明、弃用状态，不照搬全部 Provider 字段 |
| 代码片段 | Monaco 支持 snippet 插入规则；GtkSourceView 有默认值、编号焦点、Tab/Shift+Tab 和引用 | 缺占位符会话；这是编辑能力，不要求语义解析，但此前已决定暂缓 |
| 多选区/列编辑 | Monaco 有多选区接口；Scintilla 有多选区、矩形选择与多点输入 | 当前只有单选区；属于明确的控件缺口，但需要独立处理 IME、剪贴板和撤销。不将其归为三者共同具备的能力 |
| 范围装饰 | Monaco 有 decorations；Scintilla 的 indicators 可独立覆盖正文范围 | 当前 Highlights 只改前景色，缺宿主可用的背景/边框/下划线；从实际标注需求设计最小范围，不预建任意装饰平台 |
| 不可见字符 | Monaco 有 invisible/ambiguous Unicode 高亮选项；Scintilla 可为字符配置可见表示 | 当前已有空白/行尾标记；零宽、双向控制和其他控制字符仍缺表示，不能把所有非 ASCII 字符当异常 |
| Hover/参数提示 | GtkSourceView 有外部内容提供器；Scintilla 的 CallTip 接收宿主文字并可强调参数范围 | 呈现不依赖控件内置语义分析；诊断、Hover、参数提示可按宿主接入需要推进 |
| 内联建议 | Monaco 的普通 InlineCompletion 支持替换范围、片段和部分接受通知；多行 insertText 要求范围在行末结束，替换范围本身在同一行内 | 当前行末追加是合理子集；后续考虑部分接受、匹配前缀续用、单行范围替换，不直接扩成任意多行改写系统 |

## P1：近期补齐

### 补全信息展示

当前 Completion 有名称、字面量、单行 Detail、替换范围、附加编辑和最终光标；
缺候选类型、独立完整说明和弃用状态。先改善用户判断候选的能力。
细化方案见 [补全呈现提案](DesignCompletion.md#p1-补全呈现提案尚未实现)。
保留 Provider 对过滤、排序、网络与分析的所有权，普通补全与影子建议继续互斥。

### 视图状态恢复

统一捕获并恢复选区、滚动文本锚点和折叠意图，供标签页切换、虚拟化或视图销毁重建使用。
第一批只支持同 Document、同 Revision，不保存原生布局、临时补全或输入法状态，
不承诺跨进程持久化、过期偏移映射。使用现有有界排版调度，不能为了恢复同步测量全文。
见 [视图状态提案](DesignViewState.md)。

### 特殊字符表示

在现有空白与行尾功能之上，优先让零宽和双向控制字符可辨识，按需要扩展其他控制字符。
显示替代标记不能改写保存文本，必须核对原生 cluster、光标、命中、选区、换行与 IME。
不凭字符宽度估算覆盖原生几何，也不把普通中文或其他非 ASCII 文本默认标为问题。
正式实现前单独细化契约；发现 GOUI 不能表达的排版需求先记录并评估绕过。

### 保存点：价值高，依赖未满足时跳过

Model.Revision 在编辑、Undo、Redo 时递增，不能直接作为保存身份。
GOUI v0.7.0 的公开 Model 没有可随历史恢复的状态标识；已记录 [保存点设计](DesignSavePoint.md)。
不在 SourceView 复制历史栈、逐次复制/哈希全文，或借私有字段模拟该能力。
宿主可暂用保守的未保存提示，但不能把它宣称为精确的历史保存点。
依赖无法可靠绕过时跳过，继续视图状态和特殊字符；将来有明确授权时再按 GOUI → SourceView 顺序实施。

## P2：按具体应用需求推进

| 方向 | 建议边界 |
| --- | --- |
| AI 建议交互 | 按词/按行接受 → 输入匹配时保留余文 → 单行范围替换；先明确撤销边界、余文坐标、过期结果及布局，不要求宿主改变 AI 服务架构 |
| 范围装饰 | 从真实的范围背景或下划线需求开始，明确修订校验、编辑失效/映射、叠加顺序与命中；不把 Highlights 扩成混杂的诊断/装饰协议 |
| 诊断、Hover、参数提示 | 应用提供范围、消息和签名；控件负责可见呈现、导航、输入与生命周期，不负责 LSP 会话或语言解析 |
| 多光标/矩形选择 | 先确认重叠/主选区、批量编辑、列复制粘贴、IME 与历史语义；不能仅增加多根光标绘制 |
| 代码片段 | 保留 [暂缓设计](DesignSnippets.md)，不恢复实施；恢复时核对新增影子建议的互斥和 Tab 优先级 |

上述顺序是设计建议，不是这些功能已经具备；多光标和片段属于成本较大的独立专项。
外部分析应用有明确需求时，可以上调诊断/范围装饰，不要求先完成所有前置方向。

## 横向质量检查：大文件查询响应性

`internal/editor/search.go` 的正则首次查询仍同步复制全文并执行 FindAllStringSubmatchIndex；
多行字面查询也会缓存完整的规范结果。计数分帧不等于首次匹配已经分帧。
这是当前源码可确认的同步路径，本轮没有重新复现冻结或测量阻塞时长。

先以正常窗口输入测量查询切换、取消、替换和编辑并发，再确定修复方案。
执行策略必须保留跨行锚点、捕获组和规范非重叠顺序，不任意切片正则或放弃正确性。
若确认明显冻结，按质量问题优先处理，不因功能排期延后。

## 后置范围与职责

- 文档符号、定义/引用目标、重命名、格式化接入按宿主需求评估；已有 ApplyEdits 可应用外部编辑结果。
- Diff/Merge 宜作为组合多个 SourceView 的配套；文件 Diff 高亮不等于差异编辑器。
- Inlay hints、CodeLens、固定结构标题、token 字重/斜体等需要独立布局设计，保持后置。
- 项目树、文件管理、编码/BOM、磁盘冲突、Git、终端、LSP/DAP 连接和 AI 模型/网络由应用或独立配套负责。
- 不要求新增长尾语言或完整语法定义生态；优先修复已有规则对真实文件的明显问题。

## 证据与适用条件

2026-11-10 查阅官方公开文档。Monaco 在线 typedoc 未固定发行号；Scintilla 文档标注
2026-10-01；GtkSourceView 使用 API 5，所查 CompletionColumn/Snippet 页面为 5.22.1，
HoverProvider 页面为 5.20.1；GTK4 modified 页面为 4.23.3。版本页面可能独立更新，
本文不从这些接口推断内部算法、性能或所有原生平台的同等实现。

- [Monaco CompletionItem](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.CompletionItem.html)、[Snippet 插入规则](https://microsoft.github.io/monaco-editor/typedoc/enums/editor_editor_api.languages.CompletionItemInsertTextRule.html)。
- [Monaco alternative version](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.ITextModel.html#getAlternativeVersionId)、[视图状态](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.ICodeEditor.html#saveViewState)、[快照内容](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.ICodeEditorViewState.html)。
- [Monaco 多选区](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.ICodeEditor.html#getSelections)、[范围装饰](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.ITextModel.html#deltaDecorations)、[Unicode 表示](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.IUnicodeHighlightOptions.html)。
- [Monaco InlineCompletion](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.InlineCompletion.html)、[部分接受通知](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.InlineCompletionsProvider.html#handlePartialAccept)。
- [Scintilla 保存点](https://www.scintilla.org/ScintillaDoc.html#SCI_SETSAVEPOINT)、[多选区](https://www.scintilla.org/ScintillaDoc.html#SCI_SETMULTIPLESELECTION)、[范围指示器](https://www.scintilla.org/ScintillaDoc.html#SCI_INDICATORFILLRANGE)、[字符表示](https://www.scintilla.org/ScintillaDoc.html#SCI_SETREPRESENTATION)、[候选图像](https://www.scintilla.org/ScintillaDoc.html#SCI_REGISTERIMAGE)、[CallTip](https://www.scintilla.org/ScintillaDoc.html#SCI_CALLTIPSHOW)。
- [GtkSourceView 补全列](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/enum.CompletionColumn.html)、[片段焦点](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/snippet-reference.html#focus-positions)、[HoverProvider](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/iface.HoverProvider.html)。
- [GTK TextBuffer modified](https://docs.gtk.org/gtk4/method.TextBuffer.get_modified.html)。

本项目的当前行为来自源码和各专项验收；上述框架能力来自文档。
优先级、简化范围和推进顺序是结合本项目定位作出的设计判断，不是外部文档的承诺。
