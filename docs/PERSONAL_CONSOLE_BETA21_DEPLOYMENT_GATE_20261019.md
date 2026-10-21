# beta.21 部署门槛与下一阶段

当前状态：**本机beta.21已获独立批准并部署成功**。2026-10-20北京时间12:36，`my-api-beta21`（`82702a71f650…`）以准确镜像`5726995bf0a6…`运行，`127.0.0.1:3000`实际返回beta.21且容器健康；旧实例正常停止保留，不可用时间上限3.482秒。原用户/Key/渠道身份、秘密及历史账务核查通过。见[实际部署证据](evidence/personal-console-beta21-local-deployment-20261020/README.md)及[已执行方案](PERSONAL_CONSOLE_BETA21_LOCAL_DEPLOYMENT_20261020.md)。完整凭据生命周期等未验项仍如实保留，未合并、发布或打标签。

准确CI39220781498最终11/11、32原PNG独审沿用原交付。Full/LAN候选[Docker验收39361636750](https://github.com/ForceMind/MyAPI/actions/runs/39361636750)最终6/6成功；实际构建`c6641b9`与原候选`c5e44f6`的非文档文件一致、tree不同，见[镜像来源](evidence/personal-console-beta21-deployment-preparation-20261020/README.md)。另获准的最小真实事务完成正常Key调用`gpt-6-luna`输出`OK`，9输入+5输出=14 Token，前后WHAM200、同一7天窗口18%；run已停止排空，临时Key/会话/进程/新库已清理，原认证文件未变，见[真实证据](evidence/personal-console-beta21-minimal-real-20261020/README.md)。两批各3次供应商预算均已用完。

## 何时可以部署

### 已定位的本机升级对象与执行范围（2026-10-20）

切换前目标为`my-api-beta12`（`181acea8b1a3…`），原镜像`e05ae0cce323…`，原数据`/root/new-api/fresh-beta12-20261014/data/my-api.db`，现已正常停止并保留。当前`my-api-beta21`使用新生产目录`/root/new-api/beta21-20261020-bb24de8ed296/`，仍为`127.0.0.1:3000`、1 CPU/1GiB、`unless-stopped`。不能依据会话进程列表推断Docker无服务，也不能用新建空库替换旧用户和Key。

准确Full候选在本机的运行manifest为`5726995bf0a6…`，原包config摘要`510f266ab148…`。原校验器因本机OCI Id表示差异返回的失败已保留，没有放宽校验；逐blob证明两者对应同一准确镜像，见[本机预检](evidence/personal-console-beta21-host-preflight-20261020/README.md)。

以下为实际切换前已获准并完成的旧库副本批次，**不再重复申请同一备份/副本授权或增加合成批次**：

1. 原生SQLite只读一致备份、配置及身份keyring已保存到`/root/new-api/backups/beta21-preflight-20261020-32be98a3b457/`，目录0700/文件0600，完整性及外键检查通过。原备份SHA256为`d059e69bc4940b111c2a285555a56b805048570e9e3ea28668c6086c238a3ff0`，最终未变；敏感内容不入Git或聊天。
2. beta.21无网络工作副本启动资格通过，版本正确；1用户、8 Key、3渠道的原有字段、秘密、身份及策略完全一致，所比对历史账务全部保持。writer实际为legacy/epoch1，原batch=false、accounting config=1和身份keyring原样保留，不切writer。
3. 原备份另一副本配准确beta.12镜像恢复也通过；两个副本完整性ok、exit0，临时容器已停止并删除。原服务、配置、挂载和健康未变；无供应商出站，不访问Redis。原始结果及适用边界见[旧库证据](evidence/personal-console-beta21-old-db-preflight-20261020/README.md)。
4. 385条settlement fact和1条refund fact均applied；**5条历史usage_unknown继续冻结**。两个迁移backfill cursor已complete，其他4个周期游标不宣称完成。legacy/未决历史/authoritative/schema等高级维护资格仍保持原产品门槛，不开通轮换/迁移或清记录放行。

副本批次之后，维护者又独立批准实际部署；停机排空后的最终备份已重新建立，没有用较早演练快照上线。历史账务/Redis和高级维护门槛仍保持，准确最终备份及当前运行目录以新部署证据为准。

### 实际部署结果与后续边界

已执行的[切换方案](PERSONAL_CONSOLE_BETA21_LOCAL_DEPLOYMENT_20261020.md)完成正常停止、最终一致备份、新生产目录启动、准确版本/健康、公开status/setup和首页JS访问、身份/历史账务对比。最终私有备份根`/root/new-api/backups/beta21-cutover-20261020-bb24de8ed296/`，原始DB SHA256 `1f8d5d838229b0620c42d3292ef9334826fdcb874e225de7c99d68eebd4bc0fa`。原停止数据库字节未变，旧镜像和容器保留，beta.9旧容器未动。首次停机前TCP检查退出与最终成功均保留，未掩盖失败。

正式模式默认关闭验收控制，恢复原有正常后台行为，没有新增人工供应商测试。没有新增密码登录、OAuth/refresh、生成、自然429、下个窗口或分额变化专项。现有access token的一次调用不等于完整生命周期证明；轮换/迁移继续受legacy及未决历史等门槛阻断。SIWC beta.22未批准，不实施。

本次获准部署完成，不再要求重复批准或重跑同一制品；完整个人控制台主目标的后续真实业务资格仍按各项证据处理。今后回退先盘点新账务/凭据/未知外发，不能直接启旧容器丢弃上线后的新记录。用户受保护修改保持，未来新增供应商测试或其他发布操作另按明确范围授权。

## 真实测试资料与推荐首轮

离线预置、自动生成本地账号、安全交付/清理和原24次HTTP授权草案见[真实测试准备](PERSONAL_CONSOLE_BETA21_REAL_TEST_PREPARATION_20261019.md)。其中单独获准的最小3次真实事务已执行并清理，额度已用完；原24次完整草案与生产部署均未获准。

后续获准的[合成离线预置脚本](PERSONAL_CONSOLE_BETA21_OFFLINE_PRESEED_20261019.md)已完成自动建号、真实本地登录/路由和清理验证，受测流程出站计数为0；脚本不接受真实凭据，五个业务分类预算全为0，不替代真实测试或部署门禁。

沿用[前置核查](PERSONAL_CONSOLE_OAUTH_PREFLIGHT_20261019.md)的推荐：一个专用、可撤销的订阅测试账号；单生成并发；主动试验20分钟、最多10次短生成、推理重试默认关闭。OAuth、refresh、sample、models各自预算及共同HTTP上限须在真实试验前按实际路线另行确认。当前控制能力最多1200秒/1000次总尝试/10次生成，是能力上限，不是批准消耗额度。

当前控制范围要求既有、单凭据测试channel及账号，暂不支持channel=0首次创建。真实测试准备需先确认安全预置方式；若尚无既有channel，先列明该准备范围与凭据处理方法，取得真实测试授权后处理，不能临时绕过范围栅栏。

本机与账号来源、Pro套餐、旧会话、只读结果及单次真实Key调用已知，无需再提交这些资料。后续只针对未验证的OAuth/refresh等范围明确动作、预算与凭据清理；共享会话的refresh和撤销可能影响当前客户端，不能擅自刷新本机Codex会话。不在聊天、仓库或截图中传真实凭据。

429只接受自然发生或已知可重复的合法测试条件，没有自然429就标为待验，不主动耗尽订阅。下一次原生窗口重置的观察时段另行确认。缺usage仍成功、Token未知；近似分额明确误差来源，不能伪造原生窗口或换算金额池替代。

## beta.21 特有的升级与回滚注意项

新增 `upstream_acceptance_runs` 为独立脱敏控制表，通过普通与快速迁移注册，不改旧表历史账务内容。升级与重复迁移的跨数据库证据须与候选对应。验收模式默认关闭；生产启用不属于本批，且该模式只是单进程、单账号范围，不能承诺跨集群停止或全进程网络隔离。

回滚前必须先停止隔离run并确认operation、HTTP及generation槽排空、无drain_error；凭据轮换后写入的安全结果不能用旧凭据文件覆盖。旧owner run不会被新进程续跑，旧表记录可以保留，不通过删表制造回滚成功。生产回滚仍以授权目标的原生备份/恢复和旧制品校验方案为准；本批没有执行任何生产备份、恢复或回滚。

## 新会话节点

推荐 beta.21 交付记录、准确提交、Actions证据和续接提示词收口后新开会话，再进入真实账号资料与授权阶段。无需在本批隔离验证中途重开。原 `docs/NEXT_SESSION_HANDOFF.md` 与 `docs/MYAPI_MASTER_PLAN.md` 含维护者未提交修改，本批保留，使用专用 beta.21 续接文件承载新状态。
