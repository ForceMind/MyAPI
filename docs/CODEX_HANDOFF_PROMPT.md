# 新设备 Codex 启动提示词

将下面整段复制到新 Mac 的 Codex 新对话中。它只描述工作范围和仓库事实，不包含任何
密钥、服务器 `.env`、数据库或日志内容。

```text
你现在接手 MyAPI 项目在 macOS 上的开发工作。

目标：继续完成 MyAPI 的开发计划，但不从头重做已经完成的功能，不擅自发布、推送镜像、
重启服务器或修改生产数据。当前开发主机是 Mac；Linux 服务器只作为远程仓库、CI/GHCR
或经明确批准的部署环境。

仓库：
- GitHub：私有仓库 https://github.com/ForceMind/MyAPI
- 本机目录建议：~/src/MyAPI（不要放在 iCloud Drive、Dropbox、OneDrive 或网络盘）
- 首先执行：git status --short --branch、git remote -v、git log --oneline --decorate -5
- 然后读取仓库根目录 AGENTS.md、docs/MYAPI_MASTER_PLAN.md、docs/COMPLETION_AUDIT.md、
  docs/DEVELOPMENT_ON_MACOS.md；如果当前环境有 `~/.codex/AGENTS.md`，也读取它（当前 Linux
  机器上的对应路径是 `/root/.codex/AGENTS.md`）。

开发环境：
- macOS 原生开发；安装 Xcode Command Line Tools、Homebrew、Docker Desktop、Go、Bun、Node。
- Go 版本以 go.mod 为准（当前 1.25.1）；Bun 以 lockfile/CI 为准（当前 1.3.14）；Node 22.x。
- Docker Desktop 使用 Compose v2.17+，构建使用低并行度；不要长时间占满 CPU/内存。
- Electron macOS 可以在本机打包；Windows 安装包交给 Windows runner/设备，不能由 Mac 验收。

已完成并必须保留：
- Codex/Chat Completions/Responses、SSE、附件转换和参数兼容；
- 完整请求/响应/分片日志、脱敏、查询和移动端日志展示；
- 普通渠道与 Codex OAuth 额度快照、每分钟/小时/天/周趋势、时区、失败状态、可选采样
  和只读告警状态；概览和管理员渠道页面均有额度变化界面；
- Account Tier 与 Key Access Profile 兼容领域模型、注册表和清晰 Key 表单；
- MyAPI 独立品牌、UI、静态官网、LAN Lite 和 Electron macOS/Windows 合同；
- GHCR tag 自动构建 Full/LAN 镜像、不可变 digest、CLI 健康检查/备份/回滚；
- Claude Messages/Responses 兼容和官方边界；Google Antigravity transport 的官方边界；
- SQLite 迁移修复、脱敏副本升级/回滚、运行时认证探针和现有文档。

当前事实：
- main 与 origin/main 必须先重新核对；不要假定旧聊天记录中的提交仍是 HEAD；
- v0.1.0/v0.1.1 是受保护历史 tag，不得移动或覆盖；正式发布必须先确认新版本号；
- NPM、正式 GHCR 发布、生产升级、法律/NOTICE 审查仍需负责人确认；
- 真实手机、macOS/Windows 安装和 PostgreSQL 恢复是外部验收，不得用静态测试冒充；
- GitHub Actions 如果 job 在 steps: []、runner 启动前失败，应记录为 Billing/runner 外部阻塞，
  不要为了“修绿”修改无关源码；
- LAN Lite 不读取本机 Codex/Claude/Antigravity/Keychain 凭据文件，不实现凭据文件导入；
  上游凭据只在 MyAPI 管理界面显式配置。

工作规则：
1. 修改前先检查工作树和相关文件，说明本轮目标、范围、验证和风险；保护其他改动。
2. 使用 apply_patch 编辑文件；不要把 .env、数据库、日志、node_modules、dist 或密钥加入 Git。
3. 遇到核心架构问题先提出方案，不整体重写稳定模块；数据库改动兼容 SQLite/MySQL/PostgreSQL。
4. JSON 业务序列化使用 common/json.go wrapper；relaykit 必须独立构建。
5. 完成修改后运行受影响测试、相关合同检查和必要构建；记录实际命令、结果和未验证范围。
6. 不要执行 npm publish、创建/移动 tag、GHCR publish、生产重启或服务器数据操作，除非我在
   当前对话中明确授权。
7. 每轮结束报告：当前完成、验证结果、审查发现、已修复问题、剩余任务、下一步。

开始动作：
- 先只读检查仓库状态和上面列出的文档；
- 建立动态计划，标记已完成、进行中、待验证和外部阻塞；
- 优先处理真实代码缺陷或 macOS/Docker Desktop 开发迁移问题，不重复实现已验证功能；
- 没有真实设备或外部权限时，明确记录阻塞，不伪造验收。
```
