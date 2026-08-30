# Antigravity public relay integration gate

This document defines the conditions for exposing Google Antigravity through a
public MyAPI route. It is a maintenance and review contract, not a promise
that the route already exists.

Google Antigravity is a managed preview agent exposed by the Gemini
[Interactions API](https://ai.google.dev/api/interactions-api-v1). It is not a
normal `generateContent` model and it does not have the same request,
execution, or billing lifecycle as Gemini, Claude Messages, or OpenAI
Responses.

## Current state

MyAPI currently provides a standalone, non-persistent `AntigravityClient` in
`relay/channel/gemini/antigravity_client.go`. The client is intentionally
limited to the documented transport lifecycle:

- `POST /v1beta/interactions` (create)
- `GET /v1beta/interactions/{id}` (read state)
- `POST /v1beta/interactions/{id}/cancel` (cancel)
- `DELETE /v1beta/interactions/{id}` (delete)
- bounded polling implemented by the caller

It validates HTTPS, the API key, the preview agent and request identifiers;
uses an explicit payload allow-list; caps response size; redacts upstream
error bodies; and extracts only typed usage fields. It does not persist or log
the upstream interaction body. No public `/v1/interactions` route, channel
type, model alias, or ordinary Gemini fallback is registered.

This separation is deliberate. A transport client is not a relay, and a
successful provider request alone does not establish authorization, billing,
retention, or compatibility semantics for MyAPI users.

## Why the public route is gated

The normal MyAPI relay assumes a bounded request/response exchange, a known
usage unit, and a provider-specific billing result. Antigravity can execute a
background interaction, produce intermediate tool steps, remain in progress
for an unbounded period, and expose remote environments. Forwarding it through
Chat Completions or the regular Gemini adapter would silently lose state and
could charge, expose, or retry work incorrectly.

The public route must therefore be a separately reviewed interaction service.
Until every gate below is met, requests must be rejected as unsupported rather
than silently downgraded to another provider.

## Mandatory gates before public exposure

### 1. Durable interaction lifecycle

Implement a MyAPI-owned interaction record with an explicit state machine:

`accepted -> queued -> running -> completed | failed | cancelled | expired`.

The record must contain an opaque MyAPI interaction ID, owner/tenant scope,
provider interaction ID, selected agent, creation/update/expiry timestamps,
request fingerprint, and a redacted result summary. State transitions must be
monotonic, transactional, and idempotent. A duplicate create request with the
same idempotency key must not start a second provider interaction.

Background work must use bounded polling with a lease/lock, retry backoff, a
maximum lifetime, and recovery after process restart. Cancellation must be
race-safe with completion. Provider records must be deleted according to the
retention policy; deletion is not a substitute for local audit retention.

### 2. Route and permission contract

Expose a dedicated route only after its API shape is documented. The minimum
contract is:

- `POST /v1/interactions` — create; requires an idempotency key and an allowed
  Antigravity agent.
- `GET /v1/interactions/{id}` — read only the caller's interaction.
- `POST /v1/interactions/{id}/cancel` — cancel only an owned, non-terminal
  interaction.
- `DELETE /v1/interactions/{id}` — remove provider state only when policy
  permits it.

Every operation must enforce tenant/owner isolation, API-key scope, rate and
concurrency limits, maximum input size, and an allow-list of agents and
environments. Administrators may inspect metadata only through an audited
permission; raw prompts, outputs, credentials, and tool traces are never
returned merely because a caller is an administrator.

The public OpenAI-compatible surface must be explicit about lossiness. If an
OpenAI Responses adapter is offered, interaction IDs, asynchronous status, and
tool events must have documented mappings. Chat Completions must not be
advertised as lossless compatibility.

### 3. Usage, quota, and billing

Persist a normalized usage snapshot only after defining its accounting unit.
At minimum, distinguish input, output, cached, thought, tool-use, grounding,
and total tokens when the provider supplies them. Record the provider response
revision and the exact accounting timestamp.

Before charging a user, define:

- price and currency per usage unit;
- preflight budget reservation and maximum spend;
- behavior for an incomplete, cancelled, failed, or retried interaction;
- idempotent settlement and refund/reversal rules;
- rounding and timezone-independent aggregation;
- owner-visible usage versus administrator audit data.

Provider `usage` counters are not an account balance. The current official
Interactions API documentation does not define a stable balance endpoint for
Antigravity. MyAPI must show quota as `unsupported` unless a documented,
authorized provider quota API is implemented; it must never infer a balance
from tokens, HTTP status, or a successful interaction.

### 4. Tool and remote-environment security

Remote execution is a security boundary. A public adapter must define a
default-deny policy for every tool and environment capability, including code
execution, filesystem access, web access, MCP tools, network egress, image or
file inputs, and tool-generated outputs.

The adapter must not mount a user's local filesystem, browser profile,
keychain, `~/.codex`, Claude credentials, OAuth JSON, API keys, cookies,
session secrets, or process environment into an Antigravity environment. Any
provider-side environment must be explicitly selected, isolated, time-limited,
and attributable to the requesting tenant. Tool calls require request-scoped
limits, cancellation propagation, output-size limits, and audit events that do
not contain secret material.

### 5. Data protection and observability

Define retention and deletion for prompts, outputs, tool events, usage, and
provider IDs before enabling persistence. Encrypt secrets at rest, keep API
keys in the existing channel-secret mechanism, and redact credentials and
prompt-bearing error bodies from logs. Metrics should expose counts, latency,
poll attempts, terminal states, provider status, and cost totals without
capturing payloads.

Failure responses must use stable MyAPI error codes. They may include a
request ID and retry guidance, but never echo provider response bodies,
authorization headers, cookies, API keys, OAuth data, or raw tool traces.

### 6. Compatibility and regression tests

Before route registration, add a deterministic mock Interactions endpoint and
test at least:

- create, get, bounded poll, cancel, delete, and restart recovery;
- idempotency and duplicate delivery;
- every valid and invalid state transition, including cancel/complete races;
- owner/tenant isolation and permission failures;
- timeout, retry, rate-limit, oversized input/output, malformed JSON, and
  provider 4xx/5xx responses;
- usage settlement, cancellation refund, failed interaction, and rounding;
- tool-policy denial, cancellation propagation, and secret redaction;
- Responses mapping and the explicitly documented non-lossy/lossy fields;
- desktop/LAN Lite clients receiving asynchronous status correctly;
- mobile and desktop UI loading, empty, expired, failed, and unauthorized
  states.

The acceptance run must include Go unit/integration tests, frontend tests and
type checks, route-contract checks, secret/brand audits, and a resource-bounded
build. A green transport test alone is not sufficient evidence for public
exposure.

## Explicit non-goals and prohibited shortcuts

- This work is not a copy of TokenHub or New API. TokenHub is a reference for
  provider-neutral boundaries and UI ideas only; no source, runtime, branding,
  or license metadata is imported by this gate.
- MyAPI must retain its own routing, persistence, authorization, billing, and
  audit layers. A gateway setting or model alias must not bypass them.
- The adapter must not read local credentials or authorize a user's local
  Codex, Claude, browser, or Antigravity session. A future channel receives an
  explicitly configured provider secret through MyAPI's normal secret store.
- The adapter must not claim account balance, remaining quota, or subscription
  credit without an official provider API and an approved accounting model.
- Do not route Antigravity requests through Gemini `generateContent`, Claude
  Messages, or Chat Completions merely to make an endpoint appear compatible.
- Do not expose the route, publish an image, or change production configuration
  as part of transport development. Public registration requires a separate
  review of this document, the API contract, security policy, billing model,
  and the complete test evidence.

## Review checklist

The integration owner may request public-route review only when every item is
checked and linked to evidence:

1. lifecycle schema and migration reviewed;
2. state machine and restart/retry behavior tested;
3. route, scope, tenant isolation, rate limits, and audit permissions tested;
4. tool/environment policy approved with default-deny behavior;
5. usage pricing, reservations, settlement, refunds, and unsupported-balance
   behavior documented;
6. retention, redaction, encryption, and deletion tested;
7. mock-provider and real-provider compatibility evidence recorded;
8. frontend, mobile, LAN Lite, and desktop states verified;
9. resource-bounded CI/build evidence green;
10. explicit owner approval recorded before route registration or release.

Until then, keep `AntigravityClient` as an internal transport primitive and
return a clear unsupported response from any prospective public adapter.
