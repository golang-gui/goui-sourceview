# 编辑器命令快捷键

SourceView 保留平台常用默认绑定，并以每视图独立的表允许修改、解除或全部接管。
行为继续由命令式控件和既有公开方法实现；GOUI 提供 KeyGesture、平台修饰键和正常事件分发。
本模块不增加全局命令注册器、字符串按键解析、上下文表达式或多段快捷键协议。

## 公共接口

```go
type Command uint16
type KeyBindings map[Command][]gui.KeyGesture

func DefaultKeyBindings() KeyBindings
func (v *SourceView) KeyBindings() KeyBindings
func (v *SourceView) SetKeyBindings(bindings KeyBindings) error
```

Command 是固定的内建操作标识，零值与未知值无效。菜单复用同一组操作标识；映射不会改变
只读、禁用、IME、选区、撤销或生命周期要求。新增键绑定不是新增一种动作实现。

- 构造安装当前平台默认表；DefaultKeyBindings 每次返回独立表及独立切片。
- Getter 返回可直接修改的深副本。Setter 验证并复制完整表；不存在的命令没有隐式默认回退。
  调用方随后修改 map 或切片不会改变控件，两个视图即使共享 Document 也不共享可变绑定。
- nil 和空表都清除全部可配置命令键；单个命令的空切片等同未绑定。
  恢复默认时显式传入 DefaultKeyBindings，不额外维护总开关和每功能 setter。
- 一个命令可以有多个替代组合键。用 GOUI Resolve 按当前平台核对 Key、Modifiers，
  再用 KeyGesture.Matches 精确匹配事件；额外修饰键不匹配。
- 未知命令、不支持的按键/修饰位、纯修饰键、不同命令解析成同一组合键时返回错误，旧表完全保留。
  同命令解析为同一组合键的别名去重；例如 Linux 的 Primary/Control 与 macOS 的 Primary/Command。
  按命令编号排序验证，错误和匹配顺序不依赖 map 枚举。
- 命令键要求包含 Shift 之外的修饰键，或者使用 F1–F24。未修饰及仅 Shift 修饰的文字、
  Enter、Tab、Escape、方向/删除键等保留给普通输入、焦点和瞬时面板交互，配置时明确返回错误。
  这不是 Vim 模态输入系统；宿主有此需求时仍使用 GOUI 正常捕获控制器实现其输入模式。
- GUI 线程顺序更新表，下一次事件立即使用新配置。一次事件只解析、执行一次；动作回调中的
  重配不会再用新表执行同一个事件。相同配置为无操作；Setter 不改变正文、选区、历史、焦点或补全请求。
  关闭补全面板等同步回调若更换绑定或编辑状态，本次键事件停止，不向新配置或新文档继续派发。

```go
bindings := editor.KeyBindings()
bindings[sourceview.CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
delete(bindings, sourceview.CommandMoveLinesDown)
if err := editor.SetKeyBindings(bindings); err != nil {
    return err
}
```

F2 是此示例的宿主选择，不是控件新默认值。默认不绑定手动补全，尤其不拦截 Ctrl+Space；
AutomaticCompletion、ShowCompletion 以及提供者的触发字符契约不变。

## 命令范围和默认策略

当前覆盖复制/剪切/粘贴、撤销/重做、全选、注释、缩进、插入/移动/复制/删除行、选择行、
按词移动/扩选/删除、文档边界与平台行边界、括号跳转、折叠、结果导航、补全及可选面板打开。
部分公开命令没有默认键，包括补全、块注释、全部折叠/展开、仅删除选区等，可由宿主显式绑定。

默认表保留原有平台组合：Primary 在 Windows/Linux 为 Control、macOS 为 Command；
按词操作在 macOS 用 Option；复制行在 Linux 用 Ctrl+Alt+Shift，其余桌面用 Alt/Option+Shift。
macOS 保留 Command+方向键、Command+G 和 Ctrl+G 的不同含义；具体默认值以 DefaultKeyBindings
为准。[编辑设计](DesignEditing.md)中的表描述默认配置，不代表不可替换的控件硬编码。

普通文字提交、IME、单键编辑/导航、接受 Tab、Escape 后释放 Tab 的策略保持原路径。
补全列表优先处理其方向键、Enter/Tab 和 Escape；源控件未消费的事件按 GOUI 正常顺序继续传播。
解除绑定即解除消费；只读状态下仍消费已绑定编辑命令，但不修改文档，沿用原有行为。
菜单点击和直接方法调用不依赖绑定表是否为空，错误仍经原有返回值/ConnectError 契约处理。

## 内置界面与声明式层

CommandFind、CommandReplace、CommandGoTo 由 SourceView 自身响应，内部栏首次打开时创建。
当前绑定精确匹配后调用 ShowFind、ShowReplace、ShowGoTo；无需外部目标关联或全局对象查询。
FindBar 的查询/替换输入使用目标表的 FindNext/FindPrevious；其 Enter、Shift+Enter、Escape
是字段本地交互，保持现有规则。GoToBar 的提交与关闭同理。
每次事件读取所属 SourceView 的当前配置。栏内不向正文追加控制器；卸载释放订阅，不保存第二份绑定表。

ui.SourceView.KeyBindings(bindings) 在配置时复制传入表，显式指定才协调；nil 显式清空，
未指定的普通重建保留交互或命令式设置。错误配置沿用其他 ui 参数的 panic 策略，
不新增默认错误弹窗或另一套协调状态。

示例和验证程序显式选择 F2 并同步提示，不再暗示 Ctrl+Space 是默认能力。
系统/输入法先消费的键仍无法由控件取回，应用应选择其运行环境可用的组合；模拟事件通过
不作为实体键盘、操作系统保留键或所有键盘布局的验收。

## 参考与验证

- Scintilla 5 [Key bindings](https://www.scintilla.org/ScintillaDoc.html#KeyBindings) 提供默认表、
  重绑与清除。采用每编辑器可替换映射的边界；解除键后按 GOUI 正常传播，不照搬 SCI_NULL 的吞键语义。
- Monaco / VS Code 1.105.0
  [StandaloneCodeEditor](https://github.com/microsoft/vscode/blob/1.105.0/src/vs/editor/standalone/browser/standaloneCodeEditor.ts#L221)
  分离动作与绑定。本项目采用固定操作标识及 GOUI KeyGesture，不引入其服务和上下文表达式体系。
- GtkSourceView 5 [View](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.View.html)
  以动作呈现补全、移动行等能力；SourceView 继续允许公开方法独立调用。

逻辑测试验证深复制、独立视图、nil/空表、恢复默认、别名冲突、错误原子性及声明式控制。
正常 Window.DispatchEvent/CommitText 窗口验证解绑放行、只读、面板绑定、补全交互、回调重配，
并保留原快捷键、行命令、折叠、查找和补全专项回归。平台与版本记录见 [实施记录](PlanKeyBindings.md)。
