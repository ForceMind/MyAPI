# MyAPI beta.25本机部署与回滚记录（2026-10-21）

状态：维护者明确批准部署及保留账号、Key、历史并做好备份回滚；已于北京时间 **2026-10-21 11:52:11 +0800** 完成。当前[本机入口](http://127.0.0.1:3000)实际返回 `0.2.0-beta.25`，running/healthy。不可用时间上限 **4.276秒**。唯一阶段计划见[平台计划D1](../../MYAPI_PLATFORM_REBUILD_PLAN_20261020.md)。本批没有发布、标签、合并main或额外真实供应商测试。

## 准确来源与线上对象

| 对象 | 准确标识 |
| --- | --- |
| 应用验收源码 | `391ce9519835758b366eacc7defebc17387d916b`，CI39445757876最终11/11成功，完整前端1605测试 |
| 本次镜像构建/驱动源码 | `f90e091e7f95d2beab27f21a7434a6948711b411`，tree `a92f4176e3f5681ac568f535f189edb63e7138a6` |
| 应用输入关系 | 除docs及两个runtime浏览器驱动文件，所有跟踪输入blob与上述应用验收源码相同；[精确核对](application-input-equivalence.json)。构建revision及制品不同，不称同tree/同二进制 |
| 镜像验收 | [Actions39539756860](https://github.com/ForceMind/MyAPI/actions/runs/39539756860)，最终6/6成功；[原始结果](candidate-f90e091/ci-result.json) |
| Full/linux-amd64工件 | `12319340869`，producer attempt1；manifest SHA256 `2823a00f7152763e0e2df1cde40de4eb47bbc16fabb8257129ce6d6ae7cbb398` |
| 镜像tar SHA256 | `710ee7261bd42eaac7fb1746346b582cf5d5443dea268768e46f488403ea00a4` |
| 原包config摘要 | `sha256:b45455052461fd887a4e91f73cdc75651c839ad2d51ea0822c26d4e2411b93ba` |
| 本机准确镜像 | `sha256:cc4d34c65650bca04898a515348bff926a779b52408b1c3117ccd6bd4dcc9ad3`，与原包逐manifest/config/layer及运行时RootFS/配置/标签24个blob核对；[证明](candidate-f90e091/image-identity.json) |
| 新正式容器 | `my-api-beta25` / `68c31d75627a5281b458ae1cb7ddb6959ea1b0f1880b8e8fcbe9cbbbe500efff` |
| 新正式目录 | `/root/new-api/beta25-20261021-e84f72f25e3b/`，data及logs均从最终快照独立复制 |
| 端口 / 资源 / 重启 | 保持127.0.0.1:3000、1CPU、1GiB、unless-stopped、原网络new-api_default |
| 原服务（停止保留） | `my-api-beta21` / `82702a71f650a486c635a643d3c9bd01cc1c0a2cceffeae8e6b265d881dabde0`；镜像 `sha256:5726995bf0a66bf2a125c9c69a90e7d5cea0e052151ff0aaf31b375d817893d6` |
| 原数据（保持） | `/root/new-api/beta21-20261020-bb24de8ed296/data/my-api.db` 与原logs；不挂载给新版 |

Docker29的image Id为OCI manifest摘要，而现有bundle校验器期待config摘要，因此原校验器如实报告IMAGE_BUNDLE_IMAGE_MISMATCH；未修改校验器。先独立核对Actions生产manifest摘要及tar SHA，再逐blob证明本机运行的就是该原包镜像；没有替代拉取或绕过来源校验。本批仅载入未发布工件，未使用GHCR旧版或本机工作树构建。

## 备份与真实旧库验证

- 最终停机后一致备份：`/root/new-api/backups/beta25-cutover-20261021-e84f72f25e3b/`，DB SHA256 `6fbb2d217c0c579d589e89cccc9572d8cd8198bee456ab6305b6401007c86d47`。包含原生SQLite备份、全部data附加文件/历史日志、原Docker控制台日志、容器配置及稳定身份密钥；目录0700/文件0600，真实DB/凭据/配置/日志原件只保留私有目录，没有进入Git或聊天。
- 新版只用最终备份的独立运行副本；原库、原目录、原容器、原镜像及最终原备份保持，完整性ok、外键违规0。线上核对index与5入口JS均匹配断网准确镜像。[执行结果](candidate-f90e091/deployment-result.json)、[实际回读](candidate-f90e091/deployment-readback.json)。
- 当前旧库配准确f90镜像的network=none升级及原beta.21镜像恢复均成功；账号/Key/渠道身份、秘密、权限及历史账务逐原列比较保持。[断网副本原始记录](candidate-f90e091/clones-result.json)。
- **最终停机备份**含后来新增的第6条usage未知，因此又用该准确最终备份的新副本、原beta.21镜像执行一次network=none恢复；成功、完整性ok、身份/历史不变、工作容器清理完成、最终原备份SHA未变、已上线beta.25不受影响。[最终备份恢复结果](candidate-f90e091/final-backup-restore.json)。这不等于执行了生产回退。

## 数据与运行核验

启动后立即在独立一致快照核对：1个账号、8个Key、3个渠道原行及秘密/权限/策略相同；564条原日志、556条结算fact、1条退款fact与所有原历史账务原列/值保持，6条usage未知原样冻结。没有重置账号、重建Key、补扣、退款或改writer。原SESSION_SECRET、CHANNEL_QUOTA_IDENTITY_KEYS、同源/Cookie配置及其余原环境配置逐项继承；legacy/epoch1、batch=false、accounting config1保持，不接Redis。

原日志文件复制并逐前缀核对（允许运行后追加），旧Docker控制台私有备份也已保存。新版启动正常恢复旧服务已有后台刷新/采样等行为，未主动OAuth登录、强制refresh、查询模型或生成；本批启动核验期间上游凭据变化数0。它不是全进程供应商防火墙，也未解禁轮换/迁移门槛或新增SIWC。原生429/窗口重置等未决专项不由本次部署补齐。

身份/历史核验在HTTP就绪后立即完成，随后等待Docker健康，正常新业务继续落在新库；启动快照不是未来随意丢弃新记录的恢复依据。线上当前状态以实际回读和对应时点记录为准。

## 回滚路径与边界

恢复演练已通过，实际未回退。原容器停止但保留，原镜像及其对应旧库/稳定身份配置完整保留。**不要直接用旧镜像打开新版运行库，也不要把切换前备份覆盖到当前新库。**

需要恢复时，先正常停止/排空新版（不强杀），保全当时新库的一致快照、日志、凭据和未决状态；核对切换后的新增业务、Key/上游凭据轮换及归属变化、未知外发结果。只有证明没有不能丢弃的变化，才可从已验证原始备份的独立副本配原beta.21镜像恢复，或使用经核对仍与最终旧时点相同的原容器/旧库恢复。若有新增或未知结果，先保持新现场并对账，不能复活旧秘密或抹掉新记录。恢复后核对健康、版本、用户/Key/渠道身份、账务与访问，再开放使用。当前正常上线后不能盲目执行回退命令。

本次操作精确源码见[operators/](operators/)，仅作本次证据，不可重放；全局状态/随机目录/一次执行门槛属于本次操作。后续恢复应依据当时新业务状态更新步骤。

## 镜像验收修正与边界

部署前实际镜像暴露旧runtime浏览器驱动与成熟平台页面不一致：旧Call logs/Time range/API Key控件已由新Call records/日期/PC展开/手机筛选取代，默认每页100行需显式选择20行；顶部与Key页Search同名需限定main。最后严格预算负向HTTP400又被SSE客户端取消，CDP无法保留体；只在隔离驱动中先读取同一真实错误Response的clone，再原样交给应用，成功SSE/外域不缓冲、不fulfill、不注入认证、不重放。仍要求同请求ID/状态/原始JSON错误码、真实错误界面、账务与上游次数全部正确。所有旧缺体失败合同保留，新增实际回调/错ID/状态/JSON/错误码回归，轻量Node174/174通过；[本机原始结果](local/runtime-wire-observer-tests.log)。应用代码未变，完整应用CI按不变输入复用。

首次12769ca / Actions39535444563最终4/6；e2d493e / 39537013417因分页补齐被取消；fb83d01 / 39537196993在后续Key搜索定位失败后被替代；70cb2e0 / 39538220688最终因CDP体不可用失败。旧服务在这些阶段均保持运行。必要原始失败与原图见diagnostics/，不将失败或取消冒称通过。最终f90已通过完整6/6，[legacy原始资格报告](candidate-f90e091/legacy/qualification.json)26项通过，authoritative、LAN、handoff及安装器均成功。

主代理查看3张最终代表性原PNG，详情中的真实报告用量10/5及严格预算拒绝符合协议；[原图自查](candidate-f90e091/reviewed-originals.json)。没有新增独立审查或子智能体。成功详情顶部仍显示canceled读取取消提示，作为不阻断本次数据部署的界面待办记录；没有为本次部署扩展产品修复，也不宣称界面无所有问题。

两份维护者WIP SHA保持：MYAPI_MASTER_PLAN.md `e058e179a828c752b5b389cde5a88d05c5999c61029948a88e270aad7131b243`，NEXT_SESSION_HANDOFF.md `4e21c1e847db911b9143f580778ca63650337d7a330b658f8556aa1f499f1c39`；设计草稿、workflow原未提交行及其余既有修改均保留。原分支、base及Draft PR17保持，后续docs归档提交不能冒充镜像源码。[总清单](manifest.json)覆盖本目录除自身外全部文件。
