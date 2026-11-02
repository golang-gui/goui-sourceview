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
