# 编辑体验切片验收（2026-10-16）

本轮实现按功能提交，不把词法补全写成完整语言服务。

| 归属 | 提交 | 内容 |
| --- | --- | --- |
| SourceView | e097415 | 双击、三击、按词拖选与词尾命中 |
| SourceView | 159934a | 编辑菜单与权限/历史 |
| SourceView | 0507956 | 配对、包裹、成对删除、换行缩进与匹配 |
| SourceView | 04f7eba | 语言/文档词补全与会话失效 |
| SourceView | d5760fb | 声明式选项与提供者代理 |
| GOUI | 227220a | DirectWrite 完整前景覆盖回收旧记录 |
| GOUI | ffc59f3 | 通用菜单的单层键盘导航 |

GOUI 基础扩展系列为 `2f78289`；本轮最终 GOUI 文档 HEAD 为 `e345c5c`。
SourceView 初始模块、语言/配色、分析/编辑核心、控件、ui、示例及设计也已分别建立基线提交。
两仓均未推送或发布新版本；GOUI 原有 Linux arm64 的 Plan/Roadmap 改动和其他未跟踪文件保留。

## 通过

- 本机 Ubuntu 22.04.5 amd64、Go 1.25.8：`go test ./... -count=1`、
  `go test -race ./...`、`go vet ./...`、`git diff --check`。
- 同机 Pango 1.50.6、独立 X11/Xtigervnc + Xfwm4：Software 1x/2x、OpenGL 1x 分别执行
  `go run ./tests/window/experience` 和 `go run ./tests/window/editor -auto`。
  后者用实际图像资源记录 painter 与 PixelScale；两个程序均使用正常事件路径。
- 三种组合覆盖双击松开、三击整行、局部选择前景、右键保留、菜单删除/撤销、
  End/Enter 菜单选择/执行、JSON/HTML 补全确认/取消/重入/卸载，以及配对和原子历史。
  XML 命名空间/文档词、Unicode 与上下文边界在逻辑测试覆盖。
- 旧基线 `849ffbc` 加同一选择窗口回归，在首个“双击松开后保留完整词”断言失败；
  修正后相同场景通过。局部前景与同后端独立排版逐像素相等，详情见 [Testing](Testing.md#8-编辑体验窗口)。
- GOUI：源码树外快照在独立 X11 桌面运行 `go test ./... -count=1` 通过。
  菜单单元测试及正常窗口键盘事件验证通过，详见其 `doc/PlanMenuKeyboard.md`。
- Windows 10 19045 amd64 / Go 1.24.0 / SSH Session 0：SourceView
  `go test ./... -count=1` 通过，实际运行 DirectWrite 测试；GOUI 菜单相关测试与
  DirectWrite 包测试通过。
- macOS 15.7.7 arm64 / Go 1.24.9：SourceView `go test ./... -count=1` 通过，
  实际运行 CoreText 测试；GOUI 菜单相关测试通过。
- 修复、菜单、括号、补全与声明式的中间提交树分别在临时检出验证全包测试，
  最终保留的提交边界可编译、可测试。

远端使用任务临时目录快照；这些测试没有创建 SourceView 原生窗口，桌面缩放不适用。
Linux 交互也使用独立显示和窗口管理器，没有改日常桌面的焦点、剪贴板或设置。
GOUI 既有测试的图片输出留在临时快照，不向源码目录回写产物。

## 失败与未完成

- `GOWORK=off go test ./...` 仍失败：可获取的 GOUI v0.1.1 缺少
  `textedit.Model.ApplyBatch`、`textedit.BatchOptions` 等已在开发分支实现的接口。
  工作区测试不能替代独立发布，未修改 go.mod 为不可获取的版本。
- Windows/macOS 的新菜单、补全和配对桌面交互尚未执行；编译或逻辑测试不计作原生表面验收。
- 原生物理快捷键、真实 IME 的候选/组合、多屏缩放和长时间资源压力仍待验收。
- 文档词候选为局部有界扫描；没有 Schema/LSP、异步提供者、代码片段或 XML/HTML 元素自动闭合。
  括号查询预算、未知状态降级和标记生命周期见 [括号设计](DesignPairs.md)，
  补全 API/资源边界见 [补全设计](DesignCompletion.md)。

此前记录的 GUI 平台完整套件既有失败与验收缺口仍按 [Implementation](Implementation.md) 保留，
本轮的相关包通过不表示 Windows/macOS GOUI 全仓已全部通过。
