# TOML、YAML、Shell 验收记录

日期：2026-10-19。功能规则见 [语言设计](DesignHighlighting.md#9-tomlyaml-与-shell)、
[补全设计](DesignCompletion.md)、[编辑设计](DesignEditing.md)。

## 改动

- 内置三种语言及自动识别；新增 Variable 分类并接入 basic/modern 和亮暗 Scheme。
- 独立扫描状态、不可变嵌套栈/队列、当前逻辑行的延迟分类，保留原有三语言回归。
- 补全、自动配对和括号匹配复用词法上下文；不在块文本、注释或 heredoc 中生成代码建议。
- 新增三份原创示例，六个文档可独立编辑和撤销；文件按钮随宽度换行。
- 中英文 README 只更新对外语言列表；技术细节留在 doc/ 与示例指南。
- 未修改 GOUI、go.mod、go.work 或第三方依赖。联调 GOUI 为 `903daf8638063bba1a2fb23ec18ff10d545d5fb4`。

## 逻辑与原生验证

| 环境 | 执行与结果 |
| --- | --- |
| Ubuntu 22.04.5 amd64，Go 1.25.8，Pango | `go test ./... -count=1`、`go test -race ./...`、`go vet ./...` 通过 |
| macOS 15.7.7 arm64，Go 1.24.9，CoreText | 本机原生 `go test ./... -count=1` 通过；语言窗口程序与 editor 示例构建通过 |
| Windows 10 19045 amd64，Go 1.24.0，DirectWrite | 本机原生 `go test ./... -count=1` 通过；语言窗口程序与 editor 示例构建通过 |
| 关闭工作区，声明的 GOUI v0.1.1 | `GOWORK=off go test ./...` 失败：`internal/editor/plan.go:40` 的 `Model.ApplyBatch` / `textedit.BatchOptions` 不存在，是已有依赖版本缺口 |

新增逻辑断言覆盖键、字符串、转义、数字/日期、YAML Core 字面量与块缩进，
Shell 引用/替换、多个 heredoc、TAB 剥离、quoted delimiter、here-string 和算术左移。
Shell 的变量、命令替换或转义字符后紧接 `#` 时仍属当前词，只有实际词边界才开始注释。
识别用例包括扩展名优先、启动文件、shebang 与 `env` 带参数选项，未知选项保守降级。

分片断言覆盖 1/2/3/7/31 字节及 16 KiB 工作片段、Unicode、跨行引用、超过片段的键、
数字、日期小数和 delimiter；YAML 的超长数字/布尔后接空白或注释仍保留字面量颜色，
数字内部的空白则按普通标量处理。900 份固定随机种子的畸形语法输入同时检查分片一致性、
非重叠范围及完整 UTF-8 字符边界；测试生成的数据不进入源码目录。
编辑回归对删除多行起始引号、替换块标记、修改 heredoc 结束行/引用后的分类给出独立预期，
再对照新引擎及模型行索引；另验证入状态快照不被后续扫描修改。

三端新增原生测试以同一后端/字体/比例的独立单色 TextLayout 比较字形位图，
覆盖 TOML 多行字符串、YAML 块文本和 Shell 变量（含 heredoc 的 `${HOME}`），分别检查亮暗 Scheme。
比较前复制借用像素，逐字节断言尺寸/stride/像素一致；不跨平台共享字体金图。
这证明指定字形的前景正确，不等于桌面合成、真实输入法或所有语法的像素验收。

## Linux 窗口矩阵

独立 TigerVNC 桌面、Xfwm4（compositor off），通过正式窗口事件/文本提交路径运行：

```sh
go run ./tests/window/editor/languages
go run ./tests/window/editor/languages -dark
```

| 实际 painter / scale | 浅色 | 深色 |
| --- | --- | --- |
| Software / 1× | 通过 | 通过 |
| Software / 2× | 通过 | 通过 |
| OpenGL / 1× | 通过 | 通过 |

六种语言逐个切换，检查基础建议、Enter 接受和一次撤销；逐字符输入 TOML 三引号、
YAML doubled quote、Shell literal 反斜杠/结束引号；另检查括号生成、只读拒绝输入及语义文本。
每个运行程序记录实际 typography、painter、scale，断言完成后关闭；启动成功不算验收。

真实用户桌面另使用 Xfwm4（compositor on）、Software 1×、Pango，检查 1120×780 的
editor 示例：ChromeInfo 报告 Integrated enabled，三份新增样本可切换，六个文件按钮
完整显示，TOML 浅色及 YAML/Shell 深色的关键结构肉眼可辨。截图仅为观察证据；
heredoc `${...}` 的变量颜色由上述独立原生断言确认。

## 限制与副作用

- 三端全仓测试采用本地工作区；GOUI 所需接口发布前仍不能声称独立依赖构建通过。
- macOS/Windows 此切片没有运行新增窗口交互矩阵，只有原生包测试及窗口程序构建；
  三端物理快捷键、真实 IME、跨屏缩放、远端桌面合成均未在此切片验收。
- 这是词法高亮与基础编辑联动，不提供 TOML/YAML 配置校验、Shell 执行或完整上下文敏感解析。
  heredoc 正文不递归解析完整命令替换；复杂 YAML 显式键等边界见语言设计。
- 不作性能/资源占用改善结论。本次超长 token 验证针对正确性，不代替中大文件性能测量。
- 真实桌面仅启动并关闭本次示例，期间切换了本窗口焦点/主题；未修改剪贴板、文件或系统设置。
  本次 VNC 客户端与独立测试桌面已关闭，既有 SSH 连接保持可用。
- 临时日志、构建产物与观察图保存在执行机 `/tmp/sourceview-languages-wi76dpfo/`；
  远端同名临时目录保留用于复查，没有向仓库写可执行文件或临时截图。
