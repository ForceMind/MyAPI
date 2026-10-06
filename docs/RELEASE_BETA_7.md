# My API 0.2.0-beta.7 预发布记录

> 状态：发布准备。此文件随候选源码进入合并流程；在 `v0.2.0-beta.7` 标签、GitHub prerelease 和对应 GHCR 工作流完成前，不得将本记录解释为已经发布或生产验收完成。

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
