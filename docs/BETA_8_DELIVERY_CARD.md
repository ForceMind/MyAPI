# beta.8 delivery card: the MyAPI interface

## Kickoff and version boundary (2026-10-07)

The independent complete UI node in [the master plan](MYAPI_MASTER_PLAN.md#独立完整-ui-重构版新增已确认节点)
is the sole goal of **0.2.0-beta.8**. Implementation starts after the cumulative
beta.7 release, from main `1bd48522b85e91929b6a7de9a78f142a957c6147`
(tree `c562c311c8324bb5412d33de0ae43ac14f73eb8a`). PR #2 is merged; the
[beta.7 prerelease](https://github.com/ForceMind/MyAPI/releases/tag/v0.2.0-beta.7)
has four binaries and three checksum assets. Its
[Full/LAN publishing workflow](https://github.com/ForceMind/MyAPI/actions/runs/37581984565)
completed. These are prior-version facts, not beta.8 validation or evidence of
production deployment.

Before the first source publication, concurrent main documentation checkpoint
`ba52227ebe567539ddeb578ec2994539d31fa153` was inherited by a non-force
fast-forward. Its four release/deployment evidence files are preserved unchanged.

Work is isolated on `codex/beta8-full-ui-20261007`, with a new Draft PR targeting
main. The published beta.7 tag, old development branch and release remain intact.
Runtime VERSION and default images remain beta.7 until beta.8's own qualified
release preparation. Each version completes its applicable review, merge and
release sequence before implementation of the next version. Ordinary design
iteration is part of the approved UI work; it is not an extra approval gate.

## Bounded outcome

Unify the existing product's information architecture, navigation, layout,
visual hierarchy, forms, data surfaces and feedback on desktop and mobile.
Reuse React/Rsbuild/Bun, Base UI, existing features and existing API contracts.
Do not build a parallel application, invent successful requests or metrics,
change billing/routing/permissions/OAuth semantics, add backend features, enable
commerce, expand protocols/media, develop Desktop, or deploy production.

The route and task inventory is [UI_PAGE_INVENTORY.md](UI_PAGE_INVENTORY.md).
The initial audit found 61 route modules, 26 feature directories, four dashboard
sections, three log sections, two model sections and 42 settings sections.
Route modules include layout/redirect modules; these counts are not a claim of
61 independently redesigned screens.

## Design and interaction contract

- MyAPI stays the exact product name. Evolve its own blue/ink identity, rather
  than borrowing another project's brand. Use restrained raised surfaces,
  readable neutral backgrounds, clear primary actions and explicit status text
- Put routine operational tasks before optional chat tools. Keep old URLs,
  query/filter parameters, browser history and deep links compatible
- Retain existing theme/layout preferences, reduced motion, admin×user sidebar
  narrowing and all role/resource gates. Navigation must not advertise Root-only
  pages as available to ordinary administrators
- Shared page headers carry a clear task title and optional supporting context;
  long titles and actions wrap instead of disappearing. Data tables keep their
  deliberate scroll region; mobile flows remain operable at 320px
- Loading, empty, error, forbidden, stale evidence, processing, conflict and
  recovery states remain distinct. Never turn unavailable data into a healthy
  state, a zero cost, a fresh quota or a completed operation
- Keep keyboard order, visible focus, skip navigation, accessible names, touch
  targets, dialog dismissal and focus return. All new copy has seven locales
- Preserve legal notices and source provenance. Brand changes do not claim a
  complete rewrite of third-party source

## Migration batches and revisable prototypes

1. Inventory, shared shell and design system. Existing production components
   form the prototypes; synthetic browser API fixtures clearly separate a UI
   demonstration from real-account acceptance
2. Operational journeys: overview → channel/account evidence → routing preview;
   user/Key → assigned access/budget → safe denial; logs → unknown evidence →
   authorized recovery. Preserve the existing quota chart data model and dialogs
3. Settings, authentication/setup, system maintenance and remaining existing
   pages. Disabled optional pages retain guarded history and recovery
4. Cross-page qualification, pixel inspection, exact-head CI, delivery documents
   and the version's review/release handoff. No continuous decorative expansion

These are implementation batches of one version, not separate roadmap nodes.
No prototype is called functional until its interaction and result are verified.

## Acceptance matrix

- Every inventory row records its migration and applicable verification
- Behavior tests cover changed success/failure, state feedback and permissions;
  preserve beta.7 access-policy, budget, recovery, routing and commerce tests
- Seven-language copy/key checks and expanded/long text; light/dark and saved
  preferences; desktop, tablet and 320px mobile layouts
- Keyboard/focus/touch, repeated clicks, Cancel/Close, Back/Forward and interrupted
  navigation. Changed layout is validated in real Chromium and actual pixels
- Frontend tests, typecheck, changed-file lint/format and production build
- Existing root, race, independent relaykit and SQLite/MySQL 5.7/PostgreSQL 9.6
  CI remain enabled. A frontend-only change does not authorize altering them
- Publish with expected-parent protection and read back the exact commit/tree;
  qualify its own CI and screenshots. Prior beta.7 greens cannot substitute

## Current state

The recovered shared UI and nine-file self-use navigation/overview increment
are published in [Draft PR #3](https://github.com/ForceMind/MyAPI/pull/3).
Source `83f8d249ebda4ca46868fcb54ab1efee475448ff` passed all ten jobs in
[CI 37667558771](https://github.com/ForceMind/MyAPI/actions/runs/37667558771),
including root/independent relaykit, race groups and the existing database gates.
The test-merge `0dac604ca3f4e3ee16b55afe292572be8c76f749` has no file differences
from that source. Docker/site workflows are path-filtered and did not run for
this UI-only change; their old results are not claimed as new evidence.

- 159 frontend test files / 852 tests, TypeScript, changed-file lint/format and
  the production build passed. Four independent Chromium suites passed.
- The shared UI report contains nine passing journeys, all 42 registered
  settings links reached, 85 screenshots, no unexpected requests or page errors.
  Together with quota/routing/access artifacts, the checkpoint has 148 PNGs.
- Verified 320/768/1280 widths, seven languages, light/dark, role and sidebar
  restrictions, Close/Escape/focus/Back/Forward, settings failure/retry, disabled
  commerce history, and native-wheel budget footer actions at 320x900/640.
- Fixes include composed-button touch/wrapping hooks, channel menu naming,
  saved Traditional Chinese detection and localized initial funding state.
- Initial browser failures and corrections are retained in the CI history:
  table position changed; localized Close controls became ambiguous; modal
  background controls are hidden correctly; form wrappers replace slot names;
  production 503 retries take 15 seconds; viewport resize must settle before
  scroll geometry. Assertions were retained or strengthened.

Pixel review of that green checkpoint found two additional layout defects:
a long French compliance action overlapped the notice, and mobile log timing
and streaming evidence shared too little width. This follow-up places the
compliance action in normal flow and gives timing a full wrapping row, with
explicit browser non-overlap assertions. Its own final-head CI and screenshots
must pass before this follow-up is qualified; PR #3 records the latest result.

All browser data are synthetic. No real provider, bill, new production upgrade,
release or stable 0.2.0 acceptance is implied. VERSION/default images remain
beta.7. Older historical handoff documents were retained remotely after earlier
publication restrictions; local changes were preserved, not forced through.

Stop when the declared pages share the design/interaction contract, required
regressions pass and repository documents/evidence are synchronized. Do not
claim production-stable 0.2.0 on the strength of UI qualification alone.

## Remaining-page closure in progress (2026-10-08)

The full current-version plan continues beyond the qualified UI checkpoint.
The remaining analytics, disabled deployments, existing tools, pricing detail
and incomplete-auth routes now have explicit synthetic qualification journeys.
New source fixes reject ordinary users before user analytics mounts; recover
from absent chat configuration and preserve configured chat IDs; disable
deployment creation without confirmed availability and keep Root settings links
Root-only; distinguish unavailable learning/pricing/performance evidence from
confirmed disabled, missing or empty data. Read failures retain Retry without
authorizing a business mutation.

Local verification: 166 frontend files / 877 tests, typecheck and build passed;
16 exact-contract fixture tests passed. This batch still requires its own
remote CI and pixel review. No real account, model call, deployment enablement,
policy toggle, credential entry or production upgrade is part of these checks.
The retained real-account acceptance deferral is not reopened.

After all inventory rows reach their applicable UI acceptance, prepare the
beta.8 version/release candidate and exact-source gates. Main merge, a new tag,
GitHub prerelease and GHCR publication still require the corresponding explicit
authorization; production deployment is excluded. Do not stop at a partial
source batch or call the entire beta.8 plan complete while those gates remain.
