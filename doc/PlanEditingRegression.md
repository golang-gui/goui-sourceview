# 全编辑功能回归记录（2026-10-24）

## 环境与范围

- Linux amd64，Go 1.25.8，独立 X11／Xvnc／Xfwm4 合成桌面；主矩阵为
  Software／Pango，应用缩放 100%，重点交互另覆盖 150%／200%；OpenGL／Pango
  在 150% 下补充 7 组重点验证，实际 imageResource 与日志确认采用 OpenGL。
- SourceView 基线 `094d46b`；光标显露修复 `3b9a9c6`，输入顺序修复
  `074deb5`，进入坐标修复 `c1632fa`。GOUI 工作区提交
  `5734836`，本轮没有在该提交之后继续修改 GOUI。
- 单元测试直接断言模型、选区、历史、缓存及生命周期；窗口程序通过
  Window／Popover.DispatchEvent 与 IMContext 的正常入口操作，不用 DevServer。
- 原始问题文件为用户提供的 184,500,131-byte 单行 JSON（约 176 MiB），
  仅读取，不保存回文件。10 MiB 的 JSON／XML／HTML 使用既有独立生成器，
  正常退出删除样本。没有把私有样本或测试产物加入仓库。

## 缺陷与修复

1. End 后不可见光标：局部字形位置正确，但全局 float32 内容范围与显露矩形
   丢失边距。SourceView 保守表示目标边界，GOUI ScrollView 高精度计算末端后
   向保证可见的方向转换。确定性 10-DIP 字符回归及真实文件光标像素断言覆盖。
2. 冷 End 后立即 Left／输入：目标 GeometryAt 尚不可用，旧路径以视口缓存
   导航，实际落到第 4095 字节。正常输入改为按序等待目标实测停靠点，继续使用
   既有帧预算。补充组合字符、预编辑、边界、替换、删除、历史、Tab／换行、
   粘贴及上下文取消回归。对当前代码用 Go overlay 关闭按键延迟入口后，
   `TestDeferredNavigationTextAndHistoryStayOrdered` 明确失败为 `{4095 4095}`；
   该反例不是完整旧版本的性能对照。
3. FindBar 关闭后鼠标样式错误：进入事件没有更新悬停坐标，仍使用上次侧栏的
   位置。现在先读取 crossing 的局部坐标，再让共享 MotionEventController 处理。
   新单元回归及现有 find 窗口断言均验证正文／侧栏样式。
4. 基础 editor 配色检查的时序过时：原测试在同一 timer 回调连续改色并立即
   读取原生位图，未让已存在的按帧着色完成。改为每轮等待两帧，仍保留全部
   20 轮准确像素比较、原生布局身份及创建次数断言。没有修改颜色容差。

## 包内与构建验证

| 命令 | 结果 |
| --- | --- |
| `go test -json ./... -count=1` | 506 个含子用例的 PASS，0 FAIL，0 测试 SKIP；没有测试的程序包不计作测试通过 |
| `go test -race ./... -count=1` | 通过 |
| `go vet ./...`、`git diff --check` | 通过 |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...` | 编译通过，未原生运行 |
| `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` | 编译通过，未原生运行 |
| `GOWORK=off go test ./...` | **失败**：已存在的 GOUI v0.3.0 缺少 ParagraphLayout／ParagraphGeometry、HeightIndex.Top64、Projection.Slice；工作区通过不等于独立依赖通过 |

包内覆盖包括文档与视图隔离、LF／CRLF 输出、普通输入／IME 生命周期、
指针与导航、历史与撤销分组、编辑计划、搜索边界／正则、补全会话、配对、
折叠、空白标记、声明式协调、着色与缓存资源失效，以及所有内置语言规则。

## 原生窗口矩阵

首轮 28 组有 2 组失败（配色检查时序、find 光标样式），两项分别按上述原因
修正。第二轮 28 组、扩展 40 组、OpenGL 7 组全部通过，共 75 组（含 8 组
括号 setup 场景）。各行只说明实际断言
覆盖，不把窗口出现或自动关闭当作视觉质量全面验收。

| 领域 | 程序与场景 | 已验证组合 |
| --- | --- | --- |
| 输入、Backspace／Delete、撤销重做、缩进、Tab／Escape、只读、共享文档／重挂载、快照与光标显露 | `editor -auto -clipboard` | Software，100%／150%／200%，通过 |
| 双击词／三击行、拖选、部分 cluster 配色、右键菜单键鼠执行、补全接收与取消、过期异步结果、配对编辑及原子历史 | `experience` | Software，100%，通过 |
| 文档／词／显示行导航、选区扩展、删除词、注释、括号跳转、行插入与选择、GoTo | `shortcuts` | Software，100%／150%／200%，通过 |
| 移动／复制／删除行、共享选区映射、只读边界、菜单键鼠执行 | `lines` | Software，100%，通过 |
| Find／Replace／GoTo、焦点、关闭清理、布局、滚动、无选区按钮禁用 | `find`、`find_appearance` | find 100%／150%／200%，外观程序 100%，通过 |
| 多行查询、正则捕获替换、错误语法、零宽匹配、选区内查找切换与计数／高亮同步 | `search` | Modern Light／Dark，100%，通过 |
| 折叠、隐藏范围、搜索／历史与菜单状态 | `folding` | Light／Dark，100%，通过 |
| 智能 Home／End／退格、缩进／空白／选中词辅助 | `assistance` | Light／Dark，100%，通过；更细空白字符规则另由包内断言覆盖 |
| 用户自定义语言／高亮／配对／补全，内置 TOML／YAML／Shell，Markdown 与标签闭合辅助 | `extensions`、`languages`、`markup` | Light／Dark，100%，通过 |
| 快捷键替换／禁用、修饰键匹配 | `keybindings` | Light／Dark，100%，通过 |
| JSON／HTML／XML、长项目、Unicode、换行／边缘补全 | `completion -case ... -auto` | Software，100%，七组通过；navigation／scroll 100%／150%／200% 通过，含真实滚轮与滚动条拖动 |
| 选区与语法着色叠加、亮暗／Basic 外观 | `selection` | JSON／XML／HTML × Basic／Light／Dark 九组合，100%，通过 |
| 菜单与配套栏多语言 | `localization` | 八种语言，100%，通过 |
| 宽度缓存与资源复用 | `resize_cache` | Software／Pango，100%，通过 |
| 边框、焦点、滚轮、两方向滑块拖动、禁用、换行、内容收缩、主题失效 | `frame` | Basic／Light／Dark 100%，Light 150%／200%，通过 |
| 括号装饰场景准备 | `brackets -case ... -auto` | nested／adjacent／multiline／wrap／search／unmatched／selected／unfocused，100%，八组 setup 断言通过；不等于逐像素外观验收 |
| 中大多行文件的真实加载、编辑、Unicode、历史、搜索替换、共享视图与重挂载 | `largefiles -language ... -mib 10` | JSON／XML／HTML 各一组，100%，通过；操作耗时包含断言，不作性能结论 |
| 原始 176 MiB 单行文件的冷 End／Left／输入、替换、双向删除、换行、只读、剪切粘贴及历史；最后全文恢复 | `paragraph -verify-input [-nowrap] -clipboard` | 100%／150%／200% × 自动换行／不换行，六组通过 |
| End 与三次 Left／Right 光标位置及像素，正常／最大化／恢复窗口 | `paragraph -verify-end -nowrap` | Software 100%／150%／200%；OpenGL 150%，通过；每次正常／最大化／恢复共 21 步 |
| 160 KiB 单行、离屏内容与原生光栅边界 | `longlines [-wrap]` | Software 100%，两组通过；明确限制原生 raster 区域并检查非空像素 |

### 实际桌面输入

另在同一独立显示中用既有 `interaction/desktop.py --middle-edit`，通过 XTest
执行真实桌面滚动条点击、正文点击、系统剪贴板粘贴、选词替换、普通键盘输入、
退格／Delete、撤销重做与返回文首。Software／Pango，100%，原始 176 MiB 文件，
16 条桌面操作记录无失败。驱动退出 0，窗口正常关闭退出 0，应用的 `verified`
记录确认全文等于原始输入；日志中的修订为 11。此场景只测中部编辑，未宣称
做了 head／tail 或宽高拖动，后者由前述窗口矩阵的对应断言覆盖。

该驱动只发普通桌面键鼠与剪贴板输入，读取被动绘制的状态像素；没有新增接口，
没有直接修改模型。包内、正常事件分发、原生离屏像素及桌面呈现分别保留各自边界。

## 限制与副作用

- 不声明遍历了所有功能、语言、主题与缩放的笛卡尔积。窗口矩阵主要验证行为与
  既有颜色／几何断言，人工观感、真实输入法候选窗、死键／AltGr、无障碍系统集成、
  系统保留快捷键、混合显示器 DPI 切换及远端 Windows／macOS 原生行为未在本轮验收。
- 自动预编辑用例验证投影与提交生命周期，不能代替实际中文输入法操作。
- 全文比较、文件读取和生成样本断言本身可能耗时；本轮是功能回归，没有据此声称
  大文件所有操作均满足严格延迟上限。任意复杂 Unicode 段落的既有限制仍保留。
- 仅独立测试显示的窗口、指针、焦点与剪贴板发生变化；剪贴板用例覆盖该显示内容，
  不在用户工作桌面执行。不修改系统缩放、输入文件或外部账户，不推送提交。
- 完整命令输出、失败反例与原生运行记录保存在仓库外临时验证目录，未纳入源码。


## 反馈补充：行尾鼠标拖选与短行横滚

### 已复现并修复：短行向外拖选横滚

实现提交：`69c4145`。

- 原因为拖选直接将越界鼠标 X 用作显露目标，没有按当前显示行的实际停靠点
  判断。宽首行撑大总滚动范围时，短次行已经到尾仍会请求横向滚动。
- 改为命中和目标矩形共用同一实测 Geometry，水平推进以仍不可见的命中停靠点
  为条件；未滚动的轴保持当前偏移。垂直滚动保持独立，并检查信号回调重入。
  不修改 GOUI、typography、公共 API 或手动滚动条的范围。
- 原生 `frame` 增加跨帧持有拖选：宽首行加短次行，短行越界后偏移持续为零，
  长行越界仍增加偏移。用 Go overlay 恢复修复前 `input.go`／`layout.go` 时，
  同一窗口明确失败 `short second-line drag scrolled horizontally`。
- 修改后 Software／Pango × 100%／150%／200% × Basic／Modern，共六组通过；
  OpenGL／Pango 150% 也通过。`experience` 三种缩放以及 `lines` 100% 通过，
  保留双／三击、拖选、菜单、行编辑及历史覆盖。
- `go test -json ./... -count=1` 516 个含子用例 PASS、0 FAIL、0 测试 SKIP；
  `go test -race ./... -count=1`、`go vet ./...`、两目标交叉编译均通过。
  新包内断言包括短行终点、长行、左侧被遮挡的短行、侧栏拖选、换行、垂直滚动
  与巨大水平偏移、滚动回调换文档。独立发布依赖的既有失败继续保留。

### 尚未复现：End 后无法鼠标选中最后一个字符

- 新增 `paragraph -verify-pointer -nowrap -width ...`，直接从冷 End 的真实光标
  像素反向拖到前一停靠点，再正向拖回；要求精确选中 `[Len-1, Len)` 并存在
  实际选区底色。正反两次拖选之间等待双击时间窗，防止多击辅助掩盖普通拖选。
- 修复前生产代码在 480／640／800／1024-DIP 宽度 × 100%／150%／200% 的原文件
  检查均没有复现。200%／1024 首次因本轮另一测试窗口抢焦点而超时；独占焦点
  重跑通过，该超时保留日志，不计作产品缺陷或一次通过。
- 最终普通正反向拖选程序在 640-DIP 宽度、三种缩放下分别验证修复前／后代码；
  两者均通过。这些结果表明当前用例尚未覆盖用户报告的触发条件，不能称已修复。
- 在独立显示直接运行未改动的 `example/temp`，通过原生文件对话框打开原文件，
  100%／Software／Pango、800×600 原生窗口下，用 XTest 按 End、拖选末尾并复制。
  实际选区像素可见，剪贴板恰为文件最后一个字节；未写回文件。该例程是未跟踪
  用户文件，只构建验证，不修改或提交。
- 已询问用户区分拖选范围、点击停靠点或选区底色的表现；在获得更具体触发条件前
  保留待定位状态，不用代码猜测或调整字符边界掩盖尚未复现的问题。


### 用户补充顺序：冷 End 后首次正向拖选

用户明确从左向右选取末尾多个字符后，新用例 `paragraph -verify-forward -nowrap`
按该顺序逐帧派发 PointerDown／带左键状态的 PointerMove／PointerUp；初次拖选前
不使用 Left 或反向选取。它验证水平偏移不变、锚点固定、长度不变及最终到达 EOF，
并对照 End 时的真实光标像素，避免只检查滚动值而漏掉局部几何移动。

本轮使用未再修改的库代码：SourceView `458e9e3`、工作区 GOUI `5734836`。
184,500,131-byte 原文件的 Software／Pango × 100%／150%／200% ×
480／640／1024／1280-DIP 窗口共 12 组未复现；800-DIP／100% 单独验证亦未复现。
加入最终光标像素断言后，OpenGL／Pango 的 100%／800、150%／480 和 800、
200%／640 和 1024 五组也通过。

另对 `example/temp` 使用不落入仓库的 Go overlay，只添加读取公共状态的信号／
定时观察，不改变其窗口构造、样式、文档、输入或布局。通过原生文件对话框打开
原文件，在 OpenGL／Pango、100%、800×600 下用 XTest 冷 End，再首次正向拖选
约 50 个末尾字符；偏移持续不变，复制结果精确等于原文件后缀，包含 EOF 字符。
原例程文件未修改，选择器运行在测试专用 XDG 配置／缓存／数据目录。

观测到 End 自身有两次偏移调整；还尝试在首次非零偏移日志出现后立即发送拖选。
实际鼠标按下被处理时已完成第二次调整，该次仍选中至 EOF。因此这次检查**没有**
证明覆盖了两次调整之间开始拖动的时序，也不能排除未完成的显露请求干扰选择。

本轮仅新增回归，没有已确认的生产修复；问题仍待定位。已询问系统、缩放和绘图
后端，以对齐运行环境；不将多组未复现结果改写成用户报告错误或问题已经解决。


## 截图定位：GOUI TextView 的拖选偏移回退（2026-10-25）

用户截图显示无行号、无语法色的普通文本。核对两个仓库的同名 `example/temp`：
GOUI 版本是 `ui.TextView().WrapMode(ui.WrapNone)`，扩展版本是 `ui.SourceView()`。
此前 SourceView 的正反向／正向拖选通过没有覆盖 TextView 的私有编辑器路径。
本轮在 GOUI 例程复现了与截图一致的 64-DIP 位移，故不再将此反馈记为未复现。

直接 XTest：打开原 184,500,131-byte 文件，End 到末尾，首次由左至右拖选。
修复前水平偏移从 `1005022784` 退到 `1005022720`，最终停在 EOF 前四字节；
包内数学反例及正常 Window.DispatchEvent 原生程序均确认该问题。修复后相同
实际操作偏移保持不变，复制的 18 字节精确等于文件后缀，包含最后一个字符。

本轮 GUI 修复提交为 `5efc894`、`2e3633f`；本仓按 go.work 联调，go.mod 未改变。

问题位于 GOUI textEditor：自动拖选用 float32 内容减视口重新计算上限，与
ScrollView 的 float64 差值、向远端转换规则不同，鼠标仍在正文内部也启动定时器
并拉回偏移。框架现统一范围计算，未越界的轴保持偏移，越界步进按方向转换，
避免小步进在大坐标下丢失。TextInput 共享计算，未增加公开 API 或改动 typography。
框架另外补齐 TextView 的当前短行视觉端点判断；SourceView 原有修复保留。

验证环境为 Linux amd64／Pango，独立 Xvnc／Xfwm4。最终结果：

- 原文件 TextView 末尾拖选：Software／OpenGL × 100%／150%／200% ×
  640／800-DIP 窗口，共 12 组通过；原有 32 KiB 换行 caret 回归两后端 150% 通过。
- 组合窗口 `editor/frame` 同时覆盖 TextView 和 SourceView：两后端 × 三种缩放 ×
  Basic／Modern Light，共 12 组通过。最初追加的 TextView 场景误用了固定 30-DIP
  纵坐标，点中了第一行；改为按两行 ASCII 实际内容高度定位第二行中心，保留全部
  模型范围和滚动断言，重跑通过。该测试配置失败未计作产品通过。
- GOUI 全仓 2792 个含子用例 PASS，15 个手工／平台环境测试 SKIP，0 FAIL；
  SourceView 全仓 516 PASS、0 SKIP、0 FAIL。GOUI `gui/ui/gui/textedit` race 通过。
- 普通 GOUI `go vet ./...` 有 29 条既有未命名字段告警；用修复前生产文件 overlay
  验证同样 29 条、无新增。`go vet -composites=false ./...` 通过。
- 最终 26 组原生组合全部通过，窗口内探针确认实际为 `software.imageResource`／
  `opengl.imageResource`，以及各自 1／1.5／2 缩放；未把请求后端当作实际后端。
- 两仓 Windows amd64／macOS arm64、CGO=0 全仓编译通过，未原生运行。
  SourceView 关闭 go.work 后的既有未发布段落 API 缺失仍保留，go.mod 未改。

测试不保存原文件，不修改／提交两仓未跟踪的 temp 例程。源文件没有因为测试生成
图片或可执行文件；GOUI 既有测试图片的前后哈希一致。原生截图、反例和命令输出
留在仓库外临时目录。GUI 通用修复在 GOUI 提交，本仓只增加组合回归与验收记录。
