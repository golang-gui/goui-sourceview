# 菜单与查找栏多语言验收（2026-10-18）

设计见 [界面语言](DesignLocalization.md)。功能和包测试提交为 `a7a491a`，
前序基线为 SourceView `dcba5da`；窗口程序和本记录单独提交。
GOUI 依赖为 `903daf8638063bba1a2fb23ec18ff10d545d5fb4`，本轮未修改 GOUI。

## 实现

- `internal/resource` 集中定义 25 个包级文本资源，覆盖菜单全部 12 项及查找栏提示、
  输入/按钮名称和计数状态。提供简中、英、日、韩、德、法、俄、阿拉伯八语言。
- 通用编辑术语与 GOUI 对齐；保留 `mlang` 的地区匹配、英语回退和 GUI 线程切换契约。
- FindBar 的两个输入名称加入既有更新集合，修正始终为英文的问题。
  状态刷新复用资源，不再每次构造词条；完成计数改为“匹配数：N”形式。
- 菜单动作、输入和语言监听沿用既有机制；翻译不改正文、查询/替换内容、诊断错误、
  应用提供的补全或稳定标识。不增加公共 API，也不增加全局资源注册机制。

## 回归

先将新增的菜单和输入名称断言放到修改前代码执行：

```bash
go test . -run '^(TestEditorCommandTranslations|TestFindBarTranslatedInputsAndHints)$' -count=1
```

旧版失败：日、韩、德、法、俄、阿拉伯菜单回退英文；简中及其余六种语言的输入名称
仍为 Find / Replacement，六种语言提示也没有翻译。修正后通过。

新增包测试还验证：

- 25 个词条在八语言下非空、UTF-8 有效，非英语词条不遗漏为英文；`es` / `und` 回退英语。
- 地区标签下的菜单文案、查询/替换名称及提示符合独立预期。
- 计数中、查询过长、完成计数均使用当前语言；既有诊断字符串原样呈现。
- 0、1、2、5、21、1000000 的完成计数格式正确，不混用英语复数规则或错误格式占位符。

功能提交的暂存树另行归档到临时目录，以同一 GOUI 检出执行 `go test ./...` 通过，
不依赖下一笔新增窗口程序。

## 本机验证通过

环境：Ubuntu 22.04.5 / amd64、Go 1.25.8、Pango。

```bash
go test ./...
go test -race ./...
go vet ./...
```

窗口程序 `tests/window/editor/localization` 使用独立 `testdata/languages.json`，
预期由本项目编写，不从生产 Getter 导出。入口记录操作、环境、预期、副作用与退出规则。
按正常 Window.DispatchEvent / CommitText 分发输入，断言包括：

- 八语言查找栏的提示、输入名称与已完成计数；切换期间焦点、输入选区、正文选区、
  查询/选项、替换内容、模型修订和撤销/重做状态保持不变。
- 同一个已展开菜单原地切换八语言，逐项核对全部 12 项及可用状态，保留键盘活动项；
  最后在阿拉伯语界面以 Enter 执行全选，确认动作不依赖展示文本。
- 菜单长标签按自身原生文本度量完整分配，不以统一字体金图判断；
  查找栏逻辑宽度缩到 260 DIP 再恢复，输入保持可用和同一对象。
- 栏卸载期间不接收翻译，重挂载立即使用当前语言；整树卸载、重挂载、主题切换和销毁后
  状态及订阅行为正确。窗口尺寸事件验证逻辑布局，不计为实际窗口管理器缩放验收。

使用临时 TigerVNC / Xfwm4（compositor off）、`XMODIFIERS=@im=none`：

| 配置 | 通过 |
| --- | --- |
| Software / 1x | localization 八语言菜单/查找栏及状态、布局、生命周期断言；输出亮色截图 |
| Software / 2x | localization 全流程；既有 `tests/window/editor/find` 全流程 |
| OpenGL / 1x | localization 全流程；既有 `tests/window/experience` 编辑菜单与补全/配对全流程 |

运行命令为 `go run ./tests/window/editor/localization`，另可指定
`-capture-dir` 为仓库外现有目录。各配置通过 `GOUI_PLAT_PAINTER` / `GOUI_PLAT_SCALE` 设置。
截图只作观察证据，不构成像素金图；菜单内容离屏图的透明区域不是桌面最终合成效果。
临时桌面退出时清理，没有修改用户桌面、剪贴板或系统设置。

## 远端原生验证通过

SourceView 使用本任务源文件归档，GOUI 使用上述 HEAD 的干净归档；在各自临时工作区验证：

| 平台 / 排版 / 工具链 | 通过 |
| --- | --- |
| Windows 10 19045 / amd64、DirectWrite、Go 1.24.0 | `go test ./... -count=1`；localization、find、experience 三个窗口程序原生构建 |
| macOS 15.7.7 / arm64、CoreText、Go 1.24.9 | 同上 |

没有执行远端窗口场景，不将包测试或构建当作桌面、字体、DPI 或 IME 验收。

## 失败与未验证

`GOWORK=off go test ./...` 仍因 GOUI v0.1.1 缺少既有的
`textedit.Model.ApplyBatch` / `textedit.BatchOptions` 而构建失败；这是既有发布阻塞。
本次 `internal/resource` 在该命令中单独测试通过，整体独立发布仍需包含所需 GOUI API 的版本。

翻译未经过母语人工校对；不承诺阿拉伯语布局镜像。Windows/macOS 桌面、真实键盘/IME、
分数/混合 DPI、全部字体和 Linux 桌面合成器未验收。
