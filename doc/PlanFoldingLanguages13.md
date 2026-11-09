# Dockerfile 基础折叠第十三批验收（2026-11-09）

## 范围

基线 `ed07b29`，功能提交 `fd0b648`，依赖 GOUI v0.7.0。
本批仅补 Dockerfile 的 FROM 阶段、已结束的显式续行指令和 heredoc。
规则与官方参照见 [代码折叠](DesignFolding.md)，保持基本代码编辑器范围。

- 复用既有 configTokenizer 的行首 keyword、续行和 heredoc 生命周期，真正的 FROM
  开始阶段，标题保留，折叠到下一阶段前或 EOF；尾部空行留在区域外。
  首个 FROM 前导不并入阶段，重复阶段按位置区分，缺参数／单行空阶段不产生区域。
- 指令参数、COPY --from、ONBUILD 参数、注释、续行正文和 heredoc 中的 FROM 不切分。
  显式跨行指令从其标题到实际结束行产生子区域；同标题保留阶段外层，候选不交叉。
  沿用既有反斜线／反引号设置，以及 heredoc 引用、制表符剥离、多终止符队列。
- 未结束的续行／heredoc 不发布独立指令区域，外层阶段仍可保留已有文本。
  不按 exec form 的普通括号回退，不解析 Shell 块、镜像、别名、选项或构建依赖。
  采用本仓已有词法 profile，不声称完整 Docker frontend／parser directive 兼容。

修正已有 Docker 续行遇到空行／整行注释时提前结束的词法问题。
configState 增加私有 dockerContent，实际非注释 token 表示当前行有内容，跨片段保存，
真正行末复位；只有空行／注释被忽略，变量和转义参数仍会正常结束续行。
注释本身的反斜线不建立续行；续行期间的同形 escape 注释不改变转义设置。
其余配置 profile 不采用这条规则。State 仍为不可变快照，不改公开接口、依赖、GUI 缓存或 GOUI。

## 逻辑验证

`language/folding_docker_test.go` 使用固定文本与独立精确预期：

- 注册前无 folding provider；原扫描器将跨注释／空行、反引号续行及续行中的 escape
  注释后的 FROM 参数误分为指令，精确 keyword 断言失败（`before.log`）。修正后通过。
  变量／转义参数结束续行、普通注释尾部的反斜线不创建续行均单独保留断言。
- 阶段、全局前导、大小写／缩进、参数／引用／注释中的伪 FROM、ONBUILD／COPY --from，
  多行指令的阶段嵌套、单独指令、前导 ARG 续行、多行 FROM 同标题保留外层。
- 默认／反引号续行、续行期间的注释和空行、同形 escape 注释；RUN／COPY heredoc、
  引用／制表符剥离／多个终止符／续行后进入 heredoc，以及引用内的假 heredoc。
- 未闭合指令／heredoc、未闭合内层但保留阶段、空／裸阶段和不加普通括号回退。
- 1..17 字节扫描预算下，重放 ReplaceFrom，逐字节比较 token kind／class，并对比
  每行结束的完整 State；样本包含 Unicode 终止符、续行／注释及 heredoc 中的假 FROM。
- 八路并发复用 provider，起点 4090..4097 字节，9000 字节指令参数／正文和带 Unicode
  的长 heredoc 终止符，检查跨片段配对及调用隔离。预取消和扫描中途取消均返回
  context.Canceled 与空结果。

首轮功能／词法断言通过（`docker-initial.log`）；加入长行、并发及取消后，完整语言测试
通过（`language-test.log`）。首次写入测试时曾有一个字符串转义拼写错误导致编译失败
（`test-write-initial.log`），修正测试拼写后才运行上述旧代码回归断言。
没有删减原有断言、扩大 skip、降低预期或把编译失败当成功能复现。
这些样本不是 Docker 构建、镜像解析、Shell 执行或性能对照。

## 独立桌面

Linux amd64 / Go 1.25.8 / Pango；TigerVNC 1.12.0、24-bit、2560×1800，
Xfwm4 4.16.1，专用 Xauthority、D-Bus 和临时 XDG config/cache。
全部程序以 GOWORK=off 从本次源码构建，13 个程序的启动预检与 SHA-256 见 `native-preflight.json`。

新增 `tests/window/editor/folding_docker`，每组六份文档，每份两视图共享 Document，
其中一个只读。样本包括阶段／续行中的注释和空行、参数中的伪 FROM、非第零行阶段、
修改 escape 设置、COPY heredoc 和 RUN 多终止符。
通过正常 Window.DispatchEvent 模拟键盘／指针，断言精确异步候选、折叠／展开、
视图隔离、隐藏正文不进入 visibleSpans、搜索展开与真实源字节选区。
折叠保持原文／撤销历史；第一次边界 Delete 只展开，第二次编辑，Undo 恢复。
只读视图允许折叠；删除阶段头／escape 指令／heredoc 终止符／续行指令后，
两视图均得到独立预期的新范围，Undo 恢复原候选。删除最后一个 heredoc 终止符后，
其内层指令不再发布，已有阶段仍保留。文档切换覆盖旧任务取消／脱离。

Basic 六文档初验通过（`desktop-initial.log`）。最终源码的完整 28 组全部通过：

| 场景 | 实际后端／缩放 | 结果 |
| --- | --- | --- |
| 第十三批六文档，Basic | Software 1× | 通过 |
| 第十三批六文档，Modern Light | Software 1.5× | 通过 |
| 第十三批六文档，Modern Dark | Software 2× | 通过 |
| 第十三批六文档，Modern Light | OpenGL 1× | 通过 |
| 第十三批六文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第十二批六文档，Basic／Modern Dark | Software 1×／OpenGL 1.5× | 两组通过 |
| 第十一批六文档，Basic／Modern Dark | Software 1×／OpenGL 1.5× | 两组通过 |
| 第十批八份配置文档，Basic／Modern Dark | Software 1×／OpenGL 1.5× | 两组通过 |
| 第九批八份文档，Basic／Modern Dark | Software 1×／OpenGL 1.5× | 两组通过 |
| 第八批八份文档，Basic／Modern Dark | Software 1×／OpenGL 1.5× | 两组通过 |
| 第七批九份文档，Basic／Modern Dark | Software 1×／OpenGL 1.5× | 两组通过 |
| 第六批八份文档，Basic／Modern Dark | Software 1×／OpenGL 1.5× | 两组通过 |
| 第五批七份文档，Basic／Modern Dark | Software 1×／OpenGL 1.5× | 两组通过 |
| 第四批五种语言，Basic／Modern Dark | Software 1×／OpenGL 1.5× | 两组通过 |
| 第三批混合文档，Basic／Modern Dark | Software 1×／OpenGL 1.5× | 两组通过 |
| 第一／二批十一种语言，Basic／Modern Dark | Software 1×／OpenGL 1.5× | 两组通过 |
| 原有 JSON／HTML，Modern Dark | OpenGL 2× | 通过 |

完整结果为 `desktop-2cucuc24/results.json`，逐场景输出在同一目录，汇总为 `desktop.log`。
`postflight.json` 逐项核对 28 组实际后端／缩放、13 个程序 SHA-256 与专用会话清理：
全部匹配启动预检，专用进程均退出，认证文件已清理，原有草稿哈希不变。
本批没有启动崩溃或窗口断言失败。

模拟输入不等同物理键盘／IME 验收，OpenGL 不宣称硬件 GPU 验收。
WM 日志含 Another compositing manager 警告，不据 compositor 请求宣称桌面合成验证。窗口程序不操作用户桌面、用户文件、
剪贴板或系统设置，不启动 Docker、拉取镜像或执行 Shell。

## 命令与限制

日志和独立导出位于 `/tmp/sourceview-folding13-mc3ldtfi/`，命令均使用 GOWORK=off：

- `go test ./... -count=1`：通过（`go-test.log`）。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：通过（`race.log`）。
- `go vet ./...`：通过（`vet.log`）。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、
  `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：通过，分别见
  `windows-build.log`、`darwin-build.log`，未执行对应平台原生桌面。

窗口构建见 `native-build.log`、`native-regression-build.log`。
核心独立导出 `commit-core` 的全量测试与 vet 通过（`core-tree-test.log`、`core-tree-vet.log`）。
最终验收提交的独立导出 `commit-final` 也已通过全量测试和 vet
（`final-tree-test.log`、`final-tree-vet.log`）。最后补写结果仅修改本记录，已验收源码不变。
本批不改原有草稿或临时示例，未发布或推送。
物理 IME、混合 DPI、性能对照和其他平台原生后端未执行。
