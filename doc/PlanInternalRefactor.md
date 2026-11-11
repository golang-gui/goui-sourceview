# 内部实现拆分

> 后续决策：用户已允许调整 API，核心真实类型及其同包测试整体迁入公开
> `sourceview/`。下文的“根包／根目录”是各批实施时的历史布局；现行布局及
> 迁移验证见 [公开核心包迁移](PlanPackageMigration.md)。内部职责拆分继续保留。

保留根包中真实定义的 SourceView、Document 和现有公开 API；不改变导入路径，
不以类型别名重新导出整套实现。内部包不能反向导入根包，不复制编辑器状态或 WidgetBase。
未跟踪的 doc/Doing.md 和 example/temp/ 不属于本次变更。

## 批次

1. 高亮区间索引 → internal/highlight。校验、发布与 IME 映射仍由根包掌握。
2. 折叠范围与可见行索引 → internal/folding。命令与生命周期留在根包。
3. 补全列表呈现 → internal/controls。会话与实际编辑留在根包。
4. 查找、定位栏呈现 → internal/controls。编辑动作与失效检查留在根包。
5. 空白符与缩略图辅助计算 → internal/view。
6. 原生排版缓存与预览资源辅助 → internal/view。保持单一几何来源。
7. 快照编辑计划、结构／括号扫描、补全值算法、定位和锚点映射 → internal/editor。
8. 代码主题规则、标记标签、空白和括号装饰 → internal/view。
9. 快捷键校验、菜单命令资源和输入框刷新 → internal/controls。
10. 根包中仍需真实编辑状态的协调按领域合并，核对 API、资源与原生场景。
11. 折叠私有记录、不可变快照构建、候选重建和顺序／批量编辑映射 → internal/folding。

后四批用于继续实现根目录整理目标，不将完成前六批等同于所有私有逻辑已下沉。
纯算法与呈现下沉；必须访问真实文档、修订、焦点及生命周期的协调仍由根包拥有。

## 验证

每批执行 go test ./... 和 GOWORK=off go test ./...，保留既有断言。
涉及内部呈现时运行既有原生窗口程序；最终补充 race 与跨平台编译。
实际验证结果逐批记录，不将编译入口等同于窗口验证。

## 记录

- 初始基线：a91ed11，go test ./... 通过。
- 批次 1：将持久化前景区间树的构建、映射、查询移入 internal/highlight。
  Highlights、HighlightSpan 仍为根包的实际类型；原有公共、随机映射、共享快照、
  失效和原生着色测试保持在根包，通过真实 Document 边界验证内部索引。
  验证：go test ./...、GOWORK=off go test ./... 均通过。
- 批次 2：下沉范围校验、隐藏区间合并、可见行索引。两个独立算法测试随实现移动，
  保留随机枚举和非法范围断言；公共折叠、IME、预览及高度集成测试仍在根包。
  验证：go test ./...、GOWORK=off go test ./... 通过。
  原生窗口：独立 X11 会话 :90，Pango/OpenGL、1x，folding 程序通过键盘、
  指针、隐藏区域删除、搜索、历史与共享视图断言。
- 批次 3：补全列表及面板完整移入 internal/controls，原生行资源仍由列表拥有。
  根包会话通过只读状态和窄回调适配，保留真实编辑器的焦点、主题与会话身份。
  UI 主题规则也由 controls 提供，注册仍由根包统一执行；可比较外观快照用于
  根包判断是否关闭原生弹窗。控件测试迁移保留断言，根包集成测试继续运行。
  验证：go test ./...、GOWORK=off go test ./... 通过；独立 X11 会话的
  completion typing、navigation、Basic Unicode、Modern 暗色 scroll 均通过。
- 批次 4：查找／定位栏的控件构建、布局、绘制和样式盒移入 controls。
  根包栏仍是原有生命周期子控件，负责订阅、失效、查找／替换／导航及焦点；
  内部呈现对象只借用动作，不新增文档或选区状态。样式盒去重，测试随实现迁移。
  原生 find_appearance 首轮发现状态样式键大小写误改，修正后重跑通过。
  验证：两种全量测试通过；独立会话 find、find_appearance、localization 通过。
- 批次 5：空白符的有限预算索引、边界扫描及视口查询移入 internal/view；
  缩略图的像素采样和滑块计算也下沉。根包保持样式、源文映射、版本校验、
  缓存键及帧预算。WhitespaceMode 仍实际定义在根包，内部模式在边界显式转换。
  四个独立索引测试跟随移动；选区、片段、IME、资源和绘制集成测试保留根包。
  验证：两种全量测试通过；独立窗口 assistance（Modern／Basic）、
  minimap（Modern 暗色）通过，包括空白绘制和定位导航。
- 批次 6：段落原生资源及其测量几何、多行建议的试排和释放、首行预览几何、
  分页预览裁剪和大坐标定向舍入移入 internal/view。根包继续拥有段落缓存键、
  成员、着色、修订、失效、淘汰、视口及建议会话；绘制、命中和光标仍使用
  同一份测量几何。原有缓存、预览、多行建议及重入集成断言保留在根包，
  新增内部资源释放顺序、预览不触发排版、裁剪及舍入边界断言。
  补齐此前跨包 Interval 初始化的字段名，消除 go vet 的三处警告。
  验证：go test ./...、GOWORK=off go test ./...、go test -race ./...、
  go vet ./... 通过；Windows amd64、macOS arm64 的 CGO_ENABLED=0
  go build ./... 通过。公开声明与初始基线一致，外部消费者的根包类型身份、
  ui 导入、空白符枚举及高亮／折叠快照接口检查通过。
  原生窗口：Linux amd64 / Go 1.25.8 / GOUI v0.7.0，独立 X11 会话 :190，
  Xvnc + Xfwm4、Pango，未使用本地工作区替换依赖。resize_cache 软件 1×／2×；
  fold_preview Modern 软件及 Basic OpenGL 1×；inline_multiline Modern 软件／
  OpenGL 及 Basic OpenGL 1×；line_collapse、horizontal_scroll Modern 软件 1×
  均通过原有事件、几何、实际绘制、历史及资源断言。所属桌面进程已退出，
  剪贴板操作仅作用于该独立显示连接；没有更改用户文档或桌面设置。
  首次多行建议产物在首帧日志编码时两次 SIGTRAP，反汇编发现标准库
  internal/abi.NoEscape 字节为 00，与 PlanInlineSuggestion.md 的既有记录相同；
  用仓库外独立 GOCACHE 及 GOWORK=off go build -a 重建后恢复 RET。
  重建后一次 Modern 首次发布返回拒绝；在独立 D-Bus 会话、等待窗口管理器
  初始化并先运行缓存场景后，Modern 软件／OpenGL 均通过，未修改或放宽窗口断言。
  初始化拒绝的具体原因尚未定位；上述失败不计为通过，也不归因为本次拆分或
  GOUI 功能缺口。日志与产物保留在 /tmp/sourceview-internal-finish-0sdxjmkr。
  Windows/macOS 仅编译，未执行其原生窗口、物理 IME、混合 DPI 或性能验收。
- 批次 7：快照重新缩进、清理／大小写计划、缩进粘贴、缩进列计算、结构候选与
  括号匹配、行／列定位、锚点映射，以及补全候选规范化、去重、排序、附近词扫描
  和编辑列表规划移入 internal/editor。根包继续掌握最终事务、语言回调、选区历史、
  文档身份、修订、请求代次和取消。补全使用结构约束保留真实的根包公开类型，
  不通过类型别名重新导出，也不预先复制无界候选列表。
- 批次 8：主题规则与配色构建、标记标签／图元、空白符图元和括号装饰几何移入
  internal/view。根包传递已测几何、颜色和必要值，不在内部新增文档或选择状态。
  审查时保留了自定义样式表回调后的语言／缓存读取时机，增加独立回归断言，
  确认回调更新语言和匹配缓存时仍读取当前值并复用当前缓存。
- 批次 9：快捷键验证／去重、菜单 Command／Shortcut 关联及释放、补全文字键
  分类和查找输入框刷新移入 internal/controls。命令号由类型参数沿用根包类型；
  输入控制器和真实命令执行仍由根包拥有。旧菜单释放、快捷键逆向关联清理、
  视图隔离与重配集成断言保留。
- 批次 10：将补全输入保留协调合入 completion.go、多行建议协调合入
  inline_suggestion.go、分页段落协调合入 layout_cache.go，移除三个单独的私有实现
  文件。相较本轮起点，根目录非测试 Go 实现从 32 文件／12,281 行变为
  29 文件／11,247 行，减少 1,034 行；测试文件保留原位置和既有断言。
  根包公开声明对比一致；外部消费者的导入、真实命名类型身份、枚举值及快照 API
  检查通过。没有修改依赖版本、GOUI、doc/Doing.md 或 example/temp/。
  批次 6—10 按排版资源、编辑算法、主题绘制、快捷键菜单及协调代码整理分组提交。

### 批次 7—10 验证（2026-11-10）

- Linux amd64 / Go 1.25.8 / GOUI v0.7.0：go test ./...、
  GOWORK=off go test ./...、go test -race ./...、go vet ./...、git diff --check
  通过；Windows amd64、macOS arm64 的 CGO_ENABLED=0 go build ./... 通过。
- 独立 X11 :190 / Xvnc + Xfwm4 / Pango / 独立 D-Bus 和配置目录，关闭本地
  工作区，使用仓库外独立 GOCACHE 构建，未再出现上一轮异常构建产物。
  11 组原生场景通过：resize_cache 软件 1×；structural 的 Basic 软件 1×和
  Modern 暗色 OpenGL 1.5×；keybindings 软件 1×；extensions 暗色 OpenGL 1×；
  line_marks 的亮色软件 1×和暗色带 minimap 的 OpenGL 1.5×；assistance 的
  Modern 软件 1×及 Basic OpenGL 1.5×；find_appearance 和 inline_multiline
  的 OpenGL 1×。检查真实事件、撤销／重做、结构选择、异步补全、重入、
  快捷键、标记和空白符像素、查找几何／焦点及多行建议绘制。
- 最后的样式读取时机调整后，重跑两种全量测试、全量 race、vet 和两目标
  交叉构建，并重建、通过 resize_cache、find_appearance、暗色 assistance
  软件 1.5×、暗色带 minimap 的 line_marks OpenGL 1.5×。
- 所属桌面进程已退出；测试剪贴板只用于独立显示连接，没有更改用户文档、
  桌面设置或现有桌面会话。日志与原生产物保留在
  /tmp/sourceview-private-split-7q7jvne1。Windows/macOS 仅编译，物理 IME、
  混合 DPI 和性能对照未执行，不从本次功能场景推导性能结论。

### 拆分提交验证

每个职责组从暂存区导出独立源码目录，执行 GOWORK=off go test ./...，通过后再提交。
主题／绘制组初次导出发现菜单尚引用迁出后的 containsPoint；补齐同组的内部几何
调用后重新导出并通过，未留下依赖后续提交才能构建的中间提交。完整日志位于
/tmp/sourceview-refactor-commits-0ff2u997，不包含 doc/Doing.md 或 example/temp/。

### 批次 11：折叠快照与编辑映射

接续 f2064dd。公开 FoldRegion 和 Folds 保留根包的真实定义；私有记录下沉，
公开快照只复制其值。规范化快照构建、按字节端点保留意图、普通和单调批量编辑
映射、隐藏区间合并移入 internal/folding。根包保留修订／Reset／IME 检查、
分步接收预算、选区保护、可见区刷新、通知和生命周期，不另建编辑状态。
折叠主文件从 903 行降到 774 行；根包实现合计从 11,247 行降到 11,118 行。
原有根包折叠、共享文档、接收取消、重入与撤销断言保留，新增内部快照字节端点、
升／降序及非单调编辑、边界失效、无 splice 映射、切片隔离和隐藏区间断言。

验证：go test ./...、GOWORK=off go test ./...、go test -race ./...、
go vet ./...、git diff --check 通过；公开声明对比和外部消费者类型身份／枚举／
快照接口检查通过。Windows amd64、macOS arm64 的 CGO_ENABLED=0 go build ./...
通过，未执行其原生窗口。

Linux amd64 / Go 1.25.8 / GOUI v0.7.0，独立 X11 :190、Xvnc + Xfwm4、
独立 D-Bus／配置／GOCACHE，Pango：7 组窗口场景通过，分别为 resize_cache
软件 1×、folding 的软件 1×与暗色 OpenGL 1.5×、folding_data 的 Basic 软件
1×、structural 的 Modern 暗色 OpenGL 1×、fold_preview OpenGL 1×、
line_collapse 软件 1×。覆盖候选重建、共享视图隔离、隐藏区域删除、搜索显露、
批量编辑与撤销／重做、外部十万候选分步接收、首行预览和长行收起。
所属桌面进程已退出，剪贴板操作仅作用于该显示连接；用户文档、设置和其他桌面
会话未改动。物理 IME、混合 DPI、其他原生 OS 及性能对照未执行。
日志和产物位于 /tmp/sourceview-folding-split-xqre5fl8。
提交前另从暂存区导出独立源码，GOWORK=off go test ./... 通过。
