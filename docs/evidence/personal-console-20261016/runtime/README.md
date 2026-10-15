# beta.16 隔离真实应用证据

已归档两份原始安全报告和四张原始代表 PNG。两个模式各完成 5 项报告检查、14 张截图，合计 10 项检查及 28 张截图；两份报告均通过，前端运行错误均为 0。全部 28 张图已实际打开检查，截图保留驱动遮敏，没有重新绘制、裁切或拼接。

来源为 [Docker SQLite smoke run 38785000844](https://github.com/ForceMind/MyAPI/actions/runs/38785000844)，已回读运行及全部 6 项作业成功。准确源码为 `78c617b1e4469c08dbed0cb097f0e9b079a97a0e`，版本 `0.2.0-beta.16`。Root 和普通 owner 的实际前端标识均为 `rv.0.2.0-beta.16.78c617b1e4469c08dbed0cb097f0e9b079a97a0e.2k6e8r7p`；两个模式的构建卡原图亦显示相同值。

| 模式 | 原始安全 artifact | artifact ID | 报告 |
| --- | --- | --- | --- |
| legacy | `myapi-personal-legacy-78c617b1e4469c08dbed0cb097f0e9b079a97a0e` | `11997947403` | [原始报告](legacy-qualification.json) |
| authoritative | `myapi-personal-authoritative-78c617b1e4469c08dbed0cb097f0e9b079a97a0e` | `11997638372` | [原始报告](authoritative-qualification.json) |

仅下载这两个约 0.9 MB 的浏览器安全 artifact，没有下载镜像 bundle、构建日志、数据库或凭据文件。`manifest.json` 保存全部源报告及 28 张源图的 SHA256、尺寸、字节数、artifact 归属和入库文件哈希；未入库的 24 张原图仍属于对应 CI artifact，保留期为七天。

## 实际验证范围

每个 writer 使用全新临时 SQLite、隔离应用容器和 `http://127.0.0.1:19090` 回环固定上游。临时测试凭据只用于该隔离环境，未访问真实服务商、生产数据库、历史账务或生产凭据。

- 英文 1280 桌面以真实 Root 登录，明确选择已有身份的测试 Key，执行流式请求、保存日志并核对归属；报告记录 1 次成功请求、15 个内部用量单位。
- 中文 320 手机以真实 role-1 owner 登录，先验证现有策略拒绝，再由 Root 明确选择个人策略后执行成功请求；报告记录 1 次策略变更和 1 次成功请求。owner 修改本人或他人策略、跨 owner 读取的拒绝由驱动实际断言覆盖。
- 每个模式的真实浏览器共发送 5 次 `/pg/chat/completions` POST，其中 Root 2 次、owner 3 次；上游仅收到 2 次成功请求。三次拒绝分别为原有用户策略、有限普通 Key 额度和不合格 strict Token/USD 请求。拒绝检查要求上游计数不增加、日志及用量状态保持对应预期；strict 请求的预留和费用预留均为零。
- 浏览器刷新后，普通 owner 的有限 Key 保留剩余 985、已用 15、请求计数 1；刷新不重放请求。报告同时确认实际 writer 模式和前端元数据一致。
- 驱动截图前检查可见文字及输入值不含临时秘密，并要求 Key 明文读取次数为零；截图遮盖输入框、Key 明文样式文字及测试登录名。本次只归档白名单安全报告及这些原始 PNG。

## 代表原图

| 画面 | 文件 |
| --- | --- |
| legacy：Root 桌面成功调用及上报 Token | [1280 调用详情](legacy-fresh-persisted-log-en-1280.png) |
| legacy：owner 手机有限普通额度拒绝 | [320 拒绝结果](legacy-finite-quota-rejection-zh-320.png) |
| authoritative：owner 手机成功调用 | [320 调用详情](authoritative-existing-persisted-log-zh-320.png) |
| authoritative：Root 桌面不合格 strict 请求拒绝 | [1280 拒绝结果](authoritative-unqualified-strict-rejection-en-1280.png) |

## 证据限制

固定上游明确上报输入 10、输出 5、合计 15 个 Token；桌面实际详情显示 `Usage reported`。本次真实应用旅程没有模拟“回答完整但缺 usage”，不能用这些图证明 Token 未知分支。缺 usage 的个人详情应依据普通应用 CI 的专属回归与截图单独说明。

本次旅程没有配置原生窗口分额池，也没有触发原生窗口份额耗尽。普通额度拒绝和 strict 预算拒绝不能称为“原生窗口分额拒绝”。全池分解、近似分额的浏览器与三库证据由本批其他验证记录承担。

手机调用图停留在详情首屏，Token 卡位于屏外；不能把这张截图当作下方完整布局验收。`existing-policy-after` 原图保留上一条错误 toast，策略保存是否成功以报告和驱动回读为依据，不能从旧 toast 推断失败。两个 writer 的有限额度拒绝原图错误码不同，报告仅证明拒绝及状态不变，不据此宣称同一底层拒绝原因。

运行源码身份限定为上面准确 SHA；后续仅测试、环境示例或文档修改的候选不能改写这些原始报告的源码身份。该运行不包含发布、部署或合并 main。
