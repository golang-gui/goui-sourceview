# 语言 profile 折叠第六批验收（2026-11-08）

## 范围与实现

基线 `357fa74`，功能提交 `19726b8`，依赖 GOUI v0.7.0。
新增 Objective-C、Objective-C++、SCSS、Less、Perl 的内置折叠，
设计与官方语法参照见 [代码折叠](DesignFolding.md)。
复用 structuralFolding，不增加公共 API、依赖或 GOUI 修改。

- Objective-C 两种 profile 复用 C 扫描器观察代码括号、字面量和块注释。
  另在声明深度配对 interface／implementation／协议定义和 end，包括分类／扩展。
  协议候选等待后续 token；前向声明和 protocol 表达式不制造区域或丢弃外层声明。
  宏及续行不参与代码／声明配对，Objective-C++ 沿用原始字符串规则。
- SCSS/Less 复用 codeTokenizer 的观察路径，处理规则／mixin／控制块等代码括号、
  字面量和注释。SCSS 插值保留外层引号／注释，内层独立观察；Less 变量插值
  不变成代码块，反引号只作为字面量，不执行内嵌 JavaScript。
- Perl 在原扫描器增加私有边界观察。引用操作符候选可回退；成对／非成对的
  多段替换等待第二段和修饰符结束，heredoc 以实际标签身份配对，POD 等待 cut。
  数据尾部与名称位置的伪操作符不参与配对。

状态由本次 worker 所有，不进入不可变 State 或共享高亮缓存。
公开 Scan 仍传 nil 观察器；Document 取消／修订核验、视图状态与编辑历史契约不变。
本批按起点归并、同起点外层优先并过滤交叉范围，关闭行后的其他正文仍保持可见。

## 逻辑验证与失败记录

`language/folding_profiles_test.go` 使用固定文本与独立精确范围：

- 注册前五种公开定义均确定失败于 `builtin has no profile folding provider`，
  记录于 `registration-before.log`；接入后通过。
- Objective-C 类／分类／扩展／协议、跨行协议头、多个前向声明、protocol 表达式、
  方法体和字面量中的伪指令、未完成头、宏续行、Objective-C++ raw 字符串、
  普通续行字符串、相邻注释与关闭行后的其他代码。
- SCSS 嵌套规则、选择器／引号／URL／注释插值、转义伪插值、嵌套注释及未完成插值；
  Less detached ruleset、变量插值、URL 中的双斜线、反引号与普通字符串续行。
- Perl 代码块、嵌套引用分隔符、跨行操作符、普通名称回退、两种多段替换、
  正则修饰符、修饰符后的其他语句、transliteration、未完成第二段、相邻字面量、
  第二段前的注释、多个／缩进 heredoc、POD／未闭合 POD、数据尾部和名称位置伪操作符。
- 五种语言的未闭合／错配／关闭行后正文；八路并发复用 provider，边界位于
  4090..4097 字节，正文 9000 字节；范围／状态隔离、长行扫描途中取消。
- Perl heredoc 标签长 5000 字节并跨分片；五种语言以 1..17 字节预算逐步比较
  公开 Scan 和观察路径的 token、Consumed、ReplaceFrom 与 State.Equal，包含 Unicode。

实现期间补测发现：声明深度的 `@protocol(...)` 会清掉已有 implementation，
实际得到空范围而应为 `[{0 2}]`（`objc-expression-before.log`）。
改为待确认协议候选，只有确认定义才替换当前声明；表达式／前向声明保留外层。
该用例及方法体内表达式、前向声明保留外层的补充用例均通过。

初次完整语言回归有一个预期写错：内层注释关闭行还有 `inspect(1)`，
按原有结束行规则不能将这一行隐藏，因此不能产生起止同为 1 的内层区域。
保留跟随代码的样本并断言只有外层 `[{0 4}]`，另加独占关闭行样本，明确断言
外层和内层 `[{0 5} {1 2}]`。初次输出保留在 `language-initial.log`，
最终全语言通过记录为 `language-test.log`；没有更改生产结束行规则以迁就预期。

没有运行 Objective-C 编译器、Sass/Less 或 Perl，不将词法范围样本当作完整语法验证。

## 本机独立桌面

Linux amd64 / Go 1.25.8 / Pango；TigerVNC 1.12.0，24-bit，2560×1800，
Xfwm4 4.16.1，专用 Xauthority、D-Bus 和临时 XDG config/cache。
所有程序使用 `GOWORK=off` 构建，窗口探针记录实际 painter 与 PixelScale。

新增 `tests/window/editor/folding_profiles`，每组八个文档：Objective-C 实现／协议、
Objective-C++ raw 字符串、SCSS 规则／字符串插值、Less detached ruleset 与反引号、
Perl heredoc／多段替换。每份两视图共享 Document，其中一个只读。
通过正常 Window.DispatchEvent 模拟键盘／指针，断言精确异步范围、折叠／展开、
隐藏文本快照、搜索展开及真实源字节选区；折叠不改正文／历史，视图状态独立。
边界第一次 Delete 仅展开，第二次编辑，Undo 恢复。等待两视图候选就绪后检查只读折叠。
删除外层关闭 token 后，必须只剩精确预期的内层范围；单一区域协议文档应变成空结果。
Undo 后等待原候选恢复，切换文档同时覆盖旧任务取消与脱离。

| 场景 | 实际后端／缩放 | 结果 |
| --- | --- | --- |
| 第六批八个文档，Basic | Software 1× | 通过 |
| 第六批八个文档，Modern Light | Software 1.5× | 通过 |
| 第六批八个文档，Modern Dark | Software 2× | 通过 |
| 第六批八个文档，Modern Light | OpenGL 1× | 通过 |
| 第六批八个文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第五批七个文档，Basic | Software 1× | 通过 |
| 第五批七个文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第四批五种语言，Basic | Software 1× | 通过 |
| 第四批五种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 第三批混合文档，Basic | Software 1× | 通过 |
| 第三批混合文档，Modern Dark | OpenGL 1.5× | 通过 |
| 前两批十一种语言，Basic | Software 1× | 通过 |
| 前两批十一种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 原有 JSON/HTML folding，Modern Dark | OpenGL 2× | 通过 |

完整 14 组均通过，无窗口断言失败；结果为 `desktop-84vox0q0/results.json`，
汇总日志为 `desktop.log`，各场景完整输出在同一桌面子目录。
OpenGL 通过不宣称硬件 GPU 验收；模拟输入不等同物理键盘或 IME 验收。
所有应用及本次专用 X11/WM/D-Bus 会话已退出，认证文件已清理；
没有操作用户桌面、用户文件、剪贴板或系统设置。

## 命令、提交与限制

日志与仓库外导出位于 `/tmp/sourceview-folding6-EmBYpieJ/`，全部命令使用 `GOWORK=off`：

- `go test ./... -count=1`：通过（`go-test.log`）；两个提交各自的独立索引树导出
  同样通过（`core-tree-test.log`、`final-tree-test.log`）。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：通过（`race.log`）。
- `go vet ./...`：通过（`vet.log`）；两个提交独立导出也通过
  （`core-tree-vet.log`、`final-tree-vet.log`）。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、
  `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：交叉构建通过，
  分别见 `windows-build.log`、`darwin-build.log`，未在这两个平台运行原生桌面。

基础与窗口构建日志另有 `base-test.log`、`profiles-test.log`、`native-build.log`。
两个独立导出为 `commit-core`、`commit-final`，均未包含原有草稿／临时示例；
最后补写验收结果仅修改本记录，没有改变已验证源码。
本轮全仓及两个独立导出未重现前几批记录的 semantic 测试进程启动段错误，
不据此声称其原因已确认或问题已修复。

物理 IME、混合 DPI、性能对照与其他平台原生后端未执行。
Scala 3 缩进块、其余尚未具备专用规则的语言仍是后续工作；本批不解析全部编译器语法、
预处理结果、Sass/Less 求值、Perl 正则执行语义或 format 正文。
没有发现本轮需要修改或绕过 GOUI 的功能缺口，没有推送、发布或变更版本号。
按功能与窗口验证拆分英文提交，不纳入原有草稿或未跟踪临时示例。
