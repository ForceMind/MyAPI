# R1：Key 请求诊断与正文预览

本轮由维护者批准：直接在 `dev` 完成 Key → 精确请求 → 诊断 → 已保存正文/SSE → 返回，提供双端可操作预览；只读诊断生产截图，并启用生产正文采集。不合入 `main`、不部署 dev 应用、不重放供应商请求、不改历史账务。

## 查看与操作

- 当前会话预览：[http://127.0.0.1:4182/__preview](http://127.0.0.1:4182/__preview)。这是仓库应用的真实前端构建，API 使用明确标记的合成数据，绑定本机，不连接生产或供应商。会话结束后不保证进程持续运行。
- 重新运行：在 `web/` 执行 `bun run build`，回到仓库根目录执行 `node tools/ui/key-request-preview.mjs`。只读预览仅接受合成登录刷新，业务写入和生成请求均拒绝。
- 从 Key 卡片选择“查看 Key 请求”。两个 Key 名称相同，但 #21/#22 的列表、分页、统计按已保存 `token_id` 区分；改名前名称保留在其原请求上，无 ID 的历史记录不会猜测归属。
- 请求详情 → 查看请求内容：流内上下文错误、已完成但用量未知、未采集正文分别展示。打开已保存内容不会重新发送请求；响应标签和原始数据开关可以查看 SSE。
- 关闭正文和诊断保留请求位置；返回 Key 恢复筛选和选中项焦点。Key 摘要按 ID 读取已脱敏元信息，不读取明文密钥。

## 来源和验证

应用与浏览器验证源码均为 `66f75dd9ce5b32fe09446ec47874c79170487029`，版本 `0.2.0-beta.26`。最后一批修正是将 Key 范围提示移入布局组件实际渲染的内容区，并增加可见性和原始 SSE 交互断言。构建与归档提交分开，不以文档 HEAD 冒充被构建源码。

- [构建 manifest](build-manifest.json)：205 个制品文件 SHA256；[运行指纹和手机 SSE 回读](preview-readback.json)。4182 使用冻结构建目录，不受后续 `web/dist` 重建影响。
- [Chromium 报告](key-request-qualification.json)：英文 1280、中文 390、俄语 320 三条实际点击旅程全部通过。验证精确 ID、同名/无 ID 排除、分页、成功但用量未知、HTTP 200 流内错误、缺失正文、返回筛选/焦点、Key 摘要。未产生外部请求、业务写入、明文 Key 读取或页面运行错误。
- [定向 Go 测试](go-targeted.log)：model/controller 的 Key 身份、统计、自助权限、账户改名、非法 ID 回归通过。[原生 MySQL 5.7/PostgreSQL 9.6 的 Key 统计合同](native-databases.txt)也通过，SQLite 由定向测试覆盖；对应后端输入与最终源码相同。读取条件使用 GORM 绑定参数，无迁移、无结算写入。
- [最终 TypeScript 检查](typecheck-visible-scope.log)、本轮 30 个 TS/TSX 文件 lint、i18n 同步与生产构建通过。11 个新增文案已补齐七语言。
- 相关前端扩大回归 62 文件/447 项：446 通过，一项仍使用旧的按名称返回 Key 断言。更新为稳定 ID 后，该文件与正文权限/过期缓存回归共 [11 项通过](frontend-final-fix.log)；其他 61 文件的结果保持适用，不重复运行未变输入。
- [预览合成 API 的身份、缺失正文和只读合同](preview-fixture-final.log)：3 项通过。
- 最终 Key 范围修正的[8 项工作区回归](visible-scope-tests.log)通过，准确构建后的三种屏宽浏览器再次通过。
- 前一候选 `85b5af4` 的[CI 快照](ci-candidate85-snapshot.json)：全量前端 Test、typecheck、lint、构建和原生数据库作业已成功，其余浏览器/后端步骤尚未全部完成。此后 `66f75dd` 只改上述显示位置及两份验证文件；不能把旧快照称为最终源码全量通过。
- 最终源码完整 CI：[39807929357](https://github.com/ForceMind/MyAPI/actions/runs/39807929357)，交付时仍在运行；本批只为纯文档归档提交跳过重复 CI，不跳过应用候选的检查。
- 前一候选发行作业的[实际失败](ci-distribution-failure.txt)是 `handoff-upgrade-smoke.test.mjs:122` 仍断言已删除的 `codex/` 分支，而 workflow 已改为 `dev`。另有已知[安装器合同失败](installer-known-failure.txt)：`installer-workflow.test.mjs` 将安全 JSON 报告使用的 `upload-artifact` 一并禁止，本机直接运行 7 通过/1 失败。两份测试及 Docker workflow 与本轮起点相同，本批不扩大修复或放宽断言；原版本默认值不同步问题随 beta.26 对齐消除。不能将完整仓库 CI 宣称为绿色。

主代理已查看桌面正文、中文手机列表/正文/返回、俄语窄屏缺失正文的原始截图；不是独立审查。中文浏览器最初缺系统字体，安装 Noto CJK 后重新验收和截图；原始错误中的英文保留上游原文。

## 生产错误与日志开关

四次截图请求的生产后端日志均有明确的上游流内错误：输入超过该模型上下文窗口。HTTP 200 已在 SSE 错误之前发送，随后 `handler_stop`，关闭读取产生的 `http2: response body closed` 是后续现象。内部渠道错误标记不能当作上游 HTTP 500 的证据。

这些请求缺少可信 usage，进入 `usage_pending_review`。预留额度不是已确认的实际消费；本轮没有补写用量、人工结算、退款或重放。处理输入应缩短上下文，不能把自动截断或切换模型作为本批隐含行为。

已按授权启用生产正文采集，并再次[只读回查](production-final-readback.json)：

```text
FULL_CONTENT_LOG_ENABLED=true
FULL_CONTENT_LOG_MAX_MB=100
FULL_CONTENT_LOG_MAX_FILES=5
```

容器仍名 `my-api-beta25`，仍是原镜像 `sha256:cc4d34c65650bca04898a515348bff926a779b52408b1c3117ccd6bd4dcc9ad3`；公开 status HTTP 200、版本 beta.25、健康。其他环境、挂载、资源、账号/Key/渠道稳定身份已比较保持。正常停止、一致备份和同镜像重启约 5.42 秒；原停止容器及私有备份保留在主机，配置、数据库、真实 Request ID、用户名和正文不进入仓库。

启用只覆盖以后的正常请求，历史未采集正文不能补回。最终目录仅做元数据统计，尚无自然请求生成的正文文件；**未验证实际新正文落盘**，没有为此产生供应商流量。后续正常调用后可在生产正文页检查，无需重放旧请求。日志开关的本次运维变更不等于 beta.26 部署。

## 代表截图

[桌面流内错误](key-request-stream-error-en-1280.png) · [手机精确请求](key-request-requests-zh-390.png) · [手机错误诊断](key-request-stream-error-zh-390.png) · [手机原始 SSE](key-request-sse-raw-zh-390.png) · [成功但用量未知](key-request-successful-unknown-usage-zh-390.png) · [返回 Key](key-request-returned-key-zh-390.png) · [窄屏缺失正文](key-request-missing-body-ru-320.png)

本轮之外：生产 beta.26 部署、完整平台视觉接受、其他模块重构和真实供应商专项仍需各自范围授权。
