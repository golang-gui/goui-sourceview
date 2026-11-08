# GOUI SourceView v0.4.0

## 中文

本版本需要 Go 1.24 或更新版本，依赖 GOUI v0.7.0。
发布 Go 库和源码示例，采用 MIT 许可证。

### 主要变化

- 新增文档行标记、断点/书签图标和执行行呈现，支持点击、上下文请求、
  折叠汇总及编辑后位置跟踪；debug 示例展示应用接入方式。
- 新增色块 Minimap，支持当前视口、点击、拖动和滚轮导航。
- 外部分析可独立发布修订匹配的 Highlights 和 Folds 快照。
  `RefreshCompletion` 可重新查询当前补全会话，颜色更新不再要求替换语言或重开补全。
  semantic 示例使用 Go parser/types 展示着色、成员补全和 AST 折叠。
- 扩展 JSONC/JSON5、YAML、TOML、Go、JavaScript/TypeScript、CSS、Python 的折叠规则。
  外部折叠候选由文档共享，各视图独立维护折叠意图。
- 新增子词导航/扩选/删除、结构扩大/缩小选区及选区轨迹恢复。
- 新增语言缩进规则、可定制重新缩进、按缩进粘贴、合并行、Unicode 大小写转换和
  尾部空白清理；沿用一次操作一次撤销、过期取消及编辑预算。
- 菜单接入 GOUI Command/Shortcut，快捷键提示独立成列并右对齐，
  随当前绑定更新；重绑和解绑保留原有编辑输入语义。
- 适配 GOUI v0.7.0 的完整四角圆角、阴影和可选择边框。

### 升级与边界

```sh
go get github.com/golang-gui/goui-sourceview@v0.4.0
```

GOUI 从 v0.6.0 升至 v0.7.0。应用直接使用的 `Style.Radius()` 应改为
`Style.CornerRadius()`；`Painter.DrawBoxShadow` 的圆角参数改为
`geometry.CornerRadius`。等圆角使用 `geometry.UniformCornerRadius(r)`，
`Rule.Radius(float32)` 仍可使用。详见
[GOUI v0.7.0 迁移说明](https://github.com/golang-gui/goui/releases/tag/v0.7.0)。
本模块不需要本地 replace 或 go.work。

Minimap 使用有界色块表示，不绘制缩小字形。行标记只提供呈现和交互请求，不实现调试器。
外部分析示例不执行源码，也不等同完整语言服务。代码片段、保存点、视图状态恢复和
补全详情提案不属于本次交付。大文件操作仍有预算及线性步骤，未承诺硬实时延迟。
示例以源码分发，不提供预编译附件。

## English

This release requires Go 1.24 or newer and GOUI v0.7.0. It distributes the Go
library and source examples under the MIT license.

### Changes

- Added document line marks, breakpoint/bookmark glyphs and execution-line
  presentation, with pointer requests, fold summaries and edit tracking.
  The debug example demonstrates application integration.
- Added a bounded color-block minimap with viewport indication, clicking,
  dragging and wheel navigation.
- External analysis can publish revision-matched Highlights and Folds snapshots
  independently. `RefreshCompletion` re-queries an existing completion session;
  publishing colors does not require replacing the language or reopening completion.
  The semantic example uses Go parser/types for colors, member completion and AST folds.
- Expanded folding for JSONC/JSON5, YAML, TOML, Go, JavaScript/TypeScript, CSS and
  Python. Documents share external candidates while each view retains its own fold intent.
- Added word-part movement, selection and deletion, structural selection expansion
  and shrinking, and selection-trail restoration.
- Added language indentation rules, customizable reindentation, reindented paste,
  line joining, Unicode case conversion and trailing-whitespace cleanup, preserving
  single-operation undo, stale-result cancellation and edit budgets.
- Menus use GOUI Command/Shortcut associations for a separate, right-aligned
  shortcut column that follows current bindings without changing editor input routing.
- Updated rendering for GOUI v0.7.0 independent corner radii, shadows and selected borders.

### Upgrade and scope

```sh
go get github.com/golang-gui/goui-sourceview@v0.4.0
```

The GOUI dependency moves from v0.6.0 to v0.7.0. Applications using GOUI directly
must replace `Style.Radius()` with `Style.CornerRadius()` and pass
`geometry.CornerRadius` to `Painter.DrawBoxShadow`. Use
`geometry.UniformCornerRadius(r)` for equal corners; `Rule.Radius(float32)` remains
available. See the [GOUI v0.7.0 migration notes](https://github.com/golang-gui/goui/releases/tag/v0.7.0).
No local replace directive or workspace is required.

The minimap draws bounded color blocks rather than reduced glyphs. Line marks
provide presentation and interaction requests, not a debugger. The analysis
example does not execute source or provide a complete language service. Snippets,
save points, view-state restoration and proposed completion details are outside
this release. Large-file operations retain budgets and linear steps; hard real-time
latency is not promised. Examples are distributed as source only.
