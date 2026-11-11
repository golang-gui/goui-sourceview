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

首批未适配的编辑命令在多选区时禁用或明确返回 ErrMultipleSelections；第三批继续适配。
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

待第二批通过后实施。所有结果按通过、失败、未执行分别记录，不将编译当作原生验收。
