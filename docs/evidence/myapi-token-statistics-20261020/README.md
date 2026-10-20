# 缓存命中率与 Token 消耗统计（2026-10-20）

状态：本轮开发、隔离验收及主代理原图自查完成；版本 `0.2.0-beta.25`，未部署。唯一阶段记录见[平台计划](../../MYAPI_PLATFORM_REBUILD_PLAN_20261020.md)。本轮没有真实账号、供应商请求、生产数据/Redis/备份操作。

## 准确来源

| 对象 | 标识 |
| --- | --- |
| 最终候选源码 | `391ce9519835758b366eacc7defebc17387d916b` |
| tree | `f66d30c6a7f6df5106776fc76a41e43f3a564b4a` |
| Actions checkout | `427538734c242eb02273912698d8cb2e8635c374`，GitHub API核对同tree |
| CI | [39445757876](https://github.com/ForceMind/MyAPI/actions/runs/39445757876)，最终11/11成功；[原始结果](candidate-391ce95/ci-result.json) |
| 前端工件 | `12276581170` / `myapi-console-build-427538734c242eb02273912698d8cb2e8635c374` |
| 200文件指纹 | [build-manifest.json](candidate-391ce95/build-manifest.json)，SHA256 `b4ca8437ed288ffb07f5f9fb33213ea7c1b692b75f5ba62f69c68fbb3fb9589a` |
| 调用记录浏览器工件 | `12276123353`，同一checkout；只归档必要JSON和6张原PNG |
| 原分支 / PR | `codex/subscription-key-share-20261014` / [Draft #17](https://github.com/ForceMind/MyAPI/pull/17)，base保持 `codex/personal-core-20261014` |

功能统计代码首次提交为7686ae9；391ce95仅补齐同批部署环境示例版本。后续本目录归档提交仅含docs，不冒充被构建源码，不发布或部署该镜像。

## 统计合同

`GET /api/log/stat` 与 `/api/log/self/stat` 在既有quota/RPM/TPM之外增加 `data.token_usage`。按日期、模型、Key、分组、请求ID、上游请求ID及管理员用户名/渠道筛选，self绑定认证用户ID；沿用canonical projection去重。只汇总consume/error调用记录，记录数不冒充去重客户端请求数。

输入、输出、总Token以十进制字符串精确返回。有未知usage时总体为null，另报已知小计、未知条数。缺失、估算或无来源证明的usage不当零；历史默认缓存零不证明真实零。新日志只附加观测依据，不参与计费，不改schema、历史记录、digest或账务；Anthropic缓存输入与Gemini推理计数标准化一次。

命中率 = 有缓存证据记录的缓存读取Token总量 / 同批记录的标准化输入Token总量。返回分子、分母与覆盖条数；不平均百分比，不用计费倍率cache_ratio。原生明确零有效，分母零时命中率未知。读取流式逐行且受取消及10秒期限限制；失败不返回部分统计冒充完整结果。

PC四列、手机两列，方法说明默认折叠。混合记录明确标为已知小计；旧接口无新字段显示统计不可用；读取失败显示重试；身份、权限及筛选改变取消旧读取且不复用旧值。金额隐藏沿用既有设置。

## 验证

| 范围 | 实际结果与原始证据 |
| --- | --- |
| 完整前端 | 254文件、1605测试通过；[日志](candidate-391ce95/myapi-frontend-tests.log)；typecheck/lint/build通过 |
| 根模块 / relaykit | 完整测试通过；[日志节选](candidate-391ce95/backend-module-tests.log)；独立构建、JSON策略、386构建/既有账务回归与race作业通过，详见CI原始结果 |
| 新统计三库 | SQLite合同与controller/service证据边界随根模块通过；MySQL5.7/PostgreSQL9.6合同均通过，[S1原始日志](candidate-391ce95/s1-databases.log) |
| 调用记录与新增统计 | 20/20旅程、19项统计检查通过；[原始JSON](candidate-391ce95/browser/personal-console-qualification.json) |
| 原工作台 / 原生窗口分额 | 30/30、28/28，同目录保留原始JSON |
| 原图自查 | 主代理实际查看6张原PNG，[逐图判定](candidate-391ce95/reviewed-originals.json)；没有独立审查或子智能体 |

19项统计检查覆盖七语言320/1280、中文390/768/1024/1440及空范围、503手动重试。原筛选/分页/详情/已保存内容/返回流程保留；缺usage的已完成回答仍成功、Token未知。原图覆盖PC已知小计/缓存25%、法语320明确缓存0%、中文/俄语320未知、空范围和读取失败；统计区域未发现阻断。零外部请求及无业务写入断言保留，未放宽以通过验收。

本机仅定向前端：旧组件6条失败→修正通过；实际HTTP客户端取消竞态旧实现失败→修正；最终相关5文件31测试及typecheck/lint/i18n通过，原始记录见[local/](local/)。Go、完整构建、三库/Redis/race和Chromium均在既有Actions执行，本机未重跑。

首候选7686ae9的CI39445262225分发作业87/91，4项失败均因deploy/.env.example仍为beta.24；修正示例而未改测试断言。被最终候选替代后整轮取消；[原始失败与终态](diagnostics/7686ae9/)完整保留，不能称该轮全成功。

## 可查看结果

[合成控制台入口](http://127.0.0.1:4181/__preview/operator) → 调用记录；[直接查看统计](http://127.0.0.1:4181/usage-logs/common?pageSize=20)。曾切访客首页时先用控制台入口恢复合成角色。

预览使用上述准确Actions工件，TTY20365；[只读核对](candidate-391ce95/preview-probe.json)证明HTTP200、200文件指纹及5入口JS一致，HTML仅加已声明合成提示。42条合成记录中2条报告用量，已知小计1220、40条未知；缓存250/1100，明确部分覆盖。单独筛选REQ-SYNTHETIC-502可见明确缓存0%。

[预览脚本](preview-server.mjs)仅使用内存合成API、拒绝业务写入、CSP限制自身；不证明真实供应商或生产运行。进程和/tmp依赖会话；恢复须核对端口、revision和SHA，不能假定在线。Actions工件有保留期，仓库原始报告/PNG与[总清单](manifest.json)为长期依据。

## 保留项

维护者WIP未暂存或改写：MYAPI_MASTER_PLAN.md SHA256 `e058e179a828c752b5b389cde5a88d05c5999c61029948a88e270aad7131b243`；NEXT_SESSION_HANDOFF.md SHA256 `4e21c1e847db911b9143f580778ca63650337d7a330b658f8556aa1f499f1c39`。设计草稿和其他既有WIP保留；workflow只提交本批三库选择器，既有离线测试行保持未提交。原分支/base/Draft保持；历史账务冻结、轮换/迁移门槛及SIWC未批准状态不变；不合并、不标签、不发布、不部署。
