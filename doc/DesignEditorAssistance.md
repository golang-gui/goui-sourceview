# 编辑辅助

状态：已确认设计，实施见 [本轮计划](PlanEditorFeatures.md)。

## 选项与默认

缩进辅助线、智能 Home/End、智能退格及选中完整词的相同词高亮默认开启。
空白模式默认关闭，提供 None/Selection/Trailing/All/Boundary；行尾标记独立，默认关闭。
列标尺默认空列表，复制/排序/去重调用方列值，列从 1 开始。
每项使用普通 getter/setter；ui 只协调值和信号，不持有平行编辑状态。

## 绘制与坐标

普通空格显示圆点，Tab 显示沿实际文字方向的箭头，NBSP（U+00A0）与窄 NBSP
（U+202F）显示空心圆，全角空格（U+3000）显示空心方框。不将所有 Unicode 分隔符
一律视为空格；零宽字符和控制字符另行设计。

| 模式 | 显示规则 |
| --- | --- |
| None | 不显示空白标记 |
| Selection | 仅显示完整包含于当前半开选区的空白测量单元 |
| Trailing | 显示逻辑行尾的上述空白字符；全空白行全部属于尾部 |
| All | 显示全部支持的空白字符 |
| Boundary | 显示行首、行尾、连续普通空格，以及所有 Tab 和上述特殊空格；隐藏内部孤立普通空格 |

标记复用 native 测得的 Cluster 坐标及所属 TextLine 行高，不能按字节乘平均宽度。
不可分的同类空白 cluster 只标记一次，不猜测内部停靠点；混有正文的 cluster 不覆盖正文。
选区按模型 UTF-8 字节区间判断，正反方向等价，空选区不产生 Selection 标记。
IME 仅跳过投影覆盖的段落，未覆盖段落仍映射到原文行和选区。

标记不改变文本、命中、行高、折行、内容宽度、撤销或输出字节。真实 LF 才画行尾，
软折行不画，EOF 不补标记。CRLF 在模型中只有一个逻辑 LF，行尾标记使用中性回车箭头，
不重复显示 CR 和 LF；文档输出格式及 EOF 换行数量见 [文档换行](DesignLineEndings.md)。
空白模式与行尾标记独立，均属于视图状态。尾部和 Boundary 按完整逻辑行判断，
软折行不产生新的行首/行尾。标记位于现有代码区裁剪范围内。

每个 paragraph 复用创建时已有的只读度量值，按不超过 256 个 cluster 的块维护几何包围树，
不保存逐字符绘制指令，不另调原生测量。已有段落缓存（最多 128 项）拥有度量、索引和文本；
键及失效沿用文本身份、宽度、字体、Tab、Wrap、投影和 Context 的既有规则，驱逐/销毁时一起释放。
分片度量可以携带其逻辑行偏移、总长度及有界 UTF-8 读取函数，边界扫描仍使用完整逻辑行坐标，
不把片段端点视为缩进或尾部。读取函数随文本修订失效，不承担测量或原生资源职责。
索引增加的节点量与 cluster 块数量成正比；度量快照的存留量仍与缓存段落大小相关，
并不宣称仅占可见字符内存。不同 SourceView 不共享可变索引。

窗口通过现有帧调度推进可见段落索引，与排版共用每视图最多 2ms 的软预算；
同时限制单步输入量。边界扫描分步执行，已确认的标记可先显示，过期缓存直接丢弃。
绘制查询同时过滤水平和垂直视口，支持坐标与源顺序不同的双向文本；选择/模式变化只过滤和重绘。
无事件循环的显式离屏绘制会同步完成该段索引，不把窗口分帧保证扩大到此路径。

缩进列由 TabWidth 解释已有 Tab、IndentWidth 定义缩进级别；仅在前导空白内画线，
双向布局按实际前导空白几何决定位置。标尺按代码字体空格宽度定位。
纯显示变化只重绘；字体、Tab、Wrap 和折叠侧栏变化沿用现有失效路径。

相同词只在单行完整词选区触发，使用语言 Word 边界，在可见正文内突出其他完全匹配。
不改 SearchQuery/FindBar，不宣称符号引用。按可见段落计算，不另存全文件匹配缓存；使用当前文档修订、有效子语言词规则和选区。

## 编辑与导航

Home/End 在当前显示行首/末非空白停靠点和边缘间切换；Shift 保留 anchor，
Primary+Home/End 仍到全文首尾。全空白显示行使用边缘。
智能退格仅在前导 ASCII 空白且无选区时生效：按 IndentWidth 退一级，TabWidth 计算
已有列；必要时替换后缀以处理跨目标列的 Tab，形成一个撤销项。其他情况保留 Cluster 删除。

内置行列定位通过 Ctrl+G 或 SourceView.ShowGoTo 打开，输入 1 起始行、可选冒号和码点列。
有效输入回车后设置合法选区、展开覆盖折叠并滚动至光标；非法输入只显示校验提示。
Escape 关闭并按焦点归还规则恢复正文。定位与查找／替换互斥，由 SourceView 统一管理。
内部输入使用现有 TextInput、样式、语义及事件路径，不公开 GoToBar、Target 或单独声明式绑定。
移除 CommandGoTo 的快捷键不影响 ShowGoTo 的程序调用。

## 主题、语义与验收

Basic/Modern 提供 whitespace、selected-whitespace、indent-guide、ruler、occurrence 等颜色与折叠样式。
选中标记独立使用 selected-whitespace 前景，不覆盖语法文字；行尾标记按换行字节是否被选中决定颜色。
沿用 stylesheet 变化通知，不增加系统订阅，不覆盖语法前景和半透明选区。
纯装饰不伪造动作；GoTo 输入/按钮保留真实语义、名称、状态、焦点和生命周期。

测试检查 Tab/空格混合缩进、组合字符、RTL/CJK、软折行、EOF 无 LF、选区/主题和多视图。
窗口操作走正常事件路径，像素断言声明坐标、预乘颜色与容差，1x/2x 分别验证。

参考（2026-10-24 核对）：[GtkSourceView 5.22.1 SpaceDrawer](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.SpaceDrawer.html)
区分空白类型和位置；本项目采用五种常用模式，不引入矩阵配置。
[Monaco EditorOptions](https://microsoft.github.io/monaco-editor/typedoc/variables/editor_editor_api.editor.EditorOptions.html)
提供空白、标尺和选区高亮；借鉴明确选项，布局仍以本项目原生几何为准。

定位栏属于 SourceView 的内部状态，声明式重建不协调其打开状态或重新初始化输入。

[Monaco/VS Code whitespace.ts](https://github.com/microsoft/vscode/blob/main/src/vs/editor/browser/viewParts/whitespace/whitespace.ts)
（当日 main 源码）按视图行处理空白呈现；本项目借鉴视口过滤与显式模式，仍使用 GOUI
测量结果，逻辑行边界不从可见片段猜测。
[Scintilla SCI_SETVIEWWS](https://www.scintilla.org/ScintillaDoc.html#SCI_SETVIEWWS)
提供可见性与缩进相关模式；本项目不增加逐类型矩阵或复制其枚举数值。

声明式 WhitespaceMode / ShowLineEndings 只在显式声明时协调；省略时保留控件当前状态，
新控件仍默认 None/false。显式 None/false 可以关闭，互不影响。示例 View 菜单每次打开
从 getter 构造当前标记，不另建模式信号或默认快捷键。


## 指针样式与覆盖层关闭

正文／行号／折叠按钮的鼠标样式使用当前控件局部位置。PointerEnter 或覆盖层关闭
引发的 crossing 可能没有相邻 PointerMove，因此 SourceView 在共享 MotionEventController
处理进入通知前读取 CrossingContext.Position。正常移动仍复用原有 MotionInfo；
不触发文本测量，不新增控件事件协议。查找栏关闭后重新命中正文，应立即恢复文本
光标，侧栏则保持默认或折叠按钮的指针样式。
