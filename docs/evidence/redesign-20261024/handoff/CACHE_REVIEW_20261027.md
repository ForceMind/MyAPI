# 缓存专项接手记录（2026-10-27）

## 0. 2026-10-29 默认关闭的上游诊断日志

### 2026-10-30 第五轮：新Full镜像打包与独立加载验收（生产未授权执行）

- 本轮授权：在Actions打包验证，并交接服务器任务「查看状态并更新部署版本」先给出更新镜像和开启缓存会话开关的最小方案。必须等维护者对本次方案明确确认后才动生产，只观察自然流量，不主动发模型请求；旧部署/诊断授权不能代替本次确认。
- **源码与CI**：`3e6b8cca333194644a8bb2902f662095b407e090`，tree `8531c06d2ae3107201d8e10a79612ba7adc77f48`；[源码完整CI40911707379](https://github.com/ForceMind/MyAPI/actions/runs/40911707379) success，10成功/1浏览器作业跳过。[打包40924927031](https://github.com/ForceMind/MyAPI/actions/runs/40924927031) workflow_dispatch、dev、attempt1，输入api_only=true/run_browser=false/personal=false，producer与独立consumer成功、LAN安装跳过；已读所有作业/步骤和完整JSON资格报告。Full、linux/amd64、fresh SQLite，共20项全true：HTTP鉴权/脱敏、精确单次扣费、7个HTML/JS/CSS资源读取、原容器重启、另一同镜像容器的SQLite/WAL恢复等。浏览器安装/执行跳过，browser=not_run；临时容器清理成功，不代表生产数据迁移或跨版本回滚验收。
- **镜像包**：[artifact12919793136](https://github.com/ForceMind/MyAPI/actions/runs/40924927031/artifacts/12919793136)，314216667字节，内含manifest.json与images.tar（app与固定Bun测试夹具镜像；生产只使用app）。GitHub构件ZIP摘要 `sha256:74f8ee02ba7d64a3eee5f6eec01b15297c62a660f3c2729b999d6f4a891e85dd`；有效期至北京时间2026-11-06 01:51:52，未下载本机。
- **可信绑定**：producer manifest SHA256 `ad18daa79442831737bce2ff73c088fb9275c038d6368416440584ad18c08ff7`；consumer按该摘要、source/head/tree/run/producer-attempt验证并加载通过。app config/image ID `sha256:38b61c5b3f45fb0fbb4de7c851ae2b9029e691571c2a4dd1985993da23f5e5e3`。images.tar摘要在受该manifest绑定的文件内核对。本机只读日志/元数据，未下载或加载镜像。目标Docker若呈现不同loaded ID，先证明config/layers/rootfs等价，不能绕过校验器、回退旧包或使用不存在的GHCR/tag坐标。
- **报告**：[artifact12919559223](https://github.com/ForceMind/MyAPI/actions/runs/40924927031/artifacts/12919559223)，2071字节，ZIP摘要 `sha256:a3fd75a967b87980929833a89608707df5244a1e8193a32278a3c3e0678071da`；至北京时间2026-11-06 01:53:11。报告中的版本仍为0.2.0-beta.26，必须按本轮源码与镜像身份区分，不能只认版本字符串。
- **缓存开关边界**：镜像验收保持 `MYAPI_CODEX_CHAT_DEFAULT_CACHE_IDENTITY_ENABLED=false` 默认行为；开启合同的11族合成竞态已在源码CI通过，并不是本次Docker内启用或生产命中验收。开启只影响缺省身份，显式key/session、管理员覆盖删除保持；相同初始前缀/缺稳定账号/超出派生上限等限制见第四轮。不得保证一定提升真实命中。
- **服务器方案应明确**：校验来源与架构、保留旧镜像与数据库/SQLite WAL备份、准确回滚点、容器环境变量实际透传（不能只写宿主机.env或在容器内export），最小更新/启用步骤、健康与已保存自然流量核对、是否需要有界诊断以及到限关闭/恢复策略。诊断候选可沿用5分钟/新增100MiB上限，但本轮尚未授权开启；应明确诊断开关与缓存会话开关各自保留/恢复规则。不得调用测试连接、供应商采样、凭据刷新或主动模型请求，不输出正文/密钥/账号材料。已有诊断不记录出站session头，不能承诺从现有日志直接读到它。
- 本轮不发布GHCR/NPM/tag，不合main，不操作生产。随后只更新本节、README、OPERATIONS和TASKS，应用/镜像仍绑定3e6b8cca，不重建或重标成文档SHA；接手读取最新文档提交CI。服务器只接收方案任务，执行须等维护者确认。

### 2026-10-30 第四轮：获批实施A/B/C（产品完整CI通过）

维护者在第三轮方案后回复“好的，快点”，授权三项最小产品修复；没有授权生产部署、开启身份开关、主动供应商请求、发布或合main。

- 候选代码 `42a258dfecc8e584a231f6b81955440091a969d1` / [完整CI40905811752](https://github.com/ForceMind/MyAPI/actions/runs/40905811752) 首次success（10作业成功、1浏览器作业跳过），已亲自读全部作业/步骤及专项PASS日志。根模块vet/build、linux/386、relaykit独立vet/build、两模块全量、全部既有竞态、三库、Redis、分发与Desktop通过；前端同步、24/24专项、278文件1917/1917全量、构建/typecheck/lint通过。浏览器安装与执行均跳过，浏览器未验证。
- **A最小适配器边界**：接入JSON、缓冲SSE与流式SSE，标准response.usage/input_tokens_details优先；兼容终态顶层usage与prompt_tokens_details别名。缓存缺失/null/0/正数在客户端保持区别，后一个明确零可替换正数，null整体usage不覆盖之前的终态用量；缓冲也读至DONE/EOF核对后续终态。只在局部客户端投影处理，不全局重写DTO或改relaykit。
- **计费保护**：原始证据与兼容归一化分开，不凭兼容字段创建native fee proof；在DTO进入converter前重建/清空不可信内部信封与Chat影子字段。净化大小写及Unicode折叠变体，非终态usage不能建立计费依据。保留原0d9b794a存在性修复；不改价格公式、预扣/结算/退款、安全门禁。恢复缓存计数可改变正确实扣，不能称只是显示；新增A的10个顶层测试族、B的11族及原转换/证据6族在专项竞态各PASS一次，无FAIL或DATA RACE。真实路由六场景按输入20/输出3/缓存8、模型1/输出2/缓存0.5精确单扣22 quota，且精确单次发送。大小写/Unicode变体、非终态信封、缓冲后终态零/未知、大输出、缺失与null及不可信来源均有回归。
- **B默认关闭**：`MYAPI_CODEX_CHAT_DEFAULT_CACHE_IDENTITY_ENABLED=false`；仅普通Codex订阅Chat转换，缺稳定AccountID/严格模式/渠道测试跳过。生成值绑定用户、调用Key、稳定账号及模型/工具/system/developer/首user前缀，不以channel_id或旋转凭据拆分；显式身份优先，只补缺的部分。默认值在参数操作前播入，参数覆盖/删除和runtime final map最高，不在最终发送端复活字段。后续参数操作改正文时不重新派生；无显式身份且相同初始前缀会共享默认身份，不宣称能辨认两个完全相同开头的独立会话。不修改调度、亲和规则或冻结渠道配置。
- **C只读保存窗口**：区分codex_wham_usage失败、历史独立错误和codex_relay_usage_limit转发事件；旧错误及series保留，不强合身份，不标成已解决。每窗口标自己的时间，前台可见每60秒读保存接口、恢复可见立即更新并提供手动入口；未知不补零。不触发后台WHAM/凭据刷新/测试连接，不改数据库、采样频率或阈值。新增8键en/zh及5语言英文回填，浏览器未验证。
- [对照40905060265](https://github.com/ForceMind/MyAPI/actions/runs/40905060265)成功，产品与探针源码9fbf640e；与42a的relay/router/relaykit/tools差异为零。双方固定输入6请求/8usage矩阵中MyAPI两种兼容缓存均保留7、input/output20/3，缺失/null/0不混淆，8个MyAPI场景的兼容缺口finding均为空。默认关闭的B在原对照中仍不生成，开启场景由新增回环合同测试单独保护，不冒充已在生产开启。[报告12910628770](https://github.com/ForceMind/MyAPI/actions/runs/40905060265/artifacts/12910628770)9597字节，至北京时间2026-11-05 23:50；本机未下载。
- 中间版本910a59f5对照40902620496成功，但审查发现大小写/非终态信封绕过、缓冲过早退出，已补修；其完整CI40902620465被新推送取消，不作通过证据。9fbf完整CI40905060413先失败于新增键在数字键前、与JS枚举同步顺序不同，随后取消；2f761b13/40905441282专项23/24，失败因自有夹具重复source误将转发限额写成WHAM，已只改输入保留原断言。中间失败/取消均保留，不把后续通过写成这些运行已通过。
- 全程本机仅读写源文件、gofmt与轻量Git/文本检查；没有本机依赖安装、编译、测试、浏览器、Docker或构件下载。没有新Docker包、版本tag/GHCR/NPM发布、main合并、生产或真实供应商写入；生产最后读回仍为上一轮cd35f867，本轮未复查服务器状态。未来构建/部署按已通过源码42a258df与新构件身份，不可沿用旧cd35包或只凭beta.26字符串。
- 收尾只更新本节、README、OPERATIONS、TASKS和工具README，不改应用/测试/工作流，不重建镜像或重标对照报告源码；接手读取最新文档提交的完整CI。

### 2026-10-29 第三轮：缓存身份与usage兼容合成对照（仅测试/工具，产品方案待确认）

- 最新对照源码 `8f9e533cd27fc16881de2eee6eff61aa8853ce0e`；先保留并仅快进同步另一会话的f0ae4505/0d9b794a字段存在性修复，不重做。相对f0ae4505本批只新增测试、共享合成输入、比较器和Actions，修改既有测试夹具参数；未改任何产品处理器/DTO/界面/采样、生产或真实供应商。生产读回仍为cd35f867，不能把开发分支的新逻辑称为已部署。
- [合成对照40888007112](https://github.com/ForceMind/MyAPI/actions/runs/40888007112)首次success：仅runner检出sub2api固定 `a3298b46ebe239fb0720b068a1eb3383955bdac2`，添加自有测试调用双方实际实现，两边共享输入摘要与源码身份验证通过。6个请求身份场景、8个usage场景全部执行，无真实模型或WHAM请求，浏览器未运行。报告：[artifact12903288943](https://github.com/ForceMind/MyAPI/actions/runs/40888007112/artifacts/12903288943)，9625字节，保留至2026-11-05北京时间22:14。
- [完整CI40888007450](https://github.com/ForceMind/MyAPI/actions/runs/40888007450)首次success，10成功、1浏览器混合作业跳过；已读全部作业及构建/两模块/独立relaykit/既有竞态/数据库Redis等关键步骤，无失败。中间工作流上下文、错误的Sub2显式键期望、报告路径及探针变量重名失败均保留在工具README；修正只属测试基础设施，没有删改既有产品断言。中间取消/失败运行不当作通过。
- **已实测**：缺key/会话头时MyAPI不补值；sub2api补key/session，追加消息稳定，跨调用Key与上游账号均隔离。显式key在MyAPI原样透传；sub2api OAuth路径会做账号/调用Key命名空间规范化，并非原样透传，不能盲目照抄该策略。
- **已实测**：标准缓存7时双方保留7；兼容的prompt_tokens_details别名和终态顶层usage中缓存7，MyAPI变0、sub2api保留7，顶层usage还使MyAPI该合成场景input/output变0/0而非20/3。缺失/null在MyAPI当前客户端投影中变0；sub2api省略字段，且也省略明确0。合法后一终态0优先，不用前正数/最大值覆盖。多usage帧逐条保留，不把前帧字段带入后帧。
- 这些证明**合成边界的兼容差异**，不是生产A/B的根因认定；没有证据表明生产A用了哪一种上游形状，也不承诺补key必然提高真实命中率。对照工具明确product_fixed=false，本机未安装依赖、构建测试、克隆sub2api或下载产物。
- 详细矩阵、运行方式和**待维护者确认的最小方案A/B/C**集中在 [tools/cache-compat-compare/README.md](../../../../tools/cache-compat-compare/README.md)：A先补局部usage归一化/存在性投影，保留标准0优先与金额/来源资格回归；B仅Codex Chat默认身份补全，尊重显式覆盖/删除、按用户/调用Key/稳定账号隔离，不改全局调度或生产规则；C独立修连接页采样状态分类/只读刷新，不隐藏失败。三项均未实施产品逻辑，需维护者明确确认。
- **采样反馈已只读核对**：后台默认60秒GET WHAM usage保存窗口；截至21:12最新200行去重133批全成功，24h为5326成功/1次503。另5条usage_limit_reached属于模型转发标记。22:26按生产数据库与实际聚合规则推导10个显示系列：6成功窗口、3转发限额、1旧503；旧503身份为credential_scoped而成功为provider_confirmed，不能强合并。连接页将error统一标采样失败、无定时刷新/聚焦刷新；配置查询错误重试也不是业务窗口刷新。已确认误分类和历史错误持续显示的触发路径，未取得浏览器真实API响应，不能宣称解释了“所有渠道只显示失败”（渠道1无错误）。浏览器未验证，未触发采样/登录/刷新凭据。
- 随后收尾仅文档，比较器/输入/探针继续绑定8f9e533c，原对照不重标为文档SHA；接手读取最新收尾CI。后续产品修复、打包部署或诊断窗口不因本次报告自动获批。

### 2026-10-29 第二轮：空Content-Type采集修正（代码CI与镜像验证通过）

- 服务器任务的已执行窗口记录：维护者另行批准更新d2b1f664并最多5分钟/新增100MiB观察自然Harness流量。北京时间15:45—15:50窗口取得12条完整请求，关闭触发为5分钟，含排空300.315秒，新增约76.36MiB；任务回读新镜像healthy、诊断false，旧容器/备份保留、主动模型请求0。本会话只读取执行输出，没有操作生产。
- 该窗口12条 `upstream_usage` 均为 `capture_state=unsupported_content_type`，采集点Content-Type为空。这是本版诊断的未采集边界，**不是上游usage缺失、null或0**；当时仍无法确定缓存计数首次变零的位置。d2b1f664的既有CI未覆盖这一生产边界，不以其全绿宣称真实上游取证已成功。
- 维护者已批准最小修正：仅响应Content-Type为空/空白时，从同一次实际传输已读取且完整、限内的JSON正文取得显式布尔stream，true选SSE、false选JSON。不能用客户端info.IsStream：非流式Chat在转换时也强制Codex上游stream=true。正文不完整/超限、stream缺失/null/类型无效时继续标记未采集，不猜测或补零。
- 非空响应头仍按原MIME选择解析；不改响应头、转发参数、计费、门禁或重试，不新建/恢复GetBody，不增加任何HTTP读取。默认开关、8MiB上限、脱敏/轮转及JSONL读取方式不变。产品只改 `middleware/upstream_diagnostic.go`，common观察器接口和channel发送路径不改。
- 回归保留原断言，扩展显式/空/空白SSE与JSON的usage整体缺失/null、cached字段缺失/null/0/正数及精确数值；真实路由夹具禁止net/http自动补Content-Type，覆盖流式与非流式Chat。增加未知stream及非空不支持MIME的未采集对照。全部仅Actions执行，浏览器不运行。
- **修正代码** `cd35f867d43b0a42d61500221bb9f99eb457e36a` / [完整CI40846159247](https://github.com/ForceMind/MyAPI/actions/runs/40846159247)，首次success，10作业成功、1浏览器混合作业跳过。亲自读取全部作业/关键步骤：根模块及linux/386构建、两模块测试、relaykit独立构建、既有竞态/三库/Redis/前端/分发等回归通过。定向13个顶层测试族各PASS一次，无FAIL或DATA RACE；真正去掉上游Content-Type的流式/非流式Chat各4种缓存状态共8个用例全PASS，未放宽原断言，浏览器未验证。
- **本轮候选镜像**：[Full+SQLite40846469827](https://github.com/ForceMind/MyAPI/actions/runs/40846469827)，首次success，producer attempt1，源码同cd35f867、tree `124c178ea45545471d42eaa4a4a45f77b2e37e4b`、linux/amd64，输入 `api_only=true, run_browser=false, personal=false`。producer/独立加载consumer成功，LAN跳过，20项通用HTTP/API、鉴权脱敏、精确扣费、重启/同镜像SQLite/WAL恢复均true，`browser=not_run`；空头专项证明来自上条真实路由合成回归，不把通用镜像检查当生产取证。
- **仅使用本轮构件**：[镜像包artifact12885307002](https://github.com/ForceMind/MyAPI/actions/runs/40846469827/artifacts/12885307002)，314183899字节，保留至北京时间2026-11-05 17:04；[脱敏报告artifact12884514813](https://github.com/ForceMind/MyAPI/actions/runs/40846469827/artifacts/12884514813)，2069字节，至同日17:05。可信manifest SHA256 `6c683e8609e07b7acd025c53d2520502df067090a57bc20302f2e26a231da7f8`；app bundle config/image ID `sha256:acbad68c99efd301e21e1d573a90ce8f29271ac7a0bf812da8182fbed9f5106e`。身份从producer日志和独立consumer核对；tar摘要在可信manifest内，目标Docker OCI ID若表达不同仍须证明config/layers/rootfs等价，不跳过校验、不换旧包。本机未下载构件。
- **交接边界**：本轮没有操作生产、部署、开启诊断或发真实模型请求。默认关闭、现有配置和数据不改；维护者后续生产操作另行确认。旧d2b1f664镜像不含本修正，下方第一轮构件仅为历史，不能用于新窗口。当前仍是0.2.0-beta.26开发基线，按源码SHA和构件身份区分，未发布tag/GHCR/NPM。
- 随后收尾只改本节及README/OPERATIONS/TASKS，应用和镜像仍绑定cd35f867，不重建或重标为文档SHA；接手读取最新收尾提交CI。修正经合成回归验证，尚未部署后自然流量验收；不宣称缓存根因已修，也不能追回上一轮未采集的原始响应。

### 取证背景与本批授权

- 只读关联同一 Harness 会话的10条连续请求：system/developer、50个工具及顺序相同，旧消息均为下次消息的完整前缀，首次变化只是从零基 `messages[658]` 开始正常追加。正缓存那条及前后两条同渠道，却返回 `0 → 172928 → 0`；不能用这组三条的开头改写或换渠道解释。
- 已有入口/客户端响应日志没有可关联的上游原始usage；无法还原过去某条真实请求。上述是服务器已有记录的只读证据，不是新模型调用，也不证明转换后的上游正文始终相同。
- 维护者批准本批只补诊断日志：默认关闭、保留缺失/null/0/正数，构建测试全部Actions，提交推送origin/dev并读取完整CI，仓库交接供另一会话部署。不得改生产、发模型请求、自动合main、发布镜像或tag。产品缓存与统计修复不在本批。

### 实现与读取约定（CI已验证）

- 开关 `MYAPI_UPSTREAM_DIAGNOSTIC_ENABLED=false` 默认关闭；还必须 `FULL_CONTENT_LOG_ENABLED=true`。无效布尔值也保持关闭；Playground继续省略内容，不安装上游观察器。当前仅Codex渠道HTTP路径接入。
- `upstream_request` 在现有HTTP传输读取请求时采集转换/适配之后的正文，不重读正文、不安装/恢复GetBody，不改变认证、参数、严格门禁、重试与扣费。记录原出站路径，不记录上游头、host、userinfo或query；正文按既有密钥字段规则递归脱敏。
- `upstream_usage` 在既有响应处理器读取HTTP客户端返回内容时，保留解析前原始JSON usage（HTTP传输解压之后，不是TLS/压缩字节）。只提取usage，不复制响应输出正文。SSE逐事件记录，前一正数不会被后一零或缺失覆盖；JSON响应也可提取。
- 沿用相同目录、`full-content-*.jsonl`、权限和全局轮转/保留策略。记录带原 `request_id`、`channel_id` 和同请求内递增 `upstream_attempt`；不提供账号凭据或账号指纹。当前页面/API仍只读取原入口/出口阶段，新增阶段需管理员直接读取JSONL，本批不改UI/API。
- usage记录的body含 `event_type`、`usage_source`、`usage_state` 和可选原始 `usage`。usage整体缺失为 `usage_state=missing` 且无usage字段；整体null为 `usage_state=null, usage=null`；对象为present，内部 `input_tokens_details.cached_tokens` 的缺失/null/0/正数保持原样。按原始JSON数值保留，不经float64取整，不将缺失补零；结构化脱敏会调整对象键顺序，不能用日志正文的JSON键顺序判断供应商分词。
- 每个请求正文、SSE事件或JSON响应额外采集上限8MiB。超限、无效JSON、读取失败/中途关闭等明确标记 `capture_state`，不输出未经脱敏的残片、不制造零用量。仅已观察字节可作为证据，未读部分不可推断；诊断记录不参与计费或用量转换。
- 写入沿用正文日志失败处理；不因日志写入失败更改HTTP返回。启用后增加日志量，保留策略保持原值，没有后台采样、自动开关或自动清理新策略。

### 最小验收与部署交接

- Actions：真实TokenAuth/Distribute/controller/Chat→Responses/Codex适配器到精确回环合成上游，逐条比较实际收到正文与日志；覆盖缓存字段缺失/null/0/正数、原始usage整体缺失/null、数值精度、SSE分块与多终止事件、脱敏、默认关闭、Playground省略、超限/异常、不重放、严格门禁；原回归断言保留。
- 本机只编辑和格式化代码、做轻量Git/文本检查；没有安装依赖、构建、测试、浏览器、Docker或下载产物。精确SHA与本批独立CI见下方，不使用历史全绿代替。
- 交给部署会话时应先核对本节的代码SHA与完整CI，再按维护者授权取得对应构件；旧cf6773f镜像不包含本批日志功能。版本名称不足以识别改动，须核对源码SHA和镜像身份。本批没有发布tag/GHCR，不更新镜像默认坐标。
- 本会话不部署或开启生产开关。部署会话须在对应授权下决定诊断窗口，显式传入新增环境变量，观察自然流量后关闭；不得为验证主动调用模型。Compose模板已透传新开关且默认false；其他启动方式由部署会话显式传入容器，不能只改宿主机.env却漏传容器。

### 提交与CI证据

- **代码与完整CI**：`d2b1f66450364a9299f676bfa519aedc692dc367` / [40828494736](https://github.com/ForceMind/MyAPI/actions/runs/40828494736)，首次success，10作业成功、1个浏览器混合作业跳过。亲自读取全部作业及关键步骤：根模块vet/build、linux/386构建、relaykit独立vet/build、两模块测试、全部后续竞态及数据库/Redis/分发/Desktop/前端检查成功。浏览器依赖安装及Chromium执行均skipped。
- **新增诊断与原门禁**：`Verify opt-in Codex upstream diagnostics and dispatch guards` 为success。亲自读取该job日志，8个新增诊断顶层测试族、4个原会话头测试族均各PASS一次，无 `--- FAIL` 或DATA RACE。覆盖真实路由→Chat转换→Codex适配器→精确回环上游的请求正文比对和四种缓存字段状态；只用合成输入与synthetic凭据，不是供应商缓存命中证据。
- **Full + SQLite镜像构建与独立加载**：[40828937555](https://github.com/ForceMind/MyAPI/actions/runs/40828937555)，producer attempt1，源码同d2b1f664，tree `99f2cf7ace0ddf28b9e8d554c73fb27253fe609a`，输入 `api_only=true, run_browser=false, personal=false`。producer/consumer成功、LAN安装跳过；20项HTTP/API、精确单次扣费、鉴权脱敏、7个静态资源HTTP读取、重启与同镜像SQLite/WAL恢复检查全true，`browser=not_run`。已读完整步骤和JSON报告，runner临时容器已清理。
- **镜像包**：[artifact12877962270](https://github.com/ForceMind/MyAPI/actions/runs/40828937555/artifacts/12877962270)，314183899字节，linux/amd64、Full，内含manifest.json、images.tar（app与固定Bun夹具镜像；部署只使用app）。有效期至北京时间2026-11-05 14:26。**脱敏报告**：[artifact12878195602](https://github.com/ForceMind/MyAPI/actions/runs/40828937555/artifacts/12878195602)，2074字节，至同日14:27。
- **可信身份**：producer日志中的manifest SHA256为 `54935e5073d3f0c99874a75a2c9e2da127f19956bd3ae0bd72e6cc4a7516ee65`；独立consumer验证通过的bundle app config/image ID为 `sha256:e8b7e5cd97082a6526f817ed4a01ec883b893705c61933af04f7322f393c3161`。tar哈希在已绑定manifest内，由既有bundle校验器核对；本机未下载或加载构件。不得以旧镜像、注册表回退或跳过身份校验代替此包。若目标Docker加载后ID表达不同，须先证明config/layers/rootfs等价并说明差异，未证明前停止，不自动改验证器。
- 本批不发布GHCR/tag/NPM，不更新VERSION或默认镜像坐标；镜像仍报告开发基线版本0.2.0-beta.26，**请按源码SHA和构件身份区分旧beta26**。本会话未部署、未改生产、未发真实模型请求。部署/开关窗口由接手会话按对应维护者授权处理。
- 随后交接收尾提交只改本节、README、OPERATIONS与TASKS，不重建镜像或把包重标为文档SHA；应用代码继续绑定d2b1f664，接手须读取最新收尾提交CI。UI线稿及原P1/P2继续保留，未获本批实施授权。真实上游缓存根因仍待自然流量取证，本批不是缓存修复。

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
