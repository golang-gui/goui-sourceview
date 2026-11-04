# 行标记与调试位置

## 职责与参考

SourceView 提供行标记、侧栏输入和执行位置呈现；应用拥有调试会话、断点条件、
持久化及调试器通信。没有 DAP 依赖，也不把普通标记自动解释为已绑定的断点。

参考核对日期：2026-11-04。
- [VS Code 调试装饰](https://github.com/microsoft/vscode/blob/4c32afe38baa636e3c29fd16c99169eaf87259b7/src/vs/workbench/contrib/debug/browser/breakpointEditorContribution.ts)：调试层将业务状态映射为编辑器侧栏装饰。
- [Scintilla Markers](https://www.scintilla.org/ScintillaDoc.html#Markers)：行标记与侧栏操作通知。采用职责划分，不照搬 32 位标记槽和删除后合并到下一行的策略。
- [GtkSourceMark，API 5](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.Mark.html)：标记跟随文本位置；采用稳定身份和文本位置分离。
- [DAP](https://github.com/microsoft/debug-adapter-protocol/blob/main/debugAdapterProtocol.json)：实际断点可能未确认或位置调整；确认结果由应用回写。

## 文档模型

Document.AddLineMark / UpdateLineMark / RemoveLineMark / LineMarks 管理标记。
LineMark 使用零基逻辑行、Kind、State、应用提供的 Label；稳定 LineMarkID 只在
所属 Document 内有意义且不复用。LineMarkInfo 是独立快照，Valid=false 时 Line=-1。
内置普通/条件断点、日志点、书签及启用/禁用/待确认/拒绝状态；显示不代表业务操作。

内部以行首 UTF-8 byte anchor 跟踪，按 Change.Edits 顺序应用。锚点处插入采用右重力；
删除半开范围覆盖锚点或 Reset 使其失效，不静默移动为相邻行的有效断点。
未覆盖锚点的编辑及撤销重做继续位置映射；失效不会通过 Undo 猜测恢复，应用显式
更新重新绑定。标记增删及状态变化不进入文本撤销历史，也不改变文本 revision。
不承诺代码整行剪切/粘贴后语义身份迁移；这是纯文本位置跟踪。

模型通知可能重入：每个标记保留自身 revision，映射不读取未来文本；通知回调
只在追上当前 revision 后发送。重入期间尚未追上的快照临时报 Valid=false。
新建或更新为当前 revision 的标记不会重复应用排队中的旧变更。

标记采用独立、懒创建的模型订阅；无窗口时仍跟踪，删除最后一个标记时释放。
不为保留标记维持高亮引擎、定时器或原生资源。连接的拥有者是 Document；借用
外部 Model 时，应用应移除不再需要的标记，释放由该订阅保留的 Document。

## 呈现和操作（本分支实施）

标记栏位于行号左侧，与行号、折叠开关独立，默认关闭。点击/右键只发出操作请求，
不移动正文选区、不启动行号拖选。折叠范围里的标记汇总显示在折叠头，事件同时
携带原始标记和可见行，应用可明确处理多个目标，不随意删除隐藏的断点。

执行行属于视图，提供设置、清除、查询及独立 RevealLine；箭头与断点分槽呈现，
淡色背景位于选择/查找背景之下。文档变化时映射执行锚点；换文档或卸载时清理。
未完成布局时先按已有高度索引显露，不为请求同步测量整篇文本。

内置命令仅发送切换请求，无默认快捷键；应用可通过 KeyBindings 绑定 F9。
声明式 ui 提供所有相关别名、配置与事件，复用命令式对象和信号生命周期。
颜色通过 Basic/Modern 官方注册的 source-view 样式部件定制；图形使用 DIP 矢量。
语义快照描述可见标记及其状态，不强制排版；名称使用 Label 或八语默认资源。

## 验证

逻辑：插入、删除覆盖、替换边界、Unicode、批量顺序、Undo/Redo、Reset、重入、
未挂载模型更新、共享文档、订阅释放及快照隔离。
窗口：正常事件路径点击/右键/F9，选区保持，行号/折叠互不干扰，折叠汇总，
执行行与断点同处一行、独立显露、Basic/Modern 亮暗及 100%/150% 缩放。
三平台编译不替代远端原生运行；本轮要求本机独立 X11，其他平台记录未运行。

2026-11-04 本机结果：`go test ./...` 与 `GOWORK=off go test ./...` 通过。
独立 Xvnc :87 / Xfwm4 合成 / Linux 软件绘制 / GOUI v0.6.0 下，
`tests/window/editor/line_marks` 在 Basic、Modern light、Modern dark × 1/1.5
缩放的六组正常事件路径验证全部通过，并检查 example/debug 两种缩放的截图。
原生 macOS、Windows 本轮未运行。输入法预编辑投影期间暂隐藏标记，提交后恢复；
不把临时预编辑行误报为文档行。执行位置在卸载期间错过模型变更后失效，避免错误重绑。
