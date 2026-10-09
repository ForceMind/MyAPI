# Personal-core slice 1: policy and Key-limit clarity

Status: bounded slice accepted and normally merged by PR #6 (2026-10-09). New release publication is separate.
Base: `24bcf9ed223f11aa655584bce3b8335b5b43727f`, after the published beta.10 incident patch.
Branch: `codex/personal-core-clarity-20261009`.
This is the first bounded implementation of step 2 in [the personal-core card](PERSONAL_CORE_DELIVERY_CARD.md), not completion of the entire personal-core plan.

## User outcome

From API Keys, an owner can find their saved usage policy, distinguish it from the confirmed server funding mode, and understand which configuration conditions are met. The same page makes each Key's budget action visible and distinguishes ordinary internal quota, strict Token, strict USD and Codex account-window thresholds.

## Scope and invariants

- Reuse the existing policy dialog and API. Owners read only their own policy; Root changes retain explicit confirmation, revision checks and operation-ID replay.
- Read raw, live-confirmed server funding capabilities. A minimal-build presentation override is not evidence that commercial funding is disabled.
- Missing, malformed, contradictory, refreshing or failed configuration cannot be described as confirmed effective policy. A saved preference is not a request admission guarantee.
- Ordinary quota is a price-converted internal allowance, not actual input/output Token count. Removing that cap does not remove strict budgets, model permissions or account thresholds.
- Strict Token and USD may coexist. Codex account thresholds concern upstream account windows and are mutually exclusive with those strict budgets; they are not per-Key percentage accounting.
- No backend protocol qualification, ledger, writer, frozen identity, unknown-usage hold, production data or commercial-history changes.
- No version tag or image is changed by this slice. beta.10 remains immutable.

## Implementation ownership

- UI implementation: existing policy dialog, Key primary buttons, row actions, Key edit drawer and budget dialog, plus focused tests and a pure presentation resolver.
- Integration: translations, browser fixture/acceptance, delivery evidence and source checkpoint.
- Independent review: raw funding-state interpretation, owner/Root isolation, stale state and unchanged mutation semantics before integration closes.

## Acceptance ledger

- [x] Reproduce old behavior with focused tests: initial four-file run has 9 new failures and 28 pre-existing passes; separate personal-entry run has 2 failures and 1 pass.
- [x] Focused red/green tests: 67/67 passed, including raw funding-mode matrix, account-switch reset and unchanged mutation payloads.
- [x] Seven locales (23 additions each), exact placeholder preservation, sync, typecheck, changed-file lint/format and production build. Aggregate frontend: 184 files / 1109 tests passed. Browser fixture contracts: 23/23 passed.
- [x] Real Chromium at seven-language 320 px and English 1280 px; readable long labels, keyboard/touch entry, refresh/close/reopen, owner read-only and no automatic writes.
- [x] Existing backend and three-database contracts on the exact PR candidate, without claiming new backend coverage from UI tests.
- [x] Independent review and accurate-head CI, with artifact source/tree binding.

## Deferred to the following bounded slice

The actual backend structural boundary, new/existing-user end-to-end admission/limit scenarios and stable-release real-provider evidence remain separate unchecked obligations of the parent card. This presentation resolver is not claimed as completed backend architectural simplification. Reference decisions remain tied to [the fixed sub2api audit](SUB2API_REFERENCE_AUDIT.md).

## Browser and pixel findings before final acceptance

- Source `6297bf8`: 9 CI jobs passed; the new browser journeys failed because the top icon and footer shared the accessible name Close. The script now explicitly selects the footer, and language/width cases report independently without weakening the overall failure gate.
- Source `b90eccf`: all 10 CI jobs passed (31 shared journeys / 228 screenshots, plus 8 Playground journeys / 96 screenshots), but manual pixel review rejected overlapping Vietnamese/French dialog headings, French/Russian Key-save labels and the Vietnamese expiry shortcut. Automatic green was not counted as completed visual acceptance.
- The bounded correction reserves close-button space only on the two affected titles and allows wrapping/automatic height only on the existing Key drawer footer/expiry buttons. Three added layout-contract tests failed first and then passed. Browser regression now checks actual heading text rectangles, button text containment, separate controls, scroll reachability, Escape/focus restoration and discard of an unsaved quota draft. The corrected exact head subsequently passed the fresh CI and pixel review recorded below.

## Final source acceptance

- Source `a17a8e11b7600b8c3091f80047eef6cbcfa87f11`; actual PR checkout `0316e0663e3cc1d6e44b598790f0ffea367ee910`; equal tree `46960b1c0340d2c3942f2955073184c7a3d77078`.
- [CI 37957563841](https://github.com/ForceMind/MyAPI/actions/runs/37957563841): all 10 jobs passed, including applicable three-database and backend race contracts. Frontend: 184 files / 1109 tests.
- Shared UI: 31 journeys / 236 screenshots; Playground: 8 / 96. Seven-language 320px personal policy, budget and Key-editor images and English1280px equivalents were reviewed, including the originally failing headings/buttons. There are 40 recorded personal-layout checks, plus focus restoration, scroll reachability, cancellation and no-write assertions. This is scoped synthetic UI acceptance, not manual inspection of every inherited page or live provider billing.
- Original artifact `11630901670`, ZIP SHA256 `40435f6fab3b4a80d883dfd352872a1b8e4a30efeb0fc60bb9efbd7e8a43e671`; clean report checkout equals the commit above.
- Independent review passed; clean-source release:check passed with 2956 files.
- Normal merge: `ee97e2e8e8000b2d081449bc38998dd867c6e175`, same tree. Main CI is tracked separately and must not be inferred from PR green. No new tag, image or deployment belongs to this slice.
