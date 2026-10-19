# 应用自定义语言与编辑行为

语言描述、扫描协议与 GUI 行缓存分离。应用可以建立自己的源码编辑器：直接绑定语言，
提供配色、编辑策略和补全；无需 fork SourceView、导入 internal、注册全局名称或修改 GOUI。
本专题是扩展契约；现有语言各自的词法 profile 仍见 [高亮设计](DesignHighlighting.md)。

## 定义与绑定

```go
l, err := language.New(language.Definition{
    Info: language.Info{ID: "my-language", Name: "My language"},
    Tokenizer: myTokenizer{},
    Styles: []language.Kind{"my.heading"},
    Editing: language.EditingRules{
        AutoClosingPairs: []language.Pair{{Open: "<<", Close: ">>"}},
        Brackets: []language.Pair{{Open: "<<", Close: ">>"}},
    },
})
if err != nil { return err }
document.SetLanguageDefinition(l)
```

`Language` 不可变，New 和 Definition 返回的切片独立复制；tokenizer、函数闭包及
`Context.Data` 引用的数据也必须不可变。没有全局 Register。ID 只是元数据，同 ID 的
不同定义合法；按定义对象身份使词法结果、配对缓存和补全请求失效。
Document 共享语言及文本，视图保留自己的选区、生成配对和请求生命周期。
`SetLanguage(id)` 是内置语言便利入口；`language.Builtin(id)` 返回内置定义，
可从其 Definition 副本出发调整规则后 New。nil 定义恢复 Plain。

纯 Go 的 `language` 不依赖 GUI；六种内置扫描器也通过公开 Tokenizer 协议运行。
`internal/highlight` 保留行树、调度和缓存，`internal/editor` 保留命令计划：
它们不是给其他编辑器承诺兼容的公共引擎。应用无需重用 SourceView 的窗口状态机。

## 扫描契约

- Tokenizer.InitialState 返回非 nil 的不可变 State；State.Equal 精确比较所有影响未来
  扫描的信息，Context 返回该位置的词法语义。无状态扫描器可用 EmptyState。
- Input.Text 是不含 LF 的 UTF-8 片段，Offset 是逻辑行内 byte 起点；Limit 表示至少应
  消费的字节数。正文工作量约 16 KiB，加至多 32 byte 预读；不能读 Model 或 GUI。
  可消费预读，但只到实际 Consumed，不能因看到了未来字符就把未消费的状态当作已消费。
- 非空 Limit 必须取得进展；Consumed 在 `[Limit,len(Text)]` 且落在 rune 边界。
  LineEnd / DocumentEnd 描述 Text 的末端，只有 Consumed==len(Text) 才能执行相应转换。
  空行也会调用扫描器；片段末端不等于逻辑行末端。
- Token 的 Start/End 相对**当前逻辑行**，有序、不重叠、非空、rune 边界，
  End 不超过 Offset+Consumed；未返回区域保持普通前景。
- 延迟分类通过 Result.ReplaceFrom 显式回撤当前行 `[ReplaceFrom,旧前缀末尾)`，
  再发布新的 Tokens。位置在 `[0,Offset]` 且为 rune 边界；不用负 token 位置，
  不回改前一逻辑行。回撤可无新 token，从而清除误判。它不改变文本。
- 控件验证结果后发布；非法进展、范围或错误停止本次分析，通过现有错误信号报告，
  不发布失败结果。下一次文本修改或换定义可重新分析；正文编辑和历史仍可使用。
- 状态大小、嵌套和扫描时间由实现负责，不能在 Scan 中阻塞、联网或启动 GUI。
  高亮仍在 GUI 的预算调度中运行；本次没有把行引擎改为后台线程。

ContextAt 从可信前一行状态重扫至 caret，前缀最多 16 KiB。它不设置 LineEnd，
避免行尾注释在 caret 位置提前退出。若扫描越过 caret、前置状态未收敛或错误，返回未知。
编辑功能保守退化；提供者可以根据 ContextKnown 自行选择是否给候选。

## 样式与语义分别配置

`Kind` 是字符串样式键，内置 KindString、KindKeyword 等仍可用。自定义键可含
ASCII 字母、数字、下划线、横线和点，最长 128 byte；空键是 KindPlain，保留 `plain`
给其样式 part。Definition.Styles 声明需要解析的自定义 part：

```go
style.Name("source-view").Part("token-my.heading").ForegroundColor(myColor)
```

也可在显式 Scheme.Definition.Tokens 中使用自定义 Kind。未配置颜色回落到普通前景；
沿用 Basic/Modern、StyleScope、StyleChanged 与选区混色机制，不建立另一套主题注册。
本轮 token 仍只改变前景，不改字号、字重或原生排版度量。

颜色不能决定行为。Token.Class 的 String / Comment / Delimiter 标志描述词法角色；
结构配对只使用 ClassDelimiter，而非某个 Kind 名字。Context.String/Comment/Literal
控制插入策略与转义，Data 可带应用专用的不可变上下文。它们由扫描器保持一致，
不从着色结果反推完整语法树。换代码配色不会改变括号或补全结果。

## 编辑策略

`EditingRules.Word` 供双击、词移动/删除和补全取词共用；默认 Unicode 字母、数字、
Mark、下划线。Word 与静态 Suggest 可能被后台补全 helper 调用，必须可并发读取；
AllowPair/Newline 只在 GUI 线程运行。实际指针选择仍用原生 Cluster 几何，避免选中半个组合字符。

AutoClosingPairs、SurroundingPairs、Brackets 是三个独立列表，空列表明确关闭相应规则。
开闭序列是无换行 UTF-8，分别最长 128 byte；结构配对两端必须不同（引号可自动闭合，
不作为结构匹配）。完成多字符 opening 时优先最长规则，允许三反引号。
`NotInString/NotInComment`、AutoCloseBefore 与纯 AllowPair 回调控制自动插入；
选区包裹保留方向，字符串中可显式包裹，注释排除仍有效。
控件负责生成闭合端所有权、逐段跳过、删除、随编辑映射、IME 边界与撤销，应用不接管状态机。
结构匹配沿用两侧各 64 KiB、有类型栈与未知状态不猜测的规则。

Newline 回调收到有界 Before/After、Indent、当前视图 Unit 及语法上下文。
返回 NewlineEdit 可删除选区前后邻接字节并插入文本，Caret 相对插入文本；
DeleteBefore/After 不得越过给定片段，端点须 rune 边界，Text 最多 64 KiB、UTF-8/LF。
返回 handled=false 使用默认缩进与配置结构括号内换行。回调重入换文档/语言/修订/选区
会使操作取消。回调运行于 GUI，应保持纯函数。

Comments 配置行/块标记，供 ToggleLineComment / ToggleBlockComment 使用。
无对应标记时正常无操作；Ctrl/Cmd+/ 切换行注释。块命令对选区按字面包裹或剥除完整外层，
不分析嵌套块注释；换行和注释操作均为一个带前后选区的历史项。

## 应用编辑事务

```go
err := view.ApplyEdits(document, revision, edits, afterSelection)
```

文档对象与修订都必须匹配，避免延迟格式化结果误写同修订的另一文档。
Edits 是原始文档坐标，排序且不重叠，同位置插入应先合并；AfterSelection 使用规范化后
文本的坐标。复用 GOUI ApplyBatch，提交前校验所有范围和选区；一次修订、一个撤销项，
保留来源与历史选区。遵守禁用/只读、预编辑取消和回调重入检查。
这是应用正常命令接口，窗口交互测试仍通过 DispatchEvent/CommitText。

## 参考与取舍

2026-10-19 核对官方在线 API：

- [Monaco LanguageConfiguration](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.LanguageConfiguration.html)
  分开配置词、括号、自动包裹、注释与换行。采用这种职责区分，用 Go 函数/值表达，
  不引入 Monarch 正则解释器、注册中心、继承或语言服务容器。
- [Scintilla 5](https://www.scintilla.org/ScintillaDoc.html) 在线文档（页面标注 2026-07-22）
  说明词法扫描独立至 Lexilla。采用无 GUI 的扫描边界，不引入它的消息协议或原生依赖。
- [GtkSourceView 5 语言规则](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/lang-reference.html)
  将 string/comment 等 context class 与外观区分；本项目用 Class/Context 表达必要语义。
- Markdown 语法名词参考 [CommonMark 0.31.2](https://spec.commonmark.org/0.31.2/)，
  只是例子采用的子集基准，不据此宣称规范一致性。

这些是文档职责承诺及本项目取舍，不宣称重现参考编辑器的内部实现或性能。
