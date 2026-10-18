# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] — 2026-10-18

### Breaking Changes

- Quota `capacity` and `rate` are dollar strings (`"$10.00"`, `"$5.00/day"`)
  instead of numbers, and `on_exceed` accepts only `"reject"`, as the gateway
  requires.
- Provider `add_headers` is a list of `"Header-Name: value"` strings instead of
  a map.
- `apikey` is never read from the gateway, which redacts it. After an import it
  is null in state. An apply that would overwrite an `apikey` the gateway holds
  for a provider or hook, while the configuration sets none, fails with
  `aperture credential not managed`; set `apikey` or remove the provider or
  hook.
- `authorization` is no longer marked sensitive and is read back from the
  gateway.

### Added

- Writes overlay the configuration onto the gateway's current document, so
  keys the provider does not model keep the gateway's value at any depth.
  Importing a gateway and applying no longer deletes `auth_mode`,
  `compatibility`, `connectors` or whatever Aperture adds next.
- Provider `auth_mode`, `compatibility` and `model_cost_map`.
- Grant `dst`, and the capabilities `connectors`, `quotas`, `send_hooks`,
  `add_headers`, `filterable_headers`, `enable_portal`, `read_metrics`,
  `set_cors` and `cors_paths`.
- `exporters`, `database`, `connectors`, `chat_models`, `flags`, `mcp` and
  `temp_grants` as JSON attributes, left alone while unset.
- Guides for Claude Code, Codex and importing an existing gateway.

### Fixed

- The provider accepts `http://` endpoints again. Aperture serves its admin API
  over plain HTTP on the tailnet — upstream's own launcher defaults to
  `http://ai` — and authenticates callers by Tailscale identity at the network
  layer, so the HTTPS-only validation added in 0.2.1 rejected the correct
  endpoint shape (`http://ai.<tailnet>.ts.net/aperture`). `https://` stays
  accepted for gateways fronted with a Tailscale TLS certificate; every other
  scheme is still rejected, as is an endpoint whose path doesn't end in
  `/aperture`. The redirect-following, response-size, and TLS 1.3 minimum
  hardening from 0.2.1 is unchanged.
- Importing a gateway whose grants carry a capability with neither `role` nor
  `models`, such as `{"connectors": ["aperture/**"]}`, no longer fails.
- Updates no longer fail with an inconsistent result after apply: the `etag`
  changes on every write.
- A provider without `add_headers` no longer plans a change from an empty list
  to null.

### Upgrade Notes

Quote quota amounts and write `add_headers` as a list. Declare an `apikey` for
each provider or hook that holds one, or remove it, before the first apply
after upgrading; `terraform plan` works without them.

## [0.3.1] — 2026-06-03

### Changed

- Publishing moved to the public Terraform Registry (`langri-sha/aperture`),
  built and signed by GoReleaser and GitHub Actions when a GitHub Release is
  published. The Terraform Cloud private-registry release path
  (`scripts/tfc-release.sh`) is removed.

### Added

- Generated provider documentation under `docs/` via terraform-plugin-docs
  (`scripts/generate-docs.sh`).

## [0.3.0] — 2026-05-20

### Breaking Changes

- Renamed the provider from `aperature` to `aperture`, correcting a spelling
  error. This changes the provider source address to `langri-sha/aperture`,
  the resource type to `aperture_config`, and the Go module path to
  `github.com/langri-sha/aperture`.

### Upgrade Notes

Update your `required_providers` source and resource types:
```hcl
terraform {
  required_providers {
    aperture = {
      source  = "langri-sha/aperture"
      version = "~> 0.3"
    }
  }
}

resource "aperture_config" "main" { # was: aperature_config
  # ...
}
```

State migration: rename the resource in state with
`terraform state mv aperature_config.main aperture_config.main` (or remove and
re-import via `terraform import aperture_config.main default`).

## [0.2.1] — 2026-05-19

### Breaking Changes

- The provider now requires HTTPS for all Aperture endpoints. HTTP endpoints are no longer supported. Update your endpoint configuration from `http://` to `https://`.

### Fixed

- Fixed HTTP redirect following vulnerability in HTTP client
- Fixed missing HTTPS/TLS enforcement
- Fixed missing endpoint URL validation
- Fixed unbounded JSON decoder responses

### Upgrade Notes

Change your provider configuration from:
```hcl
endpoint = "http://ai.${var.tailnet}/aperture"
```

to:
```hcl
endpoint = "https://ai.${var.tailnet}/aperture"
```

## [0.2.0] — 2026-05-08

### Changed

- Rewrote provider as a singleton `aperture_config` resource that talks to
  the real Aperture admin API (`GET /aperture/config`, `PUT /aperture/config`,
  `POST /aperture/config:validate`). Prior data-source scaffolds removed.
- Refreshed `examples/quickstart/` and `examples/resources/` for the v0.2
  singleton architecture.
- README updated with full HCL usage example, import instructions, and layout.

## [0.1.0] — 2026-05-07

### Added

- Go module bootstrap (`github.com/langri-sha/aperture`).
- Internal Aperture HTTP client and typed wire structs.
- Provider entrypoint with `aperture_config` data source and resource
  scaffolds (terraform-plugin-framework).
- CI: `go test -race`, `go vet`, `terraform fmt -check` via GitHub Actions.
- `scripts/tfc-release.sh` for building and uploading to a TFC private
  provider registry.
- `examples/` layout following terraform-plugin-docs conventions.
