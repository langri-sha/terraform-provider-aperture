# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
