# beta.21 隔离验收控制接口与操作边界

适用范围：本批候选实现；执行记录见 [实施记录](PERSONAL_CONSOLE_ACCEPTANCE_CONTROL_EXECUTION_20261019.md)，前提见 [OAuth 前置核查](PERSONAL_CONSOLE_OAUTH_PREFLIGHT_20261019.md)。本文件是隔离验收说明，不是生产启用、真实账号接入或部署授权。

## 启用与身份

进程环境变量 `MYAPI_UPSTREAM_ACCEPTANCE_ENABLED=true` 显式启用，默认关闭，接口不能启用它。只用于独立测试进程、临时数据库及合成凭据。当前阶段尚未授权真实测试账号。

控制接口要求 Root 维护者的真实会话，拒绝 PAT、普通 API Key 和普通管理员。两个 POST 沿用既有控制台来源校验：匹配服务地址或配置的可信 Origin，缺少 Origin 时允许合法 Referer 回退；两者皆缺失则拒绝。单个 run 只绑定一个既有、单凭据 Codex channel/account；保存脱敏账号引用和随机进程 owner，不保存凭据、邮箱、URL 或响应内容。run 不绑定某个浏览器 SID，其他合格 Root 会话也可查看、停止当前进程的 run。重启后读取旧 run 不会取得其派发资格，也不自动恢复任务。

## 接口

所有成功响应使用现有 `{ "success": true, "data": ... }` 格式。

| 方法与路径 | 输入 | data |
| --- | --- | --- |
| `POST /api/upstream-acceptance/runs` | 下方 JSON，最多 8192 bytes，重复或未知字段拒绝 | 新建 run，所有计数为 0 |
| `GET /api/upstream-acceptance/runs/:id` | 有效 Root 维护会话 | `run`、`owner_current`、`operations`、`http_in_flight`、`can_dispatch`、`drain_error` |
| `POST /api/upstream-acceptance/runs/:id/stop` | 有效 Root 会话及通过来源校验 | 当前停止与排空状态 |

示例仅表示显式预算，channel 1 必须是当前隔离数据库中的合成账号：

```json
{
  "channel_id": 1,
  "limits": {
    "duration_seconds": 600,
    "max_total": 12,
    "max_auth": 1,
    "max_refresh": 2,
    "max_sample": 4,
    "max_models": 4,
    "max_generation": 1
  }
}
```

期限为 1–1200 秒；总预算 1–1000；各分类预算必须在 0 与总预算之间，生成预算还限制为最多 10。分类为 0 表示禁用。分类预算之和可以大于总预算，总预算仍是共同上限。这里的数字是本工具的隔离安全限制，不能解释为提供商配额。

## 请求计数与停止

每次通过最终派发栅栏的尝试消耗一次总预算及对应分类预算，HTTP 失败、429、应用层重试不返还计数。预先拒绝的请求不会占用预算；已经取得预算而在实际网络发送前取消，也可能被保守计数，因此计数是尝试次数上界，不能当作提供商成功请求数。OAuth 授权页的浏览器导航不属于 MyAPI 出站 HTTP 统计。

分类覆盖换码、refresh、WHAM/reset-credit GET、模型查询及其 GitHub client-version 查询、Codex HTTP 生成。验收 client 禁用重定向、keepalive、HTTP2 和可重放 body，避免透明重试；不修改共享 client、来源资格或普通模式。最终栅栏再次校验账号、当前凭据、精确目的地址/路径/方法/必要请求头以及预算。生成并发为 1，响应 body EOF、读失败或关闭才释放 HTTP 槽，逻辑 operation 仍保留到处理器收尾完成。

停止先锁存拒绝新请求并取消当前 context，再持久化停止状态。验收控制自身的数据库操作分别受 5 秒 context 限制；排队等待当前栅栏和各阶段耗时不能称为立即停止。已成功取得旋转凭据的 refresh 会在独立的 5 秒有界 context 内尝试安全落库，停止状态必须等待该逻辑 operation。结果未知的已发送请求保留原有待核查/退款保护；最终栅栏明确未发送的请求才允许原有未发送清理与退款。

`stopping` 是停止已受理而尚未排空。确认正常手动结束须同时满足 `status=stopped`、`owner_current=true`、`operations=0`、`http_in_flight=0`、`generation_in_flight=0`、`drain_error=false`。期限到达后的 `expired` 也必须核对同样的排空计数；它本身不证明收尾结束。`can_dispatch` 只是总体运行状态提示，具体分类、凭据和网络策略仍由最终栅栏决定。

最终预算 claim 发生存储错误，或生成槽释放、停止持久化失败，会使当前进程停止准入并标记排空失败；前置 channel/status/operation 读取失败通常只拒绝本次操作，不将全部读取错误描述为永久锁存。不允许把失败冒充正常结束；保留 run ID 与脱敏诊断，人工查明原因后重新建立隔离进程和新的显式 run。旧 owner 的 run 没有恢复发送接口。

## 当前支持范围

验收模式暂不接受新建 channel=0 OAuth、reset-credit consume、WSS/Realtime 和渠道诊断生成；自动刷新、Codex 后台模型更新和配额采样调度暂停，需使用绑定 run 的明确手动流程。其他 provider 的请求处理与普通模式沿用现有行为；验收进程整体暂停配额采样调度与 legacy updater，因此也暂停其对其他 provider 的后台采样。本机制只控制已列出的 Codex 路径，不是全进程或跨集群防火墙。账号原生窗口和 Key 分额仍是现有近似口径，历史账务及 writer 不变。新 SIWC 入口不在本批。

## 本地控制脚本

`tools/runtime/upstream-acceptance.mjs` 只接受明确的 `http://127.0.0.1:端口` 或 IPv6 literal loopback；拒绝远程地址、localhost、重定向和隐式登录。会话 JSON 文件必须只允许 owner 读取（0600），含 `access_token` 字段，且不进入仓库、日志或截图。示意命令中的文件均应由隔离 fixture 创建：

```bash
node tools/runtime/upstream-acceptance.mjs create --base-url http://127.0.0.1:4181 --session-file /tmp/synthetic-owner-session.json --spec-file /tmp/synthetic-acceptance-spec.json
node tools/runtime/upstream-acceptance.mjs status --base-url http://127.0.0.1:4181 --session-file /tmp/synthetic-owner-session.json --run-id RUN_ID
node tools/runtime/upstream-acceptance.mjs stop --base-url http://127.0.0.1:4181 --session-file /tmp/synthetic-owner-session.json --run-id RUN_ID
```

脚本不发送推理、不自动重试、不接入账号，也不将 `stopping` 改写成 `stopped`。每次操作超时 10 秒；超时不能推断服务器未执行。已知 run ID 时使用原 ID 查询；create 超时且尚未取得 ID 时不继续业务请求、不盲目重试创建，保留现场并核对隔离数据库中的脱敏 run 记录，取得准确 ID 后再查询或停止。

## 验收与部署关系

本批用合成凭据、fake transport 和 loopback 实际 HTTP 路由证明预算、范围、计数与停止；GitHub Actions 补齐三库、race、完整构建和既有页面回归。真实 OAuth 成功、原生窗口一致性及提供商 429 尚需独立账号资料和授权。候选通过不等于已经部署：还需目标实例只读兼容核查、原生备份恢复演练和准确制品/目标/维护窗口的单独部署批准；历史账务继续冻结。
