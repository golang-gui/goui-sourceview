# 查询范围、多行与正则替换

最初设计于 2026-10-19，基于 GOUI v0.2.0；当前依赖为 GOUI v0.3.0。
实现入口为 `search.go`、`internal/editor/search*.go`；FindBar 保留独立界面与会话生命周期。
本轮按功能优先，不新增外部搜索依赖，也不承诺全量搜索后台化。

## 查询与匹配

SearchQuery.Regex 默认 false，保持原有 Unicode 字面量语义。true 使用 Go regexp 语法，
IgnoreCase 对整个表达式启用 (?i)，跨行锚点/点号由调用方显式写 (?m)/(?s)。
不采用 JavaScript/PCRE 的回溯、后行断言或反向引用语义；字符串仍以 UTF-8/LF 存储。

非法表达式保留在 Search()，SearchError() 返回编译错误；Find 不产生命中，替换返回错误且
不修改文档。FindBar 显示本地化提示和错误边框，完整引擎错误通过状态名称/快照及 API 可读。
不使用日志或默认错误弹窗。空查询始终关闭匹配，即使 Regex=true。

正则用 FindAllStringSubmatchIndex 的不重叠顺序，包括其忽略紧接非空匹配的空匹配规则。
字面多行查询也采用从源区间开始的规范不重叠序列，统一绘制、导航、计数和全部替换。
零长度当前命中再次导航时向相应方向推进，再按规范序列环绕；UTF-8 内部字节不会成为命中。
空匹配没有正文背景范围，当前 caret 仍可定位。整词沿用 Word/NonWord 边界策略。

## 捕获组与事务

Regex=false 时替换文本保持字面值；true 使用 Go ExpandString 的 $1/${name}/$$ 语义。
当前替换只接受原始匹配范围，不能把 Cluster 吸附扩大的可视选区当作捕获源。
全部替换在一个原始修订上计划，再一次 ApplyBatch；上限 100000 个编辑和 32 MiB
展开后的插入文本，超限整批不执行。限制约束事务输出，不代表展开临时内存的峰值上限。

## 独立查询范围

SetSearchRange(*textedit.Range) 复制并验证原文范围；nil 恢复全文件，SearchRange 返回副本。
它不依赖 FindNext 更新的可视选区。FindBar 的“在选区中查找”打开时捕获编辑器当前范围，
关闭时清除；之后更改选区不会重新捕获。查询变化保留范围，换 Document、模型 Reset，或
卸载期间漏过模型修订后重挂载时清除。普通编辑按 Change 中的顺序映射边界：起点相同的
插入/替换保留左边界，终点包含该编辑结果。未引入通用范围装饰或公共锚点系统。

正则锚点及整词保留全文件上下文，选区仅过滤完整落在范围内的规范匹配；不把选区切片
当作新的文首/文末。空范围可接受位于该点的零长度匹配。ui 的 SearchRange/ClearSearchRange
只协调显式配置，未指定时保留交互状态。

FindBar 的范围按钮只允许从非空选区开启，已开启时始终允许关闭（编辑器本身禁用时除外）。
切换范围会即时更新当前结果、搜索背景和计数状态，具体交互见 [FindBar](DesignFindBar.md#查询与交互)。
内置查找栏关闭或编辑器卸载会释放旧会话的范围；查询及替换文本保留。

## 搜索高亮显隐

HighlightSearchMatches 默认 true；false 仅停止绘制普通/当前搜索背景，不改变上述搜索、
计数、替换契约，不改变普通选区。裸视图和同 Document 的其他视图拥有独立显隐设置。
SetHighlightSearchMatches 不重建排版或修改语法前景；未显式配置的声明式重建保留交互显隐状态。
搜索查询和原始当前匹配仍可用于导航及替换，包括零长度匹配的进展。

## 缓存与资源

单行字面量保留分块扫描。正则与多行字面量按 Search 对象缓存一个规范结果序列，键为
Model 身份、Revision、查询和范围。正则另保留对应不可变全文与捕获索引，供替换使用；
替换前按当前修订确认，旧结果不交给回调。换查询/范围/文档释放旧缓存，修订变化按需重建。
不共享视图的可变查询状态；该缓存在 GUI 线程顺序访问，无原生对象。

计数 UI 仍每帧推进 64 KiB 的起点区间；正则/多行首次构建缓存是同步全量分析，
不能把计数分帧误写成正则编译/扫描已按帧限时。内存随当前文本、捕获组和结果数增长。
本轮不以缺乏性能数据宣称大文件正则低延迟；后台化和更严格的峰值预算留待专项测量。

## 多行输入与组合

FindBar 使用 GOUI TextView + ScrollView 的公开契约编辑查询和替换，不复用到 SourceView 正文。
粘贴/程序化设置的 LF 原样保留；Alt+Enter 经 TextView 正常按键路径插入 LF。
Enter/Shift+Enter 仍查找下一/上一项，替换字段的无修饰 Enter 执行当前替换。
行高依据公开 ContentSize 和 Padding，显示至多五个逻辑行的高度预算，单字段至多 200 DIP，
更多内容由原生文本布局和滚动容器处理；不按字体 point 推断排版行高。
浮层自然宽度 512 DIP 以容纳四个选项，窄宽度仍隐藏次要操作，保持主要输入可用。

上述为当前实现记录，并不表示 GOUI 已具备紧凑自动增高的多行输入框契约。
单行垂直居中、多行自动增高、上限滚动及统一基线由 GOUI 另行规划；本轮仅修正范围交互和
搜索会话显隐，不继续在 FindBar 增加 TextView 排版补丁，见 [修正记录](PlanFindBarSearch.md)。

## 验证与参考

逻辑测试以 Go regexp 的原文坐标为参考；分别断言零长度、Unicode、跨行不重叠、命名捕获、
范围复制/映射/换文档、非法表达式和一次撤销。窗口使用真实正常事件路径验证输入与控件状态；
保留原浮层颜色、字体、窄布局断言，只按新增操作区更新自然宽度期望，并新增单行无滚动条检查。

官方参考（2026-10-19 核对；本机实际 Go 1.25.8）：
[Go regexp](https://pkg.go.dev/regexp#Regexp.FindAllStringSubmatchIndex)、
[ExpandString](https://pkg.go.dev/regexp#Regexp.ExpandString)、
[GtkSourceView SearchContext](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.SearchContext.html)、
[Scintilla search](https://www.scintilla.org/ScintillaDoc.html#Searching)、
[Monaco/VS Code FindWidget 1.105.0](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/find/browser/findWidget.ts)。
借鉴独立搜索状态和紧凑浮层；最终语法、坐标与事务以本文和 Go 契约为准，不宣称三者兼容。
