# 长段落字形来源接入验证

> 2026-10-22：以下 retained-glyph 实现和性能数字属于历史提交，该路径已经撤回。
> 当前使用既有 TextLayout；撤回范围及本次验证见文末。

2026-10-22。设计见 [DesignLayout 第 15 节](DesignLayout.md#15-完整上下文字形的折行投影)。
SourceView 对至少 32 KiB 的自动换行段落，复用 GOUI 的完整原生字形及普通 advance
投影，按可见显示行绘制。近期宽度共享本视图的不可变来源；不同视图独立拥有。
高亮／选区前景在普通值中组合，不再对完整原生段落反复 SetTextColor。

GOUI 前置包含新的 `typography.GlyphContext/GlyphLayout/GlyphMetrics`、
`graphics/gui.Painter.DrawTextGlyphs` 和 `gui/textedit.ShapedText/WrappedText`。
当前仅 Pango 提供原生字形来源；其他后端继续原有准确路径。本轮联调的 GOUI
版本为 `c9e07f3ef9380a49818e334b69eac12958aac805`（包含本批四项功能提交）。
尚不能把 `go.mod` 的旧发布版本视为已包含这些能力。

## 结果

同一独立 X11（Ubuntu 22.04.5、Xvnc、xfwm4 compositor on、Software、1×），
同一 157862 byte 单逻辑段落 JSON，两轮修改前／两轮最终修改后真实桌面操作：

| 场景 | 修改前 | 修改后 |
|---|---:|---:|
| 中部新字符，change→Paint | 614–752 ms | 248–250 ms |
| 中部短词粘贴，input→Paint | 422–782 ms | 280 ms |
| 陌生宽度缩窄，释放后桌面等待 | 647–669 ms | 84–89 ms |
| 宽度／四角拖动中的中间尺寸帧数 | 2 | 8–10 |
| 整次流程最大 GUI Timer 间隔 | 571–581 ms | 188 ms |

最终两个完整流程通过，覆盖头中尾插入、替换、选中删除、撤销重做、64 KiB 粘贴及
六种真实边缘／四角拖动；退出时整个模型与原文相等。原有至少两个中间画面的拖动
断言未改。另通过 JSON、持久 RLO 混合 Unicode 长段落的中部编辑及各两轮最大化／
还原，以及五份 TXT/XML/HTML/Markdown 语料的编辑／宽度切换，共 10 次两控件语料流程。

新增 `--rtl-insertion` 仅用于明确位于 RTL run 的中部插入场景：用真实 Right 键跨回
刚插入的字符，保留精确 byte/caret、Delete 和全文恢复断言。默认 LTR 行为及断言
保持原样。首次误用 LTR driver 的 RTL 超时未计为通过；正常 Undo／退出后全文校验
通过，随后以正确视觉方向的场景，两控件均完整通过。

原生字形范围、bidi 顺序、行尾空格、字素／连字、生命周期和图像缓存的独立断言，
以及 Software／真正 OpenGL 四档缩放的逐像素结果，详见
[GOUI 本批验证](../../goui/doc/PlanShapedWrapping.md)。其中首次完整帧不是文字着色完成
或显示器呈现总时间；包测试、窗口输入和跨平台编译分别记录，未互相替代。

通过：`go test ./...`、`go test -race .`；Windows/amd64、macOS/arm64 的本包、ui、
interaction 编译通过，未做本批原生桌面复测。`GOWORK=off go test ./...` 仍失败：
`updates to go.mod needed`。已有本地 go.mod 改动及 `example/temp` 未纳入本次修改／提交。

诊断包装器显式转发可选 GlyphContext／GlyphLayout，防止启用 profiling 后悄悄
回到旧路径。最终二进制启用 `-cpu-profile` 的真实中部编辑和宽度切换通过、全文
恢复通过，并收到 `glyph-metrics` 诊断事件；这次运行排除在性能表之外。

首个完整塑形与索引仍有同步工作，新文本编辑仍可能超过 100 ms；原生后端未提供
字形来源、或含强制分隔符的段落保持原路径。尚未完成冷索引分批发布、局部编辑后缀
收敛、超大单字素及单行零 advance 几何的工作量上限，不能据本批结果宣称 UI 已完全不卡。


## 收敛到现有 TextLayout（2026-10-22）

移除 SourceView 中的独立字形来源、跨宽度字形投影和专用前景绘制分支；创建、
测量、范围颜色和绘制均使用现有 Context／TextLayout／Painter。诊断程序移除
可选字形能力转发，保留原生调用计时、真实桌面输入、RTL 插入和最终全文一致性
验证。保留最近宽度、编辑历史、滚动估计、普通值片段与详细几何分页。

专用字形来源共享测试随被撤回协议移除，补充 >32 KiB 复杂文本的普通接口回归：
完整原生参考、近期宽度与高度变化不新增测量、颜色经现有接口更新，以及快照在
原生缓存销毁后仍然有效。已有普通 TextLayout 的像素、复杂文本、区域绘制和
缓存测试不削弱断言。

当前冷排版仍是整段同步调用，本次不宣称性能提升。前后真实操作与测试结果见下文；用户已有 go.mod 和 example/temp 不纳入本次变更。


GUI 观察程序同步识别 GOUI 普通文本的 goui.text.layoutPending 属性：TextView
合并连续分配期间的旧呈现不能算最终文本就绪。仍使用原有真实输入、拖动响应与
最终全文一致性断言；新增上层合并属于 GOUI，SourceView 沿用现有帧工作路径。


### 本次验证结果

同机工作区联调，起点 GOUI c9e07f3、SourceView c6b43ca；当前移除字形协议，
GOUI 另合并 TextView 的连续分配。最终 GOUI 为 `f9304e7`，接口移除为
`5bc83c6`；两项按功能分别提交。Ubuntu 22.04.5、Pango 1.50.6／Cairo 1.16.0，
独立 Xvnc :191、xfwm4 4.16.1 compositor、Software 1×、WindowChromeIntegrated。
用户 JSON 为只读 157862 byte，另用 58519 byte 的跨段 RLO／混合文字样本。

最终 JSON 两控件各两轮、RTL 各一轮，全部 driver=0／app=0／verified=true。
通过真实滚动、点击、插入、替换、删除、约 64 KiB 粘贴、撤销重做、宽高及边角拖动、
窗口大小恢复和关闭时全文一致性检查；未绕过事件分发。观察标记必须非 pending，
不将 TextView 暂留的旧画面当作新宽度已经排完。最终截图已查看。

性能没有保持字形实验的全部收益：SourceView 中部插入由 278.5–299.4 ms 变为
647.9–664.1 ms，TextView 为 199.5–214.3 ms → 465.1–480.7 ms（真实输入至
非 pending 桌面标记，各两轮，不含首次加载）。SourceView 的拖窄仍有约 366 ms
视区帧间隔；TextView 在上层合并后拖动期间有 12／12 个中间帧，但释放至新布局
完成仍需 579.4–591.5 ms。不能将通过响应断言说成已消除窗口阻塞；包含最终重排的 GUI Timer 最大
间隔仍为 TextView 443.2–459.3 ms、SourceView 392.8–400.5 ms。

- 工作区 `go test ./...`、`go test -race .` 通过；GOUI 全量与 gui／textedit race 通过。
- 两仓 Windows amd64／macOS arm64 受影响包仅编译通过；未做本批远端原生验收。
- `GOWORK=off go test ./...` 仍失败：`updates to go.mod needed; go mod tidy`。
  此项前批已有记录，未改动用户 go.mod 的 replace 或补作依赖发布。
- 两仓 `git diff --check` 通过；GOUI 既有 15 项手动／平台测试跳过，未计为通过。
- 保留直接撤回时 TextView 拖动检查的两次失败，以及 80 ms 合并仍有长帧间隔的
  中间结果；未降低断言。最终 150 ms 参数只合并分配，不限制单次原生调用。

原始日志、二进制、截图和逐项前后数据在
`/tmp/goui-textlayout-contract-20261022/`，详细测量边界见
[GOUI 验证记录](../../goui/doc/PlanShapedWrapping.md)。独立 X11 会话在验收结束后关闭；
窗口、指针和剪贴板的操作仅发生于该会话。未提交用户 go.mod、example/temp 或测试产物。
