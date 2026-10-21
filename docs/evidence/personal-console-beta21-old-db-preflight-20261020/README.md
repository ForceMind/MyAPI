# beta.12 实际旧库备份、beta.21 升级及旧镜像恢复

本批经[完整授权](authorization.md)于2026-10-20执行，**实际旧库升级副本和原备份配原镜像恢复均通过**。没有部署。准确原始执行记录为[备份结果](backup-result.json)、[两个副本结果](clones-result.json)，边界见[limitations.json](limitations.json)。

## 来源与私有备份

- 真实目标：`my-api-beta12`，容器`181acea8b1a34db66744fb7410baa98f9a502cf3aa95c10074f3473c6a703d4c`，`127.0.0.1:3000`。
- 原镜像：`sha256:e05ae0cce3236b54b244d8162b441054151ec99773052f5be77b79eb1a3611e2`。
- beta.21实际运行manifest：`sha256:5726995bf0a66bf2a125c9c69a90e7d5cea0e052151ff0aaf31b375d817893d6`，原包config摘要`sha256:510f266ab14831a072495e9edf00342420dadf46bb53d9fc41c15c9295021912`。同一镜像证明见[本机镜像身份记录](../personal-console-beta21-host-preflight-20261020/image-identity.json)，来源为构建提交`c6641b9d01e04c5ad3ff0434e613612a21e0b10e`、Actions39361636750，不冒称原候选的同一tree或正式发行版。
- 私有备份根目录：`/root/new-api/backups/beta21-preflight-20261020-32be98a3b457/`。原始DB在`original/my-api.db`，SHA256为`d059e69bc4940b111c2a285555a56b805048570e9e3ea28668c6086c238a3ff0`。
- 原库使用SQLite原生backup API建立一致备份，包含已提交WAL状态；完整性`ok`，foreign_key_check无违规。目录0700，全部文件0600；配置/身份keyring、DB和原始日志未进入仓库或聊天。原始备份最终SHA256未变。

备份时间为北京时间12:14:44至12:14:45；副本演练为12:17:00至12:17:10。原服务全程健康且镜像、配置、挂载、端口未变。原服务持续运行，备份时点之后的正常业务不在本备份中；将来切换必须在停止排空后另做最终备份，不能直接用本次旧时点副本上线。

## 实际通过项

| 项目 | beta.21旧库升级副本 | 原备份配beta.12恢复副本 |
| --- | --- | --- |
| 启动前与原备份相同 | SHA256一致 | SHA256一致 |
| 网络与挂载 | network=none，无发布端口，仅新副本目录 | 同左 |
| 版本 / 已初始化状态 | beta.21 / 保持 | beta.12 / 保持 |
| 用户、Key、渠道 | 1用户、8 Key、3渠道；所有原字段、秘密值、身份与策略一致 | 同左 |
| 历史账务 | 所比对历史账务表全部原行/字段值不变 | 同左 |
| writer / 执行门槛 | legacy，epoch=1；enforcing哨兵；实际启动前检查通过 | 原状态保持 |
| 完整性 / 外键 / 退出 | ok / 0违规 / exit 0 | 同左 |
| 容器清理 | 已停止并删除 | 已停止并删除 |

所有原表均按原有列完整比较，既有用户/Key/渠道凭据在内存中逐值比较，不输出秘密。beta.21新建13张表，包含`upstream_acceptance_runs`、分额及Key维护回执相关表；8个旧Key的新`credential_version`均为1。比较不将新列初始化等同于旧列账务变动。

已存在385条settlement fact，均applied；对应385个permit active；1条refund fact为applied；391条legacy usage记录中385 settled、1 admission_failed、**5 usage_unknown**，均原样保留。terminal recovery、projection、batch drain、token budget reservation及account hold记录数为0。两个迁移backfill cursor已complete；其他4个周期游标complete=0保持，不宣称所有工作游标已完成，也没有启用authoritative。

允许的副本运行变化：两次启动均新增自身system instance及后台任务记录，4个周期游标只更新时间。旧镜像另产生3条`error/upstream_transport`采样快照并更新采样目标；原有快照未改，网络=none阻断供应商出站。beta.21隔离模式未新增采样快照。未对后台尝试做物理HTTP计数，因此不把操作者供应商请求0解释为所有后台发送函数调用次数0。

## 适用边界与后续

本批证明目标旧库可按原writer/身份配置启动beta.21，并能用原备份配准确旧镜像恢复。5条usage_unknown保持未知；legacy、未决历史、authoritative及维护资格仍按产品门槛阻断轮换/迁移，禁止改writer或清记录绕过。

没有真实密码登录、OAuth/refresh、生成、下一窗口重置、真实429或分额百分比变化的新验收，不用本批替代这些证据。此前一次真实Key调用及窗口读取沿用[已完成证据](../personal-console-beta21-minimal-real-20261020/README.md)，供应商预算不追加。完整主目标和部署未完成。

原始备份和两份私有工作副本暂留用于部署决策与恢复依据；所有临时容器已删除，没有常驻副本、额外端口或新的后台服务。旧镜像恢复副本中的错误采样不回写原库。待实际部署验收结束、恢复保留期明确后再清理敏感副本；不自动删除唯一备份，文件删除不代表供应商撤销。

这是主代理结构化自查，未增加独立审查或重跑原CI。准确操作源码为[备份操作者](backup-operator.py.txt)、[副本操作者](clone-operator.py.txt)，只作本批证据，不是新的产品驱动，不应重放。复核见[verification.json](verification.json)，字节索引见[manifest.json](manifest.json)。下一步为[本机切换待批准方案](../../PERSONAL_CONSOLE_BETA21_LOCAL_DEPLOYMENT_20261020.md)。
