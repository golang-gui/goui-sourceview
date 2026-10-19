# 编辑辅助

状态：已确认设计，实施见 [本轮计划](PlanEditorFeatures.md)。

## 选项与默认

缩进辅助线、智能 Home/End、智能退格及选中完整词的相同词高亮默认开启。
空白模式默认关闭，提供 None/Selection/Trailing/All；行尾标记独立，默认关闭。
列标尺默认空列表，复制/排序/去重调用方列值，列从 1 开始。
每项使用普通 getter/setter；ui 只协调值和信号，不持有平行编辑状态。

## 绘制与坐标

空格点、Tab 箭头使用 native Cluster/Geometry 范围，不能按字节乘平均宽度。
标记不改变文本、命中、行高、折行或内容宽度。真实 LF 才画行尾，软折行不画。
尾部空白依据逻辑行。缩进列由 TabWidth 解释已有 Tab、IndentWidth 定义缩进级别；
仅在前导空白内画线，双向布局按实际前导空白几何决定位置。标尺按代码字体空格宽度定位。
纯显示变化只重绘；字体、Tab、Wrap 和折叠侧栏变化沿用现有失效路径。

相同词只在单行完整词选区触发，使用语言 Word 边界，在可见正文内突出其他完全匹配。
不改 SearchQuery/FindBar，不宣称符号引用。按可见段落计算，不另存全文件匹配缓存；使用当前文档修订、有效子语言词规则和选区。

## 编辑与导航

Home/End 在当前显示行首/末非空白停靠点和边缘间切换；Shift 保留 anchor，
Primary+Home/End 仍到全文首尾。全空白显示行使用边缘。
智能退格仅在前导 ASCII 空白且无选区时生效：按 IndentWidth 退一级，TabWidth 计算
已有列；必要时替换后缀以处理跨目标列的 Tab，形成一个撤销项。其他情况保留 Cluster 删除。

可选 GoToBar 使用同 Root 的编辑器目标绑定和 Overlay。输入 1 起始行、可选冒号和码点列；
转换为原文 byte 并吸附合法停靠点。Enter 确认，Escape 关闭，无实时跳转。非法/越界原位提示。
只有挂载时接管 Control+G（包括 macOS，避免 Cmd+G 搜索冲突）。裸 SourceView 保留给应用。
示例显式安排同时显示哪一个工具栏，不新增中心浮层协调器。

## 主题、语义与验收

Basic/Modern 提供 whitespace、indent-guide、ruler、occurrence 等颜色与折叠样式。
沿用 stylesheet 变化通知，不增加系统订阅，不覆盖语法前景和半透明选区。
纯装饰不伪造动作；GoTo 输入/按钮保留真实语义、名称、状态、焦点和生命周期。

测试检查 Tab/空格混合缩进、组合字符、RTL/CJK、软折行、EOF 无 LF、选区/主题和多视图。
窗口操作走正常事件路径，像素断言声明坐标、预乘颜色与容差，1x/2x 分别验证。

参考：2026-10-19 的 [GtkSourceView 5.21.1 SpaceDrawer](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.SpaceDrawer.html)
区分空白类型和位置；本项目先采用四种常用模式，不引入矩阵配置。
[Monaco EditorOptions](https://microsoft.github.io/monaco-editor/typedoc/variables/editor_editor_api.editor.EditorOptions.html)
提供空白、标尺和选区高亮；借鉴明确选项，布局仍以本项目原生几何为准。
