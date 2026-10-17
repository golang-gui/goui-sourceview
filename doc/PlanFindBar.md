# 编辑快捷键与可选查找栏验收（2026-10-17）

基于 SourceView `972d1e3`，联调 GOUI `6f684b652333f75c20d72aad76920528ab0d9901`。
本轮不修改 GOUI，保留其工作副本原有改动。行为见 [编辑快捷键](DesignEditing.md#13-常用编辑快捷键)
与 [FindBar 设计](DesignFindBar.md)。按编辑命令/绑定、命令式查找栏、声明式接入/示例、导航预编辑修正、
窗口验收分别提交；未推送。

已提交编辑命令/绑定 `a765025`、命令式查找栏 `081c6ec`、声明式接入与示例 `136f9d5`，
导航预编辑修正为 `7190f53`。本记录与窗口验收程序随后独立提交。
核心与命令式栏各自在只含该阶段暂存内容的独立快照中运行 `go test ./... -count=1` 通过，
未用后续接入修改掩盖中间提交的构建问题。

## 实现与回归

- 新增 InsertLine、MoveWord、DeleteWord、SelectLine、JumpToBracket，复用既有模型、计划和历史。
  接入上下插入行、缩进、整行选择、词操作、括号跳转、查找导航，以及 macOS 的 Option/Command 区分。
  修正 Shift 组合被误当作全选/剪切/复制/粘贴及 macOS Cmd+Y 重做的问题。
- FindBar 是应用组合的可选控件，不成为 SourceView 子控件。查询属于目标 SourceView，
  替换文字、打开/展开状态和计数属于栏。注册/释放目标按键与状态连接，Root/语言连接归挂载期。
- 计数按帧推进 64 KiB，查询长度超过 4096 字节时明确不计数；不保存全文件命中列表。
  文档/修订/查询失效会清零，未完成不发布假总数。Find 与计数、全部替换统一不重叠序列；
  单行查询的可见段落背景也使用同一序列。
- 薄 ui 绑定通过既有 AfterUpdate 解析同 Root 的显式目标 ID，不修改 GOUI Root 或协调器。
  Search 未指定时保留交互查询；显式空值仍可清除。两个示例改为组合 FindBar。
- 纯逻辑测试断言明确的 UTF-8/组合字符、空白、分隔符、跨行、EOF、选区及一次撤销/重做。
  计数覆盖四种块预算、跨块/跨行、Unicode folding、整词、长模式和重叠；seed 20261017
  的 300 个样本与独立 strings.Count 结果比较。模式/IM Reset 重入不得提交旧编辑。

## 已通过

- Ubuntu 22.04.5 amd64 / Go 1.25.8：`go test ./... -count=1`、`go test -race ./...`、
  `go vet ./...`、`git diff --check`。
- 独立 X11/Xtigervnc + Xfwm4、Pango 1.50.6：Software 1x/2x、OpenGL 1x。
  - `go run ./tests/window/editor/shortcuts`：上下插入行、共享视图映射、反向缩进、整行选择扩展、
    词导航/删除、Unicode Cluster、括号跳转、搜索导航、Undo、输入与命令历史隔离。
    裸编辑器的 Ctrl/Cmd+F 和未绑定 Shift+A 传递给窗口快捷键；只读修改组合键仍被消费。
    程序记录实际图像资源后端类型和 Painter.PixelScale，不只记录环境变量。
  - `go run ./tests/window/editor/find`：真实声明式树、反向选区查询种子、Enter/Shift+Enter、
    选项指针/键盘、计数、替换后推进、全部替换及单次撤销、只读禁用状态、关闭与焦点归还。
    重建不清空查询、不重复注册；目标从 a 切换到 b 后旧绑定释放；移除/重建栏不残留控制器。
    20 万字节计数尚未完成时重置文本和查询，最终只得到新文本的两个匹配。
    替换回调切换文档并指定新选区后不再跳转；打开信号回调直接销毁窗口，后续不访问失效对象。
    销毁后公共 Open 返回 ErrNotMounted，未用语义执行捷径。
  - 既有 `go run ./tests/window/experience` 在 Software 1x/2x、OpenGL 1x 通过。
    保留双击、选择颜色、菜单、补全、配对及历史断言，不把逻辑通过当作跨平台视觉验收。
- Windows 10 19045 amd64 / Go 1.24.0 / SSH Session 0；macOS 15.7.7 arm64 / Go 1.24.9：
  原生 `go test ./... -count=1` 通过，包含既有平台 typography 测试；
  shortcuts、find 两个窗口程序构建通过，未在两端运行真实桌面场景。

本机窗口运行于任务独立显示，未改变日常桌面的焦点、剪贴板或系统设置。
原生包验证在远端任务临时目录，GOUI 使用上述提交的干净归档，未写入用户远端工作副本。
暂存/快照、日志、截图与可执行文件均在仓库外临时目录。
命令式示例的查找栏在 Linux Software 1x 做过截图检查，未作像素金图或外观跨平台结论。

## 修正前后对照

- 将同一个 shortcuts 窗口程序放到 `972d1e3`，首个插入行断言
  `insert below or shared mapping` 失败；最终代码相同输入通过。
- 仅在任务快照中恢复旧的 ui.SourceView.Update 无条件 SetSearch 行为，find 程序在重建后
  `rebuild cleared state or duplicated controller` 失败；保留查询后通过。
- 将不重叠序列的独立预期放到旧搜索核心，起点 1 在 `aaaaa` 搜 `aa` 返回 `[1,3)`，
  与预期 `[2,4)` 不符；统一序列后 Find、Count 和单行装饰回归通过。
- 在同一模型/选区下加入包含 LF 的预编辑，旧几何导航把 `abcdef` 的 End 落在字节 3，
  回归 `navigation used temporary paragraph geometry {3 3}` 失败；统一取消与有效性检查后落在 6。
  使用纯模拟 typography 验证计算和 Reset 回调，不将该用例当作真实 IME 验收。

## 未通过、未执行与边界

`GOWORK=off go test ./...` 仍失败：GOUI v0.1.1 缺少 `Model.ApplyBatch` / `textedit.BatchOptions`。
这是已知依赖发布阻塞；没有伪造版本、提交 replace 或将工作区通过当作独立发布可用。

Windows/macOS 真实窗口、物理快捷键被系统/IME 截获、真实 IME、分数缩放/混合 DPI 未执行。
当前只支持单选区、字面量搜索；未实现正则、多行查询输入、查找选区范围、全局/跨文件搜索或多光标。
程序化跨行查询仍可使用核心 API；其弱背景装饰可能包含局部重叠候选，与核心序列的进一步统一
列入后续工作，不为此在每次绘制时扫描全文。相邻括号跳转不搜索任意正文的包围括号。
分块计数不代表所有查找/替换或原生排版都已异步化；本轮没有性能比较结论。
