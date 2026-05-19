# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

- Rewrote provider as a singleton `aperature_config` resource that talks to
  the real Aperture admin API (`GET /aperture/config`, `PUT /aperture/config`,
  `POST /aperture/config:validate`). Prior data-source scaffolds removed.
- Refreshed `examples/quickstart/` and `examples/resources/` for the v0.2
  singleton architecture.
- README updated with full HCL usage example, import instructions, and layout.

## [0.1.0] — 2026-05-07

### Added

- Go module bootstrap (`github.com/langri-sha/aperature`).
- Internal Aperture HTTP client and typed wire structs.
- Provider entrypoint with `aperature_config` data source and resource
  scaffolds (terraform-plugin-framework).
- CI: `go test -race`, `go vet`, `terraform fmt -check` via GitHub Actions.
- `scripts/tfc-release.sh` for building and uploading to a TFC private
  provider registry.
- `examples/` layout following terraform-plugin-docs conventions.
