# GOUI SourceView v0.3.0

## 中文

本版本需要 Go 1.24 或更新版本，依赖 GOUI v0.6.0。
发布 Go 库和源码示例，采用 MIT 许可证。

### 主要变化

- 大幅扩展内置语言：Go、Python、JavaScript/TypeScript、C/C++、Java、Rust、C#、
  Kotlin、Swift、Ruby、PHP、Lua、PowerShell，以及函数式语言和常见 DSL。
  同步提供各语言适用的词法建议、引用与括号规则。
- 增加 JSONC、JSON Lines、JSON5、CSV/TSV、EDN、HTTP、Protobuf、构建与运维配置，
  以及 AsciiDoc、reStructuredText、TeX/LaTeX、BibTeX、gettext、Mermaid、Graphviz DOT。
  EditorConfig、Git config、SSH config 等使用各自的语法规则。
- 支持 Markdown YAML/TOML 前置元数据及更多代码围栏语言；Go Template 和 Jinja2
  可显式组合 HTML、XML、YAML、TOML 宿主。扩展 XML 文件关联及 Cargo.lock 识别。
- 编辑时保留仍有效的语法颜色，减少当前行及后续行的高亮闪烁；修正焦点状态下
  鼠标悬停导致主题色边框消失的问题。
- editor 示例使用下拉列表选择 94 个内置样本，增大弹窗高度，便于浏览。
- 适配 GOUI 新的 padding 接口及声明式选区更新行为。

### 升级与边界

```sh
go get github.com/golang-gui/goui-sourceview@v0.3.0
```

GOUI 从 v0.4.0 升级到 v0.6.0；应用直接使用的 GOUI API 请同步参照
[GOUI v0.6.0](https://github.com/golang-gui/goui/releases/tag/v0.6.0) 的迁移说明。
本库不需要本地 replace 或 go.work。

语言支持提供词法着色与编辑辅助，不等于编译器、格式验证器或完整语言服务。
模板宿主由应用显式指定，不执行模板或推断生成结果；复杂文档格式的支持范围
以各语言实现为准。此版本继续提供源码示例，不分发预编译程序。

## English

This release requires Go 1.24 or newer and GOUI v0.6.0. It distributes the Go
library and source examples under the MIT license.

### Changes

- Expanded built-in languages include Go, Python, JavaScript/TypeScript, C/C++, Java,
  Rust, C#, Kotlin, Swift, Ruby, PHP, Lua, PowerShell, functional languages and common
  DSLs, with applicable lexical suggestions, quote handling and bracket rules.
- Added JSONC, JSON Lines, JSON5, CSV/TSV, EDN, HTTP, Protobuf, build and operations
  configurations, AsciiDoc, reStructuredText, TeX/LaTeX, BibTeX, gettext, Mermaid and
  Graphviz DOT. EditorConfig, Git config and SSH config use their own syntax rules.
- Markdown supports YAML/TOML front matter and more fenced languages. Go Template
  and Jinja2 support explicit HTML, XML, YAML and TOML hosts. File associations now
  include more XML formats and Cargo.lock.
- Editing retains valid syntax colors to reduce highlighting flicker on the current
  and following lines. Hovering over a focused editor preserves its accent border.
- The editor example provides a taller dropdown for selecting 94 embedded samples.
- Updated integration with GOUI padding APIs and declarative selection updates.

### Upgrade and scope

```sh
go get github.com/golang-gui/goui-sourceview@v0.3.0
```

The GOUI dependency moves from v0.4.0 to v0.6.0. For GOUI APIs used directly by your
application, see the [GOUI v0.6.0 release](https://github.com/golang-gui/goui/releases/tag/v0.6.0).
No local replace directive or workspace is required.

Language support provides lexical highlighting and editing assistance, not a compiler,
format validator or complete language service. Applications explicitly choose template
hosts; templates are not executed and generated output is not inferred. Complex document
formats have implementation-specific coverage. Examples are distributed as source only.
