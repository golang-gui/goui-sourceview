# GOUI v0.8.0 正式依赖接入

日期：2026-11-15。用户授权在 GOUI v0.8.0 发布后继续迁移；本轮不发布 SourceView。

## 基线与修改

- SourceView 起点 `78a16bf`，已完成类型化输入、Surface、焦点策略、共享补全浮层及 Run 错误处理。
- 此前本地 GOUI 基线 `538ed4fa2d677249094eb2891dbd3b1497e53bc3`。
- 发布模块 `github.com/golang-gui/goui v0.8.0`，Origin 为标签提交
  `bc47eb006836fce9fed3cfe695f6345995513199`，模块最低 Go 1.24。
- GOUI 发布后本地 HEAD `990a7aa` 是文档收尾，不将其当作 v0.8.0 的标签或模块来源。

核对[官方发布说明](https://github.com/golang-gui/goui/releases/tag/v0.8.0)及完整提交差异：
从本地基线到 v0.8.0 仅 Demo 和文档变化，gui/ui/core/layout/platform/style/theme/widgets
及 GOUI go.mod/go.sum 没有变化，生产库 API 无需再次适配。

go.mod 的 GOUI 依赖从 v0.7.0 升到 v0.8.0；go.sum 替换对应模块和 go.mod 校验，
`GOWORK=off go mod tidy` 没有其他直接/间接依赖变化。Go 要求仍为 1.24。
不修改生产 Go 代码、不添加测试镜像实现、不增加兼容分支或修改 GOUI。

中英文 README、示例说明、设计入口和发布草稿改为正式依赖；此前本地工作区失败保留在
[11-14 记录](PlanGOUILocalMigration.md)，以明确日期区分历史与当前要求。
SourceView 自身的新包布局尚未发布，其他应用联调本检出仍需使用本地 SourceView；
GOUI 已可正常下载，不再需要它的本地检出。已有忽略的 go.work/go.work.sum 保留供双仓开发。

## 验证方法与范围

依赖修改执行 `GOWORK=off go get github.com/golang-gui/goui@v0.8.0`、
`GOWORK=off go mod tidy`。以下正式依赖检查均设置 GOWORK=off，不改全局 Go 配置。
模块查询的 Version 为 v0.8.0，Replace 为空，Dir 位于下载模块缓存，避免工作区掩盖旧依赖。

Linux 环境为 Ubuntu 22.04.5 amd64 / Go 1.25.8 / Pango 1.50.6。
窗口使用独立 Xvnc/Xfwm4、software、100%/150% 应用缩放，不使用 DevServer，
输入通过正常 DispatchEvent/CommitText。程序从本仓编译，依赖来自发布模块。

选择受迁移影响的多光标、视图恢复、GOUI 接入三类窗口，各运行 Basic/Modern Light/Dark
×100%/150%，共十八组，再运行补全 typing/navigation 两组，共二十组。
这覆盖修饰键、局部坐标、焦点策略、共享浮层替换、输入/撤销、长行恢复及配套输入子树。
依赖库与上一轮联调源码相同，不将上一轮五十组窗口结果重新计入本轮。

## 执行结果

以下正式依赖检查全部通过，未使用相邻 GOUI 源码：

| 命令或场景 | 结果 |
| --- | --- |
| `GOWORK=off go list -m -json github.com/golang-gui/goui` | v0.8.0，下载模块缓存，Replace 为空 |
| `GOWORK=off go test -json ./... -count=1` | 2362 项通过，0 失败/跳过，包含命名子测试 |
| `GOWORK=off go test -race ./sourceview ./internal/controls ./internal/view ./ui` | 四包通过 |
| `GOWORK=off go vet ./...` | 通过 |
| `GOWORK=off go mod verify`、`GOWORK=off go mod tidy -diff` | 通过，无额外模块文件差异 |
| `GOWORK=off CGO_ENABLED=0 go build ./...` | Linux amd64 通过 |
| 上述构建加 `GOOS=windows GOARCH=amd64` / `GOOS=darwin GOARCH=arm64` | 均通过；交叉编译不代表原生运行 |
| 三类窗口 × 三主题 × 两缩放，以及补全 typing/navigation | 20/20 通过，正常事件分发，无断言失败 |

另执行保留的双仓工作区 `go test ./...`，通过；该结果不替代上述发布模块检查。
差异空白检查及文档本地链接目标检查通过。

本机证据根 `/tmp/sourceview-goui-v080-splbi_kb`：dependency.json 记录实际模块身份，
library-diff.log 为库代码零差异，test.jsonl / test.stderr 为全仓非缓存测试，
race.log / vet.log / verify.log / tidy.log / build-*.log 为对应检查；
programs.json、cases.json、build-window.log 与 final-matrix/results.json 记录二十组窗口。
workspace-test.log 为额外的工作区检查。日志、程序和临时桌面配置均在仓库外。

runner 的 finally 已清理本轮窗口、Xvnc、Xfwm4 和私有总线；没有操作用户桌面 :3，
所选窗口不访问真实剪贴板，没有连接 Windows/macOS 桌面。原有 doc/Doing.md、example/temp/
及 GOUI 未跟踪文件保留；未修改 GOUI，也未提交工作区或 replace。

已解除“所需 GOUI 尚未发布”的依赖阻塞，可关闭工作区独立构建本仓。
Windows/macOS 新代码原生运行、真实 Alt/Option、macOS VNC Option 映射、真实 IME、
OpenGL/Direct2D 与混合 DPI 未在本轮重验；本轮亦未重跑最低 Go 1.24 工具链。
这些缺口不因发布依赖可获取而消失。SourceView 发版继续暂缓，未推送、打标签或创建 Release。
