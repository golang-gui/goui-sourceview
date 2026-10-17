# 查找替换浮层验收（2026-10-17）

本轮依据 [设计](DesignFindBar.md) 把三行表单改为可选的编辑区浮层：默认一行查找，替换展开为两行，
输入右侧紧凑选项、矢量动作图标、弱边框和阴影，响应窄宽度，亮暗配色按编辑器隔离。
文本输入继续复用 TextInput，搜索、历史与生命周期契约不另建一套。

## 版本与实现边界

- 本轮起点 SourceView `3f17aa2`、GOUI `6f684b6`。
- GOUI `00521cc`：公开已有阴影的 Widget-local Painter facade、更新消费者 mock 与状态回归。
- GOUI `210732a`：显式 StyleScope、完整 stylesheet 的最近作用域解析、统一失效、GUI/widgets 消费者迁移。
  样式仍通过语义名称解析，不恢复逐控件颜色 setter。框架无反向领域依赖。
- 本仓 `c0edcba` 增加 scheme.FindColors；`0e81091` 更新 FindBar、薄绑定、两个 Overlay 示例和原有窗口回归；
  新外观程序与验收记录另作提交。
  代码绘图是本项目原始线条；经 GOUI icon/svg 解析，没有图标字体或外部图片资源。
  CPU SVG source 共享有界缓存；每个 Icon 仍拥有自己的 native image，卸载时释放。
- 新触达的间接依赖 oksvg（75e7ce872d46）以及已有 rasterx 为 BSD-3-Clause，随 Go 模块保留许可，
  二进制分发需携带相应声明。没有引入原生编辑器或新的平台后端。go.mod/go.sum 已 tidy。
- FindBar 只缓存一份当前配色/sheet，沿用挂载期 Root 帧同步；绘制不反复复制 token map。
  面板阴影在自身 8 DIP 留白中裁剪，透明留白也会参与普通矩形命中。

## 已通过

Linux amd64 / Go 1.25.8，X11、Pango，任务独占 Xtigervnc + Xfwm4（compositor off）：

```bash
go test ./...
go test -race ./...
go vet ./...
```

以下程序在 Software 1x/2x、OpenGL 1x 的独立桌面执行通过，原 find 程序记录实际 painter 类型与缩放：

```bash
go run ./tests/window/editor/find
go run ./tests/window/editor/find_appearance
```

- 原 find 程序保留所有查找/替换/计数、只读、历史、重入销毁和声明式重建/重定向/移除断言，
  改为 Overlay 组合并按语义名称定位图标控件。
- 外观程序检查独立的实色面板/原生输入颜色、并列亮暗配色、更换单个主题、浮层不改变正文分配、
  单行与展开尺寸、窄宽度隐藏/恢复、键盘、焦点和命中。采样与颜色空间见 Testing.md。
- Software 1x 额外执行 shortcuts 程序，通过。
- 用 `-inspect` 在真实桌面截图检查亮暗、展开、圆角/阴影、图标及文字对齐；再通过 xdotool
  缩窄本任务窗口并检查截图。没有操作用户现有桌面或更改系统设置；临时显示和进程已关闭。
- `3f17aa2` 的旧实现放入相同 Overlay 并运行新增外观程序，在 `compact natural bounds` 断言失败，
  新版同场景通过。新用例可以区分原来的表单布局，不仅是启动/输出日志。
- GOUI 本机 `go test ./...`、相关包 race，以及阴影单独 staged tree 包测试通过。
  样式作用域覆盖两树隔离、嵌套/nil、重挂载、失效合并、旧 slot 清理和循环拒绝。

Windows 10 19045 amd64 / Go 1.24.0、macOS 15.7.7 arm64 / Go 1.24.9：
通过 remote-shell 在任务临时目录复制干净 GOUI tree 与 SourceView，均通过：

```bash
# SourceView
 go test ./... -count=1
 go build ./tests/window/editor/find
 go build ./tests/window/editor/find_appearance
 go build ./tests/window/editor/shortcuts
# GOUI
 go test ./gui ./widgets ./ui -count=1
```

这些是原生编译和包测试，不代表远端实际桌面外观已验收。任务文件留在远端临时目录；
未安装、发布或推送。传输临时 HTTP 服务已关闭。

## 失败、未执行及发布限制

- `GOWORK=off go test ./...` 仍失败：go.mod 的已发布 GOUI v0.1.1 缺少 ApplyBatch/BatchOptions，
  也尚未包含当前浮层所需通用扩展。本轮补齐 SVG 间接依赖校验和后，失败点恢复为该既有接口缺口。
  需要 GOUI 发布包含上述提交的版本后更新依赖；本地工作区通过不能视为独立发布通过。
- GOUI `go vet ./gui ./widgets ./ui` 失败：既有测试的未具名 TextSelection/TextRange 字面量。
  将修改前 HEAD 与相同用户未跟踪测试放入临时副本比对，167 条具体诊断完全相同，无新增项。
  未修改或删除这些无关测试。SourceView 的 vet 通过。
- Windows/macOS 实际桌面、物理按键/IME、分数/混合 DPI 和更多字体未执行。
  不使用 DevServer 或语义动作捷径；没有将环境缺失/构建通过写成窗口场景通过。
- 本轮没有性能基准结论。计数仍分块、导航/替换仍同步，程序化多行查询弱装饰的既有边界未扩大。

完整命令日志、干净 tree、旧版对照与截图位于本机任务临时目录，源码仓库没有图片或可执行产物。
