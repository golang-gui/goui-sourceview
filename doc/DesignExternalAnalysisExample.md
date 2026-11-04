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

## 独立高亮和补全刷新

最初的接口验证确认了两处缺口：颜色必须适配 Tokenizer 并更换语言定义，导致弹窗关闭；
外部索引更新也缺少重查当前会话的入口。现在示例使用新的公开接口，删除行状态适配器：

- Analyze 在 worker 中把类型检查产生的绝对范围交给 NewHighlights；构造和校验不占 GUI。
- 服务核对 generation；GUI 核对 DocumentID/Revision 后 SetHighlights。语言仍是内置 Go，
  外部 Kind 直接由 Style 解析，不把临时语义结果注册为语言配置。
- 示例发布后显式调用 RefreshCompletion；它只刷新存在的会话，没有会话时不弹窗。
- 编辑后未相交的旧颜色暂时保留，局部失效退回词法颜色；新分析结果完整替换。旧颜色
  只供显示，补全依旧分析请求所对应的准确快照。

本例补全使用标准库在请求时重查，外部项目索引可以在相同修订更新后显式刷新。
它不要求把整个检查结果塞进 CompletionContext，也不增加 Provider 注册或通知体系。
契约详见 [外部高亮快照](DesignExternalHighlights.md) 和 [补全设计](DesignCompletion.md)。

## 参考证据

2026-11-04 核对：

- [VS Code semantic highlighting](https://code.visualstudio.com/api/language-extensions/semantic-highlight-guide)：语义高亮作为词法高亮之上的独立层，可延迟出现。采用职责划分，不移植 token 编码或注册体系。
- [Scintilla styling](https://www.scintilla.org/ScintillaDoc.html#Styling)：容器可以通过范围样式操作提交着色；参考外部分析与呈现分离，不照搬数值消息接口。
- [GtkSourceBuffer](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.Buffer.html) 与 [GtkSourceTag](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.Tag.html)：文本标签与 SourceView 呈现分离，不引入 GTK 对象体系。
- [go/parser](https://pkg.go.dev/go/parser#ParseFile)、[go/types.Info](https://pkg.go.dev/go/types#Info)：AST、Defs/Uses/Scopes 与类型检查。在线页面版本可能高于本机；示例只使用 Go 1.24 已有 API，本机以 1.25.8 验证。

## 本机验证（2026-11-04）

- `GOWORK=off go test ./...`：通过，使用 go.mod 声明的 GOUI v0.6.0。
- `GOWORK=off go test -race . ./ui ./example/semantic/analysis`：通过。
- `GOWORK=off CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`：通过。
- 同上改为 `GOOS=darwin GOARCH=arm64`：通过；Windows/macOS 原生运行未执行。
- 独立 Xvnc :87 + Xfwm4 合成、software 绘制、Pango，Modern 亮色 100%、暗色 150%：
  以下真实输入、像素及会话断言均通过。检查了示例实际窗口截图，关闭后回收专用会话进程。
- `go test ./...`（本地 go.work）：失败。相邻 GOUI HEAD 为 `ef6bb38` 时，工作区
  圆角契约已变化，既有 completion_list.go/styles.go 使用的 `Style.Radius` 不再存在，
  `DrawBoxShadow` 的圆角参数也已变为 `geometry.CornerRadius`。这些调用不是本次新增。
  未修改相邻仓库或 go.work；这是本地新版 GOUI 的兼容性缺口，不能把独立依赖验证
  写成双仓联调通过。本次接口只依赖已发布 v0.6.0 的公开能力。

本机 Go 1.25.8、Linux amd64。构建、截图和日志均在仓库外临时目录，未改原有桌面、
剪贴板、文件或系统缩放。窗口程序运行前后不依赖 DevServer。

窗口程序通过 Window.DispatchEvent / CommitText 改变变量类型、结构体字段、接受候选，
用 RenderWidget 实心 NRGBA 像素确认声明/引用的实际颜色。延迟颜色到达后检查原 Popover
身份；随后模拟同修订的外部索引可用项变化，连续刷新，只保留新候选；等待期间 Enter
不得提交旧内容，完成后仍接受之前选中的等价 Score 编辑。另验连续编辑、过期发布、
无会话刷新不打开弹窗和关闭时取消后台任务。
