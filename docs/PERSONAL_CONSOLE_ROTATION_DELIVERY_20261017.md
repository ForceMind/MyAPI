# 个人控制台凭据轮换交付记录（beta.19 候选）

状态：本批验收已完成。先通过全部资格检查后才接通合格Key入口；最终完整CI的11项作业、真实Go/浏览器、三库/Redis、race及相关实图审查全部通过。仍为未发布、未部署候选。

## 来源、批准范围和运行边界

承接 beta.18 及已批准迁移。候选版本 beta.18 → `0.2.0-beta.19`；沿用 M 和 visual-v0.2。只在现有 `authoritative` 模式、迁移游标完成且旧账务/异步义务证据合格时允许轮换。`legacy`、`bridge`、缺表和未决/未知义务继续阻断；不自动切换 writer、不清队列或补账。

本批没有操作真实凭据、生产数据库/Redis或历史账务，没有合并 main、标签、发布或部署。既有 Draft PR #17 的 base 保持 `codex/personal-core-20261014`。用户两份计划文档和设计草稿保持原有未提交状态。

## 行为与接口

Root 本人真实控制台会话可维护本人 Key。专用二次验证 scope `token.credential_rotation`、Origin、限流、严格 JSON、CAS 和 operation 幂等均由后端检查；PAT/普通用户/Admin不能提交。

- `POST /api/token/:id/credential-rotation/preview`：数据库只读资格预检，不触发采样/投影/队列清理。
- `POST /api/token/:id/credential-rotation`：同事务替换单值 Key、凭据版本加一及持久脱敏回执；Token ID、owner、分额、预算、模型/IP/routing 配置和历史保留。
- `GET /api/token/:id/credential-rotation/operations/:operation_id`：只读原 operation 回执。

新 secret 仅首次成功响应自动显示；重放和回执不含 secret。关闭/身份变更清理内存值；既有显式 Reveal Key 能力保留。无法确认提交结果时只回读原 operation，不重发提交，也不把回执404当作旧 Key 仍有效的证明。刷新恢复仅限同一标签页的 sessionStorage 生命周期。

版本复核的逻辑接纳点是该次发送前成功读取当前数据库版本；轮换后旧快照不得接纳新的发送/外层重试/Realtime新工作帧。此前已经接纳的请求可以继续完成并按 Token ID 结算，不能承诺物理撤回已接纳网络工作。legacy async 在 authoritative 模式下先建立持久登记，与轮换共用锁；不确定登记阻断资格且不能自动释放。

## 验证与证据

先在实际应用不注册提交路由的资格阶段，以e442b2a通过CI38929109218全部11作业；然后接通动态资格和专用路由，并完成当前生产前端到真实Go的验证。最终资格源码、merge checkout、文件树、Actions run/job、原图和结果均在下方及证据摘要中分别记录；文档归档提交不作为CI构建源码。

未验证范围包括生产集群、混合旧版本 worker、真实 provider/实体 Passkey、真实凭据轮换及任何部署。当前 Go/Chromium 真实 API 测试使用临时 SQLite、签名合成 Root 会话和独立2FA；其他控制台读取使用明确的 fixture。三库及 Redis7 合同由独立 Actions 作业验证。

## 实际运行页面

已使用准确CI前端，在 `http://127.0.0.1:4176/keys` 启动隔离合成交互预览并回读200/no-store，HTML逐字匹配构建。仅临时内存里的假 Key 可演示轮换，不连接真实数据库或 provider；这与 Actions 中真实 Go 轮换的证据分别标记。会话/临时服务不承诺长期托管。

## 最终源码与验证

- 资格源码`5536cf8601afe512bbefb92e86b99b038398a133`，[完整CI38935299710](https://github.com/ForceMind/MyAPI/actions/runs/38935299710) checkout为`0c72690c19f1a738f61d6ec54c86c470f679f27f`，文件树均为`ad2289bf3a13b3f3ea354d9e5705a8120456545f`。11项作业全部成功；此处与文档归档HEAD分别记录。
- 实际Go/Chromium轮换两设备6.78s通过，迁移与legacy业务拒绝两设备6.67s通过；原JSON与Go回读保护同ID、secret-free replay/receipt、关闭清除及历史不变。
- 当前三库轮换合同4.06s（SQLite0.25/MySQL0.90/PG2.90）、迁移3.45s、精确采样proof0.43s均实际通过；Redis7四合同0.46s通过。
- 36原PNG/6原JSON及哈希见[原始证据](evidence/personal-console-rotation-20261017/README.md)。独立于所审UI与Root集成实现的credential_guard实际查看23张相关原图和5份报告；无P1/P2、无明文泄露，也无裁切或非敏感内容遮挡。敏感值按测试设计遮罩。
- 本机激活后model30.281/controller0.915/router1.162s定向通过；缺Receipt表补验19.622s，迁移夹具SQLite0.94s通过。MySQL/PG、完整构建/Chromium均由Actions执行，不冒称本机。
- 原M三份资产SHA256仍为`59402599f35c05534c2269f5d279b3cb2af2adf23e5bc79215534f12347b4428`。47新增文案覆盖7语言；当前前端240文件/1540单元（新轮换21项）、type/lint/build及全部浏览器回归通过。lint为0 errors/8条原有warning。完整Go/独立relaykit、linux/386和全部既有及新轮换race通过，详见[最终摘要](evidence/personal-console-rotation-20261017/verification-summary.json)。

| 真实Go轮换 | 桌面1440英文 | 手机390中文 |
| --- | --- | --- |
| 确认 | [原图](evidence/personal-console-rotation-20261017/myapi-rotation-runtime-artifacts/en-1440/credential-rotation-runtime-en-1440-review-confirmed.png) | [原图](evidence/personal-console-rotation-20261017/myapi-rotation-runtime-artifacts/zh-390/credential-rotation-runtime-zh-390-review-confirmed.png) |
| 首次成功（值遮罩） | [原图](evidence/personal-console-rotation-20261017/myapi-rotation-runtime-artifacts/en-1440/credential-rotation-runtime-en-1440-fresh-secret-masked.png) | [原图](evidence/personal-console-rotation-20261017/myapi-rotation-runtime-artifacts/zh-390/credential-rotation-runtime-zh-390-fresh-secret-masked.png) |
| 关闭清除 | [原图](evidence/personal-console-rotation-20261017/myapi-rotation-runtime-artifacts/en-1440/credential-rotation-runtime-en-1440-closed-secret-cleared.png) | [原图](evidence/personal-console-rotation-20261017/myapi-rotation-runtime-artifacts/zh-390/credential-rotation-runtime-zh-390-closed-secret-cleared.png) |

当前预览的index SHA256为`a12c25d8094dc5ef28a6f7cef04c8c7ea5ed7b82ea1d2a91b7b7a47685f5ca30`，临时脚本`/tmp/myapi-rotation-preview.mjs`、运行仅依赖当前会话及临时文件；无长期托管保证。合成安全验证可使用任意六位测试码；这不是实际2FA。真实Go/2FA与变更不变量以CI证据为准。


## 审查、保留范围与同步

相关实现按模块交叉审查，实际原图由独立于所审UI与Root集成实现的credential_guard检查。数据库方言、Realtime夹具/终态、unknown恢复、缺回执表及共享测试库假设等问题已修复并重新验证；文档中HTTP状态、对话框恢复、事务/缓存边界及审查身份措辞也已校正，无剩余P1/P2。先前失败和取消候选的原因、源码及真实分项结果保留于[执行记录](PERSONAL_CONSOLE_ROTATION_EXECUTION_20261016.md)，不计入最终通过。

legacy/bridge、缺表、未知旧义务、未证明的混合worker和生产场景继续阻断或处于未验证范围；不切换模式、不补账，不以清理历史绕过资格。本批已经批准的实现及验收工作已完成，扩大资格或任何部署不属于本批。

最终归档仅更新docs与原始证据，相对5536cf8的非docs差异为空；分支及既有Draft PR #17按授权同步，base保持codex/personal-core-20261014。用户原有两份文档和设计草稿未纳入提交。没有操作真实凭据/生产数据，没有合并main、标签、发布或部署，历史账务继续冻结。
