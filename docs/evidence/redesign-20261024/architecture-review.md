# MyAPI 架构评估报告（任务 1：第一性原理评估、精简候选、版本差距）

- 评估对象：`/Volumes/Work/Prive/MyAPI/.claude/worktrees/dev-work`，分支 `dev`。工作区 HEAD 为 `9e84f3de`（仅在 `46a39080` 之上新增 `docs/evidence/redesign-20261024/TASKS.md`，业务代码与 `46a39080` 相同）。
- 角色：架构评估，只读。未修改、删除或新建仓库内任何文件；评估结束时 `git -c core.fileMode=false status --porcelain --untracked-files=all` 输出 0 行（亲自运行）。
- 证据标记：**[运行]** = 本次亲自运行命令得到（命令见第 7 节）；**[代码]** = 从代码看（读源码或检索得出，未运行行为）；**[猜测]** = 推断，需要核实。
- 范围：项目评估、结构精简、Full/Lite/Desktop 版本差距。UI 设计不在本报告范围。

---

## 0. 结论摘要

1. 核心价值链（鉴权 → 选渠道 → 转发 → 计量/账务 → 诊断）只占后端生产代码的一部分；最大的复杂度来源是账务持久化子系统（model 中约 1.7 万行生产代码 + 1.9 万行测试）和异步任务恢复（约 2.0 万行生产代码）。两者都在价值链上、触及计费不变量，本轮不建议精简，只建议补所有权说明。**[运行]**（行数统计）**[代码]**（归属判断）
2. 确认无引用、可直接删除（A 类）约 1.18 万行：Go 根模块 194 个不可达函数约 1,951 行；前端 74 个未引用文件约 8,387 行；10 个未引用 UI 原语约 1,448 行；若干无引用依赖。**[运行]**（deadcode、knip 与脚本）
3. 版本门禁目前由四个互不相通的开关拼成：后端 `MYAPI_EDITION=full|lan`、前端构建期 `VITE_SELF_USE_MINIMAL`、运行期选项 `SelfUseModeEnabled`、用户资金模式 `user_funding_mode`（前端以 `funding=disabled` 切换"个人工作台"）。后端不识别 `lite`，未知值回落为 Full；`/api/status` 不暴露版本，前端只能靠构建参数区分版本。**[代码]**
4. Legacy LAN 门禁（`middleware/edition.go`）已封锁支付、商业订阅、公开定价/排行、外部 OAuth、签到、邀请返利；但未封锁兑换码管理 `/api/redemption`、io.net 部署 `/api/deployments`、Midjourney `/api/mj` 与 `/mj` 转发。同时它把 `/api/subscription/self` 一并封掉，导致已有订阅的用户在 Lite 里看不到自己的订阅余额，而订阅重置任务仍在运行。**[代码]**
5. 前端存在一批按"个人模式"并行实现的页面：总览（旧版总览与个人工作台）、Key（表格与个人列表）、调用记录（通用日志与个人调用记录）、上游（channels、connections、personal-upstream 三处）。它们应在 UI 重做时按版本能力收敛为一套，不应先单独重构。**[代码]**
6. 精简候选统计：A 类 5 条，约 1.18 万行；B 类 10 条，重构涉及约 3 万行，净减少估计 3–8 千行 **[猜测]**；C 类 12 条，按版本隐藏、不删数据；D 类 14 条，需要维护者逐项确认，若 D1–D11 全部批准下线，各项加总为后端生产代码约 2.4 万行、前端约 3 万行，扣除共享代码后实际可删量估计为后端 1.8–2.4 万行、前端 2.5–3 万行 **[猜测]**（加总依据第 5.4 节的文件行数 **[运行]**）。

---

## 1. 核心价值链（第一性原理）

MyAPI 存在的理由是：用户只持有 MyAPI 下发的 Key，就能可靠、受约束、可计量、可追查地调用多个上游。逐条推导，以下五环缺一不可，其余能力都是外围：

| 环节 | 必须做对的事 | 失败后果 | 主要实现位置 **[代码]** |
| --- | --- | --- | --- |
| 1. 鉴权 | 识别下游 Key 与会话用户，执行 Key 状态、模型范围、访问方案、额度上限、限流 | 越权调用、额度失控 | `middleware/auth.go`（`TokenAuth`、`UserAuth`、`AdminAuth`）、`middleware/access_policy.go`、`middleware/rate-limit.go`、`middleware/model-rate-limit.go`、`middleware/token_budget.go`、`middleware/user_usage_policy.go`、`service/accesspolicy/`、`service/authz/`、`model/token*.go`、`model/access_profile.go` |
| 2. 路由/选渠道 | 按模型、分组、优先级、亲和、配额窗口选可用上游，并在失败时有界切换 | 请求打到不可用或无权渠道 | `middleware/distributor.go`（`Distribute`）、`service/channel_select.go`、`service/relay_failover*.go`、`service/channel_affinity.go`、`service/codex_quota_routing.go`、`model/channel_cache.go`、`model/ability.go`、`model/channel_routing*.go` |
| 3. 转发 | 协议转换、流式、上游错误映射，保持各供应商语义 | 响应错误、协议破坏 | `router/relay-router.go`、`controller/relay.go`（`relayHandler`）、`relay/*_handler.go`、`relay/channel/*`（53 个适配目录）、`relay/common`、`relay/helper`、`relaykit/`（协议 DTO 与转换，独立模块） |
| 4. 计量/账务 | 预扣、结算、退款、饱和审计，不出现负扣费或意外返还 | 资金/额度错误，不可逆 | `service/quota.go`、`service/billing*.go`（`PreConsumeBilling`、`SettleBilling`）、`service/text_quota.go`、`service/task_billing*.go`、`service/log_info_generate.go`（`attachQuotaSaturation`）、`common/quota_math.go`、`pkg/billingexpr/`、`types/price_data.go`、`model/quota_*`、`model/account_quota_*`、`model/user_quota_mutation.go`、`model/task_recovery.go` |
| 5. 诊断 | 按 Key/模型/时间/请求 ID 找到请求、上游尝试、脱敏内容、任务结果与账务状态 | 无法排错与对账 | `model/log.go`、`controller/log.go`、`middleware/full_content_logger.go`、`controller/full_content_log.go`、`model/usage_review*.go`、`controller/codex_usage.go`、`model/channel_quota_snapshot.go`、`controller/channel-billing.go`（额度历史）、`controller/channel-test.go`、`service/system_task.go` |

横切支撑：`common/`（JSON 包装、Redis、限流、环境）、`setting/`（比率、模型、运行配置）、`i18n/`、`logger/`、`constant/`、`types/`、`model/main.go`（三库迁移）。

价值链之外的外围能力（不影响"可靠转发 + 可计量 + 可诊断"）：支付/充值/钱包、商业订阅计划、兑换码、签到、邀请返利、公开定价页/排行榜/性能指标、外部 OAuth 登录、Midjourney 代理、io.net 模型部署、站点内容管理（公告/FAQ/首页）、Playground。**[代码]**

---

## 2. 模块归属表

### 2.1 后端顶层包

统计口径：行数来自 `git ls-files '*.go'` 排除 `._*` 后 `wc -l`，分生产/测试 **[运行]**；"被引用文件数"= 其他顶层包中 import 该包任一子包的生产文件数，由脚本解析 import 块得到（脚本 `scratchpad/goimp.py`，第 7 节）**[运行]**；价值链与版本归属 **[代码]**。

| 包 | 职责 | 生产/测试行数 | 被引用文件数（来源包） | 在价值链上 | 归属版本 |
| --- | --- | --- | --- | --- | --- |
| `main.go` | 启动、嵌入 `web/dist`、后台任务启动 | 398 / 0 | 入口 | 是 | 全部 |
| `common` | JSON 包装、quota 换算、Redis、限流、环境、系统日志 | 7,343 / 3,361 | 440（13 个包） | 横切 | 全部 |
| `constant` | 渠道类型、API 类型、上下文键等线上常量 | 521 / 0 | 140 | 横切 | 全部 |
| `controller` | HTTP 处理（含部分本应在 service 的业务逻辑，见 3.4） | 31,765 / 19,071 | 13（router、main） | 是 | 全部；其中支付、订阅、兑换码、签到、部署、MJ、Telegram/WeChat/自定义 OAuth 文件为 Full only |
| `dto` | MJ、Suno、视频任务 DTO | 325 / 0 | 24 | 任务转发 | 全部（MJ 部分 Full only） |
| `i18n` | 后端中英文 | 656 / 0 | 65 | 横切 | 全部 |
| `logger` | 请求关联日志 `LogInfo/Warn/Error` | 216 / 246 | 108 | 诊断 | 全部 |
| `middleware` | 鉴权、分发、限流、内容日志、版本门禁 | 4,498 / 4,005 | 33 | 是 | 全部 |
| `model` | 数据层与大部分账务状态机（98 张主库表 + 日志库） | 59,568 / 46,560 | 199 | 是 | 全部（表结构所有版本一致，见 4.3） |
| `oauth` | GitHub/Discord/LinuxDO/OIDC/通用 OAuth 提供方 | 1,662 / 269 | 4（controller、main） | 否 | Full only（LAN 已封 `/api/oauth`） |
| `pkg/billingexpr` | 计费表达式 | 584 / 1,117 | — | 是（计费） | 全部 |
| `pkg/cachex` | 缓存封装 | 377 / 97 | — | 横切 | 全部 |
| `pkg/ionet` | io.net 模型部署客户端 | 1,606 / 726 | — | 否 | Full only |
| `pkg/perf_metrics` | 模型性能指标聚合（公开排行/性能页） | 679 / 0 | — | 否 | Full only |
| `pkg/personalusage` | 个人用量策略 | 49 / 110 | — | 是 | 全部 |
| `relay` | 转发处理器、适配器、helper | 37,163 / 20,925 | 47 | 是 | 全部 |
| `relaykit`（独立模块） | 协议 DTO、请求/响应转换 | 15,902 / 5,838 | 207 | 是 | 全部 |
| `router` | 路由注册 | 1,096 / 5,127 | 1（main） | 是 | 全部 |
| `service` | 业务逻辑 | 31,289 / 29,602 | 130 | 是 | 全部；`epay.go`、`waffo_pancake*.go`、`midjourney.go`、`rankings.go`、`subscription_reset_task.go` 为 Full 功能（订阅重置任务在 Lite 也须保留，见 4.3） |
| `setting` | 比率、模型、运行、性能配置 | 9,191 / 6,052 | 115 | 是 | 全部；`payment_*.go`、`checkin_setting.go`、`perf_metrics_setting` 为 Full 功能 |
| `types` | PriceData、集合等 | 340 / 176 | 18 | 是（计费） | 全部 |

合计：生产约 20.5 万行，测试约 14.3 万行 **[运行]**，与给定的 34.4 万行总数一致。

`model` 按领域分组的生产+测试行数（文件名前缀聚合，**[运行]**）：账务（account_quota/quota_*/writer 等）约 3.8 万；异步任务恢复（task_recovery 等）约 1.0 万；渠道额度快照约 5.4k；订阅分额 subscription_share 约 4.2k；Key 凭据轮换/迁移约 4.6k；日志约 3.8k。

### 2.2 前端 feature（`web/src/features`）

统计口径：`.ts/.tsx` 行数，`__tests__` 与 `*.test.*` 计入测试，不含 locale JSON；"外部引用文件数"= feature 外 import `@/features/<name>` 的非测试文件数（其中 routes 下文件数单列）。脚本 `scratchpad/feat.py` **[运行]**。版本归属 **[代码]**（依据 `middleware/edition.go` 封锁清单、`lib/legacy-route.ts` 与 `system-settings.config.ts` 的 `selfUseHidden`）。

| feature | 职责 | 生产/测试 | 外部引用（routes/其他） | 价值链 | 归属版本 |
| --- | --- | --- | --- | --- | --- |
| about | 关于页 | 233 / 0 | 1 / 0 | 否 | 全部 |
| access-policy | Key 访问方案 | 1,132 / 1,171 | 0 / 2 | 鉴权 | 全部 |
| auth | 登录、注册、2FA、Passkey、OAuth 回调 | 5,421 / 709 | 9 / 15 | 鉴权 | 全部；注册与外部 OAuth 为 Full |
| channels | 渠道管理 | 33,640 / 5,645 | 1 / 24 | 路由 | 全部 |
| chat | 聊天预设外跳 | 393 / 145 | 2 / 2 | 否 | Full（自用构建已移除 `/chat`） |
| connections | 账号连接、窗口、分额池 | 890 / 1,001 | 3 / 2 | 诊断 | 全部 |
| dashboard | 总览、模型统计、流量 | 13,107 / 3,617 | 2 / 4 | 诊断 | 全部 |
| errors | 错误页 | 372 / 105 | 7 / 0 | — | 全部 |
| full-content-logs | 内容日志 | 1,958 / 679 | 1 / 0 | 诊断 | 全部 |
| home | 公共首页 | 2,389 / 409 | 1 / 0 | 否 | 全部（Lite/Desktop 可直达登录） |
| keys | API Key | 13,364 / 10,003 | 2 / 8 | 鉴权 | 全部 |
| legal | 用户协议/隐私 | 290 / 0 | 2 / 0 | 否 | Full（开放注册时需要） |
| models | 模型元数据；含 io.net 部署约 2.5k 行 | 11,089 / 837 | 2 / 2 | 路由 | 全部；部署部分 Full only |
| performance-metrics | 公开性能指标组件 | 202 / 0 | 0 / 8 | 否 | Full only |
| personal-upstream | 上游账号窗口 | 1,487 / 1,259 | 1 / 3 | 诊断 | 全部 |
| playground | 调试对话 | 7,152 / 4,656 | 1 / 8 | 否 | 全部 |
| pricing | 公开定价页 | 9,781 / 370 | 2 / 7 | 否 | Full only（LAN 已封 `/api/pricing`） |
| profile | 资料与安全 | 7,776 / 2,552 | 1 / 0 | 鉴权 | 全部 |
| prompt-learning | S5-P 提示词学习 | 473 / 542 | 1 / 0 | 否 | 全部（能力矩阵要求） |
| rankings | 公开排行 | 1,502 / 0 | 1 / 0 | 否 | Full only |
| redemption-codes | 兑换码管理 | 2,107 / 314 | 1 / 0 | 否 | Full only |
| setup | 安装向导 | 1,341 / 669 | 2 / 0 | 是（初始化） | 全部 |
| subscriptions | 商业订阅计划 | 3,246 / 0 | 1 / 2 | 否 | Full only |
| system-info | 实例与后台任务 | 1,343 / 0 | 1 / 0 | 诊断 | 全部 |
| system-settings | 系统设置；其中 models 1.4 万、integrations 7.1k（支付约 4.5k） | 41,431 / 2,270 | 15 / 7 | 部分 | 全部；支付、签到、部署、内容、SMTP、Worker、更新检查分区为 Full |
| usage-logs | 调用/任务/绘图日志 | 11,258 / 3,633 | 2 / 5 | 诊断 | 全部；绘图（MJ）为 Full |
| users | 用户管理 | 3,961 / 961 | 1 / 4 | 鉴权 | Full 完整；Lite 保留（LAN 下给同事发 Key 需要） |
| wallet | 钱包/充值 | 5,107 / 351 | 1 / 2 | 否 | Full only |

非 feature 目录（生产行数，**[运行]**）：`components/ui` 9,440、`components/ai-elements` 8,256、`components` 其余 6,885、`lib` 5,222、`components/data-table` 5,207、`components/layout` 4,307、`routes` 2,876、`hooks` 2,406、`assets` 2,251。

### 2.3 `relay/channel` 适配器

分组依据（三项指标均 **[运行]**，分组判断 **[代码]**/**[猜测]** 如注）：
- 注册：`relay/relay_adaptor.go` 的 `GetAdaptor`/`GetTaskAdaptor` 是否注册；前端 `web/src/features/channels/constants.ts` 是否允许新建（已注释的类型 6、9、10、11、12、13、21 不可新建）。
- 测试：目录内是否有 `_test.go`。
- 近期提交：仓库历史从 2026-08-28 导入开始，绝大多数适配器只有 3 次提交（导入、品牌迁移、JSON 包装迁移），所以以"提交数 > 3"作为导入后被实际维护的信号。
- 主计划第 7 节点名的供应商视为主流。

**核心主流**（生产行 / 是否有测试 / 提交数 / 被引用文件数）：
| 适配器 | 行 | 测试 | 提交 | 引用 | 依据 |
| --- | --- | --- | --- | --- | --- |
| openai（另服务 Azure、OpenRouter、Xinference、Custom 等类型） | 3,895 | 有 | 20 | 25 | 共享基座 |
| claude | 594 | 有 | 8 | 13 | 主计划 P3 |
| gemini（含 Antigravity 客户端） | 1,802 | 有 | 8 | 5 | 主计划 P3 |
| codex | 245 | 有 | 3 | 2 | 主计划 P3，专用 OAuth 与用量 |
| aws（Bedrock） | 865 | 有 | 4 | 1 | AGENTS.md 点名 |
| vertex | 758 | 有 | 5 | 2 | 维护信号 |
| deepseek / moonshot / siliconflow / openrouter | 222 / 145 / 258 / 22 | 无/有/有/无 | 2/3/3/1 | 1–2 | 主计划第 7 节余额适配器 |
| advancedcustom | 575 | 有 | 3 | 3 | 主计划第 7 节 |
| ollama | 1,147 | 有 | 2 | 3 | 本机模型，对 Desktop 有价值 **[猜测]** |
| tencent + tokenhub | 501 + 289 | 有 | 5 / 2 | 1 / 2 | `docs/TOKENHUB_INTEGRATION.md` |

**次要**（已注册、可新建，无额外维护信号）：ali 1,035、volcengine 1,260、zhipu 437、zhipu_4v 388、xai 305、mistral 188、cohere 423、minimax 623、cloudflare 343、perplexity 138、replicate 561、jina 108、dify 506、coze 542、submodel 103、baidu_v2 159、mokaai 203、jimeng 420、xinference 19、sub2api 23、newapi 127；任务类 task/kling 706（10 次提交）、sora 556、gemini 773、vertex 567、doubao 575、ali 841、hailuo 699、jimeng 670、vidu 487、suno 323、taskcommon 97。其中无测试：baidu_v2、cohere、deepseek、jina、mistral、newapi、openrouter、perplexity、submodel、xai、xinference、zhipu_4v、ai360、lingyiwanwu。

**疑似冷门**：
- palm 274 行：前端已注释类型 11，无法新建，只服务存量渠道 **[代码]**；PaLM API 已被上游停用 **[猜测]**。
- ai360 5 行、lingyiwanwu 9 行：只有模型名常量，经 openai 适配器转发 **[代码]**。
- baidu（v1）516 行、xunfei 474 行：仍可新建，各 3 次机械提交；与 baidu_v2 并存 **[猜测：v1 接口已被 v2 取代]**。
- task/suno 323 行（SunoAPI 类型 36）。
- 无适配器的历史类型：OpenAIMax(6)、OhMyGPT(7)、AILS(9)、AIProxy(10)、API2GPT(12)、AIGC2D(13)、FastGPT(22) 走 openai；AIProxyLibrary(21) 在 `common/api_type.go` 映射到 `APITypeAIProxyLibrary`，但 `GetAdaptor` 中对应分支被注释，返回 nil，调用方会报错（`controller/model.go:120`、`relay/rerank_handler.go:39` 等有 nil 检查）**[代码]**。
- Midjourney 代理不在 `relay/channel`：`relay/mjproxy_handler.go` 741、`service/midjourney.go` 516、`controller/midjourney.go` 331、`model/midjourney.go` 250 等，合计约 2.0k 行 **[运行]**。

渠道类型数值存在 `channels.type` 中，是数据契约。任何适配器删除都会让存量渠道失效，因此最多归 D 类，C 类只做"新建表单隐藏"。**[代码]**

---

## 3. 重复与冗余

### 3.1 版本/模式开关重复（最影响三版本落地）**[代码]**

| 开关 | 位置 | 作用 |
| --- | --- | --- |
| `MYAPI_EDITION=full|lan` | `middleware/edition.go`，`main.go:203` 挂 `EditionGuard`；`controller/codex_local_auth.go:153`；`Dockerfile`、`deploy/install.sh`、`cli/myapi.mjs`、`electron/main.js:351` | HTTP 路径黑名单；未知值回落 Full |
| `VITE_SELF_USE_MINIMAL` | `web/src/lib/self-use-build.ts`，Dockerfile 在 `lan` 时设为 true，`electron-build.yml:109`、`npm-publish.yml:130` 固定 true | 构建期裁剪导航、设置分区、语言列表、资金能力 |
| `SelfUseModeEnabled` | `setting/operation_setting/operation_setting.go:6`，安装向导写入，`/api/status` 输出 `self_use_mode_enabled` | 未设倍率模型放行等运行行为 |
| `user_funding_mode`（enabled/retirement/disabled） | `model/user_funding_policy*.go`；前端 `usePersonalWorkbenchMode()` 以 `disabled` 切换个人工作台 | 资金能力，同时被前端当作"整体界面切换" |

后果：同一后端二进制无法让前端知道自己是 Full 还是 Lite；`docs/MYAPI_PLATFORM_REBUILD_PLAN_20261020.md` 第 1 节已指出"funding=disabled 被用作整体界面切换"。

### 3.2 前端并行实现 **[运行]**（行数）**[代码]**（关系）

| 领域 | 实现 A | 实现 B | 切换条件 |
| --- | --- | --- | --- |
| 总览 | `dashboard/components/overview/legacy-overview-dashboard.tsx` 902 行 | `dashboard/components/personal-workbench/` 约 1,985 行（另有 5 个已死文件，见 A2） | `usePersonalWorkbenchMode()` |
| Key | `keys/components/api-keys-table.tsx` 等 | `personal-api-keys.tsx` 613、`personal-subscription-share-form.tsx` 641 | 同上 |
| 调用记录 | `usage-logs` 通用日志 | `personal-call-logs.tsx` 360、`personal-call-detail.tsx` 183 | 同上 |
| 上游 | `channels`（33.6k） | `connections`（890）、`personal-upstream`（1,487） | 不同路由 |
| 布局 | `authenticated-layout` + `app-sidebar` | `personal-console-layout.tsx` 280 | 同上 |

表格抽象没有重复：`useReactTable` 只在 `components/data-table/hooks/use-data-table.ts` 一处调用，9 个 feature 共用 `@/components/data-table` **[运行]**。

### 3.3 后端重复或近似重复

| 项 | 规模 | 说明 |
| --- | --- | --- |
| 支付提供方按家各写两套 | 生产约 7.7k 行（支付相关文件 9,360 行减去被关键字误匹配的渠道亲和 4 个文件 1,624 行）**[运行]** | Stripe、Creem、Waffo、Waffo Pancake、Epay 各有 `topup_*.go` 与 `subscription_payment_*.go`，另有 `payment_webhook_availability.go` 中 9 个已死判断函数 **[代码]** |
| OAuth 内置提供方与通用提供方 | `oauth/github.go`、`discord.go`、`linuxdo.go`、`oidc.go` 共 740 行；`oauth/generic.go` 678 行；`controller/custom_oauth.go` 584 行 **[运行]** | 内置四家可以表达为通用提供方的预设 **[猜测]**；Telegram、WeChat 走独立控制器 |
| 服务层到 relaykit 的转发包装 | `service/openai_chat_responses_compat.go`、`service/request_converter.go` 中 7 个函数 | 全部不可达（deadcode），属 A1 |
| 两套日志 API | `common.SysLog/SysError` 439 处，`logger.Log*` 483 处 **[运行]** | 有意分工（无 ctx 的系统日志与请求关联日志），AGENTS.md 对两者有具体要求，不建议合并 **[代码]** |
| `relaykit/relayconvert/kitutil/json.go` 与 `common/json.go` | 小 | 为保持 relaykit 独立必须存在，不算冗余 **[代码]** |
| "subscription" 一词两义 | — | 商业订阅计划（`/api/subscription`、`features/subscriptions`）与上游订阅窗口分额（`/api/subscription-share`、`model/subscription_share.go`）。后者是个人核心功能，前者是 Full 商业功能，名称相近，易误伤（见 4.2 第 3 条）**[代码]** |

### 3.4 分层问题（controller 承担业务逻辑）**[代码]**

- `controller/channel-billing.go` 2,394 行，其中第 798–1,979 行（约 1.2k 行）是额度历史的分桶、聚合、告警推导（`aggregateQuotaHistorySnapshots`、`deriveQuotaHistoryMetrics` 等），属于 service 层职责。
- `controller/channel.go` 2,465、`controller/channel_upstream_update.go` 1,194、`controller/ratio_sync.go` 1,029、`controller/codex_usage.go` 989 同类情况较多。
- 这违背 AGENTS.md 的 Router → Controller → Service → Model 分层，但只是位置问题，不影响行为。

### 3.5 只有单一调用点的包级未导出函数（AGENTS.md 要求内联）

启发式统计（脚本 `scratchpad/single.py`：未导出、包内生产代码恰好 1 处引用、测试 0 处引用；不区分是否代表业务概念）**[运行]**：

| 包 | 个数 | 行数 | 占该包未导出函数 |
| --- | --- | --- | --- |
| model | 241 | 6,489 | 911 |
| relay | 183 | 4,244 | 407 |
| service | 168 | 3,928 | 512 |
| controller | 114 | 2,390 | 447 |
| 其余 | 84 | 1,591 | — |
| 合计 | 790 | 18,642 | — |

其中不超过 15 行的 396 个，是最明确的内联候选。局限：按标识符文本计数，函数作为回调传递也算 1 处引用；名字代表业务概念的应保留（AGENTS.md 允许）。

### 3.6 死代码

- Go：`deadcode -test ./...` 报告 220 个不可达函数 **[运行]**，函数体合计约 2,099 行（脚本 `scratchpad/deadspan.py` 按声明到首个列首 `}` 计算，含前置注释）。其中 relaykit 25 个 142 行属于独立模块的公开 API，不能按根模块视角判死；`model/admin_quota_adjustment_test.go` 中 1 个测试辅助函数未使用。根模块生产代码可删 194 个，约 1,951 行。按文件最多的：`service/file_decoder.go` 136、`common/redis.go` 119、`model/channel.go` 104（10 个函数）、`common/utils.go` 97、`controller/user.go` 61、`service/sensitive.go` 50、`common/disk_cache.go` 49、`relay/common/relay_info.go` 49、`model/pricing_runtime_snapshot.go` 47。完整清单见 `scratchpad/out/deadcode.txt` 与 `deadspan.txt`。
  - 抽查：`service/sensitive.go` 的 `CheckSensitiveMessages`、`SensitiveWordReplace` 不可达，但敏感词检查仍经 `controller/relay.go:167` → `service.CheckSensitiveText` 生效，删除不影响功能 **[代码]**。
  - 构建标签：仓库只有 `common/system_monitor_unix.go` 与 `_windows.go` 两个带 `//go:build` 的文件，不涉及报告中的函数 **[运行]**。
- 前端：`knip`（仓库自带配置）报告未引用文件 76、未用依赖 6、未用 devDependency 1、未用导出 140、未用导出类型 68、重复导出 1 **[运行]**。
  - 误报 2 个：`src/i18n/config.minimal.ts`（`rsbuild.config.ts:69` 以别名引用）、`src/i18n/static-keys.ts`（i18n 扫描用的数据文件，`web/AGENTS.md` 3.1 要求登记）。扣除后 74 个文件、约 8,387 行。
  - 抽查确认：`personal-workbench/recent-records-section.tsx` 已被 `overview-recent.tsx` 取代，只剩同名 testid 被 `tools/ui/personal-workbench-browser.mjs` 使用；`account-window-list.tsx` 仅被同样未引用的 `account-windows-section.tsx` 引用；`features/home/components/*` 的分区组件未被 `features/home/index` 使用 **[运行]**。
  - `components/ai-elements` 45 个文件中 24 个未引用（上游 AI Elements 组件库的拷贝）。
  - knip 配置忽略了 `src/components/ui/**`，补充脚本检查：60 个 UI 原语（目录内另有 1 个测试文件）中 10 个无任何引用（aspect-ratio、button-group、context-menu、direction、item、kbd、menubar、navigation-menu、pagination、resizable），约 1,448 行 **[运行]**。
  - 依赖核对 **[运行]**：`@tanstack/react-virtual`、`next-themes` 在 src 中无任何 import；`auto-skeleton-react`、`tokenlens`、`@xyflow/react`、`react-resizable-panels` 只被上述未引用文件使用；`embla-carousel-react` 被 `components/ui/carousel.tsx` 使用，是 knip 因忽略 ui 目录产生的误报。
- i18n：`en.json` 共 6,954 个 key，其中 637 个在 `src` 的 `.ts/.tsx` 中找不到带引号的字面量 **[运行]**。可能有动态拼接的 key，必须用 `bun run i18n:sync` 或等价工具复核后再删，归 B 类。

---

## 4. 版本差距（对照 `RELEASE_MANIFEST.md` 2.1 能力矩阵）

### 4.1 能力矩阵到代码的映射

| 能力（2.1 行） | Full | Lite | Desktop | 现状 **[代码]** |
| --- | --- | --- | --- | --- |
| 转发、鉴权、Key、日志、额度安全、备份恢复 | 完整 | 必须保留 | 同 Lite | 三者共用同一后端；备份恢复只有 Docker `myapi upgrade` 的环境文件回退与 `tools/runtime/sqlite-restore-smoke`，无产品内备份入口 |
| 组织/用户/权限、订阅支付、复杂运维、多节点 | 完整 | 按矩阵决定，降级保留历史数据 | 不另行实现 | 只有 Legacy `lan` 的路径黑名单；无 `lite` 枚举 |
| 数据库与拓扑 | 三库 + Redis 等 | SQLite 优先 | 平台 `userData` | Electron 已用 `userData/data` 下 SQLite（`electron/main.js:310-371`）；Lite 服务器形态无专门默认值 |
| 本机/LAN/公网 | 可选 | 可选 | 可选，默认本机 | Electron 默认 `127.0.0.1`，托盘可切 LAN；无公网向导；访问模式未与功能版分离 |
| S5-P 提示词学习 | 支持 | 支持 | 支持 | 后端路由未被 LAN 门禁封锁；前端路由存在但不在侧栏 `use-sidebar-data.ts` 中 |

### 4.2 需要新增或改造的门禁点

后端：
1. `middleware/edition.go`：新增 `lite`，`lan` 作为 Legacy 别名映射到 Lite；按 RELEASE_MANIFEST 2.2，新旧来源冲突时拒绝启动。当前"未知值回落 Full"会让拼错的 `MYAPI_EDITION=lite` 直接暴露 Full 全部入口。改为拒绝启动属于行为变化，需维护者确认 **[代码]**。
2. `/api/status`（`controller/misc.go`）增加 `product_edition` 与能力列表（只增字段，不改既有字段），让前端在运行期裁剪导航；保留 `VITE_SELF_USE_MINIMAL` 作为兼容。**[代码]**
3. `isLANRestrictedPath` 当前遗漏 **[代码]**：
   - `/api/redemption`（兑换码管理，前端在自用构建里已隐藏 `/redemption-codes`，后端未封）；
   - `/api/deployments`（io.net，前端设置分区 `model-deployment` 已 `selfUseHidden`）；
   - `/api/mj` 与 `router/relay-router.go` 的 `/mj` 转发（Midjourney）；
   - `/api/user/topup/self`、`/api/user/topup/info` 等已被 `/api/user/topup` 前缀覆盖，无遗漏。
   扩大封锁会改变现有 Legacy LAN 实例的接口行为，需维护者确认。
4. 同一函数封锁了整个 `/api/subscription` 前缀，包括 `/api/subscription/self` 与 `/api/subscription/self/preference`。但 `main.go:139` 的 `StartSubscriptionQuotaResetTask` 在所有版本都运行，Full → Lite 降级后，已有订阅的用户仍会被按订阅计费/重置，却看不到自己的订阅。建议 Lite 对"自己的订阅与资金历史"开放只读，购买入口继续封锁（前端 `can_view_funding_history` 已有只读例外的设计）**[代码]**。
5. 与 `/api/subscription` 无关的 `/api/subscription-share/*`（Key 分额，个人核心功能）当前未被误封（前缀匹配要求 `/api/subscription/`），改造时必须保持 **[代码]**。
6. 后台任务：`perfmetrics.Init`、Midjourney/任务轮询、订阅重置在所有版本运行。Lite 中商业订阅重置必须继续（历史订阅），性能指标聚合可按版本关闭（只影响公开排行）**[代码]**/**[猜测]**（关闭的影响需核实）。

model / 迁移：
7. 所有版本执行同一套 `AutoMigrate`（`model/main.go` 中主库 98 张表 + 日志库 `Log`、`BillingLogProjectionIdentity`）。不要按版本跳过 Full-only 表的迁移：一旦跳过，Lite → Full 切换会在旧库上首次建表，Full → Lite 后再回升也会因 schema 不一致失败。版本只影响 HTTP 暴露和后台任务 **[代码]**。

前端：
8. 导航与路由：以 `use-sidebar-data.ts`、`system-settings.config.ts`、`lib/legacy-route.ts`、`use-top-nav-links.ts`、`profile-dropdown.tsx` 中已有的 `SELF_USE_MINIMAL` 分支为基础，改为读取运行期能力；`/wallet`、`/subscriptions`、`/redemption-codes`、`/pricing`、`/rankings`、`/chat`、`/models/deployments`、`/usage-logs/drawing`、`/system-settings/content` 等路由需要在 `beforeLoad` 层按能力拦截，而不仅是隐藏菜单 **[代码]**。
9. `usePersonalWorkbenchMode()` 不应再决定版本外观；资金模式只控制金额与资金动作（与平台方案第 2 节一致）**[代码]**。

electron（Desktop）：
10. `electron/main.js:351` 固定 `MYAPI_EDITION: 'lan'`，应改为 `lite` 并单独传入访问模式；托盘文案中英文混写且未接 i18n（如 `'重启并启用 LAN…'` 与 `'LAN status and connection help…'`），不满足七语言要求 **[代码]**。
11. `electron/package.json` 的 `"publish": null`，无更新器；窗口关闭、退出应用、停止后台服务三者的区分只在 `main.js:531`、`806`、`817` 有基础实现；Linux 有打包 target 但 CI 只覆盖 macOS/Windows（RELEASE_MANIFEST 第 7 节）。按 TASKS.md 范围边界，本轮不做更新器 **[代码]**。

cli：
12. `cli/myapi.mjs` 只接受 `full|lan`（173–216 行）；`cli/lib/legacy-installation-profile.mjs`、`release-manifest.mjs`、`installation-state.mjs` 已实现画像/结构校验但未接线。接线依赖 RELEASE_MANIFEST 第 9 节待决事项，本轮只需让 CLI 能写出 `lite` 并兼容 `lan` **[代码]**。

### 4.3 "必须保留历史数据"对降级的约束

- 表与字段不删除；三版本共享 schema（见第 7 条）。
- Full → Lite 时隐藏的功能，其数据仍可能参与计费：用户余额（充值、兑换码、签到、邀请返利得到的 quota 都已写入 `users.quota`）、商业订阅额度（`user_subscriptions`、`subscription_pre_consume_records`）。因此 Lite 只能封写入/购买入口，不能停止这些数据在计费路径上的读取与结算。**[代码]**
- 进行中的外部状态（未回调的支付订单、Midjourney/视频异步任务）在切换前必须排空或保留回调能力：若 Lite 封锁 `/api/stripe/webhook` 等回调路径，切换时尚未完成的订单会无法入账。建议切换前检查未完成 `top_ups`、`subscription_orders` 与未终态 `tasks`，有则阻止切换（RELEASE_MANIFEST 第 5 节）。**[代码]**/**[猜测]**（需核对各支付回调对未完成订单的处理）
- 外部 OAuth 绑定：Lite 封锁 `/api/oauth` 后，只用 GitHub/OIDC 等登录、没有密码的用户将无法登录。降级前需要提示或要求先设置密码/Passkey。**[代码]**/**[猜测]**（未核对是否存在无密码账户的创建路径）

---

## 5. 精简候选分级

### 5.1 A 类：确认无引用，可直接删除（可逆，git 可恢复）

| # | 内容 | 规模 | 证据 |
| --- | --- | --- | --- |
| A1 | Go 根模块不可达函数（不含 relaykit 公开 API） | 194 个函数，约 1,951 行，分布 model 435、service 428、common 398、controller 221、relay 194 等 | `deadcode -test ./...` **[运行]** |
| A2 | 前端未引用文件（排除 2 个误报） | 74 个文件，约 8,387 行；最大：`home/components/hero-terminal-demo.tsx` 551、`ai-elements/context.tsx` 440、`open-in-chat.tsx` 409 | `knip` + 抽查 **[运行]** |
| A3 | 未引用 UI 原语 | 10 个文件，约 1,448 行 | 脚本 **[运行]**；建议放到 UI 重设计阶段一并决定，避免删后又加回 |
| A4 | 无引用依赖 | `@tanstack/react-virtual`、`next-themes`（立即）；`auto-skeleton-react`、`tokenlens`、`@xyflow/react`、`react-resizable-panels`（随 A2/A3） | 检索 **[运行]** |
| A5 | 未使用的测试辅助函数 | `model/admin_quota_adjustment_test.go:22` `getAdminAdjustment` | deadcode **[运行]** |

合计约 1.18 万行。A1 中有一部分位于计费相邻文件（`service/quota.go` 的 `PostConsumeQuota`、`model/user.go` 的 `DeltaUpdateUserQuota`、`controller/topup.go` 的 `getPayMoney` 等），删除不可达代码不改变行为，但应在单独提交里由审查者逐项看过。

### 5.2 B 类：可合并或重构，行为不变

| # | 内容 | 规模 | 证据 | 备注 |
| --- | --- | --- | --- | --- |
| B1 | 四个模式开关收敛为"功能版 + 能力列表"，后端单一来源 | 至少 40 个文件（检索 `SELF_USE_MINIMAL\|SelfUse` 的文件列表在 40 条处截断）**[运行]** | 3.1 **[代码]** | 与任务 4 合并做；`/api/status` 只增字段 |
| B2 | 前端并行实现收敛（总览、Key、调用记录、上游、布局） | personal 侧约 4.1k 行 + 旧总览 902 行 | 3.2 **[运行]** | 与 UI 重做合并，不单独做 |
| B3 | `controller/channel-billing.go` 额度历史聚合下沉到 service；其他大型 controller 同类处理 | 约 1.2k 行搬移 | 3.4 **[代码]** | 纯搬移；需对应包测试 |
| B4 | 内联单调用点短辅助函数 | 396 个（≤15 行），约 3–4k 行 **[猜测]** | 3.5 **[运行]** | 每批限一个包；保留业务概念命名的函数 |
| B5 | 去掉 knip 报告的未用导出 | 140 个导出 + 68 个类型 | **[运行]** | 删 `export` 或删定义 |
| B6 | i18n 疑似未用 key | 637 个 × 7 语言 | **[运行]** | 必须先用 i18n 工具复核 |
| B7 | OAuth 内置 4 家改为通用提供方预设 | 约 740 行 | 3.3 **[猜测]** | 登录行为风险中等；Lite 不暴露，建议推迟 |
| B8 | 支付提供方抽象为统一接口 | 约 7.7k 行 | 3.3 **[代码]** | 触及支付与计费契约，Full only，建议不在本轮 |
| B9 | 前端文案区分"商业订阅"与"订阅分额" | 文案层 | 3.3 **[代码]** | 不改 API 路径与字段 |
| B10 | 账务子系统补所有权说明（不改代码） | model 约 1.7 万行 | 2.1 **[运行]** | 精简风险高于收益；先写清每张表的写入者与恢复路径 |

### 5.3 C 类：按版本隐藏而不删除（Lite/Desktop 隐藏，Full 保留）

| # | 功能 | 后端入口 | 前端入口 | 现有门禁 **[代码]** |
| --- | --- | --- | --- | --- |
| C1 | 支付/充值/钱包 | `/api/user/topup*`、`/api/user/pay`、`/api/user/{stripe,creem,waffo,waffo-pancake,epay}`、`/api/{stripe,creem,waffo,waffo-pancake}/webhook`、`/api/option/payment_compliance` | `/wallet`、设置 `payment` | 后端已封；前端已隐藏 |
| C2 | 商业订阅计划 | `/api/subscription*` | `/subscriptions` | 已封；需开放只读 `self`（4.2 第 4 条） |
| C3 | 兑换码管理 | `/api/redemption` | `/redemption-codes` | 后端未封 |
| C4 | 签到 | `/api/user/checkin` | 设置 `checkin` | 已封 |
| C5 | 邀请返利 | `/api/user/aff`、`aff_transfer` | 钱包内 | 已封 |
| C6 | 公开定价/排行/性能指标 | `/api/pricing`、`/api/rankings`、`/api/perf-metrics`、`/api/ratio_config` | `/pricing`、`/rankings` | 已封 |
| C7 | 外部 OAuth 与自定义 OAuth | `/api/oauth`、`/api/user/oauth`、`/api/user/:id/oauth/*`、`/api/custom-oauth-provider` | 登录页、设置 `auth` | 已封 |
| C8 | Midjourney 代理与绘图日志 | `/mj/*`、`/api/mj` | `/usage-logs/drawing` | 未封 |
| C9 | io.net 模型部署 | `/api/deployments` | `/models/deployments`、设置 `model-deployment` | 后端未封，前端已隐藏设置分区 |
| C10 | 站点内容管理（公告、FAQ、API 信息、Uptime Kuma、首页内容） | `/api/option` 相应键 | `/system-settings/content` | 前端已隐藏 |
| C11 | SMTP、Worker 代理、更新检查 | 选项键 | 设置分区 | 前端已隐藏 |
| C12 | 冷门/历史渠道类型在新建表单中隐藏（存量照常转发） | 无 | 渠道类型下拉 | 已隐藏 6、9–13、21；可扩展到 palm 以外的疑似冷门类型 |

### 5.4 D 类：用户可见功能下线，必须维护者逐项确认

对数据库表字段一律不建议删除；下表"数据表"一栏只说明受影响的数据，下线也保留表。行数为生产代码 **[运行]**。

| # | 功能 | 现有入口 | 数据表 | 规模（后端 / 前端） | 影响面 |
| --- | --- | --- | --- | --- | --- |
| D1 | 在线支付（Stripe、Creem、Waffo、Waffo Pancake、Epay） | C1 所列 | `top_ups`、`options` 中支付键 | 约 7.7k / 钱包 5.1k + 设置约 4.5k | 未完成订单回调、已充值余额照常可用 |
| D2 | 商业订阅计划 | `/api/subscription*` | `subscription_plans`、`subscription_orders`、`user_subscriptions`、`subscription_pre_consume_records` | 约 3.0k / 3.2k | 订阅额度参与计费与重置任务，下线须先处理存量订阅 |
| D3 | 兑换码 | `/api/redemption`、`/api/user/topup`（兑换） | `redemptions` | 465 / 2.1k | 未使用兑换码作废与否 |
| D4 | 签到 | `/api/user/checkin` | `checkins` | 350 / 设置约 200 | 小 |
| D5 | 邀请返利 | `/api/user/aff*` | `invite_reward_grants`、`aff_quota_transfers`、`users.aff_*` | 估计 <1k **[猜测]** / 钱包内 | 未转出的返利额度 |
| D6 | 公开定价页、排行榜、性能指标 | C6 所列 | `perf_metrics` | 约 1.7k（rankings 621、perf_metrics 1,018、usedata_rankings 66）/ 1.15 万 | 公开站点信息 |
| D7 | Midjourney 代理 | `/mj/*`、`/api/mj` | `midjourneys` | 约 2.0k / 日志页一部分 | 存量 MJ 渠道与任务 |
| D8 | io.net 模型部署 | `/api/deployments` | 选项键 | 约 2.4k / 2.5k | 已创建的远端部署不再可管理 |
| D9 | 外部登录（GitHub、Discord、LinuxDO、OIDC、Telegram、WeChat、自定义 OAuth） | C7 所列 | `user_oauth_bindings`、`custom_oauth_providers`、`users.*_id` | 约 3.6k（`oauth/` 1,662 + 相关 controller/model 文件）/ 认证与设置约 3k | 无密码用户无法登录 |
| D10 | 删除疑似冷门适配器（palm、ai360、lingyiwanwu、baidu v1、xunfei、suno） | 渠道类型 | `channels.type` 存量值 | 约 1.6k / 常量 | 存量渠道失效 |
| D11 | 上游验收运行（beta.21 测试工具进入产品） | `/api/upstream-acceptance/runs` | `upstream_acceptance_runs` | 约 1.0k / — | 维护者测试流程 |
| D12 | `docs/` 历史阶段文档与证据归档 | — | — | 跟踪文件 1,177 个，其中 `docs/evidence` 1,011 个 | 审计追溯；建议迁到独立归档分支而非删除 |
| D13 | `patches/`（6 个补丁）与 `bin/` 旧迁移 SQL、`time_test.sh` | `package.json` 的 `files` 包含 `patches/` | — | 6,702 行 | 源码分发包内容 |
| D14 | `new-api.service` 兼容 systemd 单元 | 文件 | — | 小 | 仍用旧单元名的安装 |

若 D1–D11 全部批准下线，各行规模加总为后端生产代码约 2.4 万行、前端约 3 万行；扣除共享代码后实际可删量估计为后端 1.8–2.4 万行、前端 2.5–3 万行 **[猜测]**。

---

## 6. 风险与执行顺序

触及计费或对外契约、需要特别小心的改动 **[代码]**：
- 计费：A1 中计费相邻死代码；4.2 第 4 条（订阅只读开放）；4.3 降级规则；B8；D1、D2、D3、D5。必须遵守 AGENTS.md 的饱和审计与 quota 换算规则。
- 对外契约：`/api/status` 增字段（B1）；EditionGuard 封锁范围变化（4.2 第 1、3 条）；`MYAPI_EDITION` 新值与未知值处理；CLI 环境变量；Electron 传给后端的环境变量；渠道类型数值（D10）。
- 数据库：不改表结构；所有版本保持同一迁移集。

建议顺序（每批单独提交，验证通过再进入下一批）：

| 批次 | 内容 | 验证命令 |
| --- | --- | --- |
| 0 基线 | 不改代码，记录基线 | `go build ./...`；`go vet ./...`；`cd relaykit && GOWORK=off go build ./...`；`cd web && bun run typecheck && bun run lint && bun run test && bun run build`；`bun run knip` |
| 1 前端 A 类 | A2、A4（A3 推迟到 UI 阶段） | `bun run typecheck`、`bun run lint`、`bun run test`、`bun run build`；knip 未引用文件数从 76 降到 2（剩两个误报）；`rg` 确认 `tools/ui` 脚本不 import 被删文件 |
| 2 后端 A 类 | A1 按包分批：common → setting → middleware → relay → service → controller → model；A5 | 每批 `go build ./...`、`go vet ./<pkg>/...`、`go test ./<pkg>/...`；重跑 `deadcode -test ./...` 确认条目消失且无新增；计费相邻文件另跑 `go test ./service/... ./model/... ./relay/common/...` |
| 3 版本门禁（任务 4） | B1、4.2 第 1–9 条，C1–C12 的门禁部分 | 新增 `middleware` 表驱动测试（full/lite/lan/未知值 × 路径）；`go test ./middleware/... ./controller/...`；前端路由守卫测试；三库迁移检查（不改 schema，但要确认迁移集不随版本变化） |
| 4 Desktop（任务 5） | 4.2 第 10–12 条 | `npm run desktop:check`、`npm run lan:check`、`node --test electron/test/*.test.mjs`、`node --test cli/test/*.test.mjs` |
| 5 结构重构 | B3、B4、B5、B6、B9 | 同批次 2；B6 用 `bun run i18n:sync` 复核 |
| 6 UI 阶段 | B2、A3 | UI 角色负责 |
| 7 D 类 | 逐项按维护者决定 | 每项给出数据保留检查与回退说明 |

---

## 7. 已运行的工具与结果

| 命令 | 结果 |
| --- | --- |
| `git -c core.fileMode=false ls-files '*.go'` + `wc -l`（排除 `._*`） | 成功，1,525 个文件 |
| `go list -f '{{.ImportPath}} ...' ./...` | 成功，91 个包（未在报告中直接使用） |
| `go run golang.org/x/tools/cmd/deadcode@latest -test ./...`（工作目录为仓库根，输出写到 scratchpad） | 成功，退出码 0，220 条 |
| `cd relaykit && GOWORK=off go build ./...` | 成功，退出码 0 |
| `rsync` 将 `web/`（不含 node_modules、dist、`._*`）复制到 `scratchpad/webcopy` 并软链 node_modules，执行 `./node_modules/.bin/knip --no-progress --reporter compact` | 成功运行，退出码 1（发现问题时 knip 返回 1），293 行输出 |
| `git log --name-only` 统计 `relay/` 下各适配目录提交数 | 成功（目录 pathspec `relay/channel` 在本卷上无输出，改用 `relay/*` 后在脚本中过滤） |
| Python 脚本（`goimp.py`、`feat.py`、`deadspan.py`、`single.py`、`domains.py` 及内联脚本） | 成功；位于 `scratchpad/`，均只读仓库 |
| `git -c core.fileMode=false status --porcelain --untracked-files=all` | 0 行，仓库未被修改 |

未运行：`go vet ./...`、`go test`、`bun run typecheck/test/build`（本轮只评估，未改代码，基线验证留给执行批次 0）；relaykit 内部的 deadcode（relaykit 无 main 包，deadcode 不适用）。

---

## 8. 未覆盖的部分

1. 未读完 `model/` 账务子系统与 `task_recovery.go` 的完整逻辑，B10 与 4.3 中涉及其行为的判断只到文件级别。
2. 未核对各支付回调对"Lite 下未完成订单"的具体处理（4.3 第 3 条标为猜测）。
3. 未核对是否存在只有外部 OAuth、没有密码的账户创建路径（4.3 第 4 条）。
4. 未逐个检查 790 个单调用点函数是否代表业务概念；未逐个复核 637 个 i18n key。
5. 未检查 `tools/`（约 4.2 万行，大量浏览器验收脚本）在 UI 重做后会失效的范围；未评估 `website/`、`deploy/` 的精简空间。
6. 未做三库实际迁移验证与运行时验证；所有门禁差距来自代码阅读。
7. 未统计后端 i18n（`i18n/locales/*.yaml`）的未用条目。
8. 适配器"冷门"判断没有使用生产渠道数据（无授权读取生产库），只用注册状态、测试、提交数。
