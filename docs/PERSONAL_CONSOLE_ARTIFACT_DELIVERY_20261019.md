# beta.20 第一阶段：制品与隔离验收

状态：第一阶段完成。Docker6/6、准确源码CI11/11、独立制品/44原图审查及证据归档通过。版本保持未发布候选 `0.2.0-beta.20`，部署与回滚方案未执行。

## 候选与范围

功能分支 `codex/subscription-key-share-20261014`，准确源码 `cdeef8b52792a09b341d074521d59cb5cc7f62fb`，tree `1fdefa9eb6edf291dee57e0e92cb8a9bd49f61fa`。CI checkout `37a138b65c038302eda6eaf447e329ec529722b9` 与候选 tree 相同。相对 beta.20 归档 `44f5b52` 只改 runtime 验收工具/合同、Docker smoke 和文档，没有产品代码、schema、计费算法、版本或视觉变化。

[手动 Docker39203596121](https://github.com/ForceMind/MyAPI/actions/runs/39203596121) 最终6/6作业成功；[准确源码 CI39203586608](https://github.com/ForceMind/MyAPI/actions/runs/39203586608) 最终11/11作业成功。PR 自动 Docker39203586542 因白名单 skipped，不计成功。沿用 Draft PR #17，base 保持 `codex/personal-core-20261014`；不合并、不打标签、不发布、不部署。

## 补齐的验收合同

原恢复工具只比较部分 ID/用量，不能完整证明恢复后权限与有限配置保持。新增15项确定性篡改用例：Root/普通用户状态，Key 状态/是否无限/模型开关/模型范围/组/到期，渠道状态/地址/模型/组，结算状态/fact/intent。旧验证器实际15项失败，修正后26/26通过；相关 runtime 回归120/120通过。

恢复现同时核对角色、状态、Key 身份及有限配置、渠道配置、两条日志的用户/Key/渠道关联及 Token/内部用量、applied settlement fact 和可选 intent，前后必须相同。秘密摘要和请求身份仅在内存参与比较，返回报告不包含它们。新增固定安全错误码，不输出原始异常/API/秘密。未修改任何账务执行规则。

工作流为安装与每个消费场景保存独立安全报告、源/树/运行身份及已验证 manifest，Full/LAN 测试 bundle 和原图保留30天。失败 `tee` 文件可能为空；必须同时具备最终作业成功与完整报告，不能从文件存在推出通过。没有 registry push 或发布操作。

## 运行、证据与审查

Full/LAN 不可变测试 bundle 已生成，平台为 `linux/amd64`；两份完整 tar 均在本机重算大小/摘要，并与独立下载的成功消费报告外部 manifest 锚匹配。镜像ID、准确摘要、Actions工件名、过期时间及下载方式见[证据索引](evidence/personal-console-artifact-20261019/README.md)。CI工件保留至2026-11-18，当前 `/tmp` 另保留完整 tar；没有本地加载或部署。

六项 Docker 作业覆盖：干净镜像生产、LAN源码安装、Full legacy个人流程、Full authoritative个人流程、LAN同镜像恢复、固定旧源码handoff。两个个人场景各26项总合同通过，分别使用新临时库和真实安装/登录表单/API，执行选定Key流式请求、用量/日志、权限和预算拒绝、模型范围收窄/恢复及刷新无重放。LAN19项通过，包含新补齐的恢复合同；handoff8项通过，明确无结算历史可迁移、有结算历史必须拒绝，原备份独立副本可由旧镜像恢复，数据库/WAL字节及原身份/限额/用量保持。

76个原文件（Docker30张1280/320、独立Go/Chromium14张1440/390原PNG及对应报告/日志）逐字归档并核对SHA256。非实现者实际查看44/44原图、复核76/76大小与摘要、外部manifest与OCI来源/配置、安装及恢复报告和部署方案；无剩余P1/P2。主代理另实际看4张Docker代表原图。洋红块属于捕获遮罩；旧拒绝通知和滚动modal截图的证据限制已记录，不用静态图代替操作或恢复断言。

[实际可点击隔离页面](http://127.0.0.1:4180/keys) 已运行准确 CI 前端，HTTP200/no-store，HTML SHA256 `b64e86b119ff8e366df39dcc926e406e6b31e7c5a4e0c296b885e67b62d84aa8` 与下载工件逐字一致。仅临时内存合成删除/预算/Key策略 API；没有真实数据库、provider 或凭据。浏览器运行验收在 Actions，本机只核验 HTTP/来源；服务依赖当前会话，不是部署或长期托管。

完整CI还实际通过243文件/1562前端单元、类型/lint/生产构建、所有既有Chromium、独立Go迁移/轮换、SQLite/MySQL5.7/PostgreSQL9.6、Redis、完整Go/vet/race、relaykit独立构建及386。测试门控skip不当作真实上游/凭据或生产验证。准确CI的62项runtime/恢复合同原日志与前端原日志已归档；15项红线和120项本机runtime仅保留执行工具输出摘要，未伪造原stdout。

## 部署与回滚方案

见[完整方案](PERSONAL_CONSOLE_BETA20_DEPLOYMENT_ROLLBACK_PLAN.md)。本机合成项目实际 dry-run 指向 beta.20，`writes=[]`、`dockerOperations=[]`，环境摘要不变且未创建备份。安装/版本8合同、升级结构19合同、干净源码副本11项升级与配置行为合同通过；首次工作区复制受保护路径失败记录保留。dry-run 不证明发行镜像存在、数据库兼容或生产可部署。

本阶段不读取或修改真实凭据、生产数据库/Redis/备份和历史账务。Docker 两种 writer 仅在新建合成实例验收；CI 独立 Go/Chromium 迁移/轮换使用合成签名会话与2FA，不混称镜像凭据轮换。已验证合成账号的实际表单/API密码登录；真实账号/真实凭据登录、真实2FA、实体Passkey/安全proof、真实上游窗口/429/误差、原生MySQL/PostgreSQL备份恢复、authoritative Redis恢复、真实历史升级仍未验证。轮换的 legacy/bridge/未知资格继续阻断，不切 writer 绕过。

最终归档只含文档与原始证据，使用 `[skip ci]` 避免为同一应用/验收输入重复构建；相对 cdeef8b 的非docs差异为空。功能分支/Draft PR正常同步，人工确认项保留未勾选。现有用户计划文档及设计草稿保持。下一阶段真实测试账号/上游和目标环境操作需独立范围与授权。
