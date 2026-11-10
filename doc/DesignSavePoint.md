# 保存点设计

状态：2026-11-10 复核提案，尚未实现；价值高，但不阻塞不依赖 GOUI 扩展的近期工作。
需要 GOUI textedit 的最小历史状态能力；当前只记录需求，不修改 GOUI 或建立本地兼容层。
依赖未具备时跳过此项，继续 [视图状态恢复](DesignViewState.md)。

## 问题与所有权

应用需要在编辑后显示未保存标记，在 Undo 回保存位置时清除标记。
Model.Revision 对编辑、Undo、Redo 都递增，不能用于判断历史保存状态。
Document.Model() 允许应用直接编辑或撤销，只包装 SourceView 命令不能完整跟踪。

| 所有者 | 职责 |
| --- | --- |
| GOUI textedit | 正文历史状态身份，随撤销/重做恢复，不认识文件 |
| SourceView Document | 保存基线，组合自身的输出行尾格式 |
| 应用 | 路径、编码/BOM、写入、原子替换、外部冲突及保存顺序 |

保存点表达历史身份，不是全文相等比较。删除后重新输入相同文字仍可为 modified；
Undo/Redo 回到保存历史状态才恢复 clean。不在输入时复制全文、计算全文哈希或维护第二套历史。
clean 不证明磁盘文件存在；未命名文件是否需要保存由应用独立组合。
影子建议不进入 Model，预览本身不改变 modified；正式接受才产生正文历史状态。

## GOUI 最小需求（未提供）

候选扩展只有一个只读能力，最终命名由 GOUI 设计确认：

```go
func (m *Model) StateID() uint64
```

- 仅在同一 Model 生命周期内比较。实际提交编辑分配新标识，Undo/Redo 恢复对应历史标识。
- 分配器不能随 Undo 倒退，新分支不能重用被丢弃 redo 状态的标识。
- 连续输入合并保留初始 before、更新最终 after；批量编辑一个状态变化。
  SetText 不同内容清空历史并分配新标识；已有无操作规则保持不变。
- 在 Change 通知前更新，不改变 Revision 单调语义。回调查询可能看到重入后的最新状态，
  不能把查询结果冒充某条旧 Change 的状态。
- 查询 O(1)，每历史步骤只增加常数元数据，不开放历史栈或增加正文副本。

读取 StateID 不改变历史；保存前显式调用已有 BreakUndoGroup，避免保存位置被后续输入合并吞掉。
该能力覆盖普通/原子/批量编辑与历史路径，不涉及 typography 或原生后端。
2026-11-10 核对发布依赖 GOUI v0.7.0 的 gui/textedit/model.go、batch.go，
公开 Model 仍未提供此能力。本文不把规划接口当作可用契约。
宿主可临时保守地提示“发生编辑后未保存”，成功保存后清除；该提示不能保证 Undo 回保存位置自动恢复 clean。
这是有限绕过，不作为精确保存点实现；可靠能力缺失时先跳过，继续视图状态等工作。

## Document 接口提案

```go
func (d *Document) CaptureSavePoint() SavePoint
func (d *Document) MarkSaved(point SavePoint) error
func (d *Document) Modified() bool
func (d *Document) ConnectModifiedChanged(fn func()) signal.Handle
```

SavePoint 字段私有，保存 Document 身份、历史 StateID 和 LineEnding，不持有正文或对象引用。
CaptureSavePoint 在模型所属序列先 BreakUndoGroup 再捕获；不改变保存基线、正文或 Revision。
切断输入合并是明确的 API 副作用，不把它藏在普通只读查询中。

MarkSaved 表示调用方已成功保存该点；不写文件。零值或其他 Document 的点返回错误且不改基线。
允许标记已成功写入的旧状态：保存 A 时继续编辑 B，A 完成后 MarkSaved(A)，当前仍 dirty；
Undo 回 A 才 clean。重复标记同一点正常无操作。保存失败不能调用。
应用串行化同一目标的实际替换，并按成功写入顺序发布；控件不能推断磁盘最终内容。
后台完成通过 GUI 投递再操作 Document，不在 worker 直接访问活模型。

Modified 比较当前 StateID 和 LineEnding 与保存基线。行尾按枚举值比较，不能比较变化代次：
LF 改 CRLF 再改回 LF 可恢复 clean。编码/BOM 等应用配置由应用独立组合。
构造完成后的正文和行尾作为初始基线；NewDocument 必须在检测 CRLF 后建立基线。
NewDocumentWithModel 不清除借用模型历史；两个 Document 借用同一 Model 可有独立基线。
SetText 不自动标记保存；应用完成加载后显式捕获并标记，避免把任意程序修改当作加载。

## 保存与异步快照

同步流程：捕获点 → 序列化/写入 → 成功替换目标 → MarkSaved(point)。
现有 WriteTo 支持分段输出与重入检查，不是后台快照，也不保证文件替换原子性。
失败或 ErrOperationChanged 保留旧基线。

大文本异步保存由应用在所属序列分段复制正文，并核验捕获时的 Revision/行尾代次。
任何分段前后发现变化都取消整份快照，不能拼接不同修订；完整核验后 worker 只写不可变快照。
成功后再发布对应点，后续编辑仍标记为 dirty。CaptureSavePoint 本身不是文本快照。
持续输入可能使快照反复取消；持久化文本快照属于另一个通用需求，不为保存点复制私有 piece tree。

## 信号与资源

Modified 不依赖挂载，不能只在 acquire/release 的分析订阅中维护。
每次 ConnectModifiedChanged 用已有 signal.Handles 组合模型、行尾和保存基线的信号连接，
各连接记录最后观察值；调用方断开 Handle 时一并释放，不新增永久 model → document 引用。
ui 只提供 SavePoint 别名，应用按已有生命周期连接 Document 信号，不复制控件级 modified 状态。

订阅不立即调用，调用方先读 Modified。这是当前状态通知，不是历史审计：重入或 Block 期间
可合并中间状态。回调不传过期 bool，由调用方读取最新 Modified；内部先更新观察值再调用外部。
需支持回调再次编辑、MarkSaved、Block 或断开自身，不重复报告无变化状态。

## 实施与验证

GOUI 先独立实现/验证历史身份，SourceView 再集成，分别提交。未发布时仅用不提交的 go.work
联调，记录所需提交；发布依赖未满足时不能声称 GOWORK=off 通过新增能力验证。
重点测试：

- 编辑、保存、继续输入、Undo 恰好回保存边界、Redo 离开；保存前后输入不可合并。
- Undo 后新分支、不可达保存点、SetText、批量编辑、不同 Model 相同数值标识。
- 旧快照成功/失败、行尾往返、正文与格式同时变化、初始 CRLF。
- 直接 Model 操作、无挂载视图、共享模型独立基线、信号重入/阻塞/释放。

以无桌面测试为主；独立窗口用真实输入、撤销和失败保存展示标题标记，大文件确认查询不扫全文。
执行两仓对应测试及 SourceView GOWORK=off 验证，原生未验证的平台单列。

## 参考与取舍

2026-11-10 复核公开契约，未执行参考编辑器性能对照；固定版本源码引用保留为历史设计依据：

- [Scintilla 保存点](https://www.scintilla.org/ScintillaDoc.html#SCI_SETSAVEPOINT)
  （文档 2026-10-01）按撤销位置判断修改并通知进入/离开；采用历史身份，不引入文件写入。
- [Monaco getAlternativeVersionId](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.ITextModel.html#getAlternativeVersionId)
  在线 API 承诺 undo/redo 可返回相同值；[VS Code 1.105.0 textModel.ts](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/common/model/textModel.ts#L1270)
  的 _applyUndoRedoEdits 在延迟通知前恢复 alternative version。采用状态与修订分离，不复制服务体系。
- [GTK4 TextBuffer modified](https://docs.gtk.org/gtk4/method.TextBuffer.get_modified.html)
  （2026-11-10 所查 get_modified 页面库版本 4.23.3，GtkSourceView 使用该基础）说明修改标记。
  该页面不能单独证明撤销或异步保存语义；以上规则是本项目的明确设计要求。
