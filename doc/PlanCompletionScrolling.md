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

正式依赖、跨平台构建、外观和最终工作区结果随下一切片补充。尚未作耗时或内存基准，
布局复用断言不等于总体性能提升结论。
