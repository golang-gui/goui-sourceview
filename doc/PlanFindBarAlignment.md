# FindBar 顶部操作行对齐

2026-10-19；SourceView 基线 c67a9fd，本地 GOUI b58948f。设计见
[FindBar](DesignFindBar.md#浮层布局与配色)。

## 问题与修正

旧布局将选项、计数及操作按钮按输入框的完整高度居中。多行文字或软换行增高输入框后，
这些控件向下移动，不便与首行输入一起操作。

FindBar 独立测量顶部操作行，按原生选项/状态尺寸保留紧凑高度；输入视口仍按内容增长。
展开、大小写/整词/正则/选区选项、计数、上下项、关闭及替换动作在各自操作行内排列。
GOUI 的文字排版、滚动、输入与命中继续复用现有公开契约，本轮无需修改 GOUI。

## 回归验证

在原有 `tests/window/editor/find_appearance` 中保留外观、布局、主题隔离、窄宽度、焦点和命中断言，
新增紧凑行内控件边界及相对输入外框顶部的位置断言。分别验证查询/替换增高、达到高度上限、
正常 WheelEvent 滚动、收缩，以及无 LF 查询的软换行。输入通过 Window.DispatchEvent / CommitText。

同一新增断言在未修改的生产代码上失败：多行查询使匹配计数距输入外框顶部从 8.5 DIP 移至
36.5 DIP，退出码 1。这是回归复现，不列为修正后通过项。

首次调试断言时，零匹配状态将计数的样式名切换为 error，按单一样式名查找的测试代码取到 nil。
修正测试查找并增加显式存在断言后，才完成上述旧实现复现；不将该测试代码异常当作产品缺陷。

环境为 Go 1.25.8 / Linux amd64 / Pango / X11（独立 Xvnc :146、Xfwm4），
软件绘制；窗口日志确认实际缩放为 1x / 2x。所有验证不依赖 DevServer。

通过：

- 本地 GOUI b58948f：`go test ./...`、`go test -race ./...`、`go vet ./...`。
- `go build -race -o /tmp/sourceview-find-top ./tests/window/editor/find_appearance`，
  以 `DISPLAY=:146 XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1/2`
  运行；14 pt 的 1x / 2x，以及 `-font-size 18` 的 1x 全部断言通过。
  增高阶段同时验证 Modern 亮暗主题；较大字体仍满足选项内容不裁剪的原有断言。
- 无 replace 的正式依赖快照：`GOWORK=off go test ./...`。
- 同一正式依赖快照：`GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`、
  `GOWORK=off GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`。
- `git diff --check`、受影响文档本地链接检查。

`-capture-dir /tmp/sourceview-find-top-captures` 输出的挂载控件渲染图用于检查亮暗多行布局，
不是桌面合成截图。未执行 OpenGL、Windows/macOS 原生窗口、实体输入法和混合显示器缩放验收；
交叉构建不代替这些结果。

验证只修改测试文档内存，不修改真实剪贴板或系统设置。截图、二进制和日志保留在仓库外，
结束时关闭本轮独占的窗口、窗口管理器与 :146 显示会话，保留既有桌面。

## 依赖条件

工作区原有 go.mod 的本地 replace 保留、不提交。正式依赖仍为 GOUI v0.2.0，
独立依赖测试使用仓库外、无 replace 的快照并关闭 go.work。
内容驱动输入测量的既有版本条件见 [TextView 测量记录](PlanTextViewSizing.md#依赖条件)；
本轮对齐修正不补做该通用能力，也不将旧版本的编译通过当作完整多行窗口体验验收。
