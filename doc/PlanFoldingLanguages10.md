# 基础配置文件折叠第十批验收（2026-11-08）

## 范围

基线 `df95e9d`，功能提交 `efc0000`，依赖 GOUI v0.7.0。
本批按基本代码编辑器范围补足 Nginx、go.mod／go.work，以及 INI、GitConfig、
EditorConfig、systemd、Desktop Entry。规则与官方参照见 [代码折叠](DesignFolding.md)。
Scala 3 专用布局方案取消；本批不包含 DOT／Diff 或新的语法／语义解析能力。

- Nginx 复用已有字面量观察及结构折叠，只配对实际 `{}`，排除引号、转义、变量中的
  伪花括号。普通括号／方括号不进入配置块栈；只观察实际闭合的多行引号。
- Go 模块／工作区复用 modTokenizer，仅配对 `()`，不把版本区间 `[]` 当作块。
  引号、反引号和注释中的分隔符排除，不校验指令、路径、版本或依赖语义。
- 五种分节格式共用小型文本范围提供者，标题识别仍依赖各自词法分类。
  Git quoted subsection 的 `]`、EditorConfig glob 字符类、配置值、续行和注释
  分别沿用已有扫描器；不增加配置解析器、include 读取、glob 匹配或值求解。
- 完整标题开始新节，保留标题行；后续正文和注释到下一节前折叠，尾部空行可见。
  相邻空节不发布无内容区域，重复节名按位置分开，首节之前的文本保持可见。
  不完整标题或带未识别尾随内容的行不切断前节。

不修改公开 Tokenizer、不可变 State、公共 API、模块依赖、GUI 高亮缓存或 GOUI。
所有新增可变状态由单次折叠调用所有；取消、修订核验、视图隔离和结束行规则沿用现有实现。

## 逻辑验证

`language/folding_config_test.go` 使用固定源文本及独立精确范围：

- 注册前八种格式均失败于 `builtin has no basic configuration folding provider`，
  见 `registration-before.log`；接入后通过。
- Nginx 嵌套块、闭合／未闭合多行字符串、变量、转义、注释、正则中其他标点，
  以及关闭行后还有正文时的可见行保留。
- go.mod／go.work 多组括号、注释／双引号／反引号中的伪分隔符、版本区间、
  关闭行尾和未完成分组。
- INI 正文／注释、重复节名、空节、残缺标题、尾部空行；Git quoted subsection 中的
  关闭括号、标题后的注释、普通／引号续行和未关闭的 subsection 引号。
- EditorConfig 前导 root、glob 字符类／转义、值中的括号和不支持行尾注释的标题；
  systemd 普通节／跨注释续行／值中的伪标题；Desktop Entry 本地化键、值和残缺标题。
- 每种格式八路并发复用 provider，边界位于 4090..4097 字节、正文长 9000 字节，
  检查长行／分片与调用隔离。另检查预取消和扫描中途取消均返回 context.Canceled 与空结果。

首轮精确范围和并发测试直接通过（`config-initial.log`）；完整语言测试通过
（`language-test.log`）。没有删除既有断言、增加 skip 或修改扫描器来迎合折叠预期。
样本验证的是词法／文本边界，未运行 Nginx、依赖解析或各配置工具的语义校验。

## 独立桌面

Linux amd64 / Go 1.25.8 / Pango；TigerVNC 1.12.0、24-bit、2560×1800，
Xfwm4 4.16.1，专用 Xauthority、D-Bus 与临时 XDG config/cache。
全部程序以 GOWORK=off 构建，启动预检及 SHA-256 保存在 `native-preflight.json`。

新增 `tests/window/editor/folding_config`，每组八份文档，覆盖全部新增格式；
每份两视图共享 Document，其中一个只读。
正常 Window.DispatchEvent 模拟键盘与指针，断言精确异步候选、折叠／展开、
视图隔离、隐藏正文不泄漏到 visibleSpans、搜索展开与真实源字节选区。
折叠不改变原文或撤销历史；第一次边界 Delete 仅展开、第二次编辑，Undo 恢复。
候选就绪后检查只读折叠；删除关闭分隔符或首节标题后，只剩独立预期的其他区域，
两个视图同步重新分析，Undo 恢复候选。切换文档覆盖任务取消／脱离。

| 场景 | 实际后端／缩放 | 结果 |
| --- | --- | --- |
| 第十批八份配置文档，Basic | Software 1× | 通过 |
| 第十批八份配置文档，Modern Light | Software 1.5× | 通过 |
| 第十批八份配置文档，Modern Dark | Software 2× | 通过 |
| 第十批八份配置文档，Modern Light | OpenGL 1× | 通过 |
| 第十批八份配置文档，Modern Dark | OpenGL 1.5× | 通过 |
| 第九批八份文档，Basic | Software 1× | 通过 |
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
| 原有 JSON/HTML，Modern Dark | OpenGL 2× | 通过 |

Basic 八文档初验已通过（`desktop-initial.log`）；完整 22 组全部通过，
汇总见 `desktop.log`，完整结果为 `desktop-4cyflizu/results.json`，场景输出在同一目录。
实际后端／缩放及最终程序 SHA-256 已逐项核对，无窗口断言或启动失败。
全部本次程序、专用 X11／WM／D-Bus 会话已退出，认证文件已清理。
模拟输入不等同物理键盘／IME 验收，OpenGL 通过不宣称硬件 GPU 验收。
窗口程序不操作用户桌面、用户文件、剪贴板或系统设置。

## 命令与限制

日志和独立导出位于 `/tmp/sourceview-folding10-df55xeo2/`，命令均使用 GOWORK=off：

- `go test ./... -count=1`：通过（`go-test.log`），核心独立导出也通过（`core-tree-test.log`）；最终验收提交的独立导出也通过（`final-tree-test.log`）。
- `go test -race . ./language ./internal/highlight ./internal/editor ./ui`：通过（`race.log`）。
- `go vet ./...`：通过（`vet.log`），核心独立导出也通过（`core-tree-vet.log`）；最终验收提交的独立导出也通过（`final-tree-vet.log`）。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、
  `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：通过，分别见
  `windows-build.log`、`darwin-build.log`，未执行对应平台原生桌面。

基础与窗口构建分别见 `base-test.log`、`native-build.log`、`native-regression-build.log`。
两个独立导出为 `commit-core`、`commit-final`，均不含原有草稿或临时示例。
最后补写结果只修改本记录，已验证源码不变。本批未修改 GOUI、模块版本、原有草稿
或临时示例，未发布或推送。
物理 IME、混合 DPI、性能对照和其他平台原生后端未执行。
