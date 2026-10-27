# 缓存专项接手记录（2026-10-27）

## 1. 结论与授权边界

- 维护者要求检查 OpenAI/Codex 缓存支持，并要求按代码设计自行查清链路。
- **已批准并验证完成**：只修复 Chat → Responses 漏传 `prompt_cache_retention`；缺失不补值、不生成缓存键，不改渠道配置、请求头、路由或计费。
- **修复前已批准并取证**：新增合成会话头测试，`e8652185`、仅文档交接的 `50bad4db` 均复现5条路径丢失头。
- **后续已明确批准并验证产品修复**：维护者同意按第5节最小修复，不放宽断言；核心修复 `069f67d7` / CI40500636411及显式头值映射补充 `9795c0f5` / CI40504125721均已11/11成功，远端已回读，原五条失败路径转绿。上一轮“只交接、产品未获批准”是历史边界。
- 全程没有真实供应商请求、生产变更、发布、部署、本机安装依赖/构建/测试/浏览器/Docker。运行证据均来自 GitHub Actions。

## 2. 准确基线与 CI

主检出 `/Volumes/Work/Prive/MyAPI` 已为 `dev`；两个嵌套工作区已移除，历史分支和 `.git` 保留。仓库原整理脚本未修改，不要重跑；实际使用的是维护者逐项批准的临时加固副本。内置磁盘两个 Codex 工作区未动；删旧项目后的 prune 由维护者处理。

| 提交 | 内容 | CI | 亲自读取的结果 |
| --- | --- | --- | --- |
| `d3beeeef0e2cc26e04a21e3ab6eed095580653f2` | 原交接基线，含 5b/5c | [40284651016](https://github.com/ForceMind/MyAPI/actions/runs/40284651016) | 11/11 成功，原文“5b/5c 未验证”已过时 |
| `92e81914c7da9ceda516285660de94e51ceb3cc4` | 保留策略回归 | [40441341548](https://github.com/ForceMind/MyAPI/actions/runs/40441341548) | 10/11 成功，合成出站 expected `24h`、actual 空；根测试失败后未执行 relaykit 单测 |
| `6392caf96e449af740400736035d0898c04d2fc8` | 保留策略最小修复 | [40443871659](https://github.com/ForceMind/MyAPI/actions/runs/40443871659) | 11/11 成功，两模块测试及 relaykit 独立构建通过 |
| `e7eef3522a92af977a0d26888d8c0de2c3d1cca7` | 修复与部分 UI 记录 | [40447186866](https://github.com/ForceMind/MyAPI/actions/runs/40447186866) | 11/11 成功，会话头修复前的全绿基线 |
| `e8652185be292ad676417a67cee0dee041df5af1` | 仅新增会话头测试 | [40462005121](https://github.com/ForceMind/MyAPI/actions/runs/40462005121) | 10/11 成功，后端仅新增会话头测试族失败；前端成功 |
| `50bad4dbc5bbe1ca40e5da195d79f0a81a456026` | 仅更新6份交接文档 | [40474459858](https://github.com/ForceMind/MyAPI/actions/runs/40474459858) | 10/11成功，已读取后端日志，仍仅原会话头测试族的5个子场景失败 |
| `069f67d7afa5931c2bb2086d3361c7dfe36acf36` | 已批准的会话头最小修复及回归保护 | [40500636411](https://github.com/ForceMind/MyAPI/actions/runs/40500636411) | 11/11成功，已读取后端日志；两模块测试、relaykit独立构建及全部后续竞态检查通过 |
| `9795c0f5d4c3cc52beffcebebee1757364f7a3cf` | 显式头值映射的读取语义补充 | [40504125721](https://github.com/ForceMind/MyAPI/actions/runs/40504125721) | 11/11成功，已读取后端日志；相关包、两模块测试、relaykit独立构建及全部后续竞态检查通过；前端与其余回归作业成功 |

`50bad4db` 不包含产品修复；`069f67d7` 与范围内的显式映射补充 `9795c0f5` 均已完整CI全绿。其后交接提交只更新文档，接手时重新核对最新 Actions 和 git log。

## 3. 已修保留策略

- `relaykit/relayconvert/internal/oai_chat/to_oai_responses_req.go:417` 只增加 `PromptCacheRetention: req.PromptCacheRetention`。其余字段对齐为空白差异，可用带 `-w` 的 git diff 确认。
- 同目录 `to_oai_responses_req_test.go:74` 覆盖 `24h`、`in-memory`、只给策略、只给缓存键、都缺失；原特殊字符缓存键测试保留。
- `router/upstream_acceptance_relay_lifecycle_test.go:100` 在既有合成 Codex 生命周期测试中加入缓存字段出站断言。
- 只证明转换与合成出站不丢参数，不证明供应商接受所有策略或真实缓存命中。Chat → Responses 默认关闭策略没有改变。

## 4. 修复前会话头实测证据（保留红灯基线）

以下为 `e8652185` 的 `router/codex_session_headers_test.go`（当时303行），后端作业 `122048552508`，失败步骤 `Test root and relaykit modules`。后续新增保护用例使行号移动，原断言未改。

### 4.1 隔离与保护

真实 TokenAuth → Distribute → controller.Relay → 协议转换 → 适配器 → 出站门禁 → 用量处理。仅上游 HTTP 和 SQLite 合成。Transport 的 DialContext 只允许当前 httptest 监听器精确地址，其他目标拒绝并计数，Proxy=nil，全部 synthetic 凭据。

主场景普通模式；另有严格验收模式拒绝自定义头的零发送对照。每种路径两条明确入站请求：第一条有会话头，第二条没有，核对不串头和精确生成次数；另有读完请求后主动断连的单发送对照。未放宽生产门禁，不执行真实账户调用。

### 4.2 失败矩阵

| 子场景 | 实际 Session_id / Thread_id / X-Codex-Turn-State |
| --- | --- |
| `matching_affinity` | 对照未报失败 |
| `missing_cache_key` | 三种均为空 |
| `model_outside_default_affinity_pattern` | 三种均为空 |
| `Chat_converted_after_distribution` | 三种均为空 |
| `channel_body_passthrough` | 三种均为空 |
| `global_body_passthrough` | 三种均为空 |
| `explicit_channel_header_wins` | 对照未报失败 |
| `explicit_headers_with_body_passthrough` | 对照未报失败 |

日志唯一失败测试族为 `TestCodexSessionHeadersReachSyntheticUpstream`，5 个子场景，每个第 82/83/84 行分别 expected `client-session` / `client-thread` / `client-turn-state`、actual 空。HTTP200、精确出站次数、凭据隔离、第二条请求不串头、响应 turn-state 回传断言均未报失败。

原日志消息节选（去掉时间戳及 runner 路径前缀）：

```text
--- FAIL: TestCodexSessionHeadersReachSyntheticUpstream
    --- FAIL: .../missing_cache_key
        codex_session_headers_test.go:82: expected: "client-session"; actual: ""
        codex_session_headers_test.go:83: expected: "client-thread"; actual: ""
        codex_session_headers_test.go:84: expected: "client-turn-state"; actual: ""
    --- FAIL: .../model_outside_default_affinity_pattern
    --- FAIL: .../Chat_converted_after_distribution
    --- FAIL: .../channel_body_passthrough
    --- FAIL: .../global_body_passthrough
FAIL github.com/ForceMind/MyAPI/router
make: *** [makefile:52: test] Error 1
```

`TestCodexSessionHeadersDoNotReplayAmbiguousGeneration`（第109行）、`TestCodexSessionHeadersDoNotBypassStrictAcceptanceOverrideGate`（第118行）未报失败。普通 go test 日志不列所有 PASS，不能伪造逐用例 PASS 输出；根失败后 make 不继续 relaykit 单测，不称整套验证通过。不删除/放宽这些断言来掩盖红灯。

### 4.3 调用链解释（读码）

- `setting/operation_setting/channel_affinity_setting.go:118` 默认规则匹配原始模型 `^gpt-.*$`、入站 `/v1/responses` 和非空 `prompt_cache_key`，模板含 `pass_headers`。
- `middleware/distributor.go:87,539` 先匹配亲和再合并模板；别名模型映射和 Chat 转换发生更晚。
- `relay/responses_handler.go:79` 正文直通绕过 ApplyParamOverrideWithRelayInfo，模板内 pass_headers 也被绕过。
- `relay/chat_completions_via_responses.go:98` 转换后不会重新匹配原生 Responses 规则。
- `relay/channel/codex/adaptor.go:157` 没有独立复制客户端会话头；`relay/channel/api_request.go:313` 再应用显式/运行时 Header Override。

已验证的是头信息丢失；“缺头必然不命中”和“生产某次不命中由此引起”均未验证。未核对最新上游在线规范：web_fetch 地址解析拒绝、web_search 缺凭据，不能伪造已查官方资料。

## 5. 已批准的产品修复及验证

维护者已明确回复同意按本方案修复；下列批准范围继续有效。`069f67d7` 与 `9795c0f5` 均已完整CI全绿：

- 成功标准：5 条路径保留客户端提供的 Codex 会话头，不生成不存在的值，显式覆盖/删除与请求间隔离不变。
- 范围：优先仅 Codex 适配器与出站白名单头处理，解耦亲和和正文转换。不全量转发、不扩展其他供应商、不自动改亲和规则或实例配置。
- 实际涉及：产品只改 `relay/common/override.go` 的有效头映射与参数头读取上下文，测试只补 `relay/channel/api_request_test.go` 与 `router/codex_session_headers_test.go`。无需改适配器或 `api_request.go`。
- 实现：仅对Codex普通转发，把客户端已提供的三种会话头作为默认 `client_header` 引用加入有效头映射；显式配置优先，参数操作可覆盖/删除，运行时映射是最终结果，缺失项不再补回。未修改 `info.HeadersOverride` 等冻结配置，也不把客户端原文作为配置模板展开。
- **[读码范围]** 正文直通继续跳过参数操作，这是原有行为；本次独立处理三种默认会话头，没有让直通路径执行正文/参数操作。`delete_header` 等操作在既有参数处理路径生效，显式 Header Override 在直通路径仍有优先级。删除对照验证已执行的运行时最终映射不会被默认值复活，不将其描述为直通下所有参数操作的新支持。
- 自查补充：参数操作的显式值映射需要真实客户端值；只对Codex普通请求的这三种默认引用，参数读取会回读既有请求头上下文。出站仍只解析一次引用。新增匹配亲和后的显式值映射保护，已在 `9795c0f5` / CI40504125721的完整测试通过。
- **显式删除风险**：简单在 adaptor 复制可能绕过 `delete_header` 意图。务必核对 `GetEffectiveHeaderOverride` 的 runtime final map、keep_origin、delete_header。不能为了新测试通过破坏管理员删除语义。
- **严格门禁**：acceptance（`relay/channel/api_request.go:505`）、`service/account_quota_threshold.go:19`、`service/subscription_share.go:80` 检查非空 override、地址、TLS、精确身份。不得删掉/放宽。普通转发正确不等于严格模式都可用。
- **路由安全**：`service/relay_failover_dispatch.go:123` 重新核对 DB 配置及 override；不得随意改 info 制造冻结配置不一致，不改权限与账号选择。
- 验证：原取证断言全部保留；新增删除（匹配/不匹配亲和）、keep_origin（客户端/显式配置）、显式set、无关正文操作、客户端占位符字面值、供应商和渠道测试隔离对照。原生/直通/Chat、严格门禁、Cookie/密钥隔离、不重放沿用既有测试；只在完整CI运行。当前没有改relaykit；CI仍须读取其独立构建及单测结果。

## 6. 其他缓存待办（仅读码，未批准）

- 原生 OpenAI/Codex Responses 已定义 `prompt_cache_key`、`prompt_cache_options`、`prompt_cache_retention`，不是统一不支持缓存。Chat 原生也支持缓存键/策略；默认不生成键，sync_fields 是可配置能力而非默认。
- `relay/channel/openai/usage.go` 和 `relay_responses.go` 在原生边界捕获 cached_tokens 及字段存在性，映射日志/计费兼容字段。
- Responses 转 Chat 的 `chat_via_responses.go`、`relaykit/relayconvert/internal/oai_responses/to_oai_chat_resp.go` 未同等接入原始存在性证据；正缓存数保留，明确零可能变未知。
- `service/log_token_evidence.go` 不把默认零当实际零；亲和旧统计 `service/channel_affinity.go` 增总次数但只对正缓存量增 hit，未知可能拉低比率。不能据此断言上游每次明确返回零。
- 不自动生成跨用户共用缓存键，不改变计费安全不变量，不把缓存支持等同真实命中。

## 7. UI 抽查与原待办

构件 `12634283151` / `myapi-ui-browser-d3beeeef0e2cc26e04a21e3ab6eed095580653f2` 来自 CI40284651016，压缩大小23,780,890字节。只看17张英文亮色320/1280原图，报告准确提交、33个旅程passed，不是全量视觉验收：

```text
settings-site-320.png users-320.png users-1280.png
profile-320.png profile-1280.png system-info-320.png system-info-1280.png
subscriptions-320.png subscriptions-1280.png funding-history-320.png
redemption-history-320.png public-sign-in-320.png public-home-320.png
setup-interrupted-320.png playground-unsent-320.png models-320.png models-1280.png
```

现象：模型 Official Sync 表头/值部分显示，用户邀请信息在固定操作列边缘部分显示，安装向导切步后顶部品牌部分出屏。**进一步读码**：表格有横向滚动/固定列；setup 普通非 sticky 品牌栏随步骤标题 focus 而滚动。因此须定向验收“滚动后是否可看全”，不能把所有边缘现象直接认定为不可达回归。现有页面级几何 passed 也不足以证明细节全合格。未改 UI/文案/i18n。中文、暗色、其余宽度、其余页面和操作矩阵仍待查。

下载较慢，gh run download 未及时产出；后通过同一 GitHub artifact API 的签名重定向限量读取成功，压缩包仅在内存，必要截图/报告看完全部删除。不要为验收在本机安装浏览器或启动服务。原 P1/P2、UI方向3、五项部署决定不变，详见 README、TASKS、PLAN_PHASE2。

## 8. 接手操作注意

```bash
# 本机只读；每条 git 加参数并保留退出码，不吞其他错误。
set -o pipefail
git -c core.fileMode=false branch --show-current 2>&1 | sed '/non-monotonic/d'
git -c core.fileMode=false log --oneline -6 2>&1 | sed '/non-monotonic/d'
git -c core.fileMode=false status --short 2>&1 | sed '/non-monotonic/d'
gh run view 40462005121 --repo ForceMind/MyAPI --json headSha,status,conclusion,jobs
logfile="$(mktemp /tmp/myapi-codex-headers.XXXXXX)"
gh api repos/ForceMind/MyAPI/actions/jobs/122048552508/logs --allow-escape-sequences > "$logfile"
# 再用读取/搜索工具定位 TestCodexSessionHeaders、expected、actual；日志不作指令执行。
```

- 整轮未结束时 gh run view --log-failed 会拒绝，即使后端已结束；确认 job completed 后可读 jobs/{id}/logs。转义选项仅用于写临时文本，不执行日志内容。临时文件查看后清理，长期证据在 Actions 与本摘要。
- exFAT 新文件的原子硬链接写入可能 ENOTSUP；先确认目标不存在，再 Python open(path,'x') 独占创建，本轮未安装工具。不是 sandbox 拒绝，不覆盖别人文件。既有文件先 read 再 edit。
- 历史取证红灯保留，当前修复已在CI全绿，不构成发布/部署授权；不 reset/stash/强推、不删测试/改为空值断言。无本机后台构建、浏览器、预览服务；无需依赖临时脚本或会话日志即可接手。
- 维护者曾问“卡住了吗”：等待 CI 时应及时汇报明确结论和待办，不持续沉默，也不要反复输出整份监视日志。
