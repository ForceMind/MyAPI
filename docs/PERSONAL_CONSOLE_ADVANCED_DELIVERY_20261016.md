# beta.17 高级维护交付记录

状态：本批实现、相关验证、独立审查与证据归档完成；候选版本 `0.2.0-beta.17`，未发布、未部署、未打标签。沿用 `codex/subscription-key-share-20261014` 和 [Draft PR #17](https://github.com/ForceMind/MyAPI/pull/17)，未合并 main。

## 已完成

- 原生窗口重选/恢复：按已确认视觉展示当前及目标窗口、样本时间与新鲜度、全池影响、双确认、未知结果回读和独立关闭池。缺少旧样本/实际恢复连接时显示 Unknown；重选与恢复的 generation/估计差异按既有服务端语义说明。
- 个人安全：密码、账号绑定、Passkey、2FA/备份码、登录会话及现有控制台 Access Token 的完整反馈、读取失败恢复、会话/角色隔离与敏感状态清理。提交中保护一次性秘密返回；未知结果不承诺回滚。密码/删号无法安全回读时先人工核验，再显式决定是否重试。Root 自助删号禁用。
- Key 模型范围：独立三模式编辑，目录仅作为建议，已有/退役模型保留原样，拒绝全部需要明确确认。GET/PUT窄接口仅供启用owner，Root不越权；只写两列，保留额度和其它元数据，普通旧编辑不覆盖省略的模型字段。每次重开先fresh GET，未知保存回读失败保持写锁。
- 保留原M、柔和蓝与五页面桌面/手机外壳，修正暗色portal及手机长ID/动作布局。七语言新增83项、既有vi绑定相关3项修订；版本入口保持beta.17一致。现有渠道登录和Key、个人主页面无金额、完整回答缺usage仍成功/Token未知，以及原生窗口近似误差与容量不预留语义保持。

详细行为见[接口合同](PERSONAL_CONSOLE_ADVANCED_API_20261016.md)。凭据轮换/跨账号迁移仅交付[现有能力和四项必要决策](PERSONAL_CONSOLE_CREDENTIAL_DECISIONS_20261016.md)，没有原地API Key轮换、双值宽限或跨账号迁移的新实现，也未对真实凭据执行生成/替换。

## 准确源码与验证

应用资格源码 `0a1f6c4cfe4d227612791d4ad32e138f23a0ed3a`，CI checkout `c88c8089660e064b31920a02e057e3df34508229`，两者文件树均为 `684782c16074ad62cda6bdb2816a9691342d4e39`。补验证据driver提交 `3820baf5a519479e99127846585cf8ff718e3134` 只改截图脚本、既有手动workflow和记录，来源守卫确认产品差异为空；随后归档提交只改文档与证据，应用保持不变；归档自动CI与准确资格运行分别记录。

- [完整CI 38855315874](https://github.com/ForceMind/MyAPI/actions/runs/38855315874)：10项作业全部成功；233文件/1498单元、type/lint/production build、71核心浏览器、窗口17/模型17/安全20高级维护旅程及六个兼容脚本通过。后端vet/build/test/386/JSON policy、relaykit独立构建、相关race、SQLite/MySQL5.7/PostgreSQL9.6/Redis与分发合同通过。模型三库窄写测试实际执行，默认SQLite不替代三库。
- [隔离Docker 38855486670](https://github.com/ForceMind/MyAPI/actions/runs/38855486670)：6项作业全部成功；准确最终源码 `0a1f6c4cfe4d227612791d4ad32e138f23a0ed3a`，legacy/authoritative各6检查/15原图、0运行错误。真实模型范围两次窄写后恢复、每次打开GET、Root跨owner404、额度/元数据/上游计数保持。源码与CI checkout文件树相同，运行revision使用source0a1f6c4，CI使用PR checkoutc88c8089；构建标识分别记录，不声称二进制相同。
- [截图补验38857597290](https://github.com/ForceMind/MyAPI/actions/runs/38857597290)：使用同一合格c88构建，20安全旅程再次通过，三卡失败原图完整；仅driver3820baf变化，来源/树/产品差异核对通过。
- 本机缓存定向Go、166 Node合同、i18n同步、格式/lint和driver语法检查通过。完整构建/Vitest/Chromium由Actions执行；定向缓存lint保留既有额度编辑测试4项query-selector warning、0 error；完整CI lint通过。

[证据索引](evidence/personal-console-advanced-20261016/README.md)包含桌面/手机原始PNG、三份高级报告、核心报告、两份实际应用报告、运行/版本/三库/只读预览记录及SHA256。前六失败候选与第七轮视觉修正保留，不作为验收通过证据。

## 审查与运行页面

实现者交叉审查权限、窄写/CAS、会话隔离、未知写锁和一次性秘密；非实现者实际打开最终代表桌面/手机/暗色/错误恢复图，审查范围内无未解决P1/P2。已修复长ID挤压、手机动作进入底栏、暗色portal仍浅色、同SID角色变化展示、Passkey按钮高度、全局缓存阻止新GET及文档过度承诺恢复能力。测试locator修正只作用driver，没有为测试更改产品语义。

本地[只读运行页面](http://127.0.0.1:4174/dashboard/overview)服务最终Actions构建及合成GET；概览、Keys、资料入口HTTP200，模型GET200，业务写入和生成Token405。实际浏览器交互/截图在Actions完成，不能将本机HTTP检查称为本机浏览器验收。该预览依赖当前环境会话，不是部署或长期托管。

合成状态测试不能证明真实服务商或实体Passkey设备；WebAuthn使用CDP虚拟认证器。Docker使用reported usage及隔离回环上游，不能称为缺usage实流、原生窗口拒绝或生产历史恢复的证明；缺usage语义由专属UI/API/relay测试覆盖。

## 保持与后续边界

历史账务、生产数据库、Redis、凭据和备份继续冻结。无schema、计费表达式或分额归因算法变更，未合并main、标签、发布或部署。未新增独立历史池目录，当前UI从明确选择的现存Key发现有效池。用户已有两份计划文档与设计目录差异保留，未纳入本批提交。

本批高级维护无剩余实现项。下一批凭据轮换/跨账号迁移需要先决定：对象与身份、旧值失效及恢复、迁移Key/窗口集合与历史边界、账号并存及二次验证/CAS/幂等/审计控制；这些事项尚未实施。
