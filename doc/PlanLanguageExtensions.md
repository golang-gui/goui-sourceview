# 自定义语言、编辑与补全验证

日期：2026-10-19。契约见 [语言扩展](DesignLanguageExtensions.md)、
[补全](DesignCompletion.md)、[配对](DesignPairs.md)。

## 交付范围

- 公开不可变 Language、分片 Tokenizer/State/Context/Token，六种内置扫描器使用相同契约。
  internal 保留缓存、调度和命令计划，不将整个内部设施变成公共框架。
- 自定义样式键与词法角色分开；按定义对象身份绑定，支持同 ID 的不同定义，
  换语言/文档后刷新颜色与行为，并取消旧请求。
- 自定义词、自动闭合/包裹/结构序列、换行及注释；多字符序列跟踪和历史由控件负责。
- ApplyEdits 校验文档身份及修订，复用 GOUI 原子批次和历史选区。
- 自定义补全拥有候选和顺序，支持空前缀触发、每项 Range/Caret/AdditionalEdits；
  单工作队列、取消、最新请求替换和 GUI 投递，不向后台暴露活模型。
- 新增 `example/markdown` 与仅用公开 API 的语言子包；Integrated/Modern、亮暗覆盖、
  样本、列表续行、反引号配对、链接/引用补全、Bold 和注释操作。它是演示子集，
  不是内置 Markdown 或完整 CommonMark。
- 新增独立扩展窗口验证，并按异步接口迁移原有体验/补全窗口程序，保留各项断言。

本次没有修改 GOUI、go.mod 或本地工作区配置。联调 GOUI HEAD 为
`903daf8638063bba1a2fb23ec18ff10d545d5fb4`，其既有工作副本改动保持原状。

## 本机测试

Ubuntu 22.04.5 amd64、Go 1.25.8、Pango，本地工作区包括本仓与相邻 GOUI。

- `go test ./... -count=1`、`go test -race ./...`、`go vet ./...` 通过。
- 分项提交前将各阶段暂存内容检出到仓库外临时目录，分别运行 `go test ./...` 通过，
  避免后续未提交改动掩盖阶段依赖。
- 旧扫描器回归语料经测试适配器走公开协议，保留逐字节分片、畸形输入和跨行恢复断言。
- 新增定义拷贝隔离、同 ID 失效、非法扫描结果、无扫描器编辑、多字符跟踪、
  样式键与结构角色分离、换行重入、注释、文档身份/UTF-8/选区校验、附加编辑撤销、
  候选顺序、单工作队列取消和稳定配置等断言。
- Markdown 样本包含长 Unicode 标题、代码、注释、结束围栏及畸形围栏，比较
  1/2/3/7/31/4096 byte 分片与一次扫描，并独立断言分类、编辑规则和候选范围。

过程中窗口回归发现普通文本修改后仍调度空 tokenizer，导致 Advance 崩溃；已修正无扫描器
分支并增加 Pure/自定义无扫描器的元数据断言。逐字节样本也发现 Markdown URL/围栏状态
跨片段不一致，修正后保持相同预期，没有删除旧断言或扩大 skip。
最终复查还复现同 ID 切到无扫描器定义仍复用旧颜色的问题；颜色身份加入定义代次，
断言清除旧色、恢复扫描后重新着色，且行 ID、原生排版对象和度量保持复用。

## 窗口验证

独立 TigerVNC / Xfwm4，合成关闭。使用正常 DispatchEvent/CommitText，未使用 DevServer：

| 命令 | 后端与比例 | 结果 |
| --- | --- | --- |
| `go run ./tests/window/editor/extensions` | Software 1×，OpenGL 1× | 通过 |
| 上述命令加 `-dark` | Software 2×，OpenGL 1× | 通过 |
| `go run ./tests/window/experience` | Software 1× | 通过 |
| `go run ./tests/window/editor/completion -case scroll -auto` | Software 1× | 通过 |
| `go run ./tests/window/editor/completion -case unicode -dark -auto` | Software 2× | 通过 |
| `go run ./tests/window/editor/completion -case json -auto` | OpenGL 1× | 通过 |
| `go run ./tests/window/editor/languages` | Software 1× | 通过 |
| `go run ./tests/window/editor/selection` | Software 2×，三语言 × 三主题 | 通过 |

扩展窗口检查词移动、三反引号/Enter/撤销、空前缀触发、范围/光标、引用附加编辑和
一次撤销、提供者排序、同 ID 更换取消、错误仅报告一次，以及编辑语义长度/可见文本。
探针记录实际 Pango、painter 和 pixel scale；这些是场景断言，不等于物理输入/IME 验收。

真实本机桌面（Xfwm4 合成开启、Software 1×、Pango）另启动 Markdown 示例：
ChromeInfo 报告 mode=1、enabled=true；观察亮/暗标题、强调、代码、链接、注释，
实际键入 `[` 唤出链接、Enter 接受并进入 URL，选中文本保留语法色。
截图只作观察证据，不作为跨平台字体金图。

## 发布和平台边界

- `CGO_ENABLED=0` 的 Markdown 示例构建：Linux amd64、Windows amd64、macOS arm64 通过。
  Windows/macOS 本轮未原生运行，异步 GUI 投递、窗口、真实 IME、物理快捷键和混合 DPI
  仍需在对应平台验收；交叉构建不算原生通过。
- `GOWORK=off go test ./...` 仍失败：声明的 GOUI v0.1.1 缺少
  `internal/editor/plan.go:40` 使用的 Model.ApplyBatch / textedit.BatchOptions。
  这是已有依赖发布缺口；没有以机器 replace 或不可获取版本掩盖，不能声称独立发布验证通过。
- 不作性能/内存改善结论，没有新增原生编辑器或第三方语法依赖。后台提供者必须配合取消；
  不合作任务会延迟后续请求，控件不能强制终止外部 Go 函数。
- 本次真实桌面只操作并关闭自建窗口，切换了其焦点/主题；未改剪贴板、磁盘内容或系统设置。
  本次 VNC 客户端及独立桌面均已关闭。临时构建、阶段检出及观察图保留在
  `/tmp/sourceview-extensions-1D3DzRMy/`，仓库不含这些产物。
