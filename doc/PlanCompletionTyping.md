# 持续输入补全修复与验收

2026-11-09，基线 `e536adc`，GOUI v0.7.0。设计见 [持续输入补全](DesignCompletionTyping.md)。
仅修改 SourceView，既有补全设计草稿和其他草稿不纳入提交。

## 问题与实现

普通按键、模型变更、重新请求三条路径都会关闭会话，连续输入重复销毁／创建弹窗。
词内输入／Backspace／Delete 使用一次精确编辑许可延续会话，保留异步预览但禁止旧候选提交。
新快照校验词范围、有效语言定义和请求身份，复用原 Popover，保留仍存在的选中项。
手动、自动、尚未返回首个结果的会话均适用；撤销仍使用原有模型历史元数据。

## 回归证据

日志目录 `/tmp/sourceview-completion-typing-ja_2cvnl/`：

- 基线源码导出后加入同一份公开窗口用例，旧二进制在输入按键时失败
  `typing closed/recreated popup or lost focus`（desktop-before.log）。后续在首帧就绪后的对照再次确认输入前 popovers=1、visible=true、focused=true，
  按键后同一断言失败（desktop-before-final.log）。
- 首次窗口运行未等宿主首帧，初次弹窗未保持并等待超时（desktop-initial.log）。
  该轮未独立断言具体关闭来源，不算通过。
  另一轮在列表增长后、原生尺寸分配前读取可见项导致断言失败（desktop-second.log）。
  新用例先等待宿主绘制／焦点及候选项分配完成，保留全部稳定性、候选和编辑断言。
- 复核新增的 Word 回调切文档用例复现旧输入写入替换文档（word-callback-before.log）。
  查询回调前后校验上下文后通过，retention-v2.log／retention-v3.log 保留结果。

completion_input_test.go 断言词内插入、前后删除、旧候选不可提交、许可只消费一次，
外部／共享视图修改、选区及模型回调重入必须取消；另断言普通文本按键不取消、快捷键仍取消。
Word 回调更换文档时，原文档与新文档均不能接收旧输入。

## 窗口与最终检查

新增 `-case typing -auto` 通过 Window.DispatchEvent／CommitText 验证：
默认／手动会话，连续字符、Shift、中文字符、Backspace、Delete、左右移动、候选恢复与选中保留，
Go 及 Markdown 内嵌 JSON 的实际词法定义保持，
慢速／过期／首个结果尚未返回的 Provider、禁用旧行、提交范围、Undo、词边界／外部修改和回调换文档。
主窗口标题标记稳定阶段；专用 X11 观察器监听原生 Create/Map/Unmap/Destroy，验证没有隐藏再显示，
不只比较 Go 层 Popover 对象身份。

首轮 23 组窗口通过，四种稳定阶段 × 五种主题／后端配置均无原生窗口增删／显隐事件。
补足 Word 回调防护和词内 Delete 后复验通过，再补 Go／嵌入 JSON 的真实语言覆盖：
最终 23 组也全部通过（desktop-code.log）。新增场景分别跑 Basic SW 1×、Modern Light SW 1.5×、
Modern Dark SW 2×、Light GL 1×、Dark GL 1.5×；navigation／scroll／json／long／unicode／wrapped／
edge／html／xml 各 Basic SW 1×、Dark GL 1.5×，回归 18 组。

六种稳定阶段（默认、手动、pending、Go、内嵌 JSON、Unicode）× 五种配置共 30 段，
原生 Create/Map/Unmap/Destroy 均为零，阶段起止齐全。实际后端／缩放、运行程序 SHA256、
全部专用认证文件删除／进程退出、既有七份草稿哈希不变均已核对（postflight-code.json）。

GOWORK=off 全量测试、race、vet、Windows amd64／macOS arm64 CGO_ENABLED=0 交叉构建
两轮均通过（b1-checks.json、b2-checks.json）。功能提交 `b11292f`，独立索引导出 core
的全量测试／vet 也通过。新增 Go／嵌入语言用例后，Windows amd64／macOS arm64 交叉构建再次通过。
窗口用例及记录的独立索引导出 final 的全量测试／vet 通过；最后补写仅改变本验收记录。

Linux amd64 / Go 1.25.8 / Pango，专用 TigerVNC 1.12.0／Xfwm4 4.16.1，隔离 DISPLAY、认证及 XDG 配置。
WM 含 Another compositing manager 警告，不宣称硬件 GPU 或桌面合成验收。
物理输入法、混合 DPI、其他系统原生运行及性能对照未执行；交叉编译不代替原生运行。
