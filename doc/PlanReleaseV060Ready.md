# v0.6.0 发布准备复验

日期：2026-11-15。维护者恢复 v0.6.0 发布准备；交付为 MIT Go 库与源码示例，无预编译附件。
**已正式发布**：[v0.6.0](https://github.com/golang-gui/goui-sourceview/releases/tag/v0.6.0)。准备阶段与发布阶段均未修改 GOUI 或 SourceView 代码。
下文先保留发布前复验，正式身份与公开验证见[发布完成](#发布完成)。
[中英文发布说明](ReleaseV060.md)与[历史暂缓记录](PlanReleaseV060.md)分别维护当前结果和旧失败。
历史记录中的剪贴板副作用继续保留，不能以本轮恢复成功覆盖旧问题。

## 冻结源码与依赖

- 验收代码：`bc3c567ac9bb563434938f6233015d173cbebfa4`，分支 `feature/multiselection`。
- `git archive` 共 573 个文件；SHA-256：
  `9ed6f16614ded2d95c73095066063f146416c971fa62f4269518cdaa1bb44df9`。
- 三端使用同一归档；两端传输后核对归档 SHA-256 及全部 573 个文件哈希。
  构建与执行均在仓库外干净副本，不带入 go.work、replace、未跟踪文件或本地 GOUI。
- go.mod 保持 Go 1.24、GOUI v0.8.0；GOUI 标签提交为
  `bc47eb006836fce9fed3cfe695f6345995513199`，无本地替换。
  其他依赖、许可证和资源不变；新增编辑器示例样本为本仓原创嵌入内容。
- 记录整理后仅有 Markdown 变化；最终准备提交重新归档，核对非 Markdown 文件与上述验收源码一致。
  最终归档的完整提交及摘要保留在证据目录的 `final-candidate.json`，不在文档中自引用提交 SHA。

## 环境与原生检查

| 项目 | Linux | Windows | macOS |
| --- | --- | --- | --- |
| OS / CPU | Ubuntu 22.04.5 / amd64 | Windows 10 19045.6466 / amd64 | macOS 15.7.7 / arm64 |
| Go | 1.25.8 | 1.25.8 | 1.24.9 |
| 排版 / 绘制 | Pango 1.50.6 / software | DirectWrite / Direct2D | CoreText / OpenGL |
| 桌面 | 独立 Xvnc + Xfwm4 合成会话 | 实际 RDP 登录桌面，1600×900 | 实际 VNC 登录桌面，1650×1050 |
| `go test -json ./... -count=1` | 2362 通过，0 失败/跳过 | 2362 通过，0 失败/跳过 | 2362 通过，0 失败/跳过 |
| `go mod verify`、`go mod tidy -diff`、`go vet ./...` | 通过 | 通过 | 通过 |
| `CGO_ENABLED=0 go build ./...` | 通过 | 通过 | 通过 |
| 二十个示例/窗口入口逐一构建 | 通过 | 通过 | 通过 |
| 原生窗口矩阵 | 49/49 通过 | 49/49 通过 | 49/49 通过 |
| `go test -race -json ./... -count=1` | 2362 通过，0 失败/跳过 | 未执行 | 未执行 |

所有包检查使用 `GOWORK=off GOTOOLCHAIN=local`，正式构建与窗口程序使用 CGO=0；
Linux race 单独启用 cgo。2362 按 go test JSON 的命名 Test/pass 计数，包含子测试。
新增 LTR 段落随机分页原生回归 `TestParagraphLTRRandomPageSeam` 在三端均通过。

远端首次下载 GOUI v0.8.0 时，官方 proxy.golang.org 的 HTTPS 探针均在 12 秒超时；
首次脚本停在 tidy 下载，版本与 verify 已通过，其余检查当时尚未执行。
确认进程身份后中止本任务脚本和下载子进程，保存 `network-attempt-logs`；SSH 命令返回 255，
不将其计为包测试失败或通过。改用本轮命令环境 `GOPROXY=https://goproxy.cn` 重跑完整原生检查，
保留 go.sum 校验，未关闭 sumdb、未改全局 Go 环境。下载完成后两端再次 `go mod verify` 通过。

## 窗口覆盖

每端 49 组：多光标、保存点、特殊字符、视图恢复、GOUI 接入分别运行
Basic / Modern Light / Modern Dark × 应用缩放 100% / 150%，共 30 组；
另有 18 组基础输入、编辑体验、结构命令、搜索/替换、补全输入/导航、长行收起、
单/多行建议、水平滚动、键绑定、外框、空白与选区回归，以及 1 组 `multiselection -clipboard`。

GOUI 接入包含类型化回调、Surface、指针焦点策略、共享瞬态面被替换后的编辑与旧 presenter 隔离。
剪贴板专项验证真实平台服务的多选区复制/剪切、三个目标逐行分配，以及按两个目标各自缩进粘贴多行块。
矩阵有明确断言、超时、PASS 标志和退出码；三端均无失败或跳过。未修改或弱化断言。
输入经正常 Window.DispatchEvent；这些自动窗口结果与下述系统原生输入分别记录。

## 原生示例输入

两端均运行冻结源码构建的 `example/editor -theme light`，应用缩放 100%。
键鼠经过登录桌面的系统输入路径，不使用 DevServer、控件私有状态或语义动作捷径。

| 场景 | Windows | macOS |
| --- | --- | --- |
| Alt/Option 点击添加光标 | RDP Alt：页脚 2 selections，通过 | 系统 CGEvent Option：2 selections，通过 |
| Alt/Option+Shift 拖过三行数字 | RDP：3 selections，通过 | 系统 CGEvent：3 selections，通过 |
| 列选后输入 x，再一次撤销 | 三行 100/200/300 → x，768 字节；撤销回 774 字节 / Saved，通过 | 同样通过 |
| Ctrl/Cmd+D、跳过匹配、选区撤销 | 两个 product 选区及跳过/恢复位置正确，通过 | 同样通过 |
| 两个 product 同时替换为 x，正文撤销 | 762 字节，Undo 恢复 774 / Saved / 两个选区，通过 | 同样通过 |
| 单点真实拼音预编辑/候选/提交/撤销/取消 | 微软拼音，通过 | 简体拼音，通过 |
| 多选区真实拼音预编辑/提交/撤销/取消 | 微软拼音，通过 | 简体拼音，通过 |
| 编辑 → Mark saved → 编辑 → Undo/Redo | 775 / Saved、776 / Modified、775 / Saved 正确，通过 | 同样通过 |
| 特殊字符开关、U+200B tooltip、亮暗切换 | 616 字节、Saved 不变，通过 | 同样通过 |

真实 IME 使用普通 N、I 键驱动系统拼音服务，观察有下划线的 ni 预编辑与系统候选窗，
再按空格提交“你”，不是调用 CommitText 或直接发送 Unicode。单点原文 774 字节 / Saved，
预编辑期间正文仍 774 / Saved，提交后 777 / Modified，Undo 恢复 774 / Saved；
再次 ni 后 Escape 取消，正文不变。多选区只在主选区显示预编辑，两个 product 均保持未提交；
空格后同时替换为“你”，正文为 766 字节 / Modified；一次 Undo 恢复 774 / Saved 和两个选区，
再次预编辑再 Escape 取消也保持原文及集合。

首次 macOS 键盘复验紧接输入源菜单关闭发送点击，未建立目标插入点；观察后重新点击并核对
页脚 Ln 12，再运行全部快捷键和编辑断言。Windows 单独 Shift 未切换 IME 中英文模式，
因此一次尝试得到普通 ni；完整撤销恢复 774 / Saved 后，改用任务栏明确切到中文并重新验收。
两次操作准备问题保留截图，不以其冒充 IME 或快捷键通过。

### macOS Option 与 VNC 映射

本次 VNC 客户端的 `key-down ALT` 不能代表 Option：仓库外只读 CGEvent 探针实测
combined-session flags 从 `0x20000000` 变为 `0x20100008`，包含 Command mask `0x100000`，
不含 Option mask `0x80000`，左 Option keycode 58 状态仍为 false；释放后恢复基线。
Shift / Control 探针分别包含对应标志。VNC Alt 点击仍为一个选区，Alt+Shift 拖动仍为连续选区，
这是当前远程输入链路的限制，不能据此归因 SourceView 或宣称 VNC Option 已通过。

替代验收用仓库外 `cginput.m`，从本轮新建 Terminal 执行：通过
CGEventCreateMouseEvent、CGEventSetFlags、CGEventPost 向系统事件流发送 Option 点击及
Option+Shift 拖动，由 AppKit 原生窗口进入 GOUI 正常事件分发。
标志含义采用 SDK 的 `kCGEventFlagMaskAlternate` / `kCGEventFlagMaskShift` 常量，参见
[Apple CGEventFlags](https://developer.apple.com/documentation/coregraphics/cgeventflags)。
CGPreflightPostEventAccess 在 SSH 为 false、Terminal 为 true，使用 Terminal 已有授权，
未申请新权限或修改系统设置。该路径是系统注入测试，不是物理 Option 按键测试。
程序只构造原生输入，不导入本仓库、不访问控件，也不随库发布。

## 桌面恢复与证据

窗口测试前备份并核对完整剪贴板数据：Windows 在实际登录桌面的 STA PowerShell 中保存
4 种显式格式，macOS 保存 1 项 / 1 种类型；快照仅存远端私有临时目录。
矩阵和手工验收结束后恢复并逐项核对格式及字节一致，敏感快照已删除，未下载或纳入日志归档。
本轮剪贴板恢复成功不撤销 11-12 未备份导致的历史副作用。

两端输入源恢复到原状态（macOS ABC、Windows 原拼音输入源的英文模式），未新增输入源或更改快捷键。
两端示例均退出码 0。关闭本轮 Windows PowerShell、macOS 两个新建 Terminal，
保留原有 Terminal、Safari、IDE 和其他应用。Linux 私有 Xvnc/Xfwm4 已退出，未操作用户 :3 桌面。
释放键鼠并断开本轮 mac/windows 桌面客户端，保留原有 SSH 连接。

本机证据根 `/tmp/sourceview-v060-ready-mqt1iqgv`，远端目录分别为
`/private/tmp/sourceview-v060-ready-mqt1iqgv` 与 Windows TEMP 下的同名目录。
`candidate.json` / `manifest.json` 记录源码；`linux`、`mac/native-logs`、`win/native-logs`
记录包检查，`linux-matrix/results.json`、`mac/results.json`、`win/results.json` 各记录 49/49。
`mac/evidence-logs.tar.gz`、`win/evidence.zip` 保存远端原生、窗口、网络中断及示例退出日志；
`screenshots` 保存手势、真实 IME、快捷键、保存点、特殊字符和桌面恢复证据。

## 发布状态与覆盖边界

此前 Windows Alt 缺陷和两端真实 IME 验收缺口已在 GOUI v0.8.0 下复验通过；
macOS Option 已通过系统事件路径验收，VNC 映射限制按上文保留，不计为物理键通过。
本轮未发现需修复的新代码缺陷。原生 Wayland、Windows ARM64、macOS Intel、物理键盘、
混合显示器 DPI 和辅助技术操作未执行；Windows/macOS race 未执行，均不计为通过。

截至准备提交，origin/main 仍为 `a91ed11`，候选代码较其超前 36 个提交；v0.6.0 标签及 Release 不存在。
准备阶段未推送、打标签或创建 Release。维护者随后明确授权正式发布，按[发布流程](Release.md)
将最终候选纳入 origin/main 历史，创建 annotated tag（消息 `Release v0.6.0.`），
核对空附件 Draft 后发布，并执行官方 Go proxy 与无 replace 消费模块验证。
准备提交时这些外部步骤尚未执行；正式发布及安装文档收尾见下节。

## 发布完成

维护者明确发出“请发布”后，于 2026-11-15 完成：

- main 从 `a91ed11` 快进到最终准备提交 `c17b728834adc2fefbc7499f8d4c9102ccfd7c58`，推送后核对远端一致。
  该候选只比三端验收代码 `bc3c567` 增加七份 Markdown 变更，449 个非 Markdown 文件逐项一致。
- 最终候选 git archive 共 574 个文件，SHA-256：
  `366b616f3e471155ccbfe9a81d9001b3a9f871b8ea2bb86c888b75f31596cf19`。
  独立干净副本 `GOWORK=off` 的 mod verify、tidy -diff、vet、CGO=0 build 均通过；
  `go test -json ./... -count=1` 为 2362 通过、0 失败/跳过。三端窗口与 race 沿用上述相同代码的验收结果。
- 创建 annotated tag `v0.6.0`，消息 `Release v0.6.0.`；仅推送本标签。
  标签对象 `be9a18f31abbac93f7d078b7315cca386a62ba77`，远端 peeled SHA 为上述完整候选 SHA。
- 先创建 Draft，逐项核对 tag、目标 SHA、中英文正文与 0 个手工附件，再转正式发布。
  [Release](https://github.com/golang-gui/goui-sourceview/releases/tag/v0.6.0) 于 `2026-11-15T08:12:38Z` 公开，Latest 正确，draft/prerelease 均 false。
  GitHub 自动源码归档仍可用，不制作 Demo、安装包、签名或公证附件。
- 仓库外使用全新 `GOMODCACHE`、`GOWORK=off GOTOOLCHAIN=local CGO_ENABLED=0`，
  仅用 `GOPROXY=https://proxy.golang.org`，开启 `GOSUMDB=sum.golang.org`，无镜像回退、replace 或工作区。
  `go mod download -json github.com/golang-gui/goui-sourceview@v0.6.0` 成功，Origin.Hash 与标签一致，
  Origin.Ref 为 `refs/tags/v0.6.0`；模块校验 `h1:smw7FpA1wl5a/6wcUTZ+OtlVWqConqKih0eeG2aX7NY=`。
- 新建消费模块执行 README 中的
  `go get github.com/golang-gui/goui-sourceview/sourceview@v0.6.0`，随后 tidy、mod verify、build、运行与 tidy -diff 全部通过。
  实际构建 sourceview、ui、language、scheme；正文替换、保存点 Modified 和 Undo 回到 clean 均有断言。
  全部模块 Replace 为空，GOUI 实际选中 v0.8.0。首次官方代理查询成功，无传播错误或重试。
- 公开验证后更新中英文 README、示例与应用技能接入说明，移除新包布局尚未发布／必须工作区的过期描述，
  同步当前设计状态与发布记录。收尾仅有文档变化，不移动发布标签。

发布阶段未操作任何桌面或剪贴板，保留已有未跟踪 `doc/Doing.md`、`example/temp/`。
Windows/macOS race、物理键、Wayland、Windows ARM64、macOS Intel、混合 DPI 和辅助技术操作的
未验证边界保持不变，不以本次代理下载或消费运行补算为通过。

发布证据 `/tmp/sourceview-v060-publish-0rtny7e_`：`draft.json` 与 `release.json` 保存 Draft/正式核对，
`release-body.md` 为发布正文；`public-checks` 保存官方下载、模块列表、构建与运行的命令、输出和退出码，
`consumer` 是无 replace 的消费模块，`module-cache` 是本次独立下载缓存。准备和原生证据根保持不变。

收尾验证：`go test ./...`、`GOWORK=off go test ./...` 均通过（测试命中缓存）；
差异无空白错误，12 份修改文档中的 190 个本地链接目标存在。收尾仅修改 Markdown。
