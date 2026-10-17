# Basic / Modern 接入验收（2026-10-17）

设计见 [DesignStyles](DesignStyles.md)。GOUI 的通用注册入口在 `903daf8`；
SourceView 编辑区和 FindBar 在 `29e0255`，补全及 Scheme 边界调整在 `8c79d3f`，未改变外观的样式通知保留补全在 `495f0a5`。
两份功能提交均额外将暂存树归档到临时目录，使用同一 GOUI 检出执行 `go test ./...`，
确认各自提交可编译和通过包测试。示例与本记录独立提交。

## 实现范围

- 根包自动注册 Basic / Modern，应用正常选择 Modern 即生效，无额外适配包。
- 默认代码色来自当前 stylesheet；显式 SetScheme 只影响代码，nil 恢复样式。
  纯颜色变化保留文本几何缓存，字体变化按原路径释放；关闭弹窗回调后重新读取当前外观。
- FindBar 使用自身所在样式树，去掉内部固定 Basic sheet；字体、焦点、圆角及阴影
  来自注册规则。匹配计数、选项按原生字体测量，避免 Modern 或大字体下裁切。
- 补全从所属编辑器解析样式，保持局部作用域；使用 UI 字体、主题色匹配前景与独立列表色。
  本次默认补全仍为紧凑矩形列表，不增加新的透明窗口或定位机制。
- 移除 Scheme 的 Find/Completion 调色板以及自动代码配色 Settings 订阅。
  原 UI 配色测试迁移到规则所属控件，保留颜色独立性、不透明默认与装饰 alpha 断言。
- 命令式示例切换整张 Modern 表；声明式示例通过 Root.StyleSheet 跟随设置重建。

## 通过

Linux 环境：Ubuntu 22.04.5、amd64、Go 1.25.8，原生排版 Pango。

```bash
go test ./...
go test -race ./...
go vet ./...
```

包测试涵盖登记完整性、token part 对应、并发构造及旧快照、同字体换色不重排、
代码字体/前景覆盖、显式 Scheme、回调换方案/作用域/文档、UI 色与代码选区隔离、
字体/焦点/圆角、装饰透明度、Popover 所有者作用域与原生资源释放。
原有补全分列几何、逐字颜色、cluster 省略、滚动及取消断言继续通过。

补充回归将“未改变外观的样式通知应保留补全”断言加入修正前代码，确认失败；
修正后通过。窗口程序在打开补全后经 Window.DispatchEvent 移入/移出编辑区，
跨帧确认会话仍存在，再继续候选交互；避免只模拟 Popover 内的悬停而漏掉编辑器状态变化。

窗口使用临时 TigerVNC / Xfwm4（compositor off），`XMODIFIERS=@im=none`。
以下矩阵均通过正常 Window / Popover 事件分发，不使用 DevServer：

| 后端 / 缩放 | 通过场景 |
| --- | --- |
| Software / 1x | find_appearance 默认及 18 pt；completion json/dark、long/dark、unicode/light；find 行为和声明式协调 |
| Software / 2x | find_appearance；completion scroll/dark；find 行为和声明式协调 |
| OpenGL / 1x | find_appearance；completion unicode/light；find 行为和声明式协调 |

程序位于 `tests/window/editor/find_appearance`、`completion`、`find`。
Find 外观断言包括亮暗原生像素、两侧主题隔离、SetScheme 不改变 UI、折叠/展开、
窄宽度恢复、控件文本完整分配、焦点与普通命中；尺寸检查按原生输入测量关系，
不以统一字体金图代替几何。像素采样为 sRGB 不透明内部平面，避开文字、边框与 AA。
补全自动场景验证编辑器悬停保留、宽度稳定、键盘/悬停/滚轮、留白不接受、指针接受、焦点及撤销。
临时桌面、窗口随脚本退出清理；未更改用户桌面、剪贴板或系统设置。

远端使用本任务归档，在独立临时目录联调：

| 平台 | 工具链 | 通过 |
| --- | --- | --- |
| Windows 10 19045 / amd64 | Go 1.24.0 | SourceView `go test ./... -count=1`；GOUI theme/basic、theme/modern、gui、widgets、ui；三个窗口程序构建 |
| macOS 15.7.7 / arm64 | Go 1.24.9 | 同上，使用对应原生排版实现 |

远端离屏排版后端分别为 DirectWrite 与 CoreText；没有执行桌面缩放验收。

三个构建目标是 `tests/window/editor/shortcuts`、`find`、`find_appearance`。
远端包测试没有依赖桌面，不赋予 DPI/合成器验收结论。

## 失败与未验证

`GOWORK=off go test ./...` 仍失败：go.mod 的可获取基线 GOUI v0.1.1 不含
`textedit.Model.ApplyBatch` / `textedit.BatchOptions` 等既有依赖扩展，也没有本次 Register。
该条件不是本轮新失败。所需 GOUI 改动尚未发布，未伪造版本、提交机器 replace 或将工作区
测试算作独立发布验证。

GOUI `go test ./...` 和主题 race 通过，但 `go vet ./...` 有 222 条既有诊断；
修改前 HEAD 加原已存在的相关未跟踪测试后逐条对照一致。详情见 GOUI 的
[注册验收](../../goui/doc/PlanThirdPartyStyles.md)。已有文档改动、未跟踪测试未纳入 GOUI 提交。

Windows/macOS 实际桌面外观、物理输入、IME、分数及混合 DPI 尚未验收；
Linux 本轮运行不覆盖真实输入法候选窗口或桌面合成效果。
