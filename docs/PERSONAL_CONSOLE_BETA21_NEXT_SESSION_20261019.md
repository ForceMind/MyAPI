# 历史 beta.21 续接提示词（2026-10-21交接说明）

当前已部署beta.25；维护者要求更换agent并将所有仓库待提交资料归档。最新入口为[NEXT_SESSION_HANDOFF.md](NEXT_SESSION_HANDOFF.md)顶部及[平台计划](MYAPI_PLATFORM_REBUILD_PLAN_20261020.md)末尾反馈。下方beta.21状态、提示词和“未提交/受保护修改”均保留为当时记录，不能直接作为当前任务执行；本次明确授权已将两份WIP、设计草稿、离线工具及证据一并提交。当前产品/UI仍未获维护者接受，后续未新增供应商预算或部署授权。

# beta.21 收口后续接提示词

最新主线状态（2026-10-20）：**本机beta.21已获独立批准并部署成功**。维护者明确批准“现在按该方案部署，维护窗口10分钟”；北京时间12:36，`my-api-beta21`（`82702a71f650a486c635a643d3c9bd01cc1c0a2cceffeae8e6b265d881dabde0`）以准确Full镜像manifest `5726995bf0a66bf2a125c9c69a90e7d5cea0e052151ff0aaf31b375d817893d6`运行，`127.0.0.1:3000`实际HTTP200/beta.21且容器健康。生产目录`/root/new-api/beta21-20261020-bb24de8ed296/`；原`my-api-beta12`正常停止exit0，原目录和准确旧镜像`e05ae0cce323…`保留。不可用时间上限3.482秒。见[实际部署证据](evidence/personal-console-beta21-local-deployment-20261020/README.md)和[已执行方案](PERSONAL_CONSOLE_BETA21_LOCAL_DEPLOYMENT_20261020.md)。

停机后最终原生一致备份为`/root/new-api/backups/beta21-cutover-20261020-bb24de8ed296/original/my-api.db`，SHA256 `1f8d5d838229b0620c42d3292ef9334826fdcb874e225de7c99d68eebd4bc0fa`，配置/身份keyring私有0700/0600；原停止DB字节未变，没有用12:14演练快照上线。1用户、8 Key、3渠道的原字段/秘密/策略与历史账务保持，legacy/epoch1及5条usage_unknown冻结，轮换/迁移门槛不解除。后续回退必须先核验上线后新账务与凭据，不能直接启旧容器或丢弃新记录。

部署的正常后台恢复已获批准，正式实例验收模式默认关闭；没有新增人工供应商测试、登录、刷新或生成。先前实际旧库断网副本/恢复批次也已完成，按[旧库证据](evidence/personal-console-beta21-old-db-preflight-20261020/README.md)保留，不再申请同一授权、重放操作者脚本或扩展合成工具。当前获批部署目标可按真实证据完成，完整个人控制台主目标的其余真实专项不能据此冒称通过。

此前Docker隔离构建[39361636750](https://github.com/ForceMind/MyAPI/actions/runs/39361636750)6/6成功，Full/LAN制品按[记录](evidence/personal-console-beta21-deployment-preparation-20261020/README.md)。最小真实事务已完成`gpt-6-luna`输出OK、9输入+5输出=14 Token、前后WHAM200/7天18%，停止排空及敏感清理通过，原认证文件未变，见[真实证据](evidence/personal-console-beta21-minimal-real-20261020/README.md)。两个3次上游预算均已用完，不重放、不追加。旧OAuth/refresh未做新增真实专项，完整凭据生命周期、分额变化、自然429及下次窗口不冒称通过。已批准切换明确保留这些限制，不能擅自刷新本机共享Codex会话，也不实施SIWC beta.22。

本批隔离验收完成，准确CI39220781498最终11/11成功，32原PNG独审通过。现在可用下方提示词准备真实测试资料与授权；准确来源与归档状态以同目录交付及证据索引为准。

真实阶段、已用完的最小3次授权和未批准的24次完整草案见[记录](PERSONAL_CONSOLE_BETA21_REAL_TEST_PREPARATION_20261019.md)。自动本地建号/正常登录已在本次真实事务执行，无需维护者提供MyAPI用户名密码，不改变正式登录流程。

合成工具见[离线预置脚本交付](PERSONAL_CONSOLE_BETA21_OFFLINE_PRESEED_20261019.md)：仍仅支持生成合成凭据，原13项新Node、3项原CLI及3个顶层Go路由测试证据保持；本次真实验收直接调用已有API，没有修改合成脚本或新增产品驱动。工具/测试仍未提交或进入新Actions，不混称原候选CI。

复制以下提示词开启新会话：

```text
接手 /root/myapi 的 MyAPI 个人控制台。先读取根 AGENTS.md、web/AGENTS.md，以及：
docs/PERSONAL_CONSOLE_IMPLEMENTATION_HANDOFF_20261015.md
docs/PERSONAL_CONSOLE_ACCEPTANCE_CONTROL_EXECUTION_20261019.md
docs/PERSONAL_CONSOLE_ACCEPTANCE_CONTROL_API_20261019.md
docs/PERSONAL_CONSOLE_BETA21_DELIVERY_20261019.md
docs/evidence/personal-console-acceptance-control-20261019/README.md
docs/PERSONAL_CONSOLE_BETA21_DEPLOYMENT_GATE_20261019.md
docs/PERSONAL_CONSOLE_BETA20_DEPLOYMENT_ROLLBACK_PLAN.md
docs/PERSONAL_CONSOLE_OAUTH_PREFLIGHT_20261019.md
docs/PERSONAL_CONSOLE_BETA21_REAL_TEST_PREPARATION_20261019.md
docs/PERSONAL_CONSOLE_BETA21_OFFLINE_PRESEED_20261019.md
docs/evidence/personal-console-beta21-deployment-preparation-20261020/README.md
docs/evidence/personal-console-beta21-minimal-real-20261020/README.md
docs/evidence/personal-console-beta21-host-preflight-20261020/README.md
docs/evidence/personal-console-beta21-old-db-preflight-20261020/README.md
docs/PERSONAL_CONSOLE_BETA21_LOCAL_DEPLOYMENT_20261020.md
docs/evidence/personal-console-beta21-local-deployment-20261020/README.md

先核对实际分支、HEAD、git status、差异和Goal，不从旧main重置。现有分支codex/subscription-key-share-20261014，Draft PR #17，base codex/personal-core-20261014，均保持。beta.21功能源码ac3aa6c8a5dca0e227ff7805ce7a4b37533a158c；打包与PG测试夹具修正后准确候选c5e44f6140eccf83679d65ed94b9337319cfdf5b，treedb5e63575787de004fbed11f4d9baca5a1c78c90。证据归档是后续docs-only提交，不能冒充被构建源码；准确CI39220781498最终11/11成功，checkout0eadc1ac332031145cd425111cc0800dc2026526与候选同tree；32原图独审通过。

已确认visual-v0.2、原M与柔和蓝，沿用已有五页及高级Key维护，不需要再次确认视觉。只有我管理控制台，其他人只用Key；个人页面不显示金额余额；完整回答缺usage仍成功，Token未知；分额按账号原生订阅窗口近似分配，明确误差。

保留Token ID、单值API Key轮换并接受短暂中断；跨账号保持Key身份、逐个迁移。轮换兼容性、authoritative/backfill/schema和未决义务未证明时继续阻断，不切writer绕过。beta.21只新增默认关闭的隔离run、范围/分类与总预算/单生成并发/期限/停止排空；重启不续跑，成功旋转凭据允许有界安全落库。它不是跨集群停止或全进程防火墙。新SIWC beta.22未批准，未实施。

本机部署已经获准并完成，先按最新部署证据只读核对当前线上容器/镜像/挂载/健康和Goal，再处理维护者真实使用反馈；不重新申请同一部署授权、不重放执行脚本。后续完整真实测试仍按未验项列明账号、旧OAuth/refresh路线、原生窗口/429、精确动作预算和授权，不能沿用已用完预算。推荐单账号、单并发、主动20分钟、最多10次短生成、推理不重试；各类HTTP和总上限须另行明确，不能把能力上限当消耗授权。不主动烧空订阅，没有自然429则保留待验。观察下一次原生窗口重置另定时段。

后续已确定使用本机与本机账号，先获准并用完3次只读HTTP，随后另获准并用完一次最小真实事务的3次HTTP：2次WHAM和1次gpt-6-luna正常Key生成，单并发、20分钟上限，无OAuth/refresh/模型查询/重试；实际14 Token、输出OK，窗口前后同为7天18%。临时Root/单渠道/专用Key/新库已正常预置、停止排空及清理，原认证文件未改。该授权已用完，不能重放或自动追加；24次草案未批准。Docker39361636750实际c6641b9源码6/6成功，与c5e44f6非文档文件一致但tree不同。具体来源、摘要、真实结果与清理按两个新证据索引。随后已另获准并完成本机实际旧库一致备份、准确beta.21断网副本升级及原备份配准确beta.12镜像恢复，原身份/秘密/账务均保持，该副本批次时原服务健康未停。之后维护者独立批准10分钟实际切换，beta.21已部署并健康服务于原端口；停机后最终备份、当前生产目录/镜像、身份账务保持及旧实例停止保留，按实际部署新证据为准。历史账务/Redis仍冻结，不合并main、不标签、不发布，不擅自再部署或回退。旧OAuth/refresh和分额变化保持未验；正式原有后台恢复已获本次授权，没有新增人工供应商测试，不再索要已知资料或要求重复批准同一切换。

保留我的未提交修改：docs/MYAPI_MASTER_PLAN.md（SHA256 e058e179a828c752b5b389cde5a88d05c5999c61029948a88e270aad7131b243）、docs/NEXT_SESSION_HANDOFF.md（SHA256 4e21c1e847db911b9143f580778ca63650337d7a330b658f8556aa1f499f1c39），以及docs/design/personal-console-visual-v0.1/与visual-v0.2/drafts/，不擅自暂存或覆盖。

必要时按明确文件所有权并行，只有主代理串行跑Go，缓存GOCACHE=/tmp/myapi-go-cache、GOMAXPROCS=2、GOMEMLIMIT=768MiB、GOPROXY=off、-p1。重型构建/三库/Redis/race/Chromium使用既有Actions。所有gh/远程git按环境权限流程；保留Draft PR模板、AI说明及未勾选人工确认项。无需重跑未变化输入的已有证据；只读核对后给出下一步具体范围。
```

临时预览和`/tmp`制品依赖会话环境，恢复时先核对进程、端口、build revision及工件SHA；不可假定服务仍在线。准确CI工件有保留期，仓库原图与原始证据是长期交接依据。
