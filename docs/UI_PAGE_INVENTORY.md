## beta.9 Playground extension (2026-10-09)

The beta.8 inventory below remains historical shared-UI evidence. Current beta.9
adds explicit owned-Key selection, bounded media, owner-scoped draft/history,
readable seven-language mobile identity and the existing Usage Logs path.
The additional real-browser coverage and exact candidate evidence are recorded in
[the beta.9 delivery card](BETA_9_DELIVERY_CARD.md),
[release candidate record](RELEASE_BETA_9.md) and [Draft PR #4](https://github.com/ForceMind/MyAPI/pull/4).
No live provider billing, persistent Files API or production acceptance is implied.

# MyAPI beta.8 page and task inventory

Audited from `1bd48522b85e91929b6a7de9a78f142a957c6147`, not a proposed backend.
Shared shell/component migration and the main operational page checks are now
implemented. The exact-source checkpoint is documented in
[BETA_8_DELIVERY_CARD.md](BETA_8_DELIVERY_CARD.md). This inventory does not claim
that every optional route and every business operation has browser acceptance.

All families below have applicable synthetic UI qualification at source
`db7864049b5691ae4d2b222a896067333808edc4`: four Chromium suites, 19 shared journeys,
42 settings links and 234 screenshots. See the journey matrix below. This is UI
qualification, not live provider, account, bill or deployment acceptance.

| Family | Existing entry points and task | Gates and preservation contract |
| --- | --- | --- |
| Overview and analysis | `/dashboard/{overview,models,flow,users}`; account evidence, activity, model/user analysis | User analytics remains admin-only; chart evidence and user-scoped guide preferences stay intact |
| Channels and accounts | `/channels`; CRUD, account windows/charts/events, model discovery, mapping and routing preview | Existing channel resource permissions; no summed unrelated quotas, credential exposure or invented availability |
| Model catalog | `/models/{metadata,deployments}`; metadata and existing optional deployment UI | Admin-only, existing deployment enablement/authorization; no new deployment capability |
| API Keys | `/keys`; CRUD, reveal, limits, budget and assigned-access readback | Owner privacy, stable action menu, existing assigned-access/budget semantics |
| Users | `/users`; management, admin scopes and assigned user/Key access | Role hierarchy, CAS, inheritance/empty/disabled semantics and safe conflict refresh |
| Usage and recovery | `/usage-logs/{common,drawing,task}`; filters, details, attempts, budget evidence and pending review | Own/all scope, Root recovery authority, unknown holds and idempotency unchanged |
| Content logs | `/full-content-logs`; request records | Existing admin/resource authorization and redaction |
| Profile | `/profile`; identity/security/preferences | Existing password, passkey, OAuth and user preferences |
| Funding history | `/wallet` | Commercial disabled/retirement modes retain history; no enabled purchase controls without capability |
| Optional administration | `/subscriptions`, `/redemption-codes` | Existing guarded history/recovery, self-use visibility and funding gates |
| Existing tools | `/playground`, `/chat/$chatId`, `/chat2link`, `/prompt-learning` | Preserve current feature gates and request behavior; no new model calls or learning functionality |
| System information | `/system-info`; build revision, instances and tasks | Root-only read/operation boundaries remain |
| Site settings | `/system-settings/site/{system-info,notice,header-navigation,sidebar-modules}` | Root-only; keep configured content and navigation preferences |
| Authentication settings | `/system-settings/auth/{basic-auth,oauth,passkey,bot-protection,custom-oauth}` | Root-only; credential/auth behavior remains unchanged |
| Billing settings | `/system-settings/billing/{quota,currency,model-pricing,group-pricing,quota-writer,payment,checkin}` | Root-only; existing formula, money, quota writer and optional-commerce gates |
| Model settings | `/system-settings/models/{global,routing-reliability,openai-pricing-source,gemini,claude,grok,channel-affinity,model-deployment}` | Root-only; no route/price changes caused by presentation; existing optional deployment gate |
| Security settings | `/system-settings/security/{rate-limit,sensitive-words,ssrf,token-limits}` | Root-only; no expansion of permissions or access |
| Content settings | `/system-settings/content/{dashboard,announcements,api-info,faq,uptime-kuma,chat,drawing}` | Root-only and current minimal-build hiding |
| Operations settings | `/system-settings/operations/{behavior,alerts,email,worker,logs,performance,update-checker}` | Root-only; update checker shows release details, it does not install; no enabled external alert target |
| Setup | `/setup` | Existing first-install guard, validation and completion; no new installer |
| Authentication | `/sign-in`, `/sign-up`, `/register`, `/forgot-password`, `/reset`, `/user/reset`, `/otp`, `/oauth`, `/oauth/$provider` | Preserve redirects, session/OTP/OAuth/passkey semantics and interrupted journeys |
| Errors | `/401`, `/403`, `/404`, `/500`, `/503`, `/errors/$error` | Correct states and useful safe recovery/navigation |
| Existing public pages | `/`, `/about`, `/pricing`, `/pricing/$modelId`, `/rankings`, `/privacy-policy`, `/user-agreement` | Shared design consistency only; preserve custom content, guards, notices and optional commercial behavior; no marketing expansion |

## Shared migration surfaces

- `components/layout`: application/public shells, contextual navigation, page
  frame, header and pagination footer
- `components/ui`: existing Base UI primitives, cards, tables, forms, dialogs,
  notices, empty and loading states; token-driven light/dark presentation
- `features/system-settings/components`: common settings frame, section and
  save/error behavior, preserving all 42 registered sections
- `i18n/locales`: en, zh, zh-TW, fr, ja, ru, vi, using repository translation tools

## Specific constraints discovered in the current UI

The root sidebar currently puts chat tools before operational tasks. System
Settings is visible to ordinary administrators although its route is Root-only.
Channels stacks evidence, events, preview and its table; reorganizing hierarchy
must not remount or reset active account/preview state or break old quota links.
The shared section title truncates long text, and the skip link needs a reliable
focusable main target. These are explicit behavior checks for the shared batch.

The beta.7 320/1280 access/event screenshots were inspected during the initial
audit. They show existing production components with synthetic data, not a
beta.8 design or real-account validation. Fresh beta.8 screenshots are required.

## Per-family qualification matrix (2026-10-08)

All rows use production-build Chromium with explicit isolated synthetic fixtures.
Full business mutations are not claimed for read-only or unavailable optional
routes. Existing module unit/contract tests remain enabled alongside these checks.

| Inventory family | Passing journey / evidence | Applicable limit |
| --- | --- | --- |
| Overview and analysis | existing-page-families; remaining-dashboard-role-data-states; remaining-dashboard-performance-recovery | Owner/admin reads, populated/empty/error, ordinary-user denial; no real metrics |
| Channels and accounts | existing-page-families; channel-task-navigation; quota and routing suites | Existing chart/action contracts; no provider credentials or real channel change |
| Model catalog | existing-page-families; remaining-deployment-disabled-and-unavailable; model-drawer-unavailable-draft-recovery | Metadata plus guarded deployment recovery; no provisioning; Root-option reads excluded for normal admin |
| API Keys | existing-page-families; keyboard-drawers-and-history; access suite | Synthetic CRUD/access/budget evidence; no real key disclosure |
| Users | existing-page-families; access suite | Existing role/assigned-access contracts; no live user mutation |
| Usage and recovery | existing-page-families; quota/routing suites | Log/timing/stream/attempt surfaces and guarded recovery; no real settlement |
| Content logs | existing-page-families | Synthetic protected records; no sensitive source logs |
| Profile | existing-page-families | Existing identity/preferences surface; no password/passkey changes |
| Funding history | existing-page-families | Disabled-commerce history remains reachable, no purchase |
| Optional administration | existing-page-families; role-and-preference-boundaries | Subscription/redemption history and visibility; no commerce enablement |
| Existing tools | remaining-playground-read-only-and-gate; remaining-chat-missing-presets-and-recovery; remaining-prompt-learning-loading-error-recovery | Unsent drafts, history, disabled/error/retry, no model call or learning mutation |
| System information | existing-page-families; role-and-preference-boundaries | Root-only surface; no maintenance operation |
| Site settings | settings-deep-links; responsive-themes-and-seven-languages | All 4 links, synthetic read/feedback |
| Authentication settings | settings-deep-links; settings-loading-error-recovery | All 5 links; no auth/security reconfiguration |
| Billing settings | settings-deep-links; settings-loading-error-recovery | All 7 links; no money/price-policy changes |
| Model settings | settings-deep-links | All 8 links; no routing/deployment changes |
| Security settings | settings-deep-links | All 4 links; no security/permission expansion |
| Content settings | settings-deep-links | All 7 links; preserve capability hiding |
| Operations settings | settings-deep-links | All 7 links; no install, restart or alert-target activation |
| Setup | public-auth-setup-and-errors | Interrupted synthetic setup, validation/guard; no real administrator creation |
| Authentication | public-auth-setup-and-errors; remaining-auth-alias-and-incomplete-flows | Alias, OTP and incomplete OAuth recovery; no submitted credentials/provider login |
| Errors | public-auth-setup-and-errors | 401/403/404/500/503 and error recovery |
| Existing public pages | public-auth-setup-and-errors; remaining-pricing-disabled-route-guards; remaining-pricing-content-and-recovery | Optional-route redirect checked separately from explicitly enabled pricing content; missing/error/Retry/Back |

Shared checks cover 320/768/1280, light/dark, seven-language settings, saved
preferences, navigation/keyboard/focus/history, disabled/error/empty/loading states
and native-wheel dialog footer access. Pixel inspection confirmed chart marks,
complete narrow pricing tabs, non-overlapping compliance/log groups and sensible
remaining-route layouts. Real provider/bill and complete live authentication
acceptance remain explicitly deferred, not fabricated to close a row.

Review follow-up: unit behavior regressions protect model drafts across background
options failure/retry and deployment request generations across interrupted tab
visits. Failed connection-cache reentry retains its real guard and keyboard Retry.
The additional drawer Chromium journey passed on db786404 and is included in the
19 journeys / 234 screenshots above; background-refetch retention has separate
component-test coverage.
