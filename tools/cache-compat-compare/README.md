# Cache compatibility comparison

本工具只取证，不修改 MyAPI 或 sub2api 产品逻辑，不是缓存命中率或生产验收。

## 输入与执行

- `fixtures.json` 是两边共享的纯合成输入：6个请求身份场景（首条、追加、显式缓存键、显式会话头、另一个调用Key、另一个上游账号）与8个SSE用量场景（标准正数/零/缺失/null、Chat明细别名、终态顶层usage、后一终态零/无usage）。
- MyAPI调用实际TokenAuth/分发/Chat→Responses/Codex发送路径，精确回环HTTP上游返回合成400，停在回复解析前，记录实际出站缓存键/session指纹；真实供应商地址一律由测试Transport拒绝。
- sub2api固定 `a3298b46ebe239fb0720b068a1eb3383955bdac2`，仅在Actions检出；把本目录的 `.go.txt` 测试添加到其service包，调用实际ForwardAsChatCompletions与包内已有HTTP recorder，不改任何上游产品文件。两边认证采用各自测试夹具，只比较相同payload和会话信号，不比较认证协议或TLS行为。
- MyAPI实际流式响应处理器与sub2api实际apicompat转换器分别处理相同事件；缺失/null/0/正数按实际输出记录，不在报告里补零。MyAPI开发分支可能已包含另一会话新增的字段存在性证据修复，报告绑定实际测试SHA，不能冒充生产cd35f867已部署该修复。
- `.github/workflows/cache-compat-compare.yml`只在dev推送本批指定对照源文件时自动执行，并提供workflow_dispatch入口，无浏览器；纯README/交接修改不重复触发对照。根模块常规CI仍会执行两项新probe，未设置报告目录时不写报告。
- 所有Go依赖安装、编译、测试均在GitHub-hosted runner；本机只编辑/格式化这些源文件，不克隆sub2api、不安装依赖、不运行probe或Python比较器。

## 报告语义

`comparison.json`/`comparison.md`记录双方源码SHA、同一输入文件SHA256、每个case实际结果和差异。工作流成功只表示有界合成对照执行完整、既有规范用量及不重放/网络隔离断言通过，**不表示所有兼容形状正确或产品已修复**（`product_fixed=false`）。

对于原始缺失/null但客户端输出0、原始正数未保留、显式0被省略，报告分别标明差异。没有修改或删除任何已有断言；非标准兼容形状先观测，再由维护者确认最小产品修复方案。禁止用取最大缓存值覆盖合法终态零。

只保存合成数据的计数、状态和指纹及测试日志，不输出凭据/正文，不发真实模型请求，不主动触发WHAM额度采样，不动生产，不合main，不发布镜像或tag。报告保留7天。

## 同轮额度采样只读调查

维护者反馈连接卡片/详情页“用量采样失败”。后台是默认每60秒GET WHAM usage，保存5小时/周额度快照，与单次请求cached_tokens不同。服务器只读截至北京时间2026-10-29约21:12：最新200行去重133批全部成功；24小时5326批成功、1批HTTP503失败；另5条usage_limit_reached属于模型转发限制标记，不能混称WHAM失败。

从生产同版代码看，连接窗口API保留失败/限额历史系列，页面仅筛选codex_rate_limit+percent，再将status=error统一标“Usage sample failed”；查询无refetchInterval且refetchOnWindowFocus=false。列表/详情的配置查询错误重试只重试各自查询，不是窗口刷新；业务状态error不等于窗口查询isError，因此窗口的网络错误重试按钮也不会因此出现。全局QueryClient也没有定时刷新。该路径能造成旧错误持续显示，但尚未读取维护者浏览器的实际API响应/缓存，因此不宣称已确认唯一页面根因。本轮不改UI/采样逻辑、不隐藏失败或补零；最小方案需另行确认。

## 对照过程中的修正

- 首份工作流40884590708在创建阶段失败：job.env不能用runner上下文，已改为受支持的github.workspace；未运行任何测试。
- 40884981577实际运行后，MyAPI探针通过；sub2api显式键场景的原样透传假设失败，实际得到确定性UUID。复核固定源码openai_codex_account_identity.go:181-188，该路径确实按API Key/账号namespace重写body缓存键。保留红证据，改为该固定输入/公开算法的精确golden期望，未改MyAPI显式键断言或上游产品。
- 同次归档路径仍指旧目录，导致没有上传报告；已统一为实际报告目录。观测器同时逐条保留客户端usage帧，读取最新帧前清除旧缓存值，避免把上一帧正数带入缺失字段。这是报告工具修正，不是转发或计费修改。
