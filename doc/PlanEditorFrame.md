# 编辑器统一外框验收（2026-10-22）

设计见 [样式接入](DesignStyles.md#滚动编辑区域外框)。SourceView 基线为
`226dcdc`；GOUI 基线为 `006d4e2`，需要新增的 `2a0dfef`（ScrollView frame）
及 `1d3aebd`（TextView 主题与使用者迁移）。本轮未改动 platform / typography，
未增加公共接口、自动继承样式或额外容器。

## 范围

- Basic / Modern 的 `source-view` 显式提供 frame / trough / thumb；正文保持
  独立语法配色，侧栏 border part 不改作外框。
- 同步命令式、声明式示例、窗口用例与技能接入说明，宿主明确指定
  `source-view` 样式。FindBar 和补全内部 ScrollView 保留原有独立外壳。
- 新增 `tests/window/editor/frame`，在一个 Integrated 窗口中同时验证
  GOUI TextView 与 SourceView。用户未跟踪的 `example/temp/main.go` 仅本地
  增加两个宿主样式调用，不纳入提交；保留用户已有 go.mod / replace。

## 通过

环境为 Ubuntu 22.04.5 / amd64 / Go 1.25.8 / Pango，独立 Xvnc `:84`、
Xfwm4 合成开启，未使用 DevServer。使用已跟踪源码和本次新增文件的双仓
临时快照及不提交的 go.work，执行 `go test -json ./... -count=1`：415 项
通过、零失败。`go vet ./...` 通过。GOUI 全量为 2405 项通过、10 项既有跳过；
针对 ScrollView / frame 的 race 通过。

- `frame`：Software 的 Basic / Modern Light / Dark × 1/1.5/2 倍九组通过，
  OpenGL 1.5 倍通过（Mesa llvmpipe）。断言点击聚焦、焦点转移时正文尺寸
  稳定、滚轮、双轴拖动、禁用、换行隐藏横条、短文本隐藏双条以及主题失效。
- 另以 XTest 的真实鼠标输入、滚轮、双轴拖动和原生窗口调整，检查 150%
  下的边框、圆角与滚动条；1350×780 物理分配稳定后两编辑器均保留内框。
- `find_appearance`：Software 1.5 倍下查找、替换、输入框增高、顶端按钮、
  两种主题隔离与滚动通过。`completion -case scroll -auto`：Software
  1 倍和 2 倍的键盘、滚轮、拖动、点击补全与撤销通过。
- GOUI `text_sizing`：Software 1/1.5/2 倍通过；TextInput 与裸 TextView
  正文逐像素比较，外框各自按职责处理。`text_caret` 在 1.5 倍通过。
- `CGO_ENABLED=0` 的 Windows amd64、macOS arm64 示例及新增窗口程序构建通过，
  只代表交叉构建。GOUI 对应 gui/ui/theme/Demo 目标也通过。

## 失败、限制与副作用

- `GOWORK=off go test ./...` 在干净快照依赖 GOUI v0.2.0 时，仍因既有
  `textedit.Fragment`、`NewFragment`、`Geometry.LineAtY / SelectionLines`
  等未发布能力而构建失败。本地工作区不能代替独立发布验证；新 frame 同样
  需要上述 GOUI 提交。实际用户工作树的本地 replace 另提示需要更新 go.mod，
  未擅自执行 tidy 或提交机器路径。
- `completion -case scroll -auto` 在 Software 1.5 倍报
  `panel width changed after paint`。未修改的两仓基线同环境同命令也失败，
  记录为既有分数缩放问题，不放宽断言或扩大本轮到 Popover 重构。
- Windows/macOS 原生窗口、系统缩放、真实 IME 和混合 DPI 未执行。
- 一次 GOUI 全量运行误继承 `DISPLAY=:3.0`，既有平台剪贴板用例覆盖了该
  会话 CLIPBOARD，运行前内容未知，无法恢复。随后固定 `:84` 全量重跑通过；
  最终窗口场景不改剪贴板、文件或系统设置。程序、截图、快照及日志均留在
  仓库外临时目录，隔离窗口和本次 X11 会话在验收后关闭。
