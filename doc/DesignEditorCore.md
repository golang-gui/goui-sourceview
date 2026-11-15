# 独立编辑内核

状态：独立内核已随 v0.6.0 发布；2026-11-11 的独立化是本设计基线。
后续多光标和列编辑见[多选区设计](DesignMultiSelection.md)，平台验证边界见[发布验收](PlanReleaseV060Ready.md)。
本设计更新此前复用 GOUI 文本模型与段落设施的方案。验证记录见 [验收](PlanEditorCore.md)。

## 所有权与依赖

SourceView 自有正文、行索引、修订、编辑事务与历史，选区及预编辑归各视图。
文档通过 Document 公开。内部存储模型保持私有，不建立第二个公开文本框架。

- internal/text：分段正文、行索引、顺序变更通知、原子批量修改、差量历史、文本位置、
  临时投影、已测量片段、编辑几何和高度索引。没有 Widget、窗口、计时器或原生资源。
- internal/editor：源码编辑命令、搜索、括号、缩进等计划，使用自有文本设施。
- internal/view：段落原生资源、长段落页面、视口工作集及绘制。其调度仍由 SourceView 的
  事件循环队列驱动，不将策略放入 typography。
- sourceview：Document、视图状态、输入与模型协调、语言、高亮、折叠及语义呈现。
- ui：自有文本值类型的别名、属性与信号绑定，复用 GOUI 的协调和生命周期。

GOUI 继续提供 Widget、事件循环、统一信号、输入法连接、剪贴板、滚动容器、样式、
原生 TextLayout 和 Painter。查找/替换等普通输入控件仍可使用 GOUI 控件，它们的文本值
只在边界转换，不持有正文模型。编辑内核不调用 gui/textedit 或 gui.ParagraphLayout。

GOUI 自身的控件会传递依赖其文本包；不以整个应用的传递依赖图为空作为验收目标。
目标是 SourceView 没有对其文本模型、历史、投影、几何或段落实现的调用和类型依赖。

## 迁移基线及许可证

以已发布 GOUI v0.7.0（33ce8a9e02f57364f2f18aaed1b36d4ca32ef127）的公开
[gui/textedit](https://github.com/golang-gui/goui/tree/v0.7.0/gui/textedit) 和
[gui.ParagraphLayout](https://github.com/golang-gui/goui/blob/v0.7.0/gui/paragraph_layout.go)
为迁移基线，形成在本仓独立维护的实现。来源采用 MIT，版权为 2026 xuges@qq.com，
与本仓 [LICENSE](../LICENSE) 相同，迁入文件保留来源、版权与 SPDX 声明。
分词与字素边界使用既有版本 uniseg v0.4.7（MIT、Go 1.18、Unicode 15.0.0），
从间接依赖改为直接依赖；双向上下文沿用 x/text v0.22.0（BSD-3-Clause、Go 1.18）。
两者是纯 Go、无附带字体或语言资源，不增加原生编辑器或平台后端依赖；分发保留各自许可。
Unicode 数据表及分词不替代原生 cluster 度量，版本升级仍需核验边界与资源开销。

仅迁入实际使用的模型、位置、投影、几何、片段、高度索引及段落缓存，不引入密码显示、
输入过滤或候选换行等无关设施。不改变 GOUI 仓库，不访问其私有 textEditor。

保留分段存储与行查询、历史 piece 引用、原子校验、变更通知顺序、原生 cluster 边界、
页面预算、上下文索引、缓存上限和坐标精度，先做所有权迁移，避免同时改算法与功能。
测试覆盖自有实现，不通过别名或包装调用旧实现；迁入的回归用例按其主题维护。

## 公开接口

Document 直接提供 Text、EqualText、Len、Revision、LineCount、LineRange、LineAt、
Slice、IsRuneBoundary、SetText、Replace、ReplaceAtomic、ApplyEdits、Undo、Redo、
CanUndo、CanRedo、BreakUndoGroup、ClearHistory 和 ConnectChange。

移除 Model() 和 NewDocumentWithModel，使用方共享同一个 Document；创建时 NewDocument
检测原始换行格式。SetText 规范化正文并重置历史，保留已经选择的输出换行格式。
范围是规范化 UTF-8/LF 正文中的半开字节区间。空文档保留一个逻辑行。

使用当前检出的应用需要同步迁移，移除的入口不保留依赖 GOUI 的兼容包装：

| 原入口 | 当前入口 |
| --- | --- |
| `doc.Model().Text/SetText/Slice/Revision/...` | `doc.Text/SetText/Slice/Revision/...` |
| `NewDocumentWithModel(model)` | `NewDocument(text)`；跨视图共享同一个 Document |
| `textedit.Range/Selection/Edit/Change` | `source.TextRange/TextSelection/TextEdit/TextChange` |
| `ui.TextModel`、GOUI `ui.TextSelection` 绑定 | `sourceui.Document`、`State[sourceui.TextSelection]` |
| 文档 `ApplyBatch` | `Document.ApplyEdits`；需要历史选区时用 `SourceView.ApplyEdits` |

Document.Undo/Redo 修改共享历史，不主动恢复某个视图的历史选区；需要恢复选区时调用
SourceView.Undo/Redo。Document.ConnectChange 的 Edits 切片供观察者只读，修改前自行复制。


TextRange、TextSelection、TextEdit、TextChange 由 SourceView 定义并通过 ui 提供同名别名。
内部模型和原生几何类型不公开。原有单选区、修订和事务语义在本次迁移中保持不变。
GUI 语义快照仍填写 GOUI 的公开 TextEditingInfo，边界显式复制文本值；附加属性只发布
脱离模型的普通值。外部语法语义示例仅依赖公开 SourceView 值类型。

## 行为与生命周期

模型按所属序列访问；共享 Document 的视图分别持有选区、投影和可变原生缓存。
回调可能再次修改正文，变更按修订顺序通知，历史操作结果属于该次操作的修订。
普通连续输入保留来源和合并边界，批量修改仍是一条独立历史项。只读策略由视图维护，
应用仍可直接修改文档。背景分析读取不可变结果或有限文本快照，不操作原生对象。

长段落每次仅准备有限页面，上下文索引在事件循环中按预算推进；未测量区域保留估计值，
Paint、Snapshot 和缓存查询不触发补测。原生对象只由所属视图销毁，依次释放布局与图像。
换宽度、编辑、字体/样式变化及卸载保持既有失效和过期检查，不将整个段落同步重新排版。

## 实施和验证

按自有文本设施、长段落资源、Document 与使用者迁移、集成验收分功能提交。
同步修改示例、窗口程序、声明式绑定、AI 接入说明及当前设计入口，不将旧设计记录当作新契约。

逻辑验证：随机编辑与字符串参考、批量原子性、Unicode/CRLF、行索引、历史合并与清理、
共享文档及回调重入；几何验证包括 cluster、双向文本、折行亲和性、片段隔离和高度索引。
原生验证复用已有窗口程序，普通事件和真实 XTest 输入覆盖插入、删除、替换、粘贴、
撤销重做、选择、查找、折叠、视图重建、长单行滚动及窗口宽高变化。

迁移前后同一独立 X11 会话配置、样本、软件绘制和缩放进行对照，记录输入到绘制的范围，
不把接口返回时间当作真实操作耗时。全量及 GOWORK=off 测试、针对性 race 和三平台编译
通过后交付；未运行的原生平台如实记录。本次不新增多光标、列编辑或新的 typography API。
