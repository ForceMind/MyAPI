# beta.18 凭据维护运行证据

状态：本批已通过验收，完整CI11项作业均成功。原始 PNG/JSON 来自同一次 GitHub Actions，不使用生产数据库、真实账号或真实凭据。PNG 保留原字节，没有重绘、事后裁切或拼接。

## 来源

- 功能分支源码 `6274bac5e0c0d8a618a4a6083c59f1cfe39d561c`。
- [CI 38911014066](https://github.com/ForceMind/MyAPI/actions/runs/38911014066) checkout `6a7c4ec18f99fabf810052e9549b64f72ba76367`，二者 tree 均为 `0c9218cbf4faf2845033e61fdf46f563dd11256f`；源码提交和 PR checkout 标识分别记录。
- 生产前端产物 `myapi-console-build-6a7c4ec18f99fabf810052e9549b64f72ba76367`，artifact `12049869105`；index SHA256 `37dfed715745bb37df331628eb8332b08562b04b7af5421c8d91dca9c4a32801`。
- 原图与报告产物 `myapi-credential-maintenance-6a7c4ec18f99fabf810052e9549b64f72ba76367`，artifact `12050048188`；[维护作业 117006748662](https://github.com/ForceMind/MyAPI/actions/runs/38911014066/job/117006748662)成功。
- 候选版本 `0.2.0-beta.18`；原 M SHA256 `59402599f35c05534c2269f5d279b3cb2af2adf23e5bc79215534f12347b4428` 保持。未标签、发布或部署。

## 真实维护接口的浏览器集成

`myapi-credential-runtime-artifacts/` 保存两个原始报告及8张原图。1440英文和390中文均使用真实 Go router、签名 Root 会话和2FA，在各自临时 SQLite 上完成预检、确认、提交、回执及关闭重开。维护4路由和 `/api/verify` 由真实 Go 处理；其他控制台列表、启动和安全状态读取明确使用 fixture。

每场景 maintenance GET 2次200，preview/verify/commit/receipt 各1次200；轮换 POST 1次404；无重复提交、页面错误或外部请求。Go 断言整行 Token（含 secret、owner、ID、预算/财务字段）、已有 source/target usage、snapshot、generation 和 Root 金额不变；源分额0、目标25%、目标 allocation ID复用，assigned channel只从7改为8，恰好1条不可变回执。两场景合计7.102s通过。

| 状态 | 桌面1440 | 手机390 |
| --- | --- | --- |
| 打开与轮换只读 | [加载](myapi-credential-runtime-artifacts/en-1440/credential-maintenance-runtime-en-1440-loaded.png) | [加载](myapi-credential-runtime-artifacts/zh-390/credential-maintenance-runtime-zh-390-loaded.png) |
| 双确认与保持25% | [确认](myapi-credential-runtime-artifacts/en-1440/credential-maintenance-runtime-en-1440-confirmed.png) | [确认](myapi-credential-runtime-artifacts/zh-390/credential-maintenance-runtime-zh-390-confirmed.png) |
| 提交和非秘密回执 | [已提交](myapi-credential-runtime-artifacts/en-1440/credential-maintenance-runtime-en-1440-committed.png) | [已提交](myapi-credential-runtime-artifacts/zh-390/credential-maintenance-runtime-zh-390-committed.png) |
| 重开 | [目标已为当前账号](myapi-credential-runtime-artifacts/en-1440/credential-maintenance-runtime-en-1440-reopened.png) | [重新打开](myapi-credential-runtime-artifacts/zh-390/credential-maintenance-runtime-zh-390-reopened.png) |

390重开截图首屏只展示标题/Key/轮换区；目标账号出现在DOM由driver等待和报告断言证明，截图本身不证明画面外内容。输入在截图前由driver遮敏；没有将 Key、JWT、2FA secret、proof 或临时数据库入库。

## 合成异常与多设备状态

`myapi-credential-browser-artifacts/` 为准确生产前端加明确合成 API 的4条旅程、13张原图：1440阻断指导、1440键盘成功、390读取失败重试、320越南语暗色长文及未知提交。unknown关闭重开和同标签刷新始终保留同一 operation ID，1次POST/4次receipt GET；404不开放新提交。错误与违规计数均为空。

代表图：[阻断指导](myapi-credential-browser-artifacts/desktop-blocked-guidance-blocked.png)、[读取失败](myapi-credential-browser-artifacts/mobile-read-failure-read-failure.png)、[320完整主按钮](myapi-credential-browser-artifacts/narrow-dark-long-unknown-confirm-actions.png)、[未知结果刷新后](myapi-credential-browser-artifacts/narrow-dark-long-unknown-unknown-reloaded.png)。

## 审查与边界

非实现者 credential_guard 实际查看前候选全部13张，并对照 visual-v0.2 的03/07；最终准确来源8张真实图及3张合成代表图再审通过。首次首屏焦点、320主按钮溢出两项P2已关闭；无剩余P1/P2，未见敏感值。主线程实际复核桌面/手机提交及代表状态；credential_plan另回读最终真实报告和接口契约。

三库结果见 `database-summary.json`；完整回归最终状态另见 `verification-summary.json`。失败候选详见实施记录，不计入最终通过。

本证据不证明真实服务商 OAuth/TLS、真实账号/生产迁移或实体 Passkey。API Key轮换的缓存locator及全部legacy/async在途生命周期兼容尚未证明，因此无轮换提交路由。sessionStorage恢复仅限同一标签生命周期，不覆盖关闭标签、清站点数据或跨设备。

实际只读预览：[127.0.0.1:4175/keys](http://127.0.0.1:4175/keys)，准确CI构建和合成读取/预检；迁移与轮换提交均405，无真实数据库。预览依赖当前会话/临时文件，不承诺长期托管。真实Go页面流程由上述临时服务测试证明，测试结束即清理。

历史账务、生产数据库/Redis/凭据/备份继续冻结；未合并main、发布或部署。完整交付见[凭据维护交付记录](../../PERSONAL_CONSOLE_CREDENTIAL_DELIVERY_20261016.md)。
