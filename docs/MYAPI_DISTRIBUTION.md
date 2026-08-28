# MyAPI 自建发行版与 NPM 包

MyAPI 是这套自建实例与部署工具的发行名称，基于 New API
`v1.0.0-rc.25`（上游提交
`f116414284162ad15d8925f7bca494c109b83e93`）。本发行版继续保留 New API、
QuantumNous、AGPL、NOTICE、About 页面和可见原项目链接。

## 可见实例品牌

仓库新增 `web/public/myapi-logo-v1.png`，不会覆盖上游的 `logo.png` 或
`favicon.ico`。部署后在“系统设置 → 站点信息”填写：

```text
SystemName = MyAPI
Logo = https://你的域名/myapi-logo-v1.png
ServerAddress = https://你的域名
Footer = 留空
About = 留空
```

运行时设置会同步更新页面标题、Header、登录页、首页和 favicon。页脚与 About
页面仍保留 New API 原项目归属，这是许可证和仓库规则要求的一部分。

这些值保存在应用数据库的 Options 表中，CLI 的 `configure` 只生成部署环境文件，
不会替你修改数据库。应先部署包含新 Logo 的镜像，确认 Logo URL 返回
`image/png`，再在后台保存上述设置。若启用 Passkey，还需设置：

```text
passkey.rp_display_name = MyAPI
passkey.rp_id = 你的域名主机名（不含协议）
passkey.origins = https://你的域名
```

## NPM 发行包

独立发行包名称：

```text
@forcemind/myapi
```

它不会在 `npm install` 时自动运行 Docker，也不会读取或上传密钥。包内包括完整
源码、Dockerfile、部署模板、历史补丁、许可证和零运行时依赖 CLI。

### 初始化

```bash
npx @forcemind/myapi init ./myapi-source
npx @forcemind/myapi configure \
  --project-dir ./myapi-source \
  --public-url https://myapi.example.com
npx @forcemind/myapi doctor --project-dir ./myapi-source
npx @forcemind/myapi build --project-dir ./myapi-source
npx @forcemind/myapi up --project-dir ./myapi-source
```

`configure` 生成 32 字节随机 `SESSION_SECRET`，并将 `deploy/.env` 权限设为
`0600`。初始化后的 `package.json` 会被标记为 `private: true`，同时生成
`.gitignore` 与 `.npmignore`；`.dockerignore` 也会阻止密钥、数据库、日志和运行
目录进入 Docker 构建上下文。

`myapi up` 会先检查：公网地址必须是非占位的 HTTPS origin、会话密钥长度至少
48 字符、端口合法且 `docker compose config --quiet` 通过。检查失败时不会启动。

### 接管已有数据

不会复制、删除或迁移数据，只把明确路径写入 `.env`：

```bash
npx @forcemind/myapi adopt \
  --project-dir ./myapi-source \
  --data-dir /root/new-api/data \
  --logs-dir /root/new-api/logs
```

`down` 永远不会附带 `-v`，不会删除 Docker volume。

## 发布检查

```bash
npm test
npm run release:state
npm run source:manifest
npm run pack:check
npm pack --dry-run --json
npm publish --dry-run --access public --registry=https://registry.npmjs.org/
```

首版当前完整源码 tarball 约 4.25 MB（解包约 17 MB、约 2,000 个文件）；实际
数字以 `npm pack --dry-run --json` 为准。

正式发布前必须满足：

- Git 工作树干净，tag、`VERSION` 与 `package.json.version` 一致；
- Go、relaykit、前端、普通构建和精简构建全部通过；
- tarball 中不存在 `.env`、数据库、日志、OAuth JSON、token、缓存、`dist` 或
  `node_modules`；
- npm 账户拥有 `@forcemind` scope，首版完成 2FA 发布；
- 后续使用 `.github/workflows/npm-publish.yml` 的 Trusted Publishing 与
  provenance。

`patches/manifest.json` 中的六个补丁仅是早期定制阶段的历史记录；当前完整源码和
NPM 包才是后续部署的源事实。
