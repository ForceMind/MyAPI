# beta.21 隔离验收控制证据

状态：隔离验收完成，准确CI11/11成功；32/32原图独审通过，原文件摘要核对完成。最终交付见[交付说明](../../PERSONAL_CONSOLE_BETA21_DELIVERY_20261019.md)。

## 来源

- 最终候选 `c5e44f6140eccf83679d65ed94b9337319cfdf5b`，tree `db5e63575787de004fbed11f4d9baca5a1c78c90`，版本 `0.2.0-beta.21`。
- [CI39220781498](https://github.com/ForceMind/MyAPI/actions/runs/39220781498)，checkout `0eadc1ac332031145cd425111cc0800dc2026526`；`ci-source-tree.json`证明与候选同tree。完整结论见`ci-final.json`：11/11成功。
- 核心功能 `ac3aa6c`；`c6b2ad1`只修正证据npm排除/安全打包规则与说明；`c5e44f6`只修正PostgreSQL重开测试fixture。`source-reuse.json`比对2968个Go/module/VERSION/driver/前端文件，只有该fixture发生变化，生产与前端字节保持。
- 本地只用合成凭据、临时SQLite、fake transport/literal loopback；实际Go路由执行与页面合成API截图分别说明。无真实提供商请求、账号/凭据/生产数据/Redis/备份、历史账务操作或部署。

## 原始日志与实际结果

`execution-results.json`保存准确命令、退出码、测试名及不重复计数方法。最终本地Go为21顶层/77叶子通过，CLI3例通过。MySQL/PostgreSQL本地skip不计为通过；最终CI实际三个数据库共48叶子通过，包括预算分类/总量、单生成、scope/owner、停止、到期/重开、并发争抢、无效输入和存储拒绝。

| 文件 | 含义与范围 |
| --- | --- |
| `logs/final-core.*` | 模型/service/controller最终定向测试；SQLite及前置明确skip |
| `logs/relay-boundary.*` | 两writer8例及实际Relay/真实HTTP控制路由；未发送退款、已发送未知、等待正常结算与handler返回 |
| `logs/model-reopen-fixed.*` | 修正独立连接池fixture后的本地SQLite复验 |
| `logs/cli.*` | loopback目的地址、显式预算、stopping语义与无重试3例 |
| `logs/backend-final.stdout`、`logs/frontend-final.stdout` | 最终成功后端/前端完整job原始输出，含所有race、独立relaykit、386与Chromium |
| `logs/credential-final.stdout`、`logs/distribution-final.stdout` | 实际Go/Chromium合成维护及manifest/pack/65运行工具合同成功原始输出 |
| `logs/b2-final.stdout` | 最终CI三库完整原始job日志，新run48叶子全部实际执行 |
| `logs/myapi-frontend-tests.log` | 准确候选243文件/1562测试通过的原始输出 |
| `logs/myapi-frontend-typecheck.log` | 准确候选类型检查原始输出，最终job状态另核对 |
| `logs/pack-fixed.*`、`logs/pack-canary.json` | 干净临时archive、两个打包修正、tracked合成canary；manifest/pack3231文件通过，证据目录0文件入包，运行源码/测试保留 |

原始失败保留，不算最终通过：`control-second`缺少合成auth表单的固定redirect_uri，补正fixture；`distribution-first.stdout`是CI39218472756的pack拒绝旧证据.log；`b2-first.stdout`是CI39219648708的PG夹具清理误关原连接池。该测试工厂显式注入`postgres.Config.Conn`，复用dialector也复用pool；重开现重新构造factory，未改产品代码或断言。`pack-dirty-fixture.stdout`保留未跟踪canary被dirty-tree门禁拒绝的结果；第一次npm子进程EPERM只有工具输出，未伪称存在原始stdout。其他first-go/integration-first/service-fixed/router-runtime是早期过程记录，不重复加到最终计数。

## 实际运行与原图

[可点击合成预览](http://127.0.0.1:4180/keys)使用最终准确CI前端与临时内存API；`preview-verification.json`证明HTTP200/no-store、候选/build身份及HTML逐字一致，SHA256 `bd4f5ad61c4e25f8666cc990e83a8e3029e33ba0439ac6ed6ae427d6302a6456`。本机HTTP核验，真实Chromium操作在Actions；本地没有连接Go生产实例。控制机制本身由真实Go httptest控制路由与Relay证明，不能把合成预览当作该接口联调或真实账号证明。临时服务依赖会话，不是部署或长期托管。

`original-artifact-index.json`逐文件记录准确Actions工件与原路径；截图逐字复制，未编辑。`reports/`为原始资格报告。归档32张：14张真实Go/Chromium的合成迁移与轮换（1440桌面/390手机），窗口/模型/安全6张合成API维护页，以及五页及成功未知/失败状态12张；完整工件保留其余语言/状态，归档采用代表图，不声称已人工查看所有工件截图。32张独审已全部实际查看，无新增P1/P2；窗口替换两张在报告中functional=false，只支持展示结论。

两张fresh-secret-masked轮换原图使用深色秘密遮罩，遮罩内内容不属于视觉可验范围；部分维护截图是滚动modal当前视口、测试可能保留前一步拒绝提示，不能仅凭截图判定整套流程或不可见按钮。交互、权限、未知结果与无重放以原始报告和Go断言为准。个人页面不展示金额余额，缺usage仍请求成功/Token未知，原生订阅窗口近似分额与误差说明沿用确认视觉。

## 独审与未验证

[独审记录](independent-review.md)区分源码/文档/打包/fixture只读审查及图像审查。源码无阻断P1/P2；32原图复核、准确三库/race/完整CI均通过。`manifest.json`已为本目录除自身外的88个文件记录大小/SHA256；原始终端输出通过`.gitattributes`按字节保留。

真实OAuth/refresh供应商接受情况、原生窗口/429/近似误差、生产/跨集群、原生MySQL/PostgreSQL恢复、真实历史升级未验证。没有重新生成beta.21 Docker测试bundle；自动Docker分支门禁skip，不计成功，旧beta.20制品不重标新版本。所有发布/标签/main合并/部署均未执行，历史账务继续冻结。
