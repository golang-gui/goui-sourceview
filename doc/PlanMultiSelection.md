# 多光标与列编辑实施记录

分支 feature/multiselection，基线 2d27cb4，依赖已发布 GOUI v0.7.0，无跨仓修改。
设计见 [多光标与列编辑](DesignMultiSelection.md)。

## 第一批：多选区闭环

已完成状态集合、完整前后历史与亲和性、一次事务/修订/集合通知、基本输入/导航、
剪贴板分配、主光标预编辑与全部提交、可见选区/光标绘制、状态保存及薄声明式绑定。
程序集合严格校验且上限 4096；旧单选区 setter/声明明确移除附加选区。
模型使用批量端点映射和结果区间索引，分散光标逐批生成普通值计划，不要求全部页同时驻留。
生成文本与复制总量按 64 MiB 预算控制；纯删除并集，冲突替换原子失败。

通过：go test ./...、GOWORK=off go test ./...、针对 internal/text/sourceview/ui 的 race，
完整 SourceView 测试重复三轮。逻辑新增覆盖独立随机端点映射、合并、方向、事务、重入、
共享文档、粘贴末尾 LF、IME、32 个跨页位置、取消和原生布局失败只报告一次。
Ubuntu 22.04.5 amd64、Go 1.25.8；独立 TigerVNC / Xfwm4 compositor on、Pango / Software：
多选区窗口 Basic/Modern Light/Dark 各 100%/150% 六组通过，含中文、复制剪切分配粘贴、
撤销重做、SaveState/RestoreState 和 24 个分散长段落光标。旧 editor/binding/selection/
view_state 在 100%/150% 八组通过。真实 IME 服务和其他平台原生窗口未执行。

一次独立测试进程在 0.003 秒以 SIGSEGV 退出，没有用例输出；不能确认根因或归为既有问题。
随后独立 verbose 全量、完整模块、race 和三轮 SourceView 测试均通过，保留原失败记录。
首轮截图工具在测试窗口退出时发生 BadDrawable；用例自身全部通过，调整外部截图采集后
完整矩阵重跑通过，没有改变功能断言。日志在 /tmp/sourceview-multiple-3_fs021a/，
窗口旧功能回归在 /tmp/sourceview-phase1-native-vz0az1bx/；失败截图采集日志保留在
/tmp/sourceview-multiple-7lsyyny9/。全部专用进程退出，剪贴板仅覆盖独立显示，不写回样本。

首批期间未适配的编辑命令在多选区时禁用或明确返回 ErrMultipleSelections；已在第三批接入。
补全与影子建议保留单点边界，不篡改用户配置。

## 第二批：列编辑

已完成可配置添加/矩形指针手势、键盘列扩展、上下添加光标、下一个/全部字面匹配和按行拆分。
列选择按原生 cluster 和显示行生成，短行钳制、隐藏行跳过、不可连续表达的双向范围整次拒绝；
保留临时矩形意图，编辑后固定文本位置。分帧准备与后续输入排队，静止指针不重置未完成工作。
不折行长段落按逻辑行处理，横向工作窗口有原生页数上限，保留绘制视口；垂直目标 X 使用 float64。
拆分以原主选区活动端所在的最后有效行为主，末端恰在下一行起点时排除下一行。

通过：go test ./...、GOWORK=off go test ./...、go test -race ./internal/view ./sourceview ./ui。
定向用例覆盖原生 Tab/中文/组合字符、双向范围拒绝、远端分页窗口、静止大矩形、数量上限、
跨扫描块匹配/回绕、拆分方向/末行、文档切换取消；另验三条 100000 字节长行的远端矩形。
在第一批同一 Linux/Pango/Software/TigerVNC/Xfwm4 环境，六组主题/缩放窗口均通过，
新增矩形拖选后立即输入、添加指针、键盘列扩展、全部匹配及上下添加光标，全部经正常事件分发。
窗口日志 /tmp/sourceview-phase2-awu3_oan/；独立依赖和 race 日志为
/tmp/sourceview-phase2-independent.log、/tmp/sourceview-phase2-race.log。
其他平台原生交互未执行，桌面与剪贴板副作用仅发生在已关闭的独立显示。

## 第三批：编辑辅助与输入合并

已完成配对/包围/生成闭括号跳过、成对删除、语言新行、智能退格、Tab 停靠、词/子词命令、
按行选择、括号/标签跳转、整行插入/移动/复制/删除、行/块注释、结构扩缩及后台大小写、清理、
重缩进和重缩进粘贴。整行事务合并行块并保留各光标；结构历史可恢复扩选时合并的光标。
折叠和长行收起调整所有受影响光标，空白/折叠标记与右键命中使用完整集合。
后台作业复用一份快照模型；选项修改、语言回调重入和新输入使旧计划失效，保留回调新排入的工作。
补全和影子建议仍保留已确认的单点边界；无新增第三方依赖和 GOUI 修改。

连续同来源普通输入/同方向逐字删除在完整集合衔接时合并撤销，正文每次输入仍一次修订/通知；
历史只保留最初/最终集合。配对、IME、粘贴、词删除、智能缩进删除、移动、来源/数量变化等中断分组。
新增明确预期覆盖多点输入/删除合并、来源/方向/分支、完整前后历史、配对/标签联动、同标签两端不重复镜像、
整行去重及结果坐标、结构合并后缩选、回调修改选项/重新排队、跨段落 cluster 端点与只读。
另补真实原生折行矩形、折叠行跳过、改宽后固定选区、跨页键盘列扩展/收缩、重叠文本匹配保留原主范围。
跨页键盘用例先复现锚点原生页限制，再验证活动边缘/相邻停靠点准备修正。

核心验证通过：go test ./...、GOWORK=off go test ./...、GOWORK=off go vet ./...；
go test -race ./internal/... ./sourceview ./ui（后续选择命令修改后追加 SourceView race）。
关闭工作区的 -count=1 全量通过；最终代码另有全量与独立依赖回归。
Windows amd64、macOS arm64 在 CGO_ENABLED=0 下 go build ./... 及 SourceView 测试交叉编译通过，
不将其计作原生运行。环境仍为 Ubuntu 22.04.5 amd64 / Go 1.25.8 / GOUI v0.7.0，无 go.work。

独立 TigerVNC / Xfwm4 compositor on / Pango / Software，Basic、Modern Light/Dark ×100%/150%：
多选区窗口六组通过，含连续输入撤销、配对/跳过/删除、后台大小写/清理/合并/重缩进、
不同目标相对缩进粘贴与取消；BindSelections 原生协调六组通过，覆盖局部更新、写回副本、
源切换、单/多绑定互斥、回调替换、卸载无订阅及重挂载。
旧窗口三十二组最终矩阵全部通过：editor、单选区 binding、find、experience、selection、line_marks、
view_state、lines、structural、两种 assistance、completion typing/navigation、长段落 input/forward/end，
各 100%/150%。最终日志 /tmp/sourceview-phase3-regression-accepted-ui98bbgr/。
合计本批最终四十四组窗口通过；专用显示和后台进程均已退出。

保留失败与处理记录：

- 选项代次最初误用于通用有效性检查，导致既有影子建议重排测试空指针失败；已限定为编辑计划检查，
  既有断言未改，定向及完整模块/race 重跑通过。原日志 /tmp/sourceview-phase3-options.log。
- 首轮窗口程序编译失败后脚本继续运行旧产物，该矩阵（/tmp/sourceview-phase3-amf9wamv/）废弃；
  后续构建脚本改为失败立即退出，重建后才计验收。
- 一次重缩进粘贴返回 33 字节，与第二目标未知上下文时的字面降级结果一致；新增确定性用例验证相同 33 字节正文与完整选区，
  及分析完成后的 41 字节结果。无 tokenizer 的试验准备返回字面粘贴，不能用来验证相对缩进；
  最终窗口场景观测原 JSON tokenizer 的末行完成后再发输入，保留原 41 字节精确正文断言。
  原日志 /tmp/sourceview-phase3-final-multiple-3t552398/、/tmp/sourceview-phase3-verified-ot5hdux2/。
- 两次完整旧窗口回归的 150% completion typing 在 Go 样本准备后超时；单独重跑和第二批对照构建均通过，
  不能据此确认是既有失败或唯一根因。用例原依赖一个定时器间隔来等待分析，现通过公开 tokenizer 观测完成后
  仍只发一次 F2；保留全部原断言、20 秒超时与正常事件路径。失败日志
  /tmp/sourceview-phase3-regression-gp8vxtlf/27-completion-1.5.log、
  /tmp/sourceview-phase3-regression-final-h4up_zh2/27-completion-1.5.log；对照在
  /tmp/sourceview-phase2-baseline-run-get8jy03/。不将前述失败矩阵整体记为通过。

最终新窗口日志 /tmp/sourceview-phase3-complete-2nqzc1b2/；多选区绑定日志
/tmp/sourceview-phase3-final-multibinding-8kedqj6w/。核心最终检查在 /tmp/sourceview-phase3-complete-*.log，
全 internal race 在 /tmp/sourceview-phase3-delivery-race.log，交叉编译在 /tmp/sourceview-phase3-cross/。
未执行：Windows/macOS 原生窗口、真实 IME 服务及物理键盘；未作性能结论。
桌面与剪贴板副作用仅发生在专用显示，未写回样本或操作用户已有桌面。

## 后续修正：示例接入与默认窗口入口

用户回报后，在 c5205b8 稳定复现不带 `-clipboard` 的窗口启动失败：可选粘贴场景未执行，
先前 ReindentSelection 的 F8 绑定仍存在；取消后台编辑场景又将 F8 绑定给 UpperCase，
SetKeyBindings 正确拒绝冲突，测试程序 panic。默认入口的六组主题/缩放均失败，日志保留在
/tmp/sourceview-reported-default-vkalpnbf/。此前四十四组通过记录只覆盖了本用例的 `-clipboard`
路径，不能扩展为默认入口也通过。

修复后，各场景从 DefaultKeyBindings 独立配置临时 F8，不依赖其他场景清理绑定；
保留原有正文、完整选区、撤销、取消及超时断言，没有放宽 skip、条件或测试预算。
`example/editor` 增加 Selection 菜单、随完整集合更新的选区计数、当前配置对应的鼠标/快捷键提示，
并加入 `Go — multicursor.go` 演示样本。状态栏经 ConnectSelections 更新，只移除附加选区也立即刷新；
菜单复用控件公开方法，示例没有额外光标状态或输入捷径。

通过：go test ./...、GOWORK=off go test ./...、GOWORK=off go vet ./...；
Windows amd64、macOS arm64 在 CGO_ENABLED=0 下构建 example/editor 与 multiselection 窗口程序。
核心日志 /tmp/sourceview-example-fix-delivery-*.log，交叉编译日志 /tmp/sourceview-example-fix-windows.log
与 /tmp/sourceview-example-fix-darwin.log。
Ubuntu 22.04.5 amd64 / Go 1.25.8 / GOUI v0.7.0；独立 TigerVNC / Xfwm4 / Pango / Software：
Basic、Modern Light/Dark ×100%/150% ×默认/`-clipboard` 共十二组全部通过，
日志 /tmp/sourceview-example-fix-o6jvvkn2/。命令分别为：

```sh
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software go run ./tests/window/editor/multiselection
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software go run ./tests/window/editor/multiselection -clipboard
```

第二条会覆盖测试显示的真实剪贴板，仅在专用桌面运行；`-basic`/`-dark` 和 GOUI_PLAT_SCALE
按上述矩阵分别组合，不将单个配置或包测试当作窗口矩阵通过。

示例以真实 XTest 键鼠输入实操并查看截图（Modern 亮/暗、100%）：从 Sample 选择新样本，
菜单添加上下光标、下一个/全部匹配、按行拆分、只保留主选区均可用。全部匹配显示 4 个选区，
将 product 同时改成 x 后减少 24 字节，一次撤销恢复；数字列选显示 3 个选区，
三组数字替换成 9 后减少 6 字节，一次撤销恢复。只保留主选区时主范围不动、计数降为 1。
只读模式可添加选区，输入不改正文；860×540 最小窗口的工具栏、操作提示和状态栏可见。
这部分为截图人工核验，不冒充程序断言或像素金图；截图和应用日志在
/tmp/sourceview-example-interaction-36gqbmfl/。外部合成状态查询脚本首次漏设 XAUTHORITY 导致空连接退出，
纠正后确认合成选择所有者是该专用显示的 xfwm4；该脚本失败与编辑器断言分开记录。

专用窗口、桌面与后台进程均已退出。没有写回样本、使用用户已有桌面或覆盖其剪贴板。
未执行其他平台原生窗口、真实 IME 或物理键盘；本轮不作性能结论。

## 后续修正：统一 Alt 鼠标手势

按用户澄清，所有平台默认 Alt+左键点击添加光标、Alt+左键拖动追加选区、
Alt+Shift+左键拖动矩形选择，移除 Linux 默认 Ctrl / Ctrl+Shift 的例外。
保留 SelectionModifiers 的精确匹配、覆盖和禁用契约；示例提示随配置自动同步。
专题设计补充 VS Code 官方鼠标手势与 Scintilla 修饰键配置/窗口管理器边界的取舍，
README 同步操作说明。

新增 TestDefaultAltSelectionGesturesAndOverride：修改默认值前在 Linux 复现
`default mouse gestures: got {2 3}, want Alt / Alt+Shift`，修改后通过；另断言不匹配
Ctrl、仅 Shift、Alt+Ctrl，以及自定义 Ctrl / Ctrl+Shift 可覆盖默认值。
窗口场景改用字面 Alt / Alt+Shift 构造正常 DispatchEvent 输入，不再从被测配置推导输入；
独立断言默认值，并增加保留已有两个光标、Alt 拖动追加第三个范围的完整集合断言。

通过：go test ./...、GOWORK=off go test ./...、GOWORK=off go vet ./...。
GOWORK=off、CGO_ENABLED=0 下，Windows amd64 和 macOS arm64 的 go build ./... 均通过；
交叉构建不计为原生运行。核心日志 /tmp/sourceview-alt-tests.log、
/tmp/sourceview-alt-independent.log、/tmp/sourceview-alt-vet.log。
Ubuntu 22.04.5 amd64 / Go 1.25.8 / GOUI v0.7.0，无 go.work；
独立 TigerVNC / Xfwm4 compositor on / Pango / Software：
Basic、Modern Light/Dark ×100%/150% ×默认/`-clipboard` 共十二组全部通过，
命令同上一节，新增默认值、Alt 添加/拖选和 Alt+Shift 矩形场景均通过。
日志 /tmp/sourceview-alt-gestures-5604758f/。

示例使用真实 XTest 键鼠输入并人工查看截图（Modern Light、100%）：

- Xfwm4 的 easy_click=Alt 时，Alt+点击和 Alt+拖动均未追加选区，计数仍为 1，窗口位置未变；
  同一设置下 Alt+Shift 拖动已选中三行数字，计数为 3。不能将正常 DispatchEvent 的通过
  等同于桌面一定会把 Alt 事件交给控件。
- 仅将该临时桌面的 easy_click 改为 Super 后，Alt+点击产生两个光标；输入 x 在两处插入，
  774 字节变为 776，一次撤销恢复正文及两个光标。
- 保留上述两个光标，Alt+拖动追加第三行的 300 范围，计数为 3；输入 x 后两处插入、
  一处替换，一次撤销恢复正文和完整三个选区。
- Alt+Shift 拖动选中 100/200/300 三行数字，输入 9 全部替换，774 字节变为 768；
  一次撤销恢复正文及三行列选。

这部分为真实事件与截图人工核验，不计作程序断言或像素金图；截图/应用日志在
/tmp/sourceview-alt-interaction-_ose3se7/。临时 Xfwm4 的会话总线和配置目录经进程环境确认，
未连接或修改用户桌面配置。首次外部交互脚本在构建完成前启动，因二进制尚不存在而报
FileNotFoundError；等待构建成功后重启并完成上述验证，该次编排失败不计作应用验收。

专用窗口、桌面、会话总线与后台进程均已退出，剪贴板只在矩阵的独立显示内覆盖，
未写回样本或操作用户已有桌面。未执行 Windows/macOS 原生交互、真实 IME 或物理键鼠；
本轮不作性能结论。

## 后续修正：Linux 示例使用 Ctrl 添加选区（2026-11-12）

用户确认在 Xfce4 保留 Alt 窗口操作，采用编辑器配置避让的方式。
`example/editor` 在与键盘快捷键相邻的初始化处读取 SelectionModifiers，仅在 Linux 将 Add 覆盖为 Ctrl，
Column 保留 Alt+Shift；经公开 SetSelectionModifiers 应用并处理错误。提示从控件配置生成，
因此显示 Ctrl+点击添加光标，README 和专题设计同步说明 Ctrl+拖动追加范围与操作顺序。
控件本身及 Windows/macOS 示例仍使用原默认值，无 GOUI 修改或窗口管理器设置写入。

通过：go test ./...、GOWORK=off go test ./...、GOWORK=off go vet ./example/editor。
核心日志 /tmp/sourceview-ctrl-example-tests.log、/tmp/sourceview-ctrl-example-independent.log、
/tmp/sourceview-ctrl-example-vet.log。GOWORK=off、CGO_ENABLED=0 下，Windows amd64 与 macOS arm64
的 example/editor 构建通过，产物最终置于仓库外临时目录；不计作其他平台原生交互通过。

Ubuntu 22.04.5 amd64 / Go 1.25.8 / GOUI v0.7.0，无 go.work；
独立 TigerVNC / Xfwm4 4.16.1 compositor on / Pango / Software，Modern Light 100% 与 Dark 150%
两组均以真实 XTest 键鼠输入并人工查看截图验收。两次均从专用 WM 进程环境确认会话总线与配置目录，
只读查询 easy_click=Alt，整个验证保留该设置：

- Ctrl+点击从一个光标变为两个；输入 x 分别插入，正文 774 字节变为 776，
  一次 Ctrl+Z 恢复正文和两个光标。
- 保留这两个光标，Ctrl+拖动选中第三行的 300，形成两个光标和一个范围；输入 x 后两处插入、
  一处替换，一次 Ctrl+Z 恢复正文及完整三个选区。
- 先按住 Alt+Shift，再用左键拖动选中 100/200/300；输入 9 将三组数字替换为 9，
  正文 774 字节变为 768，一次 Ctrl+Z 恢复正文与三个范围。
- 界面提示分别显示 Ctrl+点击与 Alt+Shift+拖动，状态栏随两个/三个选区更新。

以上是示例真实事件的截图人工核验，不冒充程序断言或像素金图；本轮未改动测试条件、skip 或超时。
截图及应用日志 /tmp/sourceview-ctrl-example-fhs56xiz/、/tmp/sourceview-ctrl-example-dark-zv32johy/。
保留外部编排问题：100% 场景已全部完成并截图，正常关闭应用时，桌面清理先于 xdotool 完成，
关闭动作报 X connection broken；应用及桌面 runner 正常退出。150% runner 在应用退出后留出
短暂清理间隔，全部场景与正常关闭返回零。第二组首次启动命令曾拼错临时目录，应用未启动，
纠正路径后才执行并计入上述验收；不将这次脚本启动失败记作应用失败或通过。

两组专用应用、窗口管理器、桌面与会话进程均已退出；没有操作用户已有桌面、修改其配置或剪贴板，
没有写回样本。未执行 Windows/macOS 原生窗口、真实 IME 或物理键鼠；本轮不作性能结论。
