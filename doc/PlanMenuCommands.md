# 菜单 Command / Shortcut 接入验收（2026-11-07）

## 范围

基线为 GOUI v0.7.0 升级提交 `4af4a75`。本轮只改 SourceView 的菜单适配、
editor 示例和相关回归，不改 GOUI、不增加公开 API 或第二套键盘控制器。

内置菜单项引用私有 gui.Command；关联的 gui.Shortcut 从所属视图已验证的
KeyBindings 生成，首个替代组合键作为提示。未绑定操作不显示提示，菜单仍可执行。
GOUI 负责标题/提示的独立测量、列宽及右对齐；不把文本拼进标题。
已打开菜单随重绑、解绑、语言和可用状态更新。配置验证失败保持旧提示。

Shortcut 只建立呈现关联，不向控制器注册；现有按键匹配、IME、补全及延迟输入保持原路径。
替换菜单、主动取消并换文档或卸载时，断开激活连接和 Shortcut 反向引用。
原生菜单在激活前正常关闭时保留操作连接，避免取消合法菜单点击；
动作仍核验原文档、挂载代次、当前模型和 canCommand。
示例 Edit 菜单通过同样的 GOUI 公开关联接口读取当前绑定，移除标题中手写的 F4 文本。

设计见 [编辑菜单](DesignEditing.md#10-编辑菜单) 和 [快捷键](DesignKeyBindings.md)。

## 回归与失败定位

- `menu_test.go` 验证配置原子性、替代组合键次序、视图隔离、无额外控制器、
  解绑后动作可用、旧激活断开及 Shortcut 反向引用释放。
- 示例单测验证标题/提示分离和临时菜单关联清理。
- 新增 `tests/window/editor/menus`：通过 Window/Popover.DispatchEvent 打开和执行菜单，
  比较公开 MenuItemInfo 与实际标签、列右边界、实时重绑/解绑、语言/只读/选区更新、
  删除/撤销、卸载重挂及换文档。列边界允许 0.01 DIP 浮点误差；不比较字体金图。
- 用仓库外 Go build overlay 将 menu.go / key_bindings.go 恢复到基线，保留新窗口用例：
  Basic 1× 确定失败于 `command 1 hint="" want="Ctrl+Z"`。未修改工作区构造旧版。
- 初始分数缩放窗口用例在语言/快捷键列更新后立即读取暂态布局，出现可见行未完成分配、
  旧坐标点击失败。现等待已完成的菜单帧和稳定分配后再断言，保留全部行、列和动作检查。
- 原 localization 用例同样在简中切换期间读取到暂态 14/16 行；补充菜单帧/稳定分配的
  就绪条件和 45 秒总期限。16 条独立文案、状态、焦点和原有标签裁剪容差均保留。

## 独立桌面结果

Linux amd64 / Go 1.25.8 / Pango，独立 Xvnc 24-bit + Xfwm4 compositor，临时 XDG 目录。
关闭测试窗口和本轮显示服务，不更改用户桌面、系统设置、剪贴板或文件。
Software/OpenGL 的实际后端由原有 keybindings / completion 窗口日志核对。

`/tmp/sourceview-menus-odyu4gdt/desktop-o_bmzeyy/results.json` 的 10 组场景全部通过：

| 场景 | 后端 / 缩放 |
| --- | --- |
| 新菜单集成回归 | Software Basic 1×、Modern Light 1.5×、Dark 2×；OpenGL Light 1×、Dark 1.5× |
| 既有 keybindings | Software Light 1×、OpenGL Dark 1.5× |
| 八语言 localization | Software 1.5× |
| JSON/XML/HTML × Basic/Modern Light/Dark 选区 | Software 1×，单个程序覆盖九个组合 |
| 补全导航与过期回调 | OpenGL Light 1× |

实际 editor 示例另用仓库外验证 overlay，在正常窗口中通过指针打开 Edit、
核对 F4/Shift+F4 的元数据与右边界、点击 Expand selection 并与普通 F4 输入结果比较。
该场景通过，结果见 `desktop-a2ezbdnv/results.json`。验证脚本和观测 PNG 保存在
同一临时目录，不加入产品流程或仓库。

## 命令与限制

全部命令使用 `GOWORK=off`，日志在上述临时目录：

- `go test ./... -count=1`、`go test -race . ./example/editor ./ui`：通过。
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`、
  `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...`：交叉编译通过。
- 完整 `go vet ./...` 仍有升级前即存在的 28 条 unkeyed fields 诊断；
  与 v0.6.0 基线逐行对照，未引入新诊断。`go vet -composites=false ./...` 其余检查通过。
- Windows/macOS 仅交叉编译，未执行原生桌面。物理 IME、系统快捷键截获、混合 DPI
  和硬件 GPU 不在本轮验收范围；未发现需要修改 GOUI 的功能缺口。

GOUI 升级与本轮菜单按功能分别提交；已有设计草稿和 example/temp 保留，不推送或发布。
