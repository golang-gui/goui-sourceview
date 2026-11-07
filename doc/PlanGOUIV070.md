# GOUI v0.7.0 升级验收（2026-11-07）

## 范围与版本

- SourceView 基线 `1d011a3`；仅修改本仓库，不改相邻 GOUI 或 MarkDoc。
- GOUI 从 v0.6.0 升至正式发布的 v0.7.0，标签提交
  `33ce8a9e02f57364f2f18aaed1b36d4ca32ef127`。
  参考 [官方发布说明](https://github.com/golang-gui/goui/releases/tag/v0.7.0)，
  接入设计见 [第 11 节](DesignGOUIIntegration.md#11-goui-v0702026-11-07)。
- 保持 Go 1.24。随 GOUI 模块要求更新 purego 0.10.0 → 0.10.2、
  goexlib/cgo 0.1.1 → 0.1.2、nanovgo 0.0.10 → 0.0.13；其余库保持原版本。
- 所有构建和验证显式使用 `GOWORK=off`，无 replace、go.work 或全局 Go 配置修改。
  中英文 README 区分开发分支与已发布 SourceView v0.3.0，后者仍依赖 GOUI v0.6.0。
- 保留已有 DesignCapabilityGaps、DesignCompletion、Roadmap 修改，以及未跟踪的
  DesignSavePoint、DesignSnippets、DesignViewState、Doing 和 example/temp。
  升级改动单独提交，不推送或发布。

## 实施与回归

- 样式读取和可比较缓存保存完整 CornerRadius；每角负值/NaN/Inf 在缓存前归零。
  单角变化且最大圆角不变时，仍会关闭旧补全会话并释放原生布局。
- 查找表面/输入框使用公共 `PaintStyledBox`；保留 BorderSides 的缺省全边、
  单边和显式空边框语义。阴影独立绘制并携带同一四角值。
- 补全选中行使用公共 `PaintStyledBackground`，保留完整行绘制后裁剪、左右 2 DIP、
  上下 1 DIP 留白、完整行命中和原文字位置。实际圆角缩放交给 GOUI。
  外壳由 Popover 绘制；最大圆角仅用于安全留白。
- 保留原先等圆角、窄视口、非法半径、部分行裁剪和缓存断言，新增非对称轮廓、
  单角失效、独立阴影轮廓、单边/空边框及无效圆角可比较性回归。
- completion 窗口新增 `-corners -case json -auto`，以原生 2× RenderWidget
  验证两个圆角和两个直角的内容像素；采样避开文字与抗锯齿，NRGBA 零误差。
  Popover 外壳与内容绘制分离，圆角处内容应透明，直角处为指定不透明绿色。

## 命令结果

环境：Linux amd64、Go 1.25.8、Pango；运行时直接使用发布模块。

| 检查 | 结果 |
| --- | --- |
| `GOWORK=off go test ./... -count=1` | 通过，包括全部示例/窗口程序编译；无显示连接需求的测试正常执行 |
| `GOWORK=off go test -race ./...` | 通过 |
| `GOWORK=off go mod verify` | 通过，全部模块已验证 |
| `GOWORK=off go mod tidy -diff` | 通过，无差异 |
| `git diff --check` | 通过 |
| `GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...` | 交叉编译通过 |
| `GOWORK=off GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` | 交叉编译通过 |
| `GOWORK=off go vet ./...` | 失败：editing_test.go / folding_test.go 共 28 条 unkeyed fields 诊断；与升级前 HEAD / GOUI v0.6.0 逐行一致，无新增 |
| `GOWORK=off go vet -composites=false ./...` | 其余检查通过；不将此结果代替完整 vet 通过 |

vet 对照使用仓库外的 `git archive HEAD` 副本，未改当前或相邻仓库以构造基线。
既有诊断保持记录，本轮不顺带重写缩进/折叠测试。

## 独立桌面

使用本轮专属 Xvnc 24-bit / Xfwm4 compositor 会话及临时 XDG 目录。
各用例完成后自动退出，最终关闭本轮显示服务和窗口管理器。
实际后端由窗口日志中的 Software / OpenGL imageResource 核对；
OpenGL 通过不表示已验收硬件 GPU。

20 组不同场景全部通过；重跑的三组 Software 外框不重复计数：

| 场景 | 后端与缩放 | 断言范围 |
| --- | --- | --- |
| completion 非对称角 | Software Light 1.5× / Dark 2×；OpenGL Dark 1.5× | 四角实际内容像素、焦点、输入、接受与撤销 |
| completion Basic | Software 1× | 原有等圆角、交互与完整行命中 |
| completion scroll | Software 1.5×；OpenGL 1× | 像素/行滚轮、真实滚动条拖动、末项、键盘显露与稳定宽度 |
| completion navigation | Software 1× | 光标移动、复用弹窗、过滤、选区、嵌入语言、延迟/过期 provider 和回调重入 |
| frame | Software Basic 1× / Light 1.5× / Dark 2×；OpenGL Light 1.5× / Dark 2× | 官方外框颜色、焦点/悬停、视口稳定、双轴拖动、禁用、换行、内容缩短、主题失效及长短行拖选 |
| find_appearance | Software 1×；OpenGL 2× | 亮暗并列、独立样式、焦点/命中、窄宽恢复、独立多行输入与滚动/缩回 |
| selection | Software 2×；OpenGL 1.5× | 每组都覆盖 JSON/XML/HTML × Basic/Modern Light/Modern Dark；选区像素、语法保色、缓存和菜单 |
| highlight | Software Light 1×；OpenGL Dark 1.5× | 连续输入/删除、未完成分析帧、前缀/后续行像素与撤销/重做 |
| localization | Software 1× | 八语言查找栏和已打开菜单、窄宽、焦点、历史、主题与重新挂载 |
| binding | Software 1× | 正常事件回写、局部属性绑定、修饰顺序、换源、卸载及历史 |

验收产物位于本机临时目录 `/tmp/sourceview-v070-en817jq7`。
20 组结果见 `desktop-2crjaqtu/results.json` 和 `desktop-1xu_eoln/results.json`
的通过项并集；日志、二进制和截图均不写入仓库。

## 失败定位与限制

- 初次编译发现绘制迁移后 graphics 导入不再使用，移除后全仓测试通过。
- 初次新增圆角像素用例错误地将原生 Popover 外壳背景视为内容 RenderWidget 的一部分，
  期望白色而得到透明。核对公开绘制契约后，按独立内容的透明预期修正，未放宽误差。
- OpenGL Light 1.5× 的原有 frame 用例连续复现 `pointer/focus changed content bounds`：
  视口高从 387.65686 变为 379.65686，正好相差后出现的 8-DIP 横向滚动条。
  升级前 GOUI v0.6.0 / HEAD 同样复现。用例在异步正文测量完成前记录聚焦基线，
  现改为等待两个编辑器均有实际双向溢出和已分配的滚动条后开始；
  保留 30 秒期限和全部原有尺寸/颜色断言，增加失败时的前后尺寸。修正后五组外框全部通过。
  同一就绪条件通过仓库外 Go build overlay 应用于 GOUI v0.6.0 基线，
  OpenGL Light 1.5× 原用例也全部通过；结果见 `desktop-x4t8o76d/results.json`。
- 一次并行启动新旧版本对照因临时显示号冲突而未能建立会话；后续改为串行运行，
  未以该次环境失败计入场景通过。
- Windows/macOS 仅交叉编译，未做本轮原生窗口、DirectWrite/CoreText、真实 IME、
  物理键盘、跨显示器 DPI 或硬件 GPU 验收。本轮未发现需要修改 GOUI 的功能缺口。
