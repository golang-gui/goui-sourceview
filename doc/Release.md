# SourceView 发布流程

参考 [GOUI 发布流程](https://github.com/golang-gui/goui/blob/main/doc/Release.md)，
按本模块的交付形式执行：准备并提交 → 冻结源码 → 独立验证 → 推送标签 →
核对 Draft → 正式发布 → Go proxy 与消费方复核 → 文档收尾。
发布授权来自维护者的明确指令，读取本文本身不授权提交、推送或发布。

## 交付与依赖

- 默认发布 Go 库及仓库中的源码示例。示例编译和运行验证不等于分发二进制 Demo；
  未明确发布二进制时，不创建安装包、签名、公证或空的附件校验文件。
- 标签使用 `vX.Y.Z`，首次正式版本为 `v0.1.0`。0.x 的公共契约变化同样写入
  中英文 Release Notes；不把设计规划或未验证的平台写成已交付能力。
- go.mod 使用已发布、实际可获取的 GOUI 版本，禁止提交本机 replace 或 go.work。
  更新依赖后检查 tidy、go.sum 和直接/间接分类，不顺带升级无关依赖。
- 检查许可证、示例资源来源和文档入口。README 面向使用者，不混入验收日志。
- 若维护者另行要求发布示例二进制，再采用 GOUI 的三平台正式构建、macOS 签名
  公证、附件摘要、Draft 下载与公开下载核对流程；正式附件须来自同一冻结提交。

## 冻结与验证

1. 检查本地和远端已有标签、Release 及 Draft；续跑核对已有结果，不重复创建。
2. 审阅改动并按功能提交，推送 main，记录完整候选 SHA。保留用户未跟踪文件。
3. 从候选的 git archive 创建仓库外干净副本；测试和构建均使用 `GOWORK=off`，
   不把本地相邻 GOUI 或未提交文件带入候选。冻结后源码变化须重新提交、冻结并验证。
4. 使用实际 Go 工具链记录 `go mod verify`、`go mod tidy -diff`、
   `go test -json ./... -count=1`、`go vet ./...` 及适用的 race 结果。
   正式构建使用 `CGO_ENABLED=0`；race 使用宿主 C 工具链并单独开启 cgo。
5. 对 Linux amd64、Windows amd64、macOS arm64 执行 `go build ./...`，
   并按改动风险运行原生排版与窗口场景。桌面验证遵循 Testing，通过正常输入路径，
   不使用 DevServer 捷径；Linux 使用独立显示会话，远端使用专用临时目录。
6. 记录通过、失败、跳过与未执行。新增失败先修复；既有失败要复现、解释影响，
   未经维护者确认不能自行接受。交叉构建、原生测试和桌面操作分别记录。

全量命令和完整退出码保存在临时发布目录。日志、截图和测试程序不是默认发布附件；
验证环境、候选 SHA、GOUI 版本、平台/后端/缩放及限制归入 Plan 和 Release Notes。

## 标签与发布

- 全部门槛通过后，确认候选在 origin/main 历史中，新标签尚不存在。
  创建 annotated tag（消息 `Release vX.Y.Z.`），仅推送本次标签并核对远端
  peeled SHA。已推送标签不删除、不移动、不重写；标签推送即可能被 Go proxy 获取。
- 使用 `gh release create --draft --verify-tag` 和已审阅的中英文说明创建 Draft。
  核对 tag、正文、Draft/Prerelease 状态和附件范围；无二进制发布时附件为空。
  无手工附件并不影响 GitHub 自动提供源码归档。
- 用 `gh release edit --draft=false --prerelease=false --latest --verify-tag`
  正式发布，再核对 URL、公开状态和 Latest。网络中断后先检查实际状态，不重复创建。
- 在仓库外使用 `GOWORK=off GOPROXY=https://proxy.golang.org` 查询发布版本，
  核对来源 SHA；新建无 replace 的消费模块实际下载并构建公开包，验证发布依赖完整。
  代理传播延迟保留错误并重试，不为此更改标签。
- 公开验证后更新中英文 README 的版本安装指引，在 Plan 记录最终结果和缺口，
  按授权提交并推送收尾文档。收尾提交不改变发布标签。
- 清理本轮窗口、独立显示会话和临时传输服务，保留已有远端连接及用户工作区。
  发布后发现代码或附件错误，使用新的补丁版本处理，不暗中替换既有版本。
