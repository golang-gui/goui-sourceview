# 独立编辑内核实施与验收

设计见 [独立编辑内核](DesignEditorCore.md)。分支 feature/editor-core，GOUI v0.7.0。

## 第一批：自有文本设施

迁入公开文本设施的实际使用部分到 internal/text，保留来源、版权及 MIT 许可说明，
形成自有正文分段存储、行索引、修订、原子批量修改、差量历史、位置、预编辑投影、
测量片段、编辑几何和高度索引；包内没有 Widget 或原生资源。

迁入对应回归用例，包括独立字符串参考的随机编辑、存储释放、通知重入、批量原子性、
来源与历史选区、Unicode 与长词、投影隔离、双向/折行几何、测量片段所有权及高度索引。
go test ./internal/text 通过；已有工程迁移前 go test ./... 通过。

## 第二批：自有长段落布局

在 internal/view 迁入 GOUI v0.7.0 公开 ParagraphLayout，替换其值类型依赖为
本仓 internal/text；原生 TextLayout、Painter、4 KiB 页面、最多 12 页缓存、
前缀方向上下文、坐标精度及失效算法保持基线行为。实现和迁入回归均保留来源与许可。
uniseg v0.4.7 转为直接依赖，版本不变；不新增字体、语言资源或原生库。

包内新增原生完整段落参考：ASCII、中文/组合字符/emoji、RTL、isolate/override，
验证软折点、cluster、基线、页接缝编辑和巨大坐标下的绘制/命中；原生送测上限 8192 byte。
固定度量检查末块估宽及分页基线；视图隔离检查不同宽度的资源、销毁、宽度及编辑失效。
原生调用在实际测试 goroutine 锁线程，Linux Pango 无桌面测试通过。

`go test ./internal/view` 通过。第三批切换 Document 与使用者后再记录完整集成验收。
迁移基线二进制、样本和日志均在仓库外 `/tmp/sourceview-core-tluj_grl/`。
