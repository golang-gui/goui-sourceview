# 真实大文档布局与交互验证

2026-10-21。设计见 [布局第 12 节](DesignLayout.md#12-当前视口的段落分帧准备)。
本轮自主优化文本模型查询与 SourceView 视口准备，保留完整 Unicode 排版正确性。
不重新加入 Reflow，不使用后台原生测量。单条极长段落的通用范围测量仍未完成。

## 输入与复现

从官方项目或 Gutenberg 下载下列实际文件。原文与日志保留在仓库外的临时语料目录；
本库只提交来源、大小、SHA-256 和可复现工具，不重新分发原文。模型按既有契约将
CRLF 归一化为 LF。下表行数是归一化后模型 `LineCount`，包含末尾 LF 后的空行；
manifest 的 lines 是原文件 splitlines 计数。

| 输入与来源 | 原始 bytes | 模型行数 | 最长逻辑行 bytes |
| --- | ---: | ---: | ---: |
| [War and Peace TXT](https://www.gutenberg.org/files/2600/2600-0.txt) | 3,359,405 | 66,033 | 84 |
| [GitHub REST API JSON](https://raw.githubusercontent.com/github/rest-api-description/main/descriptions/api.github.com/api.github.com.json) | 13,502,446 | 357,042 | 16,907 |
| [OpenGL Registry XML](https://raw.githubusercontent.com/KhronosGroup/OpenGL-Registry/main/xml/gl.xml) | 2,774,722 | 47,773 | 427 |
| [War and Peace HTML](https://www.gutenberg.org/files/2600/2600-h/2600-h.htm) | 4,039,247 | 77,451 | 139 |
| [Rust releases Markdown](https://raw.githubusercontent.com/rust-lang/rust/main/RELEASES.md) | 931,116 | 16,413 | 744 |

SHA-256 完整值在 [corpus.json](../tests/window/editor/corpus/testdata/corpus.json)。
分支 URL 会变化；下载脚本校验大小及哈希，变化时失败，不能默默更换验收输入。
网络下载不由 `go test` 触发，运行工具需主动指定仓库外目录：

```bash
python3 tests/window/editor/corpus/fetch.py /tmp/sourceview-corpus
go build -o /tmp/sourceview-corpus-check ./tests/window/editor/corpus
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 /tmp/sourceview-corpus-check -file /tmp/sourceview-corpus/github-api.json
```

其他四个文件分别运行，换 `GOUI_PLAT_SCALE=2` 做独立缩放验证。可选
`-capture-dir` 指向已有临时目录输出最终视口图。Python 仅用于下载工具，不是库的依赖。
Gutenberg 与各项目的使用条件以原始来源为准；此次只读取与本机验证。

最初 GitHub commits API 返回 403 后改用 raw 内容及哈希固定；WHATWG 单页 HTML
下载超时，其不完整文件未作为验证输入。不能把下载失败记作控件通过。

另将已校验的 GitHub API JSON 放入数组四次，生成合法的派生压力文件：
54,009,790 bytes、1,428,166 行，最长行仍为 16,907 bytes，SHA-256 为
`cde2cf709c880c46fce3a99a5d41bc704ab81fbfb0cf0a68d5f2aad3b68ae24a`。
这是明确生成的 54 MB 压力输入，不是宣称原站点直接提供了该文件。

```bash
python3 - /tmp/sourceview-corpus <<'PY'
import pathlib, sys
directory = pathlib.Path(sys.argv[1])
data = (directory / 'github-api.json').read_bytes()
(directory / 'github-api-4x.json').write_bytes(b'[' + b','.join([data] * 4) + b']\n')
PY
```

该输入的 15 项窗口验证通过：load action 118.392 ms，中部点击与插入 38.514 ms，
滚动条拖动 0.689 ms，恢复宽度和恢复高度均无新建正文布局／度量导出。仅一次功能
压力运行，不作为性能对照。同步 SetText 在这个规模仍有可观察延迟，不能计作已分帧加载。

## 实现范围

GOUI 的 `gui/textedit` 为每个至多 64 KiB 块建立 LF 索引，消除行范围／偏移查询中的
重复块扫描。只改通用模型私有实现；共享查询也供 TextInput、TextView 使用，未重写
它们的视口调度。GOUI 的独立回归、查询对照见其 `doc/PlanTextLineIndex.md`。

SourceView 的缺失段落／颜色交给 Root 帧回调按软预算逐个准备；只处理最新需求，
轮转视图，保持旧高度估计与当前可见／reveal 工作集。Paint、Snapshot 和 IME 坐标
更新不因缺失正文而补排；屏外选中词边界只读相邻字符，关闭的空白装饰不再扫描正文。
公共 API 未增加；快照增加 `layoutPending` 说明是否尚有视口工作。

回归覆盖：Paint 无隐式正文创建、到期停止批次、慢 native 调用后让出、最新宽度／
视口覆盖、颜色保持几何身份、文档重入取消、自动失败抑制、双视图公平推进、首尾
reveal 收敛、高视口超过 128 行仍保留工作集、缩小后释放、编辑旧身份不被误保留，
以及真实原生折行高度估计和滚动条稳定。

## 窗口验证与对照

Linux amd64／Go 1.25.8／X11 `:3.0`／XFCE+xfwm4，Software painter，Modern，
WindowChromeIntegrated，Pango 1.50.6／Cairo 1.16.0，字体 monospace 12 pt，
WrapWordChar。窗口 960×640 DIP；禁用折叠与自动补全以隔离文本布局，按文件扩展名
使用 Plain／JSON／XML／HTML／Markdown 高亮。程序在真正入口 OS 线程创建 GUI。
这组语料对照不代表所有产品功能均开启的综合体验；默认折叠的综合复验另见下文。

每个文件 15 个阶段：加载、Ctrl+End、Ctrl+Home、滚轮、竖滚动条远距拖动、中部点击
插入中文、撤销、文末导航后插入新行、撤销、重做、恢复原文、变窄、恢复宽度、变矮、
恢复高度。按键与指针走 Window.DispatchEvent，文本提交走 Window.CommitText。
每阶段核对整个模型与独立预期原文、语义可见摘录与模型切片一致，并检查独立视口
渲染有足够文字墨迹；原文件不改写。宽高按钮经普通命中触发尺寸变化，但变化是
相邻控件重新分配，未模拟窗口管理器连续拖边。

旧版 SourceView `f72e3b1` + GOUI `a618df5` 的临时 worktree 只复制同一验证程序与
benchmark；新版本为 SourceView `cde1243` + GOUI `4018c7a`，同机同配置运行，
旧／新交替，每文件各三次。30 次窗口运行、
450 个阶段均通过。计时不含文件下载与读取；load 包含模型 SetText 和其通知，
不包括创建 Application 和窗口。每次日志带输入 SHA-256。

下表是三个样本的中位数，单位 ms。action 是正常事件／调用返回时间；frame 是
frame 回调开始到最后一个探针 Widget.Paint 的观测最大值，包含软件绘制，不包含
完整桌面合成，也不是帧时间分位数。定时器投递间隔还包含测试自身像素扫描等开销。

| 输入 | 拖滚动条 action 旧→新 | 中部点击＋插入 action 旧→新 | 变窄阶段 frame 旧→新 |
| --- | ---: | ---: | ---: |
| TXT | 10.393→0.719 | 20.912→13.074 | 45.111→31.255 |
| JSON | 10.842→0.744 | 27.478→21.272 | 42.659→38.932 |
| XML | 10.182→0.793 | 32.546→30.226 | 48.001→31.515 |
| HTML | 13.619→0.758 | 26.009→20.190 | 46.657→36.581 |
| Markdown | 12.906→1.882 | 20.421→11.883 | 44.396→32.122 |

五种文件恢复最近宽度及恢复高度均未新增正文 native layout 或 MeasureMetrics。
新版本滚轮 action 三次中位数为 0.090–0.101 ms。不是所有阶段的 frame 观测值均改善，
例如 TXT 滚轮 43.701→47.166 ms、JSON 恢复宽度 23.760→26.840 ms；软件绘制、
调度和测试开销仍在，不能据 action 或模型查询数据宣称整体达到固定帧率。

JSON 的 2x 额外 15 阶段通过；其观测最大 frame 为 186.193 ms，不能将低 DPI 的
耗时外推为 2x 绘制表现。已有 resize_cache 在 1x／2x 均通过，宽度 632→512→632 DIP
的正文创建／度量导出为 2→3→3，恢复宽度及随后点击 byte 3 没有新创建或导出。
selection 的 Basic／Modern Light／Modern Dark × JSON／XML／HTML 九组在 1x／2x
均通过完整文字、透明选区、焦点、显式前景覆盖及资源释放断言。

selection 首次 2x 运行在主题切换后过早采样，观察到上一个阶段的显式粉色前景；
改为等待至少两帧且 `layoutPending=false`，保留原有全部颜色、像素和布局身份断言，
增加总超时后重新完整运行通过。没有扩大颜色误差或删掉失败断言。

## 综合场景发现的编辑失效问题

追加运行原有 `largefiles`，默认启用折叠，自动补全仍关闭。第一次 50 MiB JSON
在中部定位后过早检查可见范围；与 selection 相同，改为等至少两帧且没有待准备布局，
保留全文、可见范围和颜色断言。总步数仍为 53，每步仍最多等待 120 秒。

第二次外部总超时 180 秒先于整组结束；日志已通过中部／尾部语法色与后续编辑，
不能把该次中断算作通过。第三次允许足够总时长后完成 53 步，但保留了一个真实失败：
共享视图编辑后，选区正确而尾部 marker 移出可见范围，进程返回 1。

源码和独立回归确认未折叠候选的映射会重置所有高度，并导致 changed 再清空全部布局。
另外 LF splice 后旧 visibleLineMap 总行数会让 ensureHeights 再次重置已测高度。
修复 `d4bf6e7` 仅在存在隐藏行时重建折叠几何；普通编辑保留未变布局与 spliced 高度，
同步行数映射。新回归在 `cde1243` 上对普通插入与 LF 插入均明确失败，修复后通过；
检查 suffix 布局身份、屏外测量高度、最终 caret 可见性及任务收敛，不删除原有断言。

修复后 1 MiB JSON 的 53 步通过，50 MiB JSON 也完整通过；后续分析调度修正后的最终
JSON／XML／HTML 结果见下文。

## 分析中的重复偏移查询

`d4bf6e7` 在 50 MiB JSON 完成全部 53 步、零失败，但 50 MiB XML 在中部语法色阶段
超过原有 120 秒步超时；没有延长步超时或删掉颜色断言。追加等待日志观察待准备状态。
10 MiB XML 的同一完整程序通过，用于区分规模相关的推进速度与内容错误。

源码检查发现每个 token 的两个 UTF-8 边界校验重复求 lineStore.offset；offset 跳过
整块时也重复累加块内行。现在每个扫描片段保存一次 lineStart，整块跳转使用已有子树
byte 累计，只在最终命中块内扫描。没有改变 Scan 输入、预算、状态收敛或 UTF-8 校验。
增加独立逐字节行号／偏移回归，覆盖 4,096 行、多块及文档尾空行。

纯 Go 分析对照使用相同语料和 benchmark，旧版本为 `d4bf6e7` 加同一 benchmark，
每组一次完整建索引与初始着色、三次重复；文件读取和 Model 构建在计时之外，不创建
GUI，不包括帧调度或原生渲染。下列是中位数 ms；不是 UI 延迟或吞吐保证。

| 输入 | 原版 | 复用行首及累计值 |
| --- | ---: | ---: |
| TXT（仅元数据） | 19.130 | 18.661 |
| JSON | 3312.782 | 1040.423 |
| XML | 733.037 | 252.593 |
| HTML | 403.187 | 187.552 |
| Markdown | 99.519 | 40.035 |

```bash
GOUI_CORPUS_DIR=/path/to/corpus go test ./internal/highlight -run '^$' -bench '^BenchmarkCorpusAnalysis$' -benchtime=1x -count=3
```

未提供目录时该 benchmark 显式 skip，不隐式联网；常规回归全部不依赖语料目录。

## 空闲推进与最终复验

纯分析优化后，50 MiB XML 的中部语法检查在约 119 秒时通过，尾部仍超过 120 秒。
继续改用 GUI Timer，并观察到旧验证程序每 140 ms 轮询时主动 RequestPaint，使软件
绘制持续占用 GUI，影响本应空闲推进的场景。最终去掉等待函数中的强制绘制：控件
必须自己安排后续工作，验证程序只查询并保留全部断言。没有延长步超时。
增加 SourceView 错误信号失败检查、帧数和 SourceView 自建 Timer 回调计数，便于
区分无任务、布局就绪、待着色及实际错误；未把诊断日志当作通过依据。

实现 `f6b9a4d` 复用现有 frameQueue 的 GUI Timer，每 8 ms 推进约 2 ms 纯分析，
屏外结果变化不请求整窗绘制。帧回调继续负责 native 段落；最后卸载停止 Timer 并断开
信号，无待办停止，宿主不可启动时保留原帧后备。新逻辑回归检查屏外分析不要求绘制／
不创建 native 布局、可见 token 改变才失效、截止时不推进及 Timer 失败后不丢任务。

最终运行两仓版本为 SourceView `f6b9a4d`、GOUI `4018c7a`，验证工具随本记录提交。
相同环境、相同原有生成器，每语言 50 MiB，均完成 53 步、零失败、退出码 0：

| 输入 | bytes / 行数 | 首／中／尾语法色等待 | 共享编辑后可见性 |
| --- | --- | --- | --- |
| JSON | 52,428,615 / 257,006 | 1.138 / 4.061 / 4.322 s | 通过 |
| XML | 52,428,618 / 240,502 | 2.281 / 9.889 / 11.592 s | 通过 |
| HTML | 52,428,644 / 275,944 | 1.777 / 6.129 / 6.763 s | 通过 |

等待包含实际扫描、调度、计时器间隔与程序校验。调度实现和验证程序负载都作了调整，
不能将这些秒数与上述超时直接计算为纯性能倍数。查找 50 MiB 的计数仍约需 28 秒，
本轮未改变其调度；颜色允许稍后出现，内容、选择与输入不等待整份分析。

最终再跑五种原始下载文档及 54 MB 派生 JSON，各 15 阶段、全部通过（共 90 阶段）；
恢复宽度／高度均无新建正文布局或度量导出。该最终复验不是三轮性能统计的替代。
额外 JSON 2x 的 15 阶段通过。同步 SetText 的局限和 2x 软件绘制耗时仍保留，不承诺
任意文本／后端都满足固定帧率。

最终代码再次通过 1x／2x 的 resize_cache 和九组选区外观回归。额外以 `-race` 构建
XML 语料窗口程序，15 阶段通过、未报告 data race；这不检测原生库内部的数据竞争。
下载工具另验证实际下载 Markdown、既存文件哈希不符时报错、拒绝仓库内输出目录，
以及仅删除自己创建的临时下载文件。

## 验证状态和限制

- 两仓 `go test ./...`：通过。
- SourceView `go test -race ./...`；GOUI `go test -race ./gui/textedit ./gui`：通过。
- SourceView 受影响包 `go vet`：通过；全仓 `go vet ./...` 被用户原有未跟踪的
  `example/temp/main.go:58:2: unreachable code` 阻断，未修改该文件。
- 两仓受影响目标 Windows amd64、macOS arm64 交叉构建通过；未运行原生窗口。
- `GOWORK=off go test ./...`：失败，`updates to go.mod needed`。现有 v0.2.0 不含
  此前的 Rasterize／Fragment 契约，用户已有 replace 和 example/temp 保留不提交；
  工作区联调不等于已可独立发布。

首次 TXT 验证发现首尾 reveal 的邻域与实际视口漏测，导致回到文首后一直 pending；
已修正并增加独立回归。首次延迟准备还暴露高度重置使滚动条隐藏／出现、重复测中间
宽度，现保留旧高度估计，原 resize_cache 的 2→3→3 计数恢复。

4 ms／2 ms 仅为批次软预算。Model.SetText、单个 native 段落、编辑所需的准确
cluster、词法单步和软件栅格化仍可能超时；不保证任意无换行几十 MB 文本不卡顿。
此次实际原始输入最大约 13.5 MB，最长行约 16.9 KB，未验证 GB 文件或整份压缩为一行
的 JSON。通用段落内测量仍须解决已记录的 bidi／上下文反例，不能以独立截串替代。
未执行物理键盘、IME、Windows/macOS 原生运行及混合 DPI 拖动。桌面副作用仅为
自动打开、聚焦并关闭验证窗口；未自动写入剪贴板、设置或输入文件。
