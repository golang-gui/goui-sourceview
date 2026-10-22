# 长段落字形来源接入验证

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
