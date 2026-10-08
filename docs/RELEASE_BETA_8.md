# v0.2.0-beta.8 release candidate

## Status and authorization boundary

This is preparation for the existing full-interface beta.8 plan, not a published
release or a production-upgrade instruction. Draft PR #3 targets main:
<https://github.com/ForceMind/MyAPI/pull/3>.

The candidate uses `VERSION=0.2.0-beta.8`, the matching root package version and
matching default Full image references. These references do not prove that the
images exist. Do not pull or deploy them until the publication evidence below is
complete. No NPM publication is included in this candidate.

Main merge, creation of the new `v0.2.0-beta.8` tag, GitHub prerelease and Full/LAN
GHCR publication require their bounded authorization. Use the existing protected
workflows and environment approvals; do not relax their gates or create an
alternate publishing workflow. No force push, tag replacement, stable `latest`
promotion, credential expansion or production deployment is included.

## Included scope

- Shared MyAPI navigation, page hierarchy, feedback and responsive design across
  the existing operational pages and 42 registered settings sections.
- Desktop/mobile, seven-language and theme behavior; keyboard/focus, native
  scrolling, repeated dismissal and browser-history recovery.
- Existing optional analytics, deployment, tools and model-price detail pages,
  with separate loading, unavailable, empty, disabled and forbidden states.
- Root-only model-price configuration reads; normal administrators retain model
  metadata editing without fetching system options.
- Synthetic interrupted authentication/setup and optional-commerce history
  acceptance, without enabling commerce or submitting credentials/model calls.

Backend billing, routing, access enforcement, OAuth and chart data semantics are
preserved. There is no database migration or deletion in this UI version.
Existing channels, historical quotas, usage/log records, users, Keys, permissions,
accounting and identity keys remain. Hiding optional money-related navigation is
not a request to erase historical financial or recovery records.

## Evidence to finish before publication

1. Record the final PR head and tree; all applicable CI jobs and four browser
   suites must pass on that source. Inspect actual screenshot pixels, including
   narrow pricing tabs, populated analytics and scroll-reachable dialog actions.
2. Synchronize the page inventory and delivery card; record remaining live-account
   limits. Generate the source manifest from the clean exact build checkout and
   verify packaged file hashes. A local dirty-tree manifest is not release proof.
3. After authorized merge, qualify the exact main commit and tag that commit;
   ensure tag, `VERSION`, package version and workflow-resolved SHA match.
4. Through `release.yml`, produce all four existing platform binaries and three
   checksum files. Verify the release is a prerelease and is not latest.
5. Through `docker-build.yml`, produce existing Full and LAN amd64/arm64 images,
   immutable version references and digest/signature/provenance evidence. Verify
   both editions against the exact tag. Do not create new Lite/Desktop channels.
6. Record verified asset names, hashes, immutable image digests, source SHA and
   workflow URLs here. Then report that the bounded beta.8 artifacts are ready
   for deployment planning and pause before any actual production action.

## Candidate evidence ledger

- Prior UI checkpoint: `f566ddf29843e97dedd748d6b2d0bdbae424b4bb`, all ten jobs in
  <https://github.com/ForceMind/MyAPI/actions/runs/37673758425>; four Chromium
  suites and 150 PNGs. This is historical evidence, not the final candidate.
- Remaining-route follow-up source: `945a031ac5b4f67958d574ed3f70e37f959ce3e2`,
  tree `b9c24e045fcf0fe511536314d1226bd33d821b8d`,
  <https://github.com/ForceMind/MyAPI/actions/runs/37720080573>.
  Local 167 frontend test files / 881 tests, typecheck, production build and
  changed-file lint passed. All ten remote jobs and all four browser suites passed;
  18 shared UI journeys / 42 settings links / 232 PNGs. Actual pixels confirm
  populated charts, complete mobile pricing labels and corrected access boundaries.
  Test merge d865be1d0cefc5854a216849cd0de6d7b3e1ef71 has no file differences.
- Final reviewed UI source: `db7864049b5691ae4d2b222a896067333808edc4`, tree
  `ccdd1b55068b6784d2c319072edbbfedeb01bc77`;
  <https://github.com/ForceMind/MyAPI/actions/runs/37723533843> passed all ten jobs.
  167 frontend files / 894 tests; four Chromium suites; 19 journeys, 42 settings
  links and 234 PNGs. Test merge `c194c7f44defb0ecf7059558fc24e33f622664fd` has
  no file differences. Website check 37723533838 passed; Docker smoke was skipped
  by its existing branch allowlist, not counted as a build success.
- Clean exact-file-tree release:check passed, including manifest/pack validation:
  2,896 files / 28,515,398 unpacked bytes.
- Merged/tag SHA, release assets and container digests remain pending authorization
  and existing protected workflow results. No deployable-image claim is made.

## Review follow-up

Independent review prompted draft-preservation and interrupted-deployment fixes
with demonstrated failing tests. Failed connection-cache reentry is also covered
through the actual guard and keyboard Retry. These changes do not alter backend
permissions, deployment/provider contracts or billing. Their db786404 exact-head CI
and new drawer screenshots passed and supersede the earlier candidate; use
PR #3's latest verified head, never merge the older candidate merely because its
checks were green.

## Explicit limits

Browser fixtures are synthetic, not successful live authentication, paid model
usage, supplier-bill reconciliation or production upgrade/restore acceptance.
Those previously deferred live-account checks remain deferred. This remains a
bounded prerelease, not proof of production-stable 0.2.0. Beta.7 deployment facts
and prior provider/billing evidence must not be presented as beta.8 validation.
