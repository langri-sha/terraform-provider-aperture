# AGENTS.md

Context for AI coding agents working in this repo.

## What this is

A Terraform provider, written in Go using the
[Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework),
for [Aperture by Tailscale](https://tailscale.com/docs/aperture). See
[`README.md`](./README.md) for the user-facing description.

## Upstream surface

Aperture exposes an HTTP admin API. Source of truth is the OpenAPI
document at `/aperture/openapi.json` on a live gateway; the relevant
operations are:

- `GET /aperture/config` → `{ config: <hujson> }` + `ETag` header.
  API keys in provider blocks are redacted server-side.
- `PUT /aperture/config` → If-Match header required (uses the ETag
  from a prior GET). Body is `{ config: <hujson> }`. Returns 412 on
  ETag mismatch, 422 on validation error.
- `POST /aperture/config:validate` → `{ valid, errors }`. Useful as
  a pre-flight check before PUT.

Auth is by Tailscale identity at the network layer. The provider
sends no Authorization header. The caller must be on the tailnet
with the admin role grant either in Aperture's own grants[] or in
the tailnet ACL.

The configuration document is a singleton: one HuJSON document per
gateway. Modeled in HCL as `resource "aperture_config" "main"`.

Top-level keys (mirror upstream verbatim — do not rename):

- `providers` — map of LLM provider configs (`baseurl`, `models[]`,
  `apikey`, `authorization`, `name`, `description`, `cost_basis`,
  `preference`, `disabled`, `add_headers`).
- `grants[]` — Tailscale grant entries: `src[]` and
  `app["tailscale.com/cap/aperture"][]` capabilities (`role`,
  `models`).
- `quotas` — token-bucket spend limits (`capacity`, `rate`,
  `on_exceed`).
- `hooks` — webhook configs (`url`, `apikey`, `authorization`,
  `timeout`, `disabled`, `fail_policy`, `preference`).
- `auto_cost_basis` — boolean.

Don't invent fields. If upstream doesn't document it, leave it out.

## Layout

```
cmd/terraform-provider-aperture/   # main.go — providerserver entrypoint
internal/aperture/                  # HTTP client (config.go: wire types; client.go: GET/PUT/validate)
internal/provider/                  # plugin-framework provider + resources
examples/                           # terraform-plugin-docs example layout + quickstart
docs/                               # generated docs
scripts/tfc-release.sh              # release pipeline for TFC private providers
.github/workflows/                  # CI
```

## Conventions

- **Atomic commits.** One logical change per commit.
- **No idempotent fluff.** No `cmd || true`, no swallowed errors, no
  "tolerate pre-existing state" patterns. Strict failure with a clear
  error beats silent recovery.
- **Don't speculate, verify.** Before claiming "Aperture supports X",
  read `/aperture/openapi.json` from a live gateway or
  `tailscale/aperture-cli`'s source. Web-fetched marketing pages are
  not authoritative.
- **Comments explain *why*, not *what*.** The reader can see what the
  code does. They can't see why a field is `Optional` instead of
  `Required`, or why we preserve apikeys from prior state on Read.
- **No emojis** unless explicitly requested.
- **HCL field names mirror upstream JSON.** `baseurl`, not `base_url`;
  `apikey`, not `api_key`. Keep HCL → HuJSON one-to-one wherever
  possible so users can cross-reference Aperture docs without
  translation.

## Sensitive-field handling

`GET /config` returns redacted apikeys. The Read method calls
`preserveSensitiveFromPrior` to keep the user-supplied values from
state instead of overwriting with the redaction marker. If you add a
new sensitive field to the schema, update the merge function or the
provider will leak redacted values into `terraform plan` output.

## Common commands

```sh
go mod tidy
go vet ./...
go test ./...

# Format example HCL
terraform fmt -recursive examples/

# Generate docs (once tfplugindocs is wired up)
go generate ./...

# Release to TFC private provider registry
op run --env-file=.tfc-release.env -- scripts/tfc-release.sh 0.2.0
```

## Releasing

The provider is distributed via TFC private providers, not the public
registry, while it's pre-1.0. `scripts/tfc-release.sh` does the
build/sign/upload dance end-to-end. Required env vars — documented
in `scripts/tfc-release.sh` — set in `.tfc-release.env` (gitignored):

- `TFC_ORG`, `TFC_TOKEN` — TFC org and a user/team token with
  registry-providers write permission.
- `GPG_KEY_ID` — 40-char fingerprint of the signing key. The secret
  key must be importable locally; bootstrap from 1Password:
  `op read 'op://Rashadnyk/Aperture TFC Provider Signing Key/private_key' | gpg --import`.

Provider uses no encryption subkey by design (sign-only). Don't
recreate it — the fingerprint is referenced in `versions.tf`
consumers.
