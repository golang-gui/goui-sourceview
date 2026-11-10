# 长行收起验收

2026-11-09；实现前基线 `545c567`。对应 [长行收起设计](DesignLineCollapse.md)。

2026-11-10 补充：左侧长行箭头与结构首行预览已统一，验收见 [统一折叠预览](PlanFoldPreview.md)。本文保留首次交付记录。

## 交付范围

- 语言无关的手动长行收起：默认展开，自动换行下已测量高度达到约 8 行时提供入口。
- 保留原文和原生段落缓存，显示首个软换行及可点击省略标记；字形边界对齐原生 cluster。
- 右键菜单仅显示相关行的一个操作；新命令可自定义快捷键，文案含现有八种语言。
- 搜索、选区、导航和编辑进入隐藏内容时展开；只读、复制完整原文、同文档双视图独立、调宽/调窄及重挂载可用。
- IME 目标行先展开；投影中的其他逻辑行保持收起，合成期间暂停标记动作，预编辑不改提交文本。

没有新语言解析、单行内部的嵌套范围折叠、全文格式化或 GOUI 仓库变更。依赖仍为 GOUI v0.7.0。

## 环境与命令

Linux amd64、Go 1.25.8、Pango、独立 Xvnc 24-bit / Xfwm4 桌面；软件渲染和 OpenGL。
临时 Xauthority、XDG 配置/缓存和日志在仓库外；结束后清理所属桌面进程和认证文件。

| 验证 | 结果 |
| --- | --- |
| `go test ./...` | 通过 |
| `GOWORK=off go test ./...` | 通过；不依赖本地 go.work / replace |
| `go test -race . -run 'TestLongLineCollapse\|TestResetInitialization\|TestNativeResetPaintsAtAllocatedWidth' -count=1` | 通过 |
| `go test -race ./internal/... ./language/... ./scheme/...` | 通过 |
| Windows amd64 / macOS arm64，CGO 关闭、GOWORK=off 的根包 `go test -c` 和新窗口入口 `go build` | 交叉编译通过；未做原生运行 |

[单元与原生排版用例](../line_collapse_test.go)覆盖字节锚点映射、修改/重置/文档切换、原生缓存复用及销毁数、
选区与导出、RTL affinity、CJK/组合字符 cluster、分页段落、IME 投影及回调重入、语义快照和调宽后的首帧排版。

## 真实窗口

入口：[`tests/window/editor/line_collapse`](../tests/window/editor/line_collapse/main.go)。

```sh
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/line_collapse
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=2 go run ./tests/window/editor/line_collapse -dark
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=opengl GOUI_PLAT_SCALE=1.5 go run ./tests/window/editor/line_collapse
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/line_collapse -basic
```

上述四组配置均通过：正常 Window 事件通道的 F6/F7 自定义键、中英文右键菜单、Command/Shortcut 提示、省略标记点击、
双视图、只读、完整复制、搜索、下移、编辑与撤销、窗口内面板变宽/变窄、重挂载、卸载后文本变化、
单行 JSON、Go 长字符串、超过 64 KiB 的折行段落及从后一段向上进入其真实末页。复制只影响此私有显示会话的剪贴板。

窗口用例以转发 Painter 观察每一次真实绘制：已收起行必须使用当前宽度的原生段落画出正文，不依赖 RenderWidget。
独立 X11 采样另通过页脚帧号配对实际客户区像素，覆盖四组配置各 10 个收起阶段：
普通收起、菜单、中文菜单、只读、窄面板、恢复宽度、重挂载、JSON、Go 和分页长行。
预览文字必须存在，预览与后续短行之下的空白区不得出现隐藏内容。
最终一次矩阵共通过 177 次真实收起态绘制断言；40 个阶段的首帧均捕获并通过像素检查。

像素取样使用 X11 客户区 RGB 图像和标准 8-bit 灰度转换，预览区域排除行号、边框和省略标记；检查下方空白时
排除预期的 AFTER/TAIL 短行。亮色正文阈值 <161，暗色 >150；不使用跨字体抗锯齿金图。原图采样后保留，窗口关闭后再压缩 PNG，
避免编码延迟使采样错过短帧；没有人工延长控件的过渡帧。人工核对了普通长行与分页长行的截图。

## 相关回归

- 现有 `tests/window/editor/menus`：软件 1× Light / 2× Dark，提示列对齐、语言切换、无效映射、活动菜单更新等通过。
- `tests/window/editor/settext`：软件 1× / OpenGL 1.5×，各 15 种全文替换场景通过。
- `tests/window/editor/resize_cache`：软件 1×，返回最近宽度不重新排版通过。
- `tests/window/editor/horizontal_scroll`：软件 1×，普通/跨分页阈值/已分页长行的滑块拖动、撤销和再次显露光标通过。

## 实现过程中发现并修正的问题

1. 从后一段向上进入收起的分页行时，初版落到首页字节 4035，不是真实最后显示行。新回归用例明确失败后，
   修正为展开并按需获取末页几何，单元及真实窗口已验证。
2. 早期菜单常驻增加两项后，中文 2× 菜单变高，原窗口测试仮定可一次读取全部项而超时。
   诊断确认 GOUI 正常使用虚拟化可见菜单行，不是命令丢失。产品调整为只在相关长行显示一个操作，位置在结构折叠组前。
   原菜单回归测试文件保持原样，另有新窗口用例验证中英文的新操作。
3. 逐帧绘制断言捕获到恢复宽度时的空白预览。修正为当帧布局准备已收起行的新宽度；普通段落仍用帧预算。
4. 像素检查发现分页正文可在一行预览以下继续绘出：GOUI v0.7.0 的页内 SetClipRect 会替换外层显式裁剪。
   该组合裁剪限制与公开 Painter 交集适配已记录在 [设计](DesignLineCollapse.md)；新增页内裁剪回归和像素检查通过，未修改 GOUI。

## 边界

上述原生结果适用于 Linux / Pango、该独立 X11 桌面和指定后端/缩放。没有 Windows/macOS 原生运行、物理键盘/IME、
混合显示器 DPI 或硬件 GPU 验收。IME 是组件投影和回调边界的单元验证，不代替物理输入法。
没有性能基准结论；预览高度有限不等于单次原生排版可中断或有固定耗时。
