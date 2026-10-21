# beta.21 本机实际部署记录

**2026-10-20北京时间12:36完成本机beta.12→beta.21切换，服务健康。**维护者已[明确批准](authorization.md)本次10分钟维护窗口；本记录是实际部署证据。此前的CI、真实小范围调用与断网旧库演练仍按各自来源引用，不互相冒充。

## 实际线上对象与制品

| 对象 | 已核实值 |
| --- | --- |
| 访问 | `http://127.0.0.1:3000`，实际GET status/setup与首页/入口JS成功 |
| 当前版本 / 容器 | `0.2.0-beta.21`，`my-api-beta21`，`82702a71f650a486c635a643d3c9bd01cc1c0a2cceffeae8e6b265d881dabde0`，running / healthy |
| 准确镜像manifest | `sha256:5726995bf0a66bf2a125c9c69a90e7d5cea0e052151ff0aaf31b375d817893d6`，Full/linux-amd64，使用本机镜像，未拉取或重建 |
| 原包config摘要 | `sha256:510f266ab14831a072495e9edf00342420dadf46bb53d9fc41c15c9295021912`；与本机manifest为同一镜像的[逐blob证明](../personal-console-beta21-host-preflight-20261020/image-identity.json)保持 |
| 构建来源 | Actions39361636750 / `c6641b9d01e04c5ad3ff0434e613612a21e0b10e`；与原候选非文档文件相同、tree不同，不称正式发行tag |
| 当前生产目录 | `/root/new-api/beta21-20261020-bb24de8ed296/data/my-api.db`；日志为同级`logs`，各挂载至`/data`与`/app/logs` |
| 资源 / 网络 | 1 CPU / 1GiB，`unless-stopped`；原`new-api_default`网络及原别名，仍只发布`127.0.0.1:3000` |
| 正常模式 | `MYAPI_UPSTREAM_ACCEPTANCE_ENABLED`默认关闭；无验收run，恢复原服务已有后台行为；没有新增人工供应商测试 |
| 原实例 | `my-api-beta12` / `181acea8b1a34db66744fb7410baa98f9a502cf3aa95c10074f3473c6a703d4c`，原镜像`sha256:e05ae0cce3236b54b244d8162b441054151ec99773052f5be77b79eb1a3611e2`，已正常停止、exit0并保留 |

原始操作结果见[deployment-result.json](deployment-result.json)，12:37:14的独立回读见[final-status.json](final-status.json)。回读确认本机HTTP200/beta.21、容器持续健康、准确挂载/资源/重启策略和旧实例停止。

## 停机与最终备份

北京时间12:36:10.399请求正常停止，12:36:10.651确认exit0；12:36:13.881首次实际GET status返回beta.21/HTTP200。以发出停止到首次成功探测计算，不可用时间**上限3.482秒**，并非持续探针测得的精确中断时长。12:36:18.466完成健康、数据对比及重启策略设置，处于获准10分钟维护窗口内。

停止采用`docker stop --time -1`，操作者最多等待30秒且不升级为SIGKILL；实际约0.25秒正常退出。最终备份在停止后通过SQLite原生backup API建立，没有使用12:14演练时点的较早快照上线。

- 最终私有备份根：`/root/new-api/backups/beta21-cutover-20261020-bb24de8ed296/`。
- 原始最终DB：`original/my-api.db`，SHA256 **`1f8d5d838229b0620c42d3292ef9334826fdcb874e225de7c99d68eebd4bc0fa`**。
- 同目录保存原完整配置/稳定SESSION_SECRET/身份keyring、停止状态、私有manifest及部署后核查副本和日志；目录0700、文件0600，未进入仓库或聊天。
- 新生产库启动前与最终原始备份SHA256一致。最终备份未变，原停止实例的数据文件也未被新镜像挂载或修改。

## 验证结论

首次上线后的原生一致核查副本完整性ok、外键违规0；所有原表按原有列比较。1用户、8 Key、3渠道的原字段、应用Key秘密、上游凭据、策略及稳定身份表完全保持；本次核查时自动后台凭据变化数为0。新增schema正常初始化，不宣称新列本来存在。

历史账务原行/字段值全部保持，writer仍legacy/epoch1，执行门哨兵enforcing；385条settlement fact、1条refund fact均applied。**5条usage_unknown仍冻结**，没有补扣/退款、清记录、切writer或开通高级维护。最初部署核查仅新增本进程system instance；正常运行后允许原有后台任务继续产生新的记录，不声称生产库永久字节不变。

本机status保持self-use=true、funding=disabled；原已初始化setup状态保留。首页HTML与入口JS都实际返回200，摘要按操作结果保存。没有用用户密码登录、重新生成正式Root、调用Key维护、人工生成或强制refresh；未改共享Codex认证文件。未增加浏览器验收，已验视觉及CI沿用原证据。

## 首次检查与恢复边界

首次操作者在停机前把一个保持的TCP连接当作在途业务，返回`ACTIVE_CLIENT_CONNECTIONS_PRESENT`，**未停旧服务、未建立部署目录或启动候选**。结果与准确源码分别保留为[first-preflight-result.json](first-preflight-result.json)、[first-preflight-operator.py.txt](first-preflight-operator.py.txt)。TCP连接状态不能判断HTTP请求是否在执行，因此改为观察项，采用服务端正常Shutdown/exit0作为实际排空条件；没有放宽应用启动、账务或身份资格检查。最终[执行源码](deployment-operator.py.txt)仍禁止强杀和失败后的不安全自动回退。

本次成功，未执行回退。旧容器、旧原目录、准确旧镜像与最终备份均保留，beta.9已退出实例保持不动。上线后的新账务、渠道凭据刷新或归属变更不能由旧快照撤销；将来需要回退，先正常停止新实例并保存现场，再盘点新增业务/秘密/未知外发，不能直接`docker start my-api-beta12`复活旧凭据或丢弃新记录。旧镜像不得读取已升级的当前生产库。

旧OAuth/refresh仍无新增真实专项，完整生命周期、分额百分比变化、自然429与下个窗口保持未验；轮换/迁移继续遵守原有门槛，SIWC beta.22未批准。当前完成的是这次获准的本机切换及公开访问/数据保持核查，不将整个个人控制台主目标的全部真实验收标为完成。

仓库源码/版本/分支未改，未合并、发布、打标签、推送或暂存用户文件。两份受保护文档SHA256不变。本次为主代理结构化自查，非新增独立审查；见[verification.json](verification.json)与[manifest.json](manifest.json)。
