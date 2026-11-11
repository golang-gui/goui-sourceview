# 多光标与列编辑

状态：2026-11-11 开始实现，三批分别验证和提交；完成情况见 [实施记录](PlanMultiSelection.md)。
编辑模型、历史、选区与布局均在本仓，不扩展 GOUI。

## 契约

Selections 返回副本，第一项为主选区，其余按文档顺序；集合非空，上限 4096。
SetSelections 严格校验 UTF-8 范围及数量，错误不修改旧状态；SetSelection 保留钳制并恢复单选区。
重复光标、相交范围及落在范围端点的光标合并，相邻非空范围保留；合并优先主选区方向。
每光标有独立折行亲和性与垂直目标 X。只滚动主光标，通知在完整状态发布后发出，回调重入使旧操作失效。

编辑使用同修订原坐标计划，去重/检查冲突后一次 ApplyBatch；一次修订与完整前后历史选区。
纯删除可取并集，冲突替换整次失败。整行操作按行去重，不向同一行重复操作。
外部批量接口 ApplyEditsWithSelections 保留完整后选区，旧 ApplyEdits 显式给出单一后选区。
撤销恢复执行视图的完整选区，其他视图映射自己的位置。连续多点输入合并在第三批接入。

## 输入、布局及资源

基本输入、删除、换行、导航和剪贴板作用于全部选区。复制按文档顺序 LF 连接，保留空段，全部空则无操作。
粘贴规范化后行数恰好匹配则逐行分配，否则复制完整内容；尾 LF 空段参与计数。
预编辑仅显示主光标，提交原子替换全部选区；取消无正文修改。异步输入核验整个集合代次。
单点补全与影子建议在多选区中不可用，配置不变；配对、结构和其他命令在第三批逐项适配。
原生度量逐批获取，确认的计划保存普通值，不要求所有光标页面同时驻留；共享帧预算，迟到结果不覆盖用户操作。
绘制只消费可见已有几何，主光标设置 IME 矩形。保存视图状态包含完整集合与亲和性。
语义标准 Selection 保留主选区，SourceView 属性补充完整集合，无装饰性动作。

## 列选择

矩形是选区集合的生成操作。按实际显示行/cluster 命中，支持自动换行、Tab、中文、组合字符和比例字体。
短行钳制行尾，不补空格或拆 Tab，跳过折叠隐藏行。不能用每行单逻辑范围表达的双向矩形整次拒绝。
矩形拖动/键盘扩展保留起点与 X 意图；完成后选区固定到文本，不因改宽重新选中文字。
鼠标修饰键单独配置，键盘命令接入 KeyBindings，普通点击单选区，Escape 取消临时界面后清除附加选区。

## 声明式与验证

ui 增加 Selections/BindSelections/OnSelections，复用 BindState 和局部协调，单/多选区声明与绑定互相替换。
验证逻辑、事务、历史、重入、共享文档、异步取消、Unicode、IME、折叠、缓存页数以外的分散光标；
独立 X11 正常事件路径覆盖主题/缩放、鼠标和键盘，测试结果与平台缺口单列。

## 参考与取舍

2026-11-11 已核对官方文档及固定版本源码，没有运行参考编辑器：

- [Monaco/VS Code 1.104.3 CursorCollection](https://github.com/microsoft/vscode/blob/1.104.3/src/vs/editor/common/cursor/cursorCollection.ts#L148)：主选区与重叠规范化；采用值集合，不复制对象体系。
- [同版 editStack](https://github.com/microsoft/vscode/blob/1.104.3/src/vs/editor/common/model/editStack.ts#L21)：前后完整选区；在本仓历史维护。
- [同版列选择](https://github.com/microsoft/vscode/blob/1.104.3/src/vs/editor/common/cursor/cursorColumnSelection.ts)：保留矩形意图；本项目按原生几何，不使用字符平均宽度。
- [Scintilla 多选区](https://scintilla.org/ScintillaDoc.html#MultipleSelection)（页面 2026-10-01）：矩形和虚拟空间分离；不承诺跨应用矩形格式。
- [GtkSourceView 5 View](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.View.html)（查阅 5.23.0）和 [GTK4 用户动作](https://docs.gtk.org/gtk4/method.TextBuffer.begin_user_action.html)（4.24.1）：采用一次用户动作统一分组，不依赖 GTK。
