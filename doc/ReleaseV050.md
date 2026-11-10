# GOUI SourceView v0.5.0

## 中文

需要 Go 1.24 或更新版本，依赖 GOUI v0.7.0。发布 Go 库与源码示例，采用 MIT 许可证，不附预编译二进制。

### 主要变化

- 新增由宿主提供结果的单行、多行内联建议（影子文字），默认关闭。支持 Tab 整段接受、Esc 取消及单步撤销；多行预览按实际排版高度让后续正文下移，参与换行与滚动，不进入文档、保存、搜索或输入法预编辑，也不影响共享文档的其他视图。
- 扩充现有编程、脚本、配置、模板和文本语言的基础折叠。覆盖常见括号块、关键字块、注释、章节、记录和多行值；规则保持有界的词法/结构识别，不引入完整语法语义解析器。
- 新增长逻辑行的视图内折叠。自动换行达到至少三个原生显示行时可收起；统一结构与长行的 gutter 操作、单行预览和展开入口，保留真实逻辑行号、文本及历史。
- 修复持续输入时补全弹窗反复关闭重建造成的闪烁；词内输入、删除及候选刷新复用活动弹窗。
- 修复输入长行后手动拖动水平滚动条受到旧光标定位请求干扰的问题。
- 改善 SetText 全文替换、文档切换和重新挂载后的首屏准备，避免等待新布局时出现空白过渡帧。

### 升级与边界

```sh
go get github.com/golang-gui/goui-sourceview@v0.5.0
```

GOUI 依赖仍为 v0.7.0，无需本地 replace 或 go.work。

内联建议由应用负责模型、网络、请求代次和取消。当前仅支持逻辑行末追加，最多 64 KiB / 256 个逻辑行；首段追加片段不接受 Tab，后续行支持原生 Tab 缩进。不含换行的建议仍使用一行裁剪和省略策略。尚未提供行中替换、部分接受或代码片段会话。

折叠支持通用、明显的规则，不承诺所有合法语言语法的完美识别。完整语言服务、保存点、视图状态恢复、补全详情和多光标不属于本次交付。

### 验证

同一冻结提交通过 Linux amd64、Windows amd64、macOS arm64 原生包测试与构建；Linux 另通过全仓 race。窗口验收覆盖内联建议、折叠、补全、水平滚动、全文替换和键绑定。不同平台字体与快捷键的测试适配、实际后端和缩放、失败复验及未覆盖项会记录在发布验收文档。物理输入法、混合显示器 DPI 和硬件 GPU 未在本轮验收。

## English

Requires Go 1.24 or newer and GOUI v0.7.0. This MIT-licensed release distributes the Go library and source examples without prebuilt binaries.

### Changes

- Added optional host-provided single-line and multiline inline suggestions. Tab accepts the whole suggestion, Escape dismisses it, and acceptance is one undo unit. Multiline previews reserve native-measured view space and participate in wrapping and scrolling without entering the model, saved output, search, IME preedit, or other views of the shared document.
- Expanded basic folding across existing code, script, configuration, template and text languages. Rules recognize common bracket/keyword blocks, comments, sections, records and multiline values through bounded lexical/structural processing, without a full semantic parser.
- Added view-local collapse for long logical lines that occupy at least three measured display rows. Unified structural and long-line gutter controls, one-row previews and expansion affordances while preserving source, history and logical line numbers.
- Kept completion popovers alive during typing, deletion and candidate refresh within the current word, avoiding repeated close/reopen flicker.
- Fixed stale caret-reveal requests interfering with manual horizontal thumb dragging after typing long lines.
- Prepared replacement viewports before their first paint after SetText, document changes and reattachment, avoiding blank layout-transition frames.

### Upgrade and scope

```sh
go get github.com/golang-gui/goui-sourceview@v0.5.0
```

The GOUI dependency remains v0.7.0. No local replace directive or workspace is required.

Applications own AI models, networking, request generations and cancellation. Inline suggestions currently append only at logical line ends, with a 64 KiB / 256-line limit. Tabs are rejected in the initial appended fragment; subsequent lines support native tab indentation. Suggestions without newlines retain one-row clipping and ellipsis. Mid-line replacement, partial acceptance and snippet sessions are not included.

Folding recognizes common, obvious structures rather than every valid language construct. Full language services, save points, view-state restoration, richer completion details and multiple cursors are outside this release.

### Validation

The same frozen commit passed native package tests and builds on Linux amd64, Windows amd64 and macOS arm64, with an additional full race run on Linux. Native-window scenarios cover inline suggestions, folding, completion, horizontal scrolling, full-text replacement and key bindings. Platform-specific probe adaptations, backends/scales, retries and limitations are recorded separately. Physical IME, mixed-monitor DPI and hardware GPUs were not verified in this release.
