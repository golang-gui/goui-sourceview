# 长段落分配与宽度复用验证

2026-10-22。SourceView 从 `d4d606f` 到 `8aa754e`；GOUI 从 `9b4cbeb` 到
`6e4fc88`／`1eabb0b`。设计见 [Layout 第 14 节](DesignLayout.md#14-初始高度估计与局部编辑的滚动范围2026-10-22)，
完整数据和测量边界见 [GOUI 记录](../../goui/doc/PlanTextLayoutAllocation.md)。

## 本批变更

SourceView 在初始长单段文本上使用至多 512 byte 的原生样本估计滚动范围；局部编辑
以一段替换一段时保留先前高度作为暂定值，避免未测量期间滚动条消失。完整原生段落
的精确结果始终覆盖估计，样本不参与绘制或编辑坐标。本控件原有的近期宽度缓存不扩大。

GOUI 同期为 TextView 的长段落保留少量近期完整文本／宽度，与历史共享缓存预算；
恢复已测宽度时复用原生布局和普通值几何。本批没有新增公共 API 或 typography 策略。

**有界行组、跨宽度 advance 复用和局部编辑后缀收敛仍未完成；冷的新文本／新宽度
依然可能阻塞 UI。本批没有将同步完整调用换个调度位置宣称为分帧排版。**

## 验证

在独立、带认证的 X11 `:188`／Xfwm4 compositor 会话验证，1500×1100，Pango 1.50.6、
Cairo 1.16.0、software painter、缩放 1、Modern、WindowChromeIntegrated；XTest 输入，
XGetImage 观察，无 VNC 传输。用户既有桌面、其他 X11 会话未执行键鼠或剪贴板操作。

用户 `1.json` 为 157,862 byte 的单段文本。两仓前后各两轮完整真实编辑、首尾 64 KiB
粘贴／删除／历史及六项实际窗口拖动；另各两轮中部编辑和三次最大化／还原。所有完成
窗口均正常退出、全文恢复，原文件 SHA-256 未变。

TextView 首次完整 Widget 帧约 708–711 → 408–419 ms；恢复近期确切宽度的外部可见
等待约 428–1,043 → 79–84 ms。它们分别是布局到 Paint、键鼠发送到像素可见的不同
边界，不是文件加载时间或 API 基准。新字符仍约 450 ms，未改善；SourceView 未见
稳定的新输入收益，中部短词 input→Paint 约 776–789 → 792–844 ms，仍需继续优化。

失败项保留：TextView 两轮四项宽／角落拖动仍只有一个中间 viewport；SourceView
一轮全部通过，另一轮角落缩小只有一个。原断言要求至少两个，没有削弱断言或将最终
尺寸正确替代响应性。基线 SourceView 两轮通过，新增一次失败尚未归因，完整原生
折行仍过长。具体所有数据、GUI Timer 间隔及波动见 GOUI 记录。

既有公开语料另外各跑 TextView／SourceView 一轮中部真实编辑及两次最大化／还原：
128 KiB 长段 txt、2.7 MB XML、3.9 MB HTML、0.93 MB Markdown、3.3 MB 多行 txt，
十个场景全部通过并全文恢复。这些只有优化后验收，不能作为前后提升结论。
来源／许可沿用 [语料清单](../tests/window/editor/corpus/testdata/corpus.json)。

新增原生回归覆盖样本上限、重复测量、滚动条分配仅一次完整排版、局部编辑延后
测量前保留高度、全文删除后范围缩小；完整原生参考校验 RLO、Arabic、中文、Tab、
组合字符、emoji 和零 advance 文本。将用例复制到基线归档时能复现失败，修正后通过。

通过：两仓 `go test ./...`、GOUI `GOWORK=off go test ./...`、GOUI
`go test -race ./gui ./gui/textedit`、SourceView `go test -race .`。Windows amd64／
macOS arm64 的两个受影响测试包均交叉编译通过，本批未做这两平台的原生运行。

仍失败：SourceView `GOWORK=off go test ./...` 提示 `updates to go.mod needed`。
用户原有 `go.mod` replace 和 `example/temp` 均保留，不执行 tidy 或将 workspace
通过解释成独立发布验证通过。两仓其他未跟踪工作未纳入提交。

## 桌面复现入口

`tests/window/editor/interaction/desktop.py` 新增 `--width-cycles N`，使用 WM 正常的
Alt+F10 最大化／还原，校验 native 窗口、viewport、selection、caret 和长度恢复。
正常态和最大化态的 GUI resize inset 分别为 5／0 DIP，观察器现在识别两个实际
marker 原点，仍校验同一个 magic／header。原拖动断言及正常输入路径保留。

先在独立合成 X11 会话启动 interaction 示例，再在同一会话执行：

```bash
python3 tests/window/editor/interaction/desktop.py \
  --file /outside/document.json --kind text --output /outside/full-case
# 另启原始窗口，再单独观察近期宽度复用：
python3 tests/window/editor/interaction/desktop.py \
  --file /outside/document.json --kind text --output /outside/width-case \
  --middle-edit --width-cycles 3
```

`--kind source` 验证 SourceView；此外必须核对应用 exit 0 与 `verified`。产物在仓库外
`/tmp/goui-layout-next-20261022/`，包括未通过断言、最初环境／marker 配置失败的记录，
未混入通过场景。窗口均已退出；自建 X11／WM 结束后释放测试焦点、指针、剪贴板，
既有会话保留。没有推送远端。
