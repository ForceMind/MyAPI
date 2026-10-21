# 合成离线预置证据

本轮只使用生成的合成凭据、临时 SQLite、真实本机 HTTP 路由与 Node CLI。没有真实账号、供应商业务请求或生产数据操作。

交付、命令、边界与失败处理见[脚本交付](../../PERSONAL_CONSOLE_BETA21_OFFLINE_PRESEED_20261019.md)。基线 HEAD 为 `c6641b9d01e04c5ad3ff0434e613612a21e0b10e`；本轮文件仍为本地未提交变更，不能套用旧候选 CI 身份。

| 原件 | 范围 |
| --- | --- |
| `offline-node.tap` | 新脚本最终13个Node行为用例，13通过、0失败/跳过。HTTP边界为测试替身，不冒充服务端集成。 |
| `control-node.tap` | 既有控制脚本3例回归，3通过、0失败/跳过。 |
| `router-go.jsonl` | 最终真实Go路由回归的原始 `go test -json` 输出，3顶层通过，包括新CLI生命周期、既有Relay收尾和控制接口权限/Origin。 |
| `verification.json` | 实际命令、退出状态、环境版本、准确工具/测试/workflow字节摘要及保护文件摘要。 |
| `manifest.json` | 本目录除自身外各文件的大小/SHA256。 |

真实Go夹具不注入登录身份：脚本调用现有setup/login，库中核对正常密码登录会话。服务端默认HTTP transport与当前provider client的拒绝拨号拦截先做正向自检，再计数完整流程出站尝试为0；无生成、refresh、OAuth、采样或模型业务调用。测试通过还证明停止排空、Key停用、SID撤销、私有文件删除和外部sentinel保留。

这不是全进程或操作系统网络隔离证明；fixture没有启动完整应用后台调度。脚本只拆除自己创建的文件，数据库/进程由夹具或操作者负责。没有新的Actions、race、三库或浏览器结果；源码审查为主代理结构化自查。

首次沙箱socket拒绝及创建返回ID0的过程失败按交付记录说明，不混入最终通过数量。本目录不包含真实秘密、Root会话、完整Key、上游凭据或临时数据库。原beta.21的88项归档未修改。
