# FindBar 范围交互与搜索高亮修正

2026-10-19；SourceView 基线 077f4c0，GOUI 正式依赖 v0.2.0。
用户授权修正问题 2、3；输入控件排版交由 side 单独规划和处理，本轮不修改 GOUI 或字段布局。
设计见 [FindBar](DesignFindBar.md)、[查询](DesignSearch.md)。

## 修改范围

- 选区按钮替换为四角选区边界与文本行图形，维持八语言名称和 Basic/Modern 正常状态样式。
- 未开启且无选区时禁用，开启后可关闭；范围切换从原选区起点立即定位并更新首个计数分块。
  捕获范围保持独立，空结果保留普通选区；公开空范围/零长度正则契约不变。
- 搜索背景显隐成为视图属性，查询与导航不受影响；FindBar 关闭清范围并隐藏背景，重开恢复。
  卸载/换目标清理旧会话；回调更换状态后放弃旧导航，不覆盖新状态。
- ui 只在显式配置时协调搜索显隐，普通重建保留交互状态。

## 验证

验证包括普通及 GOWORK=off 全仓测试、必要的 race、原生窗口的
空选区/只读、字面量/正则范围切换、零/多结果、分帧计数、关闭与 F3、重开及声明式重建、
换目标/卸载/回调重入，以及现有选区像素断言中的搜索显隐。
窗口输入通过 Window.DispatchEvent/CommitText，不使用 DevServer 或语义动作捷径。

通过结果（Linux amd64 / Go 1.25.8，依赖 GOUI v0.2.0，go.work 只含本模块）：

- go test ./...、GOWORK=off go test ./...、go test -race ./...、go vet ./... 通过。
  Windows amd64 / macOS arm64 的 CGO_ENABLED=0 全仓构建通过；不是对应平台原生运行验收。
- 隔离 Xvnc :140 / 2560x1800 / Xfwm4（--compositor=on）/ Pango：扩展 search 窗口最终 18 个
  场景在 software 配置的 1x Light、2x Dark 下以 -race 运行通过，包含未挂载目标不受误清理的断言。
  find 窗口在实际 *software.imageResource、PixelScale=1 下通过快捷键、重建、换目标和卸载断言；
  另一次实际 *opengl.imageResource、PixelScale=1 的 find 窗口也通过。
- selection 窗口 JSON/XML/HTML × Basic/Modern Light/Modern Dark 九组原生像素断言通过，
  新增关闭及恢复搜索背景后继续断言原色值、选区合成和语法前景，没有放宽原有容差。
  此轮该窗口日志实际 scale=1，未单独探测其 painter 类型。
- find_appearance 在 software / scale=2 配置下通过浮层颜色、字号、焦点、命中和窄宽度回归；
  查看了实际展开截图，新的选区边界图标可见，字段布局没有调整。
- 未执行 Windows/macOS 原生窗口、物理 IME、分数/混合 DPI 或实体键鼠。
  测试只使用内存文档，不写剪贴板或系统设置；二进制、截图和日志均位于 /tmp。
  临时 :140 桌面及其窗口管理器已停止，原 :3 桌面未关闭。

额外发现：初版计数夹具为 80000 字节单行（20000 个 foo），带 race 的原生窗口在第 16 场景
超过 90 秒。另用 35 秒 SIGQUIT 获取堆栈，主线程位于 paintAssistanceBackground -> paintRange ->
GOUI v0.2.0 textedit.Geometry.Selection 的同词高亮路径；该绘制逻辑本轮没有改动。
此超长行组合没有通过，不作为成功验收。计数分块用同为 80000 字节的 20000 短行替代，
范围坐标及所有结果/分块断言保留；超长行的同词/空白装饰成本留作独立问题，不以增加超时掩盖。

## GOUI 前置要求

Find/Replace 所需的紧凑多行输入能力在 GOUI 单独规划：稳定基线与单行垂直居中，保留 LF，
按实际排版增高至上限后滚动，文字/Placeholder/光标/选区/命中/IME 使用同一坐标。
本轮保留原字段实现，不将范围与高亮修正依赖到该前置，也不宣称输入框居中已经修复。
