# FindBar 内容测量接入

2026-10-19；SourceView 基线 c8cf6c9，GOUI 内容测量修正 f60e393、输入外观修正 b58948f。
设计见 [FindBar](DesignFindBar.md#浮层布局与配色)。

## 变更

Find/Replace 保留 TextView + ScrollView，字段显式使用 ScrollVertical、MainWeight(0)
和最大高度 200 DIP。布局先扣除按钮/计数/选项，再按真实字段宽度 Measure；
两行各自增长，不再读取 ContentSize 或按逻辑行数估算。父约束更小时进一步限制。
FindBar 继续拥有外框，GOUI 负责文字、Placeholder、光标与命中排版。

## 验证

Linux amd64 / Go 1.25.8 / Pango / TigerVNC 1.12.0 / Xfwm4 4.16.1，临时 :145。
工作区位于仓库外，不修改或提交仓库的 go.work。

通过：

- 本地工作区 `go test ./...`、`go test -race ./...`。
- `GOWORK=off go test ./...`，以正式 GOUI v0.2.0 验证依赖仍可解析及编译。
- `tests/window/editor/find_appearance`：Software 1x/2x，OpenGL 1x + 18 pt。
  保留原有外观/命中/焦点断言，新增多行查询不拉高替换、上限滚动、删除后
  高度/范围/偏移归零断言。扩展状态的尺寸使用两行实际高度与既有边框/间距独立计算。
- 既有 `tests/window/editor/find`、`tests/window/editor/search`，Software 1x。
- 工作区 Windows amd64、macOS arm64 `go build ./...`。

窗口按正常事件路径输入，未修改剪贴板或系统设置。窗口/截图验证不替代 Windows、
macOS 原生运行和物理 IME 验收；这些本轮未执行。

## 依赖条件

go.mod 仍声明可获取的 GOUI v0.2.0，未添加 replace 或不可获取的伪版本。
v0.2.0 缺少本轮内容驱动 Measure 和统一基线行为，完整窗口体验依赖 GOUI
f60e393 / b58948f。关闭工作区的 Go 测试通过仅说明现有公开 API 可解析/编译，不代表旧版本已
具有该窗口行为。待包含本轮修正的 GOUI 版本发布后，升级正式依赖并重复独立窗口验收。
