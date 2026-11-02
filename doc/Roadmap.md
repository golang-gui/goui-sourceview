# SourceView 路线

本项目定位为完整代码编辑器，独立实现控件并按需复用 `goui/gui/textedit`。
首版必须包含可编辑基线及 **JSON、XML、HTML 高亮**。
详细规格见 [总设计](DesignSourceView.md)，实施切片及验收状态只在 [Plan](Plan.md) 维护。

## 首版开发主线

1. 核对公开接入与原型风险，建立 Document、行元数据和编辑计划。
2. 独立 Widget 实现输入、IME、选择、滚动与原生布局。
3. 三语言扫描器、增量失效、配色及可见工作调度。
4. 完成实际所需的 GOUI 批量编辑、Tab、剪贴板通用扩展并接入。
5. 集成缩进、查找替换、行号、装饰、声明式、示例与跨平台验收。

这些步骤不以扩展 TextView 为前置。G1/G2/G3 的具体契约与阻塞关系见
[GOUI 接入设计](DesignGOUIIntegration.md)，不把提案写成当前依赖已有能力。

## 首版边界

单光标、字面量搜索、词法高亮；HTML 的特殊正文边界及内层 JavaScript/CSS 着色已有支持。
未知语言保留普通文本编辑。
首版不引入完整 IDE、浏览器编辑器或必须完成完整解析才能输入的依赖。

## 后续代码编辑能力

当前优先顺序为折叠、编辑辅助、标记语言完善与查找增强；具体实现状态见
[结构化编辑记录](PlanEditorFeatures.md)。编程语言、多光标、通用装饰及语言服务呈现后置。

当前先验证中大文件的打开、编辑、导航与搜索替换，优先修正在实际场景复现的功能问题；
资源占用和系统性性能优化后置。窗口结果、定位修正及待验证范围见 [中大文件验证](PlanLargeFiles.md)。

- 结构/标签联动、折叠规则扩充、行标记与丰富侧栏。JSON/XML/HTML 确认范围的基础折叠已有实现，详见上述记录。
- 多光标、矩形选区和更完整的编辑/导航命令。
- 语法树驱动的缩进、结构化注释操作及代码片段。
- 更多语言、JSX/TSX、语言方言细化与规则生态；本轮 C/C++、Java、Rust、C#、Lua 的具体范围见 [高亮设计](DesignHighlighting.md#12-ccjavarustclua)。
- 书签及装饰的稳定位置追踪。
- 与文本历史一致的可撤销换行格式转换，以及按需提供的混合换行逐行保真。
- 查询后台化与更严格的峰值预算，在专项测量后决定。
- 异步语义补全、Schema/LSP、悬浮信息、诊断及语言服务。
- 经测量确有需要的后台分析、快照、超长行窗口化 shaping。

这些是演进方向；具体优先级和公共 API 按专题设计，不为首版预建完整扩展框架。

## 语言扩充顺序

现有标记、配置和脚本语言保持维护，新增编程语言按可验证的词法状态逐批推进：

1. C、C++、Java、Rust、C#、Lua：已有识别、引用/注释状态、编辑规则、建议及样本。
2. PowerShell、Ruby、PHP、JSX/TSX：已有对应引用、插值、heredoc 与混合语言规则，具体范围见高亮设计；验收结果见语言验证记录。
3. Kotlin、Swift、Dart、R：已有词法、编辑规则及示例。
4. Zig、Objective-C、Objective-C++、Scala、Perl：已有词法、编辑规则及示例。
5. Diff/Patch、Windows Batch、HCL/Terraform、GraphQL、SCSS、Less：已有词法、相应编辑规则及示例。
6. Go Modules、Dotenv、Properties、Git Ignore/Attributes、Starlark、Groovy：已有独立语言配置、词法、相应编辑规则及示例。
7. Go Template、Jinja2、systemd unit、Desktop Entry、Nix、jq、PromQL、F#、Elixir、Erlang、Julia、Clojure、Haskell、OCaml：已有独立词法、编辑规则及示例；两种模板可显式组合 HTML 宿主。

各批语言的验收状态见语言验证记录。优先复用现有 token、上下文和
分片协议，新增语言不以引入解析器框架或语言服务为前置。

- CSV：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- TSV：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- AsciiDoc：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- reStructuredText：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- TeX/LaTeX：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。

- BibTeX：已有词法、相应编辑规则及样本；范围见 [文本格式设计](DesignTextFormats.md)。
