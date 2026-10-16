# beta.17 高级维护运行证据

本批承接 beta.16；原图来自 GitHub Actions 的 production build Chromium 和临时 Go/SQLite 应用，不使用生产数据或真实服务商凭据。入库 PNG 均为驱动原始字节，没有重绘、事后裁切或拼接。`manifest.json` 保存尺寸、字节数及 SHA256；`verification-summary.json` 汇总准确源码和运行结果。

## 来源与验证

- 应用资格源码 `0a1f6c4cfe4d227612791d4ad32e138f23a0ed3a`；CI checkout `c88c8089660e064b31920a02e057e3df34508229`，文件树同为 `684782c16074ad62cda6bdb2816a9691342d4e39`。
- 完整 [CI 38855315874](https://github.com/ForceMind/MyAPI/actions/runs/38855315874)：10项作业；233文件/1498单元、type/lint/build、71核心浏览器及六个兼容脚本。高级窗口17、模型17、安全20旅程另列原始报告。
- 模型窄写合同的 SQLite/MySQL5.7/PostgreSQL9.6 实际通过，配置测试结果见 `database-summary.json`，不是以默认SQLite测试替代三库。
- 真实应用 [Docker 38855486670](https://github.com/ForceMind/MyAPI/actions/runs/38855486670)：准确最终源码 `0a1f6c4cfe4d227612791d4ad32e138f23a0ed3a`，六项作业成功，legacy/authoritative各6项检查、15张源图及0运行错误。源码与CI checkout文件树相同；运行标识使用source 0a1f6c4，CI使用PR checkout c88c8089，不能称二者构建标识或二进制字节相同。
- [安全原图补验38857597290](https://github.com/ForceMind/MyAPI/actions/runs/38857597290)：同一已合格c88构建、仅证据driver `3820baf5a519479e99127846585cf8ff718e3134`，20/20再次通过；三张失败卡完整截图。来源守卫实际确认产品差异为空；`security-source-identity.json`记录build/producer/driver身份。默认安全报告来自补验，`advanced-security-qualification-ci.json`保留完整CI原报告。
- 本机仅执行缓存定向Go、166 Node合同、格式/lint、i18n同步、脚本检查和预览HTTP核对；完整构建、Vitest和Chromium均在Actions执行。`local-checks.json`标明各自准确输入。

## 代表原图

| 场景 | 桌面 | 手机 |
| --- | --- | --- |
| 全池原生窗口重选 | [1440重选复核](advanced-window-zh-1440-replacement.png) | [390重选复核](advanced-window-zh-390-replacement.png) |
| 原窗口恢复 | [1440恢复复核](advanced-window-zh-1440-recovery.png) | [390恢复复核](advanced-window-zh-390-recovery.png) |
| Key模型范围 | [1440三模式](advanced-model-scope-zh-1440-light-open.png) | [390三模式](advanced-model-scope-zh-390-light-open.png) |
| 个人安全 | [1440安全设置](advanced-security-zh-desktop-1440-panel.png) | [390安全设置](advanced-security-zh-mobile-390-panel.png) |
| 密码维护 | — | [390密码弹窗](advanced-security-zh-mobile-390-password-dialog.png) |
| 暗色、长文和未知 | [1280暗色窗口](advanced-window-en-1280-replacement.png) | [320暗色模型未知](advanced-model-scope-vi-320-dark-unknown-readback.png)、[长旧ID](advanced-model-scope-en-320-light-mobile-preserved-model.png)、[暗色密码](advanced-security-vi-mobile-320-dark-password-dialog.png) |
| 安全读取失败恢复 | — | [Passkey失败卡](advanced-security-zh-mobile-390-error-retry-passkey-failed.png)、[2FA失败卡](advanced-security-zh-mobile-390-error-retry-two-fa-failed.png)、[会话失败卡](advanced-security-zh-mobile-390-error-retry-sessions-failed.png)、[显式重试后恢复](advanced-security-zh-mobile-390-error-retry-recovered.png) |
| 一次性秘密与完整安全流 | [已遮敏Token](advanced-security-en-desktop-1280-complete-flow-access-token-redacted.png)、[完整流结束](advanced-security-en-desktop-1280-complete-flow-completed.png) | — |
| 真实Go集成 | [legacy持久化调用](runtime-legacy-fresh-persisted-log-en-1280.png)、[实际构建标识](runtime-legacy-runtime-build-en-1280.png) | [authoritative持久化调用](runtime-authoritative-existing-persisted-log-zh-320.png)、[模型窄写](runtime-authoritative-model-scope-restricted-zh-320.png) |

`advanced-*-qualification.json`是实际原始浏览器报告。模型/窗口图可能只在journey内列出文件，全部来源原图仍在对应Actions artifact。真实应用两份`runtime-*-qualification.json`分别记录15张源图；本目录选择代表图，其余保留于该次运行的安全artifact，不下载镜像bundle、数据库或凭据文件。

## 审查、修正和边界

实现者交叉审查权限、CAS/窄写、会话隔离、unknown写锁和敏感结果；非实现者实际打开最终代表桌面/手机/暗色/失败恢复原图。暗色portal、手机长模型ID、底栏动作、Passkey按钮高度、Root角色展示和文档恢复承诺问题已关闭；Remove Passkey实测44px，三卡失败标题与Retry由各自原图完整展示；审查可见范围内无未解决P1/P2。静态截图不代替运行断言，也不证明画面外内容。

前六候选失败及第七轮视觉问题的定向修正保存在`*-candidate-failures.json`和实施记录，不当作通过结果。安全Token和密码自动化定位修正只调整driver，未改产品实现以迁就测试。

浏览器高级状态测试使用合成API；Passkey使用Chromium CDP虚拟认证器，不是实体硬件验证。真实Docker用全新临时SQLite、隔离Redis与回环上游，使用reported usage；不证明真实服务商可用性、缺usage实流、原生窗口拒绝或生产历史恢复。完整回答缺usage仍成功/Token未知由专属UI/API/relay合同覆盖。runtime截图中的洋红块是驱动对输入/临时凭据的遮敏，不是产品视觉。

本地只读预览为 [4174](http://127.0.0.1:4174/dashboard/overview)，使用CI构建及合成GET；业务写入和生成Token返回405，预览不是部署。预览会话与临时文件依当前环境保留，不承诺长期托管。

历史账务、生产数据与凭据继续冻结；没有合并main、标签、发布或部署。凭据轮换及跨账号迁移只交付[能力与必要决策](../../PERSONAL_CONSOLE_CREDENTIAL_DECISIONS_20261016.md)，未实施。
