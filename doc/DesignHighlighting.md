# 语言、高亮与增量分析设计

> 状态：首版实现规格。首版采用自有纯 Go 状态扫描器，实际支持 JSON、XML、HTML、TOML、YAML、Shell。
> 算法不创建 GUI/原生资源；调度由已挂载视图驱动。

## 1. 对外语言接口与 token 模型

`language` 包公开 ID、样式分类与只读元数据，内置 Plain、JSON、XML、HTML、TOML、YAML、Shell。
初始通过 Document.SetLanguage 显式设置；可提供纯函数 Guess(filename, sample)，
根据扩展名及有限前缀推测，结果不明确则 Plain。推测不能悄悄覆盖显式语言，
也不自动读取文件、执行内容或下载语法。

首版不公开任意脚本、正则规则解释器或全局 Register API。
内部语言目录将 ID 映射到版本化、不可变的扫描器工厂；将来增加语言不要求改 GUI。
各语言有独立状态类型，不能以一个 XML 开关假装覆盖 HTML。

计划公共接口如下，Kind 的完整常量按后面的分类加 `Kind` 前缀：

```go
package language

type ID string
const (
    Plain ID = ""
    JSON ID = "json"
    XML ID = "xml"
    HTML ID = "html"
    TOML ID = "toml"
    YAML ID = "yaml"
    Shell ID = "shell"
)
type Kind uint8
type Info struct {
    ID ID
    Name string
    Extensions []string
    MIMETypes []string
}
func Lookup(id ID) (Info, bool)
func Guess(filename string, sample []byte) ID
```

Lookup 返回元数据的独立切片副本。Guess 最多检查 4 KiB：已知扩展名优先，
未知时才识别 UTF-8 BOM 后的 XML 声明、HTML doctype/html 起始标记或 JSON 容器前缀；
其余返回 Plain，不据有限前缀承诺文件语法合法。`.xhtml` 作为 XML，HTML 仅对应自身规则。

内部 token 是 `[Start,End) + Kind`，位置相对逻辑行起点，Kind 包括：
Plain、Property、String、Escape、Number、Keyword、Delimiter、Tag、Attribute、
Entity、Comment、Meta、Invalid、Variable。分类是词法样式，不是 LSP semantic token。
Kind 定义在 `language`，供扫描器和 `scheme` 使用，避免公开 API 引用 internal 包类型。
Token 无颜色、字体、Widget 或 Model 引用；按 Start 排序、不重叠、端点是 rune 边界。
相邻同类可合并；未记录部分表示 Plain，LF 不作为行内 token。
XML/HTML 属性值可以拆为 String / Entity，不能用相互覆盖的 token 表达子样式。

## 2. 扫描协议与时间片

内部扫描逻辑由 `初始状态 -> 输入片段 -> token 片段 + 下一状态` 构成。
状态包含当前词法模式及必要上下文，是不可变值或持久结构，不可返回后继续修改。
State.Equal 必须精确比较影响后续扫描的所有信息；hash 仅用于加速，不能作为等价证明。

驱动提供逻辑行内最多 16 KiB 的工作片段、至多 32 byte 的分隔符预读、
本片段行内 offset、是否到达真实 LF、是否到达文档 EOF。
状态能跨片段保存注释、引号、转义及分隔符匹配进度；只有真实行尾才发布该行出状态。
片段边界不是语法行尾，不能把跨片段的转义或结束标记重置。

- 每次非空扫描必须推进输入；暂缓消费只允许保留有限未决分隔符，驱动补足预读后必须推进。
- 从 Model.Slice 读取片段，末端不是 rune 边界时最多退回 3 byte 后重试，不复制整条超长行。
- 默认每个驱动帧执行约 2 ms 扫描工作，片段间检查时钟；这是初始调度参数，非已达性能承诺。
- 任何单个扫描循环也不能无限遍历整行。切换语言/修订后使未完成片段失效，禁止新旧文本拼接。
- 未完成行只保留可安全展示的同修订前缀 token；未知后缀为 Plain，不能补一个猜测出状态。
- Plain 模式无需扫描全文；语法失败不改变文本、历史、选区或输入能力。

持久 JSON 上下文避免每行复制整个嵌套栈。首版嵌套上下文预算为 1024 层；
超过时保留基本字符串/数字/分隔符分类并记录分析降级，不把合法深嵌套直接标成语法错误。
恢复至已保留的上下文边界后再恢复属性分类。预算与退化样本纳入测试。

## 3. JSON 扫描规格

语法基准为 [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259)，首版不混入 JSONC。
本节的状态与恢复是本项目编辑器策略，不是 RFC 对错误输入的要求。

| 状态或上下文 | 行为 |
| --- | --- |
| 值 / 容器 | 分类结构符、数字、true/false/null；对象保存期待属性/冒号/值/分隔符的轻量上下文，数组保存值上下文 |
| 属性字符串 | 在对象期待属性位置开始的引号字符串归 Property；冒号可以出现在下一行，不靠向后读未来行才能决定上一行 token |
| 值字符串 | 双引号定界；合法转义拆为 Escape，非法转义与未转义控制字符局部标 Invalid |
| 行尾 / EOF | JSON 字符串不能跨 LF；未闭合尾部仍可显示为字符串，LF 后退出字符串模式，保留容器恢复上下文 |
| 错误片段 | 非法字面量/数字局部 Invalid；在逗号、括号等位置重新同步，始终推进 |

数字扫描按负号、整数、可选小数和指数分别推进，不把 01、1e+ 等非法片段
误当完整合法数；不借机做 JSON Schema 或对象键唯一性验证。
转义按源字节着色，不解码修改模型；Unicode escape 的语法检查不等于验证所有 Unicode 语义。

测试必须含属性名与冒号分行、转义引号、截断转义、未闭合字符串后的合法新行、
错误容器输入与删除开引号后的前向恢复。不通过“全文 JSON 解析失败即全 Plain”实现高亮。

## 4. XML 扫描规格

基准为 [XML 1.0 Fifth Edition](https://www.w3.org/TR/xml/) 的词法结构。
XML NameStartChar / NameChar 使用该版范围规则，不能以 ASCII `\w` 或一般字母分类代替。

| 模式 | 进入 / 退出及注意点 |
| --- | --- |
| Content | `<` 切换标签/特殊标记，实体引用拆分；普通正文为 Plain |
| StartTag / EndTag | 名称归 Tag，属性名归 Attribute；由 `>` / `/>` 返回正文 |
| AttrSingle / AttrDouble | 引号配对，可跨 LF；内部 `>` 不结束标签，实体单独分类 |
| Comment | 从注释起始标记到结束标记，跨行及跨片段保存匹配进度 |
| CDATA | 精确识别 CDATA 分隔符，内部不解释标签或实体 |
| ProcessingInstruction | 保持引号/结束分隔符所需状态，遇真实终结符退出 |
| Declaration / Doctype | 跟踪引号、注释及内部子集括号，只有处于正确层级的 `>` 才终结 |

实体只识别名称或数字引用的源语法，绝不读取外部实体或网络资源。
声明着色不等于 DTD 验证；未闭合属性/注释/CDATA 保持状态直到修复或 EOF。
非法局部字符可以标 Invalid，不能因为缺少闭合标签就停止分析整个后续文档。
正文与属性中的实体、Unicode 名称、DOCTYPE 引号内的 > / ]、跨行注释均要独立断言。

## 5. HTML 扫描规格与嵌入边界

基准为 [WHATWG HTML tokenization](https://html.spec.whatwg.org/multipage/parsing.html#tokenization)。
首版实现用于着色的词法 profile，不构造 DOM，不宣称完整树构建或浏览器一致性。

- Data、TagOpen/TagName、Before/AfterAttribute、双引号/单引号/未引号属性、Comment、
  Doctype/BogusDeclaration 分别处理。标签和属性的识别按 ASCII 大小写不敏感。
- 布尔属性合法存在；引号属性可跨行；未引号属性由空白或正确终结符结束，错误字符局部恢复。
- 标签名保存有限的候选信息，用于识别特殊正文模式，不积累任意长的原文属性值。
- style/xmp/iframe/noembed/noframes 使用 RawText；title/textarea 使用 RCDATA；
  plaintext 的剩余正文不再按标签扫描。实体只在适用模式分类。
- script 使用 ScriptData 及 escaped / double-escaped 词法子状态，按 HTML 源码边界处理，
  不让 JavaScript 字符串引号决定 HTML 是否退出 script。
- 特殊正文退出时校验对应结束标签名称及后续边界，ASCII 忽略大小写；
  `</scriptx>` 不能结束 script。行尾不是自动退出条件。
- 首版不在 script/style 正文内做完整 JS/CSS tokenization；正文用 Plain，
  但必须保留正确的 HTML 结束边界。将来内层扫描器不能压制外层终结。
- SVG/MathML 命名空间、模板语法、完整实体表和解析错误报告不在此 profile 中；
  外来内容按已声明的保守标记策略呈现，不能称完整 HTML parser。

本项目必须按上述 profile 写边界样本，不能原样照搬另一个编辑器的 HTML 正则列表。
例如普通 ScriptData 中的 JavaScript 字符串 `"</script>"` 仍含 HTML 终结序列，
而 escaped/double-escaped 模式需要各自规则；这正是“嵌入语言边界”与“JS 字符串”分离的原因。

## 6. 逻辑行元数据

共享 lineStore 使用有累计 byte 长度/行数的分块平衡序列，目标叶块约 128 项，
支持按 byte offset 找旧行、行区间 splice 和按行遍历。不是另一份文本或 Piece Tree；
不存正文，只存每行长度（含是否有 LF）、稳定 LineID、内容版本及分析结果引用。

原因：Change 通知的模型已是新版本，且回调可能看到更晚修订，不能再询问 Model
来获取旧范围所在行。旧 lineStore 必须先逐项解释 Change.Edits：

1. 在旧元数据中定位 Start/End 的行与行内位置。
2. 从 Edit.Text 的 LF 分割计算新行长度，结合旧前缀/后缀长度，splice 受影响行。
3. 未修改的后缀 LineID 保留；修改/合并/拆分的行建立新内容版本，清理旧 token/布局引用。
4. 维护失效区间及对应 byte delta，变更只向后平移未修改 token 的全局位置，不逐项重写行内 token。
5. 全部通知追上模型修订后，才用 Model.Slice 读取当前待分析片段。

起步、Reset 或修订断层时用分块文本读取重建元数据；重建为可分帧工作，
索引尚不可用时视图可从 Model 的逻辑行 API 获取当前可见内容。
重建绑定特定修订；中途再次编辑时放弃该次未完成索引并按最新修订重新安排，
不能把针对不完整旧索引的局部 splice 当成有效更新。此期间保持输入和 Plain 显示可用。
空文档是一行，末尾 LF 后还有一行，行内长度不包含分隔 LF。
索引与 Model 对齐是独立断言，不只比较两份高亮结果。

## 7. 增量传播算法

每行有效分析记录至少含 `LineID、ContentVersion、LanguageEpoch、InState、OutState、
TokenVersion、Complete`；行号不是身份。另维护有序 dirty 区间，不能只有一个
“从某行到 EOF 全脏”的布尔值而丢失后续独立修改。

- 从 dirty 行之前最近有效出状态恢复；前文未知时向前找到有效锚点或文首初始状态。
- 依次扫描并发布当前行的新 token。旧 token 即使当前文字未变，入状态改变后也不再有效。
- 仅当当前行新出状态精确等于旧的有效出状态，且下一行身份/内容版本仍对齐，
  才能在这一段停止传播；之后仍有 dirty 区间则继续处理其起点。
- 若旧状态已丢弃或行未完成，不能以 hash、行号或相同文本独自证明收敛。
- 插行/删行通过 lineStore 对齐后缀；不能将旧第 N 行的状态直接交给新第 N 行。
- 语言切换使所有旧状态/token 失效并取消未完成扫描；文本未变时保留 LineID 和行长度，不将语言代次变更混成文本身份变更。Plain 无需制造全文 token。
- token 缓存淘汰不代表行状态失效；可依据保留入状态重扫可见行恢复 token。
- 预编辑不参与共享分析；正式提交后才按普通模型变更失效。

例：XML 中删掉某行注释起始符，可能影响直到原注释终结后的状态；
若在后面另有一次独立修改，前一段收敛不能跳过后一段。
本算法参考 [VSCode 1.104.3 状态存储](https://github.com/microsoft/vscode/blob/1.104.3/src/vs/editor/common/model/textModelTokens.ts#L216) 的状态传播思想，
行身份、索引、调度和缓存布局由本项目自行实现。

## 8. 调度、缓存与发布

Document 有已挂载视图时选一个活跃 Root 作为分析驱动者：
连接其 ConnectFrame，RequestPaint 请求后续工作帧；驱动者离开时移交，
旧回调由 epoch 失效。只在可见且存在未完成分析时持续请求帧。
隐藏或最后一个视图卸载时暂停，不用自旋 Post 保持后台全文分析。
同一 Root 的分析请求由一个内部队列汇合，初始总预算 4 ms、单 Document 配额约 2 ms，
轮转服务多个 Document，不能让每个视图各花 2 ms 而把帧时间放大为视图数倍。
队列只在还有挂载使用者时持有帧连接，空队列不主动请求新帧；
正常显示/布局帧重新检查有效可见性，恢复此前隐藏而暂停的文档工作。

优先满足可见行，但不能跳过未知前置状态。冷启动直接跳到文件尾时先以 Plain 显示
未知区域，按有效检查点补齐分析；不得从任意可见行“假定根状态”制造错误颜色。
已知入状态的可见行 token 恢复优先于不可见 token 的存储。

Token payload 首版 LRU 目标 16 MiB，超长行以片段缓存；
行元数据与出状态 O(行数)，另统计持久栈节点的内存，不能将其漏出缓存测量。
当前可见工作集可以钉住；超预算时记录实际占用并淘汰不可见 payload，
不无限保留每个历史修订。

发布结果同时核对 Document、模型修订、LanguageEpoch、LineID/ContentVersion 和任务代次。
同修订的 token 变化向视图发内部范围失效；不会发文本 Change 或改编辑历史。
若词法实现违反推进/范围契约，停止该工作项、丢弃错误 token、报告一次可定位的分析失败，
保留纯文本编辑；不每帧重复报错。

## 9. TOML、YAML 与 Shell

2026-10 扩展内置 `language.TOML`、`language.YAML`、`language.Shell`，不新增依赖或
全局规则注册。沿用同一 Document、行身份、调度与 token 缓存；扫描分别在
`internal/highlight/toml.go`、`yaml.go`、`shell.go`，不借用 XML 状态。

### 9.1 规范及参考取舍

词法依据 [TOML 1.1.0](https://toml.io/en/v1.1.0)、
[YAML 1.2.2](https://yaml.org/spec/1.2.2/) 与
[Bash 5.3 引用规则](https://www.gnu.org/software/bash/manual/html_node/Quoting.html)、
[重定向规则](https://www.gnu.org/software/bash/manual/html_node/Redirections.html)。
这些是语法依据，不代表控件执行配置或 Shell，也不承诺完整语法验证。

参考源代码为 Lexilla `rel-5-4-5` 的
[LexTOML](https://github.com/ScintillaOrg/lexilla/blob/rel-5-4-5/lexers/LexTOML.cxx)、
[LexYAML](https://github.com/ScintillaOrg/lexilla/blob/rel-5-4-5/lexers/LexYAML.cxx)、
[LexBash](https://github.com/ScintillaOrg/lexilla/blob/rel-5-4-5/lexers/LexBash.cxx)，
Monaco `v0.55.1` 的
[YAML](https://github.com/microsoft/monaco-editor/blob/v0.55.1/src/basic-languages/yaml/yaml.ts)、
[Shell](https://github.com/microsoft/monaco-editor/blob/v0.55.1/src/basic-languages/shell/shell.ts)，
以及 GtkSourceView `5.18.0` 的
[sh.lang](https://gitlab.gnome.org/GNOME/gtksourceview/-/blob/5.18.0/data/language-specs/sh.lang)、
[yaml.lang](https://gitlab.gnome.org/GNOME/gtksourceview/-/blob/5.18.0/data/language-specs/yaml.lang)。
以上规则与本项目扫描器均属平台无关的词法层，不据此推断 GUI 后端表现。

采用显式跨行状态、嵌套引用恢复和 string/comment 编辑上下文的思路；不引入原生依赖、
XML 规则解释器或通用正则状态机。YAML 块文本按实际缩进退出，Shell 多个 heredoc
按所属命令上下文排队，不以单条结束正则代替状态，也不照搬固定长度 delimiter 限制。

### 9.2 分类与边界

| 语言 | 分类与状态 |
| --- | --- |
| TOML | 裸键、带引号键、点分键和表名归 Property；表、数组、内联表结构归 Delimiter；四种字符串形式分别保留引号/多行状态；基本字符串转义含 1.1 的 `\e`、`\xHH`；布尔归 Keyword，数字归 Number，日期/时间归 Meta，`#` 注释归 Comment |
| YAML | 普通映射键由随后有效冒号确认 Property；引号标量归 String；Core 布尔/null 归 Keyword，数字归 Number；`on/off/yes/no` 保留 Plain；锚点、别名、标签、指令和文档边界归 Meta；流式结构归 Delimiter |
| YAML 块文本 | `|`、`>` 保存父缩进、可选缩进数字和首个非空内容行的缩进；支持 chomp 标记；空行不中断块；缩进不足的下一行恢复 YAML；序列中映射键的列参与父缩进计算 |
| Shell | 保留字归 Keyword，变量/赋值名归新增 Variable；普通词保持 Plain；单引号、双引号、ANSI-C 引号分别处理反斜杠；命令/算术/参数替换及反引号使用持久上下文栈，结束后恢复外部引号 |
| Shell heredoc | `<<`、`<<-` 读取去除引用后的 delimiter，任一引用抑制正文变量着色；`<<-` 只剥离前导 TAB；同一命令的多个文档按顺序读取，转义换行延后开始；结束行必须完全匹配；`<<<` 与算术左移不创建 heredoc |

这是编辑期的容错词法 profile：不校验 TOML 数据类型/日期取值、YAML schema 或标签解析，
不展开 YAML alias，不构造 Shell AST，不执行命令、不查 PATH。Shell heredoc 正文提供
变量着色，但不递归解析正文中命令替换的完整 Shell 语法；Shell 的完整上下文敏感语法、
YAML 复杂显式键的语义着色及配置校验不在本切片中。

### 9.3 分片及状态所有权

长键只有遇到冒号才能确认分类，heredoc 的结束行也只能在实际行尾确认。
内部 `Scan` 允许返回起点为负的 token：位置仍相对本片段，但只能回溯当前逻辑行，
终点不越过本次已消费输入。`appendToken` 撤回重叠的已发布范围再写入新分类；
引擎检查范围并增加行 paint version，着色修正不触发文本修改、历史或重新排版。
原有 JSON/XML/HTML 仍只需向前发布 token，调用方统一使用该合并操作。

普通 literal 只保存有限的候选拼写与流式数字状态，另记起始长度；不累积整条 YAML 键。
Shell delimiter 的拼写按不可变片段保存，结束后建立不可变片段序列，正文逐字节匹配；
无固定 delimiter 截断，也不为判断结束行读取整条超长行。
嵌套栈与 delimiter 可由同文档的行状态共享，扫描只复制当前状态及待处理队列，
绝不修改旧行快照。`State.Equal` 比较完整值/栈/队列/拼写，不以地址或哈希充当语义相等。
状态的所有者仍为 Document，引擎重建、编辑失效和文档销毁时由 Go 回收不再引用的数据；
token 的 16 MiB 预算不包含状态引用的 delimiter 拼写，此类状态随输入大小增长。

### 9.4 语言识别

已知扩展名优先：`.toml`、`.yaml/.yml`、`.sh/.bash`。Shell 另识别
`.bashrc/.bash_profile/.bash_login/.bash_logout/.profile`，以及位于 byte 0 的
`sh`/`bash` shebang（含常用 `env`、`env -S` 写法）。最多读取调用方的 4 KiB 样本，
不 I/O、不执行 shebang。未知文件不凭 `key=value` 或 `key: value` 猜 TOML/YAML，
`.env` 不自动视为 Shell；显式 SetLanguage 不受 Guess 影响。
