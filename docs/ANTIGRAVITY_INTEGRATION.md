# Google Antigravity compatibility boundary

Google Antigravity is not a second name for the Gemini `generateContent`
endpoint. The managed Antigravity agent is exposed through Google's preview
Interactions API at `POST /v1beta/interactions`, with an `agent` (for example
`antigravity-preview-05-2026`) and an optional `agent_config`. See the
[official Antigravity agent documentation](https://ai.google.dev/gemini-api/docs/antigravity-agent)
and the [official Interactions API reference](https://ai.google.dev/api/interactions-api-v1)
for the current schema and availability.

## What MyAPI supports today

MyAPI keeps the existing Gemini, Claude Messages, and Codex channels unchanged:

- Gemini uses the official Generate Content API and `x-goog-api-key`.
- Claude uses the Anthropic Messages API and its existing adaptor.
- Codex uses the existing Responses-compatible adaptor and OAuth channel flow.
- The Gemini package contains an Antigravity configuration boundary with a
  corresponding test suite in
  `relay/channel/gemini/antigravity.go` and a dedicated, non-persistent
  `AntigravityClient` in `relay/channel/gemini/antigravity_client.go`. The
  client validates the HTTPS endpoint and preview agent, sends only an
  explicit Interactions payload, and supports create, bounded status polling,
  cancellation, and deletion. It extracts the provider's typed `usage`
  counters without storing the upstream body.

The boundary is deliberately not registered as a new channel type or silently
routed through Generate Content. Wire IDs remain unchanged. It also does not
read `~/.codex`, Claude credentials, browser profiles, keychains, or any other
local credential store; the API key must be supplied through MyAPI's normal
channel secret when a future adapter is enabled.

## Current limitation and integration gate

The normal MyAPI relay lifecycle expects one request/response exchange and
provider-specific billing. The standalone client is deliberately not wired to
that lifecycle or to a public channel yet. It is a safe transport building
block, not a claim that OpenAI Chat, Claude Messages, or Gemini Generate
Content traffic can already execute an Antigravity agent. A future adapter may
use the client only after it has an explicit route/channel policy and billing
contract.

A production adapter must be added only after all of the following are
implemented and tested:

1. A dedicated interaction request/response lifecycle, including background
   polling, cancellation, and stateful continuation semantics.
2. Explicit mapping of agent output and tool events to the selected public API
   (OpenAI Responses is the closest fit; Chat Completions is not lossless).
3. Usage/budget accounting and limits that distinguish agent tokens from normal
   Generate Content usage.
4. Request policy controls for remote code execution, filesystem, web access,
   MCP tools, and image inputs. No local filesystem or user credential mount is
   allowed by default.
5. A compatibility test suite against a documented mock Interactions endpoint.

The first-phase client test suite covers the transport portion of these gates
in `antigravity_client_test.go`: create/get/cancel lifecycle, bounded polling,
usage extraction, strict input/ID validation, HTTPS/auth requirements, and
error-body redaction. The suite is checked into the repository, but this
workspace currently lacks the Go toolchain and recent CI runs failed before
starting a runner; treat execution as pending until a Go-capable local or CI
environment runs it. It does not grant ordinary Gemini traffic access to the
client.

The preview agent's `agent_config` accepts a much narrower contract than a
normal Gemini generation config. MyAPI's compatibility boundary uses the
documented `type: "dynamic"` marker and the client therefore rejects arbitrary
`temperature`, `top_p`, `top_k`, `stop_sequences`, and `max_output_tokens`
fields rather than silently forwarding them. (Those options may be valid for a
model interaction's separate `generation_config`; they are not implied for the
Antigravity agent.) Preview agent names and model availability can change, so
they are configuration data rather than a new stable wire identifier.

The two current Google references expose a schema-version detail that must stay
visible during future runtime verification: the Antigravity guide shows
`agent_config.type: "antigravity"` in its provider-specific examples, while the
current Interactions OpenAPI definition models dynamic agents with a required
`type: "dynamic"` marker. MyAPI follows the latter typed API definition for
this transport boundary and does not silently switch between the two values.
The provider-specific example must be confirmed against a live, authorized
preview request before either value is changed. Likewise, continuation uses
the documented `environment` field with the prior `environment_id`; the
transport never emits an invented `environment_id` request property.

The official documentation currently describes this as a preview agent and
documents the Interactions API request shape, but it does not define a
provider-neutral account-balance endpoint for MyAPI to poll. Until Google
publishes a stable quota/billing API for this agent, the quota history UI must
show `unsupported` rather than infer a balance from interaction responses.

## Recommended configuration while the adapter is pending

For standard Gemini model access, continue using the existing Gemini channel
or an Advanced Custom route targeting `/{version}/models/{model}:generateContent`.
For Claude-compatible providers, use the existing Claude channel or the
Advanced Custom Claude Messages route. Do not label either route
“Antigravity”; doing so would misrepresent the API and its execution model.
