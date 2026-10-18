---
page_title: "Import an existing gateway"
description: |-
  Bring a gateway that was configured in the Aperture console under Terraform without losing anything, and what to do about credentials.
---

# Import an existing gateway

`aperture_config` is a singleton, so the import ID is always `default`:

```shell
cat > main.tf <<'EOF'
provider "aperture" {
  endpoint = "http://<aperture-hostname>/aperture"
}

resource "aperture_config" "main" {}
EOF

terraform init
terraform import aperture_config.main default
terraform show
```

Copy what `terraform show` prints into the resource block, then run `terraform plan`. It should report no changes.

## Keys the provider does not model

An apply replaces the gateway's whole document, but only keys the configuration manages are set or removed. A key the provider does not know, at any depth, keeps the value the gateway holds, and so does a top-level section you leave unset (`exporters`, `database`, `connectors`, `chat_models`, `flags`, `mcp`, `temp_grants`). Declare one of those sections as JSON when you want to manage it:

```terraform
resource "aperture_config" "main" {
  database = jsonencode({
    captures = { retention = { duration = "24h" } }
  })
}
```

Once set, the section is replaced as a whole. Removing it from the configuration leaves the gateway's copy as it is.

Comments in the console's document are not kept when Terraform writes it. Unmodelled keys inside a grant or capability follow the element's position, so reordering grants moves them with the position, not the grant.

## Credentials

The gateway redacts every `apikey` it returns, so the provider cannot read them back. State never holds a value read from the gateway, and the redaction placeholder is never written back.

Importing and planning work whatever the gateway holds. An apply fails with `aperture credential not managed` when it would overwrite an `apikey` the gateway holds for a provider or hook and the configuration sets none. Either set the key:

```terraform
hooks = {
  audit = {
    url    = "https://hooks.example.com/aperture"
    apikey = var.audit_hook_api_key
  }
}
```

or remove the provider or hook that holds it, which deletes the key with it. Providers in passthrough mode need no `apikey`, so none is required of them.

`authorization` is the header scheme (`bearer`, `x-api-key`, `x-goog-api-key`, `hec` or `cf-aig-authorization`), not a secret, and is read back as is.
