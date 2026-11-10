# 统一折叠栏与结构首行预览验收

2026-11-10；实现前基线 `f53248c`。设计见 [长行收起](DesignLineCollapse.md) 与 [结构折叠](DesignFolding.md)。

后续将长行门槛调整为实际至少 3 个显示行，新增边界验收见 [长行门槛](PlanLineCollapseThreshold.md)。本文保留统一入口的首次验收记录。

## 缺陷与修正

原始样本为约 158 KB、含 Base64 PNG 的有效单行 JSON。此前长行只能从右键菜单收起；在 PNG 字符串引号之前
插入两次换行后，结构分析产生逻辑行 0..2 的根范围。箭头隐藏后两行，却保留首逻辑行的全部软换行，产生折叠错位的观感。
实测未发现该箭头的 Y 坐标或逻辑行范围计算错误。

现已为自动换行的长行提供左侧箭头，普通文本同样可用。结构收起时其首逻辑行也压为一个原生显示行；
同一逻辑行最多一个箭头。长行与结构意图重叠时，箭头、预览省略标记及结构展开命令一次清除两种意图。
搜索、选区和导航进入隐藏首行后缀时恢复结构；IME 目标段落在建立投影前展开并核对回调后的生命周期。
结构派生预览不写入手动 `CollapsedLines()`，不改变语言规则、原文、修订、撤销历史或共享视图状态。

## 逻辑与原生排版回归

[`fold_preview_test.go`](../fold_preview_test.go) 覆盖：

- 无语言结构规则的长行箭头、固定栏宽、软换行续行不重复箭头，以及禁用控件仍有不可执行的可见语义。
- 结构首行高度、隐藏正文与快照、选区迁移、单一箭头/省略标记和两种意图一次性展开。
- 搜索/显式选区/下移/End/Delete 边界展开首行；搜索后继隐藏内容时清除重叠意图。
- 收起/展开的首帧当前宽度段落、调整宽度、关闭/恢复自动换行、其他段落 IME 保持预览和信号重入。
- IME 目标首行先展开；展开回调替换 Document 后不得继续预编辑。
- 超过 64 KiB 的结构首行使用首页预览，内部页裁剪不能扩大预览边界，搜索远端页能恢复显示。

使用仓库外的 Go overlay 还原基线实现时，两个新回归分别明确失败于“无折叠列”和“保留完整首行高度”。
overlay 仅将新测试调用的组合操作名映射为基线已有结构操作以使旧代码可编译；断言和样本未改变。
IME 新用例在修正前也明确失败于目标结构未展开及回调后继续预编辑，修正后通过。

首次全量测试发现旧布局用例将正文宽度写死为 viewport 减内边距。已纳入新增折叠栏宽度；
最新视口、行范围和禁止过期宽度排版的断言保留，没有删除测试或放宽条件。

| 命令 | 结果 |
| --- | --- |
| `go test ./...` | 通过 |
| `GOWORK=off go test ./...` | 通过，使用已发布 GOUI v0.7.0 |
| `GOWORK=off go test . -race -run 'Test(Wrapped\|LongLineCollapse\|Fold\|.*IME\|.*Composition)' -count=1` | 通过；最后新增的分页结构用例另行以 race 通过 |
| Windows amd64、macOS arm64，`CGO_ENABLED=0 GOWORK=off go build ./tests/window/editor/fold_preview` | 交叉编译通过，未做原生运行 |

## 本机独立桌面

Linux amd64 / Go 1.25.8 / Pango，私有 Xvnc 24-bit、Xfwm4 合成桌面。临时 Xauthority、配置、缓存、可执行文件与
采样产物均在仓库外；结束后清理本次桌面进程及认证文件，没有操作用户的工作桌面。

新入口：[`tests/window/editor/fold_preview`](../tests/window/editor/fold_preview/main.go)。

```sh
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/fold_preview -file /path/to/single-line.json
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=2 go run ./tests/window/editor/fold_preview -dark
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=opengl GOUI_PLAT_SCALE=1.5 go run ./tests/window/editor/fold_preview
XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/fold_preview -basic
```

四组均通过，第一组实际读取用户报告的原始 JSON；其余使用仓库内生成的约 161 KB 单行 JSON。
经正常 Window 指针、键盘和 CommitText 路径验证：直接打开的长行箭头、收起/展开、在 PNG 引号前插入换行、
逻辑行 0..2 的结构箭头、一行首行预览、点击省略标记展开、隐藏光标迁移、下移恢复、正文变窄/变宽、
关闭/恢复自动换行、F3 搜索恢复及撤销换行。最终切换为无语言折叠器的普通文本再次点击箭头。
原始文件只读，不将临时换行写回磁盘；验证前后 SHA-256 一致。

转发 Painter 对每次实际收起态绘制检查正文存在且使用一行裁剪，四组分别 38 / 33 / 39 / 39 次，共 149 次。
独立 X11 客户区采样通过页脚帧号与真实 Paint 配对，四组各 8 个阶段的首帧均捕获并通过像素检查，共 32 个阶段。
每张图预览区需有正文，下方直到视口底部不得有隐藏文字；排除行号、边框、省略标记。
采样为 8-bit RGB 转灰度，亮色正文阈值 <161、暗色 >150；不使用跨字体抗锯齿金图。
已人工核对原始 JSON 的结构收起截图。没有以离屏渲染替代窗口截图，也没有延长控件过渡帧。

附带原有窗口回归全部通过，总计 10 组：

- `line_collapse`：软件 1× Light / 2× Dark；自定义快捷键、中英文菜单、只读、完整复制、共享视图、导航、编辑撤销、重挂载和分页长行。
- `folding`：软件 1×；结构快捷键、精确指针命中、隐藏边界删除、搜索和共享视图。
- `menus`：软件 1×；提示列、命令映射、本地化、只读/选区及真实菜单动作。
- `settext`：软件 1×；15 种全文替换场景、79 次真实绘制检查。
- `resize_cache`：软件 1×；返回最近宽度复用原生排版。

原 `line_collapse` 回归会在私有桌面临时占有测试剪贴板；随进程/桌面结束，不修改用户桌面的剪贴板。

## 验证边界

无新增 GOUI 能力缺口；沿用长行收起设计中已记录的公开 Painter 裁剪交集适配，没有修改 GOUI。
Windows/macOS 仅交叉编译；未验收物理输入法、真实显示器 DPI 切换或硬件 GPU。
IME 结论来自原生排版的组件回归，不能替代物理输入法测试。没有性能基准或固定排版耗时承诺。
