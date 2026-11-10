# 内联建议第一版验收

2026-11-10；基线 `d78b84c`。设计与接口见 [内联建议](DesignInlineSuggestion.md)。

本记录保留第一版范围与验收；后续多行支持及扩大后的输入上限见 [多行扩展验收](PlanInlineMultiline.md)。

## 交付

默认关闭的宿主提供型建议：每个视图一个逻辑行末的单行字面量，最多 4096 UTF-8 字节。支持淡色预览、原生 cluster 边界截断、
Tab 接受、Esc 清除、一次编辑/撤销、普通补全和输入法互斥，以及宿主结果的文档/修订/光标校验。
`ui` 增加薄配置和类型别名；Basic / Modern 增加 inline-suggestion part。没有 AI 模型、网络请求或新语言解析。

预览不进入 Model、VisibleText 或 Preedit，不改变复制/导出/搜索/历史及其他共享视图。不改变正文高度与滚动范围。
当前只占一个显示行；多行、行中替换、流式更新与完整 bidi 注入留待后续。RTL 视觉内部的逻辑行末暂不显示独立追加预览，
不可见建议不能截获 Tab 或通过接受方法写入。正文变化清除旧建议，由宿主重新发布。

## 单元与原生排版

[`inline_suggestion_test.go`](../inline_suggestion_test.go) 与 [`ui` 配置回归](../ui/sourceview_test.go) 已通过：

- 默认关闭、非法单行文本/字节范围、旧修订、其他文档、非行末位置；错误替换不得破坏已有建议。
- 预览不更改模型/历史/导出/搜索/输入法语义；快照单独报告建议，Paint/Snapshot 不分配原生排版。
- Tab 精确接受、选区位置、单步撤销/重做，以及接受前普通输入的独立撤销边界。
- Esc、选区/导航、编辑/重置、文档替换、失焦、只读、禁用、配置关闭、普通补全及 IME 清理。
- CJK/组合字符/emoji 预览截断边界、调整宽度复用建议布局、主题变化准备新外观、视口外和 RTL 不可见位置不误接受。
- 共享文档的视图隔离、原生创建回调重入、原生失败返回及创建/销毁计数。
- 声明式默认关闭、显式开启、后续省略恢复默认。

| 命令 | 结果 |
| --- | --- |
| `go test ./...` | 通过 |
| `GOWORK=off go test ./...` | 通过，使用已发布 GOUI v0.7.0 |
| `GOWORK=off go test . ./ui -race -run 'TestInline' -count=1` | 通过 |
| Windows amd64 / macOS arm64，`CGO_ENABLED=0 GOWORK=off go build ./tests/window/editor/inline_suggestion` | 交叉编译通过，未做原生运行 |

## 本机独立会话

入口：[`tests/window/editor/inline_suggestion`](../tests/window/editor/inline_suggestion/main.go)。
Linux amd64、Go 1.25.8、Pango、私有 Xvnc 24-bit / Xfwm4 合成桌面，临时配置/认证/缓存/可执行文件在仓库外。
静态宿主结果经公共 setter 发布，真实按键和文字输入经 Window.DispatchEvent / CommitText；不使用语义动作捷径。

```sh
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/inline_suggestion
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=2 go run ./tests/window/editor/inline_suggestion -dark
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=opengl GOUI_PLAT_SCALE=1.5 go run ./tests/window/editor/inline_suggestion
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/inline_suggestion -basic
```

四组通过：Tab/Esc/撤销/重做、恢复普通 Tab 制表位、继续输入使旧建议失效、旧修订拒绝、只读/禁用/失焦、
CJK 精确接受、长建议省略与窄/宽面板、真实补全弹窗优先接受、卸载重挂载及拒绝旧文档结果。
转发 Painter 对每次实际建议绘制检查正文存在、淡色与一行裁剪，分别 64 / 49 / 66 / 65 次，共 244 次。

独立 X11 客户区按页脚帧号与 Paint 配对，检查四组各 12 个阶段，共 48 个预览图像；其中 44 个捕获到该阶段首个实际帧，
其余 4 个使用首个捕获帧，不将其报告为首帧像素通过。每个实际可见帧另有 Painter 断言。
四组 Esc 清除后的原预览区域均为 0 个文字像素，未留下残影。已人工核对亮色的实际截图。

图像使用合成后 8-bit RGB 转灰度；预览相对邻近空白正文灰度差 >35 为文字，需超过 10 像素。
一行预览下方直到视口底部不应有隐藏文字，亮色阈值 <161、暗色 >150；排除行号/边框/另一视图。
清除后的原建议区域排除原光标边缘，文字像素必须为 0。不使用跨字体金图或放宽抗锯齿容差。

附带原补全窗口回归：软件 1× Light 的 `-case navigation -auto` 与软件 2× Dark 的 `-case typing -auto -dark`，
覆盖异步/延迟/过期 provider、词内移动、弹窗复用、输入重过滤和接受。两组均通过。

## 验证中修正与记录

- 单元失焦用例最初使用新的 EventDispatcher 直接派发 Focused=false，未建立旧焦点路径，因而没有真实离开事件。
  改为同一个 dispatcher 先进入再离开；窗口另经 SetFocusedWidget 验证真实失焦。
- 首个窗口产物在第一帧编码日志时发生 SIGTRAP，两次启动均失败。反汇编显示该产物标准库 internal/abi.NoEscape 首字节为 00，
  已有正常产物为 RET（c3）；`GOWORK=off go build -a` 后恢复 RET，未改产品代码即可进入场景。异常字节来源尚未定位，
  不归因为 GOUI 缺口，也不将失败运行算作通过；后续增量构建与四组场景正常。
- 普通 Tab 用例最初假定插入 4 个空格；实际第 6 列到下个 4 列制表位应为 2 个空格，修正为精确预期，保留 Tab 回退断言。
- 首次附带补全回归误用默认交互模式，已主动终止所属测试进程/桌面；改为明确的 -auto 场景重新验证，不将交互启动算作通过。

## 边界与副作用

GOUI 没有完整注入文字/ViewZone 的接口组合，第一版绕过和后续限制记录在设计文档，没有修改 GOUI 或用户已有缺口草稿。
原生结论限所列 Linux 环境；Windows/macOS 仅交叉编译。IME 使用组件投影回归，未验收物理输入法；未测真实显示器 DPI 切换、
硬件 GPU、AI 网络客户端或性能基准。没有用户文件、剪贴板或设置变更，结束后清理所属桌面进程和认证文件。
