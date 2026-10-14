# Agents orientation — `langri-sha/terraform-provider-aperture`

A Terraform provider, written in Go using the
[Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework),
for [Aperture by Tailscale](https://tailscale.com/docs/aperture). See
[`README.md`](./README.md) for the user-facing description and
[`CONTRIBUTING.md`](./CONTRIBUTING.md) for the layout, the development commands
and the checks every change must pass.

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

## Conventions

- **No idempotent fluff.** No `cmd || true`, no swallowed errors, no
  "tolerate pre-existing state" patterns. Strict failure with a clear
  error beats silent recovery.
- **Don't speculate, verify.** Before claiming "Aperture supports X",
  read `/aperture/openapi.json` from a live gateway or
  `tailscale/aperture-cli`'s source. Web-fetched marketing pages are
  not authoritative.
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

## Release

Publishing a GitHub Release for a `vX.Y.Z` tag releases the provider: the
Release workflow builds and signs it with GoReleaser, and the public Terraform
Registry ingests it as `langri-sha/aperture`. Follow the Releasing steps in
[`CONTRIBUTING.md`](./CONTRIBUTING.md), which start with the hand-kept
`CHANGELOG.md`.
