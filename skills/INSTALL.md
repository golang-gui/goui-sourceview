# 安装 SourceView 应用开发 Skill

本文件供执行安装的 AI Coding 工具阅读。用户可直接发送：

> 请按照 https://github.com/golang-gui/goui-sourceview/blob/main/skills/INSTALL.md 的说明，安装 SourceView 应用开发 Skill。

英文提示：

> Install the SourceView application development skill following https://github.com/golang-gui/goui-sourceview/blob/main/skills/INSTALL.md.

## 安装内容与来源

将官方仓库 `golang-gui/goui-sourceview` 的 `skills/goui-sourceview-app/` 完整目录
安装为 `goui-sourceview-app`，目标目录中直接包含 SKILL.md：

```text
goui-sourceview-app/
├── SKILL.md
└── references/
    ├── integration.md
    └── customization.md
```

INSTALL.md 本身不是另一个技能。GOUI 仓库只提供指引，不保存 SourceView 技能副本。
本次安装不安装/升级 Go 模块，不修改应用 go.mod / go.work，不自动安装 goui-app 技能。

## 安装步骤

1. 确认当前工具及版本支持的原生技能机制，优先使用其安装工具；需要目录时查该工具
   的官方说明或本地配置，不猜测别的产品路径，不向多个候选目录重复安装。
   遵从用户指定的项目级/全局范围；未指定时采用工具明确的默认范围并报告。
   只有范围或路径无法确定时才询问；工具不支持时说明限制，可以直接阅读技能，
   但不能声称已经完成原生安装，也不要改写 AGENTS.md 或添加专属 Agent 适配器。
2. 来源仅为 [SourceView 官方仓库](https://github.com/golang-gui/goui-sourceview)。
   遵从用户指定的 tag、分支或 commit，未指定时使用 main；解析后尽量固定到同一 commit
   获取入口与全部参考。原生安装器支持子目录时使用 `skills/goui-sourceview-app`；否则
   在仓库外临时下载/检出该目录，再按工具机制安装。用户已认可的本地检出也可作为来源。
3. 检查下载内容、frontmatter 和相对引用。复制完整技能目录，不只复制入口、不安装整个仓库、
   不执行仓库脚本。来源不可确认、网络失败或所选 revision 尚无技能时报告具体原因，
   不换用未知镜像，也不把“尚未发布”解释成工具不支持。
4. 先检查安装目标。内容一致时报告已安装；存在不同版本或本地修改时保留原内容，
   说明来源和差异，已有明确更新授权才覆盖。安装失败时保留旧技能，避免留下半份可发现目录。
5. 检查 `name: goui-sourceview-app`、非空 description，以及 references 的存在与可读性。
   使用工具提供的校验/枚举机制；区分文件安装、技能发现和本会话已加载。
   报告安装范围、实际位置、来源 revision、验证结果及工具要求的刷新/新会话步骤。

使用提示：“请使用 goui-sourceview-app Skill，帮我给 GOUI 应用加入一个可定制的代码编辑器。”
只有工具支持 `$技能名` 语法时才使用 `$goui-sourceview-app`。
技能更新和应用依赖版本独立；不因技能内容较新而擅自升级依赖。
