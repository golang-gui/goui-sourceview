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
