# beta.21 隔离验收控制交付

状态：本批隔离交付完成，准确候选CI11/11成功，源码/文档及32原PNG独审通过。本批不是发布或部署，真实账号/凭据、生产数据和历史账务继续冻结。

## 候选与范围

版本 `0.2.0-beta.21`，分支 `codex/subscription-key-share-20261014`，继续Draft PR #17，base `codex/personal-core-20261014`。核心功能提交 `ac3aa6c8a5dca0e227ff7805ce7a4b37533a158c`；分发与数据库fixture修正后准确候选 `c5e44f6140eccf83679d65ed94b9337319cfdf5b`，tree `db5e63575787de004fbed11f4d9baca5a1c78c90`。后续证据归档不冒充被构建源码。

实现默认关闭、需显式启用的单进程/单个既有Codex账号验收run：总预算、OAuth换码/刷新/采样/模型/生成分类预算，单生成并发、期限、停止锁存、HTTP body与外层operation分别排空、重启不接管旧run。只保存脱敏引用和计数；有效Root维护会话控制，POST沿用现有可信来源校验。配套脚本只连接literal loopback，无推理发送、自动登录或写请求重试。

停止后不会再准入新请求；已成功取得旋转凭据允许有界安全落库，未结束时仍stopping。最终栅栏明确未发送的请求清理text/share证据并允许原有退款；已发送但结果未知时保留待核查与退款保护。旧Key身份、来源资格、近似原生窗口、缺usage成功规则和账务writer保持。验收进程暂停后台配额采样、自动refresh和Codex模型更新；不是全进程或跨集群网络停止。具体边界见[接口说明](PERSONAL_CONSOLE_ACCEPTANCE_CONTROL_API_20261019.md)。

统一更新VERSION、npm元数据和部署示例中的候选版本。没有改前端组件、翻译键或既定视觉；实际Go接口证明与页面合成API回归分别报告，不把页面截图当作控制机制或真实供应商证明。

## 验证与修正

本地最终21顶层/77叶子Go测试及CLI3例通过。Go测试只用临时SQLite、合成凭据、fake transport和loopback。两writer共8例覆盖最后准入拒绝的零发送/退款、已发送未知的保守收尾，以及HTTP与逻辑operation排空；真实TokenAuth→Distribute→Relay证明停止等待正常结算及handler返回。真实HTTP控制路由验证会话/Origin、一次受控采样、到限拒绝与停止后零新增派发。成功refresh后触发停止仍持久化新的合成旋转凭据。MySQL/PostgreSQL本地明确skip；最终Actions实际三个数据库共48叶子通过。

原始失败保留：初版合成auth请求缺固定redirect_uri，修正fixture后通过，未削弱协议；首次CI39218472756分发合同发现既有证据.log进入npm包。新增证据目录`.npmignore`与pack禁止整目录规则，干净archive副本实际manifest/pack通过，tracked canary不被收录；包内运行源码/测试保留。最初本地嵌套npm的EPERM及未跟踪canary导致dirty-tree拒绝分别排除环境与fixture问题，未改安全断言。

三名代理按文件所有权分工，实际配置gpt-6-astra/high，主代理唯一串行运行Go。非实现者`acceptance_final_audit`只读复核源码、原始测试和文档，无剩余阻断P1/P2；没有把他人的实现自查冒充独审。此前发现的诊断operation遗留、后台模型误占预算、无界DB等待和到期收尾问题已修正。包范围修正与PG重开fixture补审通过；32/32原PNG逐张查看，无视觉P1/P2。详见[独审记录](evidence/personal-console-acceptance-control-20261019/independent-review.md)。

准确候选[CI39220781498](https://github.com/ForceMind/MyAPI/actions/runs/39220781498)最终11/11成功：三库、Redis、全部Go/vet/独立relaykit/386与race（含新控制机制）、243文件1562前端测试、类型/lint/生产构建及全部既有Chromium通过。manifest/pack实际3232文件、运行工具65合同通过。checkout0eadc1ac332031145cd425111cc0800dc2026526与候选同tree。结果、实际运行预览与原图来源见[证据索引](evidence/personal-console-acceptance-control-20261019/README.md)。CI39219648708还揭示PG fixture复用Config.Conn连接池，重建factory得到独立池后最终三库全过。此前失败CI不作为最终通过；Docker自动工作流因分支白名单skip，不计成功，也未把beta.20测试bundle重标beta.21。

[实际合成预览](http://127.0.0.1:4180/keys)运行最终准确CI前端与临时内存API，HTTP200/no-store及HTML逐字核对通过；控制机制另有真实Go路由/Relay测试，不能把预览当作本机真实DB或供应商验证。32原PNG与13原报告逐字对应工件并核对SHA256；14图为真实Go/合成DB，18图为合成API页面，窗口替换两图只支持展示。所有图独审无P1/P2，暗色秘密遮罩内部不验。服务依赖会话，不是部署或长期托管。

## 剩余事项与下一步

本批收口后进入真实测试资料和动作预算确认，仍需另行明确真实账号/凭据的使用范围。旧Codex OAuth/refresh供应商接受情况、真实原生窗口/429/近似误差、原生MySQL/PostgreSQL恢复和目标历史兼容仍未验；可选独立SIWC beta.22未实施。

[部署门槛](PERSONAL_CONSOLE_BETA21_DEPLOYMENT_GATE_20261019.md)与原有恢复方案仅为准备。只有候选验收、真实测试、目标兼容与原生恢复具备证据，并明确准确制品/目标/维护窗口的部署授权后才可执行；现在无法承诺日期。功能分支推送、Draft PR和临时预览都不等于部署。

建议本批准确证据归档并同步后新开会话，使用[续接提示词](PERSONAL_CONSOLE_BETA21_NEXT_SESSION_20261019.md)。维护者两份未提交计划文档和设计草稿均保留，不擅自暂存或覆盖。不合并main、不标签、不发布、不部署，历史账务继续冻结。
