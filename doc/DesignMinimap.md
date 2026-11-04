# 色块地图

## 定位与参考

色块地图是 SourceView 内部的可选导航区域，默认关闭，不公开单独的控件类型。
采用 Monaco 的色块概览思路，不渲染可读的小字号文字，也不建立第二套文本编辑器。
参考核对日期 2026-11-04：

- [Monaco minimap options](https://microsoft.github.io/monaco-editor/typedoc/interfaces/editor_editor_api.editor.IEditorMinimapOptions.html)：renderCharacters=false、maxColumn 和不同压缩模式。首版选择有界、整篇适配的色块概览，不复制全部配置。
- [VS Code minimap 源码](https://github.com/microsoft/vscode/blob/4c32afe38baa636e3c29fd16c99169eaf87259b7/src/vs/editor/browser/viewParts/minimap/minimap.ts)：令牌颜色及视口覆盖层分离，参考其呈现职责。
- [GtkSourceMap API 5](https://gnome.pages.gitlab.gnome.org/gtksourceview/gtksourceview5/class.Map.html)：绑定主视图的导航概览。其复用 SourceView 子类的方案不适合本项目的大段落测量约束。
- [Scintilla markers](https://www.scintilla.org/ScintillaDoc.html#Markers)：侧栏标记与正文位置解耦；不把色块地图当成断点业务模型。

## API 与布局

ShowMinimap / SetShowMinimap 控制可见性；MinimapOptions / SetMinimapOptions 配置
Width（默认 120 DIP，40..320）、MaxColumns（默认 120，1..256），零值选择默认。
非法配置返回错误并保持原值。ui 完整提供同名声明式设置和类型别名；省略保留现状。

地图固定在正文右侧、ScrollView 的滚动条内侧。统一 textRight 扣除地图宽度，参与
自动换行、正文绘制裁剪、可见布局范围、横向显露和浮动查找栏定位。窄窗口缩减地图
实际宽度，为侧栏和正文保留至少 80 DIP；过窄时地图暂不占位。
切换宽度沿用正文按宽度缓存布局及旧高度估计，单独改变采样列数不使正文几何失效。

地图使用正文现有高度索引及折叠后的可见行映射。短文件保持每个视觉行约 2 DIP，
长文件压缩为视口高度；不创建离屏原生 TextLayout，不要求全文高亮完成。
未测量段落使用正文的估计高度，长折行段落按高度比例选择 UTF-8 安全片段。
这是内容密度/颜色概览，不承诺逐字或逐视觉行的精确缩小，不参与正文换行和光标定位。
压缩采样可能略过细小文本；颜色未就绪时用普通前景色，后续逐步刷新。

## 调度与缓存

普通 CPU 位图每视图持有，最多 256×2048×4 字节；每两个像素行采样一段，
每段最多读取 4096 字节、最多显示 MaxColumns 个单元。宽字符按单个概览单元表示。
缓存键包含高亮行稳定 ID、token version、语言 epoch、方案、采样偏移、列数和 TabWidth。
保留最多 2048 段；编辑不改变其他行 ID，未改变的片段可复用。达到上限清理缓存。
切换文档/语言/配色清理缓存，几何或 revision 改变重新分桶，同一文档保留现有位图渐进替换，仅像素改变时请求重绘；不继续显示旧文档位图。
卸载/隐藏释放全部缓存，视图间不共享可变状态。

地图任务复用既有分析定时器，每个事件循环回合最多 16 个采样段并遵守时间预算；
不逐文档行遍历。正文布局和输入仍在原有帧调度内完成。高亮进展请求下一轮地图刷新，
不为了地图主动分析未就绪的行。绘制只上传已生成的有界位图，再画视口和执行位置；
原生 Image 帧内创建销毁，兼容 RenderWidget 的独立 Painter，不缓存跨 Painter 对象。
非关键缩略图上传失败时只保留背景与导航覆盖层，下一次绘制重试，不弹出错误窗口。

## 输入、样式与语义

点击/拖动通过 ScrollIntoView 请求现有 ScrollView 滚动，保留横向偏移、焦点、
光标和选区。拖动锁定起始总高度和滑块比例，避免异步测量导致指针映射跳动；
文档编辑、尺寸变化、取消、隐藏和卸载终止拖动。滚轮沿正常 ScrollView 路径处理。
地图区域不触发正文菜单或文字选择。预编辑期间保留区域但暂停采样和导航，避免投影
坐标误当成文档行；提交后恢复。地图使用独立 minimap 角色，只声明真实支持的 click
动作，不伪造文本编辑或键盘动作；快照描述区域、视口和 pending，预编辑时标为禁用。
viewport 属性使用 SourceView 局部 DIP，Bounds 沿用窗口局部 DIP。键盘导航继续由正文提供。

Basic 和 Modern 使用 minimap-background、minimap-viewport、minimap-border 部件，
正文令牌颜色来自同一 Scheme。执行位置用细色条标示；普通行标记仍在左侧标记栏。
首版不增加诊断/搜索/断点专用概览模型，后续可复用公开行标记和实际诊断接口。

## 验证范围

逻辑验证配置、采样预算、UTF-8/Tab、颜色与片段复用、折叠映射、长折行、独立视图、
revision/配色/卸载失效。独立 X11 窗口验证点击拖动、滚轮、选区保持、开关、宽高变化、
短/多行/超长单行、亮暗主题与 1/1.5 缩放。其他平台原生运行不计为本机通过。

## 2026-11-04 验证记录

环境：Linux X11 / 独立 Xvnc :87 / Xfwm4 合成 / 软件绘制 / Pango，GOUI v0.6.0。
使用自己的临时显示服务、DBus 会话和临时目录，结束后回收这些进程；未修改原桌面、
用户文件、剪贴板、显示缩放或系统设置。截图为 example/debug 的实际桌面显示。

- `go test ./...`、`GOWORK=off go test ./...`：通过。
- `go test -race . ./ui -run 'LineMark|ExecutionAnchor|Minimap' -count=1`：通过。
- `GOWORK=off CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`：通过。
- `GOWORK=off CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...`：通过。
- `tests/window/editor/minimap`：Basic、Modern light/dark × 1/1.5 六组通过。
  包括点击、拖动、滚轮、焦点/选区保持、宽高改变、关闭再开启、480000 字节单行的
  中间输入、输入后采样收敛及远处导航；通过 RenderWidget 和桌面截图检查呈现。
- `tests/window/editor/line_marks -theme dark -minimap`：1/1.5 两组通过，组合验证
  行标记请求、折叠汇总、执行位置与标记叠加、RevealLine 及编辑后的锚点跟随。
- Windows/macOS 原生窗口未运行；交叉构建不能代替原生运行。

窗口程序日志中的输入耗时从 Window.DispatchEvent / CommitText 入口计至返回，
阶段耗时还包含定时器等待、正文排版、高亮和地图工作，不等同于地图自身耗时或上屏时间。
本机长行输入仍观察到数十毫秒的同步处理；本批没有据此宣称解决全部正文编辑延迟，
也没有把单次数字作为跨平台性能保证。地图自身不新增全文排版和同步全文扫描。
