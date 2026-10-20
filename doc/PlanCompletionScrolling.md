# 补全弹窗滚动与外观调整

2026-10-20，SourceView 基线 5d5f3e3，本地 GOUI b58948f，正式依赖 v0.2.0。
设计见 [补全设计](DesignCompletion.md)。本轮保留用户已有的 go.mod 本地 replace，不提交该配置。

## 滚动职责

- 保留固定行高的自绘候选内容，面板通过 GOUI ScrollView 提供真实滚动条、滚轮、裁剪和偏移。
- 滚动与选中分离；键盘请求行滚入视口，鼠标确认仍走正常 Popover 事件分发。
- 可见片段包含部分行，命中和语义边界按实际偏移和裁剪计算。
- 缓存按候选索引保留重叠行，释放离开视口的布局；列宽变化使旧布局整体失效。
- 自定义内容只实现 Scrollable 和 ScrollIntoViewRequester，不复制 ScrollView 或 ListView 私有实现。
- 弹窗保持非模态；主窗口中的 SourceView 处理补全按键，IME 和原生焦点不转移。

结构性滚动条由 GOUI ScrollBar 实际处理输入。GOUI v0.2.0 的 ScrollView.Snapshot 隐藏内部滚动条，
窗口专项通过普通公开控件树定位 ScrollBar，再分发正常指针事件验证拖动；未新增测试接口。
框架未来可评估滚动条语义呈现，当前不据此修改 GOUI 或阻塞滚动实现。

## 验证

滚动切片：

- 本地 GOUI b58948f：`go test ./...`、`go test -race ./...`、`go vet ./...` 通过。
- Linux amd64 / Go 1.25.8 / Pango / Xvnc :147（2560×1800）/ Xfwm4，
  `go build -race -o /tmp/sourceview-completion ./tests/window/editor/completion`；
  软件绘制 1x 浅/深色、2x 深色的 `-case scroll -auto` 通过。
  程序输出实际 painter 类型和缩放，验证像素量、行单位、拖动到底、键盘 Reveal、指针确认、焦点和撤销。
- 使用 5d5f3e3 生产源码与新增窗口断言复现：旧实现以 `wheel changed the selected candidate`
  退出码 1 失败。新实现通过，未把旧行为继续作为预期。
- 原有排版、着色、取消/错误回滚、重入和原生 Cluster 省略断言保留；新增像素部分行、
  重叠布局身份、独立资源计数验证。八行高视口与九行相交时，布局为 18 个、含省略探针峰值为 19 个。
- 首次拖动验证因 ScrollView 的语义快照不包含内部滚动条而失败；改为从公开控件树取得真实 ScrollBar。
  2x 首次测试暴露固定延时早于首帧布局，专项改为等待已分配的实际视口，有 8 秒超时失败条件，
  后续自动步骤从就绪时刻启动。没有跳过断言或将空视口算作通过。

## 外观与回退

- 普通非模态 Popover 使用 `sourceview.completion` StyleName，统一绘制背景、边框、圆角和阴影；
  内容不再绘制第二层外壳。Modern 使用官方 menu 的 8 DIP 圆角和阴影，Basic 为 4 DIP 圆角。
- 优先创建透明表面；公开 ErrUnsupported/ErrUnavailable 且没有内容错误时，核验会话后仅重试一次
  不透明宿主。失败宿主先脱离内容并销毁；重试失败保留两次错误，内容错误不触发重试。
- ScrollView 和候选留在圆角安全区域，较大自定义圆角同步增加留白并修正锚点。
  Basic、Modern 亮暗及自定义 32 DIP 圆角/2 DIP 边框有独立几何断言。
- 滚动条使用官方 scroll-view 规则和状态；移除旧的装饰 scroll-thumb part，迁移说明见样式设计。
- 按职责拆分提交：滚动行为为 32de6a0 `Use ScrollView for completion browsing.`，外观单独提交。

## 最终验证

以下均为 2026-10-20 本机结果。GOUI 工作区基线 b58948f，验证期间相邻仓库推进到 87787c6；
本轮没有修改 GOUI。所有二进制、日志、截图和独立依赖检出都在仓库外临时目录。

| 范围 | 命令或场景 | 结果 |
| --- | --- | --- |
| 本地依赖 | `go test ./...`、`go test -race ./...`、`go vet ./...` | 通过 |
| 正式 v0.2.0 | 无 replace、无 go.work 的源码副本执行 `GOWORK=off go test ./...`、`GOWORK=off go test -race ./...`、`GOWORK=off go vet ./...` | 通过 |
| Windows amd64 | 正式依赖，`GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...` | 通过，仅交叉构建 |
| macOS arm64 | 正式依赖，`GOWORK=off GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...` | 通过，仅交叉构建 |
| Modern 透明表面 | Pango / software / 1x，json、xml、html、long、unicode、scroll、wrapped、edge，各浅/深色，`-auto -expect-surface transparent` | 16 个场景通过 |
| 缩放 | software / 2x，scroll 浅/深色、unicode 深色 | 通过 |
| Basic | software / 1x，scroll，`-basic -auto -expect-surface transparent` | 通过 |
| OpenGL | Pango / 实际 opengl.imageResource / 1x，scroll 深色 | 通过，未把软件回退当作 OpenGL |
| 无合成器 | Xfwm4 compositor=off，确认 X11 `_NET_WM_CM_S0` 无所有者；software / scroll 深色 / 1x、2x，`-expect-surface opaque` | 通过，实际 Transparent=false |
| 正式依赖的窗口程序 | v0.2.0 单独构建 `-race` 补全专项；scroll 透明 1x 浅色、2x 深色及不透明 1x 浅色 | 通过 |
| 既有使用者 | `tests/window/experience`、`tests/window/editor/extensions`，software / 1x | 通过，含补全确认、取消、过期请求、附加编辑和撤销 |
| 语言集成 | `tests/window/editor/languages`，software / 1x、2x | TOML/YAML/Shell/JSON/XML/HTML 的补全、撤销及原有引号输入断言通过 |
| 实际外观 | Xfwm4 合成开启，Modern 深色 / software / 1x 桌面截图 | 人工检查圆角、细边框、柔和阴影、内容与滚动条留白；非跨平台像素金图 |

窗口程序通过普通 Window/Popover.DispatchEvent 和 CommitText 验证；实际软件/OpenGL 后端、
Pango、缩放及透明模式写入日志，运行带 race。语言集成首次在 Show 后同步读取虚拟行，以
`missing expected proposal for toml` 失败；改为独立步骤并在 8 秒期限内等候真实视口后再检查
候选与插入，保留原有断言，不等待特定候选来规避失败。首轮 Xfwm4 无合成器启动因旧实例自动重启占用
显示连接而失败，确认是本轮独立桌面的管理器后替换，实际无合成器场景随后通过。

原问题复现日志为 `/tmp/sourceview-completion-old-native.log`；最终专项日志为
`/tmp/sourceview-completion-{场景}-{light|dark}.log`，正式依赖日志为
`/tmp/sourceview-completion-release-{test|race|vet}.log`，缩放、回退及使用者日志使用同一前缀。
实际截图为 `/tmp/sourceview-completion-modern-dark.png`，这些临时产物不作为发布资源。
独立测试桌面 :147、测试窗口与本轮启动的 Xfwm4/Xvnc 已释放，既有桌面管理器未修改；
未访问剪贴板、修改系统设置或写入源码目录的运行产物。

未执行 Windows/macOS 原生桌面、物理 IME、系统快捷键拦截及运行中跨屏缩放验收。
尚未作耗时或内存基准，布局身份与计数断言只证明重叠资源复用，不等于总体性能提升结论。
