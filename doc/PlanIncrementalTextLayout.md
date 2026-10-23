# 长段落排版优化记录

## 分段实验的原生坐标修正（2026-10-23）

SourceView v0.1.0 准备阶段，正式依赖 GOUI v0.3.0。Mac 的六个子项失败集中
在两个测试内原型：`TestBoundedFragmentProbeMatchesSimpleParagraphs` 的 word/json，
以及 `TestExplicitContextFragmentProbe` 的 longRLO/longLRO/longRLI/unicode。
连同两个父测试共计八项失败；正常控件未启用这两种有界输入实验。

原型把每片 `Fragment.Size().Height` 累加为下一片原点，混淆了字形/行占用范围
与下一行的推进距离。CoreText 可有行间空白和首行顶部偏移，例如字形高 12、
行距 15、首行顶部 2.75，此时片段末行底部并不能确定下一行基线。原生失败
首处纵向偏差为 0.24024、2.02899 或 2.3125 DIP，后续还可能累积。

修正只在实验辅助代码保存片段放置坐标：每步保留一条完整的重叠行，排除可能
被输入边界截断的末行；下一步用重叠行实测基线对齐首行，再发布之前完成的行。
下一步仍重新测量同一完整行，原有行/簇范围、尺寸、方向、坐标和基线严格比较
及 512/2048 字节输入上限均保留，没有增加误差、读取完整参考结果来定位或改变
Typography / Fragment 的公共契约。

新增不依赖字体的固定度量回归，独立要求每四字节一行，Y 为 `15*i+2.75`、
基线为 `15*i+12`，同时检查簇与全部源字节覆盖。Linux 定向及全包测试通过；
Mac 同源原生根包第一轮 267 项全通过，包含原六个失败子项。新增固定度量回归
及后续完整发布验证统一见 Plan。

Windows 第一轮仍在 longRLI 的数字簇横坐标失败（X=0，完整段落 X=174.78906），
需继续核对重放的 isolate 内上下文。Mac 的纵向修正不作为通用 Unicode 分段
算法已经成立的证明；原来的隐式双向上下文反例仍必须验证不等价。

## 范围与状态

2026-10-20，保持完整正确性。本轮撤回 Reflow，改为上层片段缓存与可视区布局
设计；GOUI 提供通用值设施，本仓只接入公共契约，不复制 TextView 私有 editor。

- [x] 核对 Scintilla／Monaco、三个原生后端的相关能力，记录独立窗口原型失败。
- [x] GOUI 的几何索引和可视行选区查询；本仓选择／搜索／出现词／预编辑绘制接入。
- [x] 长段落全选仅产生可视行矩形的包内回归。
- [x] Git 回退 Reflow 接入，修订两仓片段测量及布局设计。
- [x] 普通值 Fragment 与按需详细几何分页接入。
- [x] 有限送测实验，长词／JSON 的完整行及 cluster 对照。
- [x] 最近有效宽度的完整布局复用、资源界限及正常窗口输入验证。
- [x] 显式 bidi 上下文携带实验与隐式上下文反例。
- [x] 完整逻辑段落的视口分帧准备、工作集保留和真实多格式文档验证，见
  [语料验证](PlanCorpusLayout.md)；不等于下面的通用段落内片段方案已经完成。
- [ ] 正式片段测量的上下文衔接、范围缓存及可视区布局。
- [ ] 段落内片段的事件循环分步推进、可视区发布、增量布局、取消与生命周期。
- [ ] 单条极长段落未完成排版期间仍可拖滚动条、操作其他控件的真实窗口验收。

**当前仍会在首次原生长段落排版时阻塞。** 不能以查询优化宣布整个需求完成。
带 bidi 缺陷的独立窗口原型没有接入正式控件，未使用后台测量。
方案与片段准确性门槛见 [布局设计](DesignLayout.md)，后端诊断和同环境查询对照见
GOUI [实施记录](../../goui/doc/PlanIncrementalTextLayout.md)。

## 依赖

本轮使用相邻 GOUI `554dbdb` 之后的改动，需要尚未发布的 `21444f9` 中的
`gui/textedit.Geometry.LineAtY` 和 `SelectionLines`；GUI 使用方在 `25e52c1`，
诊断在 `33113b9`，设计与记录在 `b022304`。本仓代码提交为 `6224e5e`。
现有 go.mod 的本地 replace
为用户原有改动，本轮保留；没有修改全局 Go 环境或提交机器路径。

使用本地 go.work 联调。`GOWORK=off` 仍会读取现有 replace，因此该命令成功
也不能算独立依赖验证；需另外使用仓库外临时 modfile 去除 replace 检查实际
v0.2.0 依赖。已有 Rasterize 接口变化也尚未由该版本提供。

## 验证

Linux amd64／Go 1.25.8／Pango 1.50.6，通过：

- `go test ./...`、`go test -race ./...`、`go vet ./...`。
- GOUI `go test ./...` 及 `go test -race ./gui/textedit ./gui`。
- Windows amd64、macOS arm64 的 `CGO_ENABLED=0 go build ./...`。
- `XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/selection`：
  X11／XFCE，Basic／Modern Light／Modern Dark 的 JSON／XML／HTML 九组通过，
  包括正常事件分发、语法前景、选区背景像素、焦点与菜单。窗口自动关闭，
  无剪贴板或系统设置修改。

失败单独记录：

- `GOWORK=off go test ./...`：`go: updates to go.mod needed`，没有执行 tidy 改动用户文件。
- 仓库外临时 modfile 删除 replace 后，`GOWORK=off go test -modfile ... ./...`
  在新增 `LineAtY`／`SelectionLines` 编译失败；不能把工作区通过当成独立发布通过。
- GOUI `go vet ./gui/textedit ./gui` 有历史未命名字段诊断；与 HEAD 对照相同，
  详见其实施记录。

首次长段落排版仍同步，窗口响应目标未通过验收。Windows／macOS 原生运行、
真实 IME／物理键鼠和混合 DPI 未执行。上述选区窗口用例不能替代长段落性能验证。

## 点击定位回归联验（2026-10-20）

GOUI `71b25c8` 修复 TextView／TextInput 的光标移动无条件请求尺寸布局：
有效段落直接复用几何，通过既有滚动机制显示目标，缺失／失效时仍请求布局。
本仓源码不需要适配新接口；保留现有 go.mod 和用户的 example/temp。
本次 SourceView 代码基线为 `9827920`。

- 两仓 `go test ./...`，GOUI `go test -race ./gui ./gui/textedit`、
  SourceView `go test -race ./...` 通过；新增 GOUI 回归在修复前代码明确失败。
- GOUI 原生窗口 `tests/window/gui/text_caret` 在 Linux amd64／X11／XFCE／
  Pango 1.50.6／Software、Modern 1×／2× 下通过点击、导航和排版复用断言。
- 本仓 `tests/window/editor/selection` 的 JSON／XML／HTML × Basic／Modern
  Light／Modern Dark 九组正常事件与像素检查通过（Software 1×）。
- 两仓受影响包及本仓全模块的 Windows amd64／macOS arm64 无 CGO 交叉构建
  通过；不代表这两端原生运行验收。
- `GOWORK=off go test ./...` 仍报 `updates to go.mod needed`；没有 tidy、
  更改用户 replace 或把本地工作区通过写成独立发布通过。

用户 157,862 字节单行 JSON 的本机只读诊断中，TextView 与 SourceView
（JSON 高亮）的三次点击均无新增完整度量。TextView 的 32 KiB 单段回归从
每次点击两次整段排版，改为不新增排版调用。采样边界、耗时和限制详见
GOUI [光标回归记录](../../goui/doc/PlanIncrementalTextLayout.md#光标定位回归修复2026-10-20)。

首次整段排版仍同步；SourceView 着色更新后观察到一次约 293 ms 的原生
MeasureSize，尚需确认并修正颜色失效路径。2× 跳转绘制也仍有延迟。
本次修复不改变上面“通用分时排版未完成”的状态。窗口已关闭，
未写剪贴板、系统设置或用户 JSON。

## Reflow 回退与片段设计（2026-10-20）

通过 `git revert` 生成 `da26b9a`、`72e240d`，撤回 `104c300` 的实验验收记录
及 `cdf100c` 的控件／窗口测试接入。没有 reset、改写历史或改动用户 go.mod、
example/temp。GOUI 对应回退到 `dab0170`，仅额外保留独立 RTL 起点修复。

TextLayout 不参与视口、缓存、帧预算或任务管理。本仓未来保存范围＋度量副本，
复用 GOUI 纯数据片段运算，自行负责语法和源码控件呈现。
具体设计见 [布局第 11 节](DesignLayout.md#11-片段缓存接入规格实施中)；本轮没有
实现该设计，也不将独立截取片段的复杂文本准确性写成已解决。

Linux amd64，本地工作区 SourceView `72e240d`／GOUI `dab0170`：两仓
`go test ./...` 通过，GOUI 另在 `GOWORK=off` 下全仓通过。
GOUI `GOWORK=off go test -race ./gui ./gui/textedit ./platform/typography/pango`
通过。本仓 Windows amd64／macOS arm64 的 `CGO_ENABLED=0 go build ./...`
通过，仅为交叉构建，没有原生窗口验收。
本仓 `GOWORK=off go test ./...` 仍报 `updates to go.mod needed`；未 tidy。
已有测试可能重生成原有图片，未纳入提交。没有新增长段落窗口验收或性能结论，
同步长段落测量仍待新片段方案实现。

## 普通值片段接入与有限送测实验（2026-10-21）

基线 SourceView `24edd50`／GOUI `9c39d01`，本轮依赖新增且尚未发布的
gui/textedit.NewFragment 和 Fragment.Geometry。保留用户 go.mod／replace 和
example/temp，使用不提交的本地 go.work；没有更新到不可获取的版本。
GOUI `42c9b42` 提供值设施与分页，`22f3e4d` 接入 TextView／TextInput；
本仓 `5f9a81d` 接入 SourceView 并提供原生快照回归。

SourceView 将原生行／cluster 复制为独立片段，再使用按需几何页，避免创建
完整段落的全部详细停靠索引。每页 32 显示行，每个 Geometry 最多 4 页；
基础度量仍完整保留。不改变字体、颜色、原生绘制、文本编辑及输入路径。
首次和换宽仍同步原生测量，本轮未接入有限片段渲染或事件循环测量调度。

### 实验范围

`layout_fragment_probe_test.go` 仅为测试内实验，不在正式控件路径启用：

- 每次候选输入最多 512／2048 字节，保留未完成末行，下次从其起点继续。
- 每步正常创建／测量／导出度量／复制 Fragment／释放原生布局，片段只保留值。
- 8 KiB 长词和重复 JSON 在两种窗口大小下，与完整原生布局的行范围、尺寸、
  基线、cluster 范围、方向和坐标逐项一致，没有放宽误差。
- 独立防误用回归：前缀 RLO 距 XYZ 超过 4 KiB，完整布局为 RTL，直接截取 XYZ
  后窗口却为 LTR。保存普通值不会补出缺失上下文。没有开启仅普通代码快、
  复杂文本同步整段的正式分支。

因此确认“减少一次送测量有效”，尚未证明通用片段边界正确。不能将实验完整循环
称为 GUI 分帧；普通值缓存与几何分页不等于已经修复首次加载／拖宽冻结。

### 单步测量实测

Linux amd64／Go 1.25.8／Xeon Gold 5318Y／Pango 1.50.6／Cairo 1.16.0，
DejaVu Sans 12 pt，宽 520 DIP、WrapWordChar、32,768 个 a。字体预热及样本构造
不计时，无桌面窗口；其他项目验证完成后单独运行三次：

```bash
go test . -run '^$' -bench '^BenchmarkBoundedFragmentProbe' -benchtime=1x -count=3
```

| 路径 | 累计执行耗时（三次） | 最长单步（三次） |
| --- | --- | --- |
| 整段 | 1725.3 / 1729.8 / 1719.9 ms | 同左 |
| 512 字节候选 | 133.9 / 114.5 / 115.6 ms | 3.550 / 2.805 / 2.706 ms |
| 2048 字节候选 | 190.2 / 190.3 / 191.9 ms | 12.71 / 12.86 / 12.77 ms |

512 字节路径首步为 2.600 / 1.823 / 1.819 ms。单步包含创建、测量、导出、复制及
销毁；不包含事件循环等待和绘制，不是 UI 延迟保证。候选窗口间重测尾部，
因此累计工作量也与整段不同。通用上下文、换宽复用、增量编辑、估算高度、
帧预算和正式窗口响应仍待实现。

### 验证

两仓全仓测试、GOUI gui／textedit 和 SourceView 全仓 race、GOUI textedit vet
通过。SourceView 原生片段回归还覆盖布局销毁后的快照存活、JSON／Unicode／
RLO／isolate 的逐行几何及方向。Windows amd64／macOS arm64 无 CGO 交叉构建通过。
GOUI text_caret 在 X11／XFCE、Software、Modern 集成窗口 1×／2× 通过；
SourceView selection 的 JSON／XML／HTML × 三主题九组事件及像素通过。
窗口均自动关闭，没有剪贴板、系统设置或用户文件写入。未执行原生 Win／mac、
IME／物理输入／混合 DPI 验收。

失败／缺口保留：

- SourceView `GOWORK=off go test ./...` 仍报 updates to go.mod needed，未 tidy。
- `go vet ./...` 在未修改的用户 example/temp/main.go:58 报 unreachable code；
  对正式包、internal、ui、正式示例及 tests/window 的定向 vet 通过，未修改该例程。
- GOUI 首次全仓 cmd/svgpng 曾段错误；之后三次定向及两次全仓通过，根因未确定，
  未归为已证实的既有失败。详见 GOUI 同专题记录。
- 本轮没有对用户 JSON 文件、MB／GB 输入或完整可视区测量路径作性能验收。

后端保持原样；新代码仅是上层值设施、详细几何工作集、控件接入及有界送测实验。

## 最近宽度缓存与上下文实验（2026-10-21）

基线 SourceView `cf3c544`／GOUI `a618df5`。本轮只修改 SourceView，没有修改 GOUI、
typography 接口、用户 go.mod／本地 replace 或 example/temp。继续使用本地 go.work。
控件缓存与窗口回归在 `8f09aa6`，上下文实验及其设计边界在 `e0a56f1`，分别提交。

### 已实现的控件优化

段落以实际正文宽度区分完整原生布局、Fragment 和 Geometry，恢复缓存中的宽度时
直接复用，而非调用 SetSize 重排。每个文本缓存身份最多两个宽度，所有身份／宽度
共同遵守 128 项与输入字节总和 8 MiB 的原有界限；当前访问的单条大段落仍可超限。
冷启动的 revision/row 项转为稳定 LineID 时重新执行该身份的宽度淘汰，未转接的
临时项也计入全视图界限。该缓存不计作原生内存测量。

高度变化不重新测量；字体、Tab、Wrap 模式、文档重置及生命周期仍释放旧资源。
局部编辑重映射后缀行号，两个宽度继续复用未改 LineID。颜色变化按需重着色；
Snapshot 只查询当前宽度，不从另一个宽度读取坐标，也不为快照补排文本。
淘汰资源回调切换文档时终止旧操作，不能返回已销毁的段落。

新增逻辑回归覆盖宽度命中、连续缩放界限、失效、颜色、快照、冷索引、编辑后缀和
销毁重入。原生回归覆盖 ScrollView、复杂文字完整行／cluster／caret 和恢复前后像素。
相同 `TestNativeRecentWidthsWithScrollView` 在仓库外独立检出的旧提交 cf3c544 上
明确失败：恢复宽度新增一次布局、MeasureSize、MeasureMetrics 和颜色调用。
修改后通过，没有删除旧断言或改变 Unicode 容差。

### 同环境对照

Linux amd64／Go 1.25.8／Xeon Gold 5318Y／Pango 1.50.6／Cairo 1.16.0，
SourceView 默认 monospace 12 pt，8,192 个 a，WrapWordChar，无侧栏；视口宽度在
520／480 DIP 间切换（正文 508／468 DIP）。两种宽度的初次测量和字体预热不计时；
包括 LayoutVisible、锚点／高度更新，不包括绘制、窗口、事件循环等待。
旧代码和新代码运行相同 benchmark，各三次，每次六次切换，分别执行以避免竞争：

```bash
go test . -run '^$' -bench '^BenchmarkNativeRecentWidths$' -benchtime=6x -count=3
```

| 路径 | 每次切换（三次） | 新建原生布局／导出度量 |
| --- | --- | --- |
| 旧提交 cf3c544 | 95.520 / 94.988 / 95.165 ms | 各 1 次 |
| 最近宽度缓存 | 1.488 / 1.594 / 1.456 µs | 均 0 次 |

旧路径每次 Go 分配约 2.38 MB，新路径 128 B；不包括原生分配器，不能当作进程内存。
结果仅说明恢复最近宽度的缓存命中，不能推广为新宽度、首次首屏或端到端 UI 延迟。

### 上下文实验与仍未解决的目标

有限候选携带段落方向及活动 LRE／RLE／LRO／RLO／LRI／RLI，注入前缀也计入
送测字节上限，移除前缀时保留真实控制符并拒绝跨 cluster 的不安全映射。
Pango 1.50.6 下，长 RLO／LRO／RLI、混合 Unicode 在 512／2048 字节输入中与
整段原生行和 cluster 逐项一致。原有独立截取丢失 RLO 的回归保持不变。

新增反例说明活动控制符不足：跨窗口长中性序列、数字串和括号的坐标／方向仍不同。
FSI 和深度溢出不在实验支持范围；基础方向扫描也未分步。相关实现只在 *_test.go，
没有把它接入正式控件，没有用复杂文本同步整段路径冒充通用有界方案。

**新宽度和首次原生测量仍同步，会阻塞 UI；通用片段测量和分时发布未完成。**
后续须在上层解决隐式 bidi、括号及 shaping 上下文，再推进按帧有界工作。
本轮没有改变 TextView／TextInput，也没有恢复 Reflow 或增加后台测量。

### 验证

Linux amd64／Go 1.25.8／Pango 1.50.6，通过：

- `go test ./...`、`go test -race ./...`。
- `go vet . ./internal/... ./language ./scheme ./ui ./example/editor ./example/declarative ./example/markdown/... ./tests/window/...`。
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...` 与
  `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`，仅为交叉构建。
- `XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/resize_cache`
  及 scale=2：X11／XFCE／xfwm4，Modern 集成窗口，通过正常 PointerDown/Up
  按钮切换编辑区宽度、恢复及点击光标，窗口和所持布局均自动释放。
  两端缩放的编辑区宽度均为 632→512→632 DIP；原生布局／度量导出计数为
  2→3→3，恢复和随后 byte 3 点击没有新增工作。首个多余布局是初始滚动条收敛，
  已按界限淘汰。Painter 正常的每帧 MeasureSize 查询继续单独记录，不能把它们
  与几何失效／重建混为一谈；包内 LayoutVisible 缓存命中另断言尺寸查询也不增加。
- `XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/selection`：
  JSON／XML／HTML × Basic／Modern Light／Modern Dark 九组通过，包括选区、颜色、
  焦点、菜单和像素。全部窗口自动关闭，没有文件、剪贴板或系统设置修改。

失败／未执行分别保留：

- `GOWORK=off go test ./...` 仍报 updates to go.mod needed，未运行 tidy，
  没有用本地工作区通过替代发行依赖验证。
- `go vet ./...` 仍在未修改的用户 example/temp/main.go:58:2 报 unreachable code；
  上述正式包定向 vet 通过，没有改动或删除该例程。
- 原生 Windows／macOS、物理输入／IME、混合 DPI、本轮 MB／GB 输入及用户 JSON
  场景未执行。宽度命中窗口用例不作为“未完成排版仍可操作窗口”的验收通过。

本轮没有改动 GOUI，因此没有新增 GOUI 提交或把此前的 GOUI 验证写成本轮重新执行。
