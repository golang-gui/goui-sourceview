# 命令快捷键配置实施记录

2026-10-19，最终验收基线 SourceView cc2f4bc。设计见 [命令快捷键](DesignKeyBindings.md)。
用户已授权实现、本机验证并按功能切片提交；本轮不修改 GOUI。

## 范围

- 固定 Command 和每视图 KeyBindings，保留平台默认操作；手动补全默认不绑定键。
- 配置复制、整表替换、清空/恢复、解析别名冲突与原子失败。
- 核心组合键、FindBar 和 GoToBar 统一读取当前表；普通输入及瞬时面板交互保持原路径。
- 薄 ui 只协调显式配置；菜单复用同一命令标识。示例/原补全专项显式选择 F2 并更新说明。

## 依赖与工作区

任务开始时 go.mod 已有用户的未提交 replace github.com/golang-gui/goui => ../goui。
保留且不纳入本轮提交。工作区验证使用该本地检出（起始 HEAD b58948f）；正式依赖仍声明 v0.2.0。
独立发布验证使用仓库外快照和已跟踪的无 replace go.mod，并关闭 go.work；不能只用
GOWORK=off 在原工作区声称排除了 replace。

核心配置、回归验证、示例和展示更新均已完成，按功能与示例分开提交。

## 验证

环境为 Go 1.25.8、Linux amd64、X11/Xvnc、Xfwm4、Pango；窗口程序在独立的 2560×1800
显示会话中运行，不依赖 DevServer。缩放比例来自程序实际输出，不以环境变量设置值代替。

| 范围 | 命令 / 场景 | 结果 |
| --- | --- | --- |
| 本地 GOUI b58948f | `go test ./...`、`go test -race ./...`、`go vet ./...` | 通过 |
| 正式 GOUI v0.2.0，无 replace 的仓库外快照 | `GOWORK=off go test ./...`、`GOWORK=off go test -race ./...`、`GOWORK=off go vet ./...` | 通过 |
| 正式依赖跨平台构建 | `GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`；`GOWORK=off GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` | 通过，未作原生运行验收 |
| 新命令绑定窗口，本地依赖、race | `go build -race -o /tmp/sourceview-keybindings-native ./tests/window/editor/keybindings`；software 1x/浅色、2x/深色，OpenGL 1x/浅色 | 全部断言通过 |
| 同一新窗口，正式依赖、race | 无 replace 快照构建；software 1x/浅色、2x/深色 | 全部断言通过 |
| 原窗口专项，本地依赖 | `shortcuts`、`lines`、`find`、`folding`、`languages`、`extensions` | software 1x，正常事件路径断言通过 |
| 补全面板专项，本地依赖 | `go run ./tests/window/editor/completion -case json -auto` | software 1x，键盘、悬停、滚轮、点击、焦点与撤销断言通过 |
| 示例截图，正式依赖 | `go run ./example/editor -theme light`，通过真实 X11 输入切换深色与 HTML | software 1x，Integrated 窗口；截图见下节 |
| 差异与文档 | `git diff --check`、本地文档链接检查 | 通过 |

窗口运行设置 `XMODIFIERS=@im=none`，分别指定 `GOUI_PLAT_PAINTER=software/opengl`、
`GOUI_PLAT_SCALE=1/2`。新窗口断言默认 Ctrl+Space 放行、显式 F2 补全、弹出层交互、
替换/删除/清空/恢复绑定、同 Document 的视图隔离、只读消费、父级快捷键接续、可选栏打开与
字段内结果导航，以及回调重配后的事件边界。特别覆盖补全面板关闭回调更换绑定时，同一按键
不会继续执行新命令；相同配置不会使当前事件失效。

首次启动补全专项误用了 `-scenario json`，程序报未知参数并退出，未执行场景。
按入口说明改为 `-case json -auto` 后通过；此参数错误不作为功能通过项。

未执行 Windows/macOS 原生窗口、实体键盘的系统保留键、原生 IME 或混合显示器 DPI 验证。
正常事件派发能确认控件不再默认消费 Ctrl+Space，不能据此承诺系统输入法的具体切换行为。
FindBar 输入区尺寸和垂直对齐不在此轮修改与验收范围内。

## 示例与展示

三个示例显式配置 F2，工具栏、Markdown 样本、嵌入 JSON 与双语示例说明同步更新。
README 的亮/暗截图来自使用正式 v0.2.0 依赖构建的实际窗口；浅色展示 JSON，深色展示 HTML，
对应标题和替代文本随截图更新。截图没有重绘或伪造界面。

验证仅修改测试文档和示例的内存状态，不改真实剪贴板或系统设置；截图更新属于本次交付。
专用桌面会话和示例进程在结束时关闭，不影响已有桌面。
