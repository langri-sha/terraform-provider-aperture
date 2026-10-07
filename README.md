# terraform-provider-aperture

[![Test](https://github.com/langri-sha/terraform-provider-aperture/actions/workflows/test.yml/badge.svg)](https://github.com/langri-sha/terraform-provider-aperture/actions/workflows/test.yml)

A Terraform provider for [Aperture by Tailscale](https://tailscale.com/docs/aperture),
the AI gateway that brokers LLM requests for a tailnet.

It manages Aperture's whole configuration as a single `aperture_config`
resource, whose attributes mirror Aperture's own config keys: `providers`,
`grants`, `quotas`, `hooks` and `auto_cost_basis`. Pre-1.0, but functional.

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

API keys come back redacted, so point them at your secret store before the next
`plan`.

## Contributing

Bug reports and pull requests are welcome. See [CONTRIBUTING.md](./CONTRIBUTING.md)
for the repository layout, local development, and the release process.

## License

Apache 2.0. See [`LICENSE`](./LICENSE).
