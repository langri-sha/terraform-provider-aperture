---
page_title: "Use Codex with Aperture"
description: |-
  Declare a passthrough Codex provider with Terraform and point the Codex CLI, signed in with ChatGPT, at the gateway.
---

# Use Codex with Aperture

In passthrough mode the gateway forwards the ChatGPT login the Codex CLI already holds, so a ChatGPT subscription works through Aperture and no API key is stored in the gateway, in Terraform state or in a secret store.

## Declare the provider

```terraform
resource "aperture_config" "main" {
  providers = {
    codex = {
      baseurl   = "https://chatgpt.com/backend-api/codex"
      auth_mode = "passthrough"
      models    = ["gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex-spark"]
      compatibility = {
        openai_chat      = false
        openai_responses = true
      }
    }
  }

  grants = [
    {
      src          = ["group:developers"]
      capabilities = [{ role = "user", models = "**" }]
    },
  ]
}
```

- `baseurl` is the ChatGPT backend, not `api.openai.com`. The ChatGPT backend accepts ChatGPT OAuth tokens only, and the platform API accepts API keys only. A subscription login and a platform key need separate providers.
- `auth_mode = "passthrough"` with no `apikey` makes the provider serve only requests that carry a client credential.
- `openai_responses` is the API format the Codex CLI speaks.
- `models` lists exact model names, and must include the model Codex is configured to use.

## Point Codex at the gateway

Sign in to ChatGPT in Codex first. Then add a model provider to `~/.codex/config.toml`:

```toml
[model_providers.aperture]
name = "Aperture Proxy"
base_url = "http://<aperture-hostname>/codex"
wire_api = "responses"
requires_openai_auth = true
```

Add `model_provider = "aperture"` at the top of the file to use the gateway by default, or select it for one run:

```shell
codex -c model_provider=aperture
```

- `base_url` uses `http://` and ends in `/codex`. Codex appends `/responses`, and the gateway strips `/codex` before forwarding to the ChatGPT backend.
- `requires_openai_auth = true` makes Codex send its ChatGPT OAuth token. Do not combine it with `env_key`.

The gateway serves this snippet itself, filled in with its own address, at `http://<aperture-hostname>/api/agent-config` under `codex-sub`.

## What passes through

Only inference requests (`/responses`) go through the gateway. Login and token refresh go straight to OpenAI. The gateway authenticates each request by Tailscale identity, so the machine running Codex must be on the tailnet and covered by a grant.

## Check it

Send a message, then open the gateway's chat, go to **Sessions** and look for the request. A `401` means Codex is not signed in to ChatGPT or `requires_openai_auth` is missing. If requests never arrive, check that `base_url` includes `/codex`.
