# Settlement review status patch after beta.9

## Published outcome

The status patch and the independently reproduced stream-completion cancellation and
legacy batch/no-Redis fixes shipped together in [beta.10](RELEASE_BETA_10.md), from
`631b3205ab2ad28df3f9372bd00f02bd27cb4c31`, after PR #5 and exact-main acceptance.
The prior beta.9 artifacts remain immutable. No user-server deployment, production
accounting change or historical pending-log rewrite is included.

## Frozen scope and historical starting point

The patch began from main `7492ec36edf68716953cc81c304f4c902d392751` as a Draft PR.
The contracts below record that scope; they do not mean merge/publication is still pending.
Personal-core work continues on its separate next delivery card.

The reported screenshot is evidence of a confusing recovery interface, not proof
of the production request's root cause. A read-only investigation and isolated
SQLite reproduction established that legacy settlement can apply successfully
while its subsequent journal update fails. The resulting prepared journal can
still expose manual recovery despite an existing automatic settlement fact.
Normal zero-reservation settlement succeeds; zero does not prove free usage.

## Required behavior

- Read the existing settlement ownership and completion evidence before offering
  manual reconciliation. An automatic intent/fact must not expose a fresh editable
  amount/evidence form. Read errors and inconsistent identities fail closed.
- Distinguish automatic settlement in progress, automatic recovery requiring
  attention, and applied settlement awaiting record finalization. Only claim an
  applied amount when durable, identity-matching completion evidence proves it.
- Use bounded, allowlisted status/reason values and translated plain-language UI.
  Do not expose raw SQL, credentials, request content or arbitrary exception text.
- Preserve legitimate unknown-usage/manual reconciliation, owner authorization,
  strict Token/USD behavior, both quota writers, and existing server-side guards.
- Refreshes, stale requests, user changes and failed reads must not restore an
  unsafe submit action. No automatic POST, guessed zero, extra debit or refund.
- Preserve pre-prepare strict-budget recovery. When durable applied accounting proves
  a known amount, reject a conflicting manual amount before preparing holds or
  writing decisions; enforce this server-side as well as in the read-only form.

## Excluded changes

No production request inspection or account mutation; no accounting algorithm,
schema, policy, writer switch, recovery replay, timeout change or relaxed guard.
Do not suppress real settlement errors or fabricate completed usage. Do not infer
that the reported production request had the same cause as the synthetic case.
This patch does not change the already published beta.9 release or deploy servers.

## Acceptance and stopping condition

1. Red/green backend regressions for both writers, automatic ownership and applied
   fact/journal lag, zero reservation, unrelated identities and read errors.
2. Frontend behavior tests for safe status, blocked manual editing, legitimate
   manual flow, same-request refresh and role/owner isolation; all seven languages.
3. Existing SQLite/MySQL 5.7/PostgreSQL 9.6 jobs exercise the changed read behavior.
4. Real Chromium covers pending and applied-record states without a mutation,
   legitimate manual access, and readable desktop/mobile status. Inspect pixels.
5. Independent review and complete applicable exact-head CI; preserve failed
   attempts and corrections. Stop at the verified Draft source candidate.

Evidence is recorded in the new patch PR against its exact source head. Local
SQLite or a previous beta.9 result never substitutes for that head's database and
browser checks. Production root-cause attribution remains unverified unless
separately authorized, narrowly scoped diagnostic evidence establishes it.
