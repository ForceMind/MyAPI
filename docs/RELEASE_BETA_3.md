# My API 0.2.0-beta.3 发布与部署记录

## 目标与授权

2026-10-04，负责人选择镜像部署，批准提交和推送本批修改，发布
`v0.2.0-beta.3` GitHub 预发布及 Full/LAN 的 Linux amd64/arm64 GHCR 镜像。
不合并分支，不提升 stable/latest，不发布 NPM，也不操作真实服务器、账号或数据库。

源码分支为 `codex/r1-usage-review-20261002`，基线 `68adefd349d30a7b6bb202f8f1b70828942da671`。
只提交当前任务文件；其他 worktree 保持原状。

## 当前进度

1. 版本和默认镜像统一为 `0.2.0-beta.3`；部署 Compose 补齐
   `CHANNEL_QUOTA_IDENTITY_KEYS` 传入，示例及操作说明同步更新。
2. 待完成候选提交的部署合同、准确提交 CI 与 Docker smoke。
   LAN 原安装脚本试验将生成隔离临时身份密钥，并在不输出密钥的条件下核对容器接收值。
3. 门禁通过后创建并推送不可变 tag，再依次调用 `release.yml` 和
   `docker-build.yml`，参数 `tag=v0.2.0-beta.3`、`confirm=PUBLISH`。
4. 回读 Release 资产、tag SHA、两种镜像的签名及多架构 digest，记录结果。

## 已有证据与支持边界

基线的 [CI 十项](https://github.com/ForceMind/MyAPI/actions/runs/37135514670)及
[Docker 三项](https://github.com/ForceMind/MyAPI/actions/runs/37135514678)成功。
本批源码没有业务逻辑或数据库变更；新版本仍须取得自己的准确提交结果。

本地发布准备验证已通过：CLI 81/81、发行与安装工作流测试 11/11、发行合同 32/32、
升级合同 19/19、Bash 语法、三份 YAML 解析、版本/默认镜像一致性及差异检查。
本机没有 Docker，实际容器密钥传入验证交给本批 Docker smoke。
`SOURCE_MANIFEST.json` 属于 NPM 打包期产物，按既有 CI 在干净检出上重新生成并核验；
不把仓库中旧打包清单当作本次镜像或二进制来源证据，也不发布 NPM。

R1 六项限定范围的实现及自动化证据见 [当前交付表](NEXT_USABLE_VERSION.md#当前-r1-六项执行表2026-10-04-0001-北京时间)。
严格 Token/USD 预算限于已纳入的官方原生 Responses 纯文本和适用冻结报价；
Codex 百分比属于独立账户窗口安全阈值。真实 OAuth、窗口重置/429、账单及目标
Full HTTPS 验收此前暂缓，发布预发布制品不改变这些未验状态。

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
