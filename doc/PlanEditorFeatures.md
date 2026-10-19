# 折叠、编辑辅助和标记语言实施记录

2026-10-19；基线 SourceView 1f0ef1b，正式依赖 GOUI v0.2.0。
设计：[折叠](DesignFolding.md)、[编辑辅助](DesignEditorAssistance.md)、[标记语言](DesignMarkup.md)。
用户授权写入设计、修改 SourceView、本机验证后分阶段提交；需要 GOUI 改动时先记录要求并跳过。

## 实施切片

- [ ] A1：可见段落映射与原生几何接入。
- [ ] A2：折叠范围、后台分析、JSON/XML/HTML 规则、交互、样式、语义和薄 UI。
- [ ] B：编辑辅助与可选 GoToBar。
- [ ] C1：XML/HTML 标签编辑、正式 Markdown。
- [ ] C2：嵌入高亮/编辑和标签名称联动。
- [ ] D：正则/捕获组替换、独立选区查询范围及多行输入。

每个切片记录实际范围和验证，未执行不写成完成；多光标、通用装饰、语言服务及编程语言后置。
查找增强沿用公开 SearchQuery，Regex=false 保留字面搜索。采用 Go regexp 语义；零长度匹配
必须进展，Find/Count/ReplaceAll 使用同一不重叠序列，替换为一次预算受限原子事务。
选区查询范围独立于 FindNext 改变的选区；编辑后映射或清除，换文档清除。非法表达式栏内反馈。

## GOUI 前置要求

暂无。遇到公开契约缺口时在此记录需求、调用证据、受阻场景及跳过范围，不修改 GOUI。

## 验证记录

待各切片补充。普通与 GOWORK=off 全仓测试；按风险 race、Windows/macOS 交叉构建和本机原生窗口。
跨平台编译不作为原生交互验收。窗口启动、像素断言和实际操作分别记录。
