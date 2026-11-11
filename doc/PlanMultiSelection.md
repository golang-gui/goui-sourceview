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
