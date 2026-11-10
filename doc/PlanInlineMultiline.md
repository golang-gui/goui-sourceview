# 多行内联建议验收

2026-11-10；基线 `f9c3026`；GOUI v0.7.0、Go 1.25.8。设计与接口见 [内联建议](DesignInlineSuggestion.md)。

## 交付范围

沿用宿主发布接口，支持逻辑行末追加的多行字面量，输入最多 64 KiB / 256 个逻辑行，CRLF / CR 规范化为 LF。
首段接在光标后；原生排版得到的首段换行余文和显式后续行以完整正文宽度显示，实际高度把后续源行下移。
建议行没有行号，不进入 Model、保存、搜索、VisibleText、Preedit 或历史；共享文档的其他视图不增加空间。
Tab 接受整段作为一个撤销单位，Esc 清除并恢复布局；单行建议保留一行裁剪与省略策略。

支持调整宽度、换行、滚动至光标上方仅剩建议续行时接受、关闭换行时的水平范围，以及按下/停留/移动/松开鼠标的真实坐标命中。
编辑、外部修改、选区、重置、失焦、只读、禁用、IME、补全与折叠操作清理旧建议及额外高度。
默认仍关闭；没有内置 AI 网络、provider 调度、流式保留、行中替换或整段重写。

## 单元与原生排版

[`inline_multiline_test.go`](../inline_multiline_test.go) 与原单行 / ui 回归覆盖：

- 多行临时高度/水平宽度仅属当前视图，后续源行逻辑位置与命中不变，保存/搜索/历史/IME 无虚拟文本。
- Tab 精确插入 LF/Tab/CJK 字面量、选区末尾、单步撤销/重做；滚出原光标后仍可接受可见续行。
- Esc、选区、前后外部编辑、全文重置、替换建议、只读、IME 与结构折叠清除后恢复旧高度。
- CJK/组合字符首段的原生换行边界，窄/宽尺寸重排，连续 100 行制表符缩进，制表宽度改变替换布局。
- 非法 UTF-8/控制字符、首段 Tab、字节与行数上限；CRLF / CR 规范化和精确接受。
- 原生首段/后续段创建失败，创建/度量/尺寸阶段重入；回调更改换行配置不能被旧准备结果覆盖，创建/销毁计数一致，不遗留高度。
- Paint / Snapshot 不创建或度量建议原生布局；70 KiB 分页源行复用已缓存的行末几何。
- 正常 EventDispatcher 的按下、同位置移动、松开，以及 blink 选择更新不得在空间收回后重新命中其他源字符。

| 命令 | 结果 |
| --- | --- |
| `go test ./...` | 通过 |
| `GOWORK=off go test ./...` | 通过；独立使用已发布 GOUI v0.7.0 |
| `GOWORK=off go test . ./ui -race -run 'TestInline' -count=1` | 通过 |
| Windows amd64 / macOS arm64，`CGO_ENABLED=0 GOWORK=off go build ./tests/window/editor/inline_multiline` | 交叉编译通过；未做原生运行 |

## 本机独立会话

入口：[`tests/window/editor/inline_multiline`](../tests/window/editor/inline_multiline/main.go)。
Linux amd64、Pango、私有 Xvnc 24-bit / Xfwm4 合成桌面。认证、配置、缓存、可执行文件和截图放仓库外临时目录。
两视图共享源文档；宿主经公共 setter 发布固定结果，按键/鼠标通过 Window.DispatchEvent，文字通过 CommitText。
转发 Painter 仅观察实际绘制；鼠标阶段切换回真实 SourceView，避免包装对象改变事件树中的焦点身份。

```sh
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/inline_multiline
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=2 go run ./tests/window/editor/inline_multiline -dark
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=opengl GOUI_PLAT_SCALE=1.5 go run ./tests/window/editor/inline_multiline
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/inline_multiline -basic
```

四组均通过，分别观察 83 / 60 / 87 / 83 次实际建议绘制，共 313 次。
最终补强重入配置核对后，另跑软件亮色 1× 回归通过，观察 81 次实际绘制。
每个实际可见建议绘制检查原生淡色和正文视口裁剪；真实 TAIL 及其原行号下移且不与建议重叠。
窗口场景包括整段接受/撤销/重做、CJK 首段自动换行及调整面板宽度、仅续行在视口内时接受、
滚动范围恢复、下移源行及虚拟行的完整鼠标事件、普通输入、外部修订、失焦和不换行的横向范围恢复。

独立 X11 客户区截图按页脚帧号配对，四组各核对 11 个可见阶段，共 44 张：每个建议裁剪区域有实际文字像素，
原光标在视口内时保留真实前缀；Esc 后原建议最后一行区域无残留文字。人工核对了亮色普通/换行和 OpenGL 缩放截图。
采样使用合成后的 8-bit RGB，以邻近空白正文为基准，每通道最大差 >35 计作建议文字，各阶段须 >10 像素；
真实前缀采用差 >70，须 >10 像素；取消后避开边框和恢复的真实第二行，原建议末行区域须为 0 个文字像素。
截图使用各阶段已捕获的最后一帧，不声称做了首帧像素验收；每次实际绘制另有 Painter 断言，没有跨字体金图。

附带四组原生回归均通过：单行建议软件 Light 1× / Dark 2×（64 / 49 次实际预览绘制）、
原折叠预览软件 Light 1×（54 次实际单行预览绘制），以及补全 `-case navigation -auto` 软件 Light 1×。
覆盖补全列表优先、旧结果/生命周期、原单行省略、统一折叠入口和长行 1/2/3 显示行门槛。

## 验证中修正

- 首轮鼠标观察器使用嵌入 SourceView 的包装对象，正常事件树聚焦包装对象时触发真实视图失焦，提前清除了建议。
  将鼠标测试切回真实控件。该现象属于测试装置的对象身份影响，不归为 GOUI 缺口。
- 在真实控件上另行复现：虚拟行按下映射真实插入点，松开却按收回后的布局二次命中，产生非空选区。
  现在记住初次命中，指针未移动时保留位置；同位置 motion、blink 和 release 都不能重新映射，实际移动继续拖选。
- 原生多行缓存额外纳入 TabWidth，避免配置改变后保留旧制表位；每次原生创建/度量后核对布局代次、字体、换行、宽度和颜色，
  防止旧配置覆盖回调重入产生的新布局；所有度量完成后再核对状态，防止尾段尺寸回调重入泄漏。

## 能力边界与副作用

GOUI 没有成套 injected-text / ViewZone；本仓使用视图局部高度绕过，没有修改 GOUI 或占用输入法投影。
TextFormat 没有制表位起点，首段 Tab 暂拒绝，宿主可用空格；后续行支持原生 Tab。缺口与取舍已记入设计文档。
行中替换、完整 bidi 注入和真实 AI 网络请求仍不在本次范围。

原生结论仅限所列 Linux 环境；Windows / macOS 仅交叉编译。物理输入法、硬件 GPU、真实显示器 DPI 切换和性能基准未执行。
不修改用户桌面、文件、剪贴板或设置；所属私有桌面结束后清理进程与认证文件。
原有未提交设计草稿与 example/temp 保留，不纳入本次提交。
