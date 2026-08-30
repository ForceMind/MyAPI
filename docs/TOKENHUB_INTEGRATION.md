# TokenHub integration boundary

MyAPI treats [TokenHub](https://github.com/astaxie/TokenHub) as an explicit
upstream gateway rather than embedding its runtime. The integration keeps
request conversion, billing, routing and persistence in MyAPI while exposing a
small provider-neutral configuration contract.

## Configuration

TokenHub settings are stored in a channel's `settings` JSON under `tokenhub`.
The channel credential remains in the existing secret field and is never
included in settings, model metadata, errors or health responses.

```json
{
  "tokenhub": {
    "enabled": true,
    "base_url": "https://tokenhub.example.invalid",
    "region": "default",
    "protocol": "openai",
    "model_list_path": "/v1/models",
    "health_path": "/health",
    "allow_model_discovery": true,
    "model_aliases": {
      "fast": "provider-model-id"
    }
  }
}
```

Supported protocol values are `openai`, `responses`, and `anthropic`. An
explicit TokenHub setting is currently supported on the legacy Tencent-
compatible channel (wire ID 23) so existing channel records remain valid. The
`anthropic` option selects the existing Claude Messages adaptor. Native
Anthropic traffic uses the Messages API; OpenAI Responses requests may use
MyAPI's tested Responses-to-Claude Messages converter when the routing policy
selects that compatibility path. This does not add Claude account login or
quota access. A missing setting preserves the legacy Tencent key-shape dispatch;
invalid settings are rejected when the channel is saved.

## Model and health probes

The `relay/channel/tokenhub` package provides bounded model discovery and a
health probe for administrative services. Model discovery is opt-in through
`allow_model_discovery`; no request is made when it is disabled. Probe errors
contain HTTP status or a generic failure reason and never echo the API key or
response body.

These probes are metadata primitives, not a second relay stack. Routing,
quota, cost accounting and audit events continue through MyAPI's existing
Router → Controller → Service → Model layers.

## Compatibility and scope

- Existing channel type and API type IDs are unchanged.
- No local credential files are read.
- TokenHub source is not copied into this repository; any future copied code
  requires Apache-2.0 and NOTICE review.
- Region, routing weights, billing reconciliation and UI management remain
  follow-up work built on this boundary.

## Google Antigravity is a separate protocol

Google Antigravity's managed agent uses the preview Gemini Interactions API,
not the normal Generate Content routes that a TokenHub OpenAI/Anthropic
gateway can proxy. MyAPI therefore does not advertise an Antigravity model in
the TokenHub channel or silently route it through the Gemini adaptor. The
validated request boundary and the explicit implementation gate are documented
in [ANTIGRAVITY_INTEGRATION.md](./ANTIGRAVITY_INTEGRATION.md). This keeps
TokenHub's provider-neutral routing contract honest while leaving room for a
future dedicated interaction lifecycle.
