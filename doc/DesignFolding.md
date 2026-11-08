# 代码折叠

状态：已确认设计，基础实现与验证见 [编辑能力计划](PlanEditorFeatures.md)。基础实现始于 GOUI v0.2.0，当前分支依赖 v0.7.0。新增语言覆盖已有 [第一批](PlanFoldingLanguages.md)、[第二批](PlanFoldingLanguages2.md)，[第三批](PlanFoldingLanguages3.md) 为 JSX/TSX/PHP 混合文档，[第四批](PlanFoldingLanguages4.md) 为 Lua/Ruby/Shell/CMake/SQL，[第五批](PlanFoldingLanguages5.md) 为 Zig/R/GraphQL/HCL/Protobuf/Starlark，[第六批](PlanFoldingLanguages6.md) 为 Objective-C/Objective-C++/SCSS/Less/Perl，[第七批](PlanFoldingLanguages7.md) 为 Nix/jq/PromQL/Clojure/EDN，[第八批](PlanFoldingLanguages8.md) 为 Elixir/Erlang/Julia/OCaml，[第九批](PlanFoldingLanguages9.md) 为 F#/Haskell，[第十批](PlanFoldingLanguages10.md) 为 Nginx、Go 模块／工作区及分节配置文件；[第十一批](PlanFoldingLanguages11.md) 补充基础 DOT／普通 Diff。上述扩展均尚未发布。

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
| JSX / TSX（第三批） | 配对标签、Fragment、代码／表达式括号、模板字符串、注释与代码 region |
| PHP（第三批） | PHP 括号、跨行字符串／heredoc／nowdoc／注释、跨片段模板控制结构，以及静态 HTML 标签／注释 |
| Lua（第四批） | function、条件／循环／do 块、repeat/until 条件、括号、闭合长字符串／长注释 |
| Ruby（第四批） | end 块、括号、多行引号／percent 字面量／heredoc／块注释；区分后置修饰和 endless 方法 |
| Shell（第四批） | if/fi、循环/done、case/esac、代码括号、命令替换、闭合多行引号／heredoc |
| CMake（第四批） | 配对控制命令、跨行参数括号、闭合引号／bracket 字面量与注释 |
| SQL（第四批） | CASE/END、括号、闭合多行引号／dollar 字符串／嵌套块注释；不推断方言 BEGIN 块 |
| Zig（第五批） | 代码括号、扫描器确认的连续 `\\` 多行字符串；不跨空行或另一个赋值续接 |
| R（第五批） | 代码括号、闭合多行引号／raw 字符串；引用名称、注释和自定义运算符内的伪括号不参与匹配 |
| GraphQL（第五批） | 查询／类型／参数／输入值括号、已闭合 block string，排除转义三引号 |
| HCL/Terraform（第五批） | 块／对象／列表／插值括号、闭合字面量／heredoc／块注释、同一模板内的 if/for 指令配对 |
| Protobuf（第五批） | message/enum/service/oneof 等括号结构及实际闭合的块注释／跨行字面量 |
| Starlark（第五批） | def/if/elif/else/for 缩进块、代码括号、实际闭合的三引号／续行字符串 |
| Objective-C / Objective-C++（第六批） | 代码括号、闭合字面量／块注释，以及类／分类／扩展／协议定义到 `@end` 的配对 |
| SCSS（第六批） | 规则／mixin／function／控制块括号、插值代码、闭合字面量／块注释 |
| Less（第六批） | 规则／mixin／detached ruleset 括号、闭合字面量／块注释；变量插值不当作代码块 |
| Perl（第六批） | 代码括号、闭合引用／多段替换／正则／heredoc 和 POD；数据尾部不当作代码 |
| Nix（第七批） | 属性集合／列表／插值括号、let/in 绑定段、闭合普通／缩进字符串和块注释 |
| jq（第七批） | if/then/elif/else/end、def 与结束分号、代码／插值括号和闭合跨行字符串 |
| PromQL（第七批） | 函数／聚合／选择器／范围括号、闭合反引号多行字符串；普通引号按词法器错误恢复 |
| Clojure / EDN（第七批） | 集合括号、闭合跨行字符串、Clojure 正则字面量，以及词法器确认完整的丢弃形式 |
| Elixir（第八批） | do/end、fn/end、代码／位串括号、闭合多行引号／heredoc／sigil，区分 do: 简写 |
| Erlang（第八批） | begin/case/if/receive/try/maybe/end、实际匿名 fun、代码／binary 括号和闭合多行字面量 |
| Julia（第八批） | 显式 end 块、代码括号、闭合多行字符串／字符串宏／嵌套注释，区分推导式和索引 begin/end |
| OCaml（第八批） | begin/struct/sig/object/end、for/while/do/done、代码括号、闭合引号／quoted string／嵌套注释 |
| F#（第九批） | 声明／控制结构与同列分支的专用缩进规则、显式 begin/class/sig/end、循环 done、括号和闭合多行字面量／嵌套注释 |
| Haskell（第九批） | where/let/do/of（及 mdo）引入的布局列表、in／分组结束／逗号终止、显式括号、闭合 string gap 和嵌套注释 |
| Nginx（第十批） | 词法器确认的配置块 `{}`、已闭合的多行引号；括号／方括号不作为块 |
| go.mod / go.work（第十批） | 词法器确认的 `()` 指令分组，排除字符串／注释内的伪分隔符 |
| INI / GitConfig / EditorConfig / systemd / Desktop Entry（第十批） | 完整节标题到下一节前的文本区域，排除值／续行中的伪标题 |
| DOT（第十一批） | graph/subgraph `{}`、属性 `[]`，词法器确认闭合的多行注释／续行引号／HTML-like 字面量 |
| Diff（第十一批） | 普通 Git／unified 文件区域及 `@@` hunk，文件头沿用词法分类，不解释补丁内容 |

其他内置语言本轮不增加通用括号或缩进回退，避免将字符串、关键字块或嵌入语言误判。
不完整结构只发布已确认范围；结束行包含其他语句时保持该行可见。第一批五种语言的
结束行若仅剩分号或逗号，视为原结构的终止符，可随结构折叠。后续批次沿用这一规则。

### DOT 与普通 Diff

按基本编辑器范围补足简单可确认的文本结构，不增加编译器、语义解析或依赖。
DOT 复用现有扫描器，在实际打开／关闭／错误恢复位置记录多行字面量和注释。
只将确认的 `{}`／`[]` 进入结构栈；字符串／注释中的伪括号排除。
HTML-like label 整体作为字面量，不构建 HTML 元素树；普通引号在无续行的 LF 恢复时
撤销未完成观察，不发布伪闭合范围。已完成的内层块可独立保留。

Diff 的文件区域保留 `diff --git` 标题，或连续 `---`／`+++` 的旧文件标题；
每个普通 `@@ -old[,count] +new[,count] @@` 开始 hunk，到下一个 hunk／文件或 EOF。
Git 文件的首对旧／新文件头属于该文件，后续 plain unified 文件可独立开始。
文件与 hunk 形成两层候选，不按变更行推导源语言结构。
已有扫描器以声明行数区分正文与文件标题；折叠复用分类及其确认的 hunk，
不另写补丁解析器。正文的 `---`／`+++`、单个空格的 context 行和无末尾换行标记
均保留在正确区域；仅尾部真正空行留在区域外。孤立文件头不形成文件区域。
二进制／rename 等 Git 元数据可随文件整体折叠，二进制正文保持不透明，
combined diff 不创建候选；不解码二进制、不应用补丁或验证补丁有效性。

公开 Scan 的 token、Consumed、ReplaceFrom 与不可变 State 不变；观察器由每次调用私有持有。
沿用取消、非交叉排序、同标题外层优先和既有视图状态机制。

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

### JSX、TSX、PHP 混合文档

外层语言注册专用 mixedFolding，仍使用原有 Document 任务、FoldRange 和视图状态。
观察器全部由单次 worker 所有；公开 Tokenizer State 不包含可变观察器或 GUI 对象。

JSX/TSX 在现有扫描器确认进入、完成或撤销标签候选时记录事件，保留完整名称和起始行。
配对区分大小写；Fragment 使用独立的空名称；自闭合标签不产生元素范围。
标签属性中的表达式可以再包含 JSX，未完成的外层标签与内层标签分别保存。
TSX 泛型候选回退时撤销对应标签观察，不根据所有尖括号猜测标签。
代码区域继续使用 JS/TS 扫描器，观察实际字符串／注释边界，模板插值与 JSX 切换保留
外层字面量；属性引号也可形成跨行范围。JSX 正文作为正文，不用 String Context 转换
把整段正文误判为字面量。错名、未闭合标签不发布元素范围。

PHP 由原扫描器记录实际代码片段的字节区间。PHP 结构只读取这些区间内的确认 token，
括号与控制头可跨越多次 `?>` / `<?php` 切换；HTML、script/style 中的代码括号不混入
PHP 栈。字面量／注释观察覆盖普通多行引号、插值、heredoc/nowdoc 和错误打开头恢复。

HTML 折叠使用保留 UTF-8 字节位置和物理 LF 的私有投影：屏蔽整个 PHP 片段，并在区间
起点留下不可充当标签名的占位符。PHP 字符串里的伪标签不会进入 HTML 匹配，标签名
夹有 PHP 输出时保守跳过；静态标签的属性里可以含 PHP。沿用 HTML 的大小写、void、
raw-text 与注释规则，script/style 形成元素折叠，本批不增加其内 JS/CSS 的独立块折叠。
PHP 内部 HTML profile 的自动标签编辑与折叠仍关闭，外层 PHP 定义提供这套专用规则。

PHP 替代语法识别 `if/while/for/foreach/switch/declare (...) :` 及各自结束关键字。
条件括号可以跨行并包含嵌套括号；分支不产生伪嵌套，方法／静态成员、变量、字符串、
普通括号体和不匹配结束词不会伪造模板头。结束须由分号或 PHP 关闭标记确认。
这只是词法结构分析，不执行 PHP、推导动态 HTML 输出或验证完整编译器语法。

所有候选按起点升序、终点降序归并，同起点保留外层；独立语言结构若发生行范围交叉，
保留较早起点的已确认结构，省略交叉候选，避免向组件提交非法范围。
关闭行仍有其他语句／标记时保留该行；JSX 结束后的独立分号或逗号属于原结构终止符。
取消检查覆盖分片扫描、PHP 投影屏蔽、控制词遍历与候选归并。投影和名称缓存随该次
worker 释放，不附着到共享 Tokenizer 或 State。

### Lua、Ruby、Shell、CMake、SQL

五种语言注册私有 keywordFolding，继续使用原有 Document 分析任务和 FoldRange。
复用现有词法器确认的 token、实际字面量／注释边界；只补齐被省略的普通代码片段，
不再次扫描字符串和注释中的伪关键字。观察器和控制栈由单次 worker 所有，
不修改不可变 State 或共享高亮缓存，不增加公开 API、依赖或 GOUI 扩展。

- Lua：function/end、if/then/end、for/while/do/end、独立 do/end 和 repeat/until。
  循环头的 do 不另开块；repeat 必须包含完整的 until 条件才结束，跨行括号、
  续行运算符和条件里的 function 块继续等待。长字符串／长注释匹配实际等号数；
  普通引号仅接受词法器确认的跨行延续，错误行末恢复不产生已闭合字面量。
- Ruby：class/module/def/begin/case、前置 if/unless/while/until、for 与 do 块。
  表达式位置区分前置控制和后置修饰，成员名称不当作控制词；循环的可选 do 不重复开块。
  方法头区分 endless 定义、普通默认参数、setter 和关键字方法名，不让无 end 的方法
  吞掉外层结构。引号、percent 字面量及插值代码分别观察；__END__ 数据不分析。
- Ruby/Shell heredoc：以实际扫描器创建的不可变标签身份关联声明与结束行，
  支持长标签、同一行多个声明、引用和各自的缩进规则，不能按 String Context 的切换猜测。
  Ruby 激活 heredoc 时记录挂起结构的栈深和元数据，用受保护的栈前缀隔离插值临时状态；
  终止符恢复挂起状态时一并丢弃未完成的内部字符串／控制块／括号观察，保留外层结构。
  不为每次激活复制整个栈，也不将检查点写入共享 State。
- Shell：关键字必须出现在命令位置；echo 等普通参数中的 if/fi/done 不配对。
  case 模式与扩展模式括号独立处理，支持 ;;、;&、;;& 分支结束。
  `$()` 和反引号命令替换建立独立控制栈，内部代码可以折叠；引号正文、参数展开、
  算术展开和 heredoc 正文不进入命令结构匹配。未确认 then/do 的块不发布。
- CMake：仅在词法器确认的顶层命令及其参数括号闭合后处理配对控制，命令名不区分大小写。
  覆盖 if、while、foreach、function、macro、block 及对应 end 命令；参数、变量、
  引号和 bracket 数据中的同名文本不作为命令。普通跨行命令参数仍可按括号折叠。
- SQL：只增加 CASE/END 表达式结构，不将事务 BEGIN/COMMIT 或存储过程的方言控制块
  混为通用 BEGIN/END。END IF/LOOP 等限定结束词不会关闭 CASE，END CASE 中的 CASE
  不再开块；引用标识符不作为关键字。观察实际多行引号、dollar 标签和嵌套块注释。
  dollar 结束标签必须是连续源文本：物理 LF 清除部分匹配，扫描分片之间仍保留进度。
  这一点同步修正公开 SQL 分词，避免跨 LF 拼成伪结束标签后暴露字符串内部关键字。

继续使用同起点外层优先和交叉范围过滤；未闭合结构不发布，关闭行后的其他语句保持可见。
取消检查覆盖分片、长普通代码片段、控制遍历和候选归并；并发调用的观察与恢复状态相互隔离。
这些规则是词法折叠，不承诺完整 AST、执行语义、Shell 全部方言或 SQL 存储过程语法。

### Zig、R、GraphQL、HCL、Protobuf、Starlark

六种语言复用 structuralFolding。R、GraphQL、HCL 在原扫描器的实际打开／关闭／恢复点
观察字面量和注释；Protobuf、Starlark 复用 codeTokenizer 已有的观察路径。
公开 Scan 仍传 nil 观察器，分词、不可变 State 和 GUI 高亮缓存保持原有契约。
本批范围统一排序、同起点外层优先并过滤行范围交叉，不改变前几批的候选生成策略。

- Zig 的多行字符串没有成对关闭引号。扫描器记录每行实际确认的 `\\` 起点，
  连续下一行仅在该起点前都是空白时续接；空行、其他代码或新的赋值结束前一个区域。
  EOF 可结束已确认的连续行串，只有一个物理行不产生折叠。普通引号／引用名称内的
  伪 `\\` 不成为行串，不从行首／行尾 String Context 推断它。
- R 匹配原扫描器确认的普通／raw 关闭位置，raw 括号种类和 dash 数量沿用词法器。
  相邻字符串独立观察；字符串、反引号名称、注释和 `%...%` 运算符中的括号不配对。
- GraphQL 的 block string 只在真实三引号处关闭，`\"""` 是正文转义；普通引号在
  错误行末恢复时撤销观察，不能吞并后面的查询。查询、片段、schema 和输入值按真实
  括号配对，不将字符串／注释中的伪结构当作代码。
- HCL 的引号和 heredoc 跨 `${...}`／`%{...}` 保留外层观察，内层字符串另行入栈；
  块注释及字面量的相邻开关按实际位置处理。只在扫描器确认进入并闭合 `%{...}` 后
  处理 if/for/endif/endfor；else 不再开块。忽略注释和 strip marker，空条件、
  错配结束词与未知指令不发布控制块。`$${`／`%%{` 沿用正文转义，不能伪造指令。
  每个字符串／heredoc 保存独立模板块栈，嵌套插值头保存所属模板，不让内层 endif
  关闭外层 if。该状态仅由本次 worker 所有，不附着到 hcState 或持久表达式 frame。
- Protobuf 复用实际代码括号和块注释，不根据关键字文本再次解析声明，不读取 import、
  执行 protoc、解析 option 类型或承诺编译器完整语法。相邻普通引号不是一个多行字符串。
- Starlark 复用已有缩进、括号续行和显式反斜线续行机制，块头限定为
  def/if/elif/else/for，支持紧凑条件与跨行头。Python 的 class/while/try/with 等
  保留字不成为 Starlark 块；列表推导、字典冒号和字符串内容也不形成伪块。
  只接受空格缩进，空行／注释不产生独立区域，三引号内容中的缩进不终止外层 suite。

只发布能够按词法边界确认的范围；非法代码的恢复沿用词法器，不增加通用缩进回退、
region 约定、新公开 API、依赖或 GOUI 扩展。并发、分片、取消与修订检查沿用既有契约。

### Objective-C、Objective-C++、SCSS、Less、Perl

五种语言继续复用 structuralFolding、worker 私有词法观察与统一范围归并。
Objective-C 两种 profile 使用现有 C 扫描器，SCSS/Less 使用 codeTokenizer，
Perl 在原扫描器的打开／关闭／回退路径增加私有观察。公开 Scan、不可变 State、
分词结果、Document 任务和视图状态契约不变，不增加 API、依赖或 GOUI 修改。

- Objective-C：只在词法器确认的声明深度匹配 `@interface`、`@implementation`、
  协议定义与 `@end`，包括分类和类扩展。打开头须先确认名称；`@protocol` 直接跟
  `(` 是表达式，确认名称后继续等待后续 token，逗号／分号则是前向声明。
  前向声明和表达式既不打开新区域，也不清掉已有 implementation 区域。
  方法体、消息发送、引用字面量、注释和宏中的伪指令不作为声明。
- Objective-C++ 沿用 C++ 的原始字符串和代码括号；两种 profile 均保留 C 的续行字面量、
  块注释和预处理行识别。宏定义及其物理续行不参与代码／声明配对，实际闭合的字面量
  和注释仍可折叠；不展开宏、不计算条件编译、不增加 Objective-C 的显式 region 约定。
- SCSS 的普通规则、mixin、function、控制块和映射／调用依据代码括号。
  `#{...}` 使用原扫描器的真实插值切换，外层引号／注释保留观察，内层字面量／注释
  独立入栈。转义伪插值不打开结构；URL 正文按字符串排除，URL 参数可按真实括号折叠。
  关闭行仍有正文时保持该行可见，不能为了保留一个内层区域吞掉注释后的表达式。
- Less 的嵌套规则、mixin 和 detached ruleset 依据代码括号；`@{...}` 变量插值
  沿用变量 token，不制造代码块。URL 中的 `//` 不变成注释；反引号 JavaScript
  仅作为闭合字面量折叠，不执行或进一步解析内部代码。
- Perl 的 quote-like 操作符先记录候选开头，真实分隔符确认后继续观察；若扫描器回退
  为普通名称（如 `q =>`），撤销候选。跨行操作符／分隔符保留操作符所在标题行。
  成对／非成对的 s/tr/y 均覆盖两段，不能在第一段后提前结束；正则／替换等待扫描器
  读完修饰符再提交结束位置。普通引号、q/qq/qw/qx/qr/m、转义和嵌套分隔符沿用词法器。
- Perl heredoc 以扫描器创建的不可变标签身份关联声明／终止行，支持引用、缩进和队列。
  POD 从实际打开命令到 `=cut` 关闭；`__DATA__`／`__END__` 后不做代码配对。
  名称位置的 q/s 等不是引用操作符，未完成的第二段、heredoc、POD 不发布伪闭合范围。

这一批只做词法结构折叠，不提供完整 Objective-C 编译语法、Sass/Less 求值、Perl
正则执行语义或 format 正文解析；错误恢复沿用原有扫描器，不用统一括号扫描替代它。

### Nix、jq、PromQL、Clojure、EDN

五种语言继续复用 structuralFolding、私有边界观察和范围归并。
queryTokenizer 记录实际字符串／注释关闭及插值进入／离开；functionalTokenizer
仅为 Clojure/EDN 记录字符串和丢弃形式的边界。公开 Scan 仍传 nil，token、
Consumed、ReplaceFrom、不可变 State 与 GUI 高亮缓存不变，不增加公开 API 或依赖。

- Nix 的集合、列表和代码括号沿用词法分类；普通／缩进字符串跨插值保留外层观察，
  内层字面量另行入栈，转义的伪插值不产生边界。路径内容不作为代码括号，
  块注释与相邻字符串按实际关闭位置观察，不根据 String Context 的切换猜测。
- Nix 在相同分组深度配对 let/in，仅隐藏绑定段，in 所在行及结果表达式保持可见。
  旧式 let 后直接跟集合时按集合括号折叠，不让它抢占外层 let 的 in；
  属性访问中的 let/in 不配对。不推断 if/then/else 的完整表达式终点。
- jq 的 if 须确认 then，elif 重新等待 then，else 不另开块，end 只关闭同深度的条件。
  def 须确认名称和参数后的冒号，只有同分组深度的结束分号才关闭函数；参数及调用内
  分号不提前结束外层 def。变量、成员名称、对象键、字符串和注释中的伪关键字不匹配。
- Nix/jq 的真实插值边界分别进入独立的控制结构状态，退出时恢复外层。
  内部孤立的 in/end 或未完成控制头不能关闭外层绑定／函数／条件，
  内层已确认区域仍可发布。此状态只由本次 worker 所有，不进入 queryState。
- PromQL 仅使用实际代码括号和闭合字符串，不猜测整条查询／二元表达式范围。
  反引号保留跨行及反斜线正文，单／双引号在词法器的错误行末恢复时撤销观察，
  不将字符串或标签里的括号／注释标记混入查询结构。
- Clojure/EDN 的列表、向量、映射、集合及 Clojure reader 条件按实际括号折叠；字符字面量、
  字符串及 Clojure 正则字面量中的伪括号不参与代码匹配。多行字面量独立观察。
  同行多个闭合符仍遵守既有规则：内层关闭后还有外层闭合符时，该行保持可见。
- `#_` 的打开与完整形式结束直接复用 fnDiscard 路径，支持嵌套、标签数据和 Clojure
  元数据前缀；按注释区域整体折叠。丢弃内容的括号不进入正文栈，字符串也不再产生
  独立的正文折叠；未完成的丢弃形式不发布已闭合范围。分号行注释不合成跨行注释块。

所有结构只基于词法边界，不执行 reader、宏、Nix 求值、jq 过滤或 Prometheus 查询，
不承诺完整编译器 AST／语法验证。保留原有取消、修订核验、视图隔离和关闭行尾规则，
不引入 GOUI 扩展或通用缩进回退。

### Elixir、Erlang、Julia、OCaml

四种语言复用 functionalTokenizer 的实际字面量／块注释边界，额外的控制栈只由
本次 worker 所有。Elixir／Julia 真实插值进入独立控制状态，退出时恢复外层，
孤立 end 不关闭外部块。引号／sigil／字符串宏内的伪关键字和括号不作为代码。
Elixir sigil 与 Julia 字符串宏的相邻字母修饰符按扫描片段延长关闭位置，
不改变公开扫描的 token、Consumed、ReplaceFrom 或不可变 State 结构。

- Elixir：从确认的 defmodule／def／if 等控制头记录起始行，到实际 do/end 配对；
  fn/end 独立入栈，else／rescue／catch／after 不另开区域。do 紧邻冒号才是简写，
  下一行的原子不能取消 do 块。成员和原子中的同名词不配对。
  位串按双尖括号配对，三尖括号移位运算符不参与折叠。
- Erlang：case 须确认同分组深度的 of；begin／if／receive／try／maybe 与 end 配对。
  匿名 fun 须确认参数及实际箭头，支持命名递归 fun 和多 clause；
  fun Name/Arity、fun Module:Name/Arity 及变量形式只是引用。
  binary 双尖括号可嵌套，连续关闭符按各自边界匹配；普通代码括号沿用统一范围规则。
- Julia：function／macro／module／baremodule／struct／抽象及 primitive type／
  循环／条件／let／quote／try／do 使用显式 end。
  括号或数组中已有表达式后的 for／if 视为 generator／comprehension，不额外等待 end；
  内部明确打开的 begin 等块仍独立匹配。索引深度中的 end 不关闭外层函数，
  begin 后的运算／范围后缀按位置表达式处理；实际 begin 代码体正常配对。
  这是词法上下文判断，不承诺完整 Julia AST、宏展开或任意错误代码的精确解析。
- OCaml：begin／struct／sig／object 由 end 关闭，确认 do 的 for／while 由 done 关闭，
  两种终止符不互换。quoted string 的标签及结束标记按真实连续字节匹配，
  支持标签跨扫描片段，物理 LF 取消部分结束标记匹配；注释内字符串／quoted string
  保护其中的伪注释关闭符，嵌套注释仍独立观察。

本批同时修正扫描器两处已复现的边界问题：Julia #= 块注释须优先于 # 行注释，
OCaml quoted string 的关闭标记不能跨 LF 拼接。公开接口和状态结构保持不变，
这两类输入的词法分类按语法修正；其他共享扫描器语言通过观察路径一致性回归。
不提供 Erlang 顶层函数 clause 的完整范围、OCaml 普通 let／match 表达式的整体终点，
也不为 F#、Haskell 或 Scala 3 加入通用缩进回退。所有已发布候选仍须实际闭合，
关闭行尾保留、同起点外层优先、交叉过滤与视图隔离沿用既有机制。

### F#、Haskell

两种语言通过 structuralFolding 复用代码括号、扫描器真实字面量／注释边界和候选归并。
布局规则单独维护在 folding_layout.go，不能套用 Python/Starlark 或统一缩进回退。
只在已确认的语言头及正文处建立候选；同层新声明、布局退出或 EOF 结束已具备正文的区域，
不将普通缩进、未完成头或仅有注释的正文当作块。空行、纯注释和字符串续行不作为退缩进。

- F#：let／use／and、type／module／member 等声明须确认等号；if 须确认 then，
  for／while 须确认 do，match 须确认 with，fun／模式分支须确认箭头。
  同列新声明结束前一声明；同列的模式分支保留 match／function／type 的外层范围，
  else／elif 保留 if，with／finally 保留 try。分支正文可以独立折叠。
  显式 begin／class／sig 到 end、循环到 done、let/in、;; 和代码括号分别限定边界。
  #light off 关闭缩进推断，保留显式结构；#light on 重新启用后续布局规则。
  指令行不当作正文退缩进，不求值条件编译。源码缩进 Tab 不推断布局，
  字符串和注释内的 Tab 仍作为其内容处理。
- F# 字符串包括普通引号、verbatim、三引号及插值。真实插值使用独立控制／分组栈，
  退出时恢复外层，不让内层 end 或同列正文关闭外部声明。
  多 dollar 的开／关花括号按实际 arity 观察，结束符不能跨物理 LF 拼接。
  注释内普通／verbatim／三引号字符串保护其中的伪注释关闭符；双反引号名称作为单个
  标识符分类，其中的关键字和括号不进入布局／分隔符栈。
- Haskell：where／let／do／of（以及已识别的 mdo）后，首个非注释 token 的实际列
  确定布局列表。列相同是同一列表的下一条目，列更小才退出；首条目的列不大于
  当前布局基线时按空列表处理。module 的 where 允许顶层声明从首列开始。
  Tab 按下一个 8 列边界计算，UTF-8 字节偏移和语法列数分别保留。
- 显式花括号不再建立同一列表的隐式候选，内部多行正文的缩进不关闭外层隐式列表；
  分组关闭、表达式结束的逗号和匹配的 in 收束对应隐式区域。
  显式 let 的引用身份保护外层 let，内层 in 不误结束外层绑定。
  代码括号和显式布局仍按真实分隔符折叠。
- Haskell string gap 支持两个反斜杠间的空白、LF 和空行，保留跨片段状态；
  缺少 gap 的普通字符串错误行末恢复时撤销未闭合字面量观察。
  普通／字符字面量、字符串及嵌套注释中的伪布局头不参与匹配。

本批修正原有扫描器的 F# 普通／verbatim 字符串错误行末恢复、注释内特殊字符串、
多 dollar 结束符跨 LF 拼接、双反引号名称分类，以及 Haskell string gap 跨行状态；公开接口和不可变状态
契约保持不变，fnState 仅增加私有 gap 标记。布局观察状态始终只由当前 worker 所有。
这是常见词法布局折叠，不提供完整编译器 AST／F# 语法过滤器、Haskell parse-error(t)
推导、任意 GHC 扩展、条件编译分支求值或 Haskell 普通顶层函数 clause 的整体范围。
Scala 3 缩进、OCaml 普通 let/match、Erlang 顶层 clause 仍需单独规则。

### 基础配置文件

后续扩展以基本代码编辑器为范围，采用明确的词法／文本边界，不以语言数量为目标，
不引入编译器语法树或语义分析。本批补足 Nginx、go.mod／go.work 和五种分节配置格式；
Scala 3 专用布局方案不实施，DOT／Diff 等格式按实际需要另行评估。

- Nginx 复用已有 configTokenizer 与字面量观察，只配对 ClassDelimiter 确认的 `{}`。
  普通括号、方括号不进入块栈，不因正则中的其他标点干扰外层配置块；引号、转义和变量
  中的伪花括号沿用扫描器分类。只发布实际闭合的多行引号／配置块，不解释指令和正则含义。
- Go 模块／工作区复用 modTokenizer，只配对其确认的 `()`，支持 require／use／replace
  等明确分组。引号、反引号和注释内容排除；版本区间的 `[]` 不额外建立区域。
  不校验模块路径、版本、依赖或任何指令的求值结果。
- 五种分节格式复用现有扫描器的标题分类。只有以 `[` 开始、有实际关闭 `]`、非空名称
  且尾部仅有空白／词法器确认注释的物理行，才开始新节。续行或值里的 `[name]`
  不成为标题；不根据字符串原文单独猜测标题。
- INI 使用本仓已有词法 profile，不宣称统一所有 INI 方言。GitConfig 的 quoted subsection
  由现有字符串分类保护，其中的 `]` 不关闭标题；续行按原有状态处理。
  EditorConfig 取完整标题的外层关闭括号，允许 glob 字符类及转义，不解析／匹配 glob；
  其不支持行尾注释的规则保留，不能把 `[*] #text` 当作完整标题。
- 节保留标题行，后续正文与注释折叠到下一节之前，尾部空行保持可见。
  相邻空节和单行节不产生无内容区域；首节之前的文本保持原样。重复节名分别按位置
  折叠，不按名称合并，也不处理 include、覆盖、继承或本地化键的语义。

新增状态只存在于当前折叠调用。公开 Tokenizer、不可变 State、GUI 高亮缓存、公共 API、
模块依赖与 GOUI 均不变；取消、修订核验、关闭行尾保留及两视图隔离沿用原有机制。

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

第三批核对 [React JSX markup](https://react.dev/learn/writing-markup-with-jsx)、
[TypeScript JSX](https://www.typescriptlang.org/docs/handbook/jsx.html)、
[PHP escaping HTML](https://www.php.net/manual/en/language.basic-syntax.phpmode.php)、
[PHP strings](https://www.php.net/manual/en/language.types.string.php)、
[替代控制语法](https://www.php.net/manual/en/control-structures.alternative-syntax.php) 和
[语句分隔](https://www.php.net/manual/en/language.basic-syntax.instruction-separation.php)。
沿用上文 Monaco provider 与 Scintilla 视图状态分离的取舍；新增专用语言边界，
不引入其解析器依赖，也不以单一 HTML 扫描替代混合语言识别。

第四批核对 [Lua 5.4 控制结构](https://www.lua.org/manual/5.4/manual.html#3.3.4)、
[Ruby 控制表达式](https://docs.ruby-lang.org/en/master/syntax/control_expressions_rdoc.html)
与 [方法定义](https://docs.ruby-lang.org/en/master/syntax/methods_rdoc.html)、
[Bash 条件结构](https://www.gnu.org/software/bash/manual/html_node/Conditional-Constructs.html)
与 [Bash 手册](https://www.gnu.org/software/bash/manual/bash.html)、
[CMake 语言](https://cmake.org/cmake/help/latest/manual/cmake-language.7.html)。
[PostgreSQL CASE](https://www.postgresql.org/docs/current/functions-conditional.html)、
[BEGIN](https://www.postgresql.org/docs/current/sql-begin.html) 和
[词法结构](https://www.postgresql.org/docs/current/sql-syntax-lexical.html) 用于确认表达式、
事务与连续 dollar 标签的差异；不据此声称支持所有 SQL 方言。2026-11-08 查询的动态页面
中 Ruby master 为 4.1 文档、PostgreSQL current 为 18；这里只采用上述已明确的语法边界，
不将文档版本或其他编辑器的解析能力变成本模块的完整语法承诺。
沿用 provider 与视图状态分离的设计，增加逐语言的词法控制策略，不引入解析器或运行时依赖。

第五批参照 [Zig 多行字符串](https://ziglang.org/documentation/master/#Multiline-String-Literals)、
[R quoting](https://stat.ethz.ch/R-manual/R-devel/library/base/html/Quotes.html)、
[GraphQL September 2025 字符串规则](https://spec.graphql.org/September2025/#sec-String-Value)、
[HCL Native Syntax](https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md)、
[Protobuf proto3](https://protobuf.dev/reference/protobuf/proto3-spec/) 与
[Starlark 规范](https://github.com/bazelbuild/starlark/blob/master/spec.md)。
2026-11-08 查阅的 Zig master、R-devel、HCL main 和 Starlark master 是动态来源；
本批采用上述具体词法／缩进边界，不将其全部语言能力或编译器版本视为已适配。
继续沿用前述 Monaco provider 与 Scintilla 视图状态分离的取舍，复用扫描器边界，
不引入这些框架的解析器或原生依赖。

第六批参照 Apple 的 [类定义](https://developer.apple.com/library/archive/documentation/Cocoa/Conceptual/ProgrammingWithObjectiveC/DefiningClasses/DefiningClasses.html)
与 [协议](https://developer.apple.com/library/archive/documentation/Cocoa/Conceptual/ProgrammingWithObjectiveC/WorkingwithProtocols/WorkingwithProtocols.html)、
[Sass 插值](https://sass-lang.com/documentation/interpolation/)、
[Less 变量插值与规则](https://lesscss.org/features/#variables-feature-variable-interpolation)、
[Perl 引用操作符](https://perldoc.perl.org/perlop#Quote-and-Quote-like-Operators) 和
[POD](https://perldoc.perl.org/perlpod)。Apple 页面为存档，Sass/Less/Perldoc 为动态文档；
2026-11-08 查询的 Perldoc 页标记 5.44.0。本批采用上述具体词法边界，
不将文档版本等同编译器运行验收。沿用已有 provider 与视图分离原则，
将声明／引用操作符差异留在语言策略，不扩大 GOUI 或引入外部解析器。

第七批参照 [Nix 字符串](https://nix.dev/manual/nix/2.32/language/string-literals)
与 [let 绑定](https://nix.dev/tutorials/nix-language.html#let-in)、
[jq 条件与函数](https://jqlang.org/manual/)、
[PromQL 字符串](https://prometheus.io/docs/prometheus/latest/querying/basics/#string-literals)、
[Clojure Reader](https://clojure.org/reference/reader) 和
[EDN](https://github.com/edn-format/edn)。2026-11-08 查阅的 Nix 字符串页标记 2.32.9、
jq 手册标记 1.8，其余为动态文档；这里只采用上述已明确的词法／reader 边界，
不将文档版本或其他编辑器的解析能力等同本模块的完整语言实现。
继续采用前述规则与视图分离、复用扫描器和最小公共契约的取舍，不引入其解析器依赖。

第八批参照 [Elixir Syntax reference](https://elixir.hexdocs.pm/syntax-reference.html)、
[Erlang expressions](https://www.erlang.org/doc/system/expressions.html)、
[Julia control flow](https://docs.julialang.org/en/v1/manual/control-flow/)
与 [begin/end 的索引语义](https://docs.julialang.org/en/v1/base/base/#end)、
[OCaml expressions](https://ocaml.org/manual/5.4/expr.html)、
[modules](https://ocaml.org/manual/5.4/modules.html) 和
[lexical conventions](https://ocaml.org/manual/5.4/lex.html)。
2026-11-08 查阅的 Elixir 页面标记 1.20.5、Erlang 为 OTP 29.1.2，OCaml 为 5.4，
Julia stable 为动态文档；采用上述具体块／字面量边界，不将文档版本等同编译器运行验收。
继续沿用前述 Monaco provider 与 Scintilla 视图状态分离，不引入其解析器或原生依赖。

第九批参照 [F# lexical filtering](https://fsharp.github.io/fslang-spec/lexical-filtering/)、
[lexical analysis](https://fsharp.github.io/fslang-spec/lexical-analysis/)、
[verbose syntax](https://learn.microsoft.com/en-us/dotnet/fsharp/language-reference/verbose-syntax)、
[Haskell 2010 lexical structure](https://www.haskell.org/onlinereport/haskell2010/haskellch2.html)
与 [layout syntax reference](https://www.haskell.org/onlinereport/haskell2010/haskellch10.html)。
2026-11-08 查阅：Haskell 为 2010 报告，F# 为动态规范页；仅采用上述明确边界，
不将规范全部规则或编译器版本视为已完成适配。
核对 [Lexilla F#](https://github.com/ScintillaOrg/lexilla/blob/master/lexers/LexFSharp.cxx)
和 [Haskell](https://github.com/ScintillaOrg/lexilla/blob/master/lexers/LexHaskell.cxx) 的词法／折叠
分离方式；它们自身的折叠策略不替代语言规范，本模块复用现有扫描器并分别处理布局，
不复制其解析器或引入原生依赖。provider 与视图状态分离继续沿用前述 Monaco／Scintilla 取舍。

第十批参照 [Nginx 配置结构](https://nginx.org/en/docs/beginners_guide.html#conf_structure)、
[Go 模块与工作区文件](https://go.dev/ref/mod)、
[Git 配置格式](https://git-scm.com/docs/git-config#_syntax)、
[EditorConfig 0.17.2](https://spec.editorconfig.org/)、
[systemd.syntax 256](https://www.freedesktop.org/software/systemd/man/256/systemd.syntax.html) 与
[Desktop Entry 基础格式](https://specifications.freedesktop.org/desktop-entry/latest/basic-format.html)。
2026-11-08 核对上述文本边界；版本号仅描述参照文档，不代表执行了对应工具或完整语义适配。
沿用前述 Monaco provider 与 Scintilla 视图分离方式，语言边界复用既有扫描器，
不复制外部解析器或引入新依赖。

第十一批参照 [Graphviz DOT 语言](https://graphviz.org/doc/info/lang.html) 与
[Git diff 格式](https://git-scm.com/docs/diff-format)（2026-11-08 核对）。
仅采用显式括号／字面量与普通文件／hunk 文本边界；Graphviz 的 HTML 标签语义、
Git combined patch、文件应用和完整格式校验不在范围。
provider 与视图状态分离沿用前述 Monaco／Scintilla 取舍，不复制外部词法器或解析器。

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
