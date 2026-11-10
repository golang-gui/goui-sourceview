# 内部实现拆分

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
