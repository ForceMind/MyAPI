# MyAPI 0.2.0-beta.7 预发布记录

> 状态：GitHub 预发布与 Full/LAN 镜像已发布并核验，现有 Full HTTPS 实例已从 beta.3 升级至 beta.7 并通过本批限定验收。发布源码固定为 `1bd48522b85e91929b6a7de9a78f142a957c6147`；后续交付文档不移动发行标签。

## 本次发布与部署执行（2026-10-07）

负责人已授权更新仓库和文档、发布并部署。最新合并基线为
`2c32384bb2d21b06c5aa6bbc7f95b5e6be9abf3d`，现有正式实例为
`0.2.0-beta.3`。本批仅更新操作文档，不改变已审业务代码。

执行顺序：同步文档到现有仓库 `main`，取得最终提交的 CI、Docker 和网站检查；
创建准确提交的不可变 `v0.2.0-beta.7` 标签；通过已有受保护 workflow 发布
GitHub prerelease 和 Full/LAN 双架构签名镜像；回读资产、digest 和来源；
在断网、资源受限的数据库副本上验证升级与升级前备份恢复；保存正式实例停机后的
一致性备份，再切换固定 digest 的 Full 镜像，核对 HTTPS 版本、健康和数据。

保留原 `SESSION_SECRET`、完整 `CHANNEL_QUOTA_IDENTITY_KEYS`、数据库路径、域名、
回环监听和资源限制。beta.7 的新访问约束不自动分配给原 Key；不切换账务模式、
不变更访问策略，不提升稳定 latest，不发布 NPM，不进行真实 OAuth 或付费请求。
副本失败时保持旧服务；目标已尝试启动后，不能直接让旧代码打开已迁移数据库，
恢复必须使用升级前数据库和配置。回旧版本也不等于撤销 beta.7 的权限策略。

执行结果：镜像发布、签名核验、隔离升级/原快照恢复及生产切换验收均已通过。
首次演练捕获后台恢复对 50 条既有 retryable 结算事实的重试运维字段更新；
经济字段、身份和状态未变，失败副本和日志保留；首轮未执行生产切换。
复核确认该后台代码与 beta.3 一致；首次副本漏带生产 Compose 的
`BATCH_UPDATE_ENABLED=true`，使失败类别从缓存不可用变为 int32 边界拒绝。
修正副本运行参数后重新演练通过；只允许既有 retryable 记录符合领取/失败收尾契约
的五个运维字段变化，所有经济、身份、状态与应用标记逐行保持。

## 生产部署限定验收（2026-10-07）

- 实际部署为下方发布证据中的 Full 固定 digest；运行版本 `0.2.0-beta.7`，源码标签为发行提交。
- 升级前原数据库快照在断网、0.5 CPU/768 MiB 的 master 副本执行真实迁移；
  beta.7 启动、SQLite 完整性、25 张保护表及新约束未自动分配检查通过。
- 另一个原快照副本启动 beta.3，核对原数据与健康通过；这是原快照恢复演练，
  没有在生产执行回滚，也没有让旧二进制打开已迁移数据库。
- 生产停止写入后保存完整数据目录归档（含 SQLite/WAL）、独立数据库基线、
  原 Compose 和环境文件；归档和文件 SHA256、私有权限均核验。
- 生产只更换 Compose 镜像引用；环境文件字节、会话秘密、完整身份密钥环、
  数据与日志挂载、回环端口、CPU/内存/PID 限制保持。
- 本地和真实 HTTPS 的 `/api/status`、首页均返回 HTTP 200，版本一致，容器健康。
  SQLite 完整性通过；历史用户、Key、渠道、日志身份及持久账务/预算/未知状态核对通过。
- 副本升级、原快照恢复和生产均观察到既有 retryable 失败重试：仅五个运维字段按
  精确契约变化，错误类别仍为缓存不可用；经济字段、身份、状态与应用标记未变。
  本批没有修复既有失败重试记录，也没有自动结算、退款或授权策略修改。
- 原用户/Key 没有自动分配新访问约束或预算；启动日志没有 fatal/panic。

私有备份、失败副本和详细验证报告保留在部署主机，不提交数据库、域名、密钥或原始日志。
真实 OAuth 登录、真实上游付费请求、供应商账单与所有业务协议没有在本批验证；
LAN 镜像已发布及自动化验收，本批生产切换仅为现有 Full 实例。

## 目标

本次 `v0.2.0-beta.7` 是包含 beta.4–7 已完成工作的累计预发布，不补造 beta.4、beta.5、beta.6 的历史 Release。

- GitHub tag：`v0.2.0-beta.7`
- GitHub Release：prerelease，必须保持 `make_latest=false`
- Full 镜像：`ghcr.io/forcemind/myapi:v0.2.0-beta.7`
- LAN 镜像：`ghcr.io/forcemind/myapi-lan:v0.2.0-beta.7`

## 已限定源码与验证基线

- 最终纯源码提交：`5dcbd35a5ecc40461e2b8e344f2bd92e9c2332bb`
- 最终文档/交接基线：`386bbae04b6adcac34448c74d9a72564f253cd53`
- CI：<https://github.com/ForceMind/MyAPI/actions/runs/37482065167>
- Docker 检查：<https://github.com/ForceMind/MyAPI/actions/runs/37482065051>
- 网站检查：<https://github.com/ForceMind/MyAPI/actions/runs/37482065028>

这些结果证明上述历史准确提交通过既有验证，但不能替代版本准备提交、最终 PR HEAD、合并后准确 `main` 提交以及最终发布标签自己的验证。

## 累计交付范围

- beta.4：模型发现与基础路由
- beta.5：多账户与有界切换
- beta.6：原生 Chat 严格预算与价格来源
- beta.7：用户/API Key 分配访问约束、请求准入与最终派送复查、账户额度窗口事件、未知用量保护、七语言界面及相关失败修正
- PostgreSQL 重复迁移放大问题已通过受控依赖修正，并保留默认值/重复迁移回归
- Key 菜单重挂载、重复错误提示和原始 Axios 错误日志问题已修正

预算、计费、权限、OAuth、路由、恢复与商业模块关闭边界保持，不因预发布而放宽。

## 发布准备要求

- `VERSION=0.2.0-beta.7`
- 根 `package.json` 版本为 `0.2.0-beta.7`
- 默认 Compose 镜像使用 `ghcr.io/forcemind/myapi:v0.2.0-beta.7`
- 安装与升级示例指向 `v0.2.0-beta.7`
- `release.yml` 仍要求已有不可变标签、版本与标签一致，并由 `PUBLISH` 和仓库变量门控
- prerelease 不移动 GitHub latest，也不移动 GHCR stable/latest

## 已回读发布证据（2026-10-07）

- 标签和镜像源码 SHA：`1bd48522b85e91929b6a7de9a78f142a957c6147`
- [GitHub prerelease](https://github.com/ForceMind/MyAPI/releases/tag/v0.2.0-beta.7)：非草稿，prerelease=true，latest=false
- [准确源码 CI](https://github.com/ForceMind/MyAPI/actions/runs/37579634699)：十项通过，包含独立 relaykit、三种数据库及前后端验证
- [Docker 验证](https://github.com/ForceMind/MyAPI/actions/runs/37579640735)：四项通过，包含 LAN 安装、Full/LAN fresh 与 Full handoff/恢复
- [网站检查](https://github.com/ForceMind/MyAPI/actions/runs/37579715495)：通过
- [发行二进制工作流](https://github.com/ForceMind/MyAPI/actions/runs/37581117703)：通过；四份二进制及三份 checksum 文件均核验 SHA256，二进制同时匹配随附校验清单
- [GHCR 工作流](https://github.com/ForceMind/MyAPI/actions/runs/37581984565)：四架构构建及两份多架构清单通过，stable latest 提升跳过
- Full digest：`sha256:1baca952e9d3d4061de67b1086f184eb24559810926e0c85c53279a9b77d9716`
- LAN digest：`sha256:19988e46dfe26d33e04849b9fe2498527004c9d50cd5da7f90fdf2cb28d59ddc`

Full/LAN 均包含 Linux amd64 和 arm64；四个架构镜像的版本、edition 和源码标签一致。
两份多架构清单和四个架构发行根的 cosign 签名均通过证书、声明和透明日志验证，
证书主体为 `https://github.com/ForceMind/MyAPI/.github/workflows/docker-build.yml@refs/tags/v0.2.0-beta.7`，
OIDC issuer 为 `https://token.actions.githubusercontent.com`。

## 原发布准备证据清单（保留历史）

下列为发布前的检查清单；当前实际结果以上面的回读证据和限定验收为准：

- 最终 `main` SHA
- `v0.2.0-beta.7` 标签实际指向 SHA
- GitHub Release URL 与 prerelease 状态
- Linux/macOS/Windows Release assets 与 checksum
- Full/LAN 多架构镜像 digest
- 发布工作流 run URL、结论和来源提交
- 安装入口最终可用性
- 限定验收之外仍未验证的真实账号、账单与生产回滚边界

预发布成功不等于生产环境验收完成。
