# 编辑器后续三批实施

2026-11-12 用户授权按顺序推进：多光标交互收尾、保存点、特殊字符可见表示。
其他新增能力暂缓，包括整行剪贴板、补全呈现、拖放、整理命令、范围装饰、诊断和片段。
实现留在本仓，不修改 GOUI，依赖 GOUI v0.7.0，不使用本地工作区掩盖独立依赖问题。

| 批次 | 状态 | 设计与范围 |
| --- | --- | --- |
| 一 | 已完成 | [多光标交互](DesignMultiSelection.md#多光标交互收尾)：选区撤销、跳过匹配、单独移除 |
| 二 | 待实施 | [保存点](DesignSavePoint.md)：历史身份、Document 基线与信号、示例可见呈现 |
| 三 | 待细化并实施 | 特殊字符表示：按原生几何显示位置，保留正文、命中、选区及输入契约 |

## 第一批验收

新增公共 UndoSelection / RemoveSelection(index) / SkipNextOccurrence，以及对应 Command 和 ui 别名。
选区历史最多 64 步、合计 16384 个光标状态；拖动合并为一步，正文修订/文档切换/卸载清空。
输入经正常事件分发；示例 Selection 菜单及提示有实际入口，Linux 继续用 Ctrl 添加/点击移除。
默认 Ctrl/Cmd+U 选区撤销、Ctrl/Cmd+Shift+D 跳过；移除主选区无默认快捷键。

通过：go test ./...、GOWORK=off go test ./...、GOWORK=off go vet ./...、go test -race ./sourceview ./ui。
定向断言完整集合与方向/亲和/垂直目标 X、只读、独立正文历史、回调重入、跨文档隔离、容量、
主选区提升、最后一个光标保留、跳过并保留其他匹配/回绕/无候选、拖动合并和静止指针。
日志 /tmp/sourceview-interaction-{target,all,independent,race}.log。

Ubuntu 22.04.5 amd64 / Go 1.25.8 / GOUI v0.7.0；独立 TigerVNC / Xfwm4 4.16.1 compositor on /
Pango / Software。多选区窗口 Basic、Modern Light/Dark ×100%/150% ×默认/`-clipboard` 共十二组通过。
新增字面事件断言：点击已有光标移除、Ctrl/Cmd+U 恢复、含多次移动的普通拖动一次撤销、
Ctrl/Cmd+Shift+D 跳过及选区撤销，保留全部旧场景断言。日志 /tmp/sourceview-interaction-finish-wx5vih0c/。
复制剪切只覆盖这些专用显示的剪贴板。

示例以真实 XTest 输入并人工查看截图（Modern Light 100%）：Ctrl 点击南行添加后再次 Ctrl 点击移除，
计数 2→1，Ctrl+U 恢复 2；选中 product 后 Ctrl+D 添加第 15 行，Ctrl+Shift+D 换到第 16 行，
第 12 行匹配保留，计数仍为 2；Ctrl+U 恢复第 15 行。多次移动的普通拖选后一次 Ctrl+U 恢复前述
完整两个匹配。该部分为截图人工核验，不冒充程序断言。截图 /tmp/sourceview-interaction-native-znxpbsf8/。
专用应用、桌面及会话均已退出；不修改用户已有桌面或样本。

Windows amd64、macOS arm64 在 GOWORK=off / CGO_ENABLED=0 下构建 example/editor 和 multiselection 程序通过。
未执行这些平台的原生窗口、真实 IME 服务或物理键鼠，不作性能结论。
