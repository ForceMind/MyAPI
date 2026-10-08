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
Runtime VERSION, root package and default image references are prepared as beta.8
in the release candidate. References are not proof of published images. Each version completes its applicable review, merge and
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

## Qualified UI source (2026-10-08)

The full bounded page migration is in [Draft PR #3](https://github.com/ForceMind/MyAPI/pull/3).
UI source `945a031ac5b4f67958d574ed3f70e37f959ce3e2`, tree
`b9c24e045fcf0fe511536314d1226bd33d821b8d`, is exercised by
[CI 37720080573](https://github.com/ForceMind/MyAPI/actions/runs/37720080573).
Test merge `d865be1d0cefc5854a216849cd0de6d7b3e1ef71` has no file differences
from that source. All ten jobs passed, including the complete backend race groups,
independent relaykit and existing SQLite/MySQL/PostgreSQL gates.

- 167 frontend files / 881 tests, typecheck, production build, changed-file
  lint/format and 16 browser-fixture contract tests passed.
- Four real Chromium suites passed: shared UI, quota/budgets, routing, access/events.
  Shared UI has 18/18 passing journeys, 42/42 settings links, 169 screenshots,
  zero unexpected requests and zero page errors. All suites total 232 PNGs.
- Main operational and remaining analytics/deployment/tool/pricing/auth families
  reached their applicable synthetic UI acceptance. The inventory records the
  exact journey mapping and limits, rather than equating redirects with content.
- 320/768/1280 widths, seven languages, themes, sidebar/role gates, keyboard focus,
  Close/Cancel/Escape, Back/Forward and interrupted flows retain regression checks.
- Budget footer Save/Close is reached using native wheel input at 320x900 and
  320x640, with hit/clip bounds and coordinate clicks. Save occurs once; reopening
  retains 20.01%. Long compliance actions and log timing groups no longer overlap.
- Pixel review confirms populated analytics has actual chart marks; narrow model
  detail labels remain complete. A running fixture clock permits real chart
  animations to finish; production chart/data logic was not rewritten.
- A closed model drawer no longer fetches Root-only system options; normal admins
  retain metadata editing. Failed Root options reads show Retry, not editable
  pricing defaults. Optional unavailable states remain distinct from disabled,
  missing and empty; no new policy, deployment or credential mutation is enabled.

Earlier failures remain visible in Actions history. The 08355a23 run exposed the
model-option prefetch and a disabled-Playground request-attribution assumption;
945a031a fixes the source boundary and attributes redirected overview reads to
that page. Assertions were retained or strengthened. Previous green checkpoint
f566ddf2 and its 150 PNGs remain historical evidence, not substituted final tests.

## Version and release closure

The remaining work is exact-source version/release preparation and its applicable
CI, evidence delivery, then the explicitly authorized merge/publication sequence.
The [beta.8 release candidate](RELEASE_BETA_8.md) records required asset, tag and
workflow evidence. No main merge, tag, prerelease, GHCR push or production upgrade
has occurred for beta.8. Default beta.8 image references must not be used before
publication is verified. NPM and later-version work are excluded.

All browser data are synthetic. No live authentication/provider/bill acceptance
or production-stable 0.2.0 is implied. The prior real-account acceptance deferral
is retained. Historical handoff documents affected by earlier publication limits
remain unchanged remotely; local work was preserved.

Stop after the declared current-version scope, evidence and authorized publication
are complete, then report deployable artifacts and pause before production action.
Do not substitute a partial UI batch for the complete current-version plan.
