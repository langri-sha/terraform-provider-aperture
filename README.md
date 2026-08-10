# terraform-provider-aperture

[![Test](https://github.com/langri-sha/terraform-provider-aperture/actions/workflows/test.yml/badge.svg)](https://github.com/langri-sha/terraform-provider-aperture/actions/workflows/test.yml)

A Terraform provider for [Aperture by Tailscale](https://tailscale.com/docs/aperture),
the AI gateway that brokers LLM requests for a tailnet.

## Status

Pre-1.0 but functional. The provider talks to Aperture's documented
admin API (`GET /aperture/config`, `PUT /aperture/config`,
`POST /aperture/config:validate`) and exposes a single
`aperture_config` resource.

## Surface

| Type | Name | Purpose |
|---|---|---|
| Resource | `aperture_config` | The singleton configuration document. CRUD via the admin API; `terraform import aperture_config.main default` works. |

The HCL schema mirrors Aperture's HuJSON keys verbatim — `providers`,
`grants`, `quotas`, `hooks`, `auto_cost_basis`. See
[`docs/`](./docs) and [`examples/`](./examples).

## Usage

```hcl
terraform {
  required_providers {
    aperture = {
      source  = "langri-sha/aperture"
      version = "~> 0.3"
    }
  }
}

provider "aperture" {
  endpoint = "http://ai.${var.tailnet}/aperture"
  # Auth is by Tailscale identity at the network layer — the runner
  # must be on the tailnet with the admin role granted. No API key.
}

resource "aperture_config" "main" {
  providers = {
    openai = {
      baseurl = "https://api.openai.com/v1"
      models  = ["openai/gpt-5.5"]
      apikey  = var.openai_api_key
    }
  }
  grants = [{
    src = ["group:developers"]
    capabilities = [
      { role = "user" },
      { models = "**" },
    ]
  }]
}
```

See [`examples/quickstart/`](./examples/quickstart) for a complete
tailnet + Aperture setup including the tailscale ACL.

### Endpoint Configuration

The provider needs the full base URL of Aperture's admin API.

**Format:** `http://host/aperture`

- **HTTP is the expected scheme** — Aperture serves its admin API over plain HTTP on the tailnet, where WireGuard already provides transport encryption and caller identity. `https://` is also accepted, for gateways fronted with a Tailscale TLS certificate. Any other scheme is rejected.
- **The `/aperture` path suffix is mandatory** — this routes to the admin API endpoints (`GET /aperture/config`, `PUT /aperture/config`, `POST /aperture/config:validate`).
- **Authentication** is handled via Tailscale identity at the network layer. No API key is needed. The Terraform runner must be on the tailnet with the appropriate admin role granted.

**Example:**
```hcl
provider "aperture" {
  endpoint = "http://ai.your-tailnet.ts.net/aperture"
}
```

### Importing an existing live config

```sh
echo 'resource "aperture_config" "main" {}' > main.tf
terraform import aperture_config.main default
```

The post-import Read pulls the entire HuJSON document from the
gateway. API keys come back redacted, so fill those into HCL
pointing at your secret store before the next `plan`.

## Contributing

Bug reports and pull requests are welcome. See [CONTRIBUTING.md](./CONTRIBUTING.md)
for the repository layout, local development, and the release process.

## License

Apache 2.0. See [`LICENSE`](./LICENSE).
