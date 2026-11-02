# 语言、高亮与增量分析设计

> 状态：实现规格。采用自有纯 Go 状态扫描器；内置语言及各自边界见本文专题。
> 算法不创建 GUI/原生资源；调度由已挂载视图驱动。

## 1. 对外语言接口与 token 模型

`language` 包公开不可变语言定义、Token/State/Tokenizer 和编辑规则，内置
Plain、JSON、JSONC、JSONLines、XML、HTML、TOML、YAML、Shell、Markdown，以及下文的配置、构建与编程语言；任意定义直接传给 Document，
不使用全局注册。完整接口、样式/语义分离和扫描校验见 [扩展契约](DesignLanguageExtensions.md)。

Document.SetLanguage(id) 是内置便利入口；SetLanguageDefinition(l) 按对象身份绑定。
Guess(filename,sample) 是纯推测函数，不自动覆盖显式语言、不读取文件或下载语法。
Lookup 返回内置元数据的独立切片副本；Builtin 返回不可变内置定义。
Guess 最多检查 4 KiB，专用文件名优先于已知扩展名，未知再识别有限前缀；`.xhtml` 按 XML。

Kind 为可扩展字符串样式键，内置 Plain、Property、String、Escape、Number、Keyword、
Delimiter、Tag、Attribute、Entity、Comment、Meta、Invalid、Variable；不是 LSP semantic token。
Token 无颜色、字体、Widget 或 Model 引用。Class 标志独立描述字符串、注释和结构符，
不因为更换 Kind/颜色而改变编辑语义。坐标相对逻辑行，rune 边界、有序、不重叠，
相邻同 Kind 且同 Class 可合并，LF 不进入 token；未记录区域保持普通前景。
XML/HTML 属性值拆为 String/Entity，不能用重叠 token 表达子样式。

## 2. 扫描协议与时间片

公开 Tokenizer 的扫描逻辑由 `初始状态 -> 输入片段 -> token 片段 + 下一状态` 构成。
状态包含当前词法模式及必要上下文，是不可变值或持久结构，不可返回后继续修改。
State.Equal 必须精确比较影响后续扫描的所有信息；hash 仅用于加速，不能作为等价证明。

驱动提供逻辑行内最多 16 KiB 的工作片段、至多 32 byte 的分隔符预读、
本片段行内 Offset、LineEnd 和 DocumentEnd。Result 坐标相对当前逻辑行；
延迟分类通过 ReplaceFrom 显式回撤当前行前缀，再应用新 Tokens，不使用负位置。
状态能跨片段保存注释、引号、转义及分隔符匹配进度；只有真实行尾才发布该行出状态。
片段边界不是语法行尾，不能把跨片段的转义或结束标记重置。

- 每次非空扫描必须推进输入；暂缓消费只允许保留有限未决分隔符，驱动补足预读后必须推进。
- 从 Model.Slice 读取片段，末端不是 rune 边界时最多退回 3 byte 后重试，不复制整条超长行。
- 每个 Root 复用一个 GUI Timer，每 8 ms 最多推进约 2 ms 纯分析，片段间检查时钟；
  错过的周期由 GOUI 丢弃，不追赶。该软预算不包含单次不可中断操作的超时承诺。
- 任何单个扫描循环也不能无限遍历整行。切换语言/修订后使未完成片段失效，禁止新旧文本拼接。
- 未完成行只提供同修订的正确性 token 前缀，不能补一个猜测出状态；显示颜色的过渡另见 7.1。
- Plain 只建立逻辑行元数据，不执行 tokenizer；语法失败不改变文本、历史、选区或输入能力。

元数据 lineStore 的子树累计包含 byte 数量。偏移／行号查询跳过整个块时使用累计值，
只在最终命中的至多 128 行块内扫描；不在树的每个分支重复扫描整块。一次扫描片段
只计算一次逻辑行的模型起点，token 和 ReplaceFrom 的 UTF-8 校验复用该值；保留全部
边界断言与跨行状态，不借性能优化省略语法校验。对照见 [真实文档验证](PlanCorpusLayout.md)。

分析与绘制分开：不为屏外每个 chunk 请求整窗 Paint。元数据就绪或可见段落的 token
版本／有效性改变时，才请求相应视图绘制；未变的可见 token 继续使用已有排版与颜色。
原生正文准备仍走 Root 的帧回调。Timer 回调不测量字体、不创建或修改 TextLayout，
不启动测量线程。Document 沿用单个驱动 Root，多个文档按轮转顺序分享时间片。

Timer 属于现有 frameQueue，首次挂载创建；无待办时停止，最后视图卸载时 Stop 并断开
连接，过期回调核对当前挂载／Root。不可启动 Timer 的宿主保留原帧推进路径，不丢工作。
折叠仍沿用现有纯快照 worker，快照创建自身尚不可中断；没有新增公共调度接口。

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

### 3.1 JSON Lines

`language.JSONLines`（ID `jsonlines`）表示 [JSON Lines](https://jsonlines.org/)，
自动识别 `.jsonl/.ndjson`。每个逻辑行是独立 JSON 值，根值可以是对象、数组、字符串、
数字、布尔或 null；不是一个跨行数组，也不接受 JSONC 注释。

JSON、JSONC 与 JSON Lines 共享 `scanJSON`：JSONC 开启注释；JSON Lines 在实际逻辑行尾
且整个传入窗口已消费时恢复初始状态，包括容器栈和过深嵌套状态。
片段末尾、预读范围和视觉折行都不能结束记录。即使上一行缺少结束括号或引号，
下一行仍从根值开始，不能把根字符串误当上一对象的属性；插入/删除 LF 后按新记录边界增量重扫。
JSON 原有的跨行容器和 JSONC 的跨行注释保持各自行为。

配色、结构符与引用/补全规则复用 JSON；JSON Lines 不提供跨记录结构折叠或注释命令。
Markdown 默认围栏支持 `jsonlines/jsonl/ndjson`，显式自定义围栏表仍由调用方决定。
HTTP 正文显式声明 `application/jsonl` 或 `application/x-ndjson` 时使用同一定义；
这些 MIME 字符串用作文件类型约定，不宣称为统一标准注册值。

这是编辑期高亮，不新增 JSON Lines 校验器：空行、BOM、多个值或损坏记录仍可编辑，
不把整个文档交给严格解码器后才显示。UTF-8、CRLF/LF 及最后一行无终止换行的文本处理
沿用 Document/Model 契约；源码中的 `\\n` 转义不会产生新的逻辑行。

2026-10-27 核对 [Monaco v0.52.2 JSON 配置](https://github.com/microsoft/monaco-editor/blob/v0.52.2/src/language/json/monaco.contribution.ts)
的 `allowComments` 和 [Lexilla JSON lexer](https://github.com/ScintillaOrg/lexilla/blob/master/lexers/LexJSON.cxx)
的 `lexer.json.allow.comments`：两者均在 JSON 体系内提供变体配置，支持复用核心的选择；
它们不是本项目按行恢复策略的依据，该策略直接来自 JSON Lines 的独立记录约束。
不引入这些框架的解析、Schema 或原生依赖。

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
- script/style 正文复用 JavaScript/CSS 扫描器；内层扫描窗口止于外层特殊标记，
  即使结束标签位于内层字符串/注释中也退出。无 type、JS MIME 或 module 的 script
  使用 JS，无 type 或 text/css 的 style 使用 CSS；其他显式 type 保持 Plain。
  type 只保留最多 65 byte 候选信息，不积累任意属性值，也不解码实体猜测脚本类型。
- SVG/MathML 命名空间、模板语法、完整实体表和解析错误报告不在此 profile 中；
  外来内容按已声明的保守标记策略呈现，不能称完整 HTML parser。

本项目必须按上述 profile 写边界样本，不能原样照搬另一个编辑器的 HTML 正则列表。
例如普通 ScriptData 中的 JavaScript 字符串 `"</script>"` 仍含 HTML 终结序列，
而 escaped/double-escaped 模式需要各自规则；这正是“嵌入语言边界”与“JS 字符串”分离的原因。

标签完成后释放其属性候选信息，普通结束标签和自闭合标签返回初始正文状态；
新标签也重新建立属性上下文，避免分片恰好止于 `>` 时与一次完整扫描产生不同出状态。

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

### 7.1 编辑期间的稳定着色

词法正确性和显示颜色分别读取。`Engine.Tokens` / `ContextAt` 继续拒绝前置状态未确认的结果，
括号匹配、自动配对、补全等不能使用过期 Class/状态。internal Engine 的 `PaintTokens` 仅供正文绘制：
同一定义、同一行身份的后续未改文本可暂用原颜色，不能因重算待办就先清成普通前景色。
语言定义按对象身份隔离；切换语言/同 ID 的新定义或全文重置后不展示旧定义的快照。

修改行按旧坐标的 Edit 映射颜色：保留前缀/后缀的相交范围，删除范围不留颜色，换行拆分、合行平移；
仅当纯插入严格位于旧 token 内时，插入文字继承该显示颜色。替换或边界不明区域暂用普通色。
保留的数据属于当前文本坐标下的显示提示，不赋予词法有效性，也不存另一份正文。

每个 Engine 最多一个工作行，新 token 前缀写入独立缓冲；有旧显示快照时，完成整行后一次替换，
相同 token 结果沿用显示版本。首次分析或快照已淘汰时仍逐片展示新前缀，未知后缀为普通色。
连续编辑取消旧工作缓冲并映射当前快照；没有快照时可保留已验证的前缀作为显示提示。
取消时保留该行待办，较早行的缓存重载不能跳过未完成分析。没有新增线程或同步全文扫描。

行快照和工作缓冲共同计入现有 16 MiB token payload 目标，最多保留一份显示快照，
不为每次输入保留修订历史。预算压力先淘汰其他 LRU payload，仍超限时可淘汰当前旧快照，
回到逐片显示的新前缀。单行条目上限、出状态与既有错误处理继续生效。
Paint-only 版本用于颜色缓存，词法就绪状态仍参与装饰刷新；实际颜色未变不调用原生重新着色。

参考 [VS Code 1.95.3 token 编辑映射](https://github.com/microsoft/vscode/blob/1.95.3/src/vs/editor/common/tokens/contiguousTokensStore.ts#L150)
的 `acceptEdit`、删除/插入范围处理（2026-11-02 源码核对）。采用保留并调整显示范围的思路，
行身份、不可变状态、单工作缓冲和预算由本项目实现；不复制其对象体系或引入语言服务依赖。

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

Token payload 的 LRU 目标为 16 MiB，显示快照与工作前缀共同计入，过渡规则见 7.1；
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
`language/scan_toml.go`、`scan_yaml.go`、`scan_shell.go`，不借用 XML 状态。

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
`sh`/`bash` shebang（含常用 `env`、`env -S` 写法）。识别 `env` 时跳过已知选项及
其参数，不把 `-u bash` 的变量名或 `--chdir bash` 的目录当解释器；未知选项保守返回 Plain。最多读取调用方的 4 KiB 样本，
不 I/O、不执行 shebang。未知文件不凭 `key=value` 或 `key: value` 猜 TOML/YAML，
`.env` 不自动视为 Shell；显式 SetLanguage 不受 Guess 影响。

## 10. 配置、请求与构建文件

JSONC、HTTP、Makefile、CMake、Nginx、INI、Dockerfile 复用现有 Token/State 协议，
不引入外部词法库、运行工具链或新的 GOUI 接口。编辑规则提供注释、配对及静态词汇，
不是配置校验、构建执行、请求发送或语义补全。

| 语言 | 首次支持的词法范围与约束 |
| --- | --- |
| JSONC | 沿用严格 JSON 的属性、数字、字符串规则，额外接受 `//`、`/* */` 和尾逗号；不是 JSON5。JSON 保持原有严格规则。JSONC 暂不绑定严格 JSON 的折叠函数，避免把注释中的括号当结构。 |
| HTTP | `.http/.rest` 请求文档：方法、URL、HTTP 版本、头名称、`@name` 与 `{{name}}` 变量。`###` 外层分隔请求；显式 Content-Type 为 JSON/XML/HTML 时复用对应正文扫描器。未知正文仅处理基础变量，不推测协议语义。 |
| Makefile | 目标/赋值名称、指令、续行、`$()`/`${}`、自动变量与 `$$`。识别 TAB 和 `.RECIPEPREFIX` 配方入口，配方不按 Make 指令着色；不是完整 shell AST 或 Make 展开器。 |
| CMake | 命令、`${}`/`$ENV{}`/`$CACHE{}`、生成器表达式、引用参数、长括号参数及注释。长括号使用等号数量与逐字符结束匹配，不截断 delimiter。 |
| Nginx | 指令、参数、块、变量、引号与 `#` 注释。指令位置由 `{ } ;` 确定，不依赖安装了哪些模块。 |
| INI | 通用编辑 profile：节名、键/值、引号、数字、整行 `;`/`#` 注释；不承诺某一程序的插值、转义、重复键或内联注释语义。 |
| Dockerfile | 指令、选项、变量、引号、JSON 形式的结构、续行及 escape 指令；heredoc 复用 Shell 的不可变 delimiter 和队列，支持多个、引用及 TAB 剥离，不执行 shell。 |

专用文件名先于扩展名：`CMakeLists.txt`、`Makefile/makefile/GNUmakefile/Makefile.*`、
`Dockerfile/Dockerfile.*`、`nginx.conf/*.nginx.conf`；其余已知扩展名优先于内容。
`.conf` 不普遍映射 Nginx，未知或含糊文件仍可显式选择语言。识别仍只读取调用方最多 4 KiB 样本。

配置扫描器只共享流式词、引用与变量的机械处理；语句、物理行和正文切换分别维护。
词只保留最多 128 byte 候选拼写与完整范围，超长 token 不积累整个字符串；
后续确认分类通过当前行 ReplaceFrom 发布。状态不可变，Equal 比较完整语义值；
Docker heredoc 的存储边界与 Shell 相同。Markdown 内置围栏表包含这些语言，无需注册。

参考与取舍（2026-10-27 查阅）：[JSONC](https://jsonc.org/) 将尾逗号作为可选能力，
本编辑 profile 接受；[REST Client](https://github.com/Huachao/vscode-restclient) 作为请求文档惯例，
不是 HTTP 标准的文件格式承诺；[GNU Make](https://www.gnu.org/software/make/manual/make.html)、
[CMake language](https://cmake.org/cmake/help/latest/manual/cmake-language.7.html)、
[Dockerfile](https://docs.docker.com/reference/dockerfile/)、
[Nginx 配置结构](https://nginx.org/en/docs/beginners_guide.html) 用于核对各自边界。
沿用 [GtkSourceView 5 语言上下文](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/lang-reference.html)
的状态分解思路，但使用本项目的 Go 扫描协议，不导入其语法资源或原生依赖。

## 11. 编程语言与 CSS

内置 `SQL`、`Protobuf`、`Go`、`Python`、`JavaScript`、`TypeScript`、`CSS` 常量与元数据。
这些语言使用带明确词法模式的 Go 扫描器；JS/TS 共享机械逻辑、分别维护词汇。
不复制原生编辑器对象，不引入解释器、编译器、AST 或 LSP 依赖。

| 语言 | 词法范围 |
| --- | --- |
| Go | 关键字、预声明类型/常量、常见数值（含基数、指数、虚数）、rune、转义字符串、反引号原始字符串、两种注释。 |
| Protobuf | proto2/proto3/edition 声明词汇、类型、字段序号、数值、单双引号字符串、两种注释；不解析引用类型或验证字段编号。 |
| Python | 关键字及常用软关键字、数值、装饰器、字符串前缀、单双/三引号、raw/bytes/f/t 字符串；插值表达式使用代码状态，支持嵌套引用与双花括号。格式规格部分不构造完整 Python 格式语法树。 |
| JavaScript | 关键字、数值、字符串、模板及嵌套 `${}` 表达式、注释、正则及字符类/转义/flags。正则/除法使用表达式起点和控制语句圆括号上下文，不承诺错误源码或所有 ASI/对象字面量歧义的完整语法解析结果。 |
| TypeScript | JS 词法加类型/声明关键字；类型推断、泛型语义、JSX/TSX 不在此 profile 中，`.tsx/.jsx` 不自动映射。 |
| SQL | 通用大小写不敏感词汇、数值、单双引号、反引号/方括号引用、参数、`--`、嵌套块注释和 PostgreSQL 风格 dollar quote。是综合编辑 profile，不声称完整实现任一数据库方言，也不执行或校验查询。 |
| CSS | 选择器、at-rule、声明属性和值、数值/单位/颜色、自定义属性、函数、引用及块注释；group rule 保持嵌套块上下文，不按 SCSS/Less 插值规则处理。 |

每次只扫描驱动提供的有界窗口，标识符只保留 128 byte 候选拼写及完整范围。
短引号、块注释、正则字符类、模板插值、SQL dollar quote 各有状态；只有真实行尾
执行行尾恢复。模板/控制圆括号/CSS 块使用不可变节点，Equal 比较内容而非节点地址。
SQL dollar delimiter 采用不可变拼写片段，结束后形成匹配拼写，存储随 delimiter 长度增长，
不将完整正文放入状态。编辑后仍由既有行入/出状态失效传播，不重新整篇扫描才能显示。
ClassString/ClassComment 与颜色独立，插值表达式恢复代码上下文；编辑、配对及补全
使用同一词法上下文。新增语言默认提供词汇建议和适用的注释/引用规则，未新增通用折叠器。

自动识别 `.sql/.proto/.go/.py/.pyi/.pyw/.js/.mjs/.cjs/.ts/.mts/.cts/.css`；
Python 另识别 byte 0 的 `python` 或带数字版本的 shebang，沿用 Shell 的 env 参数解析。
Markdown 的默认围栏表包含全部内置非 Markdown 语言及 `js/ts/py/golang/proto/make/docker` 等别名；
`NewMarkdown(nil)` 与内置 Markdown 共用同一表，显式非 nil 表仍只使用调用方的映射。

依据与取舍（2026-10-27 查阅）：[Monaco v0.52.2 JS](https://github.com/microsoft/monaco-editor/blob/v0.52.2/src/basic-languages/javascript/javascript.ts)
复用 [TS tokenizer](https://github.com/microsoft/monaco-editor/blob/v0.52.2/src/basic-languages/typescript/typescript.ts)，
支持本项目共享 JS/TS 扫描器但分离词汇的选择；其词法状态不是完整 TS parser。
[Lexilla Python 源码](https://github.com/ScintillaOrg/lexilla/blob/master/lexers/LexPython.cxx)
用于核对引用与插值需要独立状态，未复制词法表或源码。
语言边界分别核对 [Go spec](https://go.dev/ref/spec#Lexical_elements)、
[Python lexical analysis](https://docs.python.org/3/reference/lexical_analysis.html)、
[Protobuf spec](https://protobuf.dev/reference/protobuf/proto3-spec/)、
[PostgreSQL 17 lexical structure](https://www.postgresql.org/docs/17/sql-syntax-lexical.html)、
[CSS Syntax Level 3](https://www.w3.org/TR/css-syntax-3/)。SQL 参考 PG 的引用机制，但不把整个综合 profile 宣称为 PG 兼容解析器。

## 12. C、C++、Java、Rust、C#、Lua

新增 `language.C/Cpp/Java/Rust/CSharp/Lua`，沿用 `Builtin`、`Document.SetLanguage` 和
声明式 Language 属性，不增加注册中心、解析器依赖或 GOUI 接口。
各语言分别提供元数据、扫描器、注释规则、静态词汇和默认 Markdown 围栏。
使用现有 Keyword/String/Escape/Number/Comment/Meta/Variable/Delimiter 配色，
ClassString/ClassComment/ClassDelimiter 决定编辑行为，不从颜色或标识符首字母猜测语义类型。

| 语言 | 编辑期词法范围与取舍 |
| --- | --- |
| C | C23 常用关键字和保留字、数值及后缀、字符/字符串编码前缀、两种注释；行首预处理指令名用 Meta，include 的引用头文件用 String。保留反斜杠续行中的短字符串、行注释和指令状态；移除末尾反斜杠时按实际字符调整引用转义奇偶。 |
| C++ | 独立词汇表，包含协程、concept/requires 等词汇；复用 C 普通词法，增加数字分隔符、原始字符串和编码前缀。原始 delimiter 最多 16 个合法字符，正文仅在完整 `)delimiter"` 结束。 |
| Java | SE 25 关键字及常用上下文词汇、注解、数值、字符、普通字符串和三引号 text block；Unicode 转义先于词法处理，可以生成关键字、引号、注释或虚拟换行。遵守反斜杠资格与非递归转换规则；连续 u 只计状态，四位 hex 和 UTF-16 surrogate pair 转为逻辑字符，token 坐标仍指向原始 UTF-8 文本。 |
| Rust | 关键字、原始标识符、生命周期、字符/byte 字符串、普通/C/raw/byte raw/C raw 字符串、嵌套块注释；raw delimiter 记录 0–255 个 `#`，逐字符匹配结束。生命周期按 Variable，宏调用名称按 Meta；不展开宏或解析类型。 |
| C# | 关键字及常用上下文词汇、`@identifier`、数值、字符、普通/逐字/插值字符串、raw string。raw delimiter 的引号数量和 `$` 数量分别决定结束与插值括号数量；多行 raw 结束符需在行首空白之后。插值使用不可变栈恢复外层引用，表达式内独立处理注释、字符串、结构括号和条件表达式；冒号格式段按 String，不构造完整格式语法树。行首预处理指令按 Meta，不执行条件编译。 |
| Lua | Lua 5.4 关键字、常见十进制/十六进制数值、短引用和转义、`\\z` 跨行空白、行注释、长字符串/长注释。长括号记录等号数量，关闭时严格匹配同一层级；不设置任意 delimiter 长度截断。 |

这是词法编辑 profile，不是编译器：不评估 C/C++/C# 条件编译，不检查数值后缀组合、
text block/raw string 的完整缩进合法性或上下文关键字的语法位置，不承诺 C/C++
跨物理行拼接形成新 token 的全套预处理变换、trigraph 或 Java 预览版字符串模板。
普通源码续行和多行引用有明确状态；错误输入按物理行或已经确认的多行结束符恢复。
颜色及 Class 不等同于编译诊断，也不新增通用折叠器、类型推断或语言服务。

### 12.1 扫描与生命周期

`scan_c.go` 共享 C/C++ 机械逻辑和独立词汇；Java 的 Unicode 转换、Rust、C#、Lua
各自维护状态，不向既有 JS/SQL/Python 的状态结构继续添加无关语言字段。
复用 `scanLimit`、`scanWord` 和 `tokenWriter`；扫描仍采用驱动的 16 KiB 工作片段及
最多 32 byte 预读，没有整行正则或完整正文副本。

标识符保留最多 128 byte 候选拼写及完整范围；C++ delimiter 按规范有界，Rust 只存 hash 数，
Lua 只存等号数，C# 只存引号/美元/括号数及插值栈，Java 只存转换进度及待配对 surrogate。
正文长度不增加这些状态的存储。C# 栈随语法嵌套深度增长，由行缓存的状态快照持有；
节点不可变，Equal 比较节点内容，可跳过已经共享的尾部。语言定义为只读共享对象，
每个 Engine 拥有自己的词法状态、token 缓存和失效范围，不跨文档共享可变分析结果。
仍通过既有行状态收敛和事件循环预算增量发布；本批不修改调度或原生排版。

尚未确认的长 opener 可暂用候选样式，确认后通过同一逻辑行的 ReplaceFrom 修正；
不会跨行回写 token。只有真正消费到物理行尾时才执行行尾恢复，不把预读范围等同于已消费范围。
插入/删除开闭符号由既有行入/出状态传播处理，状态键包含所有影响后续扫描的值。

### 12.2 接入与编辑规则

识别 `.c/.i`；`.C` 专门识别为 C++，其余 C++ 后缀包括 `.cpp/.cc/.cxx/.c++/.hpp/.hh/.hxx/.h++/.ipp/.inl/.ixx/.cppm/.ii`。
含糊的 `.h` 默认 C++，调用方可显式选择 C。新增 `.java/.rs/.cs/.csx/.lua`，
Lua 另识别 lua/luajit 及数字版本 shebang。已知扩展名仍优先于内容，Guess 不读取文件。
Markdown 识别规范 ID 及 `c++/cc/rs/cs/c#` 别名，显式自定义围栏表不被扩充。

这些语言默认使用 `()[]{}` 结构配对；C++/Java/Rust/C# 不猜测泛型尖括号。
Rust 禁用单独输入 `'` 的自动闭合，选中后以单引号包围仍可用，避免破坏生命周期输入。
Java/C# 输入第三个双引号时不额外生成闭合引用；引用和注释中的补全、配对继续由词法上下文约束。
Lua 提供 `--` 与 `--[[ ]]` 注释规则；其余使用 `//` 与 `/* */`。
每个语言使用独立原创示例，editor 共 28 份内置样本；不调用编译器或执行样本。

### 12.3 依据与验证范围

2026-11-02 核对：[GtkSourceView 5（5.22.1）语言上下文](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/lang-reference.html)
支持按引用、注释等状态组织规则的思路；
[Monaco v0.52.2 C# 源码](https://github.com/microsoft/monaco-editor/blob/v0.52.2/src/basic-languages/csharp/csharp.ts)
将普通、逐字和插值引用分开处理；该版本的规则没有覆盖现代 raw string 的完整语法，
本项目依据语言规范自行实现，不直接移植其表。
[Lexilla Rust 源码](https://github.com/ScintillaOrg/lexilla/blob/1666f9bff3c13d428914adb449b97cfce8d58f16/lexers/LexRust.cxx)
（2026-11-02 核对的该文件最近提交）用于核对生命周期/字符、嵌套注释和 raw/byte/C 字符串的独立状态；
未引入这些项目的源码、语法资源或运行时依赖。

语言依据：[C23 N3096 工作草案 §6.4](https://www.open-std.org/jtc1/sc22/wg14/www/docs/n3096.pdf)、
[C++ 字符串](https://eel.is/c++draft/lex.string)、
[翻译阶段](https://eel.is/c++draft/lex.phases)（在线工作草案，含较新 whitespace splice 规则；
本 profile 只支持反斜杠直接接换行的通用部分）、
[JLS SE 25 §3](https://docs.oracle.com/javase/specs/jls/se25/html/jls-3.html)、
[Rust tokens](https://doc.rust-lang.org/reference/tokens.html)、
[Rust comments](https://doc.rust-lang.org/reference/comments.html)、
[C# raw string](https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/tokens/raw-string)、
[C# interpolation](https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/tokens/interpolated)、
[Lua 5.4 §3.1](https://www.lua.org/manual/5.4/manual.html#3.1)。
文档定义语法边界，参考源码帮助选择状态；本项目的正确性还需独立预期、分片、增量与窗口测试验证，
实际结果记录于 [语言验证](PlanLanguages.md)，不把参考编辑器的能力当成本项目已验证能力。

## 13. PowerShell

新增 `language.PowerShell`，识别 `.ps1/.psm1/.psd1` 及 pwsh/powershell shebang；
Markdown 默认支持 powershell/ps1/pwsh/ps 围栏。沿用既有公开语言与编辑接口、配色和增量驱动。
`scan_powershell.go` 单独维护代码、普通引用、here-string 开头/正文、两种注释、变量和数值状态。

关键字和已知文字运算符按大小写不敏感匹配；命名参数按 Attribute，变量按 Variable，
不根据运行环境猜测 cmdlet、模块或类型。单双引号和弯引号分组处理；普通字符串允许跨行，
单引号以重复引号转义，双引号额外处理反引号转义、变量和 `$()`。
子表达式用不可变引用恢复栈及圆括号深度，字符串/注释中的圆括号不影响外层结束。
here-string 开头后只允许空白和真正行尾；关闭标记必须在物理行开头，缩进后的伪结束不关闭正文。
块注释不嵌套，在首个 `#>` 结束；只有实际消费到行尾才清理行注释和普通变量尾部。

字词拼写保留至多 128 byte，正文不保存在状态；引用栈随嵌套深度增长，逐节点比较内容，
不共享文档间可变状态。没有整行正则、运行时执行、完整参数模式解析或类型推断。
编辑规则提供 `#`/`<# #>`、结构和引用配对、适用的词边界及关键词建议；不预设补全快捷键。

`EditingRules.QuoteEscape` 指定跳过自动闭合引号前使用的转义字符；零值沿用反斜杠，
PowerShell 显式设为反引号。匹配按 Unicode rune 计奇偶，保持既有 16 KiB 上限和截断处保守处理；
Context.Literal 仍表示此位置不应用该转义判断。该字段只影响引用跳过，不解释文本或替代扫描器，
使内置和自定义语言共用同一规则，不在 SourceView 中硬编码 PowerShell ID。

2026-11-02 核对 [PowerShell 7.5 引用规则](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_quoting_rules?view=powershell-7.5)、
[注释规则](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_comments?view=powershell-7.5)
及 [v7.5.0 tokenizer.cs](https://github.com/PowerShell/PowerShell/blob/v7.5.0/src/System.Management.Automation/engine/parser/tokenizer.cs)
的 ScanAfterHereStringHeader/ScanPossibleHereStringFooter/SkipBlockComment；
参考 [Monaco v0.52.2 PowerShell](https://github.com/microsoft/monaco-editor/blob/v0.52.2/src/basic-languages/powershell/powershell.ts)
分离普通引用和 here-string 的状态，但自行实现片段与插值栈，不复制其正则或依赖。
保留 [GtkSourceView 5.22.1](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/lang-reference.html)
按词法上下文组织语言的思路，最终仍遵守本项目 Tokenizer/State 契约。

## 14. Ruby

新增 `language.Ruby`，识别 `.rb/.rbw/.rake/.gemspec/.ru`、Gemfile/Rakefile、ruby shebang，
提供 ruby/rb 围栏、`#` 注释、引用/结构配对、适用词边界和关键词建议。
支持关键字、实例/类/全局变量、symbol、常用数字/字符字面量、单双/反引号、
`%q/%Q/%w/%W/%i/%I/%r/%s/%x` 和默认 `%` 引用、正则 flags、字符串插值、
列首 `=begin/=end` 注释及 `__END__` 数据段。
百分号配对 delimiter 计嵌套，正则保留字符类和转义状态；`#{}` 使用不可变恢复栈，
代码中的字符串/注释不改变外层括号深度，`#@name/#$name` 使用流式变量状态。

`<<`、`<<-`、`<<~` 分别记录 delimiter、结束缩进许可和单引号禁用插值规则。
同一物理行的多个 heredoc 按声明顺序启动；正文内插值再次声明 heredoc 时，保存当前
正文/表达式及剩余队列，子正文结束后恢复。结束候选与正文词法并行检查，只有整行匹配
才修正本行 token 并恢复上下文，不把正文中相同前缀误认为关闭。
缩进剥离是解释器值语义，本控件保留全部原始空白与坐标。

队列使用私有的不可变追加树，插入和索引随队列长度对数增长，不在每个字符复制队列，
不在换行时一次反转整串待处理声明；节点随声明数量增长，消费完成后释放当前引用。
delimiter 的完整拼写通过共用 scanText 不可变片段保留，不截断长名字；该机械工具由
Shell、SQL、Ruby 共用，规则与可变状态仍分别维护。正文不进入状态，Equal 比较完整语义值。
命令调用/位移、正则/除法使用词法上下文，错误或歧义源码不承诺完整 Ruby parser 结果，
也不推断 Rails DSL、常量类型或自动生成 end。

2026-11-02 核对 [Ruby 3.4 literals](https://docs.ruby-lang.org/en/3.4/syntax/literals_rdoc.html)，
参考 [Lexilla LexRuby 6645c18](https://github.com/ScintillaOrg/lexilla/blob/6645c18df16077f1ea19cabdf1e8bc6b2a4409be/lexers/LexRuby.cxx)
的引用、preferRE、heredoc 上下文划分。该源码的 HereDocCls 使用固定 delimiter 数组，
本项目不沿用长度限制，采用既有不可变分片协议。未复制其实现或依赖解释器。

## 15. PHP

新增 `language.PHP`，识别 `.php/.phtml` 和明确的 `<?php`/`<?=` 内容开头。
默认文件扫描从 HTML 开始；不启用依赖 PHP 配置的 `<?` 短标签，不把所有 `.inc` 映射为 PHP。
默认 Markdown php 围栏使用同一扫描器的内部代码片段模式，可以省略开标签；
调用方显式传入围栏表时不替换其选择，没有新增公开模式类型或注册体系。

PHP 区支持大小写不敏感的关键字/常用类型词、数值、变量、单双/反引号、转义、
`{$...}`/`${...}` 插值、`#[...]` 属性、两种行注释和块注释、heredoc/nowdoc。
单引号仍允许转义自身和反斜杠，不能标为完全禁用转义的 Context.Literal；nowdoc 才禁用插值和转义。
heredoc 保存完整标识符，按片段比较行首空白后的候选；标识符前缀后紧接名字字符不关闭，
完整标识符后跟非名字字符时立即回到 PHP 代码，因此同一行的逗号、分号及后续表达式继续分析。
不要求结束行只能含 delimiter/分号，不执行或验证 heredoc 缩进剥离。

外层 HTML 状态在 `<?php`/`<?=` 处暂停，PHP 可以中断 HTML 属性、注释及 script/style 引用。
`?>` 在 PHP 代码或行注释中恢复外层状态，引用、块注释及 heredoc/nowdoc 内的同样文字不退出。
暂停前完成 JS/CSS 待分类词，避免恢复后用跨 PHP 的范围覆盖已发布 token；
只提取私有 finishCodeWord 和 tokenWriter 合并/撤回工具，不改变公开扫描契约。
引用插值用不可变恢复栈，子表达式拥有自己的引用/注释状态，旧状态快照不被后续扫描修改。

外层通过不可变 HTML profile 提供适用的词、注释和建议；script/style 仍使用现有内嵌语言。
混合文档中 HTML profile 不绑定现有只识别标记文本的 Tags/Folding，避免把 PHP 字符串内的
伪标签当成配对或联动重命名目标。代码区提供 PHP 注释、结构/引用配对及词汇建议，
不做运行结果推断、完整类型/属性语义、条件执行或通用混合文档折叠。

2026-11-02 核对 [PHP tags](https://www.php.net/manual/en/language.basic-syntax.phptags.php)、
[HTML/PHP 边界](https://www.php.net/manual/en/language.basic-syntax.phpmode.php)、
[注释](https://www.php.net/manual/en/language.basic-syntax.comments.php) 和
[字符串/heredoc/nowdoc](https://www.php.net/manual/en/language.types.string.php)。
采用当前手册的现代 heredoc 结束规则（PHP 7.3 起），提供 PHP 8 常见词汇；
不宣称是某个 PHP 发行版的完整编译器或按环境探测语法配置。
源码另核对 [php-src 8.4.0 zend_language_scanner.l](https://github.com/php/php-src/blob/php-8.4.0/Zend/zend_language_scanner.l)
的 ST_HEREDOC、ST_IN_SCRIPTING 和插值状态；不移植其解析器，也不执行 PHP。

## 16. JSX / TSX

新增 `language.JSX/TSX`、`.jsx/.tsx`、jsx/tsx 及 javascriptreact/typescriptreact 默认围栏。
普通 `.js/.ts` 的识别和规则不改变。JSX/TSX 分别复用 JavaScript/TypeScript 的代码词汇、
引用、模板、正则、注释和控制圆括号状态；`scan_jsx.go` 维护标签/属性/正文/实体及恢复栈。
只增加私有 codeTokenizer.scan 边界暂停能力，普通公开 Scan 不返回非空输入零消费；
JSX 包装器处理边界字符后继续推进，没有向公开 Tokenizer 增加策略接口。

仅在代码表达式起点进入 JSX，普通比较 `a < b` 保持代码；标签支持组件名、点/冒号/连字符、
布尔属性、引用属性、展开属性、自闭合与 fragment。正文按 Plain 前景和 ClassString，
属性引用按 String，实体单独着色；正文不按 HTML script/style 或注释模式重新解释，
属性引用中的反斜杠不转义结束引号。`{}` 内恢复代码，允许对象、字符串、注释、正则、
模板及再次嵌套 JSX；关闭子表达式或根元素时恢复父代码/标记状态与深度。

TSX 为同一物理行中可确认的 `<T,>`、`<T=...>` 和 `<T extends ...>` 回到 TypeScript；
候选名只保留有界拼写与范围，可以跨多个工作片段处理长空白，不依赖固定 32 byte 猜测。
已发布的候选 Tag/Attribute 通过 ReplaceFrom 改回代码颜色；物理换行后不跨行回写，
未确认的头按 JSX 继续。`<T extends={true}/>` 的 extends 仍是属性。
泛型/标签及错误输入有语法歧义，本 profile 不承诺完整 TypeScript parser 的裁决，
特别是跨行后才出现判别符的候选与泛型约束/布尔属性歧义；普通明确写法设独立回归。

编辑规则仍只匹配 `()[]{}`，不自动成对插入尖括号，不绑定 HTML 的空元素或标签联动规则。
标记引用仅在属性值位置自动闭合；正文不触发代码配对/关键词建议，表达式恢复代码规则。
建议提供代码词汇及已知 HTML 标签/属性和 className/htmlFor/key/ref，不推断 React 组件类型或 props。
注释命令提供 JS/TS 代码区的标记；JSX 正文注释需要显式写在 `{}` 内，本批不提供按位置自动包裹正文注释的命令。
缓存、状态与原生资源所有权不变，未新增样式键、解析依赖、语言服务或 GOUI 改动。

2026-11-02 核对 [TypeScript JSX 手册](https://www.typescriptlang.org/docs/handbook/jsx.html)，
其中 JSX/TSX 的词法场景与普通 TS 尖括号断言有不同约束。
[Monaco v0.52.2 TypeScript tokenizer](https://github.com/microsoft/monaco-editor/blob/v0.52.2/src/basic-languages/typescript/typescript.ts)
作为现有 JS/TS 基础状态的参考，不将其 basic-languages 文件当成完整 TSX parser。
保持 GtkSourceView 上下文分解的思路，混合语法以本项目自己的独立分类、分片及编辑测试验收。

## 17. Kotlin

新增 language.Kotlin、.kt/.kts（含 .gradle.kts）、kotlin shebang 和 kotlin/kt/kts 围栏。
独立 ktState 支持关键字及常见软关键字、注解/标签、反引号名称、数值/范围、字符、
普通字符串、三引号、多美元插值、行注释和嵌套块注释。复用既有扫描输出和增量协议，
没有依赖编译器、修改 GOUI 或扩大公开 Tokenizer 接口。

普通引用处理反斜杠及四位 Unicode 转义；Unicode 转义只在引用内解释，不执行 Java 式预词法转换。
三引号不转义反斜杠，但仍允许插值；连续结束引号按 Kotlin 规则全部归入同一引用。
美元前缀和美元运行数量用整数保存，少于要求数量为正文，达到要求后才进入变量或表达式。
多余美元保留在正文；可跨任意工作分片，不截断为预读范围。插值用不可变栈保存引用与花括号深度，
表达式内的字符串/注释不结束外层插值。正文、引号运行和美元运行均不保留整个文本副本。
标识符只保留 128 byte 候选拼写与完整范围；未闭合普通引用按真实行尾恢复，三引号与块注释跨行保留。

编辑提供 //、/* */、结构/引用/反引号配对、关键词建议；第三个双引号不额外生成引用，
不猜测泛型尖括号、类型解析、缩进函数运行结果或实验性编译选项。
保留既有词法 Kind/Class 分离，原始引用 Context.Literal 禁用反斜杠闭合判断，插值恢复代码上下文。

依据为 [Kotlin strings](https://kotlinlang.org/docs/strings.html)、
[Kotlin 2.2 稳定多美元插值](https://kotlinlang.org/docs/whatsnew22.html)、
[语法规格](https://kotlinlang.org/spec/syntax-and-grammar.html)（页面为 1.9-rfc+0.1，现代插值另查新文档），
以及 [Kotlin.flex](https://github.com/JetBrains/kotlin/blob/master/compiler/psi/parser/src/org/jetbrains/kotlin/lexer/Kotlin.flex)
（2026-11-02 核对 requiredInterpolationPrefix、RAW_STRING 及注释深度）。
参考 [Monaco v0.52.2 Kotlin](https://github.com/microsoft/monaco-editor/blob/v0.52.2/src/basic-languages/kotlin/kotlin.ts)
的状态分解，不复制其正则，也不把旧版本的语法范围当成当前 Kotlin 的完整承诺。

## 18. Swift

Swift 使用独立不可变状态，支持 `.swift`、文本 `.swiftinterface` 与 swift shebang；
不识别二进制 `.swiftmodule`。普通/三引号字符串与扩展 `#` 定界符使用计数状态，
`\(` 或对应数量的 `\#(` 进入带括号深度的插值表达式；嵌套引用、注释不会提前结束插值。
支持反引号名称、`$0` 简写参数、属性/编译指令、关键词、数字及嵌套块注释。

裸 `/…/` 正则采用表达式起点的词法判别，优先排除注释、空白和 `/=`；
扩展 `#/…/#` 使用匹配的 `#` 数量，正文内的反斜杠仍属于正则语法，字符类中的斜杠不提前关闭。
此处是高亮器，不承诺编译器级运算符歧义消解或正则内部语义校验；不将正则内容当作字符串插值。
双引号与反引号参与基础自动配对，单引号不作为 Swift 字符引用；第三个双引号不多插闭合引用。
扩展原始引用由用户输入完整定界符，不在 `#` 后生成无法匹配的普通闭合引号。
复用现有 EditingRules.AllowPair，不增加动态配对公共接口或 GOUI 扩展。

参考 [Swift Book 词法结构](https://github.com/swiftlang/swift-book/blob/0bb7be94c7237ff0f9a4c2512a4fd3d4f23ad5cf/TSPL.docc/ReferenceManual/LexicalStructure.md)
及 [SE-0354](https://github.com/swiftlang/swift-evolution/blob/main/proposals/0354-regex-literals.md)
（Swift 5.7 实现，裸斜杠语法还受语言模式影响；读取 2026-11-02），
并核对 [Monaco v0.52.2 Swift](https://github.com/microsoft/monaco-editor/blob/v0.52.2/src/basic-languages/swift/swift.ts)。
采用词法状态和定界符计数，不复制其规则资源或依赖，不推断类型及调用目标。

## 19. Dart

新增 `.dart`、dart shebang 和 dart 围栏。独立状态处理单双引号及三引号、
小写 `r` 原始引用、短变量与 `${…}` 插值、嵌套引用/块注释、注解和符号字面量。
原始引用同时禁用转义和插值；其他引用的 Unicode/十六进制转义保持字符串 Class。
插值栈只保存外层引用与括号深度，不积累正文，不把注释和字符串内的括号当结构括号。
数字包括十六进制、指数和分隔符，成员访问和级联点号不会被数字吞入。
基础配对、注释和建议复用现有规则；单双三引号均避免第三次输入多插一个闭合引用。
不推断 Flutter 类型、导入目标或模式的语义，不引入分析器。

依据 [Dart 字符串与数字](https://dart.dev/language/built-in-types)、
[官方注释说明](https://github.com/dart-lang/site-www/blob/main/src/content/language/comments.md)
与 [SDK 扫描器](https://github.com/dart-lang/sdk/blob/876f4d842286d3ea42dd1a02590bea544cb96634/pkg/_fe_analyzer_shared/lib/src/scanner/string_scanner.dart)
（读取 2026-11-02；数字分隔符要求 Dart 3.6+），并参考
[Monaco v0.52.2 Dart](https://github.com/microsoft/monaco-editor/blob/v0.52.2/src/basic-languages/dart/dart.ts)
的词法呈现边界。所有样本为本项目原创，没有复制第三方规则资源或增加依赖。

## 20. R

新增 `.r/.R/.Rscript`、`.Rprofile`、Rscript shebang、r/R 围栏；不猜测 `.Rmd/.qmd`
混合文档或 `.RData` 二进制内容。单双引号字符串、带转义的反引号名称、注释和
原始 `r/R` 引用分别保留状态。原始引用允许单双引号、`() / [] / {}` 定界符，
以及任意数量的匹配短横线；前缀、闭合候选只保存计数，不缓存引用正文。
普通字符串允许跨物理行；未完成的原始前缀按普通引用保守恢复，不阻塞后续扫描。

关键词包括保留字和 NA/Inf/NaN/NULL 等常量，T/F 是可赋值名称，不当作保留字。
数字支持十六进制浮点、指数、L/i 后缀；`%…%` 作为完整运算符处理，其内部括号不参与配对。
赋值和 `|>` 管道按代码运算符呈现；不从函数调用名字推断内置函数或包导出。
词边界支持点号及 Unicode 标识符字符，不调用 R 运行时或依赖进程 locale。
注释、括号、普通引用与反引号提供基础编辑规则；动态原始引用定界符由用户完整输入。

参考 [R Quotes](https://stat.ethz.ch/R-manual/R-devel/library/base/html/Quotes.html)
（读取页面标记 base 4.6.0，2026-11-02）及
[R 源码 RawStringValue](https://github.com/wch/r-source/blob/R-4-4-branch/src/main/gram.y)
核对单双引号原始前缀；原始字符串自 R 4.0 起支持。
[Lexilla LexR.cxx](https://github.com/ScintillaOrg/lexilla/blob/4b654f6c86f2129cc646b0c4a1e65c8d0c8ce8a7/lexers/LexR.cxx)
也使用匹配字符与短横线计数，采用此有限状态思路，不复制其源码或规则资源。
本批继续使用已有 token/Class/增量协议，没有公共分词器契约或 GOUI 改动。

## 21. Zig

新增 Zig、`.zig`、zig 围栏、行注释/基础配对和关键词建议。独立状态处理普通字符串、字符、
`@"name"` 引用名称、`@builtin`、Unicode/十六进制转义、数值及 `\\` 多行字符串。
Zig 每个逻辑行独立分词：普通引用和 `\\` 正文在真实行尾结束；工作片段边界不重置。
`//`、`///`、`//!` 使用注释 Class，不把 `/* */` 当作块注释；原始行正文不解释转义或插值。
整数类型识别 i/u 加数字的词法形式，不验证位宽是否合法；常见历史关键词仍提供词法颜色，
不承诺通过特定 Zig 编译器的语法检查。词候选最多保留 128 byte，正文不进入状态副本。

2026-11-02 核对 [Zig 0.17.0 语言参考](https://ziglang.org/documentation/0.17.0/)
的注释、引用名称、字面量和多行字符串；参考
[Lexilla LexZig.cxx](https://github.com/ScintillaOrg/lexilla/blob/master/lexers/LexZig.cxx)
对各引用/注释模式的区分。采用本模块的分片/增量协议，不复制规则资源、引入编译器或依赖 GOUI 改动。

## 22. Objective-C 系列

Objective-C 复用第 12 章的 C 扫描器，增加语言身份及 `@` 词法，不另建一份 C 引用/预处理状态。
支持 `@interface/@implementation/@end` 等指令、id/instancetype/self/nil 等语言词汇、
对象字符串、装箱数值/布尔、集合与消息表达式中的结构符；字符串/注释内的括号仍不参与配对。
不通过 Foundation 类名猜测类型，也不从消息参数名字推断方法或属性。
基础编辑规则保留结构/引用配对、// 与 /* */，关键词建议包含 `@` 前缀，单词边界同步包含它。

`language.ObjectiveC` 的 ID 为 objective-c，默认围栏接受 objc/objectivec/objective-c。
`.m` 与 MATLAB 冲突，自动识别只检查调用方提供的前 4 KiB 中行首明确的 Objective-C 构造；
没有此类信息时保持 Plain，使用方可显式选择语言。`.h` 保持原 C++ 约定，不改动其他文件识别。
判断是有界推测，不承诺解析注释、字符串或整个编译单元后才确定语言。

2026-11-02 核对 [Apple 类定义](https://developer.apple.com/library/archive/documentation/Cocoa/Conceptual/ProgrammingWithObjectiveC/DefiningClasses/DefiningClasses.html)
和 [Clang Objective-C 字面量](https://clang.llvm.org/docs/ObjectiveCLiterals.html)
（后者注明 LLVM Clang 3.1 / Apple LLVM 4.0 起提供相关字面量）。
C/C++ 基础状态及参考框架沿用第 12 章，不引入 Clang、Cocoa 或其他运行依赖。

Objective-C++ 使用独立 ID objective-cpp，`.mm` 直接识别；围栏接受
objective-cpp/objective-c++/objcpp/objc++。同一 C 扫描器选择 C++ 基础规则，
在正常代码状态增加上述 Objective-C 语法，保留 C++ 原始引用及预处理。
原始引用正文中的 @ 指令不进入代码状态；两种语言身份的状态不能互换。
