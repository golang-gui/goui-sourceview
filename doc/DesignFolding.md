# 代码折叠

状态：已确认设计，实施与验证见 [本轮计划](PlanEditorFeatures.md)。依赖 GOUI v0.2.0。

## 范围与所有权

语言定义的可选 Folding 函数从不可变 UTF-8/LF 全文生成 FoldRange。范围行号从 0 开始，
StartLine 是保留的标题行，EndLine 包含在范围内；隐藏 StartLine+1..EndLine。
范围允许嵌套，不允许交叉；排序、去重、同起点保留外层、越界检查由组件完成。
空结果是正常无区域，nil 函数关闭该语言的折叠，不添加 ErrNoFold。

Document 共享候选范围与分析任务；SourceView 独立保存折叠状态、可见行映射和几何。
一个视图的折叠不影响另一视图，不改文本、模型修订或撤销历史。只读允许折叠。

## 分析与失效

首次挂载请求分析；连续编辑合并等待 150ms。每个 Document 最多一个 worker 和一个最新请求。
GUI 生成独立字符串快照，后台只访问快照、不可变规则和 context。通过 gui.App.Post 回到
GUI 后验证文档身份、修订、语言定义身份和任务代次。取消/过期不是错误；有效失败经现有
ConnectError 报告一次。最后视图卸载取消任务并解除 owner 引用。

首轮完整扫描优先完成行为；快照成本与扫描耗时分别测量，不以后台扫描承诺 GUI 延迟上限。
内置规则可复用纯词法函数，使用独立状态；不访问 GUI 高亮缓存，不改变现有 Tokenizer 调用契约。

编辑前后按实际 Change.Edits 顺序映射私有字节锚点。触及区域或边界立即展开并失效；
不相交区域映射后可保留状态。新结果仅继承可明确匹配的状态；Reset/换文档/换定义清理旧候选。
卸载保存视图意图，重挂载核对修订，未验证区域不隐藏。

## 可见行与原生布局

位置转换：真实文档 → IME 段落投影 → 折叠可见段落 → 原生显示行/Geometry。
GOUI HeightIndex 将高度限制为至少 1，因此仅索引可见段落，不用零高度表达隐藏。
私有可见映射保存连续行段与前缀计数；真实行与可见序号双向查询。原生缓存仍由真实
行身份和 IME 投影身份确定，可见序号不成为正文身份。纯折叠保留未变化的 native layout。
行号报告真实行号，软折行不重复编号；侧栏固定，不随横向滚动。隐藏行不计内容高度。

绘制、命中、导航、翻页、锚定、reveal、候选框与快照使用同一映射。锚点变隐藏时移到
可见祖先标题。顶部锚定与 caret reveal 沿用现有优先级，不在 Paint 里重排。

## 交互

- 默认开启、全部展开；关闭折叠展开全部。有规则时固定保留折叠列，关闭行号仍可操作。
- Fold/Unfold/ToggleFold 参数指定标题行，bool 表示实际变化；无区域/尚无结果返回 false。
- FoldAll/UnfoldAll 更新全部层级；单独展开父区域保留子区域意图。提供独立副本的区域查询和统一变化信号。
- 上下/翻页与左右跨区域移动跳过隐藏内容。Shift 扩选仍是包含隐藏正文的真实连续范围。
- 折叠导致选区端点隐藏时收为标题行末尾光标；两端可见的跨区域选区保留。
- 复制/显式删除已有选区包含隐藏正文；无选区时在折叠边界 Delete/Backspace 首次只展开。
- Find/GoTo/SetSelection/Undo/Redo 指向隐藏内容时展开必要祖先再 reveal。
- IME 开始前展开替换范围；预编辑期间不执行折叠命令，分析结果等待合成结束后重新核验。
- 键盘折叠/展开作用于光标最内层区域；Windows/Linux Ctrl+Shift+[ / ]，macOS Cmd+Option+[ / ]。
- 按钮和淡色省略标记沿用 Basic/Modern；标记空间不足不覆盖正文，不增加原生软折行。

快照保留一个真实连续 VisibleRange，其余可见原文分段放进现有 goui.sourceview 属性，
合计预算 64KiB。报告可见折叠控件的名称、边界和状态，动作经正常 Window 输入路径。

## 内置规则与测试

先支持 JSON 对象/数组、XML 成对元素/注释/CDATA、HTML 确认标签/注释。
跳过字符串中的括号、属性引号中的 >；HTML 空元素、raw-text 与大小写按自身规则处理。
不完整结构只发布已确认范围。Markdown 标题/围栏随标记语言切片加入。

逻辑测试覆盖嵌套、交叉/非法结果、随机行映射、锚点编辑、取消/过期、对象隔离。
窗口覆盖关闭行号、折行、横向滚动、选择删除、搜索、撤销、共享文档、IME、卸载与主题。

## 参考与取舍

2026-10-19 查阅 [Monaco FoldingRangeProvider](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.FoldingRangeProvider.html)
与 [FoldingRange](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.FoldingRange.html)：借鉴规则与呈现分离，使用本项目零起始行号和直接绑定。
[Scintilla 5 文档](https://www.scintilla.org/ScintillaDoc.html#Folding)（页面 2026-07-22）明确行可见性属于视图。
[VS Code 1.105.0 hiddenRangeModel.ts](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/contrib/folding/browser/hiddenRangeModel.ts)
首行保留、合并隐藏祖先；采用这一模型，位置跟踪先保持私有，不引入通用装饰 API。

折叠占位的水平范围预留行尾标记开启时的最大宽度，切换纯绘制标记不改变正文排版或滚动范围。
