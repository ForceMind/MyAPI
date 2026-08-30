# Google Antigravity compatibility boundary

Google Antigravity is not a second name for the Gemini `generateContent`
endpoint. The managed Antigravity agent is exposed through Google's preview
Interactions API at `POST /v1beta/interactions`, with an `agent` (for example
`antigravity-preview-05-2026`) and an optional `agent_config`. See the
[official Antigravity agent documentation](https://ai.google.dev/gemini-api/docs/antigravity-agent)
for the current schema and availability.

## What MyAPI supports today

MyAPI keeps the existing Gemini, Claude Messages, and Codex channels unchanged:

- Gemini uses the official Generate Content API and `x-goog-api-key`.
- Claude uses the Anthropic Messages API and its existing adaptor.
- Codex uses the existing Responses-compatible adaptor and OAuth channel flow.
- The Gemini package contains a tested Antigravity configuration boundary in
  `relay/channel/gemini/antigravity.go`. It validates the HTTPS endpoint,
  preview agent, remote environment, documented model allow-list, prompt size,
  and token budget, and builds a minimal Interactions request.

The boundary is deliberately not registered as a new channel type or silently
routed through Generate Content. Wire IDs remain unchanged. It also does not
read `~/.codex`, Claude credentials, browser profiles, keychains, or any other
local credential store; the API key must be supplied through MyAPI's normal
channel secret when a future adapter is enabled.

## Current limitation and integration gate

The normal MyAPI relay lifecycle expects one request/response exchange and
provider-specific usage accounting. Antigravity interactions can run an
agentic tool loop, persist an environment, continue with a previous
interaction, stream events, and consume a separate token budget. Therefore the
current boundary is validation and request construction only. It is **not** a
claim that OpenAI Chat, Claude Messages, or Gemini Generate Content traffic can
already execute an Antigravity agent.

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

The Interactions API currently rejects `temperature`, `top_p`, `top_k`,
`stop_sequences`, and `max_output_tokens`; the helper rejects these options so
a future adapter cannot accidentally forward incompatible generation settings.
Preview agent names and model availability can change, so they are
configuration data rather than a new stable wire identifier.

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
