# v0.6.0 发布准备与原生验收

日期：2026-11-12。Windows/macOS 真机验证及发布准备已收尾；**发布暂缓**。
维护者在确认 Windows 原生 Alt 鼠标问题后决定“只记录问题，暂缓发版”，本轮不修改 GOUI。
不把模拟窗口通过等同真实桌面输入通过，也不把失败或未执行项计为通过。
拟用版本 v0.6.0，发布 Go 库与源码示例，不制作二进制附件；
GOUI 仍为 v0.7.0，最低 Go 1.24，无 replace 或 go.work。中英文草稿见 [发布说明](ReleaseV060.md)。

## 初始冻结与环境

- 初始候选 c58b7d3dd1872633754e73e4798fcaf50154b601，git archive 共 567 个文件。
- 归档 SHA-256：47d78513afb6f150e28847f0cc17a49a4eb314af274087f52571fcc03a900ec3。
- 两端传输后核对归档及全部文件 SHA-256，原生编译和测试使用专用临时目录。
- Linux：Ubuntu 22.04.5 amd64 / Go 1.25.8 / Pango 1.50.6 / software，独立 Xvnc + Xfwm4 合成桌面。
- Windows：Windows 10 19045 amd64 / Go 1.25.8 / DirectWrite / Direct2D，实际 RDP 登录桌面。
- macOS：15.7.7 arm64 / Go 1.24.9 / CoreText / OpenGL，实际 VNC 登录桌面。
- 窗口均 CGO_ENABLED=0，包检查 GOWORK=off、GOTOOLCHAIN=local；不改全局 Go 环境。
  100%/150% 指 GOUI 应用缩放，不等同真实混合显示器 DPI。

## 首轮结果与修正

Linux 初始原生 go test -json ./... -count=1：2356 项通过、无失败/跳过；
mod verify、tidy -diff、vet、CGO=0 build 和全仓 race 通过。
Windows 初始包测试 2355 通过、1 失败；macOS 2353 通过、3 失败。
两端 mod verify、tidy -diff、vet、CGO=0 全仓及十九个窗口/示例程序构建均通过。

三个失败原因涉及测试对平台的假设：

- gui.ModAlt 在 macOS 解析为独立的 ModifierOption；裸 ModifierAlt 不是平台 Option 事件。
  默认配置和命中测试改用公开 Resolve，保留默认值及自定义覆盖断言。
- CoreText 可以将 A 与 ZWSP 合为不可拆 cluster。删除预期改用独立原生 cluster 范围，
  明确核对选区与该范围一致；保留正文、历史、保存身份、marker 和 Undo 全部断言。
- 长行中的 x advance 不能假定等于 space advance。用原生 x 度量定位，保留分页、
  有界文本窗口、滚动失效和 marker 数量断言；未修改生产字体或排版。

窗口探针同步改用实际原生行的列坐标、平台 Alt/Option，以及 Ctrl/Cmd 的主修饰键。
旧 inline_suggestion / inline_multiline 在 macOS 首轮均因固定 Ctrl+Z 失败，已改用 ModPrimary.Resolve。
适配后的两端全量包测试均 2356 通过、0 失败/跳过。适配期间 LineRange 的 error 被误当 bool，
编译明确失败；更正为检查 error 后重跑通过，保留日志，没有删除断言。

## 窗口首轮与修正

Windows 第一轮八组窗口通过；macOS 第一轮保存点、视图恢复、基础编辑、菜单/补全体验通过，
多光标、特殊字符和旧影子建议探针失败，均保留退出码与日志。

完整矩阵每端 42 组：多光标、保存点、特殊字符、视图恢复分别运行 Basic/Modern Light/Dark
×100%/150%；再运行基础输入、编辑体验、结构命令、查找/替换、补全输入/导航、
长行收起、单/多行建议、水平滚动、键绑定、外框、空白与选区像素回归。
Windows 42 组通过。macOS 首轮 38 通过、4 失败：

- 三种主题的 150% 大长行视图恢复相差约 4 DIP，原严格视口断言失败。
  新增包回归复现“编辑窗口已有几何而目标绘制视口未准备时过早完成”，先保留恢复请求至视口就绪；
  单独等待仍有差异；日志进一步确认两视图的相同字体/宽度下，分页准备顺序导致换行起点不同。
  快照新增最多 24 项普通文本边界，仅相同度量身份时应用；目标页延迟重建并保留恢复至视口就绪。
  修正后的 macOS Basic 150% 大长行窗口通过，最终冻结后重跑完整矩阵也通过。
- Basic 100% 空白圆点直径小于一个物理像素，其 2×1 覆盖被至少 2×2 的断言拒绝。
  补充实际 FillEllipse 的等半径和原生单元中心断言；亚像素覆盖按尺寸核验，较大圆仍要求二维。
  增加矩形、错误半径/中心、不可见圆和 4×1 扁条的负向回归，未改变绘制规则。

上述测试适配独立提交为 `027e8a3`；长行视图恢复修正及回归独立提交为 `88637a7`。
没有修改 GOUI、生产字体或放宽原视口误差断言。

## 最终冻结与原生检查

- 代码提交：`88637a75d252bd23da2898c7c1dbf7e793c7a359`，归档 569 个文件。
- 归档 SHA-256：`750aac929503a029e6d8a2e8116e44920165f2d8825b5036716598970ef35216`。
- 两端重新核对全部文件 SHA-256 后，从独立 `final-src` 构建与运行；未使用本地 GOUI 工作区。
- 收尾提交仅更新记录与发布草稿，不改变上述验证代码。

| 检查 | Linux amd64 | Windows amd64 | macOS arm64 |
| --- | --- | --- | --- |
| `GOWORK=off go test -json ./... -count=1` | 2359 通过，0 失败/跳过 | 2359 通过，0 失败/跳过 | 2359 通过，0 失败/跳过 |
| `go mod verify`、`go mod tidy -diff`、`go vet ./...`（GOWORK=off） | 通过 | 通过 | 通过 |
| `GOWORK=off CGO_ENABLED=0 go build ./...` | 通过 | 通过 | 通过 |
| 十九个示例/窗口入口分别构建 | 通过 | 通过 | 通过 |
| 最终原生窗口矩阵 | 42/42 通过 | 42/42 通过 | 42/42 通过 |
| `GOWORK=off go test -race ./...` | 通过 | 未执行 | 未执行 |

2359 包含命名子测试，按 go test JSON 的 Test/pass 计数。42 组窗口为前述 24 组主题/缩放
加 18 组编辑回归；正常事件分发和 CommitText 只能验证控件处理，不能替代真实鼠标修饰键或 IME。
macOS 150% 大长行恢复与 Basic 100% 亚像素空白标记均在最终矩阵严格通过。
本轮 42 组未启用多光标窗口的 `-clipboard` 专项；手工仅核对多选区复制后单点粘贴，
不作为原生剪贴板向多个目标逐行分配的专项验收。

Linux 最终矩阵第一次启动时，临时 multiselection 可执行文件首字节为 00，报告 exec format error，
未进入程序；原因未确定，保留原文件和启动日志。从同一冻结源码重新构建为合法 ELF，
使用重建程序完成 42/42；该次启动失败不计为通过，也不据此认定为 SourceView 缺陷。

## 真实示例输入：通过项与发布阻塞

真实输入使用远程登录桌面的键鼠协议，经原生窗口事件进入 `example/editor`，未使用 DevServer。
示例在应用缩放 100% 下验收，亮/暗主题均观察；100%/150% 窗口矩阵不代表真实混合 DPI 切换。

| 场景 | Windows / RDP | macOS / VNC |
| --- | --- | --- |
| Ctrl/Cmd+D、跳过匹配、选区撤销 | 通过 | 通过 |
| 多点替换、一次正文撤销恢复 | 通过 | 通过，两个 product 同时替换为 x 后恢复 |
| 多选区复制及单点粘贴 | 通过，得到 product 换行 product | 通过，同样得到两行 product，Undo 恢复 |
| 编辑 → Mark saved → 再编辑 → Undo/Redo | 通过，Saved/Modified 正确切换 | 通过，775/776 字节及 Saved/Modified 正确切换 |
| 特殊字符开关、亮暗主题、U+200B tooltip | 通过，616 字节且保持 Saved | 通过，616 字节且保持 Saved |
| Alt/Option+点击添加光标 | **失败，仍只有一个光标** | **未通过，远程 ALT 按键点击仍一个光标，原因未定位** |
| Alt/Option+Shift+拖动列选 | **失败，变为普通连续选区** | **未通过，变为普通连续选区，原因未定位** |
| 真实输入法预编辑、候选、提交和取消 | 未完成 | 未执行 |

Windows 阻塞可复现：选择 multicursor.go，先按住 Alt 再点击正文不同位置，页脚始终为
1 selection；按住 Alt+Shift 拖过三行数字，得到跨行连续选区。延迟按住 Alt 后再点也相同。
这与 DispatchEvent 矩阵通过并存，不能以模拟结果覆盖真实输入失败。

源码证据为 GOUI v0.7.0 的
[Win32 客户区输入实现](https://github.com/golang-gui/goui/blob/v0.7.0/platform/windows/win32/input_windows.go#L222)：
handlePointerMove / handlePointerButton 调用 pointerModifiers(wParam)，其只读取 MK_SHIFT 与 MK_CONTROL，
遗漏 Alt 与 Win。非客户区路径另查按键状态，不能补齐客户区事件。
微软 [WM_LBUTTONDOWN 文档](https://learn.microsoft.com/en-us/windows/win32/inputdev/wm-lbuttondown)
明确要求用 GetKeyState(VK_MENU) 判断 Alt，不能用 GetAsyncKeyState。
本轮只记录，不在 SourceView 内旁路查询平台输入、不改示例默认配置、不修改或发布 GOUI。

macOS 的远程工具 ALT 发送 VNC Alt_L；尚未建立它与本次 AppKit Option 事件的端到端证据。
GOUI v0.7.0 的 [Cocoa 指针路径](https://github.com/golang-gui/goui/blob/v0.7.0/platform/darwin/cocoa/input_darwin.go#L243)
读取 event.ModifierFlags 并映射 Option，没有 Windows 同样的静态遗漏；不能据此断言真机手势正常，
也不能将未定位问题归因为 GOUI。后续需区分 VNC 映射与实际 Option 键输入。

Windows RDP 的直接 Unicode 输入“中文 é 👩‍💻”能显示、撤销；切换输入尝试只产生 ni，
没有预编辑或候选，不算 IME 通过。macOS 的远程 Unicode type 调用没有插入 x，
改用普通按键 X 能完成上述双点替换；不将协议文本输入结果等同真实 IME。
本轮未验收原生 Wayland、Windows ARM64、macOS Intel、混合 DPI 或辅助技术操作。

## 副作用、证据和后续门槛

旧 structural 窗口程序写入原生剪贴板并在成功后清空，本轮在两端实际触发；运行前未备份，
无法保证恢复此前内容。这是测试副作用，不能声称本轮未修改系统剪贴板。
最终矩阵与后续手工测试前备份原生剪贴板，结束后恢复并逐项核对格式数据一致：
Windows 四种格式，macOS 一项一种类型；此恢复基线不能补回首轮未备份前的内容。
快照只保留在远端恢复过程，不打印内容、不纳入日志归档，核验后删除敏感快照。
Windows 首次启动时 Run 对话框尚未就绪，命令前缀丢失而未启动测试；重新观察对话框后执行成功。

两端示例均以退出码 0 关闭，只关闭本轮新建的 macOS 小 Terminal 窗口；保留原有应用、
Terminal 标签和文档。最终 Linux 私有 Xvnc/Xfwm4 已退出，未操作用户桌面 :3。
已释放键鼠并断开本轮 mac/windows 桌面客户端，保留原有 SSH 连接；未修改桌面快捷键或全局 Go 配置。

本机证据根 /tmp/sourceview-v060-gswb38qp；远端分别为
/private/tmp/sourceview-v060.5ETZIW 和 Windows TEMP/sourceview-v060-376f8be4。
logs、trial、matrix 保留首轮日志，diagnostics 为仓库外临时日志 overlay，不作为发布源码。
最终包测试见 final-linux/test.jsonl、win/final-test.jsonl、mac/final-test.jsonl；
三端 final-matrix/results.json（Linux 为 final-linux-desktop/results.json）各记录 42/42。
远端首轮及最终日志已下载至本机 mac/evidence-logs.tar.gz 与 win/evidence-logs.zip，
不含剪贴板快照；screenshots 包含真实 Alt/Option 手势失败、键盘编辑、保存点和特殊字符证据。

后续仅在维护者恢复发版工作后处理：Windows 原生输入修复及依赖版本、macOS Option 手势定位、
真实 IME 验收，再以最终源码重复受影响原生检查。当前未推送、未打 v0.6.0 标签、
未创建 Draft 或正式 Release，官方 Go proxy 的 v0.6.0 消费验证未执行。
收尾文档检查通过：差异无空白错误，六份文档的 125 个本地链接目标存在；
当前工作树 `GOWORK=off go test ./...` 再次通过（测试命中缓存）。
