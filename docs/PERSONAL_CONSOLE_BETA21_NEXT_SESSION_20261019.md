# beta.21 收口后续接提示词

本批隔离验收完成，准确CI39220781498最终11/11成功，32原PNG独审通过。现在可用下方提示词准备真实测试资料与授权；准确来源与归档状态以同目录交付及证据索引为准。

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

先核对实际分支、HEAD、git status、差异和Goal，不从旧main重置。现有分支codex/subscription-key-share-20261014，Draft PR #17，base codex/personal-core-20261014，均保持。beta.21功能源码ac3aa6c8a5dca0e227ff7805ce7a4b37533a158c；打包与PG测试夹具修正后准确候选c5e44f6140eccf83679d65ed94b9337319cfdf5b，treedb5e63575787de004fbed11f4d9baca5a1c78c90。证据归档是后续docs-only提交，不能冒充被构建源码；准确CI39220781498最终11/11成功，checkout0eadc1ac332031145cd425111cc0800dc2026526与候选同tree；32原图独审通过。

已确认visual-v0.2、原M与柔和蓝，沿用已有五页及高级Key维护，不需要再次确认视觉。只有我管理控制台，其他人只用Key；个人页面不显示金额余额；完整回答缺usage仍成功，Token未知；分额按账号原生订阅窗口近似分配，明确误差。

保留Token ID、单值API Key轮换并接受短暂中断；跨账号保持Key身份、逐个迁移。轮换兼容性、authoritative/backfill/schema和未决义务未证明时继续阻断，不切writer绕过。beta.21只新增默认关闭的隔离run、范围/分类与总预算/单生成并发/期限/停止排空；重启不续跑，成功旋转凭据允许有界安全落库。它不是跨集群停止或全进程防火墙。新SIWC beta.22未批准，未实施。

本会话先准备真实测试阶段：按已验能力列出专用测试账号资料、旧Codex OAuth/refresh路线、原生窗口/429的验收步骤、精确动作预算和所需授权。推荐单账号、单并发、主动20分钟、最多10次短生成、推理不重试；各类HTTP和总上限须另行明确，不能把能力上限当消耗授权。不主动烧空订阅，没有自然429则保留待验。观察下一次原生窗口重置另定时段。

当前只授权隔离环境，不接入真实账号、不操作真实凭据或生产数据/Redis/备份；真实测试需我后续明确资料和授权。保持历史账务冻结，不合并main、不标签、不发布、不部署。目标明确后的只读核查按已有授权范围推进；实际部署须准确制品、目标、恢复演练和维护窗口另行明确批准。不要因为候选CI通过就称已部署，也不要将旧beta.20制品当作beta.21。

保留我的未提交修改：docs/MYAPI_MASTER_PLAN.md（SHA256 e058e179a828c752b5b389cde5a88d05c5999c61029948a88e270aad7131b243）、docs/NEXT_SESSION_HANDOFF.md（SHA256 4e21c1e847db911b9143f580778ca63650337d7a330b658f8556aa1f499f1c39），以及docs/design/personal-console-visual-v0.1/与visual-v0.2/drafts/，不擅自暂存或覆盖。

必要时按明确文件所有权并行，只有主代理串行跑Go，缓存GOCACHE=/tmp/myapi-go-cache、GOMAXPROCS=2、GOMEMLIMIT=768MiB、GOPROXY=off、-p1。重型构建/三库/Redis/race/Chromium使用既有Actions。所有gh/远程git按环境权限流程；保留Draft PR模板、AI说明及未勾选人工确认项。无需重跑未变化输入的已有证据；只读核对后给出下一步具体范围。
```

临时预览和`/tmp`制品依赖会话环境，恢复时先核对进程、端口、build revision及工件SHA；不可假定服务仍在线。准确CI工件有保留期，仓库原图与原始证据是长期交接依据。
