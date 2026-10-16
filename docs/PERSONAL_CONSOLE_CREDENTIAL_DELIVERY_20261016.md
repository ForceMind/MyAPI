# 个人控制台凭据维护交付记录（beta.18 候选）

状态：已完成本批验收。完整CI的11项作业、维护真实浏览器、合成异常、三库和独立实图审查通过；轮换提交按批准条件继续关闭。

## 来源与边界

- 当前资格源码 `6274bac5e0c0d8a618a4a6083c59f1cfe39d561c`；完整 [CI 38911014066](https://github.com/ForceMind/MyAPI/actions/runs/38911014066) checkout 为 `6a7c4ec18f99fabf810052e9549b64f72ba76367`，二者文件树均为 `0c9218cbf4faf2845033e61fdf46f563dd11256f`。PR构建标识与分支提交分别记录。
- 实现链 `cd4432e → de7124a → baa13a6 → 93eec75 → be87673 → a93041e → 6274bac` 保留；失败夹具、异步驱动及原图修正详见实施记录，不隐藏或计入通过。
- 版本 beta.17 → `0.2.0-beta.18`，只更新候选元数据；未标签、未发布、未部署。原 M SHA256 `59402599f35c05534c2269f5d279b3cb2af2adf23e5bc79215534f12347b4428` 保持。
- 只使用合成及临时数据；生产数据库、Redis、真实凭据、备份和历史账务冻结。用户原有两份文档差异和设计草稿保持未提交。PR #17仍Draft，base为codex/personal-core-20261014。

## 本批行为

Root 真实会话只能维护本人 Key。单 Key 跨 Codex 上游账号迁移保持 Token ID、owner、secret、预算、历史记录及目标既有 usage；已有主/次窗口按原比例完整映射。预检和维护读取只查数据库；提交同事务更新源/目标分额、必要 assigned channel 与不可变脱敏回执。专用 Passkey/2FA scope、Origin、CAS、容量/预算/策略和未决状态检查均在后端执行。未知提交只回读回执，404 不代表未提交。

同账号 OAuth/import/refresh 增加主体连续性保护，普通渠道编辑不能暗换账号。采样 proof 绑定当前凭据生命周期与精确真实原生 snapshot；重新激活、重绑、失败均不能借用别人的新样本。

API Key 保留 ID 单值轮换采用已批准方向，当前仅提供真实只读预检及明确阻碍。旧 secret locator 和全部在途生命周期兼容性尚未证明，所以没有轮换提交路由。控制台 Access Token 的既有单值生成不扩展为 API Key 轮换。

## 已取得的检查

- 本机缓存定向 Go、格式/lint、i18n 同步、Node 语法及 YAML 解析通过；具体范围见实施记录。
- 当前CI的真实Go/Chromium在1440英文、390中文两场景通过，合计7.102s。每场景维护GET2次、预检/2FA/迁移/回执各1次，均200；轮换POST为404，无重复提交、外部请求或页面错误。Go确认Token整行、已有source/target usage、snapshot/generation及金额不变，源分额0/目标25%、目标allocation ID复用、assigned channel迁移且只写1条回执。
- 同一次CI的4个合成旅程/13PNG通过：阻断指导、键盘成功、读取失败重试、320暗色越南语unknown。关闭重开和同tab刷新仅回读原operation ID，1次POST/4次receipt GET。
- 三库migration/proof合同在当前CI均实际执行通过，包含不同op并发唯一胜者和同op幂等回放；见[三库摘要](evidence/personal-console-credential-20261016/database-summary.json)。
- 最终CI11项作业全成功：237文件/1519单元、新维护21单元、type/lint/build、完整Go/relaykit、linux/386、三库/Redis及所有race与浏览器回归通过；高级窗口17/模型17/安全20旅程和旧六个兼容脚本全过。lint有8条原有warning、0 errors，均在本批未改文件。见[最终摘要](evidence/personal-console-credential-20261016/verification-summary.json)。
- 原始21PNG/3JSON、来源和独立审查已入[本批证据](evidence/personal-console-credential-20261016/README.md)。

## 实际运行页面

只读合成预览：`http://127.0.0.1:4175/keys`，临时脚本 `/tmp/myapi-credential-preview.mjs`。它提供准确 CI 前端和明确的 synthetic UI 读取/只读预检；所有迁移和轮换提交返回 405，不连接真实后端或服务商。页面与维护 GET 已回读 200，no-store；合成 session 有效期已核对。

当前预览使用上述6a7c4ec的准确CI构建；index SHA256为`37dfed715745bb37df331628eb8332b08562b04b7af5421c8d91dca9c4a32801`。预览依赖当前会话和临时文件，不承诺长期托管。

此本地预览不能代替真实API资格。真实Go router、签名会话、2FA、迁移/回执及不变量由Actions独立SQLite浏览器测试证明；其他控制台读取为fixture。测试结束清理临时服务和数据库。多状态合成UI与真实维护API证据分别标记，不外推为真实服务商或生产迁移验证。

| 真实维护流程 | 桌面 | 手机 |
| --- | --- | --- |
| 打开 | [1440](evidence/personal-console-credential-20261016/myapi-credential-runtime-artifacts/en-1440/credential-maintenance-runtime-en-1440-loaded.png) | [390](evidence/personal-console-credential-20261016/myapi-credential-runtime-artifacts/zh-390/credential-maintenance-runtime-zh-390-loaded.png) |
| 确认 | [1440](evidence/personal-console-credential-20261016/myapi-credential-runtime-artifacts/en-1440/credential-maintenance-runtime-en-1440-confirmed.png) | [390](evidence/personal-console-credential-20261016/myapi-credential-runtime-artifacts/zh-390/credential-maintenance-runtime-zh-390-confirmed.png) |
| 已提交 | [1440](evidence/personal-console-credential-20261016/myapi-credential-runtime-artifacts/en-1440/credential-maintenance-runtime-en-1440-committed.png) | [390](evidence/personal-console-credential-20261016/myapi-credential-runtime-artifacts/zh-390/credential-maintenance-runtime-zh-390-committed.png) |

## 独立审查与剩余

credential_ui与credential_plan对对方及主线程模块只读交叉审查，预算、锁后时钟、精确凭据采样三项P1修复关闭；刷新中版本竞争、unknown跨卸载P1和412/503分类已修复并由非实现者复核。credential_guard实际查看a930的13张及最终来源8张真实/3张合成代表图，首屏焦点与320主按钮两项P2均关闭，无剩余P1/P2或敏感值。主线程也实际查看代表原图；[审查摘要](evidence/personal-console-credential-20261016/review-summary.json)记录范围。

所有legacy/async在途账务路径不在本批全面证明范围；轮换继续关闭。pending ID恢复只限同一标签生命周期，不覆盖关闭标签、清站点数据或跨设备。真实浏览器只验证合成SQLite维护流程，不证明真实provider OAuth/TLS或实体Passkey。

应用资格源码为6274bac；最终提交仅归档文档与证据，提交前检查相对资格源码的非docs差异为空，不把新文档提交标识冒充CI构建标识。分支及既有Draft PR #17按当前授权同步，base保持codex/personal-core-20261014。用户原有两份文档和设计草稿未纳入提交。

后续轮换仍需证明旧secret缓存定位与全部legacy/async在途生命周期兼容，不能以清队列、补余额或重写历史绕过。当前无需再确认视觉或本批已批准策略。本批未操作真实凭据、未合并main、未发布或部署，历史账务继续冻结。
