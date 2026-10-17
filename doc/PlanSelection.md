# 代码选区与语法色验收（2026-10-17）

设计见 [排版与配色](DesignLayout.md) 和 [样式接入](DesignStyles.md)。
功能修正在 SourceView `ac7870b`，前序基线为 `2f99ba9`；窗口验证和本记录另行提交。
本轮没有修改 GOUI，工作区依赖为 `903daf8638063bba1a2fb23ec18ff10d545d5fb4`。

## 实现范围

- Basic、Modern 亮/暗默认保留语法前景；活动选区背景 alpha=64，非活动为 32。
  背景保持真实透明度，在文字下方叠加到当前行、括号或搜索装饰。
- 复用 `SelectionForeground`：alpha=0 表示不覆盖并规范化为零值，255 为显式统一前景，
  1–254 拒绝。透明前景不会传给原生排版。注册规则显式写入零值，阻止根前景继承。
- Modern 使用主题的输入焦点边框色作为选区 tint；FindBar、补全及普通 TextView 的选区契约不变。
- 默认选区移动和焦点变化只重绘背景；仅真正生效的前景覆盖进入原生着色身份。
  空范围规范化，避免显式模式下无选区的 caret 移动重复着色。
  实际焦点另行捕获，保留原生调用后的生命周期与重入有效性检查。
- 原有 editor、experience 窗口程序显式配置统一前景，保留局部覆盖、原生位图及几何断言。

`ac7870b` 的暂存树另行归档，在临时工作区使用同一 GOUI 检出执行 `go test ./...`，
确认该功能提交本身通过，不依赖下一笔新增窗口程序。

## 回归证据与断言

将新增回归放到修正前代码执行：

```bash
go test . -run '^TestSelection' -count=1
go test ./scheme -run '^TestOptionalSelectionForeground$' -count=1
```

两条均失败：旧默认选区仍有统一前景、无法恢复语法前景，移动选区会触发原生重着色；
内置方案也不满足可选前景契约。修正后通过，另验证 alpha 边界、忽略透明 RGB、
根样式前景隔离、显式覆盖及重复样式通知的方案缓存身份。

新窗口程序在 `tests/window/editor/selection`，入口记录运行环境、操作、预期、副作用和退出规则。
固定样本与独立预期在其 `testdata/syntax.json`；由本项目编写，含中文、组合字符、emoji，
不从词法扫描器导出。窗口通过正常 `Window.DispatchEvent` 验证：

- 三语言分别在 Basic、Modern 亮/暗下全选、移动、反向选择、Tab 失焦及恢复。
- 显式前景覆盖、取消、空选区 caret 移动、失焦恢复、上下文菜单活动和恢复默认。
- 默认选择/焦点变化不调用原生颜色 setter，所有选择变化保持原生布局身份与创建数。
- 逐 byte 前景与独立设置预期色的同后端原生位图一致；不以跨平台字体金图作预期。
- 尾部空格内部采样验证当前行/搜索背景与选区的 source-over；排除 glyph AA、caret 和边框。
  颜色为 sRGB，目标不透明；仅为预乘及 8-bit 量化允许逐通道 1 byte 误差。
- 文档原生布局不重复销毁、不在释放后着色；窗口销毁后存活计数归零。

这些计数断言验证失效边界，不构成整编辑器性能或内存规模结论。

## 通过

本机：Ubuntu 22.04.5 / amd64、Go 1.25.8、Pango。

```bash
go test ./...
go test -race ./...
go vet ./...
```

Linux 窗口在临时 TigerVNC / Xfwm4（compositor off）运行，`XMODIFIERS=@im=none`。
分别设置 `GOUI_PLAT_PAINTER` 和 `GOUI_PLAT_SCALE`，依次执行：

```bash
go run ./tests/window/editor/selection
go run ./tests/window/editor -auto
go run ./tests/window/experience
```

| Painter / 缩放 | 结果 |
| --- | --- |
| Software / 1x | 三语言 × 三主题共 9 组选择/前景/像素断言及两个既有程序通过 |
| Software / 2x | 同上，渲染图像比例实测为 2 |
| OpenGL / 1x | 同上，渲染图像比例实测为 1 |

新增程序在补齐资源释放和空选区移动断言后重跑完整矩阵，27 组全部通过。
亮暗截图可见选中代码仍保留不同 token 色。临时桌面和窗口退出时清理，
没有修改用户桌面、剪贴板或系统设置；PNG 与构建产物位于仓库外。

远端使用本任务 SourceView 归档与 GOUI 上述提交的干净归档，在独立临时工作区执行：

| 原生平台 / 工具链 | 结果 |
| --- | --- |
| Windows 10 19045 / amd64、Go 1.24.0、DirectWrite | SourceView `go test ./... -count=1` 通过；GOUI 相关包测试通过；五个窗口程序构建通过 |
| macOS 15.7.7 / arm64、Go 1.24.9、CoreText | 同上 |

GOUI 相关包为 `./theme/basic ./theme/modern ./gui ./widgets ./ui`；
构建目标为 `tests/window/editor/shortcuts`、`find`、`selection`、`editor` 和 `tests/window/experience`。
远端没有运行窗口场景，不据包测试或构建赋予桌面、缩放或像素验收结论。

## 失败与未验证

`GOWORK=off go test ./...` 仍失败：可获取的 GOUI v0.1.1 缺少
`textedit.Model.ApplyBatch` / `textedit.BatchOptions` 等既有依赖扩展。
这是原有发布阻塞，尚需发布所需 GOUI API 后更新依赖；没有提交机器专属 replace 或伪造版本。

Windows/macOS 桌面外观与输入、真实物理键盘及 IME、分数/混合 DPI 未执行。
Linux 窗口矩阵不覆盖桌面合成器、系统高对比度或全部字体组合。
显式不透明前景覆盖已验收，但不据此承诺已适配系统高对比度主题。
