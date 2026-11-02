# 常见文本格式与混合高亮

延续现有 Tokenizer/State/Context 与 UTF-8 分片契约。每种格式独立 ID、文件关联、词法及编辑规则，
按独立功能提交；共享设施随实际规则增加。不解释、执行或加载文件，不增加运行依赖，不修改 GOUI。
正文不累计副本；只保存有界词汇、必要定界符和不可变结构。引用/注释使用 Class，装饰分隔符不冒充结构括号。

参照 [GtkSourceView 5.22.1 语言定义](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/lang-intro.html)、
[Scintilla/SciTE 着色](https://www.scintilla.org/SciTEDoc.html) 与
[Monaco Monarch 分词状态](https://github.com/microsoft/vscode/blob/main/src/vs/editor/standalone/common/monarch/monarchLexer.ts)，
保留各格式边界并复用已有状态缓存，不引入它们的对象体系或词法依赖。
这里是词法分类及相应编辑辅助，不承诺格式校验、求值、渲染或编译器语义。
规范读取日期为 2026-11-02；未注明版本的网页为当日在线版本。
SRT/WebVTT、iCalendar/vCard 后置，当前不声明支持。

## CSV

明确识别 .csv 和 csv 围栏。逗号是分隔符，空格是字段正文；仅字段开头的双引号开启引用。
引用字段可跨行，两个相邻双引号表示转义；保留待定闭合引用，跨分片分类与整行一致。
不自动插入引号/括号，不从首行猜测标题，不要求相同列数，不读取或执行数据。

依据 [RFC 4180 §2](https://www.rfc-editor.org/rfc/rfc4180)（Informational，2005）。

## TSV

识别 .tsv、tsv 围栏及 text/tab-separated-values。Tab 分隔单行字段，引号和反斜杠均是普通字段文本；
不沿用 CSV 的引用、转义或跨行字段，不猜测字段类型/标题，不校验列数。关闭代码自动配对和建议。

依据 [IANA text/tab-separated-values 定义](https://www.iana.org/assignments/media-types/text/tab-separated-values)。

## AsciiDoc

识别 .adoc/.asciidoc/.asc 及 asciidoc/adoc 围栏。标题、列表标记、属性声明/引用、交叉引用、行注释和行内强调/代码分别着色。
同字符且同长度的四字符以上边界用于 listing、literal、passthrough、引用和注释块；块正文保持独立状态。
边界只保存字符/长度，不缓存正文；极长行仍按分片扫描。代码块采用代码正文颜色，当前不按 source 属性解析嵌入语言；
行内未闭合标记在行末恢复，不实现完整 Asciidoctor 替换和文档结构解析。

依据 [AsciiDoc Delimited Blocks](https://docs.asciidoctor.org/asciidoc/latest/blocks/delimited/) 与 [Source Blocks](https://docs.asciidoctor.org/asciidoc/latest/verbatim/source-blocks/)。

## reStructuredText

识别 .rst/.rest 与 rst/rest/restructuredtext 围栏。为标题装饰线、列表、强调、行内 literal/引用、指令和注释着色；
标题正文不根据下一行反向改色，遵守不能修改前一逻辑行的扫描契约。
双冒号和 code/code-block/sourcecode/raw 指令开启缩进 literal 块，空行保留块状态，回到父级缩进后恢复正文；
注释续行按相同缩进边界处理。代码块使用代码正文颜色，不执行指令、include 或 Sphinx 扩展，不提供完整文档 AST。

依据 [Docutils reStructuredText Markup Specification](https://docutils.sourceforge.io/docs/ref/rst/restructuredtext.html)（revision 10399，2026-09-05）。

## TeX/LaTeX

以 latex 为 ID，tex 为围栏别名，复用一个 TeX/LaTeX profile。支持控制词/控制符、% 注释、转义、结构括号、
$ / $$ / \( / \[ 数学区域及常见数学环境；数学正文使用数值颜色，命令保持独立。
verb 与 verbatim/verbatim*、lstlisting、minted 正文使用原样字符串规则，闭合环境跨分片识别。
仅结构括号自动配对；不把排版引号当作程序字符串，不解释宏、catcode、include 或第三方包定义。

依据 [LaTeX 文档](https://www.latex-project.org/help/documentation/)、[ltmiscen.dtx](https://github.com/latex3/latex2e/blob/develop/base/ltmiscen.dtx)（2025/04/28 v1.2h）和 [ltmath.dtx](https://github.com/latex3/latex2e/blob/develop/base/ltmath.dtx)（2026/06/01 v1.2p）。

## BibTeX

识别 .bib 及 bibtex/bib 围栏。区分条目类型、引用键、字段名、数字/宏值、# 拼接和跨行引用/花括号字段；
引用内花括号保护双引号，反斜杠本身不改变 BibTeX 的引用闭合规则，不能直接套用程序字符串。
支持 @string/@preamble；@comment 与条目外文字按非条目注释处理。正文仅保存层数，不累计完整字段。
不执行样式、解析宏值或加载引用数据库；不把 .bst 的专用语言当作 .bib。

依据 [BibTeX 0.99e](https://ctan.org/pkg/bibtex) 与 [bibtex.web 的 scan_balanced_braces](https://github.com/TeX-Live/texlive-source/blob/trunk/texk/web2c/bibtex.web)。
