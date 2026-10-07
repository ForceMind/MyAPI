# MyAPI beta.8 page and task inventory

Audited from `1bd48522b85e91929b6a7de9a78f142a957c6147`, not a proposed backend.
Initial status for every family below is **migration pending**. Shared shell and
component work must be tested against each family before that status changes.

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
