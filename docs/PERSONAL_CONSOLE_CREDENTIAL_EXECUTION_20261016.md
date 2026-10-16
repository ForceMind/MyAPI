# 个人控制台凭据维护：已批准实施记录

## 目标与批准边界

维护者于 2026-10-16 确认：API Key 保留 Token ID，采用单值轮换并接受短暂中断；跨上游账号保留 Key 身份，逐 Key 迁移。实施顺序为共同预检与脱敏回执、同账号凭据主体保护、单 Key 迁移闭环、轮换安全前置。轮换兼容性未证明前不开放提交。沿用已确认 visual-v0.2、原 M 和柔和蓝。

仅合成与隔离环境；历史账务、生产数据、真实凭据及备份冻结。不合并 main、不标签、不发布、不部署。控制台 Access Token 的现有单值生成是独立能力，本批不扩大为其轮换协议。旧池关闭与渠道删除均不附带执行。

## 起点与保留内容

- 分支 `codex/subscription-key-share-20261014`，起点 `bd70f79e6a9008a8904083a3ed41754c6b1c628c`，已验证 beta.17；继续现有 Draft PR #17，base 保持 `codex/personal-core-20261014`。
- 保留未提交的 `docs/MYAPI_MASTER_PLAN.md`、`docs/NEXT_SESSION_HANDOFF.md`、visual-v0.1 和 visual-v0.2/drafts；不纳入本批提交。
- 不改历史账务模型、金额算法、历史归因和记录，不将缺 usage 的成功回答变为失败，不制造 Token 数值。
- 当前缓存批次会持久化旧 secret 的 HMAC locator，同步请求也可能携带旧 TokenKey 后结算。原 ID 轮换需要额外生命周期/缓存证明；本批先提供真实预检与明确阻断，不注册轮换提交端点。

## 方案、自查与接口

1. 新增 Root 本人 Key 的 DB-only 维护读取与迁移预检，列出窗口、目标容量、策略及未决阻碍；读取不能偷偷刷新 projection、调用上游或写审计。
2. 既有 Codex 更新必须核对新旧主体连续；新建独立渠道允许新主体。保留 CAS、精准 element 替换、缓存发布和刷新取消后的必要持久化语义。未知不能声称可恢复旧 refresh token。
3. 单 Key、单凭据 Codex 渠道迁移：保留 owner/TokenID/secret，源主体所有启用正分额窗口完整映射目标启用窗口，首批保持原比例。目标必须已有可信基线；缺池由既有配置流程建立，不在迁移中重置任何池。
4. 同一事务锁用户、Token、两侧按主体排序的 gate、pool/allocation/policy，复核版本、容量、当前凭据与可信样本。源分额归零、目标分额、Key 路由和不可变脱敏回执原子提交；目标旧 allocation/usage 必须复用，防止回迁清零。
5. pending/unsampled/uncertain、预算与分额互斥、上层 policy 不允许目标、未知主体/过期基线都阻断。旧历史不搬移；客户端未知提交只回读回执，不重放或自动回滚。

接口：`GET /api/token/:id/maintenance`；`POST /api/token/:id/account-migration/preview`；`POST /api/token/:id/account-migration`；`GET /api/token/:id/maintenance/operations/:operation_id`。四路均 Root/本人 Key，提交另需真实会话、Origin guard、限流及专用 `token.account_migration` 安全证明。输入不接受 owner/subject/policy JSON；服务端推导。轮换只读且 `commit_supported=false`。

自行复核：每一步对应批准目标，复用现有分额/策略/安全证明；接口与 UI 按同一合同开发；避免逐接口串写伪装事务；轮换不具备证明时保持关闭而不削弱账务约束。

## Goal、角色、文件所有权与资源

新持久 Goal 承载本批；主线程负责集成、controller/service/routes、proof scope、schema 注册、七语言、候选 beta.18、CI/隔离运行与文档；保持最多 4 个活跃角色，不递归。未给时间/token 预算，不设置工具预算。

| 角色 | 实际配置 | 文件所有权 |
| --- | --- | --- |
| credential_plan | gpt-5.6-sol / ultra | 先只读规划，后新增 model/token_account_migration*.go、token_maintenance*.go 及专属测试 |
| credential_guard | gpt-5.6-sol / high | Codex OAuth/import/refresh/controller 和 channel credential 保护及对应 Go 测试 |
| credential_ui | gpt-5.6-sol / high | keys 维护新组件/API/测试、personal-api-keys/row-actions 入口、独立 Chromium 驱动 |
| 主线程 | 当前主会话模型 | 其余共享集成、验证、版本、文档；不与角色争写 |

本机只用缓存定向 Go、格式/lint 和轻量脚本；完整类型/构建/测试、三库和 Chromium 优先现有 GitHub Actions。读取 cloud runtime 技能后确认当前 environment_status 工具及 /etc/codex/network-policy.json 不可用，不推断网络授权；远端操作按已有权限流程。shadcn CLI 的 bunx 不存在，使用实际 components.json/base-nova/Base UI 及已安装组件 API，不为 CLI 安装依赖。

## 验收与阶段状态

必须有权限、严格输入、CAS/幂等/事务回滚、旧历史/预算/secret 不变量、unknown 写锁、目标容量、重复迁移不清零和同主体保护回归；三库实际执行；轮换 POST 确认不存在。UI 覆盖成功/阻断/读取失败/未知、会话切换、长文本、键盘、桌面/手机/暗色与七语言。隔离实际运行页面和原始截图归档，非实现者独立审代码及实图。

状态：进行中。规划合同已确定，模型、主体保护和 UI 按所有权并行。已通过的定向检查见下方阶段记录；完整构建、三库、浏览器和原始截图尚待本批 CI 资格验证。只有实际验收完成、用户暂停或必需外部条件受阻才停止；不把前批成功结果当成本批验收。

## 首轮集成与独立审查

- controller/router 定向通过，覆盖严格输入、专用 proof、真实会话和 Origin；首次路由测试暴露仅 Origin guard 不排除 PAT，已新增独立会话中间件并保持原断言，修后通过。
- 迁移模型 SQLite 定向通过，包含双窗口、stale CAS、回迁保留 usage、容量/上层 policy、回执写失败全事务回滚和并发；MySQL/PostgreSQL 尚待 Actions，未从 SQLite 外推。
- 同主体保护的 model/service/controller 定向通过；修复泛用渠道编辑绕过、旧快照 ExpectedKey 失配时不比较主体，以及把过期用量样本误当身份失效的问题。新增后端 EN/zh-CN/zh-TW 文案，区分 unknown 与 mismatch。
- 七语言通过受控 add-missing-keys 脚本添加 53 项并同步；缓存格式和 lint 已检查，发现测试非空断言后已修正。完整 TS/单元/构建待 Actions。
- 独立角色 credential_ui 发现三个 P1：遗漏预算/预留互斥、锁等待后仍用旧时间、旧凭据重新激活借用其他凭据快照。三个问题均列为审查未通过，修复与回归进行中，不进入交付。
- 采样证明将新增 mutable SamplingTarget 的 EvidenceVersion/VerifiedSnapshotID；重新激活或换 credential 清除当前证明，真实成功保存的 native sample 与版本共同建立资格。历史 snapshot/账务不回写；原身份连续性仍用既有历史 provider-confirmed 绑定，与窗口新鲜资格分开。
- 真实 Go/Chromium 验证已接入现有 CI 的独立 job：下载同一次 CI 的准确 production build，运行合成状态矩阵及隔离 SQLite 的真实 Root 会话、2FA、迁移和回执。非维护控制台读取明确为 synthetic，不外推为完整生产集成、真实服务商或实体 Passkey 验证。

## 第二轮集成

- 模型已加入 Token/费用预算及未决 reservation 检查并纳入预检指纹；完成全部锁后重新取得数据库时间并再次完整预检，防止等待锁后沿用过期样本。SQLite 定向通过，三库尚待 CI。
- UI 阻碍按身份、基线、在途记账、预算、容量、策略和窗口分组，给出下一步操作；同类项去重。新增 25 项七语言翻译并通过受控脚本与同步，总计本批新增 78 项。
- 主线程复查发现刷新后才读取采样版本的竞争；安排采用保守资格：本轮刷新改变凭据时不建立新采样 proof，下一轮按已捕获版本实际采样后再放行。修复和定向验证尚待返回。

- 首候选冻结前：最终迁移模型定向 10.689s 通过；controller/router 维护接口定向通过；proof/credential model、service refresh/continuity、controller native TLS/duplicate/rotation loopback 定向通过。新增 configured proof 三库合同，CI 已加入；本机只实际执行 SQLite。10 个修改的 TS/TSX 文件格式/lint通过，两个 driver Node 语法检查、CI/后端 locale YAML 解析通过。
- credential_ui 独立只读复核三项模型 P1 全部关闭；credential_plan 交叉审 controller/service/router/runtime 与 proof，未见剩余 P1/P2。完整前端类型/RTL/构建/Chromium、三库和实际原图仍待 CI，当前不宣告交付完成。

## 首候选 CI 配置修正

`cd4432e` 已同步到既有 Draft PR；Actions `38902138126` 在工作流校验阶段失败（无任何作业/构建/截图，不计为通过）。修正新增 job 的 job-level env 中 runner.temp 表达式，改为 step-level env；额外加入阻断状态的合成 Chromium 原图和零写断言。应用源码和已通过定向结果保留，下一候选重新完整资格验证。
