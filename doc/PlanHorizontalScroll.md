# 输入长行后的水平滚动修复

2026-11-09，基线 `89a9baf`，GOUI v0.7.0。设计见 [布局设计](DesignLayout.md)。

## 复现与修复

内容范围未包含 Geometry.Caret 的 1 DIP 宽度，而行尾显露请求包含该宽度和右 padding。
即使宿主滚至最大值，请求仍可能不结束。小数视口还会遇到整 DIP 范围钳制，导致少量期望
边距不可达。后续拖动滑块被未完成的显露请求拉回，Home 重新定位后才恢复。

不折行内容范围计入光标宽度；显露完成检查真实光标完整可见，padding 保持为滚动目标。
保留已测覆盖区域、实际宿主偏移、请求身份与生命周期检查，不修改 GOUI、分页或公共 API。
此前布局设计中的滚动宿主与 SourceView 显露职责仍适用，不新增滚动控制器。

`horizontal_scroll_test.go` 使用独立的 10 DIP 字符度量，80／65530／66000 字符与 400／401.5 DIP
视口六组全部复现显露不结束（before-confirmed.log）。首次测试误把普通 64 KiB 段落也放入
只接受分页送测的替身，两个准备断言失败，保留 before.log；改用普通固定度量替身后确认六组均为
产品行为失败。修正后六组及全部根包测试通过（after.log），包含巨型坐标、缓存和其他显露回归。
测试保留行尾选区，验证显露结束、反复回到水平偏移零、正文／历史不因滚动改变和资源释放。

独立窗口程序 `tests/window/editor/horizontal_scroll` 从空行输入、跨 64 KiB 阈值、编辑已分页长行，
通过真实 PointerDown/Move/Up 抓取水平滑块，验证左／右／左拖动和后续帧不回弹。
光标停在行尾，未用 Home、移动光标或 SetScrollX 帮助回到原点。实际抓取前只用轨道点击准备远端，
按下滑块时断言偏移不跳变。拖动前后比较正文、修订、选区和历史可用性，之后新建独立撤销分组，
验证再次输入仍能显露行尾、Undo 恢复该字符和选区。

旧版加入同一窗口程序后在首次实际拖动失败（desktop-before.log）。修复版 Basic 软件 1×
三种样本初验通过（desktop-basic.log）。其余主题／后端／缩放及原有长行、补全窗口回归进行中。

## 检查与限制

GOWORK=off 全量测试、race、vet、Windows amd64／macOS arm64 CGO_ENABLED=0 交叉构建通过
（b1-checks.json）。独立导出与最终窗口结果待补。修复和窗口验收按英文分别提交。
日志、构建和独立导出位于 `/tmp/sourceview-horizontal-scroll-3_f5gijv/`。

Linux amd64 / Go 1.25.8 / Pango；专用 TigerVNC 1.12.0／Xfwm4 4.16.1，隔离显示、认证和 XDG 目录。
WM 的 Another compositing manager 警告不作为合成或硬件 GPU 验收。
物理输入法、混合 DPI、其他系统原生运行和性能对照未执行；交叉编译不代替原生运行。
既有草稿不纳入提交，不修改 GOUI，不推送或发布。
