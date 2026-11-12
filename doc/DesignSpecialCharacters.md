# 特殊字符可见表示

状态：2026-11-12 第三批已实现、未发布；验收与平台缺口见 [后续三批](PlanEditorFollowups.md#第三批验收)。

## 显示契约

SourceView 的 ShowSpecialCharacters / SetShowSpecialCharacters 控制独立的可见标记，默认关闭。
ui 提供同名声明式属性，省略时保留原设置；示例 View 菜单提供开关及专门样本。
本批识别 C0（不含已有空白/换行机制处理的 Tab/LF/CR）、DEL/C1，以及明确列出的零宽、连接、
双向嵌入/覆盖/隔离控制、软连字符、BOM、Unicode 行/段分隔符。普通中文、其他非 ASCII 字母和
Emoji 变体选择符不作为此项；合法 ZWJ/ZWNJ 也只是显示位置，不作为错误诊断。

标记为字符簇底部的空心小菱形；同一原生位置的多个特殊字符合并绘制，悬停显示所有不同码点与标准名称。
原生 cluster 不可分时标记整个 cluster 的起点，不发明内部光标停靠点；名称不包含原始控制字符。
这是可见位置及说明，不把原字符替换成展开标签，不修改双向算法、连接效果、行高、换行或文本宽度。
NUL 是原生边界的单独适配：Pango.Layout.set_text 官方契约说明，即便显式给出正长度也会在 NUL 截断。
因此所有后端送入原生排版的显示串把 NUL 等字节替换为 ASCII 空格，作为可测量空白单元，
正文/预编辑原值与 UTF-8 偏移不变。此适配同时覆盖普通段落、估计和长行页及页缝测量，
与标记开关无关；不承诺 NUL 与组合字符相邻时维持未经适配的 Unicode 聚类，停靠遵循实际显示串的原生 cluster。
正文、复制、查找、保存、修订、保存点及正文历史不因显示开关改变。
不额外接受输入、焦点或点击动作；语义快照公开开关状态，标准正文/选区仍用原 UTF-8 坐标。

## 几何、缓存与生命周期

只消费已经测量的自有 Geometry，按原生 Snap/Caret 定位；不估算字符平均宽度，不为标记新增原生布局。
扫描按帧预算和有限 rune 数推进，绘制与 tooltip 不扫描全文或创建原生对象。
每份标记缓存归一个视图的一个 paragraph，键包含文档修订、原生页版本、文本窗口起点/内容及局部视口。
只保留与当前视口相交的位置，长行收起时再裁剪到首行预览；tooltip 也受同一裁剪区域约束。
位置合并且类型使用固定大小位集合，内存受可见缓存文本与既有布局数量/页数限制。
滚动、窗口变化、编辑、换文档、字体/布局失效均不复用过期坐标；关闭开关释放标记缓存，段落淘汰/卸载一并释放。
预编辑期间暂不显示标记，避免把临时文本解释成已提交源码；不改变预编辑会话本身。

## 验证

分类、合并、预算推进、裁剪和安全名称以包测试验证；原生排版覆盖零宽、组合字符、Emoji、双向文本与控制字符。
验证开关前后正文/保存点/历史、原生布局身份、光标/命中/选区不变，编辑与撤销后标记失效重建，
不同视图开关和缓存隔离、预编辑、长行分页、折叠及高亮并存。
窗口程序走正常事件分发，分别记录 Basic/Modern、缩放、实际后端与未运行的平台。

## 参考与取舍

2026-11-12 核对，未运行参考编辑器：

- [Scintilla Character representations](https://scintilla.org/ScintillaDoc.html#SCI_SETREPRESENTATION)（页面 2026-10-01）：提供控制字符助记符及可配置替代表示；采用可选可见化和码点说明，不复制其按替代字符串展开宽度的呈现体系。
- [VS Code 1.104.3 viewLineRenderer](https://github.com/microsoft/vscode/blob/1.104.3/src/vs/editor/common/viewLayout/viewLineRenderer.ts#L984)：renderControlCharacters 可输出控制图形或 U+ 表示；本项目保留原生 cluster 与双向/连接排版，采用位置叠加标记。
- [GtkSourceView 5 SpaceDrawer](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.SpaceDrawer.html)（API 5、页面库版本 5.23.0）：按开关用符号可视化空白；采用专用可选显示层，不依赖 GTK，也不将该文档理解为全部 Unicode 控制字符的承诺。
- [Pango.Layout.set_text](https://docs.gtk.org/Pango/method.Layout.set_text.html)（在线文档 1.58.2）：正长度仍在 NUL 截断。本仓 NUL 样本在当前 Pango 路径实测只测出首字母宽度；采用上述等字节空白适配，不修改 GOUI 或加入原生编辑器依赖。

GtkSourceView 页面所链 GitLab 源码本轮返回 HTTP 406，未据此声称核对其内部绘制算法；
本项目几何与资源契约以自有 Geometry、paragraph 缓存源码和本仓原生实测为依据。
