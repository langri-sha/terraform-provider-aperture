# Examples

Layout follows [terraform-plugin-docs](https://github.com/hashicorp/terraform-plugin-docs)
conventions: `scripts/generate-docs.sh` renders `provider/` and `resources/`
into `docs/`.

```
provider/                          provider block example -> docs/index.md
resources/<name>/resource.tf       -> docs/resources/<name>.md
quickstart/                        end-to-end tailscale + aperture setup
```

## Quickstart

[`quickstart/`](./quickstart) is the smallest correct configuration:
a tailnet ACL declaring `tag:ai-aperture`, the Aperture provider
pointed at `http://ai.<tailnet>/aperture`, and one
`aperture_config` resource with a single LLM provider and a
developer grant. Read that first if you've never used the provider.

## Running against a local build

To try unreleased changes, build the binary and point a `~/.terraformrc`
at it with `dev_overrides`:

```hcl
provider_installation {
  dev_overrides {
    "langri-sha/aperture" = "/path/to/aperture/dist"
  }
  direct {}
}
```

Then `go build -o dist/terraform-provider-aperture ./cmd/terraform-provider-aperture`
and run `terraform init` and `terraform plan` from any of the example
directories. `init` skips the overridden provider but still installs the
others, such as `tailscale/tailscale` for the quickstart.

## Importing an existing live config

```sh
# Pre-create a stub block:
cat > main.tf <<'EOF'
resource "aperture_config" "main" {}
EOF

terraform import aperture_config.main default
```

The post-import Read pulls the HuJSON document from the gateway and
populates state, except for API keys, which the gateway redacts. Declare
an `apikey` in HCL for any provider or hook that has one before applying.
