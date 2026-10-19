# 折叠、编辑辅助和标记语言实施记录

2026-10-19；基线 SourceView 1f0ef1b，正式依赖 GOUI v0.2.0。
设计：[折叠](DesignFolding.md)、[编辑辅助](DesignEditorAssistance.md)、[标记语言](DesignMarkup.md)。
用户授权写入设计、修改 SourceView、本机验证后分阶段提交；需要 GOUI 改动时先记录要求并跳过。

## 实施切片

- [x] A1：可见段落映射与原生几何接入。
- [x] A2：折叠范围、后台分析、JSON/XML/HTML 规则、交互、样式、语义和薄 UI。
- [x] B：编辑辅助与可选 GoToBar。
- [x] C1：XML/HTML 标签编辑、正式 Markdown。
- [x] C2：嵌入高亮/编辑和标签名称联动。
- [x] D：正则/捕获组替换、独立选区查询范围及多行输入。

每个切片记录实际范围和验证，未执行不写成完成；多光标、通用装饰、语言服务及编程语言后置。
查找增强沿用公开 SearchQuery，Regex=false 保留字面搜索。采用 Go regexp 语义；零长度匹配
必须进展，Find/Count/ReplaceAll 使用同一不重叠序列，替换为一次预算受限原子事务。
选区查询范围独立于 FindNext 改变的选区；编辑后映射或清除，换文档清除。非法表达式栏内反馈。

## GOUI 前置要求

暂无。遇到公开契约缺口时在此记录需求、调用证据、受阻场景及跳过范围，不修改 GOUI。

## 验证记录

普通与 GOWORK=off 全仓测试；按风险 race、Windows/macOS 交叉构建和本机原生窗口。
跨平台编译不作为原生交互验收。窗口启动、像素断言和实际操作分别记录。

### A1

新增连续可见行段映射，HeightIndex 只维护可见段落；布局/绘制/命中/滚动/上下导航统一转换。
2000 组固定随机样本与独立行枚举一致；假排版检查隐藏段落不占高度、命中返回真实位置、
未变正文布局复用。Linux amd64 / Go 1.25.8，GOWORK=off 全仓测试通过。
本切片尚未开放折叠，不计为折叠窗口验收；实际隐藏与 IME/快照在 A2 完成。

### A2

- 语言的 FoldingFunc、范围校验、JSON/XML/HTML 规则已落地；Document 单 worker 可取消并
  丢弃过期结果，视图独立保留嵌套状态。侧栏/行末标记、菜单、快捷键、搜索揭示、删除边界、
  只读及真实原文分段快照已接入。声明式只协调开关及信号，菜单文字补齐八语言。
- 新增规则、非法范围、编辑锚点、共享文档、撤销隔离、旧结果/卸载丢弃及通知重入测试。
  旧括号像素测试显式关闭折叠以保持其独立断言；补全定位期望纳入新侧栏宽度，没有删断言或放宽容差。
- Linux amd64 / Go 1.25.8 / Pango：GOWORK=off 全仓及 race 通过；Windows amd64 /
  macOS arm64 全仓 CGO_ENABLED=0 交叉构建通过。
- 新 tests/window/editor/folding 在独立 Xvnc :140 / Xfwm4（合成开启）以 Software 1x、
  Software 2x Dark、OpenGL 1x 通过；实际 imageResource 与 PixelScale 已记录。
  Software 1x 的新窗口另以 -race 构建并运行通过。键盘/指针、隐藏删除、搜索、历史与共享视图均断言通过。
- 原 tests/window/editor -auto 在同桌面 Software 1x 通过，覆盖原编辑、颜色缓存、重挂载、
  reveal、视口锚点与指针。未执行真实物理 IME、混合 DPI 或 Windows/macOS 新窗口。
  所有窗口仅使用内存样本，未改写真实剪贴板/设置，运行后自动关闭。
- XML/HTML 首轮仅折叠确认配对；HTML 省略结束标签及通用容错 DOM 推断未包含。GOUI 无新增要求。

### B

新增空白/行尾、缩进线、标尺、完整词高亮、智能 Home/End 和一级缩进退格；
可选 GoToBar 支持 Control+G、1 起始码点行列、非法输入提示、焦点恢复及八语言。
新增薄 ui.GoToBar 与示例互斥工具栏组合。显示选项不触发正文重排。
Linux / Pango 全仓 GOWORK=off 测试通过；新增纯逻辑/假排版断言与独立
Xvnc :140 / Xfwm4 的原生按键/CommitText 窗口在 Software 环境 1x 及 2x Dark 运行通过。
本窗口未独立探测后端类型，环境设置不作为实际比例/后端证据；未执行物理 IME、RTL 视觉或跨平台运行。
GOUI 无新增要求。

### C1/C2

内置 Markdown、六种现有语言围栏、标题/围栏折叠、列表/引用 Enter、默认配色，
以及 Tags 策略、标签闭合/匹配和名称一次事务联动已接入。ui.LinkedTags 默认协调开启，
主示例增加 guide.md，自定义示例基于内置定义副本。设计文档记录了词法 profile 和尺寸边界，
未宣称完整 CommonMark/DOM 一致性，未增加编程语言。
Linux GOWORK=off 全仓及 race 通过；分片一致性、子定义隔离、保守标签规则、
名称联动/撤销和嵌入补全身份有断言。独立 Xvnc/Xfwm4 的 markup 窗口以
Software 环境 1x、2x Dark 正常 Window.CommitText/DispatchEvent 验证闭合、名称删除与撤销、
HTML 空元素、列表以及围栏内 JSON 配对和换行。比例/后端设置未在该窗口独立探测，
物理 IME/跨平台原生运行未执行；GOUI 无新增要求。

### D

SearchQuery.Regex、Go 捕获组替换、原文规范匹配缓存、零长度进展，以及独立 SearchRange
和薄 ui 协调已完成。单行字面量继续分块；正则/多行的首次规范结果同步建立，未宣称异步。
FindBar 的查询/替换改用公开 TextView/ScrollView，支持 LF 和 Alt+Enter，新增正则及选区选项，
错误表达式原位保留并显示本地化提示/边框。自然宽度增为 512 DIP，行高读取真实 ContentSize，
修正字体估算产生的单行滚动条。完整机制见 DesignSearch.md。
新增逻辑断言及正常 Window.DispatchEvent/CommitText 窗口：多行、范围隔离、捕获组一次撤销、
非法表达式、零长度、Software 环境 1x/2x Dark 与 OpenGL 环境运行通过；Software 1x 窗口
以 -race 构建运行通过。原 find 窗口及外观像素/字体/窄宽度断言通过，外观另验证 18pt/2x。
2x 大窗口最初超过 1920 像素虚拟桌面而使尺寸断言失败，在 2560x1800 隔离桌面重跑通过，
没有删除或放宽原有断言。多语言窗口的具体字段类型及完整菜单夹具同步到多行输入和折叠命令，
八语言、展开菜单原地刷新、操作可达、焦点/历史/窄布局断言通过。
GUI 测试没有写入剪贴板或系统设置。GOUI 无新增要求。

### 收尾复查

- 普通/独立依赖全仓测试、race 和 Windows amd64/macOS arm64 CGO_ENABLED=0 构建已通过；
  go vet 曾指出本轮跨包 FoldRange 测试字面量未写字段名，补齐后通过。
- 旧 extensions、languages、shortcuts、find 和编辑器 -auto 原生窗口通过。
  选区窗口显式关闭折叠侧栏以维持其 6 DIP 像素采样原点，原色值/1 色阶误差未变，
  JSON/XML/HTML × Basic/Modern Light/Modern Dark 九组全部通过。
- 中大文件复查发现候选折叠异步完成会无条件重置实测高度，干扰 EOF reveal；
  新 fallback 字体回归在修正前失败，修正后通过。现仅在可见集合变化时重建几何。
  JSON 10 MiB（10,485,603 字节/51,403 行）、XML 10 MiB（10,485,636/48,103）、
  HTML 10 MiB（10,485,574/55,191）均完整 53 步且 0 失败。
- 标签辅助取消按文档整体 256 KiB 禁用的限制，新增 300 KiB 标签闭合/名称联动回归；
  使用当前完整文本同步确认，原文和配对的临时空间随输入增长，不作低延迟承诺。
  嵌入边界仍需已分析，不猜截断前缀。没有提出 GOUI 改动要求。

### 最终交付验证（2026-10-19）

- SourceView 继续使用正式 GOUI v0.2.0；本轮未改动 GOUI，未新增依赖或机器专属 replace。
  go test ./...、GOWORK=off go test ./...、全仓 race 和 go vet 全部通过；
  Windows amd64 / macOS arm64 的 CGO_ENABLED=0 全仓构建通过。
- Linux amd64 / Go 1.25.8 / Pango / Xvnc + Xfwm4 合成环境：折叠新增窗口以 -race 再跑通过；
  原编辑器、快捷键、公共扩展、TOML/YAML/Shell、查找、八语言菜单与选区窗口均通过。
  新查找窗口覆盖 Software 环境 1x/2x Dark、OpenGL 环境；折叠窗口的实际 imageResource
  和 PixelScale 探测已在 A2 记录。其他只配置环境而未单独探测的结果不作为独立后端证据。
- JSON 50 MiB（52,428,615 字节，257,006 行）完整 53 步、0 失败；取消标签文档大小禁用后，
  XML 与 HTML 10 MiB 各再跑 53 步、0 失败。该结果是功能验收，不是延迟/峰值内存结论。
- 18pt/2x 浮层外观在 2560x1800 隔离桌面通过；1x 原色值和裁剪断言通过并查看了实际截图。
  中大文件临时数据正常删除，截图/二进制/日志在仓库外；所有临时窗口与 :140/:141 桌面已关闭。
- 技能中的“没有内置 Markdown”旧描述已修正，并补充有效子语言、标签规则、查询范围、
  折叠和 GoToBar 接入；skill-creator 的 quick_validate.py 通过。中英文 README 只更新对外能力，
  具体机制留在设计文档；本轮 Markdown 本地链接检查通过。
- 未执行 Windows/macOS 原生窗口、物理 IME、混合/分数 DPI 或 RTL 视觉矩阵。
  Markdown 为源码编辑词法 profile，不宣称完整 CommonMark 解析/渲染。
  正则缓存首次扫描与根标签确认仍同步，资源/后台优化留作后续专题。
