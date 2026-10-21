# 远端大文本编辑验证记录

2026-10-21。接续 [Linux 真实输入验证与修正](PlanEditingLatency.md)，目标是在
macOS/CoreText 与 Windows/DirectWrite 上验证真实打开、滚动、中部点击、输入、
删除、替换、撤销重做及宽高拖动；根据结果修改、复测并按功能提交。
本文记录当前完成部分与明确阻塞，**不是两端桌面验收通过记录**。

## 隔离与输入

远端 Mac 已有 GOUI 仓库较旧且有未跟踪文件，没有覆盖它。使用仓库外独立目录，
从本机 GOUI `b50da4f`、SourceView `33ac118` 导出源码，并带入真实窗口诊断程序
（对应 SourceView `fb577de`）。临时 `go.work` 同时引用两个模块，不修改全局 Go
环境或提交机器专属 replace。后续 CoreText 修改只更新该隔离目录。

转移同一份用户单行 JSON：157862 bytes，SHA-256
`7baa22a894b415fdc3425c94f8a541d0ded8f77965541c46af3c95a3364c798b`，在 Mac
重新核对一致，输入保持只读。另准备同一组 TXT、JSON、XML、HTML、Markdown
语料及 54 MB 派生 JSON；尚未完成的操作不以文件转移或窗口创建代替。

## Mac 原生验证与修正

环境为 macOS 15.7.7、Apple M1 arm64、Go 1.25.8、CoreText。窗口启动指定
Software 与 `GOUI_PLAT_SCALE=1`，创建 1100×760 的 WindowChromeIntegrated
TextView 窗口；尚未完成实际输入、屏幕缩放和呈现行为验收。

初次加载开启分析器，size 约 30 ms、metrics 约 18 ms。它们是原生阶段计时，
不代表真实点击、编辑或滚动的延迟，不与 Linux 的未分析端到端数据直接比较。

定位到每个原生换行 run 重新分配 positions、advances、indices，以及每行新建
索引表。GOUI CoreText 改为一次 `MeasureMetrics` 调用内复用临时容量、逐行清空
索引表；仍完整原生排版，输出普通值不引用临时数组，没有 Reflow、后台测量或
控件调度策略进入 typography。修改及原生参考依据见 GOUI DesignTypography。

同进程预先完成 shaping，188416 byte、1000 point 宽样本，五组每组 10 次：

| 指标 | 旧逐 run 分配 | 调用内复用 |
| --- | ---: | ---: |
| 导出 metrics 中位数 | 14.064 ms | 8.399 ms |
| 分配量／次 | 约 29.84 MB | 约 9.92 MB |
| 分配次数／次 | 约 54356 | 36250 |

不能据此声称真实远端编辑延迟下降了同样比例，也没有解决通用有限输入排版。

通过：

- GOUI `go test ./gui ./gui/textedit ./platform/typography/coretext`。
- CoreText 新回归逐字段比较旧原生导出，含 RTL、控制符、组合字、emoji、Tab、
  颜色 run、占位符和不同宽度；验证后续测量不更改已返回的普通坐标。
- GOUI `go test -race ./gui ./gui/textedit ./platform/typography/coretext`。
- SourceView `go test -race . -run 'TestNativeParagraphHistory|TestParagraphHistory|Test.*Color|Test.*Bracket|Test.*TagMatch' -count=1`。

全仓失败已在修改前后分别重现，失败测试名称集合一致，保留原断言：

- GOUI `go test ./...`：`platform/test.TestWindowInteractionCapabilities`；没有输入
  上下文时 resize 返回 `ErrUnavailable`，测试期待 `ErrUnsupported`。
- SourceView `go test ./...`：`TestExplicitContextFragmentProbe` 的 longRLO、
  longLRO、longRLI、unicode，及 `TestBoundedFragmentProbeMatchesSimpleParagraphs`
  的 word、json；独立窗口纵向累加没有与 CoreText 完整段落坐标一致。
- 同一全仓命令的 `TestNativeFragmentsSurviveLayoutRelease/override`：RLO 目标
  覆盖计算得 6 而期望 3，原生簇范围包含相邻控制符；需要独立核对范围断言。

前两种分段 probe 本来就不是启用的控件路径；新的完整输入缓存回归在 CoreText
通过。但 probe 失败仍是全仓失败，没有以增大 skip、容差或删除断言制造通过。
日志按 baseline、scratch-all、scratch-tests、scratch-bench、scratch-race 保留在
仓库外，不提交源码树中的测试图片、profile 或大文件。

## 桌面连接阻塞

- Mac：SSH 可用；已配置 VNC 两次均返回 `CONNECT_FAILED`，停止继续重试。
  系统查询显示当前诊断进程没有事件发送授权；不更改系统授权或尝试绕过。
  窗口创建和内部 paint 日志只能记作启动，不能算实际桌面操作通过。
- Windows 10 19045 amd64：RDP 可用，已观察 1600×900 桌面；SSH 配置别名的
  2022 端口返回 `Connection refused`，无法部署、读取源码及日志或原生执行测试。
  Windows 目标构建完成，但 DirectWrite 原生和桌面验证均未执行。

已经请求恢复上述连接。恢复后先核对系统、会话和缩放，部署同一份输入及前后
版本，复用正常平台输入路径；分开记录输入回显、非 pending 呈现、GUI 事件间隔、
实际中间尺寸帧和关闭时全文恢复，不能把远程调用往返时间称为控件阻塞时间。
这两端的真实操作验证、Windows 对照和根据其结果的修正仍未完成。
