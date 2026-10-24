# 文档换行格式与末尾换行

本专题描述 `document.go` 的导入识别、输出格式和分块导出。文本编辑仍使用
GOUI v0.3.0 的 UTF-8/LF 模型；没有修改 GOUI 或原生排版接口。

## 模型与格式的边界

CRLF 是一个逻辑换行，在模型中对应一个 LF。所有 Range、Selection、补全编辑、
搜索范围与语义文本偏移继续以规范化后的 UTF-8 byte 为单位，不是原文件 byte 偏移。
删除这个 LF 删除整个逻辑换行，光标不会停在 CR 与 LF 之间。

输出格式由 Document 管理，同一文档的不同视图共享。是否显示换行标记属于 SourceView，
不同视图可以独立选择。文档不拥有路径、文件句柄、编码探测或保存对话框。

```go
type LineEnding uint8
const (
    LineEndingLF LineEnding = iota
    LineEndingCRLF
    LineEndingCR
)

func DetectLineEnding(text string) (ending LineEnding, mixed bool)
func (ending LineEnding) String() string
func (d *Document) LineEnding() LineEnding
func (d *Document) SetLineEnding(ending LineEnding) error
func (d *Document) ConnectLineEndingChanged(fn func(LineEnding)) signal.Handle
func (d *Document) WriteTo(w io.Writer) (int64, error)
```

LF 是零值；String 返回 `LF`、`CRLF` 或 `CR`，供界面标签使用，不返回编码字节。
CR 作为独立格式保留，避免旧文本载入后必然改成 LF。

## 导入与混合换行

NewDocument 在调用 textedit.NewModel 之前识别原文：

- CRLF 算一次，不重复计入独立 LF 或 CR。
- 选择出现次数最多的格式；最多次数相同时选择其中最先出现的格式。
- 没有换行时默认 LF，与操作系统无关；应用可显式设置输出格式。
- 混合格式统一按选定格式导出，不保留每个换行的原始类型。
  应用需要识别混用时，在原始文本上调用 DetectLineEnding，读取 mixed 结果。
  该结果描述本次输入，不是编辑后仍自动维护的“混合状态”。
- 模型继续替换非法 UTF-8 为 U+FFFD，CR/CRLF 归一成 LF；其他字符不因换行识别被删除。
  因此非法 UTF-8 输入也不承诺原始字节保真。

NewDocumentWithModel 接收已经规范化的模型，原格式无法恢复，默认 LF。
调用方知道原格式时显式 SetLineEnding。之后通过 Model.SetText 重新装载或插入原始文本，
不会重新探测文档格式；需要重新识别时由应用在规范化前调用 DetectLineEnding 并设置。
普通粘贴、输入和撤销继续使用当前输出格式，不被插入文本的局部换行习惯切换。

## 末尾换行

末尾换行存在于模型正文中，不另存 FinalNewline 布尔值，不隐式补齐或删除：

| 输入 | 模型 | CRLF 格式导出 |
| --- | --- | --- |
| 空文本 | 空文本 | 空文本 |
| `abc` | `abc` | `abc` |
| `abc\r\n` | `abc\n` | `abc\r\n` |
| `abc\r\n\r\n` | `abc\n\n` | `abc\r\n\r\n` |

每个 LF 后都有一个逻辑行，所以有末尾换行时保留最后一个空行。
ShowLineEndings 画的是中性的逻辑换行标记：每个真实换行只画一次，
最后空行不再添加标记，软折行也不添加。标记不会把模型 LF 宣称为原文件中的 LF 字节。

## 输出与错误

```go
doc := sourceview.NewDocument("a\r\nb\r\n")
var out bytes.Buffer
n, err := doc.WriteTo(&out)
// n == 6, err == nil, out.String() == "a\r\nb\r\n"
// doc.Model().Text() == "a\nb\n"
```

WriteTo 实现 io.WriterTo，逐块读取最多 32 KiB 的规范化文本，读取边界不拆 UTF-8 rune；
CRLF 编码后的单块最多 64 KiB。不会先调用 Model.Text 再生成完整的 CRLF 副本。
返回值是实际写出的编码后字节数；空文档返回 `(0, nil)`，不调用 writer。
短写且无底层错误返回 io.ErrShortWrite；底层错误原样返回，不重复发错误信号。

文档及模型仍要求顺序访问。writer 回调修改模型修订或输出格式代次时，
本次写入结束后返回 ErrOperationChanged，不继续读取新状态。格式先改变再改回也会检测到。
writer 已写出的前缀不能撤回，WriteTo 不提供原子文件替换；应用负责文件打开、关闭、
临时文件替换策略和错误处理。它是同步序列化，不承诺异步磁盘 I/O 或不可变快照。

Model.Text/Slice 继续返回规范化内容。需要按文档格式保存时使用 WriteTo；
不能把 Model.Text 当作自动保留原换行的保存接口。
示例状态栏显示输出格式及编码后的 UTF-8 字节数，不新增保存按钮或自动写文件。

## 格式配置与编辑历史

SetLineEnding 是输出配置 setter。非法枚举返回错误，配置不变；相同值不通知。
真正变化经 ConnectLineEndingChanged 通知，重入后过期事件不继续交给后续订阅者。
订阅由调用方持有并断开，不依赖窗口挂载，不另建永久模型监听。

配置不会修改模型文本、Revision、选区、排版、文本撤销或重做栈。
应用若把输出设置变化视为文件待保存状态，可独立监听这个信号。
这不是可撤销的“转换换行格式”编辑命令。未来提供该命令时，需先确定文档格式变化如何
与文本历史统一；不以虚假文本编辑或第二套文本撤销栈掩盖当前能力边界。
混合换行逐行保真、自动补末尾换行及文件编码管理均不属于本轮实现。

## 参考与验证

2026-10-24 核对的公共文档：

- [Monaco ITextModel.getEOL / getValue / pushEOL](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.ITextModel.html#getEOL)
  区分文档 EOL、取值和进入历史的 EOL 转换。采用格式与逻辑编辑分离的职责；
  不据此推断 Monaco 内部一律使用 LF，也不复制其存储和撤销实现。
- [Scintilla 5 Line Endings](https://scintilla.org/ScintillaDoc.html#LineEndings)
  区分换行显示、编辑模式和转换操作。SourceView 保持已有模型坐标，
  不将原始 CRLF 的两个字节混入文本索引。
- [GtkSourceView 5 FileSaver.set_newline_type](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/method.FileSaver.set_newline_type.html)
  页面显示库版本 5.19.0，说明保存器单独设置输出换行类型。
  采用输出边界的格式配置，不引入其文件管理对象体系。

包内验证三种格式、混用主格式及同数规则、无末尾换行与多个空行、非法 UTF-8、
UTF-8/CRLF 块边界、真实临时文件、短写、底层错误、回调修改文本或格式、
多视图配置隔离、跨行删除与撤销、行尾标记和 EOF。实际结果与平台缺口见 [Plan](Plan.md)。
