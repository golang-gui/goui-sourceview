# 剩余语言基础折叠收尾计划与验收

2026-11-09，基线 `66df67a`，GOUI v0.7.0。用户已授权连续实现、验证并提交四批。
只做明显文本／词法边界，不增加完整解析器、语义求值或公共 API。
当前 87 个内置定义中 67 个有 Folding；以下 13 项补充有限规则，另 7 项有意保持无默认折叠。

| 批次 | 范围 | 状态 |
| --- | --- | --- |
| 1 | Makefile、Batch、GoTemplate、Jinja2 | 实现中 |
| 2 | AsciiDoc、reStructuredText、LaTeX | 待实现 |
| 3 | BibTeX、Gettext PO、Dotenv、Properties、CSV | 待实现 |
| 4 | Mermaid 的 Flowchart／Sequence 显式块；整体验收 | 待实现 |

## 边界

- Makefile：define/endef、条件块、显式续行、字面目标及可确认连续 recipe；不展开变量或 include。
- Batch：标签分节及显式 region；不推导 goto/call 或任意括号块。
- GoTemplate／Jinja2：显式控制块、对应结束标签、注释及 Jinja raw／块 set；不执行模板或恢复宿主混合结构折叠。
- AsciiDoc：等号标题和已确认分隔块；RST：常见装饰线标题、字面量／代码／注释块；不构建完整文档语法树。
- LaTeX：字面环境名、常见行首章节和原样内容；不展开宏、动态环境名或分析数学表达式。
- BibTeX：已闭合条目；PO：翻译记录；不求解字段、引用或复数规则。
- Dotenv：已闭合多行引用值；Properties：续行属性；CSV：已闭合多行引号字段，保留关闭行上的其他字段。
- Mermaid：Flowchart subgraph/end、Sequence 常见显式块/end；不扩展到全部图类型或图布局语义。

Plain、JSONLines、GoSum、GitIgnore、GitAttributes、TSV、Crontab 保持无默认规则。
不按空行、目录或业务关系猜分组；外部应用仍可通过已有 SetFolds 提供明确范围。

已有规则与视图状态分离沿用 [代码折叠设计](DesignFolding.md)。参考框架提供架构及具体可核实规则，
不把具有高亮等同具备同样折叠；以上有限规则也包含本项目自行设计的文本策略。

## 验证与提交

每批固定精确范围及反例，验证未完成输入、字符串／注释伪边界、长行分片、取消、并发和构造器。
原生窗口通过 Window.DispatchEvent 验证键盘／指针、搜索、删除、撤销、只读与共享视图隔离。
运行 GOWORK=off 的全量测试、race、vet、Windows／macOS 交叉构建；其他平台原生运行不以交叉构建代替。
功能及窗口／记录分别英文提交。各批实际命令、结果、失败修正、提交及限制在下方补写。
原有草稿与临时示例不纳入提交，不改 GOUI，不推送或发布。

## 第一批

进行中。

## 第二批

待实现。

## 第三批

待实现。

## 第四批与整体验收

待实现。
