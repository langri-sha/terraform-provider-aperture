# Examples

Layout follows [terraform-plugin-docs](https://github.com/hashicorp/terraform-plugin-docs)
conventions, so `tfplugindocs generate` can pick them up unmodified
once docs generation is wired up.

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

The provider isn't on the public Terraform Registry. Two options:

1. **TFC private provider.** `app.terraform.io/<org>/aperture` — see
   [`scripts/tfc-release.sh`](../scripts/tfc-release.sh).
2. **Local `dev_overrides`.** Build the binary and point a
   `~/.terraformrc` at it:

   ```hcl
   provider_installation {
     dev_overrides {
       "langri-sha/aperture" = "/path/to/aperture/dist"
     }
     direct {}
   }
   ```

   Then `go build -o dist/terraform-provider-aperture ./cmd/terraform-provider-aperture`
   and run `terraform plan` from any of the example directories.
   Skip `terraform init` — dev overrides bypass the lock file.

## Importing an existing live config

```sh
# Pre-create a stub block:
cat > main.tf <<'EOF'
resource "aperture_config" "main" {}
EOF

terraform import aperture_config.main default
```

The post-import Read pulls the entire HuJSON document from the
gateway and populates state. API keys come back redacted, so
fill in `apikey = ...` in HCL pointing at your secret store before
running `terraform plan`.
