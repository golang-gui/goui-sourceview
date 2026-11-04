# 外部高亮快照

## 职责与公开接口

外部语法/语义分析属于应用；SourceView 接受分析后的颜色范围，不依赖分析器、LSP
或项目索引。语言定义继续负责增量词法、上下文、配对、折叠和默认补全规则。

```go
type HighlightSpan struct {
    Range textedit.Range
    Kind  language.Kind
}
type Highlights struct { /* 不可变索引 */ }
func NewHighlights(text string, spans []HighlightSpan) (*Highlights, error)
func (d *Document) SetHighlights(revision uint64, h *Highlights) (bool, error)
```

坐标为完整文档的规范化 UTF-8/LF byte offset。范围可以跨行，必须非空、按位置递增、
互不重叠且位于 rune 边界；Kind 必须是合法样式键。构造器不排序或吞掉错误输入，
复制数据并建立索引，不持有原文、不访问 GUI，可在 worker 使用。
`KindPlain` 是普通前景的显式覆盖；取消局部覆盖应在下一份快照省略该范围。

Document 拥有一个外部层。首版使用完整快照替换，不提供多 owner、优先级注册、
增量编码或通用装饰体系。`nil` 清除整个层。零值 Highlights 代表空文档的空快照。
ui 提供同名类型别名与 NewHighlights 包装；不把模型颜色复制为声明式控件状态。

## 发布、身份与生命周期

构造器耗时随文本及范围数量增长，应在后台分析阶段完成。SetHighlights 在文档的
所属序列调用，只验证修订和长度，交换索引并通知挂载视图；不会复制或遍历全部范围。

- 修订不同，包括过期的 nil 清除：`false, nil`，保持当前颜色。
- 当前修订且长度不同：返回错误，保持当前颜色。
- 当前修订且有效：`true, nil`。不改 Model.Revision、历史、选区、滚动、语言定义、
  词法缓存、折叠、配对或补全，也不发文本变更信号。

调用方必须确认 **Document.ID、Revision、分析 generation**。相同修订也可能发生配置、
语言或项目索引变化，控件不推断这些身份。text 必须是该修订的准确文本；长度相同不能
证明内容相同。绘制查询仍核对模型 rune 边界，防止误用把非法字节范围传入原生后端。

挂载时复用 Document 已有的模型订阅；没有视图时不为了颜色永久订阅模型。再次挂载若
发现错过修订，则丢弃旧显示提示。跨文档共享同一 Highlights 不共享可变坐标或原生资源。

## 编辑后的显示提示

外部结果延迟到达时，保留未受直接编辑影响的旧颜色，以减少闪烁：

- 插入在范围起点：原范围整体后移；插入在末尾：原范围不延伸。
- 插入在内部、删除或替换与范围相交：丢弃整个相交范围。
- 后续范围根据实际编辑的字节差平移；Change.Edits 严格按模型规定的执行顺序处理。
- SetText 重载、切换语言、丢失修订链：清除；撤销/重做按普通编辑映射，不恢复旧语义结果。
- 同修订或更新修订的结果若已在较早的模型回调中发布，迟到的通知不得重复映射。

这些是绘制提示，不能作为补全、诊断、配对或折叠的语义依据。应用继续分析最新快照。

私有索引采用持久化 treap。后台按有序输入线性构造；每次编辑仅复制拆分/合并路径，
相交子树整体丢弃，后缀使用懒偏移。期望编辑代价 O(log N)，可见查询 O(log N + K)，
不为一次插入扫描全部语义范围。当前快照 O(N)，被调用方保留的旧快照由其负责释放引用；
Document 不保存快照历史。颜色键缓存每视图最多 256 项，换样式时清除。

## 绘制与配色

前景顺序：普通前景 → 词法颜色 → 外部颜色 → 显式选区前景。默认保留语法颜色的选区
仍使用原有半透明背景。外部高亮不改变 Token.Class、Context 或其他背景装饰。

外部 Kind 不必注册进 Language.Styles。显式 Scheme 通过同一 Kind 取色；否则从
`source-view`（或控件自己的 StyleName）的 `token-<kind>` part 解析并按视图缓存。
发布快照不触发 updateScheme，不会因为新颜色键出现而关闭补全。

段落仅增加颜色状态身份，继续比较最终前景范围；相同有效颜色不会重复设置原生属性，
几何缓存键不包含语义快照。长段落只查询已准备的可见片段。IME 临时文本不套用模型颜色，
未替换的前后缀按 Projection 映射保留颜色，避免整篇语义着色因预编辑消失。

色块地图查询同一外部层，版本进入采样缓存键，仍使用原有每帧行数/字节/位图上限，
同文档颜色替换期间保留旧位图，不触发全文排版或同步全文重绘。
纯颜色没有新的语义交互动作，不伪造诊断或可点击语义节点。

## 参考与取舍

2026-11-04 核对的公开文档：

- [VS Code semantic highlighting](https://code.visualstudio.com/api/language-extensions/semantic-highlight-guide)：
  语义层可延迟覆盖词法层。采用分层，不移植 provider 注册、token 编码或 delta 协议。
- [Scintilla styling](https://www.scintilla.org/ScintillaDoc.html#Styling)：
  容器可以提交范围样式；采用外部分析与呈现分离，不照搬数值消息和字节样式位。
- [GtkSourceTag](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.Tag.html)：
  文本范围标签与呈现分离。这里只需要前景快照，不引入 GTK 标签对象或通用装饰体系。

上述是接口职责参考。持久化索引和生命周期规则是本项目的实现选择，不宣称参考编辑器
使用同一种数据结构。无需修改 GOUI 或 typography。

## 验证范围

`highlights_test.go` 覆盖 UTF-8/跨行/范围校验、输入所有权、过期及错误发布、编辑边界、
随机编辑与独立数组预期、回调重入、脱离视图再挂载、共享视图/快照、撤销重做、IME、
前景覆盖顺序、自定义主题键、地图替换及原生几何复用。基准单独测发布和索引映射，
不作为真实输入、分析器或窗口延迟的结论。

真实外部分析与延迟补全的窗口验证见 [外部分析示例](DesignExternalAnalysisExample.md)。
