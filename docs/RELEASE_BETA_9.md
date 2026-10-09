# v0.2.0-beta.9 source release candidate

## 当前发布与部署执行（2026-10-09）

本节覆盖下方保留的 Draft 发布前状态。用户已明确授权将现有 Full 实例从 beta.7
部署至 beta.9；本批不改变应用源码、访问策略或账务模式，不发布新版本/NPM，
不移动发行标签或 stable/latest，不进行真实 OAuth 或付费模型请求。

- [GitHub prerelease](https://github.com/ForceMind/MyAPI/releases/tag/v0.2.0-beta.9)
  已存在，非草稿；不可变发行源码为 `c45df8c6c6bf2e66d39b58b8c2bc8d61984f901b`。
- [发行资产工作流](https://github.com/ForceMind/MyAPI/actions/runs/37913741903)和
  [镜像工作流](https://github.com/ForceMind/MyAPI/actions/runs/37913754507)成功。
  四份二进制与三份 checksum 文件均核验 SHA256，二进制同时匹配随附校验清单。
- Full digest：`sha256:84d3144651aa6525c7ba1f0db9dfc1fbb9a5a19b3c448307e7960dd5dda52ada`。
- LAN digest：`sha256:64b42bde6c982a87f5866420a523ebb35a1b5d94506f651b5810d83b3e66520e`。
- Full/LAN 均包含 Linux amd64/arm64，四架构版本、edition 和源码标签一致；
  两份多架构清单与四个架构发行根的 cosign 签名均核验通过。证书主体为
  `https://github.com/ForceMind/MyAPI/.github/workflows/docker-build.yml@refs/tags/v0.2.0-beta.9`，
  OIDC issuer 为 `https://token.actions.githubusercontent.com`。
- [准确发行源码 CI](https://github.com/ForceMind/MyAPI/actions/runs/37911737178)
  原运行取消，本次补跑十项全部成功；原取消状态不作为通过证据。
- 隔离 master 副本的 beta.9 升级和另一个原快照副本的 beta.7 恢复均通过。
  采用断网、0.5 CPU/768 MiB 与生产相同批量账务参数；26 张保护表核对通过，
  没有新增/删除表或字段，没有自动分配访问约束。

执行结果：准确源码 CI 十项全部成功；现有 Full 生产实例已从 beta.7 升级至 beta.9。
生产停机保存完整数据目录（含 SQLite/WAL）、原 Compose/环境文件和独立数据库基线后，
只切换上述 Full 固定镜像 digest。本地和真实 HTTPS 首页、`/api/status` 均返回 HTTP 200，
运行版本一致为 `0.2.0-beta.9`，容器健康。SQLite 完整性、历史用户/Key/渠道/日志身份、
既有访问约束、预算/未知状态和持久账务核对通过；没有自动分配新约束或预算。
完整备份与文件 SHA256、私有权限核验通过；原环境文件字节、密钥、挂载和运行限制保持。
升级、原快照恢复及生产均仅观察到符合原契约的既有失败重试运维字段更新；
错误仍为缓存不可用，经济字段、身份、状态与应用标记逐行保持。
本批未修复这些既有失败重试，也没有执行生产回滚。
必须保留原环境文件字节、会话秘密、完整身份密钥环、挂载、监听和资源限制。
旧程序不能打开已迁移数据库；恢复须使用升级前数据和配置。

Playground 现在必须显式选择已有自有 Key，请求按原 API 权限和计费流程执行。
合成验证不代表真实上游图片/PDF、账单或付费调用已验收。
私有备份与报告留在部署主机，不提交生产域名、私有路径、密钥、数据库或原始日志。


## Historical source-candidate status and authorization

This is the beta.9 Draft source candidate, not a published release, image, or
production-upgrade instruction. The latest published prerelease remains
[v0.2.0-beta.8](https://github.com/ForceMind/MyAPI/releases/tag/v0.2.0-beta.8)
(published 2026-10-08). Work and exact-head evidence are in
[Draft PR #4](https://github.com/ForceMind/MyAPI/pull/4).

`VERSION`, the root package, and root/deploy default Full image references name
`0.2.0-beta.9`. Those defaults are candidate metadata, not proof that the image
exists. README installation examples deliberately pin the published beta.8.
Do not deploy the beta.9 candidate defaults before separately authorized
publication and verified artifact readback. Existing beta.8 artifacts stay unchanged.

Normal scoped fixes, tests, documentation, commits, pushes, and Draft PR upkeep
are authorized. Main merge, a new tag, GitHub prerelease, GHCR/NPM publication,
production deployment, credential expansion, and the user's computer are not
included. Preserve protected workflow/environment gates; no force pushes,
tag replacement, alternate publishing workflow, or stable/latest promotion.

## Included journey

1. Open Playground and explicitly choose an existing Key owned by the signed-in
   user. No automatic first-Key selection, temporary unlimited token, or browser
   credential disclosure. Model discovery follows that selected Key.
2. Attach supported images/PDF, inspect or remove the draft, and send once through
   the existing shared authentication, routing, admission, Relay, and usage path.
   Unsupported media or policy returns an error; it is not stripped or replayed.
3. Stop cancels the active request without automatic resubmission. Failed/cancelled
   drafts remain available for an explicit next action. After reload, attachment
   history has an explicit unavailable marker rather than sending degraded text.
4. Open **Usage Logs → Common** (`/usage-logs/common`) to inspect the selected
   Key's record; the existing Token Name column/filter identifies it. Backend
   HTTP/relay tests prove selected `TokenId`, quota/log attribution and an unchanged
   unselected Key; synthetic browser logs validate only the display/navigation.

Seven-language mobile controls show the entire selected name/group/status in a
wrapped readout with a full-width 44px selector. Key identity remains readable
while controls are individually disabled. An available Stop control is not dimmed
because another descendant is disabled.

## Actual support matrix

| Input / capability | beta.9 boundary |
| --- | --- |
| Ordinary text | Existing selected-Key/model/provider policy applies. No new provider capability is promised. |
| PNG/JPEG/WEBP/GIF | Inline `image_url`; actual type-1 OpenAI-compatible and type-57 Codex paths only, checked on each attempt. The upstream model must support images. |
| PDF | Inline `file_data` through an actual type-1 OpenAI-compatible adapter only. Existing Chat→Responses mapping preserves `input_file`; upstream model support is still required. |
| Attachment limits | At most four files across conversation history and draft, 10 MiB each, 20 MiB decoded aggregate; 32 MiB server envelope. Signature, filename, role and unambiguous protocol keys are validated. Smaller deployment and existing 1 MiB assigned-access proof limits remain authoritative. |
| Strict Token/USD budgets | Media is rejected. For an explicitly selected strict Key and exact `gpt-6.1-sol`, the client emits the existing strict native-Chat envelope: required `max_completion_tokens` (1..128000), `stream_options.include_usage=true` only while streaming, and `service_tier=default`. Incompatible sampling controls are visibly disabled and ordinary-Key preferences are preserved. The server still requires its existing official type-1 route/model/funding/price qualification; fee-qualified routes also need their existing service-tier configuration. Unsupported combinations fail closed with accurate guidance; no Key fallback or broader protocol qualification. |
| File persistence | Bytes remain in memory; no attachment bytes in localStorage or diagnostics. Text/history metadata is owner-scoped. Legacy ownerless history is retained but not automatically loaded or assigned. |
| Not included | `/v1/files` upload/read/download/delete lifecycle, retention, upstream file-ID mapping, remote file URLs, opaque `file_id`, TXT/DOCX, media generation, arbitrary uploads, new billing, or new credential management. |

Payload-preservation tests use synthetic upstreams. They do not prove live
provider/reseller acceptance, actual invoices, production migrations/rollback,
physical-device native-picker behavior, or production readiness of stable 0.2.0.

## Evidence and remaining release gates

The reviewed implementation checkpoint
[`f008dc79eef64e31b0e8c7f30140f8ca797b6336`](https://github.com/ForceMind/MyAPI/commit/f008dc79eef64e31b0e8c7f30140f8ca797b6336)
passed [all ten CI jobs](https://github.com/ForceMind/MyAPI/actions/runs/37895495612)
and [website checks](https://github.com/ForceMind/MyAPI/actions/runs/37895495717).
Its explicit Playground Token SQLite/MySQL 5.7/PostgreSQL 9.6 subtests all passed.
Frontend was 178 files/1007 tests; root and independent relaykit checks passed.
Clean-checkout `release:check` included source-manifest and 2928-file pack validation.

That checkpoint's [browser artifact](https://github.com/ForceMind/MyAPI/actions/runs/37895495612/artifacts/11600308724)
contains 8 Playground journeys/24 PNGs/48 identity measurements plus 19 shared UI
journeys/42 settings deep links/171 PNGs. All seven 320px languages and desktop
1280px were inspected; identity opacity was 1 and contrast 12.79:1. Native touch
is dispatched to the control; native option selection uses Playwright
`selectOption`, not physical-device OS picker automation.

The closeout source adds actual browser multiple-image, invalid-file,
remove/Stop-cancel and owner usage-log inspection scenarios, strict-text client/server parameter alignment with accurate rejection wording,
synchronized docs, and the final head's own applicable checks.
The final PR evidence must name that head and artifacts. Earlier green results
never substitute for the subsequent commit, and a skipped Docker smoke does not
count as a pass. The card's stopping condition is not satisfied until these
closeout checks succeed.

After separate publication authorization:

1. Merge only the approved exact candidate, qualify the resulting main commit,
   and use a new `v0.2.0-beta.9` tag with matching version/source identity.
2. Use existing `release.yml` and protected image workflows for the already-defined
   platform assets and Full/LAN images. Verify prerelease/not-latest status,
   filenames, checksums, immutable image digests, signatures/provenance and source SHA.
3. NPM publication requires its own explicit inclusion and package verification;
   `npm pack` validation is not publication.
4. Return artifact links and pause before actual deployment. Real account/billing,
   installation/upgrade/rollback and production acceptance remain separate evidence.

Persistent Files API lifecycle or any later roadmap node needs a separate frozen
scope. Do not silently expand this candidate into it.
