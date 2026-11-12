# v0.6.0 发布准备与原生验收

日期：2026-11-12。维护者授权 Windows/macOS 真机编辑验证及发版准备，未授权正式发布。
当前仍在验证；不把下述失败或未执行项计为通过。发布 Go 库与源码示例，拟用版本 v0.6.0，
不制作二进制附件。GOUI 仍为 v0.7.0；最低 Go 1.24，无 replace 或 go.work。

## 初始冻结与环境

- 初始候选 c58b7d3dd1872633754e73e4798fcaf50154b601，git archive 共 567 个文件。
- 归档 SHA-256：47d78513afb6f150e28847f0cc17a49a4eb314af274087f52571fcc03a900ec3。
- 两端传输后核对归档及全部文件 SHA-256，原生编译和测试使用专用临时目录。
- Linux：Ubuntu 22.04.5 amd64 / Go 1.25.8 / Pango 1.50.6。
- Windows：Windows 10 19045 amd64 / Go 1.25.8 / DirectWrite / Direct2D，实际 RDP 登录桌面。
- macOS：15.7.7 arm64 / Go 1.24.9 / CoreText / OpenGL，实际 VNC 登录桌面。
- 窗口均 CGO_ENABLED=0，包检查 GOWORK=off、GOTOOLCHAIN=local；不改全局 Go 环境。
  100%/150% 指 GOUI 应用缩放，不等同真实混合显示器 DPI。

## 首轮结果与修正

Linux 初始原生 go test -json ./... -count=1：2356 项通过、无失败/跳过；
mod verify、tidy -diff、vet、CGO=0 build 和全仓 race 通过。
Windows 初始包测试 2355 通过、1 失败；macOS 2353 通过、3 失败。
两端 mod verify、tidy -diff、vet、CGO=0 全仓及二十个窗口/示例程序构建均通过。

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

## 窗口矩阵与待解决项

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
  修正后的 macOS Basic 150% 大长行窗口通过，完整矩阵等待最终冻结后重跑。
- Basic 100% 空白圆点直径小于一个物理像素，其 2×1 覆盖被至少 2×2 的断言拒绝。
  补充实际 FillEllipse 的等半径和原生单元中心断言；亚像素覆盖按尺寸核验，较大圆仍要求二维。
  增加矩形、错误半径/中心、不可见圆和 4×1 扁条的负向回归，未改变绘制规则。

## 副作用、证据和后续门槛

旧 structural 窗口程序写入原生剪贴板并在成功后清空，本轮在两端实际触发；运行前未备份，
无法保证恢复此前内容。这是测试副作用，不能声称本轮未修改系统剪贴板。
后续涉及剪贴板的场景先备份并恢复；不关闭原有应用或改动其文档。
Windows 首次启动时 Run 对话框尚未就绪，命令前缀丢失而未启动测试；重新观察对话框后执行成功。

本机证据根 /tmp/sourceview-v060-gswb38qp；远端分别为
/private/tmp/sourceview-v060.5ETZIW 和 Windows TEMP/sourceview-v060-376f8be4。
initial/test、trial、matrix 保留首轮日志，diagnostics 为仓库外临时日志 overlay，不作为发布源码。
源码变化后需独立提交、重新冻结、核对两端清单并执行最终原生检查；还需真实示例输入验收、
清理本轮桌面窗口/连接、整理中英文 Release Notes 与迁移说明。
发布前须单独授权 main 推送、annotated tag、Draft/正式发布与官方 Go proxy 消费验证；
当前未推送、未创建 v0.6.0 标签或 Release。
