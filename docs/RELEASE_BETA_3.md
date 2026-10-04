# My API 0.2.0-beta.3 发布与部署记录

## 目标与授权

2026-10-04，负责人选择镜像部署，批准提交和推送本批修改，发布
`v0.2.0-beta.3` GitHub 预发布及 Full/LAN 的 Linux amd64/arm64 GHCR 镜像。
不合并分支，不提升 stable/latest，不发布 NPM，也不操作真实服务器、账号或数据库。

源码分支为 `codex/r1-usage-review-20261002`，基线 `68adefd349d30a7b6bb202f8f1b70828942da671`。
只提交当前任务文件；其他 worktree 保持原状。

## 当前进度

候选提交已同步：`2f2717bffe580baf23820480a7d40d54e676199e`，远端回读一致。
[本批 CI](https://github.com/ForceMind/MyAPI/actions/runs/37165610684)十项和
[Docker smoke](https://github.com/ForceMind/MyAPI/actions/runs/37165610681)三项全部成功。
PR 合并测试树 `3311a73425ac6b2e1292aee4a1968babd85a5cf8` 与候选源码树一致；
LAN 原安装脚本实际验证了密钥传入，Full fresh 与固定旧源码升级/备份恢复通过。
不可变 tag `v0.2.0-beta.3` 已创建并推送，剥离后的远端 SHA 与候选一致，
干净源码/tag/版本检查通过。

发布任务已接受：[GitHub 预发布](https://github.com/ForceMind/MyAPI/actions/runs/37166338851)、
[GHCR Full/LAN](https://github.com/ForceMind/MyAPI/actions/runs/37166352633)。
两者从准确 tag 运行，`headSha` 均为候选；均已成功，发布与回读完成。

GitHub 预发布任务已成功，Release 回读为 `isPrerelease=true`、`isDraft=false`，
`targetCommitish` 为候选 SHA。三个校验和文件及 Linux amd64/arm64、macOS、Windows
四个二进制共七项资产已上传，平台校验和与 GitHub 返回的四项二进制 SHA256 digest
逐项一致。未在目标机器运行这些二进制。GHCR 四项架构构建及两项多架构清单全部成功，
stable/latest 提升步骤跳过；匿名回读和独立 Sigstore 验证均通过。

1. 版本和默认镜像统一为 `0.2.0-beta.3`；部署 Compose 补齐
   `CHANNEL_QUOTA_IDENTITY_KEYS` 传入，示例及操作说明同步更新。
2. 已完成候选提交的部署合同、准确提交 CI 与 Docker smoke。
   LAN 原安装脚本使用隔离临时身份密钥，在不输出密钥的条件下核对容器接收值。
3. 门禁通过后已创建并推送不可变 tag，调用 `release.yml` 和
   `docker-build.yml`，参数 `tag=v0.2.0-beta.3`、`confirm=PUBLISH`。
4. Release 资产、tag SHA、两种镜像签名及多架构 digest 已回读并记录。

## 已有证据与支持边界

基线的 [CI 十项](https://github.com/ForceMind/MyAPI/actions/runs/37135514670)及
[Docker 三项](https://github.com/ForceMind/MyAPI/actions/runs/37135514678)成功。
本批没有业务逻辑或数据库变更，已取得自己的准确提交 CI、容器及发行结果。

本地发布准备验证已通过：CLI 81/81、发行与安装工作流测试 11/11、发行合同 32/32、
升级合同 19/19、Bash 语法、三份 YAML 解析、版本/默认镜像一致性及差异检查。
本机没有 Docker，实际容器密钥传入验证已由本批 Docker smoke 完成。
`SOURCE_MANIFEST.json` 属于 NPM 打包期产物，按既有 CI 在干净检出上重新生成并核验；
不把仓库中旧打包清单当作本次镜像或二进制来源证据，也不发布 NPM。

R1 六项限定范围的实现及自动化证据见 [当前交付表](NEXT_USABLE_VERSION.md#当前-r1-六项执行表2026-10-04-0001-北京时间)。
严格 Token/USD 预算限于已纳入的官方原生 Responses 纯文本和适用冻结报价；
Codex 百分比属于独立账户窗口安全阈值。真实 OAuth、窗口重置/429、账单及目标
Full HTTPS 验收此前暂缓，发布预发布制品不改变这些未验状态。

## 制品回读结果

最终回读记录时间：2026-10-04 09:12 北京时间。

| 镜像 | 不可变多架构 digest | 架构与版本 |
| --- | --- | --- |
| `ghcr.io/forcemind/myapi:v0.2.0-beta.3` | `sha256:cdcf26dbad757ff06af11ca2e5889e76931ac0a78d4bdd272e38b95e58d3ce41` | Linux amd64/arm64；`v0.2.0-beta.3` |
| `ghcr.io/forcemind/myapi-lan:v0.2.0-beta.3` | `sha256:d9a27a0f144cd8a5369051c2129708d62c549aa09a581811bb0b7758cc0c4609` | Linux amd64/arm64；`v0.2.0-beta.3` |

四个架构的 OCI revision 均为 `2f2717bffe580baf23820480a7d40d54e676199e`，
version 和 edition 分别与目标一致。使用独立下载、官方 SHA256 已核验的 Sigstore
cosign v3.1.3 对两个不可变多架构 digest 进行 keyless 验证，均退出 0；
证书 identity 精确限定为
`https://github.com/ForceMind/MyAPI/.github/workflows/docker-build.yml@refs/tags/v0.2.0-beta.3`，
issuer 精确限定为 `https://token.actions.githubusercontent.com`。签名声明中的 digest
与匿名注册表回读一致，验证使用空 Docker 配置，没有借用本机注册表凭据。

自动 tag 桌面构建的 macOS/Windows 两项也通过，仅留 Actions 制品，发布步骤跳过；
不宣称桌面实机验收或额外发布。分支仍保留草稿 PR #2，没有合并或部署真实服务。

## 镜像部署入口

从完整发行源码检出部署文件，避免默认分支旧配置：

```bash
git clone --branch v0.2.0-beta.3 --single-branch https://github.com/ForceMind/MyAPI.git my-api
cd my-api
umask 077
cp deploy/.env.example deploy/.env
```

新机器编辑 `deploy/.env`：

```dotenv
MYAPI_IMAGE=ghcr.io/forcemind/myapi:v0.2.0-beta.3
MYAPI_BUILD_LOCAL=false
MYAPI_EDITION=full
MYAPI_BIND_ADDRESS=127.0.0.1
MYAPI_ALLOW_LAN=false
MYAPI_SESSION_COOKIE_SECURE=true
MYAPI_PUBLIC_URL=https://你的实际域名
```

另设置至少 48 字符的随机 `SESSION_SECRET`。多账户额度采样需独立配置
`CHANNEL_QUOTA_IDENTITY_KEYS=active:v1:<base64url密钥>`；具体生成与保存方式见
[部署说明](../DEPLOYMENT_CUSTOM.md#当前-r1-源码部署)。已有实例保留原会话秘密、
身份密钥环、数据库位置和访问设置，不用示例文件覆盖原 `.env`。

配置完成执行 `bash deploy/install.sh`，HTTPS 反代另外指向
`http://127.0.0.1:3000`。LAN 使用 `ghcr.io/forcemind/myapi-lan:v0.2.0-beta.3`，
并按 [LAN 说明](LAN_LITE.md)设置私网访问和 Cookie 模式。

升级必须先备份，再用隔离副本演练；回滚使用升级前备份，不能让旧二进制直接打开
已迁移数据库。详见 [升级演练](UPGRADE_REHEARSAL.md)和
[真实验收记录](R1_INSTALLATION_CHECK.md#下一步真实验收记录)。

## 恢复与停止条件

恢复时先核对本文件、Goal、Git 工作树、远端 tag 和工作流终态。不移动或重用已发布
tag；工作流失败先读具体错误，修复只在已批准范围内进行。若错误需要源码变化且制品
已发布，不覆盖原版本，先说明新版本需求。未知发布结果必须回读，不能重复猜测发布。

完成条件为候选 CI/Docker 成功、GitHub 预发布与签名 Full/LAN 双架构镜像全部成功、
tag 和镜像结果回读一致，并向负责人提供可执行部署入口。真实部署不属于本任务。
