# 公开核心包迁移

## 决策与范围

基线为 0b2c1a3。用户允许调整 API，并确认本轮只迁移 SourceView 仓库。
核心入口由 `github.com/golang-gui/goui-sourceview` 改为
`github.com/golang-gui/goui-sourceview/sourceview`，包名继续为 `sourceview`。
SourceView、Document 及其他公开类型直接定义在新包，不保留根包兼容转发层。
模块路径、GOUI 依赖、`ui`、`language`、`scheme` 及现有内部包路径保持不变。

模块根目录的 29 个实现文件、29 个同包测试与平台测试辅助整体移入 `sourceview/`。
文件名、平台后缀、测试断言和资源所有权沿用，核心代码内容逐文件保持原样。
`ui`、示例、窗口程序的 Go 导入声明同步迁移；只改 import，不替换同内容的主题
注册 ID、样式名、信号或语义标识。临时示例 example/temp/main.go 只更新导入，
保留未跟踪状态；doc/Doing.md 原样保留。Markdoc 和 GOUI 仓库不修改。

当前结构：

```text
sourceview/   公开核心控件、文档、同包测试和平台测试辅助
ui/           声明式绑定
language/     语言定义与扫描
scheme/       代码配色
internal/     私有算法和呈现
example/      应用示例
tests/        独立窗口验证
```

此前按内部职责拆分的成果继续保留，原“29 → 21”文件合并方案不在本轮执行。
内部拆分历史见 [内部实现拆分](PlanInternalRefactor.md)，现行职责见
[SourceView 设计](DesignSourceView.md)。

## 接入与版本

命令式应用更新导入声明，现有类型名和方法签名沿用：

```go
import source "github.com/golang-gui/goui-sourceview/sourceview"
```

完整类型身份随包路径变化；声明式 `ui` 别名指向新包的真实类型。
`sourceStyleID` 继续为 `github.com/golang-gui/goui-sourceview`，它是稳定的主题
注册标识，不是需要迁移的导入声明。

这次目录布局尚未发布，已发布的 v0.5.0 不包含新子包。应用使用当前检出时，
将自身模块与本检出纳入本地 Go 工作区；不提交机器专属 replace 或 go.work。
例如，从已有应用模块新建工作区：

```sh
go work init . ../goui-sourceview
```

已有工作区则用 `go work use ../goui-sourceview` 加入检出。
源码仓本身继续用 GOWORK=off 验证声明的 GOUI v0.7.0 依赖。

## 验证方式

- 核对根目录无 Go 文件、58 个核心文件字节内容未变；比较公开声明与测试清单。
- 校验新包的真实类型路径、ui 类型身份、Widget 接口及实际文档／快照调用。
- 运行 go test ./...、GOWORK=off go test ./...、go test -race ./...、go vet ./...。
- 检查旧根导入已退出 Go 调用方，文档区分当前检出与历史发布版本。
- 对 Windows amd64、macOS arm64 交叉构建，在独立 Linux 桌面验证命令式、
  声明式绑定、主题、查找、补全、结构编辑和预览场景；分别记录原生与编译结果。

## 验证记录（2026-11-10）

- 根目录 Go 文件 58 → 0，sourceview/ 保留 29 个实现、29 个测试／辅助文件；
  58 个迁移文件与基线逐字节一致，公开声明清单一致，未删改测试断言。
  65 个 Go 调用文件更新导入，其中临时示例经反向恢复 import 后的哈希与原文件一致。
  go.mod、go.sum、doc/Doing.md 哈希保持不变。
- go test ./...、GOWORK=off go test ./...、go test -race ./...、
  go vet ./...、git diff --check 通过。旧根导入 go list 失败符合移除旧包的预期；
  唯一保留旧路径字面值的 Go 代码为稳定的 sourceStyleID 常量。
- 仓库外应用模块通过临时工作区实际导入新核心包与 ui，验证 Widget 接口、
  新完整包身份、ui 类型别名、枚举值以及文档／高亮／折叠快照调用，通过。
- Windows amd64、macOS arm64 的 CGO_ENABLED=0 go build ./... 通过；
  平台测试辅助还随 ./sourceview 的 go test -c 编译核对，原生运行未执行。
- Linux amd64 / Go 1.25.8 / GOUI v0.7.0 / Pango，使用独立 Xvnc + Xfwm4
  桌面 :190、D-Bus、配置目录和构建缓存。通过：resize_cache 软件 1×、
  binding 软件 1×、find_appearance OpenGL 1×、completion navigation -auto
  OpenGL 1×、structural Modern 暗色软件 1×、inline_multiline OpenGL 1×、
  localization 软件 1×、fold_preview Basic 软件 1×。
- fold_preview -basic 在软件 1.5× 下失败：short JSON has 4 native rows,
  expected 3。将迁移前的 0b2c1a3 独立归档、构建后，在同条件复现相同断言失败；
  迁移前后在软件 1× 均通过。二进制符号核对确认前者链接旧根包，后者链接新包。
  此项为有基线证据的既有失败，不计为通过；具体分数缩放／字体断言问题未在本轮
  修改，不扩大 skip 或容差。两次非零退出及日志均保留。
- 未验证物理 IME、混合 DPI、Windows/macOS 原生桌面或性能对照。测试剪贴板
  仅作用于独立显示连接，所属桌面进程已退出，没有修改用户文档和其他桌面会话。
  Markdoc、GOUI、发布标签和提交状态未改动。本轮未提交。

日志、对照归档和原生产物：/tmp/sourceview-package-move-cee8ihvc。
