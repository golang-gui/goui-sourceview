# 编辑核心、输入与查找设计

> 状态：已实现的基础行为与后续约束。实施和验证边界见 [实施记录](Implementation.md)。
> 公共入口见 [总设计](DesignSourceView.md)，GOUI 所需提交见 [接入设计](DesignGOUIIntegration.md)。

## 1. 命令流水线

键盘、菜单、剪贴板结果与公开修改方法共用编辑命令：

1. 检查控件未销毁、文档身份、Enabled、ReadOnly，以及命令是否需要挂载。
2. 取消或按 IME 契约结束不相容的预编辑；规范化插入文本。
3. 从当前修订与选区生成不可变 EditPlan：编辑列表、来源、Before/AfterSelection、历史策略、reveal 策略。
4. 再检查捕获的修订与视图 epoch；在 GUI 线程调用 Model.Apply 或提案 ApplyBatch。
5. 模型通知更新共享行元数据，各视图映射自己的选区、取消过期操作并使相关布局失效。
6. 调用返回后，仅当对象、文档和修订仍对应这次操作时，应用计划中的最终选区和 reveal。
7. 发必要的视图状态信号；外部回调后不继续使用未重新核验的对象。

EditPlan 保持内部；应用通过 ApplyEdits(document, revision, edits, afterSelection)
提交有身份/修订防护的公开事务，规则见 [语言扩展](DesignLanguageExtensions.md)。它不另建文档或 undo 栈；真实输入仍经过
`Window.DispatchEvent -> EventController -> 编辑命令`。
公开编辑 API 是应用编程能力，不能成为自动化绕过正常输入分发的接口。
已销毁控件的修改入口返回 ErrDisabled，不能因对象仍有 Go 引用而继续修改原共享文档。

简单插入和连续删除使用现有 Apply；粘贴、IME 提交、换行等形成独立撤销单元；
不相邻修改或需自定义 redo 选区的命令使用 G1。
批次通知期间不反复将活动选区移动到每个子编辑末尾。

## 2. 选区与外部变更

选区保持 Anchor/Caret 方向，视觉 affinity 和上下移动的 preferredX 归视图。
对外的 UTF-8 选区不包含视觉状态；程序设置后清除 preferredX。
拖动、左右/Home/End、鼠标点击、历史跳转和编辑来源变化会中断普通打字合并。

外部编辑与非发起视图逐项映射端点。首版沿用 textedit.MapOffset 的端点粘性：
落在替换范围内（含端点）的旧位置移到插入末尾；前方不动，后方加长度差。
映射必须顺着 Change.Edits 的执行顺序，最终再校正 Cluster。
该策略不承担未来多光标、书签或装饰锚点的通用 stickiness。

同一视图触发 Undo/Redo 时使用 HistoryResult 的选区；其他视图只映射各自旧位置。
共享文档的历史是全局的：哪个视图触发撤销，就恢复该历史项记录的选区到该视图，
不偷偷改变原发起视图的活动选区。

Reset 使分析和投影全部失效；视图选区、查找游标与待 reveal 复位到文首。
收到修订 N 而 Model 已被重入推进到 N+1 时，仅更新 N 的元数据和端点映射，
不读取 N+1 内容来构造 N 的布局。直到观察修订追上 Model，再读取文本与度量。
修订断层则全量重建，不用旧 token 冒充有效结果。

## 3. 键盘与指针

| 操作 | 首版语义 |
| --- | --- |
| 左右 / Shift+左右 | 视觉 Cluster 导航或扩展；有选区时按视觉端点收起，不能仅比较字节大小 |
| 上下 / Shift+上下 | 跨显示行，保持 preferredX；跨段落先取得目的行可靠度量 |
| Home / End | 显示行起止；智能缩进 Home 为后续选项，首版不隐式切换 |
| Primary+Home/End | 文档首尾 |
| PageUp/PageDown | 按当前视口高度移动，并请求光标可见 |
| Backspace / Delete | 删除前/后逻辑 Cluster；段落边界删除 LF；有选区则替换选区 |
| Primary+A / C / X / V / Z | 全选、复制、剪切、粘贴、撤销；Redo 用 Primary+Shift+Z，Windows/Linux 同时支持 Control+Y |
| Tab / Shift+Tab | AcceptsTab=true 时缩进/反缩进；false 或 Escape 后一次未修改 Tab/Shift+Tab 交给 GOUI 默认焦点遍历 |
| Enter | 基础缩进；紧邻空括号中生成内外两行缩进，一次撤销，见 [括号设计](DesignPairs.md) |
| Escape | 取消预编辑或当前瞬时交互，并允许下一次 Tab/Shift+Tab 离开编辑器；不关闭应用自有查找 UI |

Primary 用 GOUI 的语义修饰键解析。Option/Alt、AltGraph 和 IME 的组合不能按
“Control+Alt 等于快捷键”推断；物理按键不生成可打印文本。
命令未处理时保持传播，处理后按 GOUI 规范停止对应事件，不能吞掉全部按键。
键盘命令实际处理时调用 KeyEvent.PreventDefault，并按需 StopPropagation；
只 StopPropagation 不能假定默认 Tab fallback 已被阻止。
输入层区分“已识别命令”和“实际发生编辑”：只读下拒绝的修改按键仍按已识别处理，
不落到无关窗口快捷键；这种预期的模式限制不发错误信号。明确放行的 Tab 例外。

单击定位；Shift+单击扩展；双击按指针所在 Cluster 的模型 Unicode word segment 选取，
三击选择逻辑整行（含 LF）。命中字符后半部时仍选择该字符所属的词。
双击后拖动以完整词扩展，反向拖动保留方向；松开不再缩为普通 caret 选区。
正文拖选复用 GOUI 手势仲裁/捕获，取消时清理状态。
点击行号选择整个逻辑行（有 LF 时包含它），末尾空行为空选区。
正文使用配置的文本指针，行号和侧栏空白使用默认箭头，可用的折叠按钮和已显示的
折叠占位标记使用手形。拖选期间保持起始区域的指针；内部查找／定位控件使用自己的指针。
悬停只读取已有折叠与段落几何，不触发原生排版；尚未绘制的占位标记不构成命中区域。
拖到视口外使用挂载期计时器推进目标与 reveal，不修改 ScrollView 私有偏移。

[GOUI focus.go](../../goui/gui/focus.go) 的默认遍历只处理未修改 Tab 和 Shift+Tab，
因此首版使用 Escape 后放行一次正常 Tab 的方案，不把 Control+Tab 的未消费等同于已导航。
其他编辑/导航输入清除这次放行状态；AcceptsTab=false 时持续放行正常 Tab。
只读模式不需要 Tab 编辑，正常 Tab 也直接交给焦点遍历。无需另建焦点链或合成第二个输入事件。

## 4. 缩进计划

- 无多行选区的 Tab：InsertSpaces=true 时推进到下一个缩进站点，插入相应空格；false 插入 U+0009。站点按前导空白的逻辑列计算。
- 多行缩进：选区覆盖的每个逻辑行首插入一个缩进单位。若非空选区的较大端点恰好在某行首，该行不计入。
- 反缩进：每行最多删除一个前导 Tab，或最多 IndentWidth 个前导空格；空行和空白不足按实际删除，不能删除内容。
- 变更前计算全部范围与最终双向选区，以 G1 一次提交。选区仍包围原选中内容，不默认收起到最后一个插入点。
- Enter 复制当前行光标前的前导空白；空括号缩进与自动闭合见 [括号设计](DesignPairs.md)，不做完整语法树格式化。
- 空白前缀中 Tab 的逻辑列推进按 TabWidth；前导之外按原生几何决定可视位置，不用 rune 数乘空格宽度。
- 重排/格式化、自动转换已有 Tab、整个文件统一缩进不由配置 setter 隐式触发。

例：`a\nb` 从 0 正向选择到 2，仅缩进第一行；
从 0 选择到 3 缩进两行。反向选择覆盖相同行，命令完成后方向仍为反向。

## 5. IME 状态机

视图状态分为 Idle、Composing、Disposed；模型正式内容与预编辑投影分开。

| 事件 | 处理 |
| --- | --- |
| 首次非空 preedit | 捕获文档/修订/选区，创建 Projection，建立独立显示布局 |
| preedit 更新 | Normalize 后更新投影及其 caret；仅失效受影响显示段落，不提交历史 |
| 空 preedit | 清除投影；空更新本身不等于需要再次插入文本 |
| commit | 以捕获的替换范围提交一次原子编辑；清投影与旧 caret affinity |
| 外部模型变化、切文档、只读、失焦、卸载 | 先标记组合代次失效，再 Reset；重入或迟到回调不得提交旧范围 |
| 取消后重入回调 | 按新的组合代次重新判定，不继续访问旧 Projection |

提交事件的 Composed 信息用于划分历史，不从字符内容猜是否中文或组合输入。
Reset 期间可能同步触发回调，故必须先改变本地状态。
真正的取消与焦点重置以 GOUI 契约核对，不假设 Reset 后原生一定补发 commit。
本地组合 epoch 只能保护本地工作，不意味着原生回调携带了来源代次。
切文档后是否可能收到旧 commit，必须在 M0 原型的各平台 Reset/焦点场景核验；
若存在不可区分的迟到提交，应在 GOUI 输入契约解决，不能靠丢弃下一次正常文本来掩盖。

投影文本只在该视图排版；未提交区域首版用普通文本样式及组合下划线。
投影受影响的段落不写共享 token 缓存，后续正式段落按模型坐标映射。
候选框使用实际投影 caret，转换到控件局部 DIP；不能报告旧模型 caret。
Snapshot 的 VisibleRange/VisibleText 始终对应已提交模型，preedit 单独描述。

## 6. 剪贴板与异步粘贴

Copy 无选区为无操作，不替换剪贴板；Cut 在只读时报错，写入成功并再次核验
文档/修订/选区后才删除。G3 未具备时不能把复制失败后的删除算作正确降级。

Paste 发起前保存 `mountEpoch、document、modelRevision、selectionEpoch、requestID`，
先登记 pending 再调用 RequestText，以兼容同步回调。
任一身份变化、卸载、禁用、改为只读、新的粘贴请求或明确取消均使旧请求失效；
回调到达后安静丢弃过期结果，不误插到新文档。
失焦取消由编辑器输入发起的待粘贴；应用显式调用仍遵守上述身份检查。

剪贴板没有文本或文本为空均为 ("", nil)，Paste 无操作，保留选区、文本和历史；非 nil 错误发一次错误信号。显式 ReplaceSelection("") 仍用于删除选区。
插入 Normalize 后文本，使用一个原子历史项。不得在回调中把当前新选区当作原请求选区。
回调可销毁控件，同一请求完成后不再访问未核验的状态。

## 7. 字面量查找替换

首版不做正则。SearchQuery.Text 按模型规则规范化；空串关闭查询。
匹配区分大小写为默认；IgnoreCase 使用 Unicode simple folding，
不做 NFC 等价化，也不把多 rune 展开的完全 case folding 混进字节映射。
WholeWord 边界定义为 Unicode Letter/Mark/Number 或下划线与非该集合之间。

搜索以 UTF-8 流进行，匹配保留原文 byte 范围；分块边界保留模式匹配状态，
不能漏掉跨行或跨读取块的命中。按 rune 扫描并使用线性匹配算法，不能逐位置重读全文。
FindNext 从当前命中的源范围之外开始；尚无当前命中时以选区边界为起点，按方向环绕一次，
匹配为从文首确定的不重叠序列，与计数及全部替换一致；不同起点不改变该序列。
不反复命中刚替换的同一项。源匹配范围与视觉选择分开：匹配可能只覆盖一个 Cluster 内的
部分 rune，展示选区需要向 Cluster 扩展，但不能因此扩大实际替换的文本范围。
视图保存 currentMatch 的原始 Range、修订、查询代次及对应 selectionEpoch；
后续用户移动选区或修改查询即取消该关联。ReplaceCurrent 优先核验这个关联，
没有关联时只接受选区文本恰好匹配查询的情况，不能随意替换整块视觉选区。

- 搜索装饰缓存是视图状态，键为文档修订、查询与选项；编辑后立即失效。
- 完整搜索可能 O(N)；首版面向验收规模同步执行显式 Find/Replace，逐阶段测时，不声称拥有可中断的原生/模型原子调用。
- 屏幕装饰只计算可见逻辑段落及查询长度上下文，不为每次键入建立全文件命中数组。
  单行查询从该逻辑行首确定不重叠匹配；程序化跨行查询的弱背景装饰仍显示局部字面量候选，
  可能包含重叠范围，当前项、计数和替换以核心的不重叠序列为准。这是多行界面的后续统一项。
- ReplaceCurrent 核验仍有效的源匹配范围并替换为字面量文本；显示中的 Cluster 扩展不能吞并未匹配字符。
  API 不自动导航；内置查找栏在检查回调未使操作过期后定位下一项，见 [配套查找栏](DesignFindBar.md)。
- ReplaceAll 先在一个修订上扫描所有不重叠命中、规范化替换并生成 G1 批次；一次校验后提交，一个撤销项。
- 预计超过 100,000 个编辑项或累计新增文本超过 32 MiB 时，在修改前返回操作预算错误；这是首版保护阈值，必须在 UI/错误说明中可识别。
- ReplaceAll 不对替换产物继续搜索；零长度模式无操作，不可能无限循环。

查找替换界面属于 SourceView 的[内部配套子树](DesignFindBar.md)，无需应用组合。
SourceView 默认处理打开查找、替换和定位的快捷键；解除相应绑定后交由应用处理。核心下一项/上一项导航可以独立使用。

## 8. 挂载、重挂载与信号顺序

挂载先注册 Document 使用者并同步修订，再连接输入/根帧并解析样式。
Document 的一次模型订阅先维护索引与失效，再让已挂载视图接收内部变更；
内部状态更新之后才允许发布该视图选区信号。内部登记的遍历必须容忍视图在回调中卸载。

卸载先增加 mountEpoch，取消 pending/preedit/拖选，再断开 frame/计时器与文档登记，
释放 native layouts。即使对象仍有 Go 引用也不保留模型到该控件的挂载期闭包。
Document 最后一个使用者离开时断开模型订阅并丢弃分析任务；重新激活时按模型修订建索引。

不保证多个视图的选区信号与模型信号构成跨视图原子通知。重入时 Selection getter
只返回对当前模型做 rune/长度钳制的副本，不回写尚在映射中的旧选区；正常追平后返回最终选区。
在观察修订滞后时调用 SetSelection，记录为针对调用时模型修订的待应用意图，
不让新坐标再次穿过旧 Edit；该修订之后的变更仍继续映射它。
Snapshot 在追平前暂不发布 TextEditing/Text 内容，只标记同步待完成及两种修订号；
度量与绘制也推迟，不将旧坐标与新文本拼成貌似一致的结果。

## 9. 声明式绑定

计划形式为 `ui.SourceView().Document(doc).ReadOnly(false)`，再由
GOUI `ui.ScrollView(child)` 组合。Document 在 Build 外创建；首版不提供每次 Build
重装全文的 Text/BindText 便利 API，避免隐式清空历史。

ViewBase 的 Self、WidgetView 的 Mount/Update/Unmount 遵循 GOUI 公共扩展。
配置显式字段与默认值分别保存，未声明的配置恢复本节点初始值；
未声明 Selection 保留用户交互，显式 Selection 在 Document 更新后应用。
Document 的语言由调用方在 Document 上设置，避免两个声明式视图竞争同一语言配置。

OnSelection/OnError 在节点状态保存最新回调，只连接一次底层信号；
OnChange 对当前 Document.Model 管理 handle，切 Document 后断开旧模型。
声明式协调不得因为自身 Update 设置选区再调用用户 OnSelection；事件写回按协调保护区分。
Unmount 释放声明式连接，不夺走 Window 对 Widget 的最终销毁责任。

## 10. 编辑菜单

使用 GOUI ContextMenuEventController 和 PopoverMenu，经正常事件分发处理右键、
macOS Control+单击、Shift+F10 和 Menu 键。指针位于实际绘制选区内时保留选区，
其他位置收起到命中 caret；键盘唤出定位在 caret 下方。
提供撤销、重做、剪切、复制、粘贴、删除、行移动/复制/删除、全选；依据挂载、Enabled、ReadOnly、
历史、选区和文档边界更新可用状态。全选保持最后一项，行操作单独分组。
菜单显示不探测或读取剪贴板。
菜单动作重新核验挂载代次和文档，再调用与键盘相同的编辑入口。
语言资源使用本模块的 mlang 文本；卸载断开语言监听，原生菜单由锚点生命周期释放。
八语言文案、匹配回退与状态保留见 [界面语言](DesignLocalization.md)。
菜单打开时保持活动选区配色，关闭后重绘。
上下键选择可用项，Home/End 到首尾，Enter/Space 执行动作；键盘导航由 GOUI 通用菜单实现。

## 11. 源码交互扩展

自动配对、包裹、成对删除、括号缩进和匹配见 [括号设计](DesignPairs.md)。
语言/文档词候选、提供者、弹窗、输入优先级及失效见 [补全设计](DesignCompletion.md)。
瞬时交互优先消费自己的确认、导航和取消按键，再进入正文命令；不产生平行撤销栈。

## 12. 逻辑行编辑

`MoveLines(down bool)`、`DuplicateLines(down bool)`、`DeleteLines()` 是根包的公开命令。
前两者的 down=true 表示向下，false 表示向上；复制行不读写剪贴板。
算法位于 `internal/editor/lines.go`，只生成已有 Plan，控件经 ApplyBatch 提交，
不引入新的文本模型、撤销栈或原生资源。未挂载对象也可使用这些纯编辑方法；
ReadOnly、禁用与销毁遵守共用编辑权限，正常输入中的模式限制不发错误信号。

### 范围与选区

- 无选区时操作 caret 所在逻辑行；折行不拆成多个待操作的行。
- 非空选区覆盖所有相交逻辑行。末端恰好在后续行行首时排除该行，与缩进命令一致。
- 移动保留文本、缩进及 LF 数量；选择的字节端点随被移动行保持相对位置和方向。
  选区曾包含块后 LF，而块移动到无尾随 LF 的 EOF 时，末端钳制到新 EOF。
- 复制完整行块，创建必要的 LF 分隔符；向上复制后选择位于上方副本，向下则位于下方副本。
  保留原来的局部选区及 Anchor/Caret 方向，不强迫扩展为整行选区。
- 删除后收起选区到删除边界。存在后续行时删除块及其后 LF；删除到 EOF 时改为包含前方 LF，
  caret 留在前一行末，避免额外空行。整份文档删除后保留模型的一个空逻辑行。
- 文首上移、文末下移，以及空文档删除是正常无操作，返回 nil，不增加修订或撤销项。
  空行之间移动仍须移动 caret；相同文本不代表没有选区变化。

### 提交与失效

移动通过删除/插入相邻行实现，被移动块内部的原文字节保持在模型中，
其他视图中的内部位置由既有 Change.Edits 映射；边界仍遵守模型端点粘性。
单个空行下移时只将其 LF 搬到相邻行之后，避免批次中出现同起点插入与删除。
复制为一个插入，删除为一个范围删除。编辑坐标都属于原文，排序、非重叠及通知顺序
遵守 ApplyBatch 的公开契约，不以多次 Apply 拼出可被观察到的中间状态。

每次实际操作增加一次修订、一个撤销项，保存明确的 Before/AfterSelection，
并与前后的连续打字分隔；Undo/Redo 不重新运行行编辑算法。
IM Reset、模型通知和选区回调仍可能重入，沿用 applyPlan 的身份检查，
回调切文档或指定新选区后，不覆盖它们。

行范围校验不复制选区正文。移动仅物化需要插入的邻行，复制仅物化行块，
单次累计插入文本（含新 LF）不得超过 32 MiB；超限在读取/修改前返回 ErrOperationLimit。
空行下移只插入一个 LF，不因邻行很长而读取它；删除不为计算计划物化被删除全文。
该预算限制输入分配，不是对原生排版或原子模型调用的耗时承诺。

### 参考与取舍

2026-10-16 核对以下官方材料；UTF-8 端点和预算为本项目的选择，没有复制实现：

- [Scintilla 5 行命令](https://www.scintilla.org/ScintillaDoc.html#SCI_MOVESELECTEDLINESUP)
  提供移动、复制、删除行；移动会扩展为整行选区。采用独立行命令，保留本项目原有局部选区。
- [VS Code 1.105.0 MoveLinesCommand](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/linesOperations/browser/moveLinesCommand.ts#L39)
  和 [CopyLinesCommand](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/linesOperations/browser/copyLinesCommand.ts#L28)
  处理末端位于下一行首的范围，并追踪选区。采用排除下一行、保留方向及邻行编辑的思路；
  本轮移动保留原始缩进，不接入它的语言自动缩进策略、UTF-16 坐标或多选区对象体系。
- [GtkSourceView 5.22.1 move-lines](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/signal.View.move-lines.html)
  定义当前行/选中行向上或下移动；采用 down 布尔方向，输入仍由 GOUI 正常分发。

### 快捷键与菜单

| 命令 | Windows | Linux | macOS |
| --- | --- | --- | --- |
| 上移/下移行 | Alt+↑/↓ | Alt+↑/↓ | Option+↑/↓ |
| 向上/下复制行 | Alt+Shift+↑/↓ | Control+Alt+Shift+↑/↓ | Option+Shift+↑/↓ |
| 删除整行 | Control+Shift+K | Control+Shift+K | Command+Shift+K |

按 [VS Code 1.105.0 行命令绑定](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/linesOperations/browser/linesOperations.ts#L71)
区分 Linux 的复制组合键，Alt 与 Primary 使用 GOUI 的语义修饰键解析；只匹配精确修饰键，
额外 AltGraph 不当作行命令。组合键能否从系统或 IME 到达应用由所在桌面决定，
正常 DispatchEvent 验证不证明物理按键未被系统截获；也可使用右键菜单或应用自定入口。

补全等瞬时交互先处理或关闭自身，再进入行命令。识别到的行快捷键即使因只读或边界
无操作，也阻止默认行为及继续传播，避免触发窗口同键动作；禁用控件遵循 GOUI 焦点和输入规则。
菜单与键盘共用可用状态和命令执行入口，文首上移、文末下移及空文档删除置灰，
空文档复制仍可创建一个新空行。语义快照复用普通菜单项的角色、Enabled、边界和动作，
不新增直接执行编辑操作的自动化动作。ui 不增加命令状态副本。

## 13. 常用编辑快捷键

下列按键表为默认配置；每视图可通过 KeyBindings 修改、删除或全部清空，
具体契约见 [命令快捷键](DesignKeyBindings.md)。方法、菜单和普通文字输入独立于该配置。

本节补充单选区编辑命令。Primary 在 Windows/Linux 为 Control，在 macOS 为 Command。
沿用真实事件路径、独立修改计划和模型历史，不建立字符串命令注册器或另一套快捷键系统。
所有几何导航在统一入口先取消预编辑，再验证文档、修订和选区代次；不能用临时段落几何
计算正式文本的行首尾，也不能在 Reset 回调切换文档后继续对新文档执行旧按键。

| 操作 | Windows / Linux | macOS |
| --- | --- | --- |
| 下方/上方插入行 | Ctrl+Enter / Ctrl+Shift+Enter | Cmd+Enter / Cmd+Shift+Enter |
| 整行缩进/反缩进 | Ctrl+] / Ctrl+[ | Cmd+] / Cmd+[ |
| 选择整行、重复向下扩展 | Ctrl+L | Cmd+L |
| 词移动/扩选 | Ctrl+左右，Shift 扩选 | Option+左右，Shift 扩选 |
| 删除前词/后词 | Ctrl+Backspace / Ctrl+Delete | Option+Backspace / Option+Delete |
| 匹配括号跳转 | Ctrl+Shift+Backslash | Cmd+Shift+Backslash |
| 下一项/上一项查找 | F3 / Shift+F3 | Cmd+G / Cmd+Shift+G，兼容 F3 / Shift+F3 |
| 行首尾/扩选 | Home/End，Shift 扩选 | 同左，并接受 Cmd+左右及其 Shift 组合 |
| 文档首尾/扩选 | Ctrl+Home/End，Shift 扩选 | Cmd+上下，Shift 扩选；保留 Cmd+Home/End |

`InsertLine(below bool)` 保留当前文字，在 caret 所在逻辑行上方或下方创建新行，复制该行的
前导空格/Tab，不把原行从 caret 拆开，也不删除多行选区；完成后在新行缩进末尾收起选区。
空文档、无尾随 LF 和末尾空行都有确定位置。一次操作一个修订/历史项，与前后输入分隔。
缩进前缀分块读取，含 LF 的新增文本最多 32 MiB，超限返回 ErrOperationLimit 且不修改模型。
这里采用基础缩进，不承诺 Monaco 的语言自动缩进。

`SelectLine()` 将选区较小端点所在行的行首作为 Anchor，选择到较大端点所在行的下一个行首；
若已在末行则到 EOF。重复按键扩展到后续行，输出为正向选区；反向输入也按覆盖范围整行扩展。
它与移动/复制行的“排除末端恰在下一行首”规则不同，这是选择扩展命令本身的语义。

`MoveWord(forward, extend bool)` 与 `DeleteWord(forward bool)` 使用代码词边界，
不把双击的 Unicode word segment 直接当作导航边界。ASCII 分隔符采用 Monaco 默认集合
（含点、斜线和括号，不含下划线）；其他非空白 Unicode 字符连续成词，目标再经原生 Cluster 校正。
不按 camelCase 拆分，不提供可配置分隔符或 Intl.Segmenter 本地化分词。

向左移动到前词起点、向右到后词末尾；忽略相邻空白，最多跨一个 LF。单个分隔符紧邻后词时
采用快速导航，例如 `foo.|bar` 左移到 foo 起点，`foo|.bar` 右移到 bar 末尾。
删除有选区时只删除选区；相邻连续至少两个空白时先只删除这段空白，单个空白随相邻词删除。
没有空白时删除一个词/分隔符串，不沿用快速跳过点号的导航规则。
向后删除行首只删除 LF；向前删除行尾删除 LF 和下一行前导空白，保留下一行正文。
生成的空括号中向后删词继续使用已有成对删除。删除是单独撤销项，文档边界正常无操作。
读取使用 4096 字节游标缓存，不为词操作物化整条超长行，但未对扫描耗时作性能承诺。

`JumpToBracket() bool` 复用已有相邻括号匹配结果，在确认的两端之间移动；不是 XML/HTML 标签跳转。
无匹配、词法状态尚未就绪、超出原有 64 KiB 匹配预算、非空选区或预编辑中均不猜测、不修改选区。
这一版不在任意正文位置搜索最近的包围括号。搜索导航会中断普通输入合并，但不产生文本历史项。

补全先处理自身或关闭，再进入编辑命令；额外 Shift/AltGraph/其他修饰键不会宽松匹配复制、剪切、
粘贴或全选。Redo 的 Ctrl+Y 仅在 Windows/Linux 使用；macOS 使用 Cmd+Shift+Z。
已识别的只读编辑命令仍消费按键且不报错。物理按键的桌面保留和 IME 截获须分别实测。

参考 2026-10-17 核对的 VS Code `1.105.0` 编辑器源码：
[行命令](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/linesOperations/browser/linesOperations.ts)、
[词命令与平台绑定](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/wordOperations/browser/wordOperations.ts)、
[词边界](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/common/cursor/cursorWordOperations.ts)、
[整行选择扩展](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/common/cursor/cursorMoveCommands.ts#L124)、
[基础导航](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/browser/coreCommands.ts)、
[括号跳转](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/bracketMatching/browser/bracketMatching.ts)。
采用单选区命令语义和平台区分；不照搬服务/多光标对象，不宣称全套 Monaco 快捷键已兼容。
Scintilla 的 [词与键盘命令](https://www.scintilla.org/ScintillaDoc.html#KeyboardCommands)
以及 GtkSourceView 的 [move-words](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/signal.View.move-words.html)
也区分词操作与字符/行操作；本项目保留 UTF-8 与 Cluster 契约，不移植这些框架的原生动作体系。


## TOML、YAML、Shell 的引用上下文

自动配对、换行和补全复用扫描器的 String/Comment 上下文；YAML 块文本及 Shell
heredoc 按字符串处理，命令替换内部恢复代码上下文。Literal 标志表示反斜杠不转义
结束引号：TOML literal、YAML 单引号、Shell 单引号中，生成的右引号仍可正常越过。
TOML 连续第三个引号按原文输入形成三引号，不再生成另一组空字符串；YAML 结束单引号
之后紧接的单引号按原文输入形成 doubled quote。只追踪本视图生成的单字节配对，
不新增三引号补全模板或多字节配对状态机。String token 中的括号不参与代码括号匹配。
