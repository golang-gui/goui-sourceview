# v0.3.0 发布记录

日期：2026-11-03。发布 Go 库与源码示例，不分发二进制附件。

## 冻结源码

- 候选与标签提交：`b2dfb9a0fe77db8b4caf8e2c78e5f2e167d38421`。
- GOUI：正式版本 `v0.6.0`，来源 `691493afe4a390de52a9036b84990a35ed827ed4`。
- 归档 SHA-256：`1f60191df4e58fac50adfb70b3b8439af6a202d21d91c22aca97b1ad746249ba`。
- 从 git archive 解包到仓库外目录；321 个文件逐一与归档比较一致。
  工作区模式和独立源码分别测试；独立验证均设置 `GOWORK=off`。
- 保留用户未跟踪的 `doc/Doing.md` 和 `example/temp/`，不纳入发布。

## 原生与构建验证

| 环境 | 工具链 / 排版 | 原生测试及子项 | 其他检查 |
| --- | --- | --- | --- |
| Ubuntu 22.04.5 amd64 | Go 1.25.8 / Pango | 946 通过，0 失败，0 跳过 | verify、tidy -diff、vet、CGO=0 build、全仓 race 通过 |
| Windows 10 amd64 | Go 1.25.8 / DirectWrite | 946 通过，0 失败，0 跳过 | verify、tidy -diff、vet、CGO=0 build 通过 |
| macOS 15.7.7 arm64 | Go 1.24.9 / CoreText | 946 通过，0 失败，0 跳过 | verify、tidy -diff、vet、CGO=0 build 通过 |

主要命令：`go mod verify`、`go mod tidy -diff`、`go test -json ./... -count=1`、
`go vet ./...`、`CGO_ENABLED=0 go build ./...`。
Linux 另执行 `CGO_ENABLED=1 go test -race ./... -count=1`；
Linux amd64、Windows amd64、darwin arm64 三目标交叉构建通过。
三端 editor 与 languages 窗口程序均从同一冻结副本构建。

远端访问 Go proxy 超时后，使用已校验的正式模块 `.info/.mod/.zip` 创建本轮临时
文件代理，保留 go.sum 校验，不修改全局 Go 环境或使用本地 replace。
Windows 的 PowerShell 初版脚本将下载提示当成错误，改为 cmd 重定向并核对原生退出码。
补齐临时代理中的平台依赖后重新执行全套检查，以上为最终完整运行的结果。

完整命令、退出码、测试 JSON、截图和归档保存在本机本轮临时发布目录
`/tmp/sourceview-v030-MnElbn/`；远端使用同名专用临时目录保留日志。
这些文件是验收证据，不作为 Release 附件。

## 窗口验证

窗口用例通过正常 Window/Popover.DispatchEvent，未使用 DevServer。
原生排版颜色断言已包含在上面的原生测试中；窗口启动与交互断言分别记录。


- Linux 独立 X11（TigerVNC、Xfwm4 合成、Software、Pango）：亮色 100% 和深色
  150% 的完整语言用例通过；每轮含 81 项补全/撤销及 79 项引用输入检查。
- macOS CoreText/OpenGL 和 Windows DirectWrite/Direct2D：深色 150% 完整语言
  用例通过，每端同样通过 81 项补全/撤销及 79 项引用输入检查。
- Linux 两种缩放下，Go Template/Jinja2 × XML/YAML/TOML 的 12 组窗口检查通过；
  声明式选区绑定检查通过。
- Linux Basic、Modern Light、Modern Dark × 100%/150% 的 6 组外框检查通过，
  覆盖聚焦/悬停颜色、滚轮、两轴滚动条拖动、禁用、换行及主题变化。
- Linux 亮色 100% / 深色 150% 的补全滚动和导航共 4 组检查通过，
  覆盖选项显露、滚动条拖动、延迟提供器与过期结果。
- Linux 首轮外部编排主动中止后调大整套程序的等待时间并完整重跑；
  未改动窗口用例的内部期限和断言。
- Windows 首次从 SSH 会话直接启动时，补全视口在内部期限内未分配。
  改为 RDP 实际交互桌面启动同一二进制后，完整用例通过。
- Windows 启动时曾因运行对话框尚未就绪，将路径前缀输入原有编辑窗口；
  随即撤销并截图核对原文恢复，未执行其中内容。重试在截图确认运行对话框后输入。
  测试程序及启动命令窗口均自动退出；本轮 RDP 连接已断开，既有 SSH 连接保留。
- 本轮创建的 Linux :87、:88 会话及窗口管理器均已关闭，原有显示会话保留。
- 真实 IME、混合 DPI、其他绘制后端组合及本轮未覆盖的大文件性能场景未重新验收。
  Windows/macOS race 未运行；没有把交叉构建或原生离屏测试代替这些检查。

## 正式发布与消费验证

- Annotated tag：`v0.3.0`，消息 `Release v0.3.0.`。
  远端 peeled SHA 与冻结候选一致；未移动既有标签。
- 创建 Draft 后逐项核对 tag、中英文正文、非预发布状态及空附件列表，再正式发布。
- 发布地址：[GOUI SourceView v0.3.0](https://github.com/golang-gui/goui-sourceview/releases/tag/v0.3.0)；
  发布时间 2026-11-03 12:37:35 UTC，已核对为公开、非 Draft、非 Prerelease 和 Latest。
- `GOWORK=off GOPROXY=https://proxy.golang.org go list -m -json github.com/golang-gui/goui-sourceview@v0.3.0`
  返回的 Origin.Hash 与候选一致。
- 仓库外新建无 replace 的消费模块，经官方代理执行 `go get ...@v0.3.0`、
  `go mod tidy`、`CGO_ENABLED=0 go build ./...`、`go run .` 和 `go mod verify` 通过。
  实际导入根包、language、scheme、ui，创建文档及声明式视图，识别 JSON5；
  依赖解析为 SourceView v0.3.0 与 GOUI v0.6.0。
- 中英文 README、示例说明及 AI 接入参考同步正式版本；收尾仅修改文档，
  不改变发布标签对应的源码。

