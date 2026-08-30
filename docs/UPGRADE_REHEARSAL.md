# MyAPI 升级与恢复演练

`myapi upgrade` 是显式操作：它会校验版本、备份 `deploy/.env`、拉取固定 GHCR 镜像、等待健康检查，并在失败时恢复环境文件和旧镜像。它不会替管理员猜测数据库类型，也不会自动复制生产数据库。

## 在副本上演练

1. 使用与生产相同版本的副本目录和脱敏数据，不挂载生产 `data`、`logs` 或数据库卷。
2. 固定 `MYAPI_IMAGE=ghcr.io/forcemind/myapi:<version>`，确认 `MYAPI_EDITION` 与目标发行版一致。
3. 对 SQLite 副本保存数据库文件的校验和；对 PostgreSQL 使用管理员批准的逻辑备份工具，并把备份写入部署目录之外的受控位置。
4. 先运行只读预检，不会写入环境文件、创建备份、调用 Docker 或访问 GHCR：

   ```bash
   npx @forcemind/myapi upgrade \
     --project-dir <副本目录> --version <新版本> --dry-run --json
   ```

   预检会检查发行版本、Full/LAN 镜像映射、环境文件、会话密钥、URL、端口和资源限制，输出
   目标镜像但不会输出任何密钥或完整环境变量。签名参数在预检中只记录为待执行，不会调用
   `cosign`；正式升级时才会执行签名验证。
5. 预检通过后，才在副本中执行 `myapi upgrade --project-dir <副本目录> --version <新版本>`，确认容器健康、登录、API 请求、日志查询和额度面板均可用。
6. 在副本中停止服务并从备份恢复，再验证用户、Key、渠道配置、日志索引和快照趋势；记录恢复耗时和缺失项。

`--dry-run` 是升级前的安全门，不等同于数据库恢复测试。恢复测试必须使用复制出的 SQLite 文件或经批准的
PostgreSQL 逻辑备份；原始生产路径和卷永远不能作为 CLI 自动操作目标。

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
