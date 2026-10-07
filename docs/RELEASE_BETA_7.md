# MyAPI 0.2.0-beta.7 预发布记录

> 状态：发布准备。此文件随候选源码进入合并流程；在 `v0.2.0-beta.7` 标签、GitHub prerelease 和对应 GHCR 工作流完成前，不得将本记录解释为已经发布或生产验收完成。

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

当前阶段：发布门禁与文档同步。发布、部署和最终验收结果完成后回填本节。

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

## 发布后必须回填的真实证据

只能在发布完成后从 GitHub/GHCR 回读：

- 最终 `main` SHA
- `v0.2.0-beta.7` 标签实际指向 SHA
- GitHub Release URL 与 prerelease 状态
- Linux/macOS/Windows Release assets 与 checksum
- Full/LAN 多架构镜像 digest
- 发布工作流 run URL、结论和来源提交
- 安装入口最终可用性
- 仍未验证的真实账号、账单、目标生产环境升级/回退边界

预发布成功不等于生产环境验收完成。
