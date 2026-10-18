# terraform-provider-aperture

[![Test](https://github.com/langri-sha/terraform-provider-aperture/actions/workflows/test.yml/badge.svg)](https://github.com/langri-sha/terraform-provider-aperture/actions/workflows/test.yml)

A Terraform provider for [Aperture by Tailscale](https://tailscale.com/docs/aperture),
the AI gateway that brokers LLM requests for a tailnet.

It manages Aperture's whole configuration as a single `aperture_config`
resource, whose attributes mirror Aperture's own config keys: `providers`,
`grants`, `quotas`, `hooks` and `auto_cost_basis`, with `exporters`,
`database` and the other JSON-heavy sections as JSON attributes. Keys the
provider does not model are left as the gateway holds them. Pre-1.0, but
functional.

## Usage

```hcl
terraform {
  required_providers {
    aperture = {
      source  = "langri-sha/aperture"
      version = "~> 0.4"
    }
  }
}

provider "aperture" {
  endpoint = "http://ai.${var.tailnet}/aperture"
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

The `endpoint` is the gateway's admin API, `http://<host>/aperture`; the
`/aperture` suffix is required. `https://` also works for a gateway with a
Tailscale TLS certificate. There's no API key: Aperture identifies callers by
their Tailscale identity, so Terraform must run on the tailnet with the admin
role.

See [`docs/`](./docs) for the full schema, and
[`examples/quickstart/`](./examples/quickstart) for a complete tailnet and
Aperture setup, including the Tailscale ACL.

## Importing an existing config

```sh
echo 'resource "aperture_config" "main" {}' > main.tf
terraform import aperture_config.main default
```

The gateway redacts API keys, so the provider never reads them. A plan after
import is clean; an apply that would overwrite a key the gateway holds fails
until the configuration sets it or removes its provider or hook. See
[importing an existing gateway](./docs/guides/import-an-existing-gateway.md).

## Guides

- [Use Claude Code with Aperture](./docs/guides/claude-code.md)
- [Use Codex with Aperture](./docs/guides/codex.md)
- [Import an existing gateway](./docs/guides/import-an-existing-gateway.md)

## Contributing

Bug reports and pull requests are welcome. See [CONTRIBUTING.md](./CONTRIBUTING.md)
for the repository layout, local development, and the release process.

## License

Apache 2.0. See [`LICENSE`](./LICENSE).
