# beta.20 制品与隔离运行证据

准确候选 `cdeef8b52792a09b341d074521d59cb5cc7f62fb`，tree `1fdefa9eb6edf291dee57e0e92cb8a9bd49f61fa`。CI checkout `37a138b65c038302eda6eaf447e329ec529722b9` 同树；归档后继提交仅文档，不能冒充被构建源码。

[Docker39203596121](https://github.com/ForceMind/MyAPI/actions/runs/39203596121) 最终6/6作业 success；[CI39203586608](https://github.com/ForceMind/MyAPI/actions/runs/39203586608) 最终11/11作业 success。所有数据均为新建合成夹具，未读取真实凭据、生产数据库/Redis/备份或历史账务。

## 制品与来源

测试平台仅 `linux/amd64`，Full/LAN 各一份不可变 bundle，均含 app 及固定 digest 的 Bun/Redis 夹具镜像。生产者没有运行应用或携带数据库/配置。消费者以外部 manifest 摘要验证源 SHA、tree、run/producer attempt、版型、tar 摘要及镜像 ID；不向 registry 回退、不发布。

| 版型 | Manifest SHA256 | 完整 images.tar SHA256 | Tar bytes |
| --- | --- | --- | --- |
| Full | `495d88ded4be4c57c2b0830a88f5388c2551e61ad1855cc3ae533e4d2e3dd40d` | `b5ea25847104e11961633b129874f440323395aa9944168b14f9df603157862b` | 353560064 |
| LAN | `0286e589ac4be406404108bd42ba6b38b0e2125c64b9ed5c6c8d93e3f6e774df` | `f27eb627f17f6ab0706c853fb76a9730696ee330f412a315cd8527198dfb6ef9` | 353166848 |

Full app ID `sha256:0bef1063e0d4ca43d72131f88fa6bf6e16c766a66d99e021e6ab0f9c04a020d3`；LAN app ID `sha256:bebd9a53d4d090da5d648e1db3b3df96584e9b86274c43a4be3c40aa2dfc344c`。独立源码安装镜像另为 `sha256:ea266cc101fe858cb782a98611c81b6116c077d7e50f78541ff8e945483687ff`，不冒充同一镜像。

`bundles/` 保留两份原始 manifest；`docker-reports/` 保存消费作业原始 `manifest.json`、外部 `manifest.sha256`、`source.json` 和 `qualification.json`。完整 tar 已下载到当前环境 `/tmp/myapi-beta20-stage1-full-bundle/` 与 `/tmp/myapi-beta20-stage1-lan-bundle/` 并重算大小/摘要，与分别下载的消费锚一致；没有本地加载或部署。

Actions 测试 bundle 工件名称如下，保留至2026-11-18（Full 05:18:23 UTC、LAN 05:20:22 UTC）。这是过期机制，不是长期发行存储；当前环境的 `/tmp` 同样没有长期保存承诺。

```text
myapi-smoke-images-39203596121-1-cdeef8b52792a09b341d074521d59cb5cc7f62fb-full
myapi-smoke-images-39203596121-1-cdeef8b52792a09b341d074521d59cb5cc7f62fb-lan
```

只下载/核对时，可对上述准确名称使用 `gh run download 39203596121 --repo ForceMind/MyAPI --name <准确名称> --dir <新的隔离目录>`；下载不会部署。不能把 smoke tar、临时 tag 或本页 digest 当作已签名正式发行版，不能据此运行生产 `upgrade`。

## 运行报告与原图

- `installer/qualification.json`：LAN 源码安装及 fresh SQLite、登录、匿名拒绝、状态与读取；资源/端口/镜像及 keyring 转发另由同作业强制断言。
- `docker-reports/full-fresh-{legacy,authoritative}/`：各26项总合同、真实安装向导/表单、对应 writer、有限 Key、零余额自用、流式请求/日志、权限拒绝、严格预算拒绝、模型范围收窄/恢复、刷新无重放。两个全新实例独立验收，不构成已有实例切 writer 或轮换许可。
- `docker-personal/{legacy,authoritative}/`：每组15张原始 PNG、6项个人旅程原 JSON。英文1280、中文320；原 M/软蓝、个人页面和实际后端流程。截图中的洋红块是 Playwright 捕获时的输入/秘密遮罩，`sk-••••` 是产品掩码；没有后期编辑图片。
- `docker-reports/lan-fresh-legacy/`：19项合同，含新补齐的恢复身份/状态/有限配置/模型/到期/结算凭证检查、SQLite+WAL 原始备份工作副本、同镜像恢复及真实登录表单/版本。
- `docker-reports/full-handoff-legacy/`：固定旧源码 `71277bf6055ce68b8cd1d11f1d10f1907d7696fd` 创建合成数据，8项合同覆盖无结算历史迁移、含结算历史拒绝且数据库/WAL字节不变、原备份独立工作副本恢复旧镜像且无上游重放。此报告的 `source_sha` 指旧源码、`target_sha` 指当前候选；相邻 `source.json.source_sha` 指当前作业候选，两者用途不同。
- `ci-independent-runtime/`：同准确源码的4份 Go/Chromium 迁移/轮换原报告及14张原始PNG，英文1440/中文390。仅这些维护/验证/回执/鉴权 API 为实际 Go，其他控制台读取仍有明确 fixture；签名合成会话/2FA及临时SQLite，不混称 Docker 镜像轮换或真实凭据验证。

静态截图不单独证明操作完成、恢复或无重放，以同次报告/断言为准。`existing-policy-after` 仍有前一条拒绝通知，政策结果由API/revision及后续成功请求证明。`model-scope` 是可滚动 dialog 的原始 locator 捕获，只证明可见范围；范围恢复和无额外上游发送由同次运行合同证明，不能把此图称为全表单/恢复证明。

## 测试、索引与边界

`original-artifact-manifest.json` 是生成的索引，列出76个原文件的逐字 SHA256、大小、来源和 run；含44原PNG（Docker30、独立Go/Chromium14）。`ci-tests/` 保存准确 CI 下载的单元/typecheck原日志及完整发行合同作业原日志；`local-tests/` 为本机合成 CLI/安装/只读结构检查的原输出，不称为 Actions 运行。

其余 `*-verification.json`、`verification-summary.json`、`test-summary.json`、`review-summary.json` 是主代理生成摘要。15项旧验证器实红与120项本机 runtime 绿灯由执行代理工具输出报告，未单独保存原 stdout；摘要明确此限制，不能冒充原始日志。baseline 验证器与 `44f5b52:tools/runtime/sqlite-restore-smoke.mjs` SHA256 相同，为 `20daf442c6c3f9f8637f82cc37b8217e5303d8155dce0a0f1d39b123ff86af02`。准确 CI 的62项恢复/运行合同及实际 LAN 恢复另有原始证据。

[本批交付](../../PERSONAL_CONSOLE_ARTIFACT_DELIVERY_20261019.md)、[执行记录](../../PERSONAL_CONSOLE_ARTIFACT_EXECUTION_20261019.md)、[部署与回滚方案（未执行）](../../PERSONAL_CONSOLE_BETA20_DEPLOYMENT_ROLLBACK_PLAN.md)。[可点击页面](http://127.0.0.1:4180/keys) 仅准确 CI 前端与临时内存合成 API，本机只做HTTP核验，浏览器验收在 Actions；不是部署或长期托管。

真实账号/真实凭据密码登录、真实2FA、实体Passkey/security proof、真实上游窗口/429/误差、原生MySQL/PostgreSQL数据库备份恢复、authoritative Redis恢复及真实历史升级均未验。轮换 legacy/bridge/未知资格继续阻断。不操作真实凭据、生产数据/Redis/备份或历史账务；不合并 main、不打标签、不发布、不部署。
