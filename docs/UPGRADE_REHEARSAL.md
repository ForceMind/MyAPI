# MyAPI 升级与恢复演练

`myapi upgrade` 是显式操作：它会校验版本、备份 `deploy/.env`、拉取固定 GHCR 镜像、等待健康检查，并在失败时恢复环境文件和旧镜像。它不会替管理员猜测数据库类型，也不会自动复制生产数据库。

> 当前 `@forcemind/myapi` 尚未正式发布到 NPM。以下命令中的 `npx
> @forcemind/myapi` 在正式发行前请替换为源码检出的
> `node cli/myapi.mjs`，或使用已经由维护者审核的本地 tarball；不要让
> `npx` 从未知的公共包解析同名命令。

源码发行合同可在没有 Docker、GHCR 或部署凭据的环境中先做只读检查：

```bash
npm run upgrade:check -- --json
```

该检查只确认 CLI 的 dry-run、资源预检、签名闸门、备份权限、健康等待和回滚路径仍与本页及回归测试一致；它不会创建环境文件、读取 `deploy/.env`、访问网络或调用 Docker。它不能替代下面的副本升级和数据库恢复演练。

## 在副本上演练

1. 使用与生产相同版本的副本目录和脱敏数据，不挂载生产 `data`、`logs` 或数据库卷。
2. 固定 `MYAPI_IMAGE=ghcr.io/forcemind/myapi:<version>`，确认 `MYAPI_EDITION` 与目标发行版一致。
3. 对 SQLite 副本保存数据库文件的校验和；对 PostgreSQL 使用管理员批准的逻辑备份工具，并把备份写入部署目录之外的受控位置。
4. 先运行只读预检，不会写入环境文件、创建备份、调用 Docker 或访问 GHCR：

   ```bash
   node cli/myapi.mjs upgrade \
     --project-dir <副本目录> --version <新版本> --dry-run --json
   ```

   预检会检查发行版本、Full/LAN 镜像映射、环境文件、会话密钥、URL、端口和资源限制，输出
   目标镜像但不会输出任何密钥或完整环境变量。签名参数在预检中只记录为待执行，不会调用
   `cosign`；正式升级时才会执行签名验证。
5. 预检通过后，才在副本中执行 `myapi upgrade --project-dir <副本目录> --version <新版本>`，确认容器健康、登录、API 请求、日志查询和额度面板均可用。
6. 在副本中停止服务并从备份恢复，再验证用户、Key、渠道配置、日志索引和快照趋势；记录恢复耗时和缺失项。

如需避免版本 tag 在拉取后被重新指向，可在副本升级时增加 `--pin-digest`，或在
`deploy/.env` 设置 `MYAPI_PIN_IMAGE_DIGEST=true`。CLI 会先拉取版本 tag，再读取本机
`RepoDigests`，严格校验 `repo@sha256:<64 hex>` 后把该 digest 写回环境文件；若同时
使用 `--verify-signature`，cosign 会验证最终 digest。解析失败会触发原环境回滚。
该选项不是 dry-run 的一部分，dry-run 不访问 Docker 或 GHCR。

`--dry-run` 是升级前的安全门，不等同于数据库恢复测试。恢复测试必须使用复制出的 SQLite 文件或经批准的
PostgreSQL 逻辑备份；原始生产路径和卷永远不能作为 CLI 自动操作目标。

## 本地镜像构建资源

当需要在工作站验证本地 Docker 镜像时，`deploy/install.sh` 会把
`MYAPI_CPU_LIMIT`、`MYAPI_MEMORY_LIMIT` 同时应用到 `docker build` 的构建容器，
并通过 `MYAPI_BUILD_PARALLELISM`/`GOMAXPROCS` 限制 Go 编译并发。资源参数应保持在
工作站可承受范围内；验证镜像必须使用临时数据目录和非生产端口。若 Docker 仅提供已弃用的
legacy builder，可额外指定 `--cpu-period`、`--cpu-quota`、`--memory` 和
`--memory-swap`；BuildKit/buildx 缺失属于主机工具链问题，不应通过重启生产服务绕过。

## 本机失败回滚测试

CLI 的环境文件和旧部署回滚路径可以在没有 Docker daemon、GHCR 或生产数据的本机上验证。测试会在临时目录
创建一个一次性失败的 `docker` 替身：第一次 `compose pull my-api` 返回失败，随后 CLI 恢复原来的
`deploy/.env` 并重新执行旧部署的 `compose up`。它不会读取当前项目的 `.env`、Docker volume 或凭据。

在源码仓库执行：

```bash
node --test --test-name-pattern="upgrade restores the environment" cli/test/myapi.test.mjs
```

这个测试只证明 CLI 的失败处理和文件权限（环境备份为 `0600`）；它不能替代副本上的真实镜像健康检查或数据库
恢复演练。真实副本升级仍应先执行上面的 `--dry-run`，再按组织批准的备份流程验证数据恢复。

## 生产前检查

- 备份文件不在 Git 工作树、Docker 镜像层或公开制品中，并设置最小权限。
- 备份与目标版本、数据库类型、时区和迁移状态一一对应。
- 新镜像健康检查通过前保留旧镜像和旧数据卷；不要强制移动现有 tag。
- 升级失败时保留 CLI 输出、容器状态和备份校验和，再人工决定是否回滚。

当前 CLI 的自动回滚范围是环境文件和镜像部署状态；数据库备份/恢复仍需按组织的 PostgreSQL/SQLite 运维流程执行。本页是演练清单，不会触发任何生产操作。
