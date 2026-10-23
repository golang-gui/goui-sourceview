<h1 align="center">GOUI SourceView</h1>

<p align="center">面向 Go 桌面应用的原生代码编辑器</p>

<p align="center"><a href="../README.md">English</a></p>

GOUI SourceView 为 [GOUI](https://github.com/golang-gui/goui) 应用提供源码编辑能力，
支持语法高亮、补全、括号匹配及 Modern 亮暗外观，可通过命令式或声明式方式接入。

项目正在开发中。

## 安装

需要 Go 1.24 或更新版本，GOUI v0.3.0 由 Go 自动下载。

```sh
go get github.com/golang-gui/goui-sourceview@v0.1.0
```

## 主要能力

- **语法高亮：** 支持 JSON、XML、HTML、Markdown、TOML、YAML、Shell。
- **折叠与导航：** 结构化标记、Markdown 标题及围栏代码折叠，行列跳转。
- **编辑辅助：** 智能缩进、空白标记、缩进线及列标尺。
- **源码编辑：** Unicode 选区、剪贴板、撤销/重做、缩进和整行操作。
- **补全与括号：** 补全建议、自动配对、匹配括号跳转及标签名称联动。
- **自定义语言：** 由应用配置高亮、编辑规则及补全。
- **查找替换：** 浮动面板，支持多行、正则、捕获组替换及选区内搜索。
- **亮暗外观：** 选中文本时保留语法颜色。
- **多语言界面：** 编辑菜单及查找替换支持八种语言。

## 界面预览

Modern 浅色 — JSON

<img src="images/editor-light.png" alt="GOUI SourceView 使用 Modern 浅色外观编辑 JSON">

Modern 深色 — HTML

<img src="images/editor-dark.png" alt="GOUI SourceView 使用 Modern 深色外观编辑 HTML">

截图来自 Linux 上实际运行的示例程序。

## 使用 AI 编写编辑器

[SourceView 应用开发 Skill](../skills/goui-sourceview-app/SKILL.md) 帮助你的 AI 接入编辑器，
并定制语法高亮、编辑规则和补全。只需告诉 AI：

> 请按照 https://github.com/golang-gui/goui-sourceview/blob/main/skills/INSTALL.md 的说明，安装 SourceView 应用开发 Skill。

AI 会按[安装说明](../skills/INSTALL.md)使用当前工具的原生技能机制完成安装。
不支持 Skill 的工具可直接阅读开发指南。安装技能不会安装或升级应用的 Go 依赖。

## 体验示例

需要 Go 1.24 或更新版本。先按[示例指南](../example/README.md) 准备开发环境，
再在仓库根目录运行：

```sh
go run ./example/editor
```

示例可以编辑 JSON、XML、HTML、Markdown、TOML、YAML、Shell，体验查找与补全，并切换亮暗外观。
另提供[声明式示例](../example/declarative)和[自定义 Markdown 编辑器](../example/markdown)。

## 许可证

[MIT](../LICENSE)
