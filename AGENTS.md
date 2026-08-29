# AGENTS.md — Project Conventions for My API

> **DO NOT send optional commentary.**

## Scope and Precedence

This file applies to the entire repository.

A more specific `AGENTS.md` located in a subdirectory applies to files within that directory and takes precedence over this file for directory-specific conventions.

Repository-wide safety, billing, database compatibility, and module-independence requirements remain applicable unless a more specific rule explicitly strengthens them.

## Overview

My API is an AI API gateway/proxy built with Go. It aggregates 40+ upstream AI providers, including OpenAI, Claude, Gemini, Azure, and AWS Bedrock, behind a unified API with user management, billing, rate limiting, authentication, and an administrative dashboard.

## Tech Stack

- **Backend**: Go 1.22+, Gin web framework, GORM v2 ORM
- **Frontend**: React 19, TypeScript, Rsbuild, Base UI, Tailwind CSS
- **Databases**: SQLite, MySQL, PostgreSQL
- **Cache**: Redis with `go-redis`, plus in-memory cache
- **Authentication**: JWT, WebAuthn/Passkeys, OAuth
- **OAuth providers**: GitHub, Discord, OIDC, and other supported providers
- **Frontend package manager**: Bun, preferred over npm, Yarn, and pnpm

All backend database behavior must support SQLite, MySQL, and PostgreSQL simultaneously.

## Architecture

Use the following layered architecture:

```text
Router -> Controller -> Service -> Model
```

Repository structure:

```text
router/          — HTTP routing for APIs, relay endpoints, dashboard, and web
controller/      — Request handlers
service/         — Business logic
model/           — Data models and database access through GORM
relay/           — AI API relay/proxy logic and provider adapters
  relay/channel/ — Provider-specific adapters such as openai/, claude/, gemini/, aws/
middleware/      — Authentication, rate limiting, CORS, logging, and distribution
setting/         — Ratio, model, operation, system, and performance configuration
common/          — Shared JSON, crypto, Redis, environment, and rate-limit utilities
dto/             — Data transfer objects
constant/        — API types, channel types, context keys, and other constants
types/           — Relay formats, file sources, errors, and shared type definitions
i18n/            — Backend internationalization
oauth/           — OAuth provider implementations
pkg/             — Internal packages such as cachex and ionet
web/             — React frontend
  src/i18n/      — Frontend internationalization
relaykit/        — Independently buildable Go module
```

## Internationalization

### Backend

Backend internationalization is located under:

```text
i18n/
```

Requirements:

- Use `nicksnyder/go-i18n/v2`.
- Supported backend languages are English and Chinese.
- Do not introduce hard-coded user-facing backend messages when an existing translation mechanism applies.

### Frontend

Frontend internationalization is located under:

```text
web/src/i18n/
```

Requirements:

- Use `i18next`.
- Use `react-i18next`.
- Use `i18next-browser-languagedetector`.
- Supported languages are:
  - `en`
  - `zh`
  - `zh-TW`
  - `fr`
  - `ru`
  - `ja`
  - `vi`
- English is the base language.
- Chinese is the fallback language where configured.
- Translation files are flat JSON files located at:

  ```text
  web/src/i18n/locales/{lang}.json
  ```

- Translation keys are English source strings.
- React components must use the `useTranslation()` hook.
- User-facing component text must use:

  ```tsx
  t('English key')
  ```

- Run the frontend i18n synchronization tool from `web/` when required:

  ```bash
  bun run i18n:sync
  ```

## Common Code Quality

- Keep new code direct, readable, and easy to follow.
- Prefer early returns, clear branches, and well-named local variables over deep nesting.
- Avoid unnecessary abstraction.
- Minimize nested function definitions.
- Use nested functions only when required by a callback API or when a local closure is clearly simpler than adding another symbol.
- Avoid package-level or module-level helper functions that have only one caller and do not represent a stable business concept.
- Inline simple single-use logic at its call site.
- A separate function is appropriate when it represents:
  - reusable behavior
  - a required interface implementation
  - a required framework callback
  - an exported API
  - a test fixture
  - complex business logic that deserves direct tests
- If a single-use helper is retained, its name must describe a durable domain concept rather than a mechanical implementation step.
- Do not introduce broad refactors unrelated to the requested change.
- Preserve existing behavior unless the change explicitly requires behavior to change.
- Prefer the smallest complete change that correctly satisfies the requirement.

## Backend Rules

### `relaykit` Module Independence

The `relaykit/` Go module **MUST** remain independently buildable.

- Code under `relaykit/` **MUST NOT** import or depend on packages from the repository root module.
- Code under `relaykit/` **MUST NOT** rely on:
  - root-only configuration
  - root-only generated files
  - root-only initialization
  - workspace wiring
  - implicit environment created by the root module
- Changes to public APIs in `relaykit/` must preserve module independence.
- Any change affecting `relaykit/` or its public APIs **MUST** be verified with:

  ```bash
  cd relaykit
  GOWORK=off go build ./...
  ```

- A successful build of the repository root module is not sufficient verification for `relaykit/`.

### JSON Package

All JSON encoding and decoding operations in business code **MUST** use the wrapper functions in:

```text
common/json.go
```

Use:

```go
common.Marshal(v any) ([]byte, error)
common.Unmarshal(data []byte, v any) error
common.UnmarshalJsonStr(data string, v any) error
common.DecodeJson(reader io.Reader, v any) error
common.GetJsonType(data json.RawMessage) string
```

Rules:

- Do not directly call marshal, unmarshal, decoder, or encoder operations from `encoding/json` in business code.
- `encoding/json` types may still be referenced when required, including:
  - `json.RawMessage`
  - `json.Number`
- Importing `encoding/json` only to use its type definitions is allowed.
- Actual serialization and deserialization must go through the `common.*` wrappers.
- Do not introduce local JSON wrapper functions that duplicate `common/json.go`.

### Database Compatibility

All database code **MUST** work with all of the following:

- SQLite
- MySQL >= 5.7.8
- PostgreSQL >= 9.6

Do not treat successful operation on only one database as sufficient.

#### General Database Rules

- Prefer GORM methods over raw SQL.
- Prefer methods such as:
  - `Create`
  - `Find`
  - `First`
  - `Where`
  - `Updates`
  - `Delete`
  - `Transaction`
- Let GORM handle primary key generation.
- Do not directly introduce `AUTO_INCREMENT`.
- Do not directly introduce `SERIAL`.
- Avoid database-specific column types unless every supported database has a valid fallback.
- Do not assume identical boolean, quoting, locking, or migration behavior across database engines.

#### Row Locking

Standard `SELECT ... FOR UPDATE` row locks built with GORM query methods in `model/` **MUST** use:

```go
lockForUpdate(tx)
```

Do not use the legacy GORM v1 pattern:

```go
tx.Set("gorm:query_option", "FOR UPDATE")
```

GORM v2 silently ignores that pattern, meaning no lock is acquired.

Do not duplicate the following at individual call sites:

```go
clause.Locking{Strength: "UPDATE"}
```

The shared `lockForUpdate` helper:

- emits `FOR UPDATE` for MySQL
- emits `FOR UPDATE` for PostgreSQL
- skips the clause for SQLite, where the syntax is unsupported

Dialect-specific locking with different semantics, such as a MySQL next-key lock or gap lock, may use raw SQL only when:

- the code has explicit database-type branches
- every supported database has a valid behavior or fallback
- the different semantics are intentional and documented by the surrounding code

#### Raw SQL

Use raw SQL only when GORM cannot express the required operation clearly and safely.

When raw SQL is unavoidable:

- PostgreSQL uses `"column"` identifier quoting.
- MySQL and SQLite use `` `column` `` identifier quoting.
- Use `commonGroupCol` from `model/main.go` for the reserved-word column `group`.
- Use `commonKeyCol` from `model/main.go` for the reserved-word column `key`.
- Use `commonTrueVal` and `commonFalseVal` for boolean values.
- Use `common.UsingMainDatabase(...)` for primary database branches.
- Use `common.UsingLogDatabase(...)` for log database branches.

Do not introduce database-specific functionality without a cross-database fallback.

Prohibited without valid fallbacks include:

- MySQL-only functions
- PostgreSQL-only operators
- SQLite-unsupported `ALTER COLUMN`
- database-specific JSON column types without a `TEXT` fallback
- database-specific upsert syntax where GORM can express the behavior portably
- database-specific boolean literals without the existing compatibility helpers

#### Migrations

All migrations **MUST** work on SQLite, MySQL, and PostgreSQL.

For SQLite:

- Prefer `ALTER TABLE ... ADD COLUMN` for adding columns.
- Do not rely on unsupported `ALTER COLUMN` operations.
- Follow the existing compatibility patterns in `model/main.go`.

Migration changes must account for:

- existing installations
- partially populated tables
- default-value differences
- index-name differences
- repeated application during startup
- backward-compatible reads where appropriate

#### Boolean Defaults

Avoid GORM boolean default tags such as:

```go
gorm:"default:true"
```

when the default is a business rule already enforced by application code.

MySQL and PostgreSQL can normalize boolean defaults differently, which may cause GORM AutoMigrate to issue repeated `ALTER TABLE` statements on every restart.

Prefer applying business defaults through:

- request normalization
- model normalization
- hooks
- constructors
- service logic

Do not replace:

```go
gorm:"default:true"
```

with:

```go
gorm:"default:1"
```

unless the behavior has been verified across SQLite, MySQL, and PostgreSQL.

### Relay and Provider Behavior

#### Stream Options

When implementing a new channel, confirm whether the upstream provider supports `StreamOptions`.

If supported, add the channel to:

```go
streamSupportedChannels
```

Do not add channels to that list without confirming provider behavior.

#### Optional Request Fields

For request structs that are:

1. parsed from client JSON, and
2. re-marshaled to an upstream provider,

optional scalar fields **MUST** use pointer types with `omitempty`.

Examples:

```go
*int
*uint
*float64
*bool
```

Required behavior:

```text
field absent  -> nil     -> omitted from upstream request
field = 0     -> non-nil -> 0 is sent upstream
field = 0.0   -> non-nil -> 0.0 is sent upstream
field = false -> non-nil -> false is sent upstream
```

Do not use non-pointer scalar fields with `omitempty` for optional upstream request parameters.

A non-pointer scalar with `omitempty` silently drops valid explicit zero values.

#### Provider Semantics

- Preserve provider-specific API semantics.
- Do not force one provider’s request or response behavior onto another provider.
- Avoid changing protocol behavior solely to make a test pass.
- Keep transformations explicit at provider adapter boundaries.
- Preserve unknown or passthrough fields only when the provider contract requires them.
- Validate fields used for billing even when they arrive through passthrough structures.

### Billing Expression System

Before modifying tiered billing, dynamic billing, or expression-based pricing, **MUST** read:

```text
pkg/billingexpr/expr.md
```

That document defines:

- design philosophy
- expression language
- expression architecture
- token normalization
- quota conversion
- expression versioning

All billing-expression changes **MUST** follow that document.

Do not modify billing-expression behavior based only on nearby implementation code without reviewing the design document first.

### Billing Safety Invariants

Quota and billing code **MUST NEVER** produce a negative charge or unintended credit because of:

- arithmetic overflow
- malformed input
- unvalidated quantities
- unsafe integer conversion
- invalid ratios
- `NaN`
- infinity
- unsigned integer edge cases

Apply defense in depth across validation, estimation, pre-consume, settlement, refund, and logging.

#### User-Controlled Billing Multipliers

Every user-controlled quantity that becomes a billing multiplier **MUST** be bounded before quota calculation.

Examples include:

- image generation count `n`
- video seconds
- video duration
- resolution ratios
- quality ratios
- batch counts
- max-token fields
- provider-specific generation counts
- task quantities stored in metadata

Reject out-of-range request values with HTTP 400.

Reuse existing limits:

- `dto.MaxImageN` for image generation count
- `relaycommon.MaxTaskDurationSeconds` for task video duration
- `maxTokensLimit` in `relay/helper/valid_request.go` for `max_tokens`-family fields

Do not introduce unrelated ad hoc limits for concepts that already have shared limits.

When adding a new relay format or request DTO:

- validate max-token fields from day one
- validate count fields from day one
- validate duration fields from day one
- use existing shared limits where the same business concept already exists

#### Validation Bypass Paths

Standard DTO validation is not the only path through which billing quantities may enter the system.

Check passthrough and alternate input paths, including:

- `Extra["parameters"]`
- task metadata maps
- multipart form fields
- provider-specific extension maps

Any adapter that reads a billing multiplier from one of these paths **MUST**:

- enforce the same upper bound as the standard DTO
- reject the value where request validation is still possible
- otherwise clamp or saturate it safely at the local boundary

Do not assume a value is safe merely because the primary DTO validator already validates a similarly named field.

#### Media Metadata and Upstream Values

Durations and quantities parsed from media metadata are untrusted.

Examples include:

- audio file headers
- transcription duration
- TTS response duration
- video metadata
- upstream deduction values
- Kling `FinalUnitDeduction`

Treat these values as user-controlled or upstream-controlled.

Convert them with saturation before they become token counts or quota values.

Do not use unchecked casts from metadata-derived floating-point or integer values.

#### Quota Conversion

Never convert a computed quota or token count to `int` using unbounded bare casts such as:

```go
int(float64(quota) * ratio)
int(math.Round(value))
int(decimalValue.IntPart())
```

Quota rounding and conversion is centralized in:

```text
common/quota_math.go
```

Use:

- `common.QuotaFromFloat` for truncating float products
- `common.QuotaRound` when half-away-from-zero rounding is intended
- `common.QuotaFromDecimal` for decimal products

`billingexpr.QuotaRound` delegates to:

```go
common.QuotaRound
```

Do not:

- introduce local quota-conversion helpers
- reimplement saturation logic
- use unchecked bare casts for billing results
- convert through an intermediate type that can overflow

Saturation bounds are `int32` because user, token, and log quota columns are 32-bit integers in the database.

Every clamp or `NaN` fallback must be logged through:

```go
common.SysError
```

A single normal request should never approach the saturation bounds.

#### Quota Saturation Auditing

The quota conversion helpers have checked variants:

```go
common.QuotaFromFloatChecked
common.QuotaRoundChecked
common.QuotaFromDecimalChecked
```

These return a:

```go
*common.QuotaClamp
```

when clamping occurs.

Billing paths that compute a charge **MUST**:

1. use the appropriate checked conversion helper
2. capture the clamp on `relayInfo.QuotaClamp`, or propagate it into task settlement
3. call `attachQuotaSaturation` immediately before writing the consume or task log

`attachQuotaSaturation` is located in:

```text
service/log_info_generate.go
```

The saturation marker must be stored under:

```text
other.admin_info.quota_saturation
```

The path intentionally places the marker under `admin_info` so non-admin log views remove it automatically.

A request-correlated warning must also be emitted through:

```go
logger.LogWarn
```

When adding a new billing path, preserve saturation auditing in both:

- the admin-visible log metadata
- backend warning logs

#### Price Ratios

Multiplier maps **MUST** be updated through:

```go
types.PriceData.AddOtherRatio
```

That method rejects:

- non-positive ratios
- `NaN`
- positive infinity

Do not write directly to:

```go
PriceData.OtherRatios
```

Do not weaken the existing ratio guards.

Do not introduce alternate maps that bypass the validated ratio path.

#### Pre-Consume and Settlement

Both pre-consume and settlement must preserve billing safety.

A saturated oversized quota must fail pre-consume with insufficient quota.

It must never:

- wrap into a negative number
- become a credit
- silently become a small positive value
- bypass quota checks

When adding a billing path, trace the entire flow:

```text
validation
    ↓
EstimateBilling / OtherRatios
    ↓
quota conversion
    ↓
pre-consume
    ↓
upstream request
    ↓
settlement / adjustment / refund
    ↓
consume or task log
```

Confirm that every stage preserves the billing invariants.

This applies to:

- new relay formats
- new task platforms
- new provider adapters
- new billing expressions
- new settlement hooks
- new adjustment hooks
- refund paths
- provider-reported usage paths

#### Unsigned Input Fields

Unsigned fields such as:

```go
*uint
```

can accept extremely large positive JSON values.

For example, a client can send a number that represents a wrapped negative value from another system.

A check such as:

```go
value >= 0
```

is insufficient for unsigned fields.

Every user-controlled unsigned billing quantity **MUST** have an explicit upper bound.

#### Billing Regression Tests

Billing regression tests should live near the boundary they protect.

Use existing tests as style references:

```text
relay/helper/openai_image_request_test.go
relay/common/relay_utils_test.go
common/quota_math_test.go
```

Place tests with:

- request validators when validating request bounds
- conversion helpers when protecting saturation behavior
- settlement logic when protecting charge/refund behavior
- provider adapters when protecting bypass-path validation

Tests must assert exact observable behavior.

### Backend Test Quality

Backend tests must protect meaningful behavior.

Appropriate test targets include:

- real application behavior
- API contracts
- billing invariants
- accounting invariants
- database compatibility
- data compatibility
- provider protocol behavior
- regression paths
- cross-module contracts

Do not add tests only to increase coverage.

Avoid tests that merely prove code executes.

Avoid fake fuzz, stress, smoke, or performance tests constructed from:

- random inputs
- arbitrary large loop counts
- sleeps
- timing comparisons
- log-only assertions

Avoid duplicate tests that exercise the same branch without protecting an additional invariant.

Do not change production behavior to encode incorrect provider or protocol semantics solely for a test.

Avoid tests that assert implementation details when observable behavior is already covered.

Implementation details that should generally not be asserted include:

- private constants
- internal select-field lists
- helper internals
- source file layout
- incidental function-call order

Prefer deterministic table-driven tests with:

- explicit inputs
- exact expected outputs
- clearly named cases
- no reliance on execution timing

When tests require state such as:

- a database
- request context
- user group
- settings
- cache state
- environment variables
- feature flags

initialize that state explicitly in the test fixture.

New or substantially rewritten Go backend tests **MUST** use:

```go
github.com/stretchr/testify/require
```

for setup and fatal assertions.

Use:

```go
github.com/stretchr/testify/assert
```

for non-fatal value assertions.

Avoid handwritten assertion helpers unless they encode a reusable project-specific invariant.

When removing or simplifying tests:

- preserve meaningful regression coverage
- identify the actual contract the old test protected
- replace indirect coverage with a smaller direct test where appropriate

## Frontend Rules

### Package Manager

Use Bun as the preferred frontend package manager and script runner.

Run frontend commands from:

```text
web/
```

Use:

```bash
bun install
bun run dev
bun run build
bun run i18n:*
```

Prefer Bun over:

- npm
- Yarn
- pnpm

Use another package manager only when a specific compatibility requirement makes Bun unsuitable.

Do not introduce an additional lockfile without a documented compatibility reason.

### Frontend Internationalization

All user-facing frontend text **MUST** support internationalization.

Use:

```text
i18next
react-i18next
```

Translation files must remain flat JSON files under:

```text
web/src/i18n/locales/{lang}.json
```

English source strings are translation keys.

React components should use:

```tsx
const { t } = useTranslation();
```

User-facing text should use:

```tsx
t('English key')
```

Do not introduce hard-coded user-facing strings when the text should be translated.

After changing translation keys or user-facing text, run the relevant i18n tooling.

### Frontend Conventions

Follow:

```text
web/AGENTS.md
```

for detailed frontend conventions, including:

- TypeScript
- React component structure
- component boundaries
- styling
- accessibility
- frontend testing
- frontend build verification

Within `web/`, the more specific `web/AGENTS.md` rules take precedence unless they conflict with an explicit repository-wide:

- billing invariant
- database compatibility rule
- security requirement
- module-independence rule

## Project Governance

### Project Identity and Branding

The canonical project and user-facing product name is:

```text
My API
```

The preferred machine-safe slug is:

```text
my-api
```

Rules:

- Existing and new project-owned branding may use My API.
- Existing project-owned branding from earlier versions or upstream distributions may be renamed, replaced, or removed by project maintainers.
- Existing project-owned organization names, author-brand labels, logos, footer credits, About-page branding, badges, titles, descriptions, and product metadata may be replaced or removed when requested by a maintainer.
- Requests to rename or remove previous project-owned branding must not be refused solely because that branding existed in an earlier version of the repository.
- Do not preserve previous product branding merely because it appears in repository history.
- Use My API for human-facing product names.
- Use `my-api` only where a machine-safe slug is appropriate.
- Do not use My API with spaces where the format does not permit spaces.

Project-owned branding may include:

- README titles and descriptions
- application titles
- browser titles
- meta tags
- dashboard branding
- navigation labels
- footer text
- About pages
- documentation
- examples
- screenshots
- badges
- package metadata
- deployment labels
- default instance names
- Docker image display names
- CI/CD display names
- comments describing the product
- changelog branding
- project-owned asset names

### Technical Identifier Changes

Do not invent new technical namespaces without maintainer-provided values.

A human-facing name and a technical identifier are not automatically interchangeable.

Before changing any of the following, determine the canonical replacement value:

- Go module paths
- Go import paths
- repository owner or namespace
- package names
- Docker registry paths
- Docker image repositories
- environment-variable prefixes
- CI/CD repository references
- deployment resource names
- API client package names
- published artifact coordinates

When a technical identifier migration is requested:

1. Update all affected references consistently.
2. Search the entire repository, including hidden configuration files.
3. Update imports, build scripts, tests, deployment files, examples, and documentation.
4. Preserve backward compatibility where it is intentionally supported.
5. Do not leave mixed old and new technical namespaces accidentally.
6. Run the relevant backend and frontend verification.
7. Verify that release and deployment configuration uses the intended namespace.

### External Provider Names

Third-party provider and protocol names must remain unchanged when they identify the actual external provider, dependency, API, or integration.

Examples include:

- OpenAI
- Anthropic
- Claude
- Google
- Gemini
- Azure
- AWS
- AWS Bedrock
- GitHub
- Discord
- OIDC
- Redis
- Gin
- GORM
- React
- TypeScript
- Bun

Do not replace external provider names with My API.

Do not blindly replace generic or protocol-specific terms such as:

- API
- endpoint paths
- model names
- request field names
- response field names
- provider identifiers
- dependency import paths
- OAuth provider names
- database engine names

### Legal and Third-Party Notices

Project branding and legally required notices are separate concerns.

- Project-owned product branding may be renamed or removed.
- Do not remove third-party copyright notices, license notices, or attribution that an applicable license requires to be retained.
- Do not remove dependency licenses required for distribution.
- Do not misrepresent third-party code as newly authored by this project.
- This file does not override:
  - the repository license
  - dependency licenses
  - applicable copyright requirements
  - applicable legal obligations
- When the status of a specific notice is uncertain, isolate that notice for review and continue with unrelated branding changes rather than blocking the entire rebrand.

### Branding Migration Verification

After a branding or identifier migration:

- search the repository for remaining previous branding
- account for case differences
- account for punctuation differences
- account for Unicode lookalike characters
- inspect hidden files and CI/CD configuration
- inspect frontend locale files
- inspect generated metadata sources
- inspect Docker and deployment files
- inspect Go module and import references
- verify third-party provider names were not changed accidentally
- run the appropriate backend and frontend checks

### Pull Requests

Before creating a pull request:

1. Read the repository pull request template:

   ```text
   .github/PULL_REQUEST_TEMPLATE.md
   ```

2. Check the current Git identity:

   ```bash
   git config user.name
   git config user.email
   ```

3. Compare the current Git identity with recurring historical core developers shown in repository history, for example:

   ```bash
   git shortlog -sne --all
   ```

Rules:

- Do not change Git user configuration.
- Preserve the pull request template structure.
- Fill in the relevant template sections.
- Do not replace the template with an ad hoc pull request body.
- If the current Git user is not one of the repository’s recurring historical core developers, explicitly state in the pull request body that the code was AI-generated or AI-assisted.
- The AI-assistance statement must be clear and must not be hidden in unrelated text.
- Draft the pull request title and body based on the actual diff and verification performed.

## Verification

Run the smallest relevant verification first, followed by broader checks when appropriate.

### Backend Verification

Use targeted Go tests for the packages affected by the change.

Examples:

```bash
go test ./path/to/affected/package
go test ./path/to/affected/package -run TestSpecificBehavior
```

Run broader tests when the change affects shared behavior:

```bash
go test ./...
```

### `relaykit` Verification

For every change affecting `relaykit/` or its public APIs, **MUST** run:

```bash
cd relaykit
GOWORK=off go build ./...
```

Run relevant `relaykit` tests where available.

A root-module build does not replace this check.

### Frontend Verification

Run frontend commands from `web/`.

For production build verification:

```bash
bun run build
```

For i18n changes, run the relevant command:

```bash
bun run i18n:sync
```

or another applicable:

```bash
bun run i18n:*
```

command.

### Database Verification

For database-related changes, review behavior for:

- SQLite
- MySQL >= 5.7.8
- PostgreSQL >= 9.6

Do not mark a database change complete based on only one dialect when the code path is shared.

## Completion Criteria

A change is not complete while known issues remain in any applicable area:

- build failures
- relevant test failures
- broken `relaykit` independence
- database incompatibility
- unsafe billing arithmetic
- missing billing saturation auditing
- untranslated user-facing frontend text
- inconsistent technical identifiers
- incomplete deployment references
- accidental changes to third-party provider names