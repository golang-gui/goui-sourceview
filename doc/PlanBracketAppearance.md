# 括号提示视觉验收（2026-10-16）

实现为 `fd6cac4`，基于 SourceView `29e2d62`，联调 GOUI `e345c5c`。
本轮未修改 GOUI。行为与主题字段见 [括号设计](DesignPairs.md)。

## 调整与回归

将光标色 2 DIP 下划线改为独立主题色的淡背景与 1 DIP 内缩边框。
仅显示已确认的配对，保持语法前景、native layout、Geometry 和输入/历史行为。
背景在搜索背景下方，边框在文字下方；同一 shaping Cluster 的矩形只绘一次。
背景/边框 alpha 可独立关闭，未匹配、未知、有选区和失焦不显示误导性单端强调。

`bracket_paint_test.go` 覆盖精确边界/线宽、绘制顺序、主题色、文字与布局对象复用，
以及未配对/字符串/选区/未知状态、独立透明、折行/bidi/滚动和共享 Cluster。
通过正常 EventDispatcher 焦点路径建立测试状态，不导出私有状态。
使用新测试与旧 Paint 函数体复验，亮暗两例均失败：0 填充、0 边框、2 条下划线；
新实现通过相同断言。

## 已通过

- 本机 Ubuntu 22.04.5 amd64、Go 1.25.8：`go test ./... -count=1`、
  `go test -race ./...`、`go vet ./...`、`git diff --check`。
- 独立 X11/Xtigervnc + Xfwm4、Pango 1.50.6：
  - Software 1x：亮暗两种主题分别检查嵌套、相邻、跨行、折行、搜索、未配对、选区、失焦。
  - Software 2x：亮暗两种主题检查嵌套与跨行。
  - OpenGL 1x：亮暗两种主题检查嵌套。
  - 实际 painter 由图像资源类型记录，缩放读取 `gui.Painter.PixelScale()`。
  - 抓取真实窗口 RGB 截图，匹配场景断言专用背景和边框色存在，抑制场景断言两色均不存在；
    颜色匹配容差为 0，只统计不透明像素，不对字体及抗锯齿边缘建立全图金图。
    截图为合成后的不透明 RGB，未把它当成借用的预乘 RGBA 文本位图。
  - 实际查看亮暗、1x/2x、跨行与搜索截图，确认细框贴合字符且文字保留语法色；
    截图颜色断言只证明出现/隐藏，边界和线宽由独立几何断言补充。
- 同一隔离桌面 `go run ./tests/window/experience` 通过输入、菜单、补全、配对与历史回归。
- Windows 10 19045 amd64 / Go 1.24.0 / SSH Session 0，以及 macOS 15.7.7 arm64 /
  Go 1.24.9：SourceView `go test ./... -count=1` 通过，包含新绘制逻辑测试和既有
  DirectWrite/CoreText 原生测试；这些不计作两端的真实窗口视觉验收。

窗口程序：`go run ./tests/window/editor/brackets [-dark] [-case CASE] [-auto]`。
具体环境、逐项预期和退出方式见入口注释；READY 只表示原生设置就绪，不表示像素验收通过。
截图、临时可执行文件和远端快照留在任务临时目录，未向源码树写产物。
独立 X11 显示没有改写日常桌面的焦点、剪贴板或系统设置。

## 仍有效的限制

`GOWORK=off go test ./...` 仍因已发布 GOUI v0.1.1 缺少 ApplyBatch/BatchOptions 等接口失败；
不是本轮新增依赖问题，不将工作区通过写成独立发布可用。
Windows/macOS 的边框原生窗口、分数缩放、不同代码字体的实际外观仍待验证；
当前验证不宣称覆盖所有字体、后端或任意 GUI transform 的物理像素对齐。
