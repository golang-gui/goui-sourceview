# TextInput Placeholder 接入验证（2026-10-19）

## 实现与版本条件

- GOUI `1ada17e`：TextInput / TextView 的 Placeholder、每视图私有缓存、语义字段及 Basic / Modern 颜色。
- GOUI `c6827e9`：两种声明式控件的 Placeholder 属性，省略时恢复为空，不修改编辑状态。
- GOUI `136e775`：独立原生窗口程序和分平台验证记录。
- SourceView `d6b499c`：FindBar 改用 TextInput.SetPlaceholder，删除两个提示 Label、坐标及显隐同步；
  创建和语言切换时设置普通翻译字符串，样式通过 input 的 placeholder part 提供。
- 起点 SourceView `9a0cb68`、GOUI `903daf8`。没有新增依赖或平台接口。
- go.mod 仍为实际可获取的 GOUI v0.1.1。当前集成需要上述未发布提交及此前的 ApplyBatch 等扩展，
  两仓通过未提交的 go.work 联调。未发布本地提交不能写成可获取的模块版本。

## 本机通过

Linux amd64、Go 1.25.8、X11 / Xfwm4、Pango：

```bash
go test ./...
go test -race ./...
go vet ./...
```

- GOUI 全仓非缓存测试为 2545 通过、15 既有跳过、0 失败；关闭工作区测试也通过。
  gui / ui / basic / modern 的 race 检查通过。GOUI vet 被既有测试中的跨包未具名结构字面量阻塞，
  新增 placeholder 文件没有对应诊断；本仓 vet 通过。
- GOUI `tests/window/gui/textplaceholder` 在 Software 1x/2x、OpenGL 1x 通过。
  检查实际提示像素、内容区裁剪、测量/基线和滚动范围不变、真实值/历史、焦点、共享视图、
  换行、字体/主题、只读/禁用和重新挂载。另经桌面键盘输入、全选删除、共享多行编辑与撤销观察显隐。
- SourceView `tests/window/editor/find_appearance` 在 Software 1x、OpenGL 1x 通过；
  `tests/window/editor/find` 在 Software 2x 通过；`tests/window/editor/localization` 在 Software 1x 通过。
  八语言用例直接断言输入框 Placeholder 及语义字段，继续保留文字、选区、历史、计数、菜单及重挂载检查。
  find 的计数断言补充步骤、实际值和查询诊断，没有修改条件或放宽等待。

## 远端结果与既有失败

远端使用任务临时目录中的已跟踪源码与本次修改，未覆盖远端仓库或带入本机无关未提交文件。

- macOS 15.7.7 arm64 / Go 1.24.9：SourceView 全仓测试通过；GOUI gui / ui / 两主题及 CoreText 包通过。
  新增占位提示窗口在 Software 1x、OpenGL 2x 通过。
- Windows 10 19045 amd64 / Go 1.24.0：SourceView 全仓测试通过；GOUI gui / ui / 两主题及 DirectWrite 包通过。
  新增窗口由 RDP 桌面交互会话启动，Direct2D 1x 完成全部自动断言并生成截图，实际 Painter 日志及三套样式已观察。
  SSH 会话启动时没有完成初始布局，15 秒超时，不计为通过；这与交互会话结果分别记录。
- GOUI macOS 全仓的 TestWindowInteractionCapabilities 失败：resize 返回 platform information unavailable，
  断言期待 unsupported platform。在原 GOUI 903daf8 单独复现同一失败。
- GOUI Windows 全仓失败包括 Direct2D TestPremultipliedNativeColors 的渐变/描边渐变颜色差异、
  Win32 TestWindowStyle 的 Integrated 样式差异，以及 SSH 会话中多个平台窗口用例超时。
  前两项在原 GOUI 903daf8 复现；窗口超时按会话验证缺口记录，没有删除断言或扩大 skip。
- macOS 的原 find / find_appearance / localization 窗口程序分别在完成计数、展开高度、菜单语言切换失败。
  使用修改前两仓完整版本重建后均复现：计数仍未完成、expanded compact bounds、zh-CN menu=[]。
  八语言查找输入和 Placeholder 检查在失败前均通过。未将这些完整场景记为通过，也未纳入本次通用提示改动修复。

## 独立构建、限制与副作用

```bash
GOWORK=off go test ./...
```

SourceView 仍失败于 GOUI v0.1.1 缺少 textedit.Model.ApplyBatch / BatchOptions；与起点相同。
即使该前置补齐，FindBar 还需要本轮 GOUI Placeholder 提交。发布兼容版本前不能宣称独立依赖验证通过。

没有验证真实中文 IME、混合显示器 DPI 或所有平台全部绘图后端；IME 显隐、取消/提交、
设置提示不打断预编辑已通过正常输入入口的包内回归。窗口启动或原生度量通过不代替这些体验验收。
新增测试创建临时窗口与 PNG，桌面输入只操作本次窗口，不使用剪贴板或修改系统设置。
GOUI 全仓沿用既有桌面测试：Windows/macOS 的 TestClipboard 会覆盖真实剪贴板，
本次未保存原内容；既有窗口与图片写入副作用保持不变。
产物保留在任务临时目录，未提交截图、可执行文件或机器专属模块配置。
