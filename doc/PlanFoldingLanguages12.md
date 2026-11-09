# SSH 配置／HTTP 请求折叠第十二批验收（2026-11-09）

## 范围

基线 `6f3589d`，功能提交 `93583d2`，依赖 GOUI v0.7.0。
本批仅补足 SSH 配置分节和 HTTP／REST 请求块，继续限定为基本编辑器的文本折叠。
规则与参照见 [代码折叠](DesignFolding.md)，不增加解析器、公共 API、依赖或 GOUI 改动。

- SSH 复用既有扫描器的行首 keyword 与参数分类，按大小写不敏感的 Host／Match 分节，
  兼容空白／等号分隔，排除 HostName、参数及注释中的伪标题。
  首节之前的全局配置保持可见；重复节按位置分开，标题保留，尾部空行可见。
  缺少词法参数的标题不开始新节，无后续内容的节不发布。
  不校验全部参数语法，不求解 Host／Match，不展开 Include 或执行 ProxyCommand／exec。
- HTTP 复用既有扫描器确认的行首三个或更多 `#` 分隔符，保留分隔标题，
  将请求头和正文作为一个区域。首个请求可无分隔符，以请求行作为标题。
  起点为已有 method 加非空 target，或未加引号的明确 HTTP(S) URL，兼容省略 GET，
  scheme 不区分大小写；不推导仅有变量的隐式请求或其它客户端方言。
- HTTP 前导变量／注释留在首个无分隔符请求之外，只有变量／注释／空行的分隔块不发布。
  请求开始后正文中的 method／URL 不起新块，非行首 `###` 不切分。
  JSON／HTML／XML 正文保持整体，不产生内部折叠；已有词法高亮照常运行。
  仅尾部空行留在区域外，不解析网络／认证／响应或读取正文引用文件。

复用现有 syntaxLines、全文 worker、取消和视图状态；没有修改 Tokenizer、不可变 State、
模块版本、GUI 高亮缓存或 GOUI。所有新增可变数据归单次调用所有。

## 逻辑验证

`language/folding_requests_test.go` 使用固定源文本和独立精确范围：

- 注册前 SSHConfig／HTTP 均失败于缺少 provider（`registration-before.log`），接入后通过。
- SSH 的 Host／Match／全局前导、大小写／等号／缩进、quoted 参数中的 `#`、变量／percent
  参数、ProxyCommand／IdentityFile 中的伪标题、相似指令、裸标题、空节、重复节和注释。
- HTTP 多块、首块无分隔符、前导变量／注释、单行请求、空块／仅变量／仅注释块、
  正文中的 method／URL、JSON 整体、坏正文后的分隔恢复、缩进／多 `#` 和伪分隔符。
  method 无 target、加引号的 method／URL、缺少地址的 URL 均不开始请求。
- 对照 REST Client 官方格式时补充省略 GET 的常见 HTTP(S) URL；旧实现精确范围失败
  （`implicit-get-before.log`），补足后通过，并覆盖 scheme 大小写和正文里的伪 URL。
- 两种格式各八路并发复用 provider，起点 4090..4097 字节、正文／URL 9000 字节，
  覆盖跨扫描预算的 keyword、参数、分隔符和裸 URL；验证调用隔离。
  预取消与扫描中途取消均返回 context.Canceled 与空结果。

首轮范围／并发通过（`ranges-initial.log`），补充 URL 后完整语言测试通过
（`language-final.log`）。最终裸 URL 长行边界的 race 也通过（`boundary-race.log`）；
最终核心独立导出包含全部断言。没有删除既有断言、扩大 skip 或改动扫描器来迎合折叠结果。
这些样本不等同 SSH 工具执行、HTTP 协议校验或所有 REST Client 语法兼容性验收。

## 独立桌面

Linux amd64 / Go 1.25.8 / Pango；TigerVNC 1.12.0、24-bit、2560×1800，
Xfwm4 4.16.1，专用 Xauthority、D-Bus 与临时 XDG config/cache。
最终程序以 GOWORK=off 从当前源码构建，12 个程序的启动预检与 SHA-256 见 `native-preflight.json`。

新增 `tests/window/editor/folding_requests`，每组六份文档、每份两视图共享 Document，
其中一个只读。三份 SSH 样本覆盖 Host／Match、大小写／等号／引用参数／伪标题／重复节；
三份 HTTP 样本覆盖 JSON 整体、首个无分隔符的 URL-only 请求及坏正文恢复。
通过正常 Window.DispatchEvent 模拟键盘／指针，断言精确异步候选、折叠／展开、
视图隔离、隐藏正文不进入 visibleSpans、搜索展开和真实源字节选区。
折叠保持原文／撤销历史；第一次边界 Delete 仅展开，第二次编辑，Undo 恢复。
只读视图允许折叠；删除节标题／分隔符后，两个视图均得到独立预期的新候选，Undo 恢复。
去掉首个 HTTP 分隔符后仍识别请求行；去掉坏 JSON 后的中间分隔符后，原两块合成整体。
文档切换覆盖旧任务取消／脱离。

补充裸 URL 前的 Basic 六文档初验通过（`desktop-initial.log`）。最终源码完整 26 组全部通过：

| 场景 | 实际后端／缩放 | 结果 |
| --- | --- | --- |
| 第十二批六文档，Basic | Software 1× | 通过 |
| 第十二批六文档，Modern Light | Software 1.5× | 通过 |
| 第十二批六文档，Modern Dark | Software 2× | 通过 |
| 第十二批六文档，Modern Light | OpenGL 1× | 通过 |
| 第十二批六文档，Modern Dark | OpenGL 1.5× | 通过 |
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

完整结果为 `desktop-yr0eb4z5/results.json`，逐场景输出在同一目录，汇总为 `desktop.log`。
`postflight.json` 逐项核对 26 组实际后端／缩放、12 个程序 SHA-256 与专用会话清理：
全部匹配启动预检，专用进程均退出，认证文件已清理，原有草稿哈希不变。
本批没有启动崩溃或窗口断言失败。

模拟输入不等同物理键盘／IME 验收，OpenGL 不宣称硬件 GPU 验收。
WM 日志含 Another compositing manager 警告，不据 compositor 请求宣称桌面合成验证。窗口程序不操作用户桌面、用户文件、
剪贴板或系统设置，不连接 SSH／HTTP 目标。

## 命令与限制

日志和独立导出位于 `/tmp/sourceview-folding12-yf4r8tzy/`，命令均使用 GOWORK=off：

- `go test ./... -count=1`：通过（`go-test.log`）。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：通过（`race.log`）。
  最终补充的语言边界断言另跑 `go test -race ./language -count=1`，通过。
- `go vet ./...`：通过（`vet.log`）。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、
  `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：通过，分别见
  `windows-build.log`、`darwin-build.log`，未执行对应平台原生桌面。

首轮无裸 URL 时的检查也通过，日志以 `-before-url.log` 保留，不替代最终源码验证。
窗口构建见 `native-build.log`、`native-regression-build.log`，初版程序保留在 `bin-before-url`。
核心独立导出 `commit-core` 的全量测试与 vet 通过（`core-tree-test.log`、`core-tree-vet.log`）。
最终验收提交的独立导出 `commit-final` 也已通过全量测试和 vet
（`final-tree-test.log`、`final-tree-vet.log`）。最后补写结果仅修改本记录，已验收源码不变。
本批不改原有草稿或临时示例，未发布或推送。
物理 IME、混合 DPI、性能对照及其他平台原生后端未执行。
