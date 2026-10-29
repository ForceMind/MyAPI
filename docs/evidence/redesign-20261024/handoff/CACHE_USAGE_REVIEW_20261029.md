# Codex 转换路径缓存用量只读审查（2026-10-29）

本文给接手的服务器会话读。证据标记：**[读码]** 只读源码得出，未运行；**[口述]** 维护者口头给出，未独立核实；**[猜测]** 无证据的推断；**[CI]** 来自 GitHub Actions 的读取结果。读码行号对应生产源码 `cd35f867d43b0a42d61500221bb9f99eb457e36a`，该提交到 dev 的 `d0d42cf4` 之间只改了文档。

## 1. 结论

- **没有找到请求 A 缓存为 0 的根因。** 无法证明 A 的上游返回的是缺失、null 还是明确的 0。
- **已确认一个证据缺陷：** Chat→Responses→Chat 转换路径不记录上游 `cached_tokens` 是缺失、null 还是 0，因此数据库 `cache_read_tokens` 对这三种情况都是 null。A 的 null 与之相符，不能解读为"上游明确返回 0"。
- **没有发现把上游正缓存数改成 0 的代码路径。** 流式多个终止事件、缓存字段复制、DTO 空值语义四处审查均无此类缺陷。
- **已排除的解释：** 同渠道多密钥（[口述] 4 个渠道共用同一个密钥，同一账号）；渠道之间拆分账号缓存（同上）。

## 2. 事实（均为 [口述]，来自维护者的任务描述）

- 2026-10-29（北京时间），同一 `channel_id`、同模型路由、同端点、同转换链：
  - A 18:17:52，输入/输出/缓存 = 649178/410/0，数据库 `cache_read_tokens=null`。
  - B 18:18:24，输入/输出/缓存 = 655733/130/10496，数据库证据 10496。
- system/developer 和 54 个工具完全一致；A 的 2290 条 messages 是 B 的完整前缀，B 在 `messages[2290]` 末尾多 7 条。
- 没有已保存的 Session/Thread 标识；诊断当时关闭，两条没有原始上游用量，也没有实际出站正文。
- 4 个渠道都是 Codex 类型，共用同一个密钥，都在维护者自己的服务器里。
- 这两条请求的数值与"B 只读到约 1.6% 的缓存"属于**数据观察**：B 的缓存 10496 只占输入 655733 的约 1.6%，看起来只是静态开头。[猜测] 它可能是 system 加工具那部分。

## 3. 已确认的缺陷 D1：转换路径丢失缓存字段存在性 [读码]

- `relay/channel/openai/chat_via_responses.go`（`cd35f867`，全文 378 行）没有调用 `markResponsesUsageEvidence`、`captureResponsesTextEvidence`。
- 原生 Responses 路径会调用：`relay/channel/openai/relay_responses.go:44`、`:122`、`:146`；定义在 `relay/channel/openai/usage.go:45`、`:87`。
- 转换路径的 `BillingUsage` 在 `relaykit/relayconvert/internal/oai_responses/to_oai_chat_resp.go:138-141` 构造，不含 `ResponsesTextEvidence`。
- `service/log_token_evidence.go:67-72` 只在兼容计数大于 0 时写 `cache_read_tokens`；`:73-87` 的零值证据分支要求 `ResponsesTextEvidence` 或 `ChatTextEvidence`，转换路径两者都没有。
- 结果：转换路径下，缺失、null、0 的数据库值都是 null；正数保留（B 的 10496 属此类）。原生路径下，明确 0 记 0，缺失和 null 记 null。

## 4. 数据结构层面的折叠 [读码]

- `relaykit/dto/openai_response.go:236` 的 `InputTokensDetails` 是指针，只表示对象是否存在；`:258` 的 `CachedTokens` 是 `int`，解码后缺失、null、0 完全相同。
- `relaykit/dto/billing_usage.go:131` 用 `InputTokensDetails != nil` 判断是否有用量，因此只保留对象存在与否。
- 真正保留字段存在性的只有原生路径的 `ResponsesTextEvidence.CacheRead *int`（`billing_usage.go:16-21`）。
- **不要把数据库兼容计数 0 当成上游明确 0。** 日志里的 `cached=0` 是兼容计数。

## 5. 审查后没有发现缺陷的点 [读码]

- 多个终止事件：`to_oai_chat_stream_resp.go:98-106` 对 `response.completed/done/incomplete` 都调用 `applyResponseMetadata`，`:134-136` 只在 `response.Usage != nil` 时整体替换；后一个带用量的终止事件覆盖前一个，null 不覆盖。原生路径（`relay_responses.go:117-148`）规则相同。这是合法的"终态为准"，**不要改成取最大缓存值**，那会覆盖合法终态零。
- `finalize` 由 `finalized` 标志只执行一次（`:435-438`），但状态里的用量仍会被后续终止事件更新；`chat_via_responses.go:350` 读最终状态，`:368` 输出最终用量块，客户端、计费和日志是同一份。
- `UsageFromResponsesUsage`（`to_oai_chat_resp.go:156-163`）在 `InputTokensDetails` 非 nil 时正确复制 `cached_tokens`。
- `response.created` 若带非 nil 用量只会写入临时值，之后被终止事件覆盖。

## 6. 未证实的假设 [猜测]

- H1：`UsageFromResponsesUsage` 只读 `input_tokens_details`，忽略 `src.PromptTokensDetails` 和 `PromptCacheHitTokens`。若某上游在 Responses 形状里把缓存数放在 `prompt_tokens_details`，会被丢掉。Codex 官方形状用 `input_tokens_details`，无证据显示它这样做。
- H2：上游对超长请求（A、B 约 65 万 token）只缓存静态开头，或缓存写入失败。
- H3：Chat 请求没有稳定的缓存键。若 Harness 的 Chat 请求体不带 `prompt_cache_key`，也没有会话头，上游缓存没有粘性。交接文档第 6 节写明默认不生成缓存键。
- H4：实际出站正文和入站日志看到的内容不同，导致前缀哈希不同。出站正文没有记录。

## 7. 分流情况 [读码]

- 默认亲和规则 `codex cli trace`（`setting/operation_setting/channel_affinity_setting.go`）要求路径匹配 `/v1/responses`，从请求体取 `prompt_cache_key`；匹配用客户端请求路径（`service/channel_affinity.go:673-684`）。Chat 客户端路径是 `/v1/chat/completions`，**不命中**，因此没有亲和。
- 无亲和时，在最高优先级层按权重做一次性随机抽取（`model/channel_routing_policy.go:159-217`），不记住上次渠道。
- 因 4 个渠道共用同一个账号 [口述]，这不会拆分账号缓存，所以不是本次 A=0 的解释。如果渠道的 base URL、代理、请求头不同，是否影响上游缓存，**未核对**。

## 8. 代码改动状态

- 测试提交 `911710bd`（只新增 `relay/channel/openai/chat_via_responses_cache_evidence_test.go`）已推送到 origin/dev。[CI] 运行 [40872167740](https://github.com/ForceMind/MyAPI/actions/runs/40872167740) 结论 failure，作业 `Backend vet, build, and test`（job 123441713081）失败，其余已完成作业成功，浏览器作业跳过。亲自读取该作业日志：16 行 `--- FAIL`，全部属于两个新增测试族——`TestResponsesToChatHandlersPreserveCacheFieldPresence` 的 12 个子用例（json、buffered_sse、stream 各 4 种缓存状态，"无 details 对象"用例通过）失败于 `chat_via_responses_cache_evidence_test.go:80`（证据对象为 nil），`TestResponsesToChatStreamCacheEvidenceFollowsTerminalUsage` 的 2 个子用例失败于 `:129`；唯一失败的包是 `relay/channel/openai`。`make test` 在根模块失败后不继续，relaykit 单测在该次运行中未执行。这是修复前的红灯基线。
- 修复提交 `0d9b794a`（只改 `relay/channel/openai/chat_via_responses.go`）随本文档一起推送；推送后的 CI 结果见仓库 Actions，写本文时尚未读取。
- 修复内容：在 3 个转换处理器里调用原生路径已有的 `captureResponsesTextEvidence`；只在用量来自上游时记录；在客户端响应体生成之后才记录，避免共享的用量对象把内部证据发给客户端；不改 token 数、结算、`Incomplete`、`Estimated`。
- 计费影响 [读码]：`ResponsesTextEvidence` 的消费方是 `service/fee_budget_price.go:130` 和 `service/log_token_evidence.go:77`。聊天预算要求来源为 `oai_chat` 且带 `ChatTextEvidence`（`service/token_budget_chat.go:84`），转换路径本来就不满足；响应预算只用于 Responses 绑定的预留行，不经过转换处理器。
- 本机没有构建、没有运行测试。验证全部依赖 GitHub Actions。

## 9. 范围外发现，未修复 [读码]

- 转换路径的非流式响应体和流式最终用量块会带出内部 `billing_usage` 对象；原生聊天路径在 `relay/channel/openai/relay-openai.go:304`、`:493` 清除了它。
- `UsageFromResponsesUsage` 会克隆上游自带的 `billing_usage`（`to_oai_chat_resp.go:138`），上游可伪造内部计费信封。
- 转换路径遇到 `response.failed`、`response.error` 时直接按错误返回（`to_oai_chat_stream_resp.go:107`、`chat_via_responses.go` 的流式回调），事件中的用量被丢弃；原生路径会保留。

## 10. 仍未知

- A 的上游原始用量到底是缺失、null 还是 0。
- A、B 实际出站正文是否一致，是否带 `prompt_cache_key`，是否带会话头。
- 同账号下上游缓存对超长请求的行为。
- 渠道的 base URL、代理、请求头是否有差异，以及是否影响上游缓存。

## 11. 建议接手会话核对和执行

1. 在 A、B 日志的管理员信息里核对：`use_channel`（前后几条请求落在哪些渠道）、`multi_key_index`、`channel_affinity`（没有该字段说明亲和未生效）。
2. 等 CI 40872167740 结束后，读后端日志，确认失败只来自新增的 `TestResponsesToChat*` 测试；再读修复提交的完整 CI。
3. 确认是否开诊断窗口，由维护者另行授权。已部署的诊断能记录出站正文和原始上游用量；窗口开关、时长由部署会话按授权处理。取证目标：出站正文是否带 `prompt_cache_key`、会话头，原始用量里 `cached_tokens` 是缺失、null、0 还是正数。
4. 不要主动改亲和规则或实例配置；不要主动发模型请求；不要用取最大值的方式合并缓存数。
