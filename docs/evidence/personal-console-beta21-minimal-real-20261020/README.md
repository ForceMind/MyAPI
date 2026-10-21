# beta.21 最小真实 Key 验收

2026-10-20北京时间11:24:40至11:24:45，维护者明确批准的一次最小真实验收完成。准确Full运行文件通过正常MyAPI setup、密码登录、channel/Key及run接口执行；没有修改产品源码、签假会话或使用渠道诊断生成。结构化复核由主代理完成，不冒称独立审查。

## 来源与授权

- 实际构建`c6641b9d01e04c5ad3ff0434e613612a21e0b10e`，可执行文件SHA256 `daa88e4c33f7050cff93a3e864faf55ef86aa002065c1a52bba8a2e366e556c9`；镜像及与原已验候选的对应关系见[制品来源](../personal-console-beta21-deployment-preparation-20261020/README.md)。未使用本机旧`web/dist`或beta.20制品。
- 只读使用已指定本机会话的access token和account ID；仅在本次全新私有SQLite中临时保存凭据。没有复制refresh token、ID token或原文件，没有OAuth、refresh、模型发现、推理重试或模型替换。
- 一个Root、一个单凭据Codex channel、一个仅允许`gpt-6-luna`的临时Key、一个run；单生成并发、期限1200秒，总HTTP上限3，sample=2、generation=1，其余分类0。详见[authorization.json](authorization.json)。原24次完整授权仍未生效。

## 实际结果

| 动作 | 结果 |
| --- | --- |
| 自动本地建号与登录 | 正常setup/login成功，临时Root无需维护者提供账号密码。 |
| 生成前WHAM查询 | 通过本机MyAPI接口读取，供应商HTTP200；Pro、主窗口604800秒（7天）、已用18%。 |
| 正常Key的`POST /v1/responses` | `gpt-6-luna`、固定提示“只回复 OK”；HTTP200，SSE报告`response.completed`，输出精确为`OK`。 |
| 本次报告usage | 输入9、输出5、总14 Token。未归档响应原文或输出正文，仅保留上述数值、完成状态和匹配布尔值。 |
| 生成后WHAM查询 | HTTP200；同一7天窗口，已用18%，reset时间相同。没有返回的窗口不补0。 |
| 本地计数 | Root余额保持0，用户请求计数为1；Key与用户的内部用量计数一致，不将内部quota换算成订阅金额或原生窗口分额。 |
| run最终状态 | `stopped`，total=3、sample=2、generation=1、auth/refresh/models=0；operation、HTTP和生成槽均0，`drain_error=false`。 |

三次请求全部在同一显式run中，分类与共同预算回读相符。两阶段各自单独获准的3次只读查询和3次本次真实查询不能合并为新的剩余额度。本次预算已全部使用，禁止自动再发请求或重开run。35次本地管理/就绪探测尝试不等于上游请求数。

## 清理与复核

依次stop并确认排空、停用Key并回读disabled、正常logout核对SID，再用原本地会话证明401/403拒绝。进程正常退出（exit 0），本次私有SQLite、会话文件、日志和敏感副本目录已删除。另行只读检查确认目录不存在、当前可见`/proc`无该可执行文件进程、临时端口无监听；原认证文件字节相同。没有撤销或改写维护者原本机供应商会话，不把本地删除当成供应商撤销，也不宣称介质物理擦除。

脱敏结果见[execution-result.json](execution-result.json)，复核见[verification.json](verification.json)。[operator.py.txt](operator.py.txt)是本次一次性操作程序的准确非秘密源码，直接调用既有API，作为审计证据保存，不新增产品驱动或修改合成CLI。程序拒绝重复执行同一事务；不应从归档重新执行。没有归档Token、账号ID、邮箱、数据库、原文响应、Cookie或认证日志。文件字节与SHA256见[manifest.json](manifest.json)。

两份受保护文档SHA256与交接一致，设计草稿及其他未提交修改保留；未暂存、提交、推送、合并、标签、发布或部署。

## 尚未证明

本次证明现有access token经正常MyAPI Key鉴权、分发、生成及清理可用。未测试旧OAuth换码或refresh、Key轮换/跨账号迁移、分额策略变化、自然429、缺usage成功、下一次窗口重置、真实目标数据库恢复或生产部署。窗口百分比前后相同只是本次观测值，不能证明零消耗、零误差或全部使用可归属；只返回一个窗口也不证明不存在其他窗口。

继续按原部署门槛收口凭据生命周期兼容性及目标恢复条件。共享本机Codex会话的刷新或撤销可能影响当前客户端，不自动执行；仍需明确后续动作、预算和范围。SIWC beta.22未批准，历史账务、生产数据/Redis/备份继续冻结。
