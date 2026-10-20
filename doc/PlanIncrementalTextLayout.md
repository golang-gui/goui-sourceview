# 长段落排版优化记录

## 范围与状态

2026-10-20，用户明确选择保持完整正确性并继续通用方案。GOUI 提供通用排版
基础，本仓只接入公共契约，不基于 TextView 或复制其私有 editor。

- [x] 核对 Scintilla／Monaco、三个原生后端的相关能力，记录独立窗口原型失败。
- [x] GOUI 的几何索引和可视行选区查询；本仓选择／搜索／出现词／预编辑绘制接入。
- [x] 长段落全选仅产生可视行矩形的包内回归。
- [ ] 保留完整上下文的可恢复分析和 shaping。
- [ ] 按事件循环间隔分步推进、测量缓存复用、增量折行、取消与生命周期。
- [ ] 未完成排版期间仍可拖滚动条、操作其他控件的真实窗口验收。

**当前仍会在首次原生长段落排版时阻塞。** 不能以查询优化宣布整个需求完成。
带 bidi 缺陷的独立窗口原型没有接入正式控件，未使用后台测量。
方案与原生能力门槛见 [布局设计](DesignLayout.md)，后端诊断和同环境查询对照见
GOUI [实施记录](../../goui/doc/PlanIncrementalTextLayout.md)。

## 依赖

本轮使用相邻 GOUI `554dbdb` 之后的改动，需要尚未发布的 `21444f9` 中的
`gui/textedit.Geometry.LineAtY` 和 `SelectionLines`；GUI 使用方在 `25e52c1`，
诊断在 `33113b9`，设计与记录在 `b022304`。本仓代码提交为 `6224e5e`。
现有 go.mod 的本地 replace
为用户原有改动，本轮保留；没有修改全局 Go 环境或提交机器路径。

使用本地 go.work 联调。`GOWORK=off` 仍会读取现有 replace，因此该命令成功
也不能算独立依赖验证；需另外使用仓库外临时 modfile 去除 replace 检查实际
v0.2.0 依赖。已有 Rasterize 接口变化也尚未由该版本提供。

## 验证

Linux amd64／Go 1.25.8／Pango 1.50.6，通过：

- `go test ./...`、`go test -race ./...`、`go vet ./...`。
- GOUI `go test ./...` 及 `go test -race ./gui/textedit ./gui`。
- Windows amd64、macOS arm64 的 `CGO_ENABLED=0 go build ./...`。
- `XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/selection`：
  X11／XFCE，Basic／Modern Light／Modern Dark 的 JSON／XML／HTML 九组通过，
  包括正常事件分发、语法前景、选区背景像素、焦点与菜单。窗口自动关闭，
  无剪贴板或系统设置修改。

失败单独记录：

- `GOWORK=off go test ./...`：`go: updates to go.mod needed`，没有执行 tidy 改动用户文件。
- 仓库外临时 modfile 删除 replace 后，`GOWORK=off go test -modfile ... ./...`
  在新增 `LineAtY`／`SelectionLines` 编译失败；不能把工作区通过当成独立发布通过。
- GOUI `go vet ./gui/textedit ./gui` 有历史未命名字段诊断；与 HEAD 对照相同，
  详见其实施记录。

首次长段落排版仍同步，窗口响应目标未通过验收。Windows／macOS 原生运行、
真实 IME／物理键鼠和混合 DPI 未执行。上述选区窗口用例不能替代长段落性能验证。
