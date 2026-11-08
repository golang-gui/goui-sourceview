# SourceView 路线

本项目定位为完整代码编辑器，独立实现控件并按需复用 `goui/gui/textedit`。
详细规格见 [总设计](DesignSourceView.md)。专题设计区分已实现契约与提案，
历史实施记录保留当时的验收范围；本文件维护未来方向。

## 当前优先级（2026-11-04）

P0 已随 v0.4.0 发布；实现边界和验证见 [P0 设计](DesignP0Editing.md)。
以下优先级保留路线依据，后续重点进入 P1。

当前开发分支已具备基础编辑、多语言高亮、查找替换、配对/标签联动、编辑辅助、折叠、
行标记和色块地图，以及独立外部高亮与补全刷新。开发分支能力不等于已发布版本；
详细盘点、参考证据和实现边界见 [能力差距与优先级](DesignCapabilityGaps.md)。

| 优先级 | 推进方向 | 设计/边界 |
| --- | --- | --- |
| P0 | 扩充配置/标记语言折叠、独立外部折叠结果、子词和结构选择、缩进与单光标命令 | [P0 设计](DesignP0Editing.md)，已随 v0.4.0 发布 |
| P1 | 代码片段、补全详情、保存点、视图状态恢复、特殊字符表示 | 下一阶段按实际应用细化 |
| P2 | 诊断、Hover/参数提示、结构导航、多光标/矩形选择、通用范围装饰 | 保持后置；外部分析应用需要时上调诊断等能力 |
| P3 | Diff/Merge 配套、Inlay/CodeLens/行内补全、固定结构标题、token 字体样式、长尾语言 | 按具体使用场景推进 |

P0 按语言折叠、外部折叠、结构选区、缩进与整理拆分提交。P1/P2/P3 仍是路线建议，
不代表实施授权。

大文件正常操作与 UI 响应性作为横向质量门槛：尤其复核同步正则/多行查询、全量折叠结果
接收和批量编辑最终事务。未测量的同步路径只记为风险，不宣称已复现卡顿或已达到延迟目标。
不以任意切片、丢失跨段语义或关闭正确性检查换取表面流畅。

已有专项设计：[语言扩展](DesignLanguageExtensions.md)、[折叠](DesignFolding.md)、
[行标记](DesignLineMarks.md)、[色块地图](DesignMinimap.md)、
[外部高亮](DesignExternalHighlights.md)、[外部分析示例](DesignExternalAnalysisExample.md)。
单光标与 UTF-8/LF 模型保持当前基线；未引用的历史首版计划不作为当前缺口清单。

其他按需演进方向包括可撤销的输出换行配置、混合换行逐行保真和语法定义生态。
文件管理、项目索引、LSP/DAP 连接、Git 与终端继续由应用/独立配套承担。

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
