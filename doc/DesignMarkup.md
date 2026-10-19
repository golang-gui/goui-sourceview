# 标记语言与标签编辑

状态：已确认方向与首轮范围，实施见 [本轮计划](PlanEditorFeatures.md)。标记语言先于编程语言扩展。

## XML / HTML

先完善标签匹配与自动闭合，再实现名称联动。规则由语言定义提供，不在通用按键路径按语言 ID 分支。
XML 名称区分大小写，HTML 按自身名称/空元素/raw-text 规则；属性引号中的 > 不结束标签。
注释、CDATA、自闭合、HTML 空元素及不确定上下文不自动插入结束标签。
输入合法开始标签的 > 后插入结束标签，caret 保留中间；跳过既有结束端需校验所有权和当前文本。
联动在已确认名称对上应用一次批量事务；另一端被改动、结构不完整或身份/修订改变时退出。
初始位置跟踪私有，不先导出通用装饰和多光标系统。所有操作服从只读、IME、撤销和重入契约。

## 内置 Markdown

language.Markdown 正式加入语言表及 .md/.markdown 识别。语法以 CommonMark 0.31.2 为依据：
ATX/Setext 标题、引用、列表、分隔线、缩进/围栏代码块、强调、代码跨度、链接/图片、引用链接、
转义、实体及 HTML 注释。任务列表、删除线和表格作为明确扩展分别测试。
这是源码着色/编辑 profile，预览和 HTML 渲染由应用组合；不据有限样本承诺完整规范解析一致性。

新增 markdown.heading/code/emphasis/link 等样式键并提供 Basic/Modern 默认前景，
暂不改变 token 字号/字重。Newline 继续列表编号、引用前缀，空项退出一级。
代码区域暂停 Markdown 列表/强调操作。标题到下一个同级/更高级标题前一行或 EOF；
围栏折叠包含结束围栏，保留开始围栏；代码里的标题不成为折叠区域。

围栏首先支持已有 JSON/XML/HTML/TOML/YAML/Shell。未知名字保持代码前景。
子语言的 token/上下文/词边界/配对/缩进/默认补全同步选择；外层 Document 语言仍为 Markdown。
应用自定义子语言用显式构造配置，不引入全局注册或隐式匹配。子语言配置和状态必须不可变，
文本片段和 token 偏移在宿主/子语言边界转换，外层围栏识别优先于子语言未闭合状态。

现有 example/markdown 继续验证公开扩展，从内置定义副本定制样式/规则及应用补全。
AsciiDoc/reStructuredText 后续按独立语法 profile 设计，编程语言、多光标和语言服务保持后置。

## 验证

取规范样本验证已支持规则，覆盖跨行/分片/围栏长度、嵌套、Unicode、畸形输入及编辑后恢复。
标签测试包含引号 >、命名空间、空元素、注释/CDATA/raw-text、配对失效、撤销与共享视图。
窗口验证标题折叠、列表 Enter/退格、围栏内外补全/配对、主题和应用自定义规则。

参考：[CommonMark 0.31.2](https://spec.commonmark.org/0.31.2/)，
[Monaco LanguageConfiguration](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.languages.LanguageConfiguration.html)。
参考用于语法与职责，不引入 Monarch 解释器、浏览器或 IDE 子系统。

## 已落实的接口与边界（2026-10-19）

`language.NewMarkdown(map[string]*Language)` 复制围栏名称表；nil 使用已有六语言及 sh/bash/yml
别名，显式空表禁用嵌入。`Context.Embedded` 给出当前位置的子定义；SourceView 用它选择词边界、
配对、注释、换行和默认补全，括号查找限制在已分析的连续子区域，Document 定义不变。

`EditingRules.Tags` 为可复制的 TagRules：大小写、空元素和 raw-text 名单；XMLTags/HTMLTags
为默认策略。Match 返回已确认名称范围，ClosingTag 对完整前缀保守决策。无需语言 ID 分支。
SourceView.LinkedTags 默认开启；普通输入/删除在原修订中验证并一次 ApplyBatch 镜像，
直接模型编辑、粘贴、IME 不镜像。每次重新确认结构而非持有跨修订的名称所有权。
未配对、改变另一端、非法新名称自然退出；普通编辑、撤销及只读语义保持。

标签辅助使用当前文档的不可变全文，不按文档大小直接禁用功能；根标签分析同步进行，
临时文本、配对数组和栈的空间随当前文档大小增长，最多保留当前分析调用的资源，
不承诺大文件标签重命名低延迟。嵌入连续区域需已分析，扫描至多各 64 KiB，不猜截断上下文。
Markdown 当前行以最多 64 KiB 不可变前缀重放，修正分片中的围栏/行内规则；超限行降为
普通/代码前景，并暂停该围栏子语言到关闭围栏。不是完整 CommonMark 解析器：Setext 下划线
着色和标题折叠已实现，但上行标题文字不回溯重着色；复杂嵌套容器/强调与链接容错继续完善。
未知围栏不运行子语言编辑规则。示例从内置定义副本定制规则/样式/应用补全。

Markdown 的三次反引号输入补为六个字符的自有配对，仅允许在至多三前导空格的行首完成，
Enter 在配对围栏之间留出内容行。HTML 注释中的伪标题不建立折叠。新增对应正常规则及
超长围栏内容行的退化/恢复断言；语言身份与标签匹配缓存均随当前修订失效。

HTML 名称比较只折叠 ASCII A–Z，不把其他 Unicode 字符变成相同名称；标签联动与折叠
使用同一规则，raw-text 的结束名称也如此。独立用例覆盖 ASCII 混合大小写、希腊字母
与 long-s 伪结束端，避免宽泛 Unicode 大小写比较误配。
依据 2026-10-19 核对的 [HTML syntax](https://html.spec.whatwg.org/multipage/syntax.html#syntax-tag-name)；
这里只采用名称比较约束，不承诺完整 HTML 树构建或可省略标签推断。
