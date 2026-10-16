# 高级维护接口与验收合同

本批候选 beta.17，承接 beta.16，不含账务迁移、凭据轮换或跨账号迁移。浏览器状态测试使用合成 API；实际 Go 集成使用 Actions 中独立 SQLite 实例和回环上游。

## Key 模型范围

`GET /api/token/:id/model-limits` 返回 `{success, message, data}`；data 仅有 `token_id`、`token_name`、`model_limits_enabled`、`models`（原样字符串数组）及 `revision`（64 位小写十六进制、不透明内容指纹）。GET/PUT 均 `Cache-Control: no-store`。当前启用用户只可维护自己的 Key，Root 不绕过 owner；不存在和非 owner 都返回 404。

`PUT /api/token/:id/model-limits` 请求恰好三个字段：

```json
{"expected_revision":"<GET 返回的 revision>","model_limits_enabled":true,"models":["example-model"]}
```

接口拒绝额外字段、非规范字段名、重复 JSON 键、缺失/null 类型、重复模型、超出 TEXT 65535 UTF-8 字节容量。新增模型 ID 不含空白、控制字符、BOM、逗号或空字符串；保留的旧 ID 原样处理，不静默规范化。旧重复项需要使用者显式移除。

- `false + list`：无额外模型限制，保留列表以备以后启用。
- `true + nonempty list`：仅选定模型，仍受路由、访问策略和上游能力约束。
- `true + []`：拒绝全部模型调用，UI 必须显式选择并确认。

候选目录仅帮助选择，不保证模型可路由。保存不会从目录中删除已有/退役模型，也不扩大其它权限。事务在现有用户/Token 行锁与缓存 fence 下检查当前元数据指纹，只写模型限制两列；额度消耗不导致表单冲突，也不被覆盖。普通元数据更新省略模型字段时不写对应列，status-only 仅写 status；额度编辑保留完整 CAS 语义。无 schema 变更。

400 为无效输入，403 为用户禁用，404 为不可访问 Key，409 为指纹冲突，503 为不可确认/不可用。503 可能在数据库提交后发生，客户端不可自动重放。冲突/未知结果需要读取当前状态；读取失败后即使编辑表单也不能解除写锁。成功保存同样 GET 回读后才关闭。

## 原生窗口维护

沿用现有 share API、Root 权限、revision 和操作 ID。不增加迁移端点。重选仅允许服务端验证同订阅主体、同原生 slot 的样本；channel 选择只过滤样本连接。重选新 descriptor 开启新 generation，保留原 Key 比例和历史，不继承当前估计；恢复保留 descriptor/generation/估计，未决调用仍有不确定性。影响是全池，需双确认；关闭池是独立操作。

现有 Key view 不包含停止前的旧样本或恢复样本实际 channel；复核卡明确显示 Unknown，不从当前配置连接推断。网络未知/401 不刷新认证后重放，显式回读前保持写锁。

## 安全维护

沿用现有密码、绑定、Passkey、2FA、备份码、会话和控制台 Access Token API。打开安全面板不生成 Token。生成属于显式替换操作，要求确认；提交中不可关闭丢弃一次性返回，成功后关闭清除显示。网络失败不会被解释成服务端回滚，旧 Token/备份码可能已失效。Root 自助删除不可用并解释原因。

请求按 user/role/session SID 及操作 generation 隔离；账号切换、会话替换、离开或新流程启动后，旧响应不得显示秘密、反馈或继续提交。同 SID 合法 auth rotation 保持可用。OAuth callback 还须匹配当前待处理流程。状态读取失败显示可重试状态；未知非秘密操作回读已有状态。

## 验收证据边界

七语言 320/1280 与中文 390/1440、暗色和长文场景由实际 production build 的 Chromium 执行。窗口、模型、安全各有独立报告与原始 PNG。合成状态测试不能证明真实服务商可用性；Passkey 以 CDP 虚拟认证器执行，不能外推为实体设备验证。

隔离 Docker personal 旅程通过真实浏览器、真实 Go 和临时数据库，验证两次模型范围窄写后恢复默认、Root 跨 owner GET/PUT 404、额度/元数据及上游计数保持。该步骤不额外发送 relay；既有 relay 成功与 usage 缺失语义由现有隔离测试保留。最终提交号、运行 ID、测试计数和截图索引另记交付记录。
