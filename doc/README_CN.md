<h1 align="center">GOUI SourceView</h1>

<p align="center">面向 Go 桌面应用的原生代码编辑器</p>

<p align="center"><a href="../README.md">English</a></p>

GOUI SourceView 为 [GOUI](https://github.com/golang-gui/goui) 应用提供源码编辑能力，
支持语法高亮、补全、括号匹配及 Modern 亮暗外观，可通过命令式或声明式方式接入。

项目正在开发中。

## 主要能力

- **语法高亮：** 支持 JSON、XML、HTML、TOML、YAML、Shell。
- **源码编辑：** Unicode 选区、剪贴板、撤销/重做、缩进和整行操作。
- **补全与括号：** 补全建议、自动配对及匹配括号跳转。
- **自定义语言：** 由应用配置高亮、编辑规则及补全。
- **查找替换：** 浮动面板，支持区分大小写和整词搜索。
- **亮暗外观：** 选中文本时保留语法颜色。
- **多语言界面：** 编辑菜单及查找替换支持八种语言。

## 界面预览

Modern 浅色 — JSON

<img src="images/editor-light.png" alt="GOUI SourceView 使用 Modern 浅色外观编辑 JSON">

Modern 深色 — HTML 与查找替换

<img src="images/editor-dark.png" alt="GOUI SourceView 使用 Modern 深色外观编辑 HTML 并显示浮动查找替换面板">

截图来自 Linux 上实际运行的示例程序。

## 体验示例

需要 Go 1.24 或更新版本。先按[示例指南](../example/README.md) 准备开发环境，
再在仓库根目录运行：

```sh
go run ./example/editor
```

示例可以编辑 JSON、XML、HTML、TOML、YAML、Shell，体验查找与补全，并切换亮暗外观。
另提供[声明式示例](../example/declarative)和[自定义 Markdown 编辑器](../example/markdown)。
