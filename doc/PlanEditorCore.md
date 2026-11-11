# 独立编辑内核实施与验收

设计见 [独立编辑内核](DesignEditorCore.md)。分支 feature/editor-core，GOUI v0.7.0。

## 第一批：自有文本设施

迁入公开文本设施的实际使用部分到 internal/text，保留来源、版权及 MIT 许可说明，
形成自有正文分段存储、行索引、修订、原子批量修改、差量历史、位置、预编辑投影、
测量片段、编辑几何和高度索引；包内没有 Widget 或原生资源。

迁入对应回归用例，包括独立字符串参考的随机编辑、存储释放、通知重入、批量原子性、
来源与历史选区、Unicode 与长词、投影隔离、双向/折行几何、测量片段所有权及高度索引。
go test ./internal/text 通过；已有工程迁移前 go test ./... 通过。

## 第二批：自有长段落布局

在 internal/view 迁入 GOUI v0.7.0 公开 ParagraphLayout，替换其值类型依赖为
本仓 internal/text；原生 TextLayout、Painter、4 KiB 页面、最多 12 页缓存、
前缀方向上下文、坐标精度及失效算法保持基线行为。实现和迁入回归均保留来源与许可。
uniseg v0.4.7 转为直接依赖，版本不变；不新增字体、语言资源或原生库。

包内新增原生完整段落参考：ASCII、中文/组合字符/emoji、RTL、isolate/override，
验证软折点、cluster、基线、页接缝编辑和巨大坐标下的绘制/命中；原生送测上限 8192 byte。
固定度量检查末块估宽及分页基线；视图隔离检查不同宽度的资源、销毁、宽度及编辑失效。
原生调用在实际测试 goroutine 锁线程，Linux Pango 无桌面测试通过。

`go test ./internal/view` 通过。第三批切换 Document 与使用者后再记录完整集成验收。
迁移基线二进制、样本和日志均在仓库外 `/tmp/sourceview-core-tluj_grl/`。

## 第三批：Document、控件与使用者

Document 独立拥有模型并公开正文、范围/行查询、修订、修改、批量事务、历史和变更信号。
移除 Model()、NewDocumentWithModel 和 ui.TextModel；TextRange、TextSelection、TextEdit、
TextChange 使用本仓值类型，ui 提供同名别名。公开契约测试覆盖未挂载文档、规范化、
非法 UTF-8 边界、批量原子性与通知顺序、撤销重做、清理历史、重载及回调重入/断开。

SourceView 的编辑命令、历史、选区、IME 投影、几何、片段、高度索引和长段落全部使用
自有实现。语言高亮、折叠、视图状态、示例、后台分析示例和窗口程序同步迁移。
普通查找输入与语义快照在 GOUI 边界显式转换值；比较 TextView 的专项程序分别连接
两种控件的变更，只共享测试观察接口，不共享或借用正文模型。

扫描全部 Go 文件，无 goui/gui/textedit 导入或 gui.ParagraphLayout/ParagraphGeometry 调用。
GOUI 普通控件仍可传递依赖自己的文本包；这不属于 SourceView 编辑内核的依赖。
未改动 GOUI 仓库、go.mod 中的 GOUI v0.7.0 或原生 typography API。
多光标、列编辑和保存点均未实施；相应未来提案仅更新所有权。

## 验证环境与结果（2026-11-11）

Ubuntu 22.04.5 amd64、Go 1.25.8、发布依赖 GOUI v0.7.0。
GOWORK 为空，没有本地工作区或 replace；本轮没有跨仓联调。

通过：

- `go test ./...`、`GOWORK=off go test ./...`、`go vet ./...`。
- `go test -race ./internal/... ./sourceview ./ui`。
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`。
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`。
- 上述两平台的 internal/...、sourceview、ui 测试程序 `go test -c` 编译。

新增测试在 Linux Pango 原生运行，包含完整段落的独立度量参考；固定度量测试不需要桌面。
Windows/macOS 仅交叉编译，未运行原生测试或窗口；不能将编译通过视为原生验收。

独立 TigerVNC X11、Xfwm4 compositor on、Pango / Software：

- 视图状态六组：Basic、Modern Light/Dark 各在 100%/150%，包含反向选区、只读、
  屏外光标、结构折叠/长行收起、重建，以及 10.5 MB 单行中段恢复和用户操作取消。
- 其余 22 组：100%/150% 各运行主编辑器 `-auto -clipboard`、binding、find、experience、
  selection、line_marks、completion typing、completion navigation（Modern Dark），
  以及 8.58 MB 单行的 `-verify-input -nowrap -clipboard`、`-verify-forward -nowrap`、
  `-verify-end -nowrap`。保留正文、选区、像素、资源、过期回调与撤销断言，全部退出成功。
- 真实 XTest 对照两组：8,580,014 byte 单行中段的滚动条定位、点击、输入/粘贴、
  替换、删除及撤销重做；另一组执行输入、删除、撤销重做、宽高拖动及两轮最大化/还原。
  迁移前后均与原始全文精确比较，应用退出记录 verified；未通过的操作不计入结果。

真实输入基线为仍使用旧文本设施的 ac64829 工作状态；构建和样本保留在
`/tmp/sourceview-core-tluj_grl/`。对照同一主机、Pango / Software、100% 和同一样本，
`desktop.py` 从 XTest 的 XFlush 前计时至 XGetImage 看到新的非 pending 绘制状态，
不是显示器扫描输出或 API 返回耗时。基线/迁移后的可见耗时观察：

| 场景 | 操作数 | 基线中位 / 最大 ms | 迁移后中位 / 最大 ms |
| --- | --- | --- | --- |
| 大单行中段 | 15 | 191.4 / 271.3 | 200.1 / 282.3 |
| 编辑及尺寸变化 | 54 | 196.5 / 1000.4 | 212.5 / 1002.8 |

这些是各一次完整场景的观察，受桌面调度和等待影响，不据此作性能改善或无回归的统计结论。
此轮没有发生最终未解决的测试失败；恢复工作时的未完成类型迁移曾造成构建失败，已修正。
没有放宽旧断言、增加跳过或把环境缺失记为通过。

日志、截图、样本及二进制在 `/tmp/sourceview-core-tluj_grl/` 的 baseline-142547、
after-161040、native、regression、bin 与平台测试目录。测试只操作独立显示，
剪贴板用例覆盖该显示的临时剪贴板；不向用户桌面发送输入，不写回样本或修改系统设置。
全部专用窗口、Xvnc、Xfwm4 与测试进程已关闭，产物保留供复核。
真实桌面 IME、Wayland、OpenGL、Windows/macOS 原生窗口未执行。
