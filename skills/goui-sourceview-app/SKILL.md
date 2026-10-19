---
name: goui-sourceview-app
description: Integrate and customize GOUI SourceView code editors in Go desktop applications. Use for github.com/golang-gui/goui-sourceview widgets, documents, highlighting, editing rules, completion, or find/replace; not for unrelated editors or maintaining SourceView internals.
---

# 用 SourceView 编写代码编辑器

帮助应用接入和定制 GOUI SourceView。遵守使用方项目的开发约定，保留已有命令式或
声明式结构。技能随 SourceView 仓库维护；GOUI 的通用界面用法可参考其 goui-app 技能，
但使用本技能不要求先安装另一个技能。

## 先确认实际依赖

- 检查应用的 go.mod、go.work 和现有代码，用下列命令找到实际选中的版本和源码：

  ```sh
  go list -m -json github.com/golang-gui/goui-sourceview github.com/golang-gui/goui
  go doc github.com/golang-gui/goui-sourceview
  go doc github.com/golang-gui/goui-sourceview/language
  ```

- 按 Version、Dir、Replace 核对公开契约；模块缓存只读。新项目先确认实际可获取且相互兼容
  的版本，不虚构 tag，不因技能或在线 main 较新而升级已有应用。
- 当前版本直接依赖 GOUI v0.2.0，已包含批量编辑、样式注册和 TextInput 占位提示等
  所需接口，正常接入不需要相邻 GOUI 检出或 go.work。只有同时修改两个模块时才按需
  使用本地工作区；不提交机器专属 replace 或修改全局 Go 环境，交付前关闭工作区验证。

按任务阅读参考，无需一次读取全部：

- 控件、Document、滚动、FindBar、主题、文件与编辑事务：读 [应用接入](references/integration.md)。
- 自定义语法、跨行状态、词边界、括号、缩进和后台补全：读 [语言与补全定制](references/customization.md)。

## 会影响应用正确性的约定

- SourceView 是独立编辑控件。`gui` 接入用根包，声明式接入用本模块的 `ui`；滚动仍由
  GOUI ScrollView 提供，查找栏按需组合到 Overlay。不要嵌套 TextView 或复制编辑状态机。
- Document 保存文本、历史和语言，可由多个视图共享；选区、滚动、配对与补全会话属于视图。
  Document 不需要 Destroy。把文档、语言定义和补全提供者放在 build 之外或明确的持久状态中，
  不在每次重建时重载文本、重新创建提供者或手工销毁窗口拥有的控件树。
- 文档范围、光标、token 和补全编辑使用归一化 UTF-8 文本的**字节偏移**。
  协议的 UTF-16 位置、用户看到的行列或原始 CRLF 文件偏移需要在应用边界转换。
- GUI 控件和已挂载文档在 GUI 线程顺序访问；后台只处理独立数据，通过 App.Post 回到界面。
  文件读取、保存、编码和项目索引属于应用。组件不会因为创建文档或技能安装而自动读写文件。
- 语言定义直接绑定到 Document，没有全局语言注册。内置 Markdown 不存在；同版本
  `example/markdown` 展示的是应用定义的演示子集，不能当作完整 CommonMark 实现。
- 根包已登记 Basic / Modern 外观，应用按 GOUI 普通方式启用 Modern 即可。
  Scheme 只覆盖代码区域；FindBar 等界面跟随所属 stylesheet。选择前景的默认行为是保留语法色。

## 验证与交付

先完成可构建的接入，再验证本次需求。自定义扫描和纯规则可做无桌面测试；涉及实际输入、
选区、IME、补全浮层和滚动时，在对应桌面通过正常事件路径验证。真实操作和模拟操作都走
Window.DispatchEvent / CommitText 等公开入口，不添加绕过输入分发的语义动作。

运行应用的测试和构建；工作区联调后补查 GOWORK=off 的依赖条件。说明实际模块版本、
OS、后端、缩放和未验证项。仅编译通过不算窗口或 IME 体验通过；内置词法高亮不代表
完整语法诊断、LSP 或项目索引已经具备。疑似组件缺陷先核对同版本 API 并整理最小复现，
不要通过修改依赖缓存或导入 internal 规避问题。
