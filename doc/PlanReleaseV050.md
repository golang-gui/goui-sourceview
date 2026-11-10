# v0.5.0 发布记录

日期：2026-11-10。按维护者要求先发布当前提交的新标签，再整理后续设计。
发布 Go 库与源码示例，不提供预编译附件。

## 冻结源码

- 候选与标签提交：`ee2d14616d6c734dd952491e747e9250760342f9`。
- Annotated tag：`v0.5.0`，消息 `Release v0.5.0.`；标签对象 `7e15fafb377a811c0dc5b2ff66f12ad0dad8014a`。
- GOUI v0.7.0；模块最低 Go 版本仍为 1.24，没有 replace 或 go.work。
- 从候选 git archive 创建仓库外干净副本，456 个文件生成逐文件 SHA-256 清单。
- 源码归档 SHA-256：`fdbff9d5eb582f573e9c01bfbbb1b8d1ebea029d014d8fc9528e9c72dd73313e`。
  远端传输前后校验归档，构建与测试没有带入未提交的设计和 example/temp。
- 标签保持上述候选；后续安装说明、发布记录与设计整理不改动已发布标签。

## 原生测试与构建

所有检查使用 GOWORK=off、正式可获取依赖和专用临时目录；未改全局 Go 环境。

| 环境 | Go / 原生排版 | go test -json ./... -count=1 | 其他检查 |
| --- | --- | --- | --- |
| Ubuntu 22.04.5 amd64 | Go 1.25.8 / Pango | 2150 项通过，0 失败，0 跳过 | mod verify、tidy -diff、vet、CGO=0 build、全仓 race 通过 |
| Windows 10 19045 amd64 | Go 1.25.8 / DirectWrite | 2150 项通过，0 失败，0 跳过 | mod verify、tidy -diff、vet、CGO=0 build 通过 |
| macOS 15.7.7 arm64 | Go 1.24.9 / CoreText | 2150 项通过，0 失败，0 跳过 | mod verify、tidy -diff、vet、CGO=0 build 通过 |

测试项计数包含命名子项。Linux 另通过 Windows amd64 / macOS arm64 的 CGO=0 全仓交叉构建；
三端原生结果分别执行，不把交叉构建计作原生通过。窗口程序均以 CGO_ENABLED=0 构建。

## 独立窗口与输入验收

所有模拟交互通过正常 Window.DispatchEvent / CommitText，无 DevServer 或语义动作捷径。
本轮覆盖相对 v0.4.0 的变化和相关回归，不声称穷尽所有语言、平台和显示比例组合。

- Linux：私有 Xvnc 24-bit / Xfwm4 合成桌面，15 组通过。
  单行与多行建议分别运行软件 Light 1×、OpenGL Dark 1.5×；
  统一折叠预览在软件 Basic 1×、OpenGL Dark 1.5× 通过；
  剩余语言折叠四批在软件 1× 通过；补全 typing、水平滚动和键绑定为软件 1×；
  SetText 和编辑器外框为 OpenGL Dark 1.5×。
- Windows：RDP 实际交互桌面，DirectWrite / Direct2D、1×，12 组最终通过。
  单行建议、多行建议 Dark、统一折叠预览、四批剩余语言折叠、补全 typing、水平滚动、
  SetText Dark、键绑定、编辑器外框 Dark。程序从已观察到的“运行”对话框启动。
- macOS：VNC 对应的登录桌面，CoreText / OpenGL、1×，相同 12 组最终通过。
  单行/多行建议验证使用平台主修饰键；折叠阈值样本按本机字体校准，具体失败和复验见下节。
  最终另跑一次多行建议 Dark 回归通过。

场景检查 Tab/Esc/Undo/Redo、真实文本与历史、共享视图隔离、输入与过期结果清理、
建议换行/滚动/命中、折叠后的搜索/删除/撤销、原生 1/2/3 显示行门槛、补全弹窗稳定，
以及手动拖动水平 thumb 与 SetText 实际绘制。每个程序 PASS 和退出码均核对。
窗口程序自身的 Painter 断言不等于跨平台桌面合成像素验收；本轮未新增三端金图比较。

## 失败与验证适配

- 新增的 inline_suggestion、inline_multiline 和 fold_preview 探针中有固定 Ctrl+Z，
  它们此前的记录只包含 Linux 原生与其他平台交叉构建。macOS 默认是 Cmd+Z。
  本轮在冻结目录外通过 Go overlay 使用 gui.ModPrimary.Resolve，保留全部场景和断言。
- macOS 的 fold_preview 首次进入短 JSON 门槛场景失败：固定 120 个数字只有 1 个原生显示行，
  预期为 2。原源码已明确这些长度针对 Linux/Pango/default-font 矩阵。
  在外部探针副本中，将两行/三行样本长度分别改为 180/240，仍精确要求原生 1/2/3 行、
  折叠入口门槛及变宽后的两行展开行为。完整折叠场景复验通过。
- 上述 overlay 只修改验证程序的快捷键和样本长度，不修改 SourceView/GOUI 库源码，
  不删除断言、不扩大 skip、不把失败首次运行计作通过。标签仍固定原候选。
- 一次远端探针重新构建在模块目录外执行，返回 go.mod not found；改为进入冻结 src 后重跑成功，
  没有修改模块配置。该启动错误不是构建通过结果。
- GitHub Draft 创建第一次遇到 TLS handshake timeout，查询确认尚无 Draft 后重试成功。
  Draft 的按标签 REST 查询返回 404，改为列出授权可见 Release 并按 ID 核对；未重复创建。
  Latest 查询一次也遇到 TLS 超时，重试后核对公开状态成功。

## 正式发布与公开消费

- origin/main 从 `27ecf0c` 快进至候选，随后只推送本次 annotated tag；远端 peeled SHA 与候选一致。
- 创建 Draft 并核对 tag、中英文完整正文、Draft/Prerelease 状态与空附件；随后正式发布。
- [GOUI SourceView v0.5.0](https://github.com/golang-gui/goui-sourceview/releases/tag/v0.5.0)
  于 2026-11-10 06:16:55 UTC 发布；已核对公开、非 Draft、非 Prerelease、Latest，附件为空。
- 官方 proxy.golang.org 查询 v0.5.0 的 Origin.Hash 为候选 SHA。
  仓库外消费模块完成 go get、tidy、CGO=0 build、run 和 mod verify，实际导入 root/language/scheme/ui，
  验证 InlineSuggestion 类型和命令式/声明式功能开关；解析到 SourceView v0.5.0 / GOUI v0.7.0。
- 官方模块源码逐文件 SHA-256 与冻结清单全部 456 个文件一致。
  中英文安装说明和本记录在公开验证后整理，不改动标签。

## 后续设计与边界

框架对照、最新缺口和优先级整理在 [能力差距](DesignCapabilityGaps.md) 与 [路线](Roadmap.md)。
补全呈现、保存点、视图恢复、片段等仍是提案；后续设计提交不属于 v0.5.0 功能交付。
保留片段暂缓、GOUI 缺口无法绕过时跳过，以及不深入语言语义解析的范围约束。

## 证据、清理和未覆盖项

日志、归档、清单、测试 JSON、外部探针 overlay 和桌面截图保存在
`/tmp/sourceview-v050-k2ilyvlq/`；远端使用同名专用临时目录。
Linux 窗口结果为 `desktop-01t0u4mb/results.json`；mac/win 目录保存原生测试和窗口退出记录。
这些不是 Release 附件。

本轮私有 Xvnc/WM 已结束，专用认证文件清理；本轮创建的 Mac VNC / Windows RDP 客户端连接已断开，
既有 SSH 连接保留。没有改动用户文件、全局配置或剪贴板，也没有关闭原有应用。
远端实际桌面曾短暂切到测试窗口；未把远端测试描述成独立的用户登录会话。

物理 IME、系统快捷键截获、真实混合显示器 DPI、硬件 GPU、Windows OpenGL、macOS Software、
远端 race 和本轮系统剪贴板交互未执行；未将未覆盖项算作通过。
