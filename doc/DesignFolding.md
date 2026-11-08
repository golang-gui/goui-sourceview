# 代码折叠

状态：已确认设计，基础实现与验证见 [编辑能力计划](PlanEditorFeatures.md)。基础实现始于 GOUI v0.2.0，当前分支依赖 v0.7.0。新增语言覆盖分为 [第一批](PlanFoldingLanguages.md) 与第二批（下述六种语言）；两批均尚未发布。

## 范围与所有权

语言定义的可选 Folding 函数从不可变 UTF-8/LF 全文生成 FoldRange。范围行号从 0 开始，
StartLine 是保留的标题行，EndLine 包含在范围内；隐藏 StartLine+1..EndLine。
范围允许嵌套，不允许交叉；排序、去重、同起点保留外层、越界检查由组件完成。
空结果是正常无区域，nil 函数关闭该语言的折叠，不添加 ErrNoFold。

Document 共享候选范围与分析任务；SourceView 独立保存折叠状态、可见行映射和几何。
一个视图的折叠不影响另一视图，不改文本、模型修订或撤销历史。只读允许折叠。

## 分析与失效

首次挂载请求分析；连续编辑合并等待 150ms。每个 Document 最多一个 worker 和一个最新请求。
GUI 按 64 KiB/步复制固定修订的字符串片段，worker 组装快照；后台只访问快照、不可变规则和 context。通过 gui.App.Post 回到
GUI 后验证文档身份、修订、语言定义身份和任务代次。取消/过期不是错误；有效失败经现有
ConnectError 报告一次。最后视图卸载取消任务并解除 owner 引用。

首轮完整扫描优先完成行为；快照成本与扫描耗时分别测量，不以后台扫描承诺 GUI 延迟上限。
内置规则可复用纯词法函数，使用独立状态；不访问 GUI 高亮缓存，不改变现有 Tokenizer 调用契约。

编辑前后按实际 Change.Edits 顺序映射私有字节锚点。触及区域或边界立即展开并失效；
不相交区域映射后可保留状态。新结果仅继承可明确匹配的状态；Reset/换文档/换定义清理旧候选。
卸载保存视图意图，重挂载核对修订，未验证区域不隐藏。

## 可见行与原生布局

位置转换：真实文档 → IME 段落投影 → 折叠可见段落 → 原生显示行/Geometry。
GOUI HeightIndex 将高度限制为至少 1，因此仅索引可见段落，不用零高度表达隐藏。
私有可见映射保存连续行段与前缀计数；真实行与可见序号双向查询。原生缓存仍由真实
行身份和 IME 投影身份确定，可见序号不成为正文身份。纯折叠保留未变化的 native layout。
行号报告真实行号，软折行不重复编号；侧栏固定，不随横向滚动。隐藏行不计内容高度。

绘制、命中、导航、翻页、锚定、reveal、候选框与快照使用同一映射。锚点变隐藏时移到
可见祖先标题。顶部锚定与 caret reveal 沿用现有优先级，不在 Paint 里重排。

## 交互

- 默认开启、全部展开；关闭折叠展开全部。有规则时固定保留折叠列，关闭行号仍可操作。
- Fold/Unfold/ToggleFold 参数指定标题行，bool 表示实际变化；无区域/尚无结果返回 false。
- FoldAll/UnfoldAll 更新全部层级；单独展开父区域保留子区域意图。提供独立副本的区域查询和统一变化信号。
- 上下/翻页与左右跨区域移动跳过隐藏内容。Shift 扩选仍是包含隐藏正文的真实连续范围。
- 折叠导致选区端点隐藏时收为标题行末尾光标；两端可见的跨区域选区保留。
- 复制/显式删除已有选区包含隐藏正文；无选区时在折叠边界 Delete/Backspace 首次只展开。
- Find/GoTo/SetSelection/Undo/Redo 指向隐藏内容时展开必要祖先再 reveal。
- IME 开始前展开替换范围；预编辑期间不执行折叠命令，分析结果等待合成结束后重新核验。
- 键盘折叠/展开作用于光标最内层区域；Windows/Linux Ctrl+Shift+[ / ]，macOS Cmd+Option+[ / ]。
- 按钮和淡色省略标记沿用 Basic/Modern；标记空间不足不覆盖正文，不增加原生软折行。

快照保留一个真实连续 VisibleRange，其余可见原文分段放进现有 goui.sourceview 属性，
合计预算 64KiB。报告可见折叠控件的名称、边界和状态，动作经正常 Window 输入路径。

## 内置规则与测试

当前内置规则如下，不以具备语法高亮推断具备折叠能力：

| 语言 | 自动折叠依据 |
| --- | --- |
| JSON / JSONC / JSON5 | 对象、数组；扩展语法的多行字符串／注释按对应词法器处理 |
| XML / HTML | 确认的成对元素、注释；XML CDATA，HTML void/raw-text 和大小写规则 |
| Markdown | 标题、围栏等标记结构 |
| YAML / TOML | 缩进／块标量、表节、多行字面量及容器 |
| Go / JavaScript / TypeScript / CSS / Python | 词法器确认的括号、跨行字面量／注释；Python 块缩进及对应 region 注释 |
| C / C++ / Java / C# / Rust（第一批） | 词法器确认的 `{}[]()`、已闭合多行字面量／块注释和下述显式 region |
| Kotlin / Swift / Dart / Scala / Groovy / PowerShell（第二批） | 词法器确认的 `{}[]()`、已闭合多行字面量／块注释、插值表达式和显式 region |

其他内置语言本轮不增加通用括号或缩进回退，避免将字符串、关键字块或嵌入语言误判。
不完整结构只发布已确认范围；结束行包含其他语句时保持该行可见。新增五种语言的
结束行若仅剩分号或逗号，视为原结构的终止符，可随结构折叠。第二批沿用这一规则。

### C、C++、Java、C#、Rust

沿用现有 Tokenizer，不引入另一套词法规则。每次 FoldingFunc 调用建立私有观察器，
从扫描器实际确认的打开、关闭、恢复位置收集字面量和块注释范围；不修改注册的
Tokenizer、不可变 State 或 GUI 高亮缓存。外层 C# 字符串跨插值表达式保留，
嵌套字符串独立入栈；Rust 块注释支持嵌套。关闭一个字面量又在同一行打开另一个时，
按各自真实边界处理，不能仅比较行首／行尾 Context.String。

- C/C++：普通字符串的物理续行、C++ 自定义分隔符的原始字符串和块注释。
  宏指令及其续行不参与代码括号配对；仍分析其中实际确认的多行字面量／注释。
- Java：文本块、块注释，以及词法器确认的 Unicode 转义产生的括号；解码仅作用于
  ClassDelimiter 片段，使用原始源文本字节位置，行号始终对应物理 LF 行。
- C#：verbatim/raw 字符串、插值表达式中的代码括号与嵌套字符串、块注释。
- Rust：普通／raw／byte／C 字符串、嵌套块注释、宏 token tree 括号；
  生命周期、标签和 raw identifier 不视为字符串。

五种语言均支持独占行的 `// region`、`// endregion`（可带 `#`）；C/C++ 另支持
`#pragma region` / `#pragma endregion`，C# 支持 `#region` / `#endregion`。
指令须由词法器确认为指令，宏续行、字符串和块注释中的伪 marker 不生效。
未闭合和与结构范围交叉的 region 不发布；排序、去重、同起点外层保留仍由组件完成。

这是词法折叠，不执行预处理器，不推导条件编译分支、C++ 模板类型、Rust 宏展开或
编译器 AST，也不增加 `#if` / `#else` / `#endif` 分支折叠。

### Kotlin、Swift、Dart、Scala、Groovy、PowerShell

沿用同一单次 worker 观察器，在现有六个扫描器的真实打开／关闭／恢复点采集边界。
进入或离开插值表达式不关闭外层字面量，表达式中的字面量独立入栈。公开 Scan 调用
传入 nil 观察器，分词规则与 State 契约保持原样；没有第二套词法分析器或公开 API。

| 语言 | 特有边界 |
| --- | --- |
| Kotlin | 三引号、多 dollar 插值、普通／嵌套表达式字面量、嵌套块注释 |
| Swift | 三引号、扩展分隔符字符串／正则、匹配 hash 数量的插值、嵌套块注释 |
| Dart | 单／双引号三引号与 raw 字符串、插值表达式、嵌套块注释 |
| Scala | 三引号、自定义插值前缀及 raw 字符串、插值表达式、嵌套块注释 |
| Groovy | 三引号、slashy／dollar-slashy、转义续行字符串和插值、块注释 |
| PowerShell | 普通多行引号、单双引号 here-string、`$()` 表达式及内层字面量、`<# #>` 块注释 |

Kotlin／Scala 延迟确认三引号结束的路径也记录准确的源位置，包括在行尾确认关闭。
PowerShell here-string 关闭符须由扫描器在行首确认；错误打开头会撤销该字面量，
缩进的关闭符仍作为正文。PowerShell 块注释在首个 `#>` 结束，不按嵌套注释处理。
反引号标识符和字符串中的变量不会变成独立多行折叠。

前五种语言支持独占行的 `// region` / `// endregion`（可带 `#`），
PowerShell 支持不区分大小写的 `#region` / `#endregion` 注释标记。
字符串／块注释中的伪标记不生效；未闭合或与结构范围交叉的 region 不发布。
多行普通字面量若被扫描器按错误恢复而终止，不将错误恢复当作已闭合范围。

Scala 本批只处理词法括号与字面量，不承诺 Scala 3 缩进块；Swift 条件编译、
Groovy 运行时语义、PowerShell 执行语义也不参与折叠分析。

逻辑测试覆盖嵌套、交叉/非法结果、随机行映射、锚点编辑、取消/过期、对象隔离。
窗口覆盖关闭行号、折行、横向滚动、选择删除、搜索、撤销、共享文档、IME、卸载与主题。

## 参考与取舍

2026-10-19 查阅 [Monaco FoldingRangeProvider](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.FoldingRangeProvider.html)
与 [FoldingRange](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.FoldingRange.html)：借鉴规则与呈现分离，使用本项目零起始行号和直接绑定。
[Scintilla 5 文档](https://www.scintilla.org/ScintillaDoc.html#Folding)（页面 2026-07-22）明确行可见性属于视图。
[VS Code 1.105.0 hiddenRangeModel.ts](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/folding/browser/hiddenRangeModel.ts)
首行保留、合并隐藏祖先；采用这一模型，位置跟踪先保持私有，不引入通用装饰 API。

2026-11-08 补充核对 [VS Code language configuration folding](https://code.visualstudio.com/api/language-extensions/language-configuration-guide#folding)
与 [Scintilla folding](https://www.scintilla.org/ScintillaDoc.html#Folding)：保留语言策略与视图状态分离，
不为尚无规则的语言添加统一缩进回退。词法边界依据
[Java SE 21 Unicode escapes](https://docs.oracle.com/javase/specs/jls/se21/html/jls-3.html#jls-3.3)、
[C# interpolated strings](https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/tokens/interpolated)
和 [Rust comments](https://doc.rust-lang.org/reference/comments.html) 核对。
本轮实现和精确范围测试只使用已有扫描器确认的边界，不宣称完整编译器语法分析。

第二批核对 [Kotlin strings](https://kotlinlang.org/docs/strings.html)、
[Swift lexical structure](https://docs.swift.org/swift-book/ReferenceManual/LexicalStructure.html)、
[Dart strings](https://dart.dev/language/built-in-types#strings)、
[Groovy strings](https://groovy-lang.org/syntax.html#all-strings)、
[PowerShell quoting](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_quoting_rules?view=powershell-7.5)
与 [comments](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_comments?view=powershell-7.6)。
[Scala 3 optional braces](https://docs.scala-lang.org/scala3/reference/other-new-features/indentation.html)
说明其缩进语法需要专门处理，本批按已确认范围保留为后续工作，不用统一缩进回退替代。

折叠占位的水平范围预留行尾标记开启时的最大宽度，切换纯绘制标记不改变正文排版或滚动范围。

## 异步结果与几何稳定性

仅改变候选范围而没有改变可见段落集合时，acceptFolds 保留已测量高度、原生段落缓存、
已完成的 reveal 和顶部锚点，只更新控制与绘制。只有折叠可见映射变化（或初始化/行数失配）
才重建高度。不能让后台结果把实际行高退回估计并恢复另一个顶部位置。

同样的原则适用于编辑时 mapFolds 对候选的映射与移除：没有隐藏行时，不因存在
候选就清空正文布局或 HeightIndex。按实际 LF splice 更新高度，并同步可见行映射
的总数，防止后续 ensureHeights 因旧行数不匹配重置未变后缀。
共享视图的局部编辑、插入 LF 和原生大文档回归见 [真实文档验证](PlanCorpusLayout.md)。

映射未相交候选时，复用同一 Change 的逻辑行 splice：位于候选前方的编辑按
Added-Removed 同时平移起止行，字节锚点仍依次 MapOffset。位于候选之后的编辑
不改变行号，不为每个候选重复执行两次 Model.LineAt。仅在修订连续且 splice 与
Edits 一一对应时采用增量；分析索引尚在重建、变更缺失或无可靠 splice 时，保留
按最终字节位置查询行号的精确路径。触及边界的失效规则、折叠意图、历史及各视图
独立状态不变，不用推算代替实际相交检查。

10 MiB JSON 窗口曾复现末尾空行不可见；独立 fallback 字体用例在旧代码中观察到高度由
4176 退回 4032 DIP 并错误排入锚点恢复，修正后保留原高度且 caret 仍在视口。
修正后 JSON/XML/HTML 各 10 MiB 的 53 步窗口断言全部通过。具体复验矩阵见实施记录。

## 独立结果与扩展语言（P0）

`NewFolds(text, []FoldRange)` 在 worker 复制、校验并预计算字节端点；`Document.SetFolds(revision, snapshot)`
显式覆盖默认 FoldingFunc。nil 恢复内置，非 nil 空快照关闭本次候选。只更换候选，不更换 Language，
不影响颜色、历史或补全。视图分步协调（256 项/2ms 软预算）后整体生效，按精确源边界继承各自折叠意图。
编辑后未相交区域继续映射，外部模式不会自动回退内置；Reset 保留来源选择，换语言退出外部覆盖。

新增 JSONC/JSON5、YAML、TOML、Go、JS/TS、CSS、Python 的折叠。引用/注释/块标量使用实际词法器；
TOML 源节不按 dotted key 合成不连续区域，Python 续行与块缩进分别处理。Go/JS/TS/CSS/Python 支持
注释中的 region/endregion，跨越已确认结构的 region 舍弃。边界与压力验证见 [P0 设计](DesignP0Editing.md)。
