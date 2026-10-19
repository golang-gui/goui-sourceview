# 语言、编辑规则与补全定制

使用公开的 language、Document.SetLanguageDefinition 和 CompletionProvider。
这些扩展可以按应用独立配置，使用同一语言 ID 不要求注册全局名称。
下面的路径相对所选 SourceView 模块目录；较完整的用法见同版本 example/markdown。

## 从已有语言定制

Language 不可变，Definition() 返回可编辑的切片副本；编辑后重新 language.New，
再绑定到 Document。Tokenizer、回调闭包及 Context.Data 引用的数据也必须保持不可变。
定义对象身份决定分析失效；仅保留同一个 ID 不会让两个不同定义共享结果。

下面是一个完整 Go 包，演示定制 JSON 的词边界、配对和应用自己的补全来源。
在应用初始化时调用 NewLanguage / NewProvider，保留返回对象供各次 build 使用：

```go
package editorfeature

import (
    "context"
    "strings"

    source "github.com/golang-gui/goui-sourceview"
    "github.com/golang-gui/goui-sourceview/language"
)

func NewLanguage() (*language.Language, error) {
    builtin, _ := language.Builtin(language.JSON)
    definition := builtin.Definition()
    definition.Info.ID = "app-json"
    definition.Info.Name = "Application JSON"
    definition.Editing.Word = func(r rune) bool {
        return language.IdentifierRune(r) || r == '-'
    }
    brackets := []language.Pair{
        {Open: "[", Close: "]", NotInString: true, NotInComment: true},
        {Open: "{", Close: "}", NotInString: true, NotInComment: true},
    }
    definition.Editing.AutoClosingPairs = brackets
    definition.Editing.SurroundingPairs = brackets
    definition.Editing.Brackets = brackets
    return language.New(definition)
}

func NewProvider() *source.CompletionProvider {
    return &source.CompletionProvider{
        TriggerCharacters: ":",
        Complete: func(ctx context.Context, c source.CompletionContext) ([]source.Completion, error) {
            if err := ctx.Err(); err != nil {
                return nil, err
            }
            if !c.ContextKnown || c.Lexical.String || c.Lexical.Comment {
                return nil, nil
            }
            var items []source.Completion
            for _, word := range []string{"true", "false", "null"} {
                if strings.HasPrefix(word, c.Prefix) {
                    items = append(items, source.Completion{Label: word, Text: word, Detail: "Application value"})
                }
            }
            return items, nil
        },
    }
}
```

将语言绑定到 `document.SetLanguageDefinition(definition)`；提供者绑定到
`editor.SetCompletionProvider(provider)` 或声明式 `.CompletionProvider(provider)`。
示例只保留两种结构括号的自动闭合/包裹，不自动闭合引号；JSON 的扫描器仍来自内置定义。

## 实现自己的高亮扫描器

先检查 `language.Tokenizer`、Input / Result / State 的实际公开注释：

- InitialState 返回非 nil 的不可变状态。State.Equal 要精确涵盖会影响后续扫描的信息，
  包括跨行字符串、注释和嵌套状态；后续扫描不能修改此前返回的状态对象。
- Scan 收到不含 LF 的逻辑行片段。Input.Offset 是行内字节起点，Limit 是最低消费量，
  Text 可能包含额外预读；Limit>0 必须进展，Consumed 落在 `[Limit,len(Text)]` 并保持 rune 边界。
  只有消费完 Text 才能应用 LineEnd / DocumentEnd 转换；片段末尾不一定是行尾，空行也会被调用。
- Token 范围相对整条逻辑行，需加入 Input.Offset；范围非空、有序、不重叠、不拆 UTF-8，
  不超过已消费位置。未返回的区域显示普通前景。Result.ReplaceFrom 可撤回当前行较早的分类，
  不能借此回改上一行或修改文本。
- Kind 只决定样式键；ClassString / ClassComment / ClassDelimiter 决定词法角色。
  State.Context 应与这些角色一致。结构括号匹配依赖 ClassDelimiter，换颜色不会产生括号语义。
- 扫描在控件的 GUI 工作预算内进行，保持有界、可增量，不在 Scan 中联网、等待或访问控件/模型。
  Tokenizer 配置可由多个文档共享，不能存放某个视图的可变扫描位置。

自定义样式键通过 Definition.Styles 声明，例如 `"app.heading"` 对应
`style.Name("source-view").Part("token-app.heading").ForegroundColor(...)`。
也可用 Scheme.Definition.Tokens。只改变前景，不从配色推断语义或修改文本排版指标。
键未提供颜色时使用普通前景；按应用自己的亮暗方案提供合适的自定义颜色。

## 编辑行为的边界

- Word 同时影响双击选词、词移动/删除和补全取词。
- AutoClosingPairs、SurroundingPairs、Brackets 分别决定自动插入、包裹选区和结构匹配，
  空列表明确关闭对应规则。引号可以自动闭合，但开闭相同的序列不能作为结构 Brackets。
- NotInString / NotInComment、AutoCloseBefore 和 AllowPair 调整插入条件。遇到未知词法状态，
  不要假定当前一定在代码中。编辑器负责配对所有权、跳过闭合端、删除及撤销，应用不重复维护。
- Newline 收到有界的 Before / After、Indent、Unit 和词法上下文；NewlineEdit 的删除量受
  片段范围约束，Caret 相对插入文本。handled=false 使用默认缩进。Comments 配置行/块注释命令。
  AllowPair / Newline 在 GUI 线程运行，保持纯规则，不在回调内修改文档或切换焦点。

## 补全任务与编辑坐标

Complete 在控件的后台 worker 上运行；只读传入的不可变 CompletionContext，不捕获活的
Widget、Document 或 Model。读取应用索引时使用自己的并发安全快照，并把 context 传给可取消
的 I/O。控件会丢弃过期回复，提供者仍应响应取消，避免阻塞该视图后续请求。

提供者完全拥有候选集合、过滤和顺序。空结果保持为空；需要保留默认静态/附近单词候选时，
显式调用 source.DefaultCompletions(c)，再由应用合并、过滤和排序。Word / Suggest 等可能被
默认 helper 在 worker 中读取，其闭包也必须可并发读取。

CompletionContext 只有有界片段，Before / After / NearbyText 都不是完整文档；不可用它们
假装完整语义分析或项目索引。需要全文时，在 GUI 线程先取得独立数据，交给应用自己的服务。
用 DocumentID / Revision 把结果对应到正确文档版本；在 GUI 线程通过现有 API 应用最终编辑。

Completion.Range 为 nil 时替换请求的词范围；Text 为空时插入 Label。Caret 是相对插入文本
的 UTF-8 字节偏移。AdditionalEdits 与主编辑使用同一原始文档坐标，不重叠；接受一次候选
形成一个撤销事务。LSP 的 UTF-16 范围、模板占位等需应用显式转换，接口本身提供的是字面编辑。

## 验证定制结果

扫描器应覆盖跨行、分片边界、超长行、Unicode、空行与文档末尾；不能只测短的完整字符串。
检查语言切换和相同 ID 的不同定义是否更新高亮/配对，颜色与词法角色是否独立。
补全检查取消、文档修改/切换、乱序结果、空结果、提供者替换、范围和单次撤销。
再用真实窗口确认选词、括号、缩进、补全接受和 IME；只比较截图不足以证明编辑内容正确。
