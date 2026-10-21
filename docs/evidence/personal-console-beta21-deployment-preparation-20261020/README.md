# beta.21 未发布镜像候选与隔离验收

2026-10-20完成：[Docker隔离验收39361636750](https://github.com/ForceMind/MyAPI/actions/runs/39361636750)，attempt 1，6/6任务成功。本批只准备制品和合成验收，没有发布、部署或新增真实账号请求。

## 准确来源

- 实际构建：`c6641b9d01e04c5ad3ff0434e613612a21e0b10e`，tree `9be0004d19fa8c0135e391d0c5d56744f9827f75`。
- 已验候选：`c5e44f6140eccf83679d65ed94b9337319cfdf5b`，tree `db5e63575787de004fbed11f4d9baca5a1c78c90`；原CI39220781498的11/11结论仍按原交付。
- 两提交的3195个非文档跟踪文件完全相同；94个差异路径均位于Docker忽略的`docs/`。这是构建输入一致证明，不是同tree声明，归档提交不能替换原被验候选身份。复验方法与摘要见[source-proof.json](source-proof.json)。
- 远端现有分支、既有工作流`personal=true`；本机未提交文件均未进入镜像，包括后续合成预置脚本及其测试。没有扩大原候选的CI覆盖声明。

## 镜像包

版本`0.2.0-beta.21`，平台`linux/amd64`，未推送镜像仓库。以下应用镜像ID是配置对象摘要，包的ZIP、内部tar和manifest摘要分别记录，不混称发行版digest。

| 版型 | 应用镜像ID | 本机临时镜像包目录 |
| --- | --- | --- |
| Full | `sha256:510f266ab14831a072495e9edf00342420dadf46bb53d9fc41c15c9295021912` | `/tmp/myapi-beta21-deployment-39361636750/full/` |
| LAN | `sha256:76dcd920c7a0ef9d3a7b455169c8ed94953fe22f54873b60fc9fc302c78801b9` | `/tmp/myapi-beta21-deployment-39361636750/lan/` |

每目录包含`manifest.json`和`images.tar`；包同时包含隔离测试用Bun/Redis夹具，不是既有CLI可直接选择的正式发行版。安装器单独构建的LAN镜像只证明安装器路径，不替换上述候选。原始工件身份、摘要和到期时间见[artifact-inventory.json](artifact-inventory.json)，7个下载ZIP均与GitHub公布摘要及字节数匹配。GitHub设置保留30天；临时目录和远程工件恢复时须重新核对，不能假定仍存在。

Full/LAN的tar摘要、配置对象摘要、源码标签与平台已在本机离线核对，见[Full校验](full-byte-verification.json)和[LAN校验](lan-byte-verification.json)。本机没有加载镜像或启动容器。长期保存的manifest与验收报告证明本次结果，不能单凭manifest重建镜像字节。

已从准确Full镜像的对应层提取含前端的Go可执行文件`/tmp/myapi-beta21-deployment-39361636750/my-api-beta21`，SHA256为`daa88e4c33f7050cff93a3e864faf55ef86aa002065c1a52bba8a2e366e556c9`，通过离线构建元数据确认beta.21、linux/amd64、CGO关闭；没有执行。见[本机运行文件来源](local-runtime-binary.json)。后续隔离验收可使用此文件，无需依赖本机Docker或重编译。

## 验证结果与边界

六个任务为镜像包生成、LAN安装器、Full新装legacy、Full新装authoritative、LAN新装legacy、Full历史交接legacy，全部成功，见[run-final.json](run-final.json)。四个运行报告分别通过26、26、19、8项检查，来源和manifest均与本次下载包一致，见[reports/](reports/)。安装器通过8项检查，见[installer-qualification.json](installer-qualification.json)。适用的7个容器/Redis/网络清理步骤均成功；不适用步骤按原工作流跳过。

Full两种writer均验证真实Go进程、实际浏览器与合成上游的Key调用和权限/计数流程；页面显示beta.21及`c6641b9`构建修订。LAN新装验证合成SQLite恢复；Full交接验证无结算历史的旧库迁移、存在历史结算时拒绝启动且DB/WAL字节不变、原备份配旧镜像恢复。Full personal流程没有执行authoritative Redis崩溃恢复；本批未证明目标真实数据库、原生MySQL/PostgreSQL备份恢复或真实供应商生成/OAuth/refresh。

结构化复核由主代理完成，没有新增独立视觉审查；原32张PNG独审结论沿用原准确来源。本批无产品源码变更，未重跑未变化的三库/race/原页面证据。总核对结果见[verification.json](verification.json)，本目录文件字节/SHA256见[manifest.json](manifest.json)。

下一步沿用已选本机和账号，明确真实Key调用、凭据写入/refresh的动作预算与清理范围，再收口目标兼容、原生恢复和维护窗口。此前3次只读预算已用完，本批没有追加使用；生产部署仍须准确制品、目标、恢复条件和窗口的独立批准。
