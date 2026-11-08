# 基础 DOT／Diff 折叠第十一批验收（2026-11-08）

## 范围

基线 `1e3b329`，功能提交 `04761c3`，依赖 GOUI v0.7.0。
本批补足 DOT 及普通 Git／unified Diff，继续限定为基本代码编辑器的词法／文本折叠。
规则与官方参照见 [代码折叠](DesignFolding.md)，不增加编译器、补丁应用或语义解析。

- DOT 配对 graph/subgraph 的 `{}`、属性 `[]`，复用已有扫描器的真实打开／关闭位置，
  收集闭合的多行注释、反斜杠续行引号和 HTML-like 字面量。
  字符串、注释内的伪括号排除；HTML-like label 整体不透明，不构建 HTML 元素树。
  无续行引号在 LF 错误恢复时撤销观察；未完成外层不发布，已完成内层可独立保留。
- Diff 提供文件／hunk 两层区域：Git 的首对旧／新文件头留在其文件中，普通 unified
  文件以连续的词法文件头 `---`／`+++` 开始；有效普通 `@@` 开始 hunk。
  已有扫描器用声明行数区分补丁正文和文件头，折叠直接复用其分类和 hunk 确认。
  不根据高亮文本再次推测补丁语法；正文中的伪文件／hunk 标记不会切断区域。
- 单空格 context 行、无末尾换行提示属于 hunk，尾部真正空行留在区域外。
  rename／二进制元数据可随 Git 文件整体折叠，二进制正文不透明；明确的 combined
  文件标记不生成折叠。不解码二进制、不应用补丁、不验证补丁有效性。

公开 Scan 的 token、Consumed、ReplaceFrom、不可变 State 和公共 API 不变。
新增可变观察由单次 worker 所有，不改模块依赖、GUI 缓存、视图状态或 GOUI。

## 逻辑验证

`language/folding_diff_test.go` 使用固定文本和独立精确范围：

- 接入前 DOT／Diff 注册断言均失败，见 `registration-before.log`；接入后通过。
- DOT 嵌套图／属性、引号与注释内的假括号、HTML-like 标签中的尖括号属性和注释、
  相邻注释、反斜杠续行、无续行错误恢复、未闭合输入和不匹配括号。
  关闭行还有其他正文时保持可见，同标题保留外层。
- 普通 Git 的多文件／多 hunk、无 Git 标记的 unified 文件、正文中的 `---`／`+++`、
  加行／context 中的假文件与 hunk 标记、缺少末尾换行、单空格 context、零计数、
  无正文、残缺头、孤立旧／新文件头、带引号／中文路径、rename／binary 和 combined。
- 公开／带观察扫描路径按 1..17 字节预算逐片比较 State、token、Consumed 和 ReplaceFrom，
  包括 Unicode、延迟属性分类、跨行字面量和 Diff 正文行数；观察不改变词法结果。
- 两种格式各八路并发复用 provider，边界 4090..4097 字节、正文 9000 字节，
  验证长行分片和调用隔离；DOT 另逐一检查注释、续行引号、HTML-like 的边界。
  预取消／扫描中途取消均返回 context.Canceled 与空结果。

首轮精确范围及并发通过（`ranges-initial.log`），完整语言测试通过（`language-test.log`）。
代码检查还修正了初始 oldHeader=-1 与首行 `+++` 的误配条件，并补充孤立新文件头断言。
未删除原有断言、增加 skip 或修改 State 来迎合折叠预期。
未运行 Graphviz 语义校验、补丁应用、源语言分析或性能对照。

## 独立桌面

Linux amd64 / Go 1.25.8 / Pango；TigerVNC 1.12.0、24-bit、2560×1800，
Xfwm4 4.16.1，专用 Xauthority、D-Bus 与临时 XDG config/cache。
程序全部以 GOWORK=off 从本次源码构建；启动预检及 SHA-256 见 `native-preflight.json`。

新增 `tests/window/editor/folding_diff`，每组六份文档、每份两视图共享 Document，
其中一个只读。覆盖 DOT 图／属性／HTML-like、注释、续行引号，以及 Git 多 hunk、
plain unified、二进制与 rename 元数据。
通过正常 Window.DispatchEvent 模拟键盘／指针，断言精确异步候选、折叠／展开、
视图隔离、隐藏正文不进入 visibleSpans、搜索展开和真实源字节选区。
折叠保持原文和撤销历史；第一次边界 Delete 仅展开、第二次编辑，Undo 恢复。
只读视图允许折叠。删除关闭行或首个文件头后，两视图均得到独立预期的候选，Undo 恢复。
删除 Git 标记后，余下 unified 文件头仍形成正确文件区域；删除 plain unified 旧文件头后，
保留独立 hunk。文档切换覆盖分析取消／脱离。

Basic 六文档初验通过（`desktop-initial.log`）。完整 24 组全部通过：

| 场景 | 实际后端／缩放 | 结果 |
| --- | --- | --- |
| 第十一批六文档，Basic | Software 1× | 通过 |
| 第十一批六文档，Modern Light | Software 1.5× | 通过 |
| 第十一批六文档，Modern Dark | Software 2× | 通过 |
| 第十一批六文档，Modern Light | OpenGL 1× | 通过 |
| 第十一批六文档，Modern Dark | OpenGL 1.5× | 通过 |
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

完整结果见 `desktop-rcrvrkgt/results.json`，逐场景日志在同一目录，汇总为 `desktop.log`。
`postflight.json` 检查 24 组的实际后端／缩放、11 个程序 SHA-256 和专用会话清理：
全部匹配构建预检，专用进程均退出，认证文件已清理，原有草稿哈希不变。
本批未出现启动崩溃或窗口断言失败。

模拟输入不等同物理键盘／IME 验收，OpenGL 不宣称硬件 GPU 验收。
WM 日志含 Another compositing manager 警告，不据此声称独立桌面合成已验证。
窗口程序不操作用户桌面、用户文件、剪贴板或系统设置。

## 命令与限制

日志及独立导出位于 `/tmp/sourceview-folding11-te4bfzxd/`。命令均使用 GOWORK=off：

- `go test ./... -count=1`：通过（`go-test.log`）；核心独立导出通过（`core-tree-test.log`），最终独立导出通过（`final-tree-test.log`）。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：通过（`race.log`）。
- `go vet ./...`：通过（`vet.log`）；核心独立导出通过（`core-tree-vet.log`），最终独立导出通过（`final-tree-vet.log`）。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、
  `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：通过，分别见
  `windows-build.log`、`darwin-build.log`，未执行对应平台原生桌面。

窗口构建见 `native-build.log`、`native-regression-build.log`；源码提交的独立导出为 `commit-core`。
最终验收提交的独立导出 `commit-final` 也已通过全量测试和 vet。
最后补写结果仅修改本记录，已验收源码不变。本批不改 GOUI、模块版本、原有草稿或临时示例，未发布或推送。
物理 IME、混合 DPI、性能对照和其他平台原生后端未执行。
