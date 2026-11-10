# SourceView 路线

本项目定位为完整代码编辑器，独立实现控件并按需复用 `goui/gui/textedit`。
详细规格见 [总设计](DesignSourceView.md)。专题设计区分已实现契约与提案，
历史实施记录保留当时的验收范围；本文件维护未来方向。

## 当前优先级（2026-11-10）

v0.5.0 已发布；P0 基础编辑和本轮通用折叠覆盖、长行折叠、单行/多行影子建议已实现。
当前能力、框架对照、明确缺口和依据见 [能力差距与优先级](DesignCapabilityGaps.md)。
这份路线不要求深入每种语言的语法语义解析，也不表示所有方向都需要立即实施。

| 顺序 | 推进方向 | 状态与边界 |
| --- | --- | --- |
| 近期 | 补全类型/完整说明/弃用状态 | [补全呈现提案](DesignCompletion.md#p1-补全呈现提案尚未实现)，尚未实现 |
| 近期 | 视图状态恢复 | [提案](DesignViewState.md)，首批同文档同修订，保存选区/滚动锚点/折叠，不保存临时 UI |
| 近期 | 特殊字符表示 | 零宽、双向控制等可见表示；先细化原生排版与命中契约，不改保存内容 |
| 条件接入 | 保存点 | [提案](DesignSavePoint.md)；GOUI v0.7.0 缺历史状态标识，无法可靠绕过时跳过，不阻塞其他工作 |
| 按需 | AI 建议部分接受/前缀续用、范围装饰、诊断/Hover/参数提示 | 现有影子建议已发布；其增强及外部结果呈现尚未实现，分析和网络归宿主 |
| 独立专项 | 多光标/矩形选择、代码片段 | 属于明确的编辑能力；多选区需完整输入/历史设计，[片段仍暂缓](DesignSnippets.md)，不自动恢复实施 |
| 后置 | Diff/Merge 配套、Inlay hints、CodeLens、固定结构标题、token 字体样式 | 由真实需求驱动，不扩大为 IDE 工作台或完整语言服务 |

多行影子建议的已实现范围见 [内联建议](DesignInlineSuggestion.md)；未来交互不能写成当前 API。
代码片段保留此前暂缓决定；本轮只记录设计，不实现表内新增方向。

大文件查询响应性作为横向质量项：源码仍有同步正则全文处理与多行结果缓存，
先复现和测量，不把未测路径宣称为卡顿，也不以任意切片牺牲跨行匹配语义。

语言高亮和折叠优先维护已有扫描器、通用结构与实际样本，不继续按语言名称穷尽深层折叠规则。
文件管理、编码/BOM、磁盘冲突、项目索引、Git/终端、LSP/DAP 连接和 AI 服务由应用/独立配套负责。
其他按需方向包括可撤销的输出换行配置和混合换行逐行保真。

已有专项设计：[语言扩展](DesignLanguageExtensions.md)、[折叠](DesignFolding.md)、
[长行折叠](DesignLineCollapse.md)、[行标记](DesignLineMarks.md)、[色块地图](DesignMinimap.md)、
[外部高亮](DesignExternalHighlights.md)、[外部分析示例](DesignExternalAnalysisExample.md)。

## 语言扩充顺序

现有标记、配置和脚本语言保持维护，新增编程语言按可验证的词法状态逐批推进：

1. C、C++、Java、Rust、C#、Lua：已有识别、引用/注释状态、编辑规则、建议及样本。
2. PowerShell、Ruby、PHP、JSX/TSX：已有对应引用、插值、heredoc 与混合语言规则，具体范围见高亮设计；验收结果见语言验证记录。
3. Kotlin、Swift、Dart、R：已有词法、编辑规则及示例。
4. Zig、Objective-C、Objective-C++、Scala、Perl：已有词法、编辑规则及示例。
5. Diff/Patch、Windows Batch、HCL/Terraform、GraphQL、SCSS、Less：已有词法、相应编辑规则及示例。
6. Go Modules、Dotenv、Properties、Git Ignore/Attributes、Starlark、Groovy：已有独立语言配置、词法、相应编辑规则及示例。
7. Go Template、Jinja2、systemd unit、Desktop Entry、Nix、jq、PromQL、F#、Elixir、Erlang、Julia、Clojure、Haskell、OCaml：已有独立词法、编辑规则及示例；两种模板可显式组合 HTML/XML/YAML/TOML 宿主。

各批语言的验收状态见语言验证记录。优先复用现有 token、上下文和
分片协议，新增语言不以引入解析器框架或语言服务为前置。

- CSV：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- TSV：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- AsciiDoc：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- reStructuredText：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- TeX/LaTeX：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- BibTeX：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- gettext PO/POT：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- EditorConfig：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- JSON5：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- EDN：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- Git config：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- SSH config：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- crontab：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- Mermaid：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- Graphviz DOT：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- Markdown 已支持 YAML/TOML 前置元数据；字幕 SRT/WebVTT、iCalendar/vCard 交换格式按实际场景后置。
