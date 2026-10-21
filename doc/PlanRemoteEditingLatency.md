# 远端大文本编辑验证记录

2026-10-21。接续 [Linux 真实输入验证与修正](PlanEditingLatency.md)，目标是在
macOS/CoreText 与 Windows/DirectWrite 上验证真实打开、滚动、中部点击、输入、
删除、替换、撤销重做及宽高拖动；根据结果修改、复测并按功能提交。
前半部分保留初次连接阶段的记录；连接恢复后的实际结果见下方
[继续验证](#连接恢复后继续验证)。并非所有桌面响应项目均已通过。

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

CoreText 临时存储优化前后，全仓失败名称集合一致。随后独立修正下面的覆盖计数，
未删除断言或扩大 skip：

- GOUI `go test ./...`：`platform/test.TestWindowInteractionCapabilities`；没有输入
  上下文时 resize 返回 `ErrUnavailable`，测试期待 `ErrUnsupported`。
- SourceView `go test ./...`：`TestExplicitContextFragmentProbe` 的 longRLO、
  longLRO、longRLI、unicode，及 `TestBoundedFragmentProbeMatchesSimpleParagraphs`
  的 word、json；独立窗口纵向累加没有与 CoreText 完整段落坐标一致。
- 同一全仓命令最初还在 `TestNativeFragmentsSurviveLayoutRelease/override` 失败：
  RLO 目标覆盖计算得 6 而期望 3。独立原生查询确认三个目标簇为 X、Y、Z+PDF；
  最后一个合法簇占 4 bytes。测试现按簇与 XYZ 目标的交集累加，仍独立要求全部
  3 bytes 被覆盖，且每个交叉簇均为 RTL；不将控制符计入可见目标字节。
  修正后的定向普通/race 测试在 Mac 与 Linux 均通过，Mac 全仓仅余上述两种 probe
  及其子用例失败。

前两种分段 probe 本来就不是启用的控件路径；新的完整输入缓存回归在 CoreText
通过。但 probe 失败仍是全仓失败，没有以增大 skip、容差或删除断言制造通过。
日志按 baseline、scratch-all、scratch-tests、scratch-bench、scratch-race 保留在
仓库外，不提交源码树中的测试图片、profile 或大文件。

启动诊断窗口在没有执行桌面编辑的情况下已按所属 PID 终止，没有正常关闭的
`verified` 记录，明确不计入通过操作；用户原文件及转移文件哈希仍相同。本次创建
的 Windows RDP 客户端已断开，原有 Mac SSH 会话及远端已有工作区保留。

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

## 连接恢复后继续验证

同日用户恢复 SSH、VNC、RDP 后继续部署、修正和实测，前面的连接阻塞不再是
当前状态。全部操作在仓库外隔离快照进行，保留远端原有仓库及未跟踪文件；
本地提交是正式交付，未向远端已有工作区覆盖或创建提交，也未 push。

环境：

| 平台 | 原生文字 / 桌面 | 机器与运行条件 |
| --- | --- | --- |
| macOS 15.7.7 arm64 | CoreText，VNC 1650×1050 | Apple M1，Go 1.25.8，Software，1x |
| Windows 10 19045 amd64 | DirectWrite，RDP 1600×900 | Xeon Gold 5318Y，Go 1.25.8，Software，1x |

窗口均为 Modern、WindowChromeIntegrated、初始 1100×760，自动换行；TextView
放在 ScrollView 内，SourceView 保留行号、折叠和标签联动，关闭自动补全。
真实键鼠由 VNC/RDP 注入正常平台事件，未使用 DevServer、编辑 API、后台测量或
额外自动化端点。测量边界、窗口与脚本复现方式见
[远端桌面观察](PlanEditingLatency.md#macos--windows-桌面观察)。

### 远端复现后落实的修改

GOUI 按功能分别提交：

- `1b7ac4f Preserve Cocoa repaint requests made during drawing.`：首帧的
  GUI endFrame 在 AppKit drawRect 内请求下一帧，直接 SetNeedsDisplay 的请求
  被当前 drawRect 收尾吞掉，之后输入已到控件但桌面不刷新。改为主 run loop
  common modes 的一次性合并请求，执行时重查原生窗口有效性。没有周期强刷、
  后台线程或排版策略进入平台。新的独立原生窗口回归，旧代码仅 1 帧并超时
  退出 1，新代码准确 4 帧、销毁后无回调并退出 0；默认原生 chrome 与实际
  Integrated 编辑窗口均验证。Linux Software 1x 同一回归通过。
- `1ae54a9 Reduce DirectWrite text position query overhead.`：保留每个簇的
  原生 HitTestTextPosition 结果，精简 SDK 调用封送。一次调用的输出放同一个
  pinned 结构体，覆盖同步原生回调引起的 Go 栈增长。与旧通用封送逐字段比较
  所有 UTF-16 位置、leading/trailing、空文、换行、Tab、CJK、组合字、emoji、
  Arabic/Hebrew、override/isolate 及宽窄约束，原生及 race 回归通过。
  同机预排后五组各 100000 查询，中位数 963.8 → 520.8 ns，135 B/4 allocs →
  96 B/2 allocs。这是 SDK 查询成本，不是完整编辑延迟。
- `9fb4380 Reuse DirectWrite raster color effects between tiles.`：实际窗口
  分析器发现有色长段落每帧多次区域光栅化，每块均重复设置同样的颜色效果，
  单块约 65 ms。仅复用 TextLayout 自有目标上已经应用的颜色效果，颜色变化、
  目标重建、外部目标绘制和失败均使状态失效，不保留外部 target。1x/2x 原生
  像素、区域变化、重着色、目标交替/重建及 race 回归通过。同机预热后五组
  每组 30 次，中位数 38.089 → 4.951 ms。这是重复光栅成本，实际编辑见下表。

原生契约、资源归属与官方参考分别记录在 GOUI DesignWindowManagement 和
DesignTypography。此前的 CoreText 临时数组复用仍保留；没有重新引入 Reflow。

### 实际操作的对照

所有完成轮次执行滚动条远跳、中部点击、插入、退格、短词输入、选词替换、
Undo/Redo、Left/Delete；普通关闭时由窗口逐字节确认恢复原文，并取得进程退出 0。
实际 GUI capture 到末尾 Widget.Paint 的同机时间，仍排除原生 Present 与远程
传输；不能把它称为用户到显示器的完整延迟。多按键选词/整词输入还包括实际
按键投递间隔，不拿该时间当一次原生调用。

Windows 同一 GUI/SourceView/窗口输入，只替换 SDK 的旧通用封送与优化实现；
未开启 profile，旧 2 轮、新 3 轮，输入均为同哈希的 157862 byte JSON：

| TextView 中部操作 | 旧 SDK 范围 ms | 新 SDK 范围 ms |
| --- | ---: | ---: |
| 首次输入 x | 337.6–338.0 | 254.1–272.4 |
| 首次替换选词 | 317.8–331.2 | 245.6–257.3 |
| 恢复原文那次 Undo | 325.1–326.1 | 245.7–251.8 |
| Backspace 命中已测文本 | 26.6–30.1 | 26.6–28.2 |
| 12 字符正常键入 | 2867.5–2896.1 | 1993.5–2059.6 |

该对照中桌面截图端首次输入约 525–656 ms；RDP 传输、截图节奏会掩盖部分差异，
所以原始 `visible_ms` 亦保留，不以 GUI 内部时间替换它。日志为仓库外
`win-sdk-before-{2,3}`、`win-final-text-{2,3,4}`。旧第 2 轮的 corner-restore
只观察到 1 个中间尺寸，其编辑及全文恢复通过，但整轮 resize 响应验收失败。

Mac 原排版基线为 GOUI `4018c7a` / SourceView `b809dcf`，两边均应用上述 Cocoa
绘制请求修正以使输入能够呈现；新快照为 GOUI `1ae54a9` / SourceView `13c8f71`
加相同观察程序（随后提交为 `3ecfd85`）。各一轮 TextView 对照，仅是实测样本，
不据单轮声称稳定百分比收益。输入法打印字符没有 GUI KeyDown，另报模型 change
到末尾 Paint：首次 x 171.6 → 148.4 ms，选词替换 173.3 → 101.5 ms；普通
Backspace 的 GUI capture 到 Paint 184.7 → 23.6 ms，Delete 145.8 → 36.9 ms。
后两项包含近期完整输入复用的收益。VNC 端就绪帧观察仍约 3–7 秒，未隐藏该延迟，
没有把它直接归为 GUI 连续冻结；新轮 GUI Timer 最大间隔约 284 ms。

DirectWrite 颜色复用另做同条件、关闭分析器的交替对照，各 2 轮；两边均已有
SDK 查询优化，只替换颜色复用。SourceView 中部操作的 GUI capture 到末尾 Paint：

| 操作 | 重复应用颜色 ms | 复用颜色效果 ms |
| --- | ---: | ---: |
| 首次输入 x | 613.8–629.7 | 437.9–439.2 |
| Backspace | 337.7–343.2 | 161.3–164.4 |
| 替换选词 | 625.8–650.5 | 433.2–441.7 |
| Undo 替换 | 327.5–331.7 | 154.9–156.2 |
| Delete | 333.3–338.2 | 159.7–162.5 |

记录为 `win-colors-before-{2,3}`、`win-colors-after-{2,3}`。桌面端首次 x 为
823–952 → 690–691 ms；退格却为 552–556 → 686–691 ms，说明 RDP/截图节奏及
后续绘制会影响观测，不能声称每项用户端延迟同比下降。新实现首次单段排版仍慢，
而非已实现分时送测。另有 `win-raster-source-json-1` 完整编辑与缩放通过，但启动
阶段同时跑了原生 race，不纳入上述性能对照。

### 多行语料与完整性

两端各实际完成以下窗口的 14 项中部操作（XML/HTML 多一项行尾定位），每轮
脚本退出 0、普通关闭全文恢复校验通过、进程退出 0。无 profile，无只加载即算通过。
来源、许可证与原始哈希沿用 [语料记录](PlanCorpusLayout.md)；CR/LF 规范化后
的模型长度如下。这里只报告各 1 轮功能验收，不对其作稳定性能比例推断。

| 控件 / 文件 | 模型 bytes | Mac | Windows |
| --- | ---: | --- | --- |
| TextView / TXT | 3293373 | 通过 | 通过 |
| SourceView / 单行用户 JSON | 157862 | 通过 | 通过 |
| SourceView / 54 MB JSON | 54009790 | 通过 | 通过 |
| SourceView / OpenGL XML | 2774722 | 通过 | 通过 |
| SourceView / Gutenberg HTML | 3961797 | 通过 | 通过 |
| SourceView / Rust Markdown | 931116 | 通过 | 通过 |

原始记录为 `mac-final-source-{json,xml-2,html,markdown,54mb}`、
`mac-final-text-txt`，以及 `win-final-source-{json,xml-2,html,markdown,54mb}`、
`win-final-text-txt`。Mac 多行语料首次 x 的 change 到 Paint 约 39–104 ms；
它排除了此前平台排队及 change 处理，不能与 Windows GUI capture 的数据混为
一个跨平台性能排名。Windows 颜色复用后还复测 XML、54 MB JSON 和 TextView
用户 JSON，分别另存 `win-colors-{xml,54mb,text-json}`。

XML 最初两端点击均落在标签名，x 合法联动修改两个标签，脚本单处插入的字节
预期不成立并超时；它们是未通过的脚本轮次，不是卡住 30 秒。正常 Undo 后全文
恢复、关闭校验与进程退出 0，再以真实 End/Cmd+Right 定位行尾重测；没有关闭
标签联动或改用编辑方法绕过输入。远端未执行头尾编辑或 64 KiB 粘贴，相关 Linux
覆盖不能写成这两端也已验证。

### 当前限制

**仍未完成通用有限输入和段内增量排版。** 全新超长单段内容、新宽度仍会进行
完整原生同步测量，首次输入和连续改宽仍可能有明显长任务。完整输入缓存命中
不能代表新输入流畅，不以这些数字声称已实现 Scintilla/Monaco 的大文档能力。

Mac 六次拖动的最终宽高、文本与光标正确，正常关闭全文恢复、退出 0，但 VNC
未采到两次中间尺寸，六项响应断言均失败。保留 GUI 尺寸/Paint 与远端截图时间，
未放宽断言；不能将其记为拖动流畅或简单断言平台冻结。Windows 当前 TextView
及 SourceView 的单行 JSON 六次拖动均观察到至少两次中间尺寸并通过最终断言；
TextView 改宽/角部的松手到远端就绪帧仍约 340–477 ms，GUI 也仍有长任务。
Mac 此轮 GUI 实际记录到 11 次视口尺寸改变，覆盖缩放区间的 Timer gap 最大约
180 ms；Windows TextView 同口径最大约 347 ms，SourceView 颜色复用前约
545 ms、复用后的独立缩放轮约 367 ms。它们是事件投递间隔，含原生 resize
循环与系统调度，不是单个函数的 CPU 时间，也不能消除 Mac 桌面响应观测缺口。

### 验证命令与未通过项

- Linux 两仓 `go test ./...` 通过；SourceView `go vet ./tests/window/editor/interaction`
  与两个 Python 观察器语法检查通过。GOUI 原生重绘回归 Software 1x 通过。
- Mac `go test -race ./gui ./platform/darwin/cocoa ./platform/darwin/frameworks/core_foundation ./platform/typography/coretext`
  通过；GOUI 全仓仍为上述 `TestWindowInteractionCapabilities` 失败。SourceView
  全仓仍为前述两种分段 probe 及子用例失败，生产路径不启用这些分段实验。
- Windows SDK/DirectWrite 原生精确对照及
  `go test -race ./gui ./gui/textedit ./platform/windows/sdk/dwrite ./platform/typography/directwrite`
  通过。SourceView 全仓在 `TestExplicitContextFragmentProbe/longRLI` 失败；
  换回旧 SDK 后单独执行同一用例，坐标偏差完全相同，非本次封送优化引入。
- Windows GOUI `go test ./...` 在服务会话 0 执行，仍有 D2D 渐变预乘颜色、
  platform/test 的 popup/window/paint 超时，以及 win32 的 TestWindowStyle 失败。
  不将服务会话全仓结果当作 RDP 交互会话的窗口验收，不顺带修改这些测试。
- SourceView `GOWORK=off go test ./...` 仍失败于 `updates to go.mod needed`。
  保留用户的 go.mod replace 与 example/temp；工作区联调通过不代表可独立发布。

### 副作用与产物

测试改变两端焦点、指针和窗口尺寸；输入文件不写入，未占用远端剪贴板。
一轮早期 Windows 坐标沿用旧位置，误切到其他编辑器并输入 x，已正常撤销，
关闭测试窗口后截图确认恢复，未保存文件；该轮不计入结果。后来固定测试窗口
位置并增加每轮唯一标记，未看到当前窗口就不发送输入。原生、CPU profile、
截图、语料、原始 JSONL 和脚本日志均保留在仓库外，不提交生成图片或二进制。
用户恢复的 SSH/VNC/RDP 连接保持，测试只清理自己创建的交互启动任务与窗口。
结束时已确认无本轮测试窗口进程，移除 Windows 本轮启动任务；本机、Mac、Windows
三份用户 JSON 的 SHA-256 仍与文首一致。观察机产物保留在
`/tmp/goui-remote-long-text-20261021-resumed/`；Mac 隔离目录为
`~/Temp/goui-long-text-20261021/`，Windows 为
`%LOCALAPPDATA%\Temp\goui-long-text-20261021\`。远端已有工作区没有被覆盖。
