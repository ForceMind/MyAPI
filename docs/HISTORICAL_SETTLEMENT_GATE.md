# 历史结算执行门禁

状态：仅为本地候选。本文档及对应代码不授权部署、生产写入或任何历史结算。

## 目的

金额兼容工作使旧余额可被准确表示，但金额可表示不代表旧结算可以执行。因此，worker 或直接恢复调用在修改未完成的结算 intent/fact 前，必须验证持久化执行许可。

门禁上线前创建的记录没有许可，默认保持冻结。门禁不根据使用日志、金额、时间戳、ID 范围或私有副本结果推断许可。应用启动本身不会修改旧 fact 的身份、金额、状态或逐腿 applied 标记。已经完成的 applied fact 可直接只读返回，防止幂等重放产生额外扣款；它不会被 claim 或再次执行。

## 持久化记录

`account_quota_execution_gates` 保存唯一的强制执行标记。只有标记的 schema 版本准确且状态为 `enforcing` 时，运行期才能签发许可。业务执行路径不会静默创建表或标记。schema 已创建且 intent/fact 表均为空时，fresh 数据库才可初始化标记。

`account_quota_execution_permits` 按 event 保存不可替换的绑定。绑定包含 event、request、request fingerprint、结算 kind、user、token、subscription、reserve receipt、writer epoch、actual quota、delta、是否应用 token、初始 funding/token applied 标记，以及允许的 false-to-true 资金腿。绑定摘要把每个整数编码为精确十进制文本，通过 `common.Marshal` 序列化，再计算 SHA-256。

运行期许可只在首次插入 intent 或直接插入 fact 的同一事务内创建。读取或重放已有记录不会补签许可。旧 intent 没有许可时，后续物化 fact 也不能取得运行期许可。

本批次的显式历史许可 API 仅供受控测试和私有副本使用。调用方必须提供完整预期 fact 绑定、初始逐腿状态、允许执行的腿、批准 ID 和证据摘要。API 锁定并比对数据库中的 fact 后才插入许可。日志与 delta 只能作为证据输入，不能直接代表生产授权。

## 恢复检查

fact scanner 只选择与有效许可关联的记录，因此冻结记录不消耗扫描额度，也不会阻止已许可记录执行。intent scanner 使用相同规则。

直接 fact 恢复在 claim 前验证许可。缺少或不匹配许可时返回冻结错误，fact 保持不变。funding 和 token 事务锁定 fact 后，重新核对完整绑定、许可状态和对应资金腿权限，再更新余额和 applied 标记。applied 标记只能从许可绑定的初始值向明确允许的 true 状态推进，不能倒退。

authoritative 结算在调用 `SettleAccountQuota` 前验证许可，还将绑定摘要、fact和租约版本传入 terminal 金额事务；该事务在任何金额写入前重新锁定并校验。写 fact 终态的后续事务再次验证。许可撤销或绑定变化不能绕过金额事务内的检查；底层 terminal receipt 保留原幂等身份。

当 `ApplyToken=false` 时，`token_applied=true` 仅表示无须执行的 Key 腿已收尾，不产生 Key 金额写入。只允许 funding 的历史许可可以完成该收尾；实际 Key 扣款仍要求 `ApplyToken=true` 及明确的 `AllowToken` 许可。

## 启动边界

原生只读迁移前探针由独立模块实现。它必须在 `AutoMigrate` 之前运行，不能通过关闭 batch update 绕过。门禁缺失时，只要 intent 或 fact 表中存在任意记录，普通升级即被阻断，避免迁移先写入部分 gate schema。安装 enforcing gate 后，无许可历史记录保持冻结，本身不阻止启动；格式错误、重复、悬空或绑定不一致的许可会阻止启动。

历史 manifest 导入和生产 schema staging 不在本候选范围内。门禁代码不包含固定事件数量或事件 ID。
