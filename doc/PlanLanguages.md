# 语言支持验收记录

日期：2026-10-19。功能规则见 [语言设计](DesignHighlighting.md#9-tomlyaml-与-shell)、
[补全设计](DesignCompletion.md)、[编辑设计](DesignEditing.md)。

## 改动

- 内置三种语言及自动识别；新增 Variable 分类并接入 basic/modern 和亮暗 Scheme。
- 独立扫描状态、不可变嵌套栈/队列、当前逻辑行的延迟分类，保留原有三语言回归。
- 补全、自动配对和括号匹配复用词法上下文；不在块文本、注释或 heredoc 中生成代码建议。
- 新增三份原创示例，六个文档可独立编辑和撤销；文件按钮随宽度换行。
- 中英文 README 只更新对外语言列表；技术细节留在 doc/ 与示例指南。
- 未修改 GOUI、go.mod、go.work 或第三方依赖。联调 GOUI 为 `903daf8638063bba1a2fb23ec18ff10d545d5fb4`。

## 逻辑与原生验证

| 环境 | 执行与结果 |
| --- | --- |
| Ubuntu 22.04.5 amd64，Go 1.25.8，Pango | `go test ./... -count=1`、`go test -race ./...`、`go vet ./...` 通过 |
| macOS 15.7.7 arm64，Go 1.24.9，CoreText | 本机原生 `go test ./... -count=1` 通过；语言窗口程序与 editor 示例构建通过 |
| Windows 10 19045 amd64，Go 1.24.0，DirectWrite | 本机原生 `go test ./... -count=1` 通过；语言窗口程序与 editor 示例构建通过 |
| 关闭工作区，声明的 GOUI v0.1.1 | `GOWORK=off go test ./...` 失败：`internal/editor/plan.go:40` 的 `Model.ApplyBatch` / `textedit.BatchOptions` 不存在，是已有依赖版本缺口 |

新增逻辑断言覆盖键、字符串、转义、数字/日期、YAML Core 字面量与块缩进，
Shell 引用/替换、多个 heredoc、TAB 剥离、quoted delimiter、here-string 和算术左移。
Shell 的变量、命令替换或转义字符后紧接 `#` 时仍属当前词，只有实际词边界才开始注释。
识别用例包括扩展名优先、启动文件、shebang 与 `env` 带参数选项，未知选项保守降级。

分片断言覆盖 1/2/3/7/31 字节及 16 KiB 工作片段、Unicode、跨行引用、超过片段的键、
数字、日期小数和 delimiter；YAML 的超长数字/布尔后接空白或注释仍保留字面量颜色，
数字内部的空白则按普通标量处理。900 份固定随机种子的畸形语法输入同时检查分片一致性、
非重叠范围及完整 UTF-8 字符边界；测试生成的数据不进入源码目录。
编辑回归对删除多行起始引号、替换块标记、修改 heredoc 结束行/引用后的分类给出独立预期，
再对照新引擎及模型行索引；另验证入状态快照不被后续扫描修改。

三端新增原生测试以同一后端/字体/比例的独立单色 TextLayout 比较字形位图，
覆盖 TOML 多行字符串、YAML 块文本和 Shell 变量（含 heredoc 的 `${HOME}`），分别检查亮暗 Scheme。
比较前复制借用像素，逐字节断言尺寸/stride/像素一致；不跨平台共享字体金图。
这证明指定字形的前景正确，不等于桌面合成、真实输入法或所有语法的像素验收。

## Linux 窗口矩阵

独立 TigerVNC 桌面、Xfwm4（compositor off），通过正式窗口事件/文本提交路径运行：

```sh
go run ./tests/window/editor/languages
go run ./tests/window/editor/languages -dark
```

| 实际 painter / scale | 浅色 | 深色 |
| --- | --- | --- |
| Software / 1× | 通过 | 通过 |
| Software / 2× | 通过 | 通过 |
| OpenGL / 1× | 通过 | 通过 |

六种语言逐个切换，检查基础建议、Enter 接受和一次撤销；逐字符输入 TOML 三引号、
YAML doubled quote、Shell literal 反斜杠/结束引号；另检查括号生成、只读拒绝输入及语义文本。
每个运行程序记录实际 typography、painter、scale，断言完成后关闭；启动成功不算验收。

真实用户桌面另使用 Xfwm4（compositor on）、Software 1×、Pango，检查 1120×780 的
editor 示例：ChromeInfo 报告 Integrated enabled，三份新增样本可切换，六个文件按钮
完整显示，TOML 浅色及 YAML/Shell 深色的关键结构肉眼可辨。截图仅为观察证据；
heredoc `${...}` 的变量颜色由上述独立原生断言确认。

## 限制与副作用

- 三端全仓测试采用本地工作区；GOUI 所需接口发布前仍不能声称独立依赖构建通过。
- macOS/Windows 此切片没有运行新增窗口交互矩阵，只有原生包测试及窗口程序构建；
  三端物理快捷键、真实 IME、跨屏缩放、远端桌面合成均未在此切片验收。
- 这是词法高亮与基础编辑联动，不提供 TOML/YAML 配置校验、Shell 执行或完整上下文敏感解析。
  heredoc 正文不递归解析完整命令替换；复杂 YAML 显式键等边界见语言设计。
- 不作性能/资源占用改善结论。本次超长 token 验证针对正确性，不代替中大文件性能测量。
- 真实桌面仅启动并关闭本次示例，期间切换了本窗口焦点/主题；未修改剪贴板、文件或系统设置。
  本次 VNC 客户端与独立测试桌面已关闭，既有 SSH 连接保持可用。
- 临时日志、构建产物与观察图保存在执行机 `/tmp/sourceview-languages-wi76dpfo/`；
  远端同名临时目录保留用于复查，没有向仓库写可执行文件或临时截图。

## 2026-10-27：十四种语言扩展

设计见 [高亮设计](DesignHighlighting.md#10-配置请求与构建文件) 和
[编程语言范围](DesignHighlighting.md#11-编程语言与-css)。

### 交付范围

- 新增 JSONC、HTTP、Protobuf、Makefile、CMake、Nginx、INI、Dockerfile、SQL、Go、Python、
  JavaScript、TypeScript、CSS 的内置定义、文件识别、词法高亮、注释/引用规则和静态词汇建议。
- HTML script/style 复用 JS/CSS，外层结束标记保持优先；非对应显式 type 保留普通正文。
  Markdown 内置与 `NewMarkdown(nil)` 共用全部语言及常用别名表。
- 修复默认补全在前缀匹配前截取 64 项的问题。窗口验证最初在 SQL 的 `SELECT` 复现无建议；
  修正为先筛选、去重再限制数量后，回归断言和三组窗口运行均通过。自定义提供者的顺序契约不变。
- editor 新增十四份原创样本，总计 21 份，通过嵌入文件系统加载；文件名选择按钮自动换行，
  各文档分别保留修改与历史。中英文 README 同步语言列表。
- 没有修改 GOUI、依赖版本或引入语法资源/第三方库。`go.mod` 仍使用 GOUI v0.4.0。

### 逻辑、原生与构建

环境：Ubuntu 22.04.5 amd64、Go 1.25.8、Pango。工作区 GOUI 为
`e9e44256e0e927da98ef22848843374e1644948e`；独立依赖验证与窗口二进制构建均关闭工作区，使用 GOUI v0.4.0。

| 检查 | 结果 |
| --- | --- |
| `go test ./...` | 通过 |
| `GOWORK=off go test ./...` | 通过 |
| `go test -race ./...` | 通过；最终扫描状态调整后重跑受影响 `language`、`internal/highlight` 和根包的 race，通过 |
| `go vet ./...` | 通过 |
| `CGO_ENABLED=0 GOWORK=off GOOS=windows GOARCH=amd64 go build ./...` | 通过，仅交叉编译 |
| `CGO_ENABLED=0 GOWORK=off GOOS=darwin GOARCH=arm64 go build ./...` | 通过，仅交叉编译 |

独立预期覆盖各语言关键结构、严格 JSON 与 JSONC 区别、跨行引用/注释、Make 自定义配方前缀、
CMake 长括号、Docker 多 heredoc、HTTP Content-Type 正文、JS 模板/正则/除法、Python 插值、
SQL 引用/参数和 CSS 分组规则。以 1/2/3/7/31 byte 及 16 KiB 分片重放，包含超长 token 和 delimiter，
比较完整 token 与出状态；1,500 份固定随机种子的畸形输入检查分片一致性、进展、UTF-8 边界和有序非重叠范围。
新增跨行起止标记编辑的独立分类断言，并对照全新引擎；检查已发布状态不被后续扫描修改。

新增十四语言的原生颜色用例：同一 Pango/字体/1× 比例下，亮暗 Scheme 的指定字形
与独立单色 TextLayout 位图逐字节比较，复制借用像素后比较尺寸/stride/内容；
这是指定颜色的离屏断言，不把它算作所有语法或跨平台字体的像素验收。

### 独立 X11 会话

TigerVNC 1.12.0、Xfwm4 compositor on，专用 `:87`，与用户桌面隔离。
二进制通过 `GOWORK=off go build` 构建，运行 `tests/window/editor/languages`：

| 实际后端 / 比例 | 外观 | 结果 |
| --- | --- | --- |
| Pango / Software / 1× | 浅色 | 通过 |
| Pango / Software / 1.5× | 深色 | 通过 |
| Pango / OpenGL / 1× | 深色 | 通过 |

程序经正常 Window.DispatchEvent / CommitText，逐个检查 20 种有静态建议的内置语言：
弹窗布局就绪、目标建议存在、方向键选择、Enter 接受、一次撤销；新增十四语言全部通过。
另检查 TOML/YAML/Shell/Python/Go 引用输入、括号生成、只读拒绝与语义文本状态。
每次运行都在全部断言后关闭窗口并记录实际后端、比例、外观。

在同一独立会话运行 editor，ChromeInfo 确认 Integrated enabled；1120×780 的 21 个文件选择按钮
完整换行显示。通过真实 X11 点击切换 JavaScript 浅色和 Python 深色，观察正则/模板与
装饰器/三引号/插值颜色，正文与工具栏无裁切。截图仅作观察，颜色正确性另有上述独立断言。
没有向用户桌面发送输入、操作剪贴板或保存样本。日志与观察截图位于执行机临时目录
`/tmp/sourceview-languages-mUzIoM/`，没有写入源码目录。
验收结束后已关闭本次示例、窗口程序及专用 X11/窗口管理器会话，保留既有用户桌面。

本次没有执行 macOS/Windows 原生窗口验收，也未测真实 IME、跨屏缩放或为性能改进作量化结论。
语言方言、模板歧义和词法恢复的适用范围以设计文档为准；高亮不是语法校验或完整语言服务。

## 2026-10-27：JSON Lines

设计见 [JSON Lines](DesignHighlighting.md#31-json-lines)。新增 `language.JSONLines`，
识别 `.jsonl/.ndjson`，与 JSON/JSONC 共用扫描核心，在真实逻辑行尾清空记录状态。
JSON/JSONC 原有注释/编辑规则与跨行容器行为不变；JSON Lines 使用 JSON 的引用与词汇规则，
不启用注释或跨记录折叠。Markdown 支持 `jsonlines/jsonl/ndjson` 围栏，HTTP 支持显式
JSON Lines MIME 正文；新增 `events.jsonl` 原创样本，editor 共 22 份样本，中英文 README 同步。
没有改动 GOUI 或依赖。

回归覆盖根值类型、未闭合对象/字符串/超深容器的行隔离、注释拒绝、CRLF、转义 `\\n`、
超长记录、最后一行无换行，以及 LF 插入/删除后的增量恢复。1/2/3/7/31 byte 和 16 KiB
分片比较 token 与完整出状态；明确检查预读到行尾但未消费到行尾时不能提前重置。
原生亮暗颜色用例将未闭合 `{` 后一行的根字符串与独立 String 色 TextLayout 比较，
避免两条扫描路径同时把它误当 Property 也通过测试。

扩充固定种子的随机输入时发现已有标记状态残留：
`go test ./internal/highlight -run TestAddedLanguageMalformedFragments -count=1`
最初失败，HTML token 相同，但完整/分片扫描的 `htmlAttr` 分别为 `"-&"` 和空。
修正标签结束/新标签开始的状态清理，保留 script/style 类型判断所需的局部值；
新增 XML/HTML 普通标签、自闭合标签、注释前缀的最小出状态断言。
保留新增的全部随机输入与断言，修正后通过；该修正与 JSON Lines 分开提交。

Ubuntu 22.04.5 amd64、Go 1.25.8、Pango 下通过：

- `go test ./...`
- `GOWORK=off go test ./...`（GOUI v0.4.0）
- `go test -race ./...`
- `go vet ./...`
- 标记状态修正的独立暂存快照：`GOWORK=off go test ./language ./internal/highlight`

专用 TigerVNC `:87` / Xfwm4 compositor on，使用关闭工作区构建的语言窗口程序：
Software 浅色 1×、深色 1.5× 均通过正常事件路径的补全、接受和撤销断言。
新增 JSON Lines 用例以不完整上一记录和下一行 `tr` 请求补全，检查接受 `true` 和撤销；
全套原有语言、引用输入、括号、只读及语义状态断言一同通过。
原生颜色断言使用 Pango/1× 的同字体独立位图比较；窗口比例验证不代替 1.5× 像素金图。
未执行 macOS/Windows 原生验收或新增性能测量。

日志和构建产物在 `/tmp/sourceview-jsonlines-L6LJ1q/`，没有向用户桌面发送输入或操作剪贴板。
验收结束后关闭本次专用 X11 与窗口管理器会话，保留原有用户桌面。

## 2026-11-02：C、C++、Java、Rust、C#、Lua

设计见 [编程语言扩充](DesignHighlighting.md#12-ccjavarustclua)。实现六种独立语言 ID、
元数据、文件识别、流式高亮、适用的注释/引用规则、静态词汇与 Markdown 围栏别名。
C/C++ 共享词法实现和独立词汇；Java Unicode 转换、Rust、C#、Lua 独立维护状态。
没有修改 GOUI、增加第三方依赖或公共扫描接口，依赖仍为 GOUI v0.5.0。
中英文 README 同步，editor 新增六份原创样本，共 28 份；样本不执行。

回归包含独立 Kind/Class 预期、1/2/3/7/31 byte 与 16 KiB 分片、完整出状态比较、
每语言 300 轮固定种子不完整源码，以及开闭符号编辑后的增量传播与全量结果比较。
长样本覆盖超长正文、C++ 16 字符 delimiter、Rust 255 个 hash、Java 连续 32 KiB 的 u、
Lua 超过 32 KiB 的等号 delimiter，以及 C# 大于预读范围的 `$`/引号/插值括号数量。
另外验证 Unicode 转义生成注释/换行、C 行注释续行、raw 标识符、嵌套注释/插值、
空合并与范围运算符、Lua 拼接、文本状态快照不变性、围栏、文件识别和 Rust 单引号规则。
普通词法、分片一致性与错误恢复均有断言；不以两条扫描路径相同代替独立分类预期。

Ubuntu 22.04.5 amd64、Go 1.25.8、GOUI v0.5.0、Pango 下通过：

工作区联调使用相邻 GOUI `b49049d`；关闭工作区的测试、窗口程序构建及交叉构建使用
已发布 GOUI v0.5.0。按 C/C++、Java、Rust、C#、Lua 拆分的每份暂存快照均另行导出，
在无 go.work 的临时目录运行 `GOWORK=off go test ./...` 通过，避免后续语言掩盖前一提交的依赖遗漏。

- `go test ./...`
- `GOWORK=off go test ./...`
- `GOWORK=off go test -race ./...`
- `GOWORK=off go vet ./...`
- `GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`
- `GOWORK=off GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`

原生颜色回归覆盖六语言亮暗 Scheme，将整行关键字、原始字符串正文、嵌套注释正文或
Unicode 转义关键字的位图与同字体、独立指定目标颜色的 TextLayout 比较。
采样范围为同排版参数的完整行 bitmap，比较尺寸、stride 和原始预乘 RGBA 字节，误差为 0；
是同一 Pango 后端 1× 断言，不将其视为跨字体/跨平台金图或 1.5× 颜色采样。

独立 TigerVNC `:87`、Xfwm4 compositor on，关闭工作区构建语言窗口程序。
Software 浅色 1×、深色 1.5× 都通过 27 种有静态建议语言的补全布局、目标建议、接受和撤销断言；
Java/C# 三引号、Rust 生命周期输入及原有引用、括号、只读、语义断言一并通过。
所有模拟输入经正常 Window.DispatchEvent/CommitText；没有使用 DevServer 或语义动作捷径。
示例窗口确认 28 个文件按钮完整换行显示，实际点击 C++ 浅色与 C# 深色观察引用、注释、
数值和插值的呈现；观察截图不代替颜色断言。

日志、可执行文件与截图在 `/tmp/sourceview-languages-six-CfIcYT/`，没有写入源码目录，
没有向用户桌面发送输入或操作剪贴板。验证后关闭本次示例及专用桌面会话。
本次未执行 macOS/Windows 原生窗口、真实 IME 或性能基准；交叉构建不等于原生场景验收。
条件编译、完整语法检查和语言服务不在本批范围，词法 profile 的恢复边界以设计为准。

## 2026-11-02：第二批语言

### PowerShell

新增语言、文件/shebang 识别、powershell/ps1/pwsh/ps 围栏、基础编辑规则与关键词建议；
新增原创 hello.ps1，示例共 29 份，中英文说明同步。设计见高亮设计第 13 节。
回归覆盖大小写、普通引用跨行、弯引号、反引号、作用域变量、相邻变量/插值、
嵌套 `$()`、here-string 伪结束、非嵌套块注释、长正文、分片状态、Class、快照与增量恢复。

Ubuntu 22.04.5 amd64 / Go 1.25.8 / Pango：`go test ./...`、
`GOWORK=off go test ./...`、`GOWORK=off go test -race ./language ./internal/highlight` 通过。
原生亮暗颜色沿用同字体、同后端、1× 的整行预乘 RGBA 零误差位图断言，新增 here-string 正文用例。
独立 TigerVNC :87 / Xfwm4 compositor on / Software 浅色 1× 通过语言窗口程序：
28 种静态建议语言的补全、接受、撤销，以及 PowerShell 重复单引号输入和全部原有断言通过。
输入均经 Window.DispatchEvent / CommitText，无 DevServer、剪贴板或文件写入。
关闭工作区构建窗口程序使用 GOUI v0.5.0；工作区联调用相邻 GOUI b49049d。
本组未单独进行 macOS/Windows 原生运行；最终批次再记录其余组合验证。
日志和构建位于 `/tmp/sourceview-language-batch2-JqwBCh/`，独立会话供本批后续组继续验证。

### Ruby

新增 Ruby、ruby/rb 围栏、识别与基础编辑规则；新增 hello.rb，示例共 30 份。
引用/百分号字面量、symbol、正则/除法、块注释、数据段、长变量、长 delimiter、
63 个同一行排队的 heredoc，以及 heredoc 插值内再次声明 heredoc 均有独立预期。
复用分片/随机/增量/Class/状态不变性测试；提取 scanText 后 Shell/SQL 原有测试保留并通过。
Ruby 非插值短引用仍允许转义结束符，Context.Literal 不将这类引用当成完全禁用反斜杠。

同上 Linux/Pango 环境，`go test ./...`、`GOWORK=off go test ./...`、
`GOWORK=off go test -race ./language ./internal/highlight` 通过。
新增 heredoc 原生亮暗 1× 位图断言通过；独立 Software 浅色 1× 窗口程序通过 29 种语言建议、
接受和撤销，以及新增 Ruby 转义单引号输入和原有全部断言。
环境、依赖、日志目录与桌面隔离方式同 PowerShell；本组未单独运行 macOS/Windows。

### 语言引用转义规则

补齐 EditingRules.QuoteEscape，保留零值的反斜杠行为，PowerShell 配置反引号。
旧实现新增回归 `GOWORK=off go test . -run TestConfigurationAndShellQuoteTyping -count=1`
复现失败：输入 PowerShell 尾部反斜杠字符串后，多保留一个生成的闭合引号。
修正后该用例、反引号转义、Ruby 默认转义、Unicode 自定义转义及配置校验全部通过。
`GOWORK=off go test ./...`、`GOWORK=off go test -race . ./language` 通过。
独立浅色 1× 正常事件窗口验证通过，并逐步断言：输入反引号转义的双引号后，
生成的另一个闭合引号仍在，光标停在它之前；不是只比较整段输入后的最终文本。
没有按语言 ID 在控件中加入特例，也未修改 GOUI。

### PHP

新增 PHP、文件/内容识别、默认 php 围栏片段模式、基础编辑规则和 hello.php，示例共 31 份。
独立预期覆盖 HTML 属性/注释/script 中断、恢复后的 JS/CSS 待分类词、行注释与块注释的
结束差异、变量和复杂引用插值、属性标记、现代 heredoc/nowdoc、长标识符及同一行后续代码。
随机输入显式加 PHP 开标签，确保实际覆盖代码模式；完整/极小分片、增量和 Class 断言通过。
另外检查默认 Markdown 围栏可省略开标签，以及混合 HTML profile 不启用不安全的标签辅助。

`go test ./...`、`GOWORK=off go test ./...`、
`GOWORK=off go test -race ./language ./internal/highlight` 通过，原生亮暗 1× 的 nowdoc 正文颜色通过。
独立 Software 浅色 1× 窗口通过 30 种语言建议、接受和撤销，PHP 转义引用与原有全部断言通过。
本组环境、依赖和日志目录同前，未单独运行 macOS/Windows；没有新增第三方依赖或 GOUI 改动。

### JSX / TSX 与批次完成验证

新增 JSX/TSX、文件识别、默认围栏及别名、适用的配对和建议；新增 app.jsx/app.tsx，示例共 33 份。
独立断言覆盖标签/属性/实体/正文 Class、fragment、自闭合、展开属性、嵌套 JSX、
对象/引用/注释/正则/模板中的括号、比较与除法、TypeScript 词汇、泛型箭头函数、
extends 属性、32 KiB 空白后的泛型判别、编辑后的传播以及状态快照不变性。
新增语言继续参加 1/2/3/7/31 byte / 16 KiB 分片、300 轮固定种子随机输入、Class 和默认围栏测试。
原生颜色新增 JSX 多行属性引用与 TSX 关键词的亮暗 1× 位图断言，沿用同字体/同后端 RGBA 零误差条件。

批次最终在 Ubuntu 22.04.5 amd64、Go 1.25.8 下通过：

- `go test ./...`（工作区相邻 GOUI b49049d）
- `GOWORK=off go test ./...`（已发布 GOUI v0.5.0）
- `GOWORK=off go test -race ./...`
- `GOWORK=off go vet ./...`
- `GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`
- `GOWORK=off GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`

关闭工作区构建最终语言窗口程序，独立 TigerVNC :87 / Xfwm4 compositor on / Pango / Software：
浅色 1× 与深色 1.5× 都通过 32 种有静态建议语言的补全、接受和撤销；
另检查 JSX 标签建议、TSX className 属性建议、JSX 属性引用的字面反斜杠、正文中的单独引号，
以及全部旧引用、括号、只读、语义文本和 PowerShell 转义中间光标断言。
按正常 Window.DispatchEvent/CommitText 输入，不使用 DevServer 或旁路语义动作。

editor 请求 1120×780 Integrated 窗口，ChromeInfo 日志确认 enabled；33 个样本按钮完整换行显示。
独立会话真实点击 PHP 浅色与 TSX 深色，观察混合语言、heredoc、泛型和标签表达式，未见正文或工具栏裁切。
截图仅作观察，不代替独立颜色/语义断言；本批没有性能量化结论、真实 IME 或 macOS/Windows 原生运行。
JSX/TSX 的词法歧义和标签/正文注释辅助边界详见设计，不将交叉构建视为原生场景验收。

最终日志、二进制和截图位于 `/tmp/sourceview-language-batch2-JqwBCh/`，没有写入源码目录，
没有向用户桌面发送输入、操作剪贴板或执行样本。验证结束后关闭本次示例和专用 X11/窗口管理器会话，
保留原有用户桌面。原有未跟踪 doc/Doing.md 和 example/temp/ 不纳入语言功能提交。

## 第三批语言（2026-11-02）

### Kotlin

新增 Kotlin 文件、shebang、Markdown 围栏及别名识别，基础编辑规则、关键词建议与 hello.kt，示例共 34 份。
独立预期覆盖注解、标签、反引号名称、普通/三引号字符串、嵌套插值、多美元插值和嵌套注释。
32 KiB 美元定界符、超长正文、闭合引号串、数字后的成员访问与范围运算参加完整/极小分片对照；
另检查随机畸形输入、增量恢复、状态快照不可变性、Class 及默认围栏。

Ubuntu 22.04.5 amd64 / Go 1.25.8：`go test ./...`（工作区 GOUI b49049d）、
`GOWORK=off go test ./...`（GOUI v0.5.0）、
`GOWORK=off go test -race ./language ./internal/highlight` 通过。
原生 Pango 亮暗 1× 字符串正文 RGBA 零误差断言通过，使用同字体、同布局的独立配色预期。
关闭工作区构建语言窗口程序，在独立 TigerVNC :87 / Xfwm4 compositor on / Software / 浅色 1× 下，
Kotlin 补全接受/撤销、三引号输入、已有语言和最终语义/只读/括号断言通过。
输入经 Window.DispatchEvent/CommitText，不使用 DevServer，无剪贴板或用户桌面输入。
日志和产物放在 `/tmp/sourceview-language-batch3-Yg4xpq/`；本阶段未运行 macOS/Windows 原生验证。
没有新增第三方依赖或修改 GOUI。Swift、Dart、R 继续按设计实施。

### Swift

新增 Swift、swiftinterface/shebang、围栏识别、编辑规则和 hello.swift，示例共 35 份。
独立断言覆盖普通/扩展/三引号引用、精确哈希数插值、嵌套插值、嵌套注释、反引号与简写参数，
以及正则字符类、扩展正则和除法；32 KiB 哈希前缀/转义/闭合参与完整与极小分片对照。
增量恢复、快照、畸形随机输入、Class、围栏及原生亮暗正文 RGBA 断言通过。

同上述环境，`GOWORK=off go test ./...`、
`GOWORK=off go test -race ./language ./internal/highlight` 通过。
独立 X11 / Pango / Software / 浅色 1× 通过 Swift 建议接受/撤销、三引号和扩展原始引用输入，
同时通过全部旧语言、语义、只读及括号断言。日志 swift-test.log、swift-window.log 位于本批临时目录。
裸正则的词法歧义边界见设计；不将高亮推断描述为编译器解析。

### Dart

新增 Dart、文件/shebang/围栏识别、编辑规则与 hello.dart，示例共 36 份。
独立预期检查单双/三引号、原始引用禁用转义和插值、嵌套表达式/注释、注解和符号、
Unicode/十六进制转义、成员/级联点号、指数与十六进制数。
原始引用正文颜色、Class、围栏、状态快照、增量恢复、长行与随机畸形分片断言通过。

同上述环境，`GOWORK=off go test ./...`、
`GOWORK=off go test -race ./language ./internal/highlight` 通过。
独立 X11 / Pango / Software / 浅色 1× 通过 Dart 建议接受/撤销、单三引号和原始尾反斜杠输入，
以及全部旧语言与最终断言。日志 dart-test.log、dart-window.log 位于本批临时目录。

### R 与批次完成验证

新增 R、文件/启动文件/shebang/围栏识别、编辑规则与 hello.R，示例共 37 份。
独立预期覆盖单双原始引用、匹配短横线和括号、误匹配后的正文、反引号名称、
保留常量与可赋值 T/F 的差别、Unicode 转义、十六进制浮点、运算符中的非结构括号。
32 KiB 前缀/闭合、长正文、跨行字符串、随机畸形输入、极小分片、增量、快照、Class、围栏通过。
新增四种语言的原生亮暗 1× 正文颜色继续使用同字体/同 Pango 布局的 RGBA 零误差预期。

批次最终在 Ubuntu 22.04.5 amd64、Go 1.25.8 下通过：

- `go test ./...`（工作区相邻 GOUI b49049d）
- `GOWORK=off go test ./...`（独立依赖 GOUI v0.5.0）
- `GOWORK=off go test -race ./...`
- `GOWORK=off go vet ./...`
- `GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`
- `GOWORK=off GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`

关闭工作区构建最终语言窗口程序与 editor，独立 TigerVNC :87 / Xfwm4 compositor on / Pango / Software：
浅色 1×、深色 1.5× 都通过 36 种语言的 38 组建议接受/撤销断言与 22 组引用输入，
含新增四语言、JSX/TSX 标签建议、只读拒绝、括号、语义文本/选区和 PowerShell 中间光标断言。
均经正常事件路径，不使用 DevServer。editor 浅色 1× 实际点击四个新样本，深色 1× 检查 Swift/R，
37 个样本按钮完整换行，窗口 ChromeInfo 确认 Integrated enabled，未见正文或工具栏裁切。
截图是外观观察，不能代替上述独立颜色/文本/光标断言。

本批日志、二进制和截图在 `/tmp/sourceview-language-batch3-Yg4xpq/`，未写入源码目录。
结束时关闭本次 editor、专用 X11 和窗口管理器，保留原有用户桌面；未操作剪贴板或执行语言样本。
没有真实 IME、macOS/Windows 原生场景或性能量化结论；交叉构建仅证明可编译。
各语言词法近似与动态引用编辑边界见设计，不承诺编译器/语言服务能力。
未修改 GOUI 或依赖，原有未跟踪 doc/Doing.md、example/temp/ 保留，不纳入提交。

## 第四批语言（2026-11-02）

### Zig

新增 Zig、文件/围栏识别、编辑规则、建议与 hello.zig，示例共 38 份。
独立预期覆盖任意整数类型拼写、内置函数、引用名称、Unicode/十六进制转义、
多行字符串及文档注释，并确认 C 块注释写法不改变后续代码状态。
长行、完整与极小分片、畸形随机输入、上下文及默认围栏通过。
`go test ./...`（工作区 GOUI b49049d）、`GOWORK=off go test ./...`（GOUI v0.5.0）通过，
原生 Pango 亮暗 1× 多行字符串 RGBA 零误差断言通过。
关闭工作区构建语言窗口程序，在独立 TigerVNC :87 / Xfwm4 compositor on / Software / 浅色 1× 下，
`-language zig` 的建议接受/撤销、引用转义输入及公共括号/只读/语义断言通过。
筛选参数用于逐语言验证，未指定时仍执行全部场景；没有场景匹配时直接失败。
本批日志和产物在 `/tmp/sourceview-language-batch4-5i1u39/`，未修改 GOUI 或依赖。

### Objective-C

新增 Objective-C、保守 .m 识别、围栏及别名、编辑规则和 hello.m，示例共 39 份。
复用 C 词法设施，独立断言验证 @ 指令、对象字符串、装箱数值/布尔与预处理头文件，
长引用、极小分片和随机畸形输入通过；既有 C/C++ 回归保持通过。
`GOWORK=off go test ./...` 及原生 Pango 亮暗 1× 指令颜色断言通过。
同本批独立 X11 / Software / 浅色 1×，`-language objective-c` 的 @interface 建议、接受/撤销、
对象引用输入及公共断言通过。自动识别保留 MATLAB/无特征 .m 的 Plain 和 .h 的 C++ 约定。

### Objective-C++

新增独立 Objective-C++ ID、.mm、默认围栏/别名及 hello.mm，示例共 40 份。
C++ 原始字符串与 @ 语法混合、长引用、模板词汇、真实行尾状态恢复和分片对照通过。
`GOWORK=off go test ./...` 与原生 Pango 亮暗 1× 原始引用正文颜色断言通过。
同本批独立浅色 1× 会话，`-language objective-cpp` 建议接受/撤销、对象引用输入和公共断言通过。

### Scala

新增 Scala、文件/shebang/围栏识别、编辑规则及 hello.scala，示例共 41 份。
独立预期覆盖普通引用不插值、嵌套插值、raw 的反斜杠/美元边界、美元转义引用、
代码引用与字符/symbol 区分、反引号及嵌套注释；长正文、连续闭合引号和随机分片通过。
`GOWORK=off go test ./...`、`GOWORK=off go test -race ./language ./internal/highlight` 通过，
Pango 亮暗 1× 三引号正文 RGBA 断言通过。独立浅色 1× 的 `-language scala` 通过
关键词建议、三引号输入、代码引用的结构配对及公共断言，没有生成多余的闭合单引号。

### Perl 与批次完成验证

新增 Perl、文件/shebang/围栏识别、编辑规则与 hello.pl，示例共 42 份。
独立预期覆盖变量、字面与插值引用、嵌套定界符、Unicode 自闭合定界符、
两段替换/转写、正则修饰符、锚点与特殊变量、方法/裸词、POD、数据区及多个 heredoc。
引用/替换/heredoc 结束后恢复代码颜色有独立关键词断言。
长正文、超长闭合名称、跨片段空白、极小分片、随机畸形输入、增量恢复、
状态快照、Class 和围栏通过；复用 heredoc 私有队列后的 Ruby 回归保持通过。
新增五语言的 Pango 亮暗 1× 原生颜色断言通过，同字体/同布局的独立预期保持 RGBA 零误差。

最终在 Ubuntu 22.04.5 amd64 / Go 1.25.8 下通过：

- `go test ./...`（工作区 GOUI b49049d）
- `GOWORK=off go test ./...`（独立依赖 GOUI v0.5.0）
- `GOWORK=off go test -race ./...`
- `GOWORK=off go vet ./...`
- `GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`
- `GOWORK=off GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`

关闭工作区构建语言窗口程序、输入着色回归程序与 editor。独立 TigerVNC :87 /
Xfwm4 compositor on / Pango / Software 的浅色 1×、深色 1.5× 均通过
41 种语言的 43 组建议接受/撤销、28 组引用输入及公共只读/括号/语义断言。
输入经 Window.DispatchEvent/CommitText；新增五语言的逐语言场景也单独通过。
既有着色回归每组通过 127 帧、60 次输入/删除及撤销重做，确认未回退当前行/后续行的显示稳定性。
editor 浅色 1× 实际点击五个新样本，深色 1× 检查 Scala/Perl；42 个样本按钮完整换行，
ChromeInfo 确认 Integrated enabled，未见正文或工具栏裁切。截图仅作外观观察。

本批二进制、日志和截图保留在 `/tmp/sourceview-language-batch4-5i1u39/`，未写入源码目录。
完成后关闭本次 editor、专用 X11 与窗口管理器，保留原有用户桌面；
没有 DevServer、剪贴板操作、语言样本执行或全局设置更改。
未执行真实 IME、macOS/Windows 原生验证；交叉构建只证明可编译，不代替原生验收。
没有 GOUI 或依赖改动；原有未跟踪 doc/Doing.md、example/temp/ 保留并排除于提交。
语言歧义和词法范围见设计，不承诺编译器解析或语言服务能力。

## 第五批语言（2026-11-02）

### Diff/Patch

新增文件/围栏识别、unified hunk 范围状态、增加/删除专用样式及 change.diff，示例共 43 份。
独立断言覆盖正文与文件头同形的 +++/---、省略行数、空旧范围、Git 元数据和缺少换行提示；
长行、长 hunk 头及极小分片对照通过。Pango 亮暗 1× 增删行 RGBA 断言通过。
首次 `GOWORK=off go test ./...` 的 TestRegisteredStylesAndTokenParts 在 styles_test.go:39
发现 diff.added 回退为普通前景色：新增样式尚未加入标准 token 部件；补齐 styles.go 后原断言及全仓复跑通过。

Ubuntu 22.04.5 amd64 / Go 1.25.8，`GOWORK=off go test ./...`（GOUI v0.5.0）通过。
独立 TigerVNC :87 / Xfwm4 compositor on / Pango / Software / 浅色 1× 下，
`-language diff` 通过补丁正文不生成配对和公共括号/语义/只读断言。
语言筛选现在也接受只有输入场景、没有建议场景的语言；未知筛选仍失败。
日志与产物在 `/tmp/sourceview-language-batch5-xOXtDs/`，未修改 GOUI 或依赖。
Windows Batch、HCL/Terraform、GraphQL、SCSS、Less 继续依次实现。

### Windows Batch

新增 .bat/.cmd、围栏、编辑规则、建议和 build.cmd，示例共 44 份。
独立断言覆盖大小写、命令 REM 与 echo 参数的差别、标签、变量/参数修饰、
双引号、^ 转义/续行及未闭合引用的真实行尾恢复；长行和极小分片通过。
`GOWORK=off go test ./...`、Pango 亮暗 REM 原生颜色断言通过。
同本批独立 X11 / Software / 浅色 1×，`-language bat` 通过 setlocal 建议接受/撤销、
反斜杠结尾路径的引用输入及公共断言。没有执行批处理或依赖 Windows 命令解释器。

### HCL / Terraform

新增 HCL/Terraform 文件/围栏、属性和模板状态、基础编辑/建议与 main.tf，示例共 45 份。
独立断言覆盖属性、嵌套插值、模板指令、字面起始标记、普通与缩进 heredoc、
嵌套 heredoc 及闭合后恢复代码；超长 Unicode 闭合名、长词/正文、极小分片和快照通过。
`GOWORK=off go test ./...`、Pango 亮暗 heredoc 正文 RGBA 断言通过。
同本批独立浅色 1×，`-language hcl` 通过 resource 建议接受/撤销、插值引用输入与公共断言。
闭合前后空白按 HCL 2.24.0 扫描器源码处理，文档与源码区别已记录；没有加载 Terraform 或 provider。

### GraphQL

新增文件/围栏、普通与块引用、变量/指令、Schema/查询词汇、基础编辑和 query.graphql，示例共 46 份。
普通/花括号 Unicode 转义、块引用中的转义三引号、变量/指令、名称、数字和真实行尾恢复有独立预期；
长正文、长名称与极小分片通过。`GOWORK=off go test ./...`、Pango 亮暗块正文 RGBA 断言通过。
同本批独立浅色 1×，`-language graphql` 通过 mutation 建议接受/撤销、三引号输入及公共断言。
没有 Schema 加载、网络请求或 GraphQL 执行。

### SCSS

新增 .scss、围栏、变量/指令、嵌套插值、URL、编辑规则和 theme.scss，示例共 47 份。
独立预期覆盖普通引用不直接插入 $ 变量、转义插值前缀、十六进制转义、URL 的 //、
嵌套选择器后恢复属性、块注释及内部注释/插值；长正文/名称/URL、极小分片与快照通过。
`GOWORK=off go test ./...`、`GOWORK=off go test -race ./language ./internal/highlight` 通过，
Pango 亮暗块注释正文 RGBA 断言通过，既有 CSS/JS/TS/Python/SQL 共享扫描器回归保持通过。
独立浅色 1×，`-language scss` 通过 @mixin 建议接受/撤销、引用中的插值输入及公共断言。

### Less

新增 .less、围栏、变量/指令/插值、基础编辑与 theme.less，示例共 48 份。
独立预期覆盖变量与已知 @ 指令、间接/属性变量、选择器/引用/URL 插值、
URL 的 //、反引号正文、嵌套规则后的属性恢复；长行/名称、快照和极小分片通过。
本批六种语言共 1,800 组固定随机畸形输入的分片对照通过，另有编辑后失效/恢复的独立预期。
独立浅色 1× 的 `-language less` 通过 @import 建议接受/撤销、引用插值、反引号输入与公共断言。

### 批次完成验证

Ubuntu 22.04.5 amd64 / Go 1.25.8，最终通过：

- `go test ./...`（工作区相邻 GOUI b49049d）
- `GOWORK=off go test ./...`（独立依赖 GOUI v0.5.0）
- `GOWORK=off go test -race ./...`
- `GOWORK=off go vet ./...`
- `GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`
- `GOWORK=off GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`

新增六语言的原生 Pango 亮暗 1× 颜色断言通过，使用同字体/同布局的独立前景色预期，
RGBA 零误差；另断言 Diff 增加/删除色分别以绿色/红色通道为主。
关闭工作区构建语言窗口程序、着色回归程序与 editor；独立 TigerVNC :87 /
Xfwm4 compositor on / Pango / Software 的浅色 1×、深色 1.5× 均通过
47 种语言的 48 组建议接受/撤销与 37 组引用/配对输入，以及公共只读、语义、括号断言。
Diff 没有建议场景；SCSS/Less 的代码选择器插值也验证了自动生成与手工闭合配合。
输入均经 Window.DispatchEvent/CommitText，无 DevServer。
既有着色回归每组通过 127 帧、60 次输入/删除和撤销重做，未回退当前行/后续行的显示稳定性。
未知语言筛选按预期以 1 退出并报告没有集成场景，避免空筛选产生假通过。

editor 浅色 1× 实际点击六个新样本，深色 1× 检查 Less/Diff；48 个样本按钮完整换行，
ChromeInfo 确认 Integrated enabled，没有观察到正文或工具栏裁切；截图仅作外观观察。
本批二进制、截图和日志在 `/tmp/sourceview-language-batch5-xOXtDs/`，源码目录没有生成产物。
结束时关闭本次 editor、专用 X11 和窗口管理器，保留既有桌面；没有剪贴板操作、语言样本执行或全局设置更改。
未运行真实 IME 或 macOS/Windows 原生场景，交叉构建只证明可编译。
没有新增依赖或修改 GOUI；保留原有未跟踪 doc/Doing.md、example/temp/，不纳入提交。
词法歧义和功能边界见设计，不承诺编译器、Schema/provider 解析或语言服务能力。

## 第六批语言（2026-11-02）

### Go Modules

新增 GoMod/GoWork/GoSum、明确文件名及围栏识别、相应编辑/建议与三份样本，示例共 51 份。
独立断言覆盖分组、版本、replace、retract 范围、原始路径和含 // 的校验值；长行/名称和极小分片通过。
`GOWORK=off go test ./...`（GOUI v0.5.0）及 Pango 亮暗 1× 颜色断言通过。
Ubuntu 22.04.5 amd64 / Go 1.25.8，独立 TigerVNC :87 / Xfwm4 compositor on / Pango / Software / 浅色 1×，
逐一运行 -language gomod/gowork/gosum，建议接受/撤销、引用和校验正文输入及公共语义/只读/括号断言通过。
日志及构建产物在 `/tmp/sourceview-language-batch6-LhHxde/`；没有 GOUI 或依赖改动，也不加载示例声明的模块。
其余 Dotenv、Properties、Git Ignore/Attributes、Starlark、Groovy 继续实现和验收。

### Dotenv

新增识别、围栏、键/值/引用/注释状态、编辑规则、export 建议与 settings.env，示例共 52 份。
独立预期覆盖变量保持字面文本、多行引用、引用/裸值的 #、双引号换行转义及路径尾反斜杠；长行和极小分片通过。
首次全仓检查的 TestConfigurationAndShellDetection 保留旧 .env=Plain 预期，按新增识别契约更新为 Dotenv；
`GOWORK=off go test ./...` 复跑和 Pango 亮暗多行正文颜色断言通过。
独立浅色 1× 的 -language dotenv 通过 export 建议接受/撤销、路径引用输入和公共断言。
Node.js 源码引用行为及未闭合引用的编辑恢复边界已记录，没有环境修改或 shell 执行。

### Java Properties

新增文件/围栏、逻辑行和转义状态、基础编辑与 messages.properties，示例共 53 份。
独立预期覆盖键的转义分隔符、奇偶反斜杠、键/值续行、续行注释字符、注释不续行、
续行前后的可选分隔符、Unicode 转义及 CRLF；长词/值和极小分片通过。
`GOWORK=off go test ./...`、Pango 亮暗续行正文颜色断言通过。
独立浅色 1× 的 -language properties 通过引号原样输入及公共断言；没有把引号解释为引用配对。

### Git Ignore

新增文件/围栏、模式分类和 rules.gitignore，示例共 54 份。
独立断言覆盖列首注释、转义 #/!、否定前缀和前导空格；glob、字符类、长行及极小分片通过。
`GOWORK=off go test ./...`、Pango 亮暗模式正文颜色断言通过。
独立浅色 1× 的 -language gitignore 通过方括号原样输入及公共断言，没有把模式当作代码配对。

### Git Attributes

新增文件/围栏、属性/宏分类、建议和 rules.gitattributes，示例共 55 份。
独立断言覆盖 C 引用路径、赋值/属性、宏、前导空白、行中 #、非否定模式及未引用空格边界；长行和分片通过。
`GOWORK=off go test ./...`、Pango 亮暗正文颜色断言通过。
独立浅色 1× 的 -language gitattributes 通过 text 建议接受/撤销、带空格路径原样输入及公共断言。
复用模式设施后的 Git Ignore 回归保持通过，没有读取/修改仓库属性或执行 Git 匹配。

### Starlark

新增文件/围栏、独立词汇、引用前缀规则与 rules.bzl，示例共 56 份。
独立预期覆盖 load、// 除法、非块注释、f 前缀不插值、raw/bytes、转义引用及三引号；长行和极小分片通过。
`GOWORK=off go test ./...`、Pango 亮暗三引号正文颜色断言通过。
独立浅色 1× 的 -language starlark 通过 return 建议、三引号与原始引用中的反斜杠/引号输入及公共断言。
Context.Literal 按原始引用的实际闭合语义设置，既有 Python 规则保持独立，没有执行构建或加载目标。

### Groovy

新增 Groovy/Gradle/Jenkinsfile/shebang、围栏、六类引用、GString、建议和 build.gradle，示例共 57 份。
独立预期覆盖短变量路径、嵌套插值、两种斜杠引用的转义/闭合、数字成员访问与范围、注解、Unicode 转义和三引号。
首次检查的 TestGuess 仍要求 build.gradle=Plain，按新增契约更新为 Groovy，保留 .gradle.kts 的 Kotlin 预期。
`GOWORK=off go test ./...` 复跑、Pango 亮暗正文颜色、长行/分片和快照通过。
独立浅色 1× 的 -language groovy 通过 def 建议接受/撤销、三引号、GString 和 dollar-slashy 原始路径输入及公共断言。
本批九种语言配置新增 2,700 组固定随机畸形输入分片对照，均通过；另有编辑后状态失效和分类恢复的独立预期。

### 批次完成验证

Ubuntu 22.04.5 amd64 / Go 1.25.8，最终通过：

- `go test ./...`（工作区相邻 GOUI b49049d）
- `GOWORK=off go test ./...`（独立依赖 GOUI v0.5.0）
- `GOWORK=off go test -race ./...`
- `GOWORK=off go vet ./...`
- `GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`
- `GOWORK=off GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`

九种新增语言配置的原生 Pango 浅色/深色 1× 颜色断言通过，以同字体、同布局的独立前景色预期比较，
RGBA 零误差。关闭工作区构建语言窗口程序、着色回归程序与 editor。
独立 TigerVNC :87 / Xfwm4 compositor on / Pango / Software 的浅色 1×、深色 1.5× 均通过
56 种语言配置的 54 组建议接受/撤销、49 组引用/配对输入，以及公共只读、语义和括号断言；
无建议的语言仅验证适用的原样输入或公共行为。输入经 Window.DispatchEvent/CommitText，无 DevServer。
既有着色回归每组通过 127 帧、60 次输入/删除和撤销重做，当前行和后续行显示稳定。

editor 浅色 1× 实际点击九个新样本，深色 1× 检查 Groovy、Dotenv、Properties、Git Attributes；
57 个样本按钮完整换行，ChromeInfo 确认 Integrated enabled，没有观察到正文或工具栏裁切。
截图仅用于外观观察；颜色和编辑正确性由独立断言验证。Go 模块样本使用 .sample 资源后缀，
显示名称去掉该后缀，避免嵌套模块影响 Go 的嵌入资源扫描。

二进制、截图和日志在 `/tmp/sourceview-language-batch6-LhHxde/`，源码目录没有生成产物。
结束时关闭本次 editor、专用 X11 和窗口管理器，保留原有 X110/X3/X42/X78 桌面。
没有执行语言样本、加载模块、操作剪贴板或修改全局设置；未运行真实 IME 或 macOS/Windows 原生场景，
交叉构建只证明可编译。本批没有新增依赖或修改 GOUI；保留原有未跟踪 doc/Doing.md、example/temp/，不纳入提交。
词法歧义、方言和版本边界见专题设计，不承诺编译器、构建求值或语言服务能力。

## 第七批语言（2026-11-02）

按每种语言单独提交；实现范围和参考版本见高亮设计第 38 节。

### 模板

新增 GoTemplate/Jinja2、明确后缀和围栏、动作/表达式/注释/raw 状态、相应编辑规则与两份样本。
NewGoTemplate/NewJinja2 显式接受纯文本或 HTML 宿主；混合源禁用纯标记的标签联动和折叠。
独立预期检查 HTML 属性、动作、script 行注释跨行恢复，以及被模板分开的 JS 单词不重新合并。
编辑器对 .gohtml 和 HTML 的 Jinja 复合后缀显式选择 HTML 宿主；通用 .tpl 保持普通文本。
分片、状态、编辑恢复、原生颜色断言与窗口输入/建议通过。

### 系统配置

新增 Systemd/DesktopEntry、unit 后缀和 .desktop/.directory、分节/键值与对应建议、两份样本。
验证 systemd 跳过注释行的续行、% specifier、Exec 美元变量、转义和 CRLF；
Desktop Entry 验证本地化键、分号、Exec 字段代码、普通值原样文本及不续行的区别。
分片、独立分类、编辑恢复、原生颜色与正常窗口输入/建议通过；没有安装 unit 或 Desktop Entry。

### 查询 DSL

新增 Nix/JQ/PromQL、后缀/围栏、独立词汇、编辑与三份样本。
检查 Nix 相对/绝对/查找路径、缩进引用特殊转义与嵌套插值；
jq 过滤器、变量/格式与字符串插值；PromQL 标签、时长、修饰词和跨行反引号引用。
分片、独立 Kind/Class、快照、编辑恢复、原生颜色与窗口输入/建议通过，没有求值或网络查询。

### 编程语言

新增 FSharp/Elixir/Erlang/Julia/Clojure/Haskell/OCaml、后缀/适用 shebang/围栏、独立词汇和七份样本。
检查 F# 逐字/三引号/多美元插值，Elixir sigil、Erlang atom/变量/字符，
Julia 非标准引用/插值/转置，Clojure 完整 #_ 形式（含嵌套丢弃、metadata、tagged literal 和 reader conditional），
Haskell 连字符后续符号（含 Unicode）与嵌套注释，OCaml 字符/类型变量、命名引用及注释内引用。
长命名定界符不按正文每字符重新拼接；不可变帧和片段引用不累计完整字符串正文。
原生颜色断言发现 F# 普通三引号漏入跨行状态，已修正并增加独立预期；
随机畸形输入发现 Elixir 将非 ASCII 字符当作 sigil 定界符而拆分 UTF-8，已修正并保留定向回归。
分片、独立分类/Context、快照、编辑恢复、原生颜色及窗口输入/建议通过。

### 验证与交付

Ubuntu 22.04.5 amd64 / Go 1.25.8，通过：

- `go test ./...`（工作区 GOUI b49049d）
- `GOWORK=off go test ./...`（独立依赖 GOUI v0.5.0）
- `GOWORK=off go test -race ./...`
- `GOWORK=off go vet ./...`
- `GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`
- `GOWORK=off GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`

十四种语言增加 4,200 组固定随机畸形输入分片对照；独立预期、长行/长定界符、
极小 UTF-8 分片、Class/Context、围栏、不可变快照和增量编辑后恢复均通过。
原生 Pango 浅色/深色 1× 颜色断言使用同布局、同字体独立前景色预期，RGBA 零误差。

关闭工作区构建窗口程序与 editor，独立 TigerVNC :87 / Xfwm4 compositor on / Pango / Software：
浅色 1×、深色 1.5× 完整语言程序均通过 70 种语言配置、68 组建议接受/撤销、63 组引用/配对输入，
以及公共只读、语义和结构配对断言。最终重编译后的 Haskell 场景另行通过。
输入走 Window.DispatchEvent/CommitText，没有 DevServer 或动作捷径。
既有着色稳定性回归两组均通过 127 帧、60 次输入/删除与撤销重做。

editor 使用 Integrated chrome（日志 mode=1 enabled=true），浅色 1× 实际点击十四份新样本；
深色 1× 检查 Jinja2、systemd、Nix、F#、Clojure、OCaml，71 个样本按钮完整换行，未观察到工具栏/正文裁切。
截图仅作外观观察；颜色与编辑结果由独立断言验证。
二进制、参考源码、截图与日志放在 `/tmp/sourceview-language-batch7-UB7CDu/`，没有生成源码目录产物。
没有运行语言样本、安装配置、读写剪贴板或更改全局设置；真实 IME 和 Windows/macOS 原生运行未执行，
交叉构建仅证明可编译。本批不修改 GOUI 或依赖，保留原有未跟踪 doc/Doing.md、example/temp/。
结束时关闭本次 editor、窗口验证程序、专用 X11 和窗口管理器，保留原有桌面。

## editor 样本下拉选择（2026-11-02）

以 GOUI v0.5.0 的 widgets.DropDown 替代多行按钮，显示语言名与文件名，保留 71 份样本的顺序、
Document 实例、文本及撤销历史。命令行文件仍作为首项；Find / Replace 保留同一行。
设计见高亮设计第 39 节，示例说明及 README 两张实际窗口截图同步更新。

Ubuntu 22.04.5 amd64 / Go 1.25.8，通过 `go test ./...`（工作区 GOUI b49049d）
及 `GOWORK=off go test ./...`（GOUI v0.5.0）；关闭工作区构建并运行 editor。
针对 editor 的 Windows amd64、macOS arm64（CGO_ENABLED=0、GOWORK=off）交叉构建均通过，
二进制写入同一临时目录；交叉构建不代表原生运行验收。
独立 TigerVNC :87 / Xfwm4 compositor on / Pango / Software，使用 xdotool 的真实窗口输入观察验证：

- 浅色 1×：展开列表后浏览 XML/HTML，再 Escape，当前仍为 JSON；
  编辑 JSON 添加 SAMPLE_MARKER，切换 XML 并直接输入 XML_MARKER，
  返回 JSON 时前一标记保留，Ctrl+Z 后恢复首字符 {，字节数由 824 恢复到原来的 811。
- 浅色 1×：滚轮移动可见条目，拖动滚动条到末尾，鼠标选择 OCaml 后正确显示 hello.ml；
  Home/End、方向键与 Enter 均能选择对应模型项。
- 深色 1.5×：实际窗口 1680×1170 像素；弹窗尺寸随缩放，滚轮、滚动条拖动和最后一项选择正常。
  命令行导入的 supplied-example.json 为首项，切换回该文件后直接输入 ZOOM_MARKER，
  字节数由 40 到 51，Ctrl+Z 后恢复原文及 40 字节；磁盘文件内容未改动。
- Find / Replace 在 HTML 的深色界面可正常打开，搜索 feature 显示两处高亮。
  截图来自 Integrated 已启用的真实窗口（日志 mode=1 enabled=true），仅截取应用边界；
  doc/images/editor-light.png 与 editor-dark.png 均为 1120×780 像素、1×，没有绘制额外 UI。

临时二进制、输入样本、操作记录与观察截图位于 `/tmp/sourceview-sample-dropdown-DbbY9Z/`。
没有使用 DevServer、操作剪贴板或修改全局设置；结束后关闭本次示例、专用 X11 和窗口管理器，
保留原有 X110/X3/X42/X78 桌面。Windows/macOS 原生运行和真实 IME 未执行；
本次只使用既有公开控件，没有修改 GOUI 或依赖，也没有改动原有未跟踪文件。

## 常见文本格式（2026-11-02）

### CSV（2026-11-02）

新增 CSV 的引用、转义引号、跨行字段及独立数据编辑规则；示例 export.csv。
独立依赖 `GOWORK=off go test ./...` 通过；新增独立词法预期、分片、编辑恢复和原生颜色断言。
独立 X11 / Xfwm4 / Pango / Software 浅色 1× 的 `-language csv` 通过正常事件输入及公共编辑断言。
本项未执行 Windows/macOS 原生验证；无 GOUI 或依赖改动。

### TSV（2026-11-02）

新增独立 TSV 分隔规则、原样引用编辑和 export.tsv 样本。
独立依赖 `GOWORK=off go test ./...` 通过；新增独立词法预期、分片、编辑恢复和原生颜色断言。
独立 X11 / Xfwm4 / Pango / Software 浅色 1× 的 `-language tsv` 通过正常事件输入及公共编辑断言。
本项未执行 Windows/macOS 原生验证；无 GOUI 或依赖改动。

### AsciiDoc（2026-11-02）

新增 AsciiDoc 文档标记、属性/引用、独立块状态及 guide.adoc 样本。
独立依赖 `GOWORK=off go test ./...` 通过；新增独立词法预期、分片、编辑恢复和原生颜色断言。
独立 X11 / Xfwm4 / Pango / Software 浅色 1× 的 `-language asciidoc` 通过正常事件输入及公共编辑断言。
本项未执行 Windows/macOS 原生验证；无 GOUI 或依赖改动。

### reStructuredText（2026-11-02）

新增 rST 标题装饰、指令、缩进 literal/注释和 guide.rst 样本。
独立依赖 `GOWORK=off go test ./...` 通过；新增独立词法预期、分片、编辑恢复和原生颜色断言。
独立 X11 / Xfwm4 / Pango / Software 浅色 1× 的 `-language rst` 通过正常事件输入及公共编辑断言。
本项未执行 Windows/macOS 原生验证；无 GOUI 或依赖改动。

### TeX/LaTeX（2026-11-02）

新增 TeX/LaTeX 命令、数学和原样环境、命令建议及 paper.tex 样本。
独立依赖 `GOWORK=off go test ./...` 通过；新增独立词法预期、分片、编辑恢复和原生颜色断言。
独立 X11 / Xfwm4 / Pango / Software 浅色 1× 的 `-language latex` 通过正常事件输入及公共编辑断言。
本项未执行 Windows/macOS 原生验证；无 GOUI 或依赖改动。

### BibTeX（2026-11-02）

新增 BibTeX 条目、嵌套/跨行字段、拼接和 references.bib 样本。
独立依赖 `GOWORK=off go test ./...` 通过；新增独立词法预期、分片、编辑恢复和原生颜色断言。
独立 X11 / Xfwm4 / Pango / Software 浅色 1× 的 `-language bibtex` 通过正常事件输入及公共编辑断言。
本项未执行 Windows/macOS 原生验证；无 GOUI 或依赖改动。

### gettext PO/POT（2026-11-02）

新增 PO/POT 翻译字段、续行引用、复数与特殊注释及 messages.po 样本。
独立依赖 `GOWORK=off go test ./...` 通过；新增独立词法预期、分片、编辑恢复和原生颜色断言。
独立 X11 / Xfwm4 / Pango / Software 浅色 1× 的 `-language po` 通过正常事件输入及公共编辑断言。
本项未执行 Windows/macOS 原生验证；无 GOUI 或依赖改动。

### EditorConfig（2026-11-02）

新增独立 EditorConfig glob/键值规则、无行尾注释断言及 settings.editorconfig 样本。
独立依赖 `GOWORK=off go test ./...` 通过；新增独立词法预期、分片、编辑恢复和原生颜色断言。
独立 X11 / Xfwm4 / Pango / Software 浅色 1× 的 `-language editorconfig` 通过正常事件输入及公共编辑断言。
本项未执行 Windows/macOS 原生验证；无 GOUI 或依赖改动。

### JSON5（2026-11-02）

新增 JSON5 独立词法、不可变结构状态、特殊数值、单引号与行连接及 settings.json5 样本。
首次增量检查发现悬空 / 没有取得扫描进展，已补齐消费并保留定向用例；原生颜色样本修正为完整、颜色一致的物理行。
常见格式另有固定随机畸形输入的整行/单字符预算分片对照，保留独立词法预期与原生颜色断言。
独立依赖 `GOWORK=off go test ./...` 通过；新增独立词法预期、分片、编辑恢复和原生颜色断言。
独立 X11 / Xfwm4 / Pango / Software 浅色 1× 的 `-language json5` 通过正常事件输入及公共编辑断言。
本项未执行 Windows/macOS 原生验证；无 GOUI 或依赖改动。

### EDN（2026-11-02）

新增 EDN 数据 profile，分离 Clojure 关键字与 reader 宏，共用数据形式扫描并增加 data.edn 样本。
独立依赖 `GOWORK=off go test ./...` 通过；新增独立词法预期、分片、编辑恢复和原生颜色断言。
独立 X11 / Xfwm4 / Pango / Software 浅色 1× 的 `-language edn` 通过正常事件输入及公共编辑断言。
本项未执行 Windows/macOS 原生验证；无 GOUI 或依赖改动。
