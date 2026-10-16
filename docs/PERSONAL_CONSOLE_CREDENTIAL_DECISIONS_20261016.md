# 凭据轮换与跨账号迁移：能力核对与必要决策

> 后续批准：维护者于 2026-10-16 确认原 Token ID、单值轮换并接受短暂中断；跨上游账号保留 Key 身份、逐个迁移。已进入 [下一批实施](PERSONAL_CONSOLE_CREDENTIAL_EXECUTION_20261016.md)。以下能力表保留 beta.17 时点，不代表 beta.18 已验收；轮换兼容未证明前保持提交关闭。

2026-10-16，只读核对当前功能分支和代码。本记录是能力审计与后续决策清单，不是轮换/迁移实施方案或生产操作授权。本批只实施窗口重选、安全设置与模型范围；历史账务、生产凭据/数据库/备份冻结，不合并、发布或部署。

## 现有能力

| 对象 | 已有能力与代码证据 | 当前没有的完整工作流 |
| --- | --- | --- |
| API Key（`model.Token`） | `controller/token.go` 的 AddToken 创建新随机 secret；GetTokenKey/GetTokenKeysBatch 在明确操作后读取现有值；UpdateToken/Token.Update 不写 `key`。路由见 `router/api-router.go` 的 token group。暂停/恢复、范围、预算与近似分额分别维护。 | 无原 Token ID 的原地轮换端点，无旧/新 secret 双值宽限期，无自动复制预算/分额/访问策略并验证撤销的迁移流程。创建一个新 Key 不等于轮换旧 Key。 |
| 控制台个人 Access Token（`User.access_token`） | `controller/user.go:GenerateAccessToken` 经 GET `/api/user/token` 生成并写入新的单值；`model/user.go:UpdateUserAccessToken` 维护该控制台认证值。这个 GET 有写入副作用。现存安全入口须准确命名、显式确认、一次性显示和关闭清理；打开面板不得调用。 | 没有宽限期、第二个并存值或完整替换恢复协议。不是 API Key，也不是上游 OAuth 凭据。本批不对真实环境执行生成。 |
| 上游 Codex OAuth 凭据 | `router/channel-router.go` 注册 OAuth start/complete、已有渠道 refresh、local-auth/import；`model/channel_credential.go:UpdateChannelCredentialIfUnchanged` 按 expected key 做数据库 CAS，并发布缓存代。 | 这些维护入口不构成账号迁移。refresh 是已有授权的刷新；重新授权/导入可以改变凭据，但不能据此自动迁移原生窗口池、Key 分配或历史。 |
| 本人登录密码、Passkey、2FA、备份码、登录会话 | 已有独立安全 API；密码/部分安全操作成功可返回合法认证版本更新。Root 不允许自助删除账号。 | 它们不是 API Key/上游账号轮换策略；本批仅完善现有安全维护 UI 和会话边界，测试只使用合成/隔离环境。 |

上述判断来自实际函数/路由，不从按钮名推断后端能力；没有读取真实 secret 或导出数据库。

## 窗口重选不承担跨账号迁移

`model/subscription_share.go:ConfigureSubscriptionShare` 要求池与候选的 `SubjectRef` 相同，重选还要求原生 `WindowSource`（主/次槽位）相同。连接的 channel ID/名字只是样本连接标识，不能代替服务商确认的账号主体。不同 descriptor 的重选开启新跟踪代，保留旧代历史和各 Key 比例，未决请求转为 uncertain，等待可信新基线；同 descriptor 的恢复保留已有估计与 generation。

不同账号的凭据替换不能把旧池历史、分配或已用量当成新账号数据。现有 Key 对旧主体的绑定不应以普通未分配调用绕过限制；关闭旧池仍是 Root 显式、独立的操作。不能从同名 channel 或刷新成功推断主体连续。

## 后续需要决定的最少事项

1. **轮换对象与身份**：API Key、控制台 Access Token 还是上游凭据；API Key 是否保留 Token ID 原地换 secret，或创建新 Token 并由维护者明确选择要复制的模型/IP/profile/预算/分额。现有创建接口不会自动完成全部关联迁移。
2. **旧值失效与恢复**：立即撤销还是允许明确宽限；新值如何验证、旧值何时停止接收、在途请求如何处理。未知提交如何回读、什么条件可回滚、如何避免失败后同时丢掉新旧值。
3. **迁移集合与窗口状态**：哪些 Key/哪些原生窗口迁移，目标池比例是否足够，旧池何时关闭；旧未决请求如何终结、目标新基线何时可信。旧历史始终保留在旧主体，不复制成目标用量或凭空清零。
4. **并存与操作控制**：是否允许旧/新账号并存过渡；Root 实时会话与二次验证、CAS、幂等标识和审计/恢复证据的要求。建议的起点是目标账号建立全新池/基线，逐 Key 显式分配并验证，最后单独决定关闭旧池；这只是待确认建议，本批未实施。

这些决定不阻止当前已授权的高级维护开发；本批交付核对结果，不擅自选择撤销期限、迁移范围或生产执行时机。
