# GOUI 本地更新适配

后续：2026-11-15 已切换为发布依赖 GOUI v0.8.0，工作区不再是构建本仓的前提，
实际结果见[升级记录](PlanGOUIV080.md)。以下保留 11-14 当时的本地版本、失败和验证边界。

日期：2026-11-14。用户授权查看本地 GOUI 更新并进行适配迁移；此前暂缓发版的决定继续有效。
只修改 SourceView；保留两仓既有未跟踪文件，不修改或发布 GOUI。

## 基线与设计

- SourceView 起点：`e5df032`，公开依赖 GOUI v0.7.0。
- GOUI 本地基线：`538ed4fa2d677249094eb2891dbd3b1497e53bc3`，分支 feature/app-dev-gap。
- GOUI v0.7.0 标签：`5006d3642ca18d5c8508a1b4109c5185d11a5537`。
- Go 仍为 1.24；新接口尚未发布，通过忽略的 `go.work` 联调两个模块。
  不提交绝对路径 replace 或不可获取的伪版本，不将本地工作区通过写成独立可发布。

| 本地 GOUI 变化 | SourceView 适配与边界 |
| --- | --- |
| `33d91b7` 类型化输入回调 | Motion、Click、Drag、ContextMenu 使用 Info 的局部 DIP；EventContext 只用于同步传播；取消迁移到 ConnectCanceled，不伪造释放 |
| `553bcd4` 等 Surface 公共宿主 | 正文焦点、补全和菜单通过 Widget.Surface 获取 owner；跳转栏内部 Surface 字段改为 Content，避免与新方法冲突；不承诺移动端完整运行 |
| `14403eb` 焦点策略 | 正文指针不再显式 SetFocusedWidget 覆盖分发器的 FocusOnPointer/CanContainFocus；显式键盘菜单仍通过宿主聚焦 |
| `b8448cc` 共享瞬态面 | 补全使用 NewTransientPopover，透明/不透明创建降级交由 GOUI；Closed 与 DismissRequest 都关闭当前会话，旧 presenter 回调不能关闭新会话 |
| `6593720` 外向滚动范围 | 窗口断言使用公开 MaxScrollX/MaxScrollY，不再套旧的向下取整假设；正文不重建 GOUI 私有滚动范围算法 |
| `33886bc` 已完成的 Chrome 几何 | 布局待处理不再禁用窗口 resize 命中；视图恢复验收的内容留出 12 DIP，滚动条不占用窗口边缘手势区域，保留原偏移变化断言 |
| `b40908c` Windows Alt 修饰键 | 接收 GOUI 正常 PointerEvent，不增加平台查询旁路；旧真机失败记录保留，源码修复不等于 SourceView 已重跑真机通过 |
| Application 生命周期/Run 结果 | 四个命令式示例处理 Run 返回错误；语义分析示例先完成 worker 清理再报告。桌面 pre-Run 构造仍是公开支持路径，不扩展为移动入口 |
| `8be59df` BeforePaint | 已核对；其在布局后、Begin 前且 RenderWidget 不触发。当前正文已使用自有有界 Frame 排版和离屏路径，不机械替换预算调度或增加空准备钩子 |
| 多击计数、空闲光标、树/表格、拖放等 | 不复制 GOUI 私有文本编辑策略；正文继续独立持有多击/选区/历史。通用控件变化由既有 Find/GoTo 和示例组合直接获得，不扩大为新增编辑功能 |

契约依据为相邻 GOUI 的 [输入回调](../../goui/doc/DesignInputCallbacks.md)、
[共享浮层](../../goui/doc/DesignPopup.md#窗口内置浮层的共享宿主2026-11-14)、
[绘制准备](../../goui/doc/DesignWidgetPainter.md#布局后绘制准备2026-11-09)
与相同提交的公开源码。没有通过反射、unsafe 或私有包访问兼容旧签名。

## 修改与验证范围

生产代码涉及 sourceview/input.go、menu.go、sourceview.go、completion.go、goto_bar.go，
internal/controls/completion.go、goto.go，以及四个命令式示例的 Run 错误处理；
窗口验证又暴露自有 paragraph_layout 的方向上下文缺陷，修正范围见下节。
不改变公共编辑模型 API、默认多光标修饰键或按键表。
声明式控件继续嵌入 GOUI ViewBase，复用通用焦点修饰与原协调生命周期，不重复增加状态。

包内回归通过正常 EventDispatcher 检查有偏移容器中的正文/补全局部坐标、Alt/Option、
真实释放与取消，保留原有用例断言。新窗口主题 tests/window/editor/goui_integration
检查焦点策略及快照、Surface 归属、三轮浮层取代后的 Enter/Undo、旧 presenter 隔离，
以及跳转/查找子树和 Escape 聚焦。输入走 Window/Popover 正常事件路径，不使用 DevServer。

还执行全仓测试、受影响包 race/vet、Windows/macOS 交叉编译及既有窗口回归。
桌面验证使用独立 Xvnc/Xfwm4，剪贴板仅属该临时显示；不操作用户桌面。
本轮不自动重启 Windows/macOS 的用户桌面验收，也不恢复发版流程。

## 开发中结果

- 首次工作区 go test ./...：旧 Click/Motion 回调签名不兼容，编译失败；记录 baseline.log。
- 迁移回调后：跳转栏 Surface 字段与 Widget.Surface 方法冲突，编译失败；
  改名内部 Content 并保留原子树/留白断言后全仓测试通过。
- 新包回归 TestTypedPointerCallbacksKeepLocalSelectionAndCancellation、
  TestTypedCompletionPointerUsesLocalRows 通过。
- 新窗口首次构建误用 Popover.Snapshot（未公开）；改用 Widget().Snapshot 后构建通过。
- 移除补全 ConnectClosed 的仓库外负对照按预期失败：另一组件取代补全后，隐藏候选截获 Enter，
  与独立预期 tr+LF 不符。最终实现三轮替换、重新打开、接受及撤销通过。
- 首轮 48 组窗口 46 通过、2 失败：Basic 100%/150% 的滚动条拖动不改变偏移。
  诊断坐标 x=995 位于 1000 DIP 窗口的右侧 resize 边缘，公开 ConnectQueryRegion
  返回 ChromeRegionRight（9）。旧用例依赖 pending layout 暂时关闭 Chrome 命中的旧行为。
  给测试内容留出 12 DIP 后 Basic 通过，原断言没有删除或放宽。
- 留边后的六组视图恢复为 2 通过、4 失败，暴露 Modern 的真实分页接缝错误
  `paragraph seam has no line at byte 954263`，未作为环境问题或通过结果处理。

## 随机分页的方向修正

独立原生包测试用 1,050,000 字节的重复 LTR/中文/希伯来文样本，先准备首屏，再随机准备
第 232、233 页，在旧实现准确复现相同接缝错误。LTR 逻辑段落的局部窗口先遇到 RTL
强字符时，原生 auto_dir 将它当成 RTL 段落；接缝重新测量又得到不同断行。

本仓窗口与接缝布局都继承已知段落方向及前一强字符上下文，前缀只在原生边界存在，
正文不变；保留原生输入大小、12 页上限和分帧预算。不忽略错误、不猜测行高或加像素补偿。
新增回归以显式 LTR 原生参考逐行核对起点、长度、X、宽度；参考区间尾端按 UTF-8
边界对齐，避免初版参考读取落在 rune 内。原代码 overlay 在独立参考的 X/宽度断言失败，
修正后通过；该专项用公开 GOUI v0.7.0 也通过，确认修复不需要新增框架契约。
设计依据与限制见[布局设计](DesignLayout.md)，独立提交 `68a8185`。

## 最终验证

环境：Ubuntu 22.04.5 amd64 / Go 1.25.8 / Pango 1.50.6 / software；独立 Xvnc、
Xfwm4 合成开启的 100%/150% 应用缩放，非真实混合 DPI。两仓均使用上述本地 GOUI 提交。

| 命令或场景 | 结果 |
| --- | --- |
| `go test -json ./... -count=1` | 2362 项通过，0 失败/跳过，包含命名子项 |
| `go test -race ./sourceview ./internal/controls ./internal/view ./ui` | 四包通过 |
| `go vet ./...` | 通过 |
| `CGO_ENABLED=0 go build ./...`，Linux amd64 / Windows amd64 / macOS arm64 | 通过 |
| 二十个示例/窗口入口分别构建，三平台各一轮 | 通过 |
| Windows/macOS `go test -c ./internal/view` | 编译通过，不代表原生运行 |
| GOUI gui 的 ClickInfo / ClickReleased / ClickReset / Motion / Drag / TransientPopup 定向测试 | 通过；未修改 GOUI |
| 原四类能力 × 三主题 × 两缩放及十八组编辑回归 | 最终 42/42 通过 |
| 新 goui_integration × Basic/Light/Dark × 100%/150% | 6/6 通过 |
| 无合成器不透明补全降级 100%、32 候选滚动/滑块/接受/撤销 150% | 2/2 通过，透明状态明确为 false |
| `GOWORK=off go mod verify`、`go mod tidy -diff` | 通过，无模块文件差异 |
| `GOWORK=off go test ./...` | **失败**：公开 v0.7.0 缺少 gui.ClickInfo 等新契约；不是独立可发布状态 |

最终窗口共 50 组通过，完整重跑保留初轮失败日志，不用单项复跑覆盖首轮结论。
最后补齐 Run 结果处理后再执行工作区 `go test ./...` 及三平台 CGO=0 全仓构建通过；
没有因示例错误处理的改动重跑无关窗口场景。

证据根 `/tmp/sourceview-goui-migration-6ffzgt5p`：baseline/first/second.log 记录编译适配，
matrix、view-state-rerun 记录首轮及留边后的失败；diagnose-chrome-result.log 记录原生边缘角色，
seam-before.log / seam-negative.log / seam-oracle.log 记录独立复现、负对照及参考核验。
final-test.jsonl、final-race.log、final-vet.log、final-build-*.log 为最终包检查/构建，
final-matrix/results.json 为 48 组，extra-matrix/results.json 为无合成器的另两组；
negative-result.log 为隐藏补全的预期失败对照。

本轮所有桌面进程、WM 和专用显示按 runner 的 finally 清理；用户桌面 :3 未操作。
structural 等旧程序仅覆盖临时显示的剪贴板；不访问 Windows/macOS 真实剪贴板或桌面。
保留已有 doc/Doing.md、example/temp/ 和 GOUI 未跟踪文件，go.work/go.work.sum 被忽略。

Windows/macOS 新代码的原生运行、真实 Alt/Option、IME、混合 DPI、OpenGL/Direct2D
以及移动端尚未在本轮验收，编译或上游已记录的测试不能代替它们。
继续暂缓发布；待兼容 GOUI 正式版本可获取后更新依赖、重新关闭工作区检查并完成真机门槛。
本轮没有推送、标签或 Release。
