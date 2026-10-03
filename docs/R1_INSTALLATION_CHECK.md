# R1 安装入口与验收边界

更新：2026-10-03 15:12 北京时间。R1 尚未封版、合并或发布；本文不是生产部署批准。

## 先分清会装到什么

- R1 源码在 `codex/r1-usage-review-20261002`，草稿 [PR #2](https://github.com/ForceMind/MyAPI/pull/2)。使用 PR 对应、已验证的准确提交，不能把 `main` 或旧 Release 当作本轮 R1。
- 当前 `VERSION`、包版本及 `deploy/.env.example` 默认镜像仍是 `0.2.0-beta.1`。`deploy/install.sh` 默认拉取该版本镜像；克隆 R1 分支后直接使用默认镜像也不会自动变成 R1。
- GitHub 已有的 `v0.2.0-beta.1` / `v0.2.0-beta.2` 是较早预发布，不是本轮 R1。没有发布新的 npm 包或容器镜像。
- `deploy/install.sh` 需要完整源码目录、`VERSION`、Compose 文件和配置，不是可单独下载后 `curl | bash` 的系统安装器。它不负责安装 Docker、开防火墙端口、申请证书或配置公网反代。
- 第一次缺少 `deploy/.env` 会创建示例并退出，要求操作者配置会话密钥和访问地址。不要把此预期退出当作安装成功。

## 当前有界试验

只验证已有安装脚本的 **Linux amd64、LAN、全新 SQLite、本地源码构建** 路径，不新增安装框架。

1. GitHub 临时 runner 检出当前 PR 测试树，拒绝覆盖已有 `deploy/.env` 或同名 `my-api` 容器。
2. 生成仅本次试验的私有配置；选择 `MYAPI_BUILD_LOCAL=true` 和按当前 SHA 命名的 `local/myapi` 镜像，避免拉旧发行镜像。随机会话秘密不输出，`.env` 被 Docker 构建上下文排除。
3. 真正执行仓库中的 `bash deploy/install.sh`，由脚本完成 Compose 校验、源码构建、启动及有界健康等待。
4. 核验容器确实使用刚构建镜像、准确 Compose 项目、健康、只监听 `127.0.0.1:18082`，运行限制为 2 CPU / 2 GiB。
5. 复用既有隔离探针完成全新初始化、匿名拒绝、LAN 自用模式、登录、管理员日志和额度接口读取；仅使用临时合成账号，不调用真实模型。
6. 清理只针对准确 Compose 项目；不上传配置、数据库或日志，不登录镜像仓库、不发布镜像。

当前修补：df1ddbe 的真实安装 job111151865218 已构建镜像并启动容器，但健康失败；原 Full/SQLite 恢复 job111151865317 通过。Compose 无条件将 HTTP FRONTEND 来源同时作为 Secure Cookie 信任来源，而应用明确拒绝 false+非空可信来源。安装器/CLI 现在从最终 Cookie 模式派生独立可信来源：LAN HTTP 明确为空，HTTPS Secure 保留原准确来源，Full 显式拒绝关闭 Secure；不放宽服务端检查。Compose 使用单横线默认表达式保留显式空值，生成 LAN 配置也写入空值以支持直接 Compose。四个 CLI/脚本合同先红后绿，CLI81、运行27、LAN、升级19和发行32合同通过，新SHA真实安装待验。

前批提交前本地证据：Bash 语法、LAN 合同检查、CLI 77 测试、运行探针 27 测试、发行 workflow 10 测试通过；YAML 两 job 解析通过。当前工作机没有 Docker/Podman，不能声称本地已经完成容器安装。真实安装结果必须以本批 GitHub Docker smoke 的 `LAN source installer and fresh SQLite` job 为准，提交后的终态记录在 PR 中。

前置 `f6ae1d1` 的 [CI37099811366](https://github.com/ForceMind/MyAPI/actions/runs/37099811366) 十项和 [Docker37099811371](https://github.com/ForceMind/MyAPI/actions/runs/37099811371) Full/SQLite-WAL 同镜像恢复已通过；此前容器通过是直接启动镜像，不替代本批安装脚本证据。

## 整版仍未完成的验收

- 新结算、待核对、Root 幂等恢复、价格发布和受限预算已接通，但严格费用/Token 当前仅限定合格官方原生 Responses 纯文本；不能宣称所有供应商、协议、工具或多模态都已具备严格预算保证。
- 百分比是 Codex 账户/窗口剩余安全阈值，不是每 Key 的共享账户消费分摊；当前资格集合不同，不能与官方 API 严格费用/Token 预算在同一 Key 混开。
- 真实容器 OAuth 授权/刷新、账户采样/重置/429避让、实际 usage 与账单对照尚未完成。
- 当前数据库证据包括隔离三库合同和 SQLite 同镜像副本恢复；指定旧版本到 R1 的升级、数据兼容、失败恢复和生产回滚未验收。不得在旧生产卷上试装或直接降级。
- Full HTTPS、目标机器资源/架构、反代及实际浏览器仍需目标环境验收；LAN loopback 成功不等于公网部署成功。
- 原独立批迁移 race 超时记录保留，不把 CI 已选择 race 通过说成全量 race 通过。

先完成本批真实安装链，失败仅修对应安装问题；不以继续堆叠支付、价格辅助或测试框架替代上述验收。合并、发布、真实部署、账户授权和密钥配置仍单独确认。
