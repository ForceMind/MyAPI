# API Key 原 ID 单值轮换合同（beta.19 候选）

仅适用于已批准的个人控制台 API Key；控制台 Access Token、上游 OAuth 凭据和 writer 模式切换不在范围内。开发与验证使用隔离环境；当前候选不发布或部署。最终验收来源见本批交付记录，实施与失败修正见 [执行记录](PERSONAL_CONSOLE_ROTATION_EXECUTION_20261016.md)。

## 权限与输入

仅启用的 Root 真实会话可以维护本人 Token。三个接口均禁止 PAT 替代会话，使用无缓存响应；两个 POST 要求同源 Origin 和限流。提交另要求 `X-Security-Proof`，scope 为 `token.credential_rotation`，method 为 `2fa` 或 `passkey`。迁移 proof 不能替代轮换 proof。

不接受查询参数。POST body 上限 8192 bytes，拒绝重复键、大小写别名、null、额外字段及尾随 JSON。Token ID 必须为有效正整数；operation/revision 为 64 位小写十六进制。所有 JSON 编解码使用 common wrapper。

## 预检与维护卡

`POST /api/token/:id/credential-rotation/preview`，body 精确为 `{}`。成功 envelope 为 `{success:true,data:preview}`。

| 字段 | 合同 |
| --- | --- |
| `status` | `ready`、`blocked` 或 `unknown` |
| `preview_revision` | 仅 ready 时有可提交摘要 |
| `expires_at` | Unix 秒；`expires_at=(floor(db_now/300)+1)*300`，有效剩余时间大于0且不超过300秒，在锁内使用数据库时钟重验 |
| `credential_version_before/after` | 凭据版本；ready 时为正精确整数且 after=before+1 |
| `blockers` | 非 null 数组，元素为 `{code,state}`；未知证据不当作无在途或零用量 |
| `effects` | 仅 ready 返回，含 `token_id_preserved:true`、`old_secret_invalidation:"immediate"`、`accepted_settlement:"continues"`、`writer_changed:false` |

`GET /api/token/:id/maintenance` 的 `rotation` 为 `{status,commit_supported,blockers}`，仅 ready 时 `commit_supported=true`；preview DTO 没有该字段。资格专属普通查询错误使 rotation 局部 unknown，不隐藏已有 migration；非法输入、拒绝、不存在、取消和超时按原错误返回。

资格要求现有 authoritative writer、完整 backfill 证据及无未决旧 usage/settlement/refund/projection/batch/async 义务。缺 writer、回执或其他必要资格表、open/unknown 发送登记及无法识别的旧任务身份均关闭资格。legacy/bridge 不允许轮换；预检不切换模式、采样、刷新投影、清队列或修改历史。

## 提交、回执与未知恢复

`POST /api/token/:id/credential-rotation`，body 精确为：

```json
{
  "operation_id": "<64 lowercase hex characters>",
  "preview_revision": "<ready preview revision>",
  "confirmed": true
}
```

以上为字段格式示意，尖括号内容不能直接提交。新 operation 使用密码学随机标识。事务锁定 owner/Token 后重新取得证据和数据库时间，通过 CAS 才替换 Key、凭据版本加一并写唯一脱敏回执；同 operation 同请求重放不再换值，不同请求复用 operation 为冲突。版本不得超出浏览器可精确表示的整数范围；上界当前凭据仍可使用，进一步轮换关闭。

首次成功 data 为 `{receipt,replayed:false,reveal:{secret}}`；secret 格式为 `sk-` 加 48 字符原值。重放为 `{receipt,replayed:true,reveal:null}`。`GET /api/token/:id/credential-rotation/operations/:operation_id` 的 data 直接为 receipt，不含 secret。

回执字段：operation_id、state=committed、token_id、credential_version_before/after、masked_key、created_at、old_secret_invalidated_at、settlement_mode=authoritative_persistent、writer_changed=false。普通 ORM 的 Model/Table/alias 更新与删除均拒绝；raw SQL/数据库权限仍属于特权迁移边界。

| 失败类别 | HTTP / disposition |
| --- | --- |
| 非法输入、拒绝、提交时前置条件/锁内资格未知、revision冲突、已知阻断 | 分别400/403/412/409/409；`not_accepted` |
| 同 operation 不同请求或身份冲突 | 409；`unknown` |
| 数据库/缓存错误或提交后结果不可确认 | 503；`unknown` |
| 认证或全局限流在处理前拒绝 | 401/429等，路由边界标记 `not_accepted` |

服务错误同时返回 `data.commit_disposition` 和 `X-MyAPI-Credential-Rotation-Disposition`；部分中间件错误只有 header。预检的 blocked/unknown 是 HTTP 200 成功 envelope 内的资格状态；412仅指提交前置条件未知。disposition仅用于失败分类，成功以完整 200 成功 envelope及合法回执为准。客户端对超时、408/499/5xx、缺可靠标记或畸形响应保持 unknown，不自动重发、回滚或生成新 operation。回执404不能证明没有提交。

浏览器在 POST 前保存且回读非秘密的 actor/Token/operation ID，同一标签页内关闭并重开对话框，或刷新页面，只回读原 operation。关闭标签页、清除站点数据和跨设备不在恢复保证内。secret 仅首次成功自动显示于内存，关闭或身份变更清理；既有显式 Reveal Key 能力保留，不能把“未收到自动显示”解释为必须再次轮换。

## 发送与保留不变量

每次新发送前成功读取当前数据库版本，是该 attempt 的逻辑接纳点；此后即使物理发送晚于轮换，也属于此前已接纳工作。外层故障转移/新 attempt 重验版本，Realtime 旧连接禁止新工作帧，已接纳响应继续转发并按原 Token ID 收尾。不会物理撤回已接纳请求，也不把缺 usage 的成功回答改成失败或 Token=0。

authoritative 下仍走 legacy async 的入口先持久登记，和轮换共享 User→Token 锁；只有新建登记允许一次发送。open/unknown 不自动过期释放；落库任务按不可变提交身份链接，MJ使用新 DispatchTokenID 而不改写旧历史。

在 Token 行中仅改变 Key 与 credential_version，并在同一数据库事务内新增脱敏回执；凭据缓存/栅栏在提交前后分别更新。Token ID、owner、状态、预算、分额、模型/IP/routing 配置及账务历史保持不变。资格不覆盖混合旧版本 worker、生产集群或真实 provider；未来任何部署仍须单独授权并确保 worker 兼容版本一致。
