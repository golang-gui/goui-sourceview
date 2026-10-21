# 括号编辑与匹配

本专题描述 `pairs.go` 与分析器编辑上下文的当前契约。控件保持单光标；
结构匹配与自动插入分开，未知语法状态不作为普通代码处理。

## 输入与历史

`AutoPairs` 默认开启，`SetAutoPairs(false)` 关闭自动插入、包裹、跳过和成对删除。
处理非组合的字面 Commit；支持配置的多字符开闭序列。粘贴、显式 ReplaceSelection 和 IME
组合提交保持原文。规则来自 Language.Editing，详见 [扩展契约](DesignLanguageExtensions.md)。键盘、输入法、指针继续走 GOUI 的事件和 IMClient 路径。

- 普通文本支持 `()`、`[]`、`{}` 与引号；JSON 排除圆括号和单引号。
- XML/HTML 还支持 `<>`，引号自动闭合限于属性值起点；`<>` 不等于元素闭合标签。
- 无选区时避开已知字符串、注释、CDATA、声明和不分析的 HTML script/raw-text 正文，
  并检查后继字符，避免在已有词中间插入多余闭合字符。
- 有选区时包裹已选文本，保留内部选区及 Anchor/Caret 方向。
- 仅跳过本视图自动生成、位置和实际文本仍有效的右括号；转义引号继续插入。
- 空配对中 Backspace 一次删除两端。输入配对、包裹、删除分别是一个历史项，
  记录明确的 Before/AfterSelection，撤销和重做不依赖重新执行输入策略。
- 紧邻的 `{}`、`[]`、`()` 中 Enter 插入内层缩进和外层闭合行，光标留在内部空行，
  一次撤销；其他位置沿用基础缩进。此换行行为独立于 AutoPairs 开关。

自动生成标记归视图，随模型 Change.Edits 顺序映射。删除或在任一分隔序列内部插入时丢弃标记；
Reset、切文档、切语言定义和卸载清空标记。最多保留最近 1024 对；较旧字符保留为普通文本。
撤销/重做恢复文本及选区，不恢复已丢弃的“自动生成”身份，避免把外部现有字符当作可跳过字符。

## 语法上下文与显示

`highlight.Engine.ContextAt` 使用可靠的前一逻辑行出状态，加当前行前缀扫描。
前缀最多 16 KiB，保留至多 32 byte lookahead 识别标签起点；若扫描跨过 caret、
前一行状态未收敛或前缀超限，就返回未知。不会同步强迫全文件完成高亮。
字符串、注释、标签名和属性值状态不依赖配色，也不要求 token payload 常驻。
未完成的 `<|>` 作为编辑位置单独识别，不能改变正常高亮对无效文本的规则。

匹配使用当前有效的词法 token，只把 ClassDelimiter 纳入；普通文本直接识别常见括号。
caret 优先检查右侧括号，再检查左侧，按配置序列的最长匹配与类型栈寻找另一端，不能跨过 `[}` 这类错配。
扫描窗口在 caret 两侧各最多 64 KiB，未就绪的 token 请求正常帧分析并推迟显示。
匹配结果按文档身份、修订、caret 和语言定义代次缓存，未知分析结果不缓存成有效“无匹配”。

先在 caret 附近检查配置序列的字节候选（序列最长 128 byte，包含内部位置及两端）。
已知语言上下文且不存在候选时，可直接缓存“无匹配”，不要求整个搜索窗口的 token
就绪；未知嵌入上下文不缓存负结论。发现候选才进入既有 ClassDelimiter 校验和范围
搜索。该检查只排除不可能的匹配，不用字符串外观代替词法角色；避免普通 caret 令
远处 token 缓存反复重载，并使可见区暂时变成“等待早期词法状态”的区域。
候选先只查询所在逻辑行的 ClassDelimiter：XML 声明/注释中的 `<`、字符串中的
括号无需加载其余搜索窗口。该行尚未就绪时正常请求分析，已知非结构角色可缓存
负结论；只有实际结构候选才加载更大的配对范围，不能因排版与高亮独立推进而把
视区反复标成等待状态。

活动且无选区时，只装饰已确认的两端，使用淡背景和 1 DIP 细边框。
未匹配、分析未完成或另一端超出搜索窗口时不显示单端提示，不把局部查询结果当成语法诊断。
失焦、选择文本和预编辑时不显示配对装饰。

几何采用实际 visual-line/Cluster 范围，上下各内缩 1 DIP；边框路径再内缩半个线宽，
保证描边留在字符单元内。宽不超过 1 DIP 或高不超过 3 DIP 的单元不画，避免反向矩形。
跨行、软折行、bidi 和滚动沿用既有几何与裁剪；两端共享同一 shaping Cluster 时去重，
避免半透明装饰叠加。文字在装饰之后绘制，语法前景和布局度量不变，不产生语义交互动作。

配色来自 `scheme.Definition.BracketMatchBackground` 与 `BracketMatchBorder`，
不借用 caret、选区或输入法颜色。两字段分别支持 alpha，透明时独立关闭相应填充或边框；
旧的自定义 Definition 未填写两字段时不显示配对装饰，不从其他颜色推断默认值。

| 配色 | 背景 | 边框 |
| --- | --- | --- |
| Light | `#e6eef7` | `#9aafc4` |
| Dark | `#35414d` | `#667a8d` |

先画当前行与括号淡底，再画搜索/选择背景，最后画括号边框和文字。
搜索背景优先于括号淡底，边框保留配对提示；配色变更走已有 SetScheme/绘制失效，
不建立新的字体布局缓存或系统设置订阅。

## 参考与取舍

2026-10-16 核对以下官方文档与固定版本源码，仅理解行为，不复制实现：

- [GtkSourceView 5 的匹配开关](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/property.Buffer.highlight-matching-brackets.html)
  是显示能力；[5.18.0 gtksourcebuffer.c](https://github.com/GNOME/gtksourceview/blob/5.18.0/gtksourceview/gtksourcebuffer.c#L116)
  有字符数量限制和延迟更新。采用有界分析思路，阈值按本项目的 UTF-8 byte 契约确定，
  不照搬其 Buffer/GTK 文本对象或时间参数。
- [Scintilla 5 括号匹配与装饰](https://www.scintilla.org/ScintillaDoc.html#SCI_BRACEMATCH)
  区分查找配对与显示。采用此职责区分，SourceView 用现有绘制设施，不引入 style-number 协议。
- [Monaco/VS Code 语言配置](https://code.visualstudio.com/api/language-extensions/language-configuration-guide#autoclosing)
  区分闭合与包裹；[VS Code 1.105.0 cursorTypeEditOperations.ts](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/common/cursor/cursorTypeEditOperations.ts#L100)
  的 overtype 使用自动闭合位置，SurroundSelectionOperation 排除正在组合的输入。
  采用显式身份与 IME 边界，首轮使用 Go 编辑计划与有限标记，不建设通用 decoration 追踪子系统。

视觉调整核对 [Monaco/VS Code 的括号背景与边框配置](https://code.visualstudio.com/api/references/theme-color#editor-colors)、
[GtkSourceView 5 配色定义](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/style-reference.html)
中的 `bracket-match`，以及 [Scintilla 的独立括号装饰](https://www.scintilla.org/ScintillaDoc.html#SCI_BRACEHIGHLIGHTINDICATOR)。
采用独立主题色和与语法着色分离的装饰；未采用改字重/字色、彩虹层级或明显的粗下划线。
这些是参考 API 的职责划分，具体颜色与 1 DIP 几何为本项目的视觉选择，不声称复制其默认外观。
