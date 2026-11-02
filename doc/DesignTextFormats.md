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

## gettext PO/POT

识别 .po/.pot，po/pot/gettext 围栏。支持 msgctxt/msgid/msgid_plural/msgstr、复数索引及多行消息的逐行字符串。
引用按 PO 的 C 风格转义处理；引用不会跨越未转义的物理换行，连续消息片段由各行独立闭合。
区分译者/提取注释、来源引用、标志、前一消息和废弃消息；特殊元数据保留 Comment Class，不误启用补全/代码配对。
只提供双引号配对，不解释复数表达式、不编译 MO、不改变翻译内容。

依据 [GNU gettext PO File Entries](https://www.gnu.org/software/gettext/manual/html_node/PO-File-Entries.html) 和 [Plural Forms](https://www.gnu.org/software/gettext/manual/html_node/Entries-with-Plural-Forms.html)。

## EditorConfig

识别 .editorconfig 及 editorconfig 围栏。采用独立状态：节正文是 glob，允许内部方括号与花括号，不按 INI 第一个 ] 提前结束。
键值仅以 = 分隔；# 和 ; 只在首个非空白位置开启注释，值中的引号、反斜杠及行尾 #/; 都是普通文本。
不加载或应用配置、不执行 glob 匹配，结构标记不自动配对，提供常用属性名建议。

依据 [EditorConfig Specification 0.17.2](https://spec.editorconfig.org/)（No inline comments、Glob Expressions）。

## JSON5

独立 JSON5 profile 和 .json5 识别，不能成为 JSONC 别名。支持单双引号、裸键、注释、行连接、十六进制、
正号、首尾小数点以及 Infinity/NaN；对象与数组使用不可变帧，键/值分类在分片和编辑后保持一致。
数值使用有限状态识别，超长数值不缓存全部文本；单词只保留有界关键词。
保留 JSON/JSONC 原有语法，JSON5 不启用 JavaScript 的 regex/template/undefined 或执行语句。
不作为完整语法校验器，错误恢复在非转义物理换行后离开普通引用。

依据 [JSON5 Data Interchange Format](https://spec.json5.org/)（1.0.0）。

## EDN

EDN 使用独立 ID、词汇和编辑规则，复用 Clojure 扫描设施中的数据形式：字符串/字符、keyword/symbol、数字、集合、tagged literal、分号注释和 #_ 完整形式丢弃。
不启用 Clojure 的 defn 等关键字、quote/deref/metadata/unquote/reader conditional 等 reader 宏；不求值标签、不自动读取命名空间。
保留不可变丢弃帧，嵌套丢弃不提前恢复正文，字符串正文不累计副本。词法数值着色不代替 EDN 数字合法性校验。

依据 [edn-format/edn 规范](https://github.com/edn-format/edn)。

## Git config

识别 .gitconfig、.gitmodules、.git/config 与 config.worktree，其他泛化 config 不猜测为 Git。
区分节/引用子节、变量和值；引号外 #/; 开始注释，引号内保留正文，值的末尾反斜杠延续下一物理行。
子节与值的转义分开处理，节不会跨行；不将单引号当作引用，不把值交给 Shell。
支持常用变量名建议，配置正文不自动插入代码括号；不读 include、不访问 Git 仓库或应用配置。

依据 [git-config Syntax](https://git-scm.com/docs/git-config#_syntax)（页面 2.56.0）。

## SSH config

识别 ssh_config、ssh_config.*、.ssh/config 和 .sshconfig，sshconfig/ssh-config 围栏。
指令名不区分大小写，支持空白或 = 分隔、双引号、引号外 # 注释、% token 及 ${ENV}。
Host/Match 模式与远端命令保持参数文本，单引号不承担配置引用语义，不把 ProxyCommand 送入 Shell tokenizer。
提供指令建议，不读取 Include、不执行 Match exec、不连接远端或应用配置。

依据 [OpenBSD ssh_config(5)](https://man.openbsd.org/ssh_config)。

## crontab

识别 crontab、.crontab/.cron，crontab/cron 围栏。区分环境赋值、五段时间、月/星期名、宏与命令正文。
# 仅在首个非空白位置开启注释；赋值/命令后的 # 保留文本，不套用 Shell 注释与引号规则。
命令中的未转义 % 单独标记，\% 保留正文；不执行命令、不安装任务或应用环境设置。
默认以用户 crontab 的五段时间边界分类；系统 crontab 的额外用户名作为命令区域的普通文本，不冒充完整方言校验。

依据 [crontab(5)](https://man7.org/linux/man-pages/man5/crontab.5.html)（Cronie）。

## Mermaid

识别 .mmd/.mermaid 与 mermaid/mmd 围栏，提供常见图类型/控制词、节点标识、连接标点、引用标签、Markdown 标签及 %% 注释。
双引号标签遵循 Mermaid 的文字边界，不引入 C 风格反斜杠转义；Markdown 引用以反引号加双引号闭合，可跨行。
兼容 %%{...}%% 配置指令的独立元数据区域；只做词法高亮，不执行 click、href 或配置，不渲染图表。
不同图类型的具体语法校验和完整结构语义不在本轮范围。

依据 [Mermaid 12.2.0 Syntax Reference](https://mermaid.js.org/intro/syntax-reference.html) 与 [flow.jison](https://github.com/mermaid-js/mermaid/blob/develop/packages/mermaid/src/diagrams/flowchart/parser/flow.jison)。
