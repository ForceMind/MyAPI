# 本机 beta.21 部署预检与 Full 合成恢复

2026-10-20已定位实际升级对象：Docker容器`my-api-beta12`，当前公开版本`0.2.0-beta.12`，健康，绑定`127.0.0.1:3000`。原镜像`sha256:e05ae0cce3236b54b244d8162b441054151ec99773052f5be77b79eb1a3611e2`；实际数据文件`/root/new-api/fresh-beta12-20261014/data/my-api.db`。只读取容器元数据、公开状态、数据库文件名/大小，没有打开旧库、复制生产数据或读取真实凭据。见[target-metadata.json](target-metadata.json)。此前“会话进程中无MyAPI”不代表本机Docker中无服务，今后按此实际目标推进。

## 候选已在本机就位

准确Full镜像包已加载。本机Docker的`image inspect Id`为OCI manifest摘要`sha256:5726995bf0a66bf2a125c9c69a90e7d5cea0e052151ff0aaf31b375d817893d6`，源包记录的配置摘要为`sha256:510f266ab14831a072495e9edf00342420dadf46bb53d9fc41c15c9295021912`。

原包校验器要求Id等于配置摘要，因此返回`IMAGE_BUNDLE_IMAGE_MISMATCH`；未放宽或修改校验器。随后从已验证SHA256的原始tar中定位本机Id对应的manifest，逐一验证manifest、所指配置与全部layer blob的摘要，并将运行时RootFS/标签与配置逐字段比较，全部相符。本机Id确为原包中的同一OCI manifest，不是另一源码或替代镜像。Bun接口夹具也按同样方法核对。准确映射见[image-identity.json](image-identity.json)。

## 本机 Full 恢复演练

仅使用本次全新合成SQLite、自动生成Root、合成Codex凭据和受限Key；没有真实账号或供应商请求。准确Full候选运行于新建内部网络，接口由已校验Bun夹具在对应容器网络命名空间内访问，不开放宿主机端口。2026-10-20北京时间11:51:13至11:51:28完成：

- 正常setup/login及合成预置成功；run各发送分类均0。
- 通过SQLite原生backup API建立一致备份，完整性检查为`ok`；恢复使用独立工作副本，启动前字节与备份一致。
- 恢复后正常密码登录成功，用户/Key/渠道ID、权限、Key秘密、模型范围、限额和计数保持一致，个人余额保持0。
- 备份中的旧run在新进程`owner_current=false`、`can_dispatch=false`，没有自动接管；原备份SHA256未变。
- 临时Key停用、会话撤销，两次容器干净退出；本批内部网络、容器与合成数据目录已删除。原beta.12实例镜像、端口及健康保持一致。

第一次宿主机端口探测超时，应用日志证明beta.21已经正常启动；该次容器/网络已清理。改为容器网络命名空间内探测后完成上述演练；未将首次失败算成功。首次结果和日志见[first-network-timeout.json](first-network-timeout.json)、[first-startup.log](first-startup.log)，最终结果见[synthetic-restore.json](synthetic-restore.json)。[restore-operator.py.txt](restore-operator.py.txt)为本次合成操作准确源码，只作证据，不是新的产品驱动。

## 实际下一步

本机候选及合成恢复已完成。下一批应直接对上述旧实例做只读一致备份，并在断网副本验证beta.12→beta.21启动/迁移及原备份配旧镜像恢复；不得再次建立合成批次替代这个实际目标。必要配置和身份keyring必须原样保留，不能换writer、清未决记录或改账务放行。旧实例和Key身份保持，副本失败时保留现场，旧服务不受影响。

生产数据库/备份和真实凭据此前明确冻结；本批未解除。实际旧库副本验证需维护者一次性批准该整批只读备份/副本操作，生产切换仍在成功证据与准确维护窗口确定后单独批准。现有本机Codex会话不刷新；先核对获准旧库中已有的脱敏刷新/用量证据，缺少证明时如实保留，不擅自追加供应商请求。

复核见[verification.json](verification.json)，字节索引见[manifest.json](manifest.json)。主代理结构化复核，无新增独立审查、部署或发布；受保护修改未变。
