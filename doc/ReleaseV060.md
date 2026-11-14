# GOUI SourceView v0.6.0

2026-11-14：当前检出已适配尚未发布的本地 GOUI `538ed4f`，需工作区，无法仅凭公开
v0.7.0 独立构建。下文三端验证数字属于 11-12 冻结候选，不是新迁移代码的原生验证。
Windows Alt 的上游修复已进入本地基线，SourceView 真机重验及其他门槛见[迁移记录](PlanGOUILocalMigration.md)。

The current checkout requires unpublished local GOUI changes and a workspace. The native results below
refer to the November 12 frozen candidate; see the [migration record](PlanGOUILocalMigration.md) for current validation.

状态：**暂缓发布**；本文件为发布说明草稿，版本尚未发布。
Windows 真实 Alt 鼠标输入存在已确认的 GOUI v0.7.0 缺陷，macOS 远程 Option 手势尚未通过，
真实 IME 验收未完成。本轮只记录问题，不修 GOUI、不推送、不打标签或创建 Release。
原生验证与发布门槛见 [准备记录](PlanReleaseV060.md)。

**Release on hold.** Windows Alt mouse input is blocked by a confirmed GOUI v0.7.0 issue;
macOS Option gestures over VNC remain unresolved, and real IME acceptance is incomplete.
No GOUI changes, push, tag or Release are included in this round.

## 中文

需要 Go 1.24 或更新版本，依赖 GOUI v0.7.0。发布 Go 库与源码示例，采用 MIT 许可证，不附预编译二进制。

### 主要变化

- 新增多光标与矩形选择，支持多点输入、删除、粘贴、导航、编辑辅助和完整选区集合的撤销/重做。
  提供上下添加光标、选择下一个/全部匹配、跳过当前匹配、按行拆分，以及独立的选区撤销。
- 新增视图状态快照，恢复同一文档、同一修订的选区、滚动锚点、结构折叠与长行收起。
  恢复使用有界排版调度；修正独立视图以不同顺序准备长行页时顶部文本不能一致恢复的问题。
- 新增 Document 保存点与修改状态通知。Undo/Redo 回到保存历史位置可恢复 clean；
  支持已完成的旧保存结果，输出行尾配置也参与比较。文件读写和保存顺序仍由应用负责。
- 新增默认关闭的特殊字符显示。菱形标记零宽、双向控制及明确控制字符的原生 cluster 位置，
  悬停显示码点与名称。NUL 在原生显示串中等字节适配为空白，避免截断行内后续文字；复制和保存保留原值。
- 编辑正文、事务、历史、预编辑、几何与长行分页策略由 SourceView 独立维护，继续复用 GOUI 的
  原生 GUI、排版、绘制、输入法、剪贴板、信号和生命周期。

### 从 v0.5.0 迁移

命令式入口移入子包，模块路径不变，根包不提供兼容转发：

```go
import source "github.com/golang-gui/goui-sourceview/sourceview"
```

`ui`、`language`、`scheme` 路径不变。移除 `Document.Model()` 和 `NewDocumentWithModel`；
使用 `NewDocument(text)`，多个视图共享同一个 Document。原模型的 Text、Slice、Replace、
ApplyEdits、Undo/Redo、Revision 和 ConnectChange 等能力直接通过 Document 使用。
范围、选区、编辑和变更使用本模块的 `TextRange`、`TextSelection`、`TextEdit`、`TextChange`，
`ui` 提供同名别名。范围仍为规范化 UTF-8/LF 正文中的半开字节区间。

控件默认 Alt/Option+点击添加光标，Alt/Option+Shift+拖动列选；可集中配置 SelectionModifiers。
Linux 编辑器示例使用 Ctrl+点击避开 Xfwm4 的 Alt 窗口操作。键盘命令通过 KeyBindings 配置；
默认 Ctrl/Cmd+U 撤销选区，Ctrl/Cmd+Shift+D 跳过当前匹配。
上述为手势配置；GOUI v0.7.0 Windows 客户区鼠标事件遗漏 Alt，实际 Alt+点击和 Alt+Shift+拖动
当前均失败。macOS 远程 Option 手势也未通过；本草稿不承诺两端鼠标验收完成。

保存前 CaptureSavePoint，写入成功后 MarkSaved；Modified 不等于磁盘文件存在，重新输入相同字节也可能仍为 dirty。
示例 Mark saved 只标记内存，不写文件。特殊字符示例为第三项 unicode.jsonc，使用 View > Special characters 开关。

### 范围与验证

补全与影子建议仍限单选区；矩形按原生 cluster 和实际显示行生成，短行钳制行尾，不填充虚拟空格。
视图快照为进程内、同文档同修订的值，不提供跨修订映射或序列化承诺。特殊字符显示不展开替代标签，
合法连接字符也不表示错误诊断。代码片段、补全信息增强、诊断等保持暂缓。

当前候选在 Linux amd64、Windows amd64、macOS arm64 的原生包测试均为 2359 项通过、零失败/跳过；
mod verify、tidy -diff、vet 与 CGO=0 全仓构建通过，Linux 另通过全仓 race。
三端最终各 42 组原生窗口通过，包含 100%/150% 应用缩放和 Basic/Modern Light/Dark；
这些模拟输入结果不消除上述真实桌面输入阻塞。
最终原生窗口、真实桌面输入及未覆盖项以[验收记录](PlanReleaseV060.md)为准；不将构建或 CommitText 当作真实 IME 验收。

## English

Requires Go 1.24 or newer and GOUI v0.7.0. This MIT-licensed release distributes the Go library and source examples without prebuilt binaries.

### Changes

- Added multiple cursors and column selection, including atomic editing, clipboard distribution, navigation,
  editing assistance and complete selection-set undo/redo. Commands add cursors, select next/all occurrences,
  skip the current occurrence, split selections into lines and undo selection changes independently of text.
- Added process-local view-state snapshots for the same document and revision, covering selections, viewport
  anchors, structural folds and long-line collapse. Restoration remains incremental and preserves bounded
  page boundaries when independent views prepare long paragraphs in different orders.
- Added document save points and modified-state notifications. Undo/redo can return to the saved history
  identity; output line endings participate in that identity. Applications still own file I/O and write ordering.
- Added optional special-character markers and Unicode tooltips. Markers identify native cluster positions
  without expanding control characters into labels. NUL becomes a length-preserving blank only at the native
  layout boundary, preventing suffix truncation while retaining original bytes for copy and save.
- SourceView now owns its text storage, transactions, history, preedit, geometry and long-paragraph layout
  policy, while using GOUI's native GUI, typography, painting, input methods, clipboard, signals and lifecycle.

### Migration from v0.5.0

Import the imperative API from `github.com/golang-gui/goui-sourceview/sourceview`; the module path and
`ui`, `language`, `scheme` paths remain unchanged. The old root package has no compatibility forwarding layer.

Replace `Document.Model()` and `NewDocumentWithModel` with direct Document methods and `NewDocument(text)`.
Share a Document across views. Use SourceView `TextRange`, `TextSelection`, `TextEdit` and `TextChange`, also
aliased by `ui`; ranges remain half-open UTF-8 byte offsets in normalized LF text.

Alt/Option-click adds a cursor and Alt/Option-Shift-drag selects columns. Applications configure mouse gestures
through SelectionModifiers and keyboard commands through KeyBindings. The Linux example uses Ctrl-click to
avoid Xfwm4 window gestures. Ctrl/Cmd-U undoes selections; Ctrl/Cmd-Shift-D skips an occurrence.
These are the configured gestures. GOUI v0.7.0 omits Alt from Windows client-area pointer events,
so Alt-click and Alt-Shift-drag currently fail. macOS Option gestures over VNC also remain unverified.

Capture a save point before writing and mark it saved only after the write succeeds. Modified state tracks
history, not byte equality or file existence. The example's Mark saved button only marks memory. Select
unicode.jsonc and enable View > Special characters to try the new markers.

### Scope and validation

Completion and inline suggestions remain limited to one selection. Column selection preserves native
clusters and clamps short lines without inserting virtual spaces. View states require the same document
and revision and have no serialization contract. Special-character markers are display aids rather than
diagnostics. Snippets, richer completion information and diagnostics remain deferred.

The current candidate passed all 2359 native test cases without failures or skips on Linux amd64, Windows
amd64 and macOS arm64, along with module verification, tidy checks, vet and CGO=0 builds; Linux also passed
the full race suite. Native-window results, desktop input and remaining coverage are recorded in the
[preparation log](PlanReleaseV060.md). Builds and CommitText do not establish real IME coverage.
Each platform also passed 42 native-window groups across the specified themes and 100%/150% application
scales. Simulated input passing does not resolve the desktop-input blockers above.
