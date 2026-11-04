# 外部语法/语义分析接入示例与接口评估

## 目标与边界

`example/semantic` 是使用方编写的简单 Go 编辑器，以实际应用验证现有公开接口。
不导入 SourceView internal，不新增控件特例或提前设计语言服务器框架。
分析器位于示例的 `analysis` 包，使用标准库 go/parser、go/types 做真实语法、名称解析、
作用域和类型检查；“外部”指在控件之外，不意味着启动外部进程或联网。

范围是内存中的单个 Go 文件，最多 256 KiB，不加载模块、依赖或 imports，不执行代码。
当前机器验证使用 Go 1.25.8、GOUI v0.6.0。分析包含语法错误时仍利用可恢复的 AST 和
已取得的类型信息；这不是 gopls，也不承诺完整项目分析或完整编辑期错误恢复。

## 使用方流程

1. GUI 在变更回调中复制文本、Document.ID、Model.Revision，提交普通不可变 Snapshot。
2. Service 持有一个后台分析 worker 和最多一个待处理快照，150ms 防抖；Slow analysis
   切换为 1200ms，便于观察快速连续编辑。旧任务取消，不并发堆积检查工作。
3. 后台 parser/type checker 产生绝对 UTF-8 byte 范围、样式键和诊断字符串。
   使用 App.Post 回到 GUI，核对服务 generation、文档身份和修订后发布。
4. 变量按检查后的类型区分 string/numeric，类型、字段、函数、常量和未定义名称也有
   独立样式键。修改初始化表达式后，声明和引用均依据新检查结果着色。
5. CompletionProvider 在 SourceView 自己的 worker 上取同一身份/修订的完整快照，
   重新分析当前位置的副本。临时用合法占位标识符修复未完成的成员访问，借助检查后的
   接收者类型提供字段/方法，或借助作用域查找提供当前位置可见的符号。
   候选详情包含对象类型，应用过滤和排序；接受仍由 SourceView 的普通编辑事务负责。

补全快照必须匹配 DocumentID、Revision 和 DocumentLength。没有匹配快照时正常返回空，
不把 NearbyText 当全文，也不向 GUI worker 传活 Model/Document。结构体成员当前覆盖
直接字段及方法集；嵌入字段提升、多文件符号、import 补全和代码片段不在示例范围。

关闭窗口时断开变更连接并取消服务。Close 不阻塞 GUI；App.Run 返回后 Wait 回收 worker。
go/parser、go/types 单次调用不能中途取消，检查前后核对 context，输入大小和单 worker
约束限制积压；不把此例包装成具有严格时间上限的通用语言服务。

## 高亮接口的实际适配代价

现有 Language/Tokenizer 是不可变配置，Scan 输入只有逻辑行内的片段及 Offset，
没有文档修订和绝对位置。它适合增量词法分析，不是异步语义结果的提交接口。

为检验现有 API，示例将每份分析结果转换为新的不可变 Language：

- 复用内置 Go Tokenizer，状态额外记录逻辑行的绝对起点，将外部跨度适配回行内范围。
- 两组有序范围合并，语义结果仅覆盖 Kind，保留底层字符串/注释/括号的 Class 与 Context。
- 原文片段与分析快照不符后，整个后缀只使用词法颜色，等待新结果。匹配的前缀可能暂时
  保留上次语义结果，属于延迟呈现，不参与语义编辑判断。
- 接受新结果调用 SetLanguageDefinition；没有修改一个已发布 Tokenizer 的内部可变数据。
- 绝对位置进入 State.Equal，会使插入导致后续状态变化，牺牲原有词法状态快速收敛。
  这是示例适配代价，不能作为大文件或生产语言服务接入的推荐方式。

## 接口评估结论

补全侧的 DocumentID/Revision、取消、后台执行、Range/Detail 和应用拥有过滤排序的契约
足以实现本例的类型驱动补全；使用方需要自行维护完整文本或项目索引，这个边界合理。

高亮侧存在明确缺口：发布颜色只能伪装成切换语言定义。当前 document.go 的
SetLanguageDefinition 会清理折叠、配对和补全，重新分析词法；因此即便正文修订不变，
外部结果到达也会关闭已有补全弹窗。窗口验证修改结构体字段后先显示新修订的补全，
再等待延迟的颜色结果，专门复现该行为，记录为 KNOWN API GAP，
不绕过内部状态重开弹窗，不将其当作完整交互已通过。

此外，已打开的补全没有外部索引更新后的独立重查通知。当前示例每次请求重新检查，
无需此能力；项目索引独立更新时需要进一步设计，不能靠更换 Provider/Language 隐式驱动。

后续建议（未实现）：在文档侧提供带 revision 的外部着色结果入口，与词法 Language
分离，只更新颜色层，不重置补全、折叠或括号状态。需要进一步明确范围校验、替换/清空、
编辑后失效、共享文档、主题键与可见范围更新契约，再决定完整快照还是范围更新。
本例不为完成演示而提前新增 SemanticProvider、LSP 客户端或通用装饰体系。

## 参考证据

2026-11-04 核对：

- [VS Code semantic highlighting](https://code.visualstudio.com/api/language-extensions/semantic-highlight-guide)：语义高亮作为词法高亮之上的独立层，可延迟出现。采用职责划分，不移植 token 编码或注册体系。
- [Scintilla styling](https://www.scintilla.org/ScintillaDoc.html#Styling)：容器可以通过范围样式操作提交着色；参考外部分析与呈现分离，不照搬数值消息接口。
- [GtkSourceBuffer](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.Buffer.html) 与 [GtkSourceTag](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.Tag.html)：文本标签与 SourceView 呈现分离，不引入 GTK 对象体系。
- [go/parser](https://pkg.go.dev/go/parser#ParseFile)、[go/types.Info](https://pkg.go.dev/go/types#Info)：AST、Defs/Uses/Scopes 与类型检查。在线页面版本可能高于本机；示例只使用 Go 1.24 已有 API，本机以 1.25.8 验证。

## 本机验证（2026-11-04）

- `GOWORK=off go test ./...`：通过。
- `GOWORK=off go test -race ./example/semantic/analysis -count=1`：通过。覆盖声明/引用类型变化、作用域与成员候选、未知控件词法状态下排除注释/字符串、UTF-8 分片、旧结果/销毁后回调及输入上限。
- `GOWORK=off CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./example/semantic/... ./tests/window/editor/semantic`：通过。
- 同上将目标改为 `GOOS=darwin GOARCH=arm64`：通过；两平台原生窗口未执行。
- 独立 Xvnc :87 + Xfwm4 合成、软件绘制、Pango，Modern 亮色 100% 和暗色 150%：
  实际输入后声明/引用改色、成员补全更新、Enter 接受、连续编辑及退出取消均通过；
  RenderWidget 实心前景像素验证通过，实际示例窗口截图已检查。
- **补全弹窗保持不受着色影响：不满足。** 修改结构体后，新修订的成员补全先出现，
  随后延迟的高亮发布关闭弹窗。窗口程序成功复现并记录此接口缺口，不算理想行为通过。
- `go test ./...`（本地 go.work 联调）：构建失败，错误为相邻 GOUI 当前工作区
  `platform/graphics/opengl/painter.go:388:29: undefined: nanovgo.BoxShadowVarying`。
  没有修改相邻仓库或 go.work；依赖 v0.6.0 的独立验证不受此工作区问题影响。

窗口验证只使用自己的临时 X11/DBus 会话，结束回收进程；没有改变原有桌面、文件、
剪贴板或系统缩放。构建产物、日志和截图保存在仓库外临时目录。
