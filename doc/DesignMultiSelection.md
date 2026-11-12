# 多光标与列编辑

状态：2026-11-11 实现三批能力，分别验证和提交；验收结果见 [实施记录](PlanMultiSelection.md)。
编辑模型、历史、选区与布局均在本仓，不扩展 GOUI。

## 契约

Selections 返回副本，第一项为主选区，其余按文档顺序；集合非空，上限 4096。
SetSelections 严格校验 UTF-8 范围及数量，错误不修改旧状态；SetSelection 保留钳制并恢复单选区。
重复光标、相交范围及落在范围端点的光标合并，相邻非空范围保留；合并优先主选区方向。
每光标有独立折行亲和性与垂直目标 X。只滚动主光标，通知在完整状态发布后发出，回调重入使旧操作失效。

编辑使用同修订原坐标计划，去重/检查冲突后一次 ApplyBatch；一次修订与完整前后历史选区。
纯删除可取并集，冲突替换整次失败。整行操作按行去重，不向同一行重复操作。
外部批量接口 ApplyEditsWithSelections 保留完整后选区，旧 ApplyEdits 显式给出单一后选区。
撤销恢复执行视图的完整选区，其他视图映射自己的位置。连续多点输入使用下述完整集合分组规则。

## 输入、布局及资源

基本输入、删除、换行、导航和剪贴板作用于全部选区。复制按文档顺序 LF 连接，保留空段，全部空则无操作。
粘贴规范化后行数恰好匹配则逐行分配，否则复制完整内容；尾 LF 空段参与计数。
预编辑仅显示主光标，提交原子替换全部选区；取消无正文修改。异步输入核验整个集合代次。
单点补全与影子建议在多选区中不可用，配置不变。配对、结构、整行操作和后台编辑作用于完整集合。
原生度量逐批获取，确认的计划保存普通值，不要求所有光标页面同时驻留；共享帧预算，迟到结果不覆盖用户操作。
绘制只消费可见已有几何，主光标设置 IME 矩形。保存视图状态包含完整集合与亲和性。
语义标准 Selection 保留主选区，SourceView 属性补充完整集合，无装饰性动作。

## 编辑辅助与历史分组

配对插入、包围、生成闭括号跳过、成对删除、语言换行和智能退格逐光标生成计划。
生成配对由视图拥有，上限 16384；不把其他视图的文字当作本视图生成的配对。
标签补齐/联动按各处上下文检查；显式光标同时编辑一对标签的两端时不再重复镜像。
重叠且不同的替换仍整批拒绝。整行移动/复制/删除先合并相交或相邻行块，保留各光标在块内的位置；
插行和缩进对同一行去重。行注释对完整涉及行集合统一判断，全部已注释才移除，否则统一添加。
词/子词导航、按行选择、括号跳转作用于各光标；扩选历史保存完整集合，缩选可恢复扩选时合并的光标。
折叠与长行收起只调整受影响光标，删除越过隐藏边界先展开，整批本次不改正文。
空白标记、折叠标记和右键选区命中使用完整集合。

大小写、重缩进、合并行、尾空白清理和重缩进粘贴复用现有单 worker 与事件循环回传，
只提交同文档/修订/集合/挂载/选项代次仍有效的结果。清理和重缩进共用一份快照模型并合并行范围，
不为每个光标重建全文模型。任一空选区触发尾空白全文件清理，与既有单点契约相同；
非空选区仅清理涉及行。重缩进粘贴先按普通粘贴分配，再按各目标语言/字面量上下文决定缩进。
所有生成文本及复制结果受 64 MiB 预算约束，批次最多 4096 个去重后的正文修改；超限不部分提交。
编辑选项变化或回调重入使待执行计划失效，不取消仅需重新排版的既有影子建议。

连续普通插入、同方向逐字删除仅在同编辑来源、相同主身份/数量、完整前后集合（含亲和性）精确衔接时合并。
每次输入仍各有一次修订和通知；历史仅保存最初前集合与最新后集合，正文片段按执行顺序保留。
替换非空选区、配对生成/跳过、换行、词删除、粘贴、IME、移动、来源切换、光标合并或分支编辑中断分组。
撤销逆序执行完整片段，重做顺序执行；任一视图发起撤销均恢复该历史项的完整集合。

## 列选择

矩形是选区集合的生成操作。按实际显示行/cluster 命中，支持自动换行、Tab、中文、组合字符和比例字体。
短行钳制行尾，不补空格或拆 Tab，跳过折叠隐藏行。不能用每行单逻辑范围表达的双向矩形整次拒绝。
矩形拖动/键盘扩展保留起点与 X 意图；完成后选区固定到文本，不因改宽重新选中文字。
鼠标修饰键通过 SelectionModifiers 单独配置（零禁用；普通和仅 Shift 手势保留）。
控件在所有平台默认 Alt+左键点击添加光标、Alt+左键拖动追加选区、Alt+Shift+左键拖动矩形选择。
桌面可能抢占 Alt 鼠标手势；此时须释放窗口管理器的对应快捷操作，或由宿主通过 SetSelectionModifiers
配置其他组合。控件不抢占系统全局手势，不修改桌面配置。键盘命令接入 KeyBindings，
AddCursor/SelectNextOccurrence/SelectAllOccurrences/SplitSelectionLines 接受挂载视图的分帧请求；
匹配按字面、区分大小写，最多 64 KiB 查询和 4096 项，超限保持原集合；全部匹配保留原主范围，排除与它重叠的候选。
拆分保持方向，以原主活动端所在的有效行为主。普通点击单选区，Escape 取消临时界面后清除附加选区。

## 多光标交互收尾

2026-11-12 增加 UndoSelection、SkipNextOccurrence 和 RemoveSelection(index)。
默认 Ctrl/Cmd+U 撤销选区操作，Ctrl/Cmd+Shift+D 跳过当前匹配；移除主选区命令无默认键，
均可通过 KeyBindings 配置。跳过用同一个有界字面扫描器替换主匹配，保留其他选区并回绕；
无候选保持完整集合，空主选区先选中当前词。只读视图允许这些选区操作。

选区撤销保存完整方向、主身份、折行亲和及垂直目标 X，独立于正文历史；每视图最多 64 步、
合计 16384 个 cursorState。一次鼠标拖动（含分帧矩形）只保留开始前集合，不记录无变化状态；
正文修订、文档切换与卸载清空历史，不映射过期历史，不恢复旧滚动坐标，恢复时显示主选区。
预编辑中不执行选区撤销，直接调用会取消待处理输入。回调修改选区或文档后保留回调的新状态。

添加选区修饰键+点击已有光标或范围时移除该项，最后一项保留；移除主项后提升文档顺序最前的一项。
点击在释放时确认；静止移动/闪烁不提前改集合，实际拖动仍走追加范围，不把拖动起点误当作移除操作。
示例 Selection 菜单提供选区撤销、跳过和移除主选区，提示从真实绑定生成。

## 声明式与验证

ui 增加 Selections/BindSelections/OnSelections，复用 BindState 和局部协调，单/多选区声明与绑定互相替换。
验证逻辑、事务、历史、重入、共享文档、异步取消、Unicode、IME、折叠、缓存页数以外的分散光标；
独立 X11 正常事件路径覆盖主题/缩放、鼠标和键盘，测试结果与平台缺口单列。

## 示例与测试入口

`example/editor` 的 Selection 菜单调用控件的公开添加光标、匹配选择和拆行方法；只保留主选区使用
SetSelection。示例不保存另一份光标集合；状态栏监听 ConnectSelections，并显示完整数量与主位置。
界面提示从当前 KeyBindings/SelectionModifiers 生成，键盘列扩展等说明放在提示和菜单按钮的 tooltip。
Linux 示例在初始化输入配置处通过 SetSelectionModifiers 将 Add 覆盖为 Ctrl，Column 保留 Alt+Shift，
使 Ctrl+点击/拖动添加选区与 Xfwm4 的 Alt 窗口操作共存；其他平台沿用控件默认鼠标手势。
这属于示例宿主的配置选择，不检测或修改窗口管理器设置，不改变控件默认契约。
`multicursor.go` 提供重复字段、对齐数字、Tab、中文与组合字符的可编辑演示，初始仍显示既有 JSON 样本。

窗口多选区验收须分别运行默认和 `-clipboard` 两条路径。各场景独立构建临时快捷键表，不依赖可选场景
清除上一场景的绑定。`go test ./...` 负责包测试和程序编译，不能替代这两条真实窗口入口的运行验收。
指针场景使用字面 Alt / Alt+Shift 输入，并独立断言默认配置，避免按被测配置构造输入而漏掉默认值错误。

## 参考与取舍

2026-11-11 已核对官方文档及固定版本源码，没有运行参考编辑器：

- [Monaco/VS Code 1.104.3 CursorCollection](https://github.com/microsoft/vscode/blob/1.104.3/src/vs/editor/common/cursor/cursorCollection.ts#L148)：主选区与重叠规范化；采用值集合，不复制对象体系。
- [同版 editStack](https://github.com/microsoft/vscode/blob/1.104.3/src/vs/editor/common/model/editStack.ts#L21)：前后完整选区；在本仓历史维护。
- [VS Code 1.104.3 cursorUndo](https://github.com/microsoft/vscode/blob/1.104.3/src/vs/editor/contrib/cursorUndo/browser/cursorUndo.ts#L62)（2026-11-12 核对源码）：文档内容变化清空选区历史，有界保存独立光标历史；采用修订隔离及容量限制，本项目恢复时揭示主选区，不保存原始滚动像素。
- [同版 multicursor](https://github.com/microsoft/vscode/blob/1.104.3/src/vs/editor/contrib/multicursor/browser/multicursor.ts)：参照跳过当前匹配而不增加数量的交互。当前绑定只支持一次组合键，不为沿用 Ctrl+K Ctrl+D 扩展通用快捷键系统，选用可覆盖的 Ctrl/Cmd+Shift+D。
- [同版列选择](https://github.com/microsoft/vscode/blob/1.104.3/src/vs/editor/common/cursor/cursorColumnSelection.ts)：保留矩形意图；本项目按原生几何，不使用字符平均宽度。
- [VS Code 多光标](https://code.visualstudio.com/docs/editing/codebasics#multiple-selections-multi-cursor)及[矩形选择](https://code.visualstudio.com/docs/editing/codebasics#column-box-selection)（动态官方文档，2026-11-11 查阅）：文档默认 Alt+点击添加光标、Shift+Alt+拖动矩形，支持配置多光标修饰键。本项目按用户要求统一采用这组鼠标默认值，保留修饰键配置；不改变已有键盘列扩展绑定。
- [Scintilla 多选区](https://scintilla.org/ScintillaDoc.html#MultipleSelection)（页面 2026-10-01）：矩形和虚拟空间分离；不承诺跨应用矩形格式。
- [Scintilla 矩形修饰键](https://scintilla.org/ScintillaDoc.html#SCI_SETRECTANGULARSELECTIONMODIFIER)（同一页面）：GTK/Qt 默认 Alt，官方说明可能与窗口管理器冲突；多选区使用 Ctrl 拖动。本项目采用可配置原则，按已确认需求使用 Alt 添加、Alt+Shift 矩形，不声称各编辑器默认完全一致。Xfwm4 下实际抢占范围见实施记录。
- [GtkSourceView 5 View](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.View.html)（查阅 5.23.0）和 [GTK4 用户动作](https://docs.gtk.org/gtk4/method.TextBuffer.begin_user_action.html)（4.24.1）：采用一次用户动作统一分组，不依赖 GTK。
