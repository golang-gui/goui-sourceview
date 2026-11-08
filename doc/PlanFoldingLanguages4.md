# 关键字语言折叠第四批验收（2026-11-08）

## 范围与实现

基线 `0962a13`，功能提交 `0024945`，依赖 GOUI v0.7.0。
新增 Lua、Ruby、Shell、CMake、SQL 的内置 keywordFolding，设计与官方语法参照见
[代码折叠](DesignFolding.md)。不增加公开 API、依赖，不修改 GOUI。

- Lua：function、条件／循环／do、repeat/until 条件、括号及闭合跨行字面量／注释。
  循环头不重复开 do；until 等待非空条件、平衡的括号／函数和续行运算符。
- Ruby：end 块、引号／percent 字面量及插值、heredoc 队列、块注释。
  区分前置控制与后置修饰、成员名称、endless 方法、普通默认参数和 setter。
  heredoc 终止时恢复挂起状态，以受保护的栈前缀隔离未完成的插值内部观察。
- Shell：命令位置的 if/fi、循环/done、case/esac、代码括号和命令替换。
  区分普通参数、分支模式、参数／算术展开和 heredoc 正文，支持多 heredoc 队列及 <<-。
- CMake：确认的顶层控制命令配对，跨行参数及闭合引号／bracket 字面量／注释。
  同名参数、变量和字面量不当作命令。
- SQL：CASE/END、括号、实际闭合的多行引号／dollar 字符串及嵌套块注释。
  不将事务 BEGIN/COMMIT 或存储过程方言当作通用 BEGIN/END；限定结束词避免错误配对。

控制和边界观察由单次 worker 所有，不写入不可变 Tokenizer State 或 GUI 缓存。
沿用同起点外层优先、交叉范围过滤、关闭行尾其他语句保留，以及原 Document 修订／取消机制。
SQL 分词有一项公开行为修正：物理 LF 清除 dollar 结束标签的部分匹配，
避免把两行文本拼成伪关闭标签；分片之间仍保存进度。

## 逻辑回归与修复证据

`language/folding_keywords_test.go` 使用公开内置 provider、固定源文本与独立精确范围预期：

- 五种语言的 provider 注册；注册前的新回归确定失败于
  `builtin has no keyword folding provider`，其余结构测试已可通过。
- Lua 分支、嵌套循环、匿名函数、repeat 的跨行／空条件、未完成头、长字符串／注释、
  转义空白续行和相邻字面量。
- Ruby 后置修饰、条件赋值、可选 do、嵌套 do、紧凑／带空格的 endless 方法、
  默认参数、setter、关键字方法名、条件默认值、成员伪关键字、percent／嵌套插值、
  heredoc 队列／嵌套／错误恢复、块注释及 __END__ 数据。
- Shell 普通参数伪关键字、分支／循环、case 扩展模式和三种分支结束符、
  `$()`／反引号／引号内命令替换、参数及算术伪关键字、引号／续行、heredoc 队列、
  嵌套替换、关闭行后的其他命令、未完成头与错配。
- CMake 大小写命令、跨行参数、参数中的伪控制、错误结束命令、未完成括号、
  bracket 字面量／注释及相邻引号；SQL 嵌套 CASE、别名关闭行、事务、
  未支持的过程控制、限定 END、引用标识符、dollar 字符串与嵌套注释。
- 五种语言的边界位于 4090..4097 字节，Ruby/Shell heredoc 与 SQL dollar 标签长达
  5000 字节；八路并发使用同一 provider 的范围／状态隔离和长行扫描途中取消。

补充边界回归时有两个真实失败，均保留原始失败日志并修正后通过：

1. SQL `$tag$` 的关闭部分分布在两条物理行，原来错误输出 `[{0 2}]`，
   正确范围为 `[{0 4}]`。修正物理换行的部分匹配；另用公开 Scan 断言正文始终为
   ClassString、伪结束后不暴露 CASE/END，真实结束标签才退出字符串。
2. Ruby heredoc 的未完成插值字符串被恢复丢弃时，折叠观察错误地用其开头配对
   外层关闭引号，输出 `[{0 2}]` 而应为 `[{0 3}]`。记录 heredoc 激活深度，
   丢弃其临时观察；另增加外层 class 与字符串、内部未闭合 if 的恢复回归，
   保护控制／括号栈前缀并恢复元数据。两个恢复样本均通过。

没有执行语言编译器、Shell 脚本、SQL 数据库或 CMake 配置，不把词法样本当作完整语法验证。

## 本机独立桌面

Linux amd64 / Go 1.25.8 / Pango；TigerVNC 1.12.0，24-bit，2560×1800，
Xfwm4 4.16.1 compositor，专用 Xauthority、D-Bus 和临时 XDG config/cache。
所有程序使用 `GOWORK=off` 构建，窗口内探针核对实际 painter 与 PixelScale。

新增 `tests/window/editor/folding_keywords`。每组运行五种语言的固定文档，
两视图共享模型，其中一个只读。通过正常 Window.DispatchEvent 验证：
精确异步候选、键盘折叠／展开、指针折叠字面量／注释、隐藏源文本快照、
搜索展开与精确源字节选区；折叠不改正文／历史、视图状态相互独立。
外层边界第一次 Delete 只展开，第二次编辑，Undo 恢复；等待候选就绪后验证只读折叠。
删除外层关闭 token 后，两视图必须只剩各自预期的内层范围；Undo 后再次等到原始范围。
切换文档也覆盖旧分析任务的取消与脱离。

| 场景 | 实际后端／缩放 | 结果 |
| --- | --- | --- |
| 第四批五种语言，Basic | Software 1× | 通过 |
| 第四批五种语言，Modern Light | Software 1.5× | 通过 |
| 第四批五种语言，Modern Dark | Software 2× | 通过 |
| 第四批五种语言，Modern Light | OpenGL 1× | 通过 |
| 第四批五种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 第三批混合文档，Basic | Software 1× | 通过 |
| 第三批混合文档，Modern Dark | OpenGL 1.5× | 通过 |
| 前两批十一种语言，Basic | Software 1× | 通过 |
| 前两批十一种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 原有 JSON/HTML folding，Modern Dark | OpenGL 2× | 通过 |

初次完整 10 组通过；补齐 Ruby heredoc 恢复保护后的最终生产源码重新构建，
完整 10 组再次通过。初次与最终结果分别为
`desktop-qnuao7xh/results.json`、`desktop-zh7vg2ww/results.json`。
这两次均无窗口断言失败，不以窗口启动替代实际场景断言，也不宣称 OpenGL 是硬件 GPU 验收。
所有应用及本次专用 X11/WM/D-Bus 会话已退出；未操作用户桌面、文件、剪贴板或系统设置。

## 命令与限制

全部命令使用 `GOWORK=off`，日志保存在 `/tmp/sourceview-folding4-wlUAEnzO/`：

- `go test ./... -count=1`：最初启用注册后的全仓通过（`go-test.log`）；
  最终源码的工作目录全仓运行有一次 semantic 进程段错误（`go-test-final.log`，下述）；
  功能与窗口验证两个提交各自的独立索引树导出全仓测试均通过
  （`core-tree-test.log`、`final-tree-test.log`）。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：
  最终源码通过（`race-final.log`）；`go test ./language -count=1` 通过（`language-final.log`）。
- `go vet ./...`：最终源码及两个提交的独立导出通过
  （`vet-final.log`、`core-tree-vet.log`、`final-tree-vet.log`）。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、
  `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：最终源码交叉构建通过，
  日志分别为 `windows-build.log`、`darwin-build.log`。

失败证据分别为 `registration-before.log`、`sql-dollar-before.log` 和
`ruby-recovery-before.log`。窗口日志汇总为 `desktop.log`、`desktop-final.log`，
各场景完整日志在上述桌面子目录。两个独立导出为 `commit-core`、`commit-final`；
导出不包含原有草稿或未跟踪临时示例。最终补写本段验收结果只改变文档，不再改变已验证源码。

最终工作目录全仓测试中，`example/semantic/analysis` 进程在约 0.002 秒报告
`signal: segmentation fault (core dumped)`，没有 Go 堆栈；该次运行记为失败。
现象与第一批的历史记录相同，原因和归属仍未确认。本次该包单独运行 20 遍通过；
仓库外导出的原始基线与当前代码各自构建后，20 次独立进程启动也均通过，
分别见 `semantic-repeat.log`、`semantic-processes.log`。没有修改 semantic 示例、
增加 skip 或删弱断言，不将未复现表述为已修复。

Windows/macOS 仅交叉构建，未运行原生桌面；物理 IME、混合 DPI、性能对照未执行。
Scala 3 缩进块及其他尚未具备专用规则的语言仍是后续工作；SQL 存储过程／方言 BEGIN 块
不在本批范围。没有发现本轮需要修改或绕过 GOUI 的功能缺口。
按功能实现与窗口验证拆分提交，不发布或推送，不纳入原有草稿与临时示例。
