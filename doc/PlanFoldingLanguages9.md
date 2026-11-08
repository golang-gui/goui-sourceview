# F# 与 Haskell 布局折叠第九批验收（2026-11-08）

## 范围与实现

基线 `9d457df`，功能提交 `edd0026`，显式结束符修正 `8a605f3`，依赖 GOUI v0.7.0。
新增 F#、Haskell 的内置 Folding provider，规则与官方参照见
[代码折叠](DesignFolding.md)。继续复用 structuralFolding、现有扫描器和范围归并，
不增加公共 API、第三方依赖、通用缩进回退或 GOUI 修改。

- F#：确认等号的声明、确认 then/do/with/箭头的控制头与正文；同列模式分支和
  else／elif／with／finally 保留外层，分支可独立折叠。显式 begin/class/sig/end、
  循环 done、let/in、;; 和实际代码括号限定范围。#light off/on 切换后续缩进推断。
- Haskell：where/let/do/of（及 mdo）后的首个非注释 token 确定布局列，同列保持
  列表、退缩进退出；显式花括号不重复建隐式候选，也不因内部缩进关闭外层布局。
  匹配的 in、分组关闭和逗号结束对应隐式表达式，显式内层 let 不抢占外层 in。
  Tab 按下一个 8 列边界计算，语法列与 UTF-8 源字节位置分开保留。
- 观察扫描器实际关闭的多行字面量和嵌套注释；注释／字符串中的伪关键字和括号不入栈。
  F# 插值使用独立控制状态，多 dollar 边界按 arity 匹配；双反引号名称不当作代码。
  Haskell string gap 保留两个反斜杠之间的空白、LF 和空行。

布局栈及观察器均由当前 worker 所有，不进入不可变 State 或 GUI 高亮缓存。
公开扫描调用继续使用 nil 观察器；fnState 只增加私有 gap 标记与 quoted-name 模式。
候选归并、同起点外层优先、关闭行尾保留、修订／取消和视图隔离沿用原有机制。
EOF 结束已有正文的隐式区域；未确认的头、空正文和未关闭字面量不发布对应区域。

## 逻辑验证与失败记录

`language/folding_layout_test.go` 使用固定源码、独立精确范围和公开 token／Context 断言：

- F#：嵌套绑定、模块／类型／成员、跨行参数头、match 分支、if／elif／else、try／
  with／finally、循环、递归 and、lambda／function、union case、显式结构、let/in、
  真实括号、#light off/on、普通／verbatim／三引号字符串和多 dollar 插值。
  包含嵌套注释、注释内特殊字符串、注释／字符串内容中的 Tab、双反引号名称，
  以及缺少等号／正文、普通缩进、源码缩进 Tab 和未闭合字面量。
- Haskell：同列 do 条目、嵌套 case／do、同／跨行 let/in、do 中的 let 声明、
  module／普通 where、显式与隐式混用、分组及逗号结束、显式／隐式内层 let、
  Tab 与 Unicode 语法列、注释、string gap／普通引号错误恢复、空布局和未完成头。
- 两种 provider 八路并发复用，起止边界位于 4090..4097 字节、正文长 9000 字节，
  另检查长行扫描中途取消。1..17 字节预算逐步比较公开／观察 Scan 的 token、
  Consumed、ReplaceFrom 与 State.Equal，涵盖 quoted name、Unicode 和多 dollar 插值；
  同时回归共享扫描器的 Elixir、Erlang、Julia、OCaml、Clojure、EDN。

实现前，两个注册断言确定失败；F# 普通／verbatim 字符串与 Haskell string gap
在首行后的 Context.String 也确定失败，见 `registration-lexical-before.log`。

首轮范围测试的两项失败保留在 `layout-initial.log`：

1. F# for ... in 中的 in 错误关闭外层 let，丢失 `{0 4}`；现区分尚在等待 do 的 for 头，
   保留原精确预期。
2. 跨行参数头误期望 `{1 2}`；两个参数各自都是单行括号，不应合并成区域。
   修正该项预期为仅外层 `{0 3}`，不改变生产括号规则或删弱既有断言。

后续边界回归还复现并修正：

- F# 多 dollar 的部分结束花括号跨 LF 拼接，Context.String 在第二个单独关闭符处
  提前变为 true。真实行末清除部分匹配，仍允许同一行的扫描片段间保留进度。
  初次失败见 `layout-boundary-before.log`。
- Haskell do 在逗号后继续吞入对齐的数组元素，得到 `{1 4}`，应为 `{1 2}`。
  增加表达式逗号终止，保留同列列表条目的原规则，见 `layout-comma-before.log`。
- F# 多行注释内的 Tab 被当作源码缩进，丢失声明区域；注释内容不触发该错误恢复。
  见 `layout-name-comment-before.log`。
- F# 双反引号名称原本拆出 keyword 和 ClassDelimiter。范围断言可能被同起点归并
  掩盖，因此另加公开 token 断言：名称须为完整 Property token，内部不出现伪代码。
  仅用基线 scan_functional.go 的仓库外 overlay 确定失败，见 `quoted-name-baseline.log`。

收尾复核补充的显式结束符断言也确定失败：内层绑定 `{1 4}` 应为 `{1 3}`，
内层条件 `{1 3}` 应为 `{1 2}`（`outer-terminator-before.log`）。closeAt 只让匹配的
外层拥有 end/done 所在行，隐式子区域保留该行可见；新增窗口文档直接断言折叠内层时
visibleSpans 仍含外层 end。该修正单独提交，相关源码与窗口矩阵再次验证。
新窗口首验误用 VisibleText 检查非连续区域，失败于 `inner fold hid outer end`
（`desktop-release.log`）。现按既有快照契约读取 visibleSpans：保留外层 end 可见性
断言，并逐个片段检查隐藏正文没有泄漏；没有修改生产快照或放宽可见性要求。

同一基线 overlay 的公开字符串／插值 Context 断言也确定失败（`lexical-baseline.log`）。
F# 注释内普通／verbatim／三引号按规范完整跳过，保护伪关闭符；Haskell 缺少 gap 的
普通字符串错误恢复撤销未关闭观察。最终完整语言测试通过（`language-final.log`），
核心提交独立导出通过当时的全部断言；显式结束符修正后全仓及独立导出也通过
（`terminator-test.log`、`terminator-tree-test.log`）；最终窗口／记录提交独立导出也通过（`release-tree-test.log`）。
本轮未增加 skip、删除既有断言或以词法检查代替编译器语法验收。

## 本机独立桌面

Linux amd64 / Go 1.25.8 / Pango；TigerVNC 1.12.0，24-bit、2560×1800，
Xfwm4 4.16.1，专用 Xauthority／D-Bus 和临时 XDG config/cache。
所有程序以 GOWORK=off 构建，窗口探针检查实际 painter 与 PixelScale。

新增 `tests/window/editor/folding_layout`，每组八份文档：F# 显式结束符可见性／绑定／同列模式分支及
quoted name／多 dollar 插值／try-finally+嵌套注释，Haskell do+case+string gap／
module where／显式与隐式布局混用。每份两个视图共享 Document，其中一个只读。

正常 Window.DispatchEvent 模拟键盘与指针，断言异步精确范围、折叠／展开、隐藏文本快照、
搜索展开及真实字节选区；折叠不修改正文／历史，各视图状态独立。
第一次边界 Delete 只展开，第二次编辑，Undo 恢复；候选就绪后检查只读折叠。
删除外层声明／布局引导词或显式终止行，两个视图只剩独立预期的内层区域，Undo 恢复。
F#／Haskell 各一份通过正常 Delete 删除行首缩进，检查两个视图的新范围
`{0 2} {1 2} {4 6}`，再 Undo 恢复原候选。文档切换覆盖任务取消／脱离。

| 场景 | 实际后端／缩放 | 结果 |
| --- | --- | --- |
| 第九批八份文档，Basic | Software 1× | 通过 |
| 第九批八份文档，Modern Light | Software 1.5× | 通过 |
| 第九批八份文档，Modern Dark | Software 2× | 通过 |
| 第九批八份文档，Modern Light | OpenGL 1× | 通过 |
| 第九批八份文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第八批八份文档，Basic | Software 1× | 通过 |
| 第八批八份文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第七批九份文档，Basic | Software 1× | 通过 |
| 第七批九份文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第六批八份文档，Basic | Software 1× | 通过 |
| 第六批八份文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第五批七份文档，Basic | Software 1× | 通过 |
| 第五批七份文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第四批五种语言，Basic | Software 1× | 通过 |
| 第四批五种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 第三批混合文档，Basic | Software 1× | 通过 |
| 第三批混合文档，Modern Dark | OpenGL 1.5× | 通过 |
| 前两批十一种语言，Basic | Software 1× | 通过 |
| 前两批十一种语言，Modern Dark | OpenGL 1.5× | 通过 |
| 原有 JSON/HTML folding，Modern Dark | OpenGL 2× | 通过 |

原始七份文档的 Basic 初验及第一次 20 组回归通过；补充 quoted-name／注释 Tab 修正后，
所有窗口程序重新构建；一次启动失败的证据与复验见下节，最终完整 20 组全部通过。
该轮为 `desktop-dl38_btj/results.json`／`desktop-verified.log`。随后补充显式结束符修正及
第八份文档，再次重建九个窗口程序；修正窗口断言后最终完整 20 组全部通过，
见 `desktop-acceptance.log`、`desktop-z566gfs4/results.json`。程序字节与预检 SHA-256
逐项一致，实际后端与缩放逐项核对，所有专用会话和认证均已清理。
实际执行见对应日志，
不将 OpenGL 通过表述为硬件 GPU 验收，模拟输入不等同物理键盘／IME 验收。
全部本次应用、专用 X11／WM／D-Bus 会话已退出，Xauthority 已清理；
没有操作用户桌面、用户文件、剪贴板或系统设置。

## 临时构建产物异常与复验

最终源码的首次完整复验（`desktop-final.log`、`desktop-o6qoidbf/results.json`）中，
新增五组全部通过，第六项 functional-basic 在 Go internal/godebug.init 中 SIGSEGV，
尚未进入应用 main。该运行不计为整套通过。

同一失败二进制单独 `-h` 启动 20/20 返回 2，上一批基线二进制 20/20 返回 0，
见 `startup-comparison.json`；因此没有把它简单标为一次随机窗口失败。
gdb 栈与寄存器保存于 `startup-gdb.log`，失败二进制保留在 `bad-bin/folding_functional`。
反汇编比较发现，在 Go 标准库 cmpbody 的地址 0x4044df，失败产物为 `eb00`，
未改源码重新构建的产物为源码所对应的 `eb08`，跳转目标分别为 0x4044e1／0x4044e9。
见 `cmp-broken.asm`、`cmp-rebuilt.asm`；两个产物的构建 ID 前三段相同，最终内容段不同。
坏产物 SHA-256 为 `ff27c3a0402da4de31b12bec6e72f6b5041c46d416dcccaf02affcc40ca0b070`，
重建产物为 `206af598fc25daecadcab5c8680cd0d3f78e29aaa58347bd0f18102c05d8a0e9`。

这是对临时产物指令字节差异的实际观察，尚不能确定差异产生原因；没有据此声称
Go 工具链、硬件、GOUI 或 SourceView 的某个源码问题已修复。
重建命令为 `GOWORK=off go build -o .../functional-rebuilt ./tests/window/editor/folding_functional`，
不修改源码、依赖、全局环境或构建缓存配置。重建后单独启动 20/20 返回 0
（`rebuilt-startup-comparison.json`），对应完整窗口场景通过
（`desktop-functional-rebuilt.log`）；九个最终窗口程序的启动及 SHA-256
保存于 `native-preflight.json`。
发现固定启动失败后主动中断的重跑记录为 `desktop-recheck.log`，不计通过；
使用重建产物再次执行完整矩阵的 20/20 场景全部通过，见 `desktop-verified.log`，
实际后端／缩放已逐项核对。
坏产物、重建产物和所有日志均在仓库外，不随源码提交。

## 命令、提交与限制

仓库外日志／导出位于 `/tmp/sourceview-folding9-gb4b2btm/`，命令均使用 GOWORK=off：

- `go test ./... -count=1`：初验通过（`go-test.log`）；最终源码同样通过（`go-test-final.log`），
  核心独立导出通过（`core-tree-test.log`）；结束符修正后全仓和独立导出也通过
  （`terminator-test.log`、`terminator-tree-test.log`）；最终窗口／记录提交独立导出也通过（`release-tree-test.log`）。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：初验通过（`race.log`）；
  补充修正后同样通过（`race-final.log`）；结束符修正后语言包 race 也通过（`terminator-race.log`）。
- `go vet ./...`：初验、最终源码及核心独立导出通过
  （`vet.log`、`vet-final.log`、`core-tree-vet.log`）；结束符修正后全仓及独立导出也通过
  （`terminator-vet.log`、`terminator-tree-vet.log`）；最终窗口／记录提交独立导出也通过（`release-tree-vet.log`）。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...` 与
  `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：初验通过；最终源码分别见
  `windows-final-build.log`、`darwin-final-build.log`，均通过；结束符修正后的两平台构建
  也通过（`terminator-windows-build.log`、`terminator-darwin-build.log`）。未执行对应平台原生桌面。

基础、初次窗口和全部最终窗口构建分别见 `base-test.log`、`native-build.log`、
`native-regression-build.log`、`native-final-build.log`、`native-release-build.log`；最终九程序
启动及 SHA-256 初验为 `native-release-preflight.json`，窗口断言修正后的版本为
`native-spans-preflight.json`（构建见 `layout-spans-build.log`）。各轮桌面日志的首行标明
完整结果目录；最终验收为 `desktop-acceptance.log`，中间失败与复验另见前述记录。
三个提交的独立导出分别为 `commit-core`、`commit-terminator`、`commit-release`，
不含原有草稿或临时示例；最后补写验收结果仅修改本记录，不改变已验证源码。
本轮未修改 GOUI、模块版本、原有草稿或临时示例，
未发布或推送。

物理 IME、混合 DPI、性能对照及其他平台原生后端未执行。
F# 完整语法过滤器、Haskell 一般 parse-error(t) 推导、任意 GHC 扩展／CPP 求值、
普通顶层函数 clause 的完整范围不在本批范围；Scala 3 缩进、OCaml 普通 let/match、
Erlang 顶层 clause 仍需后续专用规则。没有把编辑器的词法布局折叠表述为完整编译器解析。
