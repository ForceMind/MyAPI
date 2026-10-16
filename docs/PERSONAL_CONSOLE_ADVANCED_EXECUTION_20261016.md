# 个人控制台高级维护实施记录

## 目标与批准边界

维护者于 2026-10-16 授权继续完成窗口重选、安全设置与模型范围，沿用已确认 visual-v0.2。凭据轮换与跨账号迁移仅核对现有能力并列必要决策，不实施凭据替换或迁移。此前 beta.16 `630f905` 已验收，归档 HEAD `dd5d54dd4b7c106d80926dcf63a367449447cabc`；沿用 `codex/subscription-key-share-20261014` 和 Draft PR #17，不重置旧 main。

保留原 M、柔和蓝、五页面外壳、现有身份/Key、近似原生窗口与未知用量语义。历史账务、生产数据、凭据、备份继续冻结；不合并 main、不打标签、不发布、不部署。本批自动化仅使用合成浏览器 API 与 CI 临时数据库。用户现有两份计划文档差异及 v0.1/drafts 目录保持原状，不纳入提交。

## 现状与选择

- 窗口重选/恢复已有 Root、CAS revision、操作 ID、同主体/同 slot、显式确认和未知保存合同；高级入口沿用旧通用列表，须补清楚的当前/目标窗口、全池影响与动作反馈，不能暗示跨账号迁移或准点重置。
- 个人资料已保留折叠安全入口，但展开混合通知/绑定、密码、控制台访问 Token、删除账号、Passkey、2FA 与会话卡。按实际能力整理可完成流程、状态与确认，限制迟到回调和认证重放，不自动生成/轮换凭据。
- Key 模型范围藏在完整编辑抽屉的高级区域。现有 `PUT /api/token/` 会更新全部 metadata，不能发送两字段假装部分更新；单独范围编辑需要保持其它字段的窄更新接口及并发保护，或明确使用完整编辑。主线程先核对后确定最小接口，禁止把范围维护转为配额修改。
- 当前 shadcn CLI 未安装；`bun x --no-install shadcn info --json` 已核对不可运行。以实际 components.json（base-nova/Base UI/Hugeicons）和维护中的组件 API 为依据，不为此安装应用依赖。云环境状态工具/策略文件当前不可用，不据此推断网络授权；远端命令依既有权限流程执行。

## 实施顺序与文件所有权

1. 只读规划角色 `maintenance_plan`（工具支持的 gpt-5.6-sol/ultra）核对三流程合同、现有轮换/迁移能力、风险与测试。主线程自行复核步骤的目标作用、依赖、保留内容和可复用流程。
2. 三个实现角色在清楚的所有权内并行：窗口组件/API和对应测试；profile 安全组件/hooks/API和对应测试；Key 模型范围组件/入口/测试。不得递归派工，不争写共享文件。实际派工清单在后续阶段记录补齐。Terra 配置不可用，采用 AGENTS 已列的 Sol 组合承担有安全状态约束的局部实现。
3. 主线程持有必要 Go/controller/router 合同、共享样式/组件、七语言、候选版本、CI/浏览器驱动、文档与集成。预计候选 beta.17，更新统一元数据，不创建标签。新增文案由各实现者列键，主线程用规定脚本同步全部七语言。
4. 必要测试包括窗口影响/双确认/CAS/未知结果/关闭池/会话切换；模型限制 enabled 与空集合的真实语义、原有/已不可用模型保留、窄更新与并发、权限；安全读取失败/禁用能力/表单校验/显式确认/敏感结果不跨会话泄漏。成功、错误、加载、未知、空态与手机长文/键盘动作均覆盖实际改动。
5. 使用既有 GitHub Actions 执行类型/lint/build/单元/必要后端与三库、实际 Chromium 页面、桌面/手机原图；非实现者独立审代码和实图。更新本地只读 CI 产物预览，归档哈希与报告，按批提交并回读远端、更新已有 Draft PR。

## 验收、资源与停止条件

一个新持久 Goal，最多 4 个活跃角色，不递归。主线程负责接口与集成，重型依赖/构建/浏览器优先 Actions；本机仅缓存格式/lint、脚本和必要定向 Go。未指定时间/token 预算，不设置工具预算参数。

交付须是可运行的现有高级能力整理与必要修正，包含真正操作/失败恢复的测试和实际桌面/手机图；没有未解决 P1/P2、必需检查通过、源码/候选版本/证据/功能分支同步可追溯。轮换/迁移交付只读能力表、约束、必要决策，不能以未决定的新流程冒充完成。只有真实验收完成、用户暂停或必需权限/外部条件阻断才停止；一般实现细节与范围内修正连续执行。

## 阶段状态

待验证：三组高级维护 UI、窄字段 Go API、七语言与候选 beta.17 已实现，定向缓存 Go 测试通过。完整 Actions、实际截图和隔离运行仍待新源码执行；旧 beta.16 结果仅作基线。

## 实际派工与接口决定

- `maintenance_plan` 先按 Sol/ultra 只读规划，随后沿用同角色实现窗口组件/API/测试及独占 `tools/ui/window-maintenance-browser.mjs`；不触碰 profile/模型入口。
- `security` 按 Sol/high 持有 profile 安全组件、专属 API/hooks/测试及 `tools/ui/security-maintenance-browser.mjs`。必要的资料读取身份隔离保持原超时/重试合同；不改全局 HTTP/认证实现。
- `model_scope` 按 Sol/high 持有独立模型 dialog/API/helper/测试、Key 入口/types/cells/通用编辑保护与 `tools/ui/model-scope-browser.mjs`。主线程持有 Go、CI/版本、共享样式、翻译和文档。所有角色无递归派工。
- 自行复核后采用最小安全新端点 GET/PUT `/api/token/:id/model-limits`。DTO 只含 Token ID/名称、enabled/list 和 opaque revision；复用完整元数据 fingerprint，排除额度消耗。PUT 在用户/Token 行锁内验证启用 owner 和 fingerprint，仅更新两列，经现有 cache fence。普通元数据编辑省略模型字段时从 SQL 更新列排除它们，避免旧表单覆盖新范围；额度 CAS 编辑保留原 enabled/CSV 和完整 fingerprint。无新增表/列/账务算法。
- 只读规划中“true + 空集合未定义”的建议未采用：实际 relay 把它解释为拒绝全部。UI 明确三状态，拒绝全部需显式选择及确认；候选目录仅为建议，不验证成实际路由可用性、不删除未列出的旧模型，也不扩大访问策略。
- 本机缓存定向 Go/model/controller 及相关原额度元数据测试通过。受限环境的 miniredis 回环 socket 首次被禁止；同一合成测试在已批准的沙箱外重跑通过，不将首次失败归为业务缺陷。三库实际结果待 Actions，SQLite 本机成功不外推其他库。
- 已核对凭据能力并形成 [必要决策](PERSONAL_CONSOLE_CREDENTIAL_DECISIONS_20261016.md)，未调用任何真实生成/刷新/替换/迁移接口。

## 首次集成检查

- 模型独立复核发现普通编辑和 status-only 可能覆盖并发模型范围，已改为按字段是否出现选择更新列，以及仅写 status；补充确定性 handler 交错测试。保留完整额度 CAS。
- 安全独立复核发现 OAuth 旧 callback 可反馈新流程，以及一次性凭据生成中关闭会丢失返回；已加入流程身份守卫、提交中关闭锁与 deferred 响应测试。未知结果明确提示旧凭据可能已失效，不声称回滚。
- 模型未知保存回读失败后必须保持写锁，修改表单不能解锁；只有成功读取当前服务器状态才能恢复提交。
- 七语言新增 83 项经受控 add-missing-keys 脚本与 i18n:sync 同步。
- 最新本机定向命令使用 ^Test(TokenModelLimits.*|TokenMetadataPreservesWideQuota|TokenQuotaVersionIsExactAndStaleEditsWriteNothing)$，model 0.785s/controller 1.863s 通过，覆盖新字段窄写、交错更新、缓存未知和旧标识符。
- 真实 Docker personal 旅程新增 opt-in advancedModelScope：通过真实 UI 和 Go 窄端点限制后恢复模型范围，核对 Root 跨 owner 404、元数据/用量/上游调用不变；结果待 CI，不以脚本检查冒充运行通过。

首次集成缓存格式/lint通过（通用额度编辑测试中原有 query-selector warnings 保留，0 error）；三个新浏览器驱动 node --check 通过。完整隔离驱动 Node 合同 166/166 通过；首次受限回环失败后仅在批准环境重跑。窗口、模型、profile owned 文件已冻结准备新源码 CI。

## 首轮候选补充修正

源码 b85779b 已同步并触发 CI 38845560151、隔离 Docker 38845571447；CI 类型检查观察到失败，日志须等前端作业结束读取，尚不能称通过。独立审查补齐两个范围内问题：security subtree key 纳入 role，避免角色切换后旧请求留下 loading；窗口脚本保留16矩阵和owner场景，en1280/vi320改为dark并实测根主题，vi320增加完整恢复/未知回读功能流。密码/删号未知反馈、显式重试和关闭清理补确定性RTL；没有引入新的凭据恢复协议。

主线程反例检查补拒绝 models 数组 null 元素：Go JSON默认会将它转换为空字符串，不能冒充保留的旧空CSV段。新增回归同时验证 null拒绝不写入、原样旧空字符串可保留。最终定向model/controller再次通过（1.002s/2.069s）。
