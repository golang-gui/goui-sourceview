# v0.4.0 发布记录

日期：2026-11-08。发布 Go 库及源码示例，不分发预编译附件。

## 冻结源码与完整性

- 最终候选与标签提交：`da7fe3b2d3cccc1a153fd98f8c60921f16cfc809`。
- GOUI：正式版本 v0.7.0；Go 声明最低版本保持 1.24。
- 本轮 git archive SHA-256：
  `8899d5847941cf90e70b4daac75faa8ac73115a7266b296f7ebc1f8aa07493c6`。
- 从该提交导出仓库外的干净副本，358 个文件生成逐文件 SHA-256 清单。
  远端归档摘要及三端源码与清单一致；macOS 另确认没有额外文件。
- 所有命令使用 `GOWORK=off`，无 replace、go.work 或全局 Go 配置修改。
  远端使用本轮临时 file:// 模块代理，内容来自已校验的正式模块缓存；
  保留 go.sum 验证，不依赖相邻 GOUI 检出。
- 保留已有 DesignCapabilityGaps、DesignCompletion、Roadmap 提案修改，及未跟踪的
  DesignSavePoint、DesignSnippets、DesignViewState、Doing 和 example/temp，未纳入发布。
  Roadmap 收尾仅提交已发布状态，保留用户提案改动。

## 原生测试与构建

| 环境 | Go / 排版 | 测试及子项 | 其他检查 |
| --- | --- | --- | --- |
| Ubuntu 22.04.5 amd64 | Go 1.25.8 / Pango | 1021 通过，0 失败，0 跳过 | verify、tidy -diff、完整 vet、CGO=0 build、全仓 race 通过 |
| Windows 10 19045 amd64 | Go 1.25.8 / DirectWrite | 1021 通过，0 失败，0 跳过 | verify、tidy -diff、完整 vet、CGO=0 build 通过 |
| macOS 15.7.7 arm64 | Go 1.24.9 / CoreText | 1021 通过，0 失败，0 跳过 | verify、tidy -diff、完整 vet、CGO=0 build 通过 |

主要命令为 `go mod verify`、`go mod tidy -diff`、`go test -json ./... -count=1`、
`go vet ./...`、`CGO_ENABLED=0 go build ./...`。Linux 另执行
`go test -race ./... -count=1` 和 Windows amd64 / macOS arm64 的 CGO=0 交叉构建。
窗口程序也从各端同一冻结副本以 CGO=0 构建。三端测试计数包括命名子项。

此前的 28 条 vet 诊断通过为 LineIndent/FoldRange 测试字面量补齐字段名消除，
未改测试数据、断言或 skip。完整 vet 已通过，不再使用关闭 composites 检查作为发布结果。

## 窗口验收

均通过正常 Window/Popover.DispatchEvent 和 CommitText，不使用 DevServer。
下面列出本轮实际运行的组合，不能推导为全部平台/后端/缩放组合已验证。

- Linux：独立 Xvnc 24-bit / Xfwm4 compositor，Software 和实际 OpenGL。
  20 组程序全部通过，包括 Basic/Modern 亮暗菜单、语义着色与补全、Minimap、
  行标记、P0 结构编辑、键绑定、独立四角像素、查找、外框、八语言和保色选区。
  使用 1×、1.5×、2× 的相关组合；P0 在 Basic 1×、Modern Dark 1.5× 通过，
  包含约 10 MiB、十万折叠候选和 4096/4097 编辑预算边界。
- Windows：RDP 实际交互桌面 / DirectWrite / Direct2D，9 组通过。
  菜单亮暗为 1×；键绑定、语义分析、Minimap、行标记、独立四角和外框为暗色 1.5×；
  选区程序在 1× 覆盖 JSON/XML/HTML × Basic/Modern Light/Modern Dark。
- macOS：VNC 所在实际桌面 / CoreText / OpenGL，9 组通过。
  菜单亮色 1×，暗色菜单、键绑定、语义分析、Minimap、行标记、独立四角与外框 1.5×；
  选区程序在 1× 覆盖相同九个语言/主题组合。
- 后端由 keybindings、frame 等窗口中的实际 imageResource 类型核对。
  OpenGL 验收不代表已验证硬件 GPU。

## 失败定位与复验

- 初始候选 `7067f0f` 的 macOS 语义窗口在修订已发布、未编辑行仍等待重绘时，
  检出 23 个旧粉色像素。诊断确认文本和分析跨度已更新，公开 layoutPending 仍为 true。
  用例现等待该状态结束，再保留原来的旧色零像素、新色至少 5 像素断言及 30 秒总期限。
  仅修改窗口用例；重新提交、冻结为最终候选，并在三端重新执行检查和窗口程序。
- Windows 1600×900 桌面下，菜单 1.5× 用例因无法同时分配全部 16 行而在就绪阶段超时。
  改为 1× 完整执行菜单断言；该次超时未计为通过，未放宽行数、对齐或操作断言。
- 初版 Windows 批处理日志把尾部数字当成重定向句柄，结果文件为空。
  改为前置重定向后，最终逐场景退出码记录完整；不把空日志或启动成功算作验收通过。

## 正式发布与消费验证

- main 快进到冻结候选，创建 annotated tag v0.4.0，消息 `Release v0.4.0.`。
  标签对象为 `bee48241ff3f1f01401d8b2a0553df07ef11c155`，远端 peeled SHA 与候选一致。
  未移动任何已发布标签。
- 先创建 Draft，核对中英文正文、标签、非 Prerelease 和空附件列表，再正式发布。
- [GOUI SourceView v0.4.0](https://github.com/golang-gui/goui-sourceview/releases/tag/v0.4.0)
  于 2026-11-08 02:33:38 UTC 发布；已核对公开、非 Draft、非 Prerelease，并为 Latest。
- 官方 `https://proxy.golang.org` 的 v0.4.0 查询返回 Origin.Hash 与候选一致。
  独立无 replace 的消费模块完成 go get、tidy、CGO=0 build、run 和 mod verify；
  实际导入 root/language/scheme/ui，并创建行标记、折叠、高亮快照及 Minimap 配置。
  解析得到 SourceView v0.4.0 / GOUI v0.7.0。
- 官方代理下载的源码与冻结清单的 358 个文件一致。安装说明、功能列表和 AI 接入参考
  在公开验证后更新，收尾只改文档，不改变标签对应的源码。

## 证据、清理与未覆盖项

本机完整日志、退出码、测试 JSON、归档、清单和截图保留在
`/tmp/sourceview-v040-pp96qc0m/`；远端使用同名专用临时目录。
Linux 最终窗口结果为 `desktop-f6lh8xpz/results.json`；mac/win 子目录保存各端原生记录。
这些是验收证据，不作为 Release 附件。

本轮 Linux 专属显示及窗口管理器已关闭；Mac VNC、Windows RDP 连接已断开，
既有 SSH 连接保留。只在 Linux 独立测试会话使用并清空测试剪贴板，未修改用户文件、
系统配置或用户桌面剪贴板。Windows 启动命令在截图确认运行对话框后输入。

真实 IME、系统快捷键截获、混合显示器 DPI、硬件 GPU、Windows OpenGL、
macOS Software 及远端 race 未在本轮验收；未将交叉构建代替原生结果。
