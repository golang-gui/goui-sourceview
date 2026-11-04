# P0 实施与验证

日期：2026-11-04。分支：`feature/p0-editing`；基线 `fd6bb6f`，依赖 GOUI v0.6.0。
本记录对应 [P0 设计](DesignP0Editing.md)，不将功能分支描述成已发布版本。

## 交付范围

- 语言折叠：JSONC/JSON5、YAML、TOML、Go、JavaScript/TypeScript、CSS、Python；引用、注释、块标量、续行和 region 边界测试。
- 独立 Folds 快照：候选与语言分离、分片复制、worker 校验、各视图分批接收；编辑/撤销映射、过期、IME、清除与重入保护。
- 单光标结构操作：子词移动/扩选/删除，结构扩大/缩小与精确轨迹恢复。补齐 Command 和 ui 别名。
- 缩进：语言 Newline、可定制 Reindent、显式缩进粘贴；多行字面数据保护、取消与一次撤销。
- 整理：合并行、Unicode 大小写、尾部空白清理；editor 示例增加 Edit 菜单，semantic 示例同时发布 AST 折叠。

按语言折叠、外部折叠、结构选择、缩进、整理及示例拆分功能提交。没有修改 GOUI、模块版本或默认快捷键。

## 逻辑与构建

| 检查 | 结果 |
| --- | --- |
| `GOWORK=off go test ./...` | 通过，包含现有测试及新边界/撤销/重入用例 |
| `GOWORK=off go test -race ./language ./internal/editor ./example/semantic/analysis .` | 通过；不能据此推断窗口主线程行为 |
| 各功能提交导出到仓库外后 `GOWORK=off go test ./...` | 逐个检查，避免拆分后出现中间提交缺少类型或方法 |
| `GOWORK=off GOOS=windows CGO_ENABLED=0 go build ./...` | 通过，仅交叉构建 |
| `GOWORK=off GOOS=darwin CGO_ENABLED=0 go build ./...` | 通过，仅交叉构建 |
| `go test ./...`（本地 go.work） | 失败：相邻 GOUI 开发树的 Style.Radius / CornerRadius 契约已变化；当前检查时 HEAD 为 `1398487`，未认定这是引入改动的提交 |
| Windows/macOS 原生窗口、物理 IME | 未执行，本轮窗口验收范围为本机独立 X11 |

工作区失败的关键信息：completion_list.go 的 `selected.Radius undefined`，styles.go 的 `s.Radius undefined`，
以及 DrawBoxShadow 的 float32 / geometry.CornerRadius 不匹配。失败处未由本轮改动；发布依赖 v0.6.0 的
完整构建和测试通过。本轮不将这些错误隐藏为 skip，也不扩大范围适配仍在变化的相邻工作树。

## 独立窗口验证

使用自有 Xvnc `:87`、独立 D-Bus 与 xfwm4 合成器，1920×1440；Linux、Pango、software Painter，
`XMODIFIERS=@im=none`。Basic 100%/150%，Modern 亮色与暗色各 100%/150% 均通过。
每次结束关闭自有窗口和桌面进程；测试仅在独立会话设置剪贴板，结束清空测试内容，未改变用户工作桌面。

程序 `tests/window/editor/structural` 通过正常 Window.DispatchEvent、CommitText 和平台剪贴板验证：

- 子词按键、正反向选区、结构扩大/缩小、只读镜像；实际文本与选区断言。
- JSON/YAML/TOML/Python/XML 换行及 Undo；重新缩进、Redo、用户编辑取消、定制回调输入和运行中取消。
- 缩进粘贴及字面数据保护；合并行、Unicode 转换、尾部空白、精确撤销选区。
- 共享文档的外部折叠隔离、重复发布，以及十万候选接收、折叠后输入、Undo、发布中取消。
- Modern 亮色 150% 另验证约 10 MiB JSON 的内置分析、1001 个折叠范围、真实输入和撤销。
- 约 10 MiB 文本的 4096 处原子整理；4097 处请求只报告 ErrOperationLimit，正文和历史不变。
- 约 10 MiB 自动换行单段中的选择、大小写转换与撤销。

已有 folding、shortcuts、semantic、assistance 窗口程序在 150% 通过。semantic 验证增加发布 Folds 后，
原有补全窗口身份/选项、颜色与迟到结果断言仍通过。assistance 保留原有空白像素与 GoTo 流程，未替换或弱化断言。
editor 示例在 Modern 暗色 150% 启动并检查正文、工具栏和浮层；截图留在仓库外。

## 大样本观察

以下是同一独立桌面的实际按键/窗口结果，不是纯模型接口基准：

- 外部样本：10,700,000 字节、200,001 行、100,000 个候选；提交后视图异步接收。
- 整理样本：10,248,192 字节、4096 处实际删除；输入事件返回后仍可处理其他事件，随后一个事务应用。
- 长段样本：10,800,000 字节的单段，自动换行，选中前四个字节转换。
- 10ms GUI timer 记录事件循环间隔；ready 是从按键/发布进入到首次观察到预期文本或候选数量的时间，
  精度受轮询和绘制影响。settled 额外保留至少 250ms 用于检查迟到结果，不当作纯操作耗时。
  Paint probe 只记录同窗口的绘制轮次，不能证明每个字形已经呈现，也不替代物理 IME/DPI 验证。

| 环境 | 十万候选 ready | 折叠后输入分发 | 4096 处整理 ready | 整理期间最大心跳间隔 |
| --- | --- | --- | --- | --- |
| Basic 100% | 3.288s | 4.659ms | 1.482s | 40.985ms |
| Basic 150% | 4.078s | 4.528ms | 2.425s | 129.012ms |
| Modern 亮色 100% | 3.319s | 4.252ms | 1.482s | 42.386ms |
| Modern 暗色 150% | 4.197s | 4.717ms | 2.500s | 140.501ms |
| Modern 暗色 100% | 3.264s | 6.158ms | 1.460s | 41.092ms |
| Modern 亮色 150% | 4.174s | 5.790ms | 2.887s | 139.493ms |

这些数值包含软件绘制、排版、GC、调度和测试观察成本，不是硬实时保证，也没有用它们宣称相对其他编辑器的优势。
150% 下部分场景仍会出现百毫秒级心跳间隔；没有把整个控件的大文件延迟写成已解决。
最终折叠交换及普通编辑仍有 O(F) 的步骤；通用批次通过单调坐标归一化和前缀差值避免 O(F×E)。
更大的原子整理受 4096 编辑预算限制，不通过拆分用户可见事务伪装通过。

日志与截图位于本次运行的 `/tmp/sourceview-p0-delivery`、`/tmp/sourceview-p0-extra`、`/tmp/sourceview-p0-json` 和 `/tmp/sourceview-p0-menu`；临时产物不提交。
