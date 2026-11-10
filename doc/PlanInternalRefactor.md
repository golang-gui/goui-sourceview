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
