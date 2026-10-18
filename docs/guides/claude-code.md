---
page_title: "Use Claude Code with Aperture"
description: |-
  Declare a passthrough Anthropic provider with Terraform and point Claude Code, signed in with a Claude subscription, at the gateway.
---

# Use Claude Code with Aperture

In passthrough mode the gateway forwards the credential Claude Code already holds, so a Claude Pro or Max login works through Aperture and no API key is stored in the gateway, in Terraform state or in a secret store.

## Declare the provider

```terraform
resource "aperture_config" "main" {
  providers = {
    anthropic = {
      baseurl       = "https://api.anthropic.com"
      authorization = "x-api-key"
      auth_mode     = "passthrough"
      models        = ["claude-haiku-4-5", "claude-sonnet-4-6", "claude-opus-4-7", "claude-opus-5", "claude-sonnet-5", "claude-haiku-5"]
      compatibility = {
        anthropic_messages = true
        openai_chat        = false
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

- `auth_mode = "passthrough"` forwards the client's own `Authorization` or `x-api-key` header. There is no `apikey`, so the provider serves only requests that carry a client credential.
- `authorization = "x-api-key"` is the header scheme Anthropic uses. The gateway forwards whichever credential the client sends, including an OAuth bearer token.
- `models` lists exact model names. Wildcards in this list match nothing; use globs in `grants` to control access instead.
- Without a grant that names the models, nobody can use them: Aperture denies by default.

## Point Claude Code at the gateway

Sign in to the subscription first (`claude`, then `/login`). Then set only the gateway's base URL, with `http://` and without a `/v1` suffix:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://<aperture-hostname>"
  }
}
```

Save that as `~/.claude/settings.json` to use the gateway everywhere, or pass it to a single run:

```shell
claude --settings ./aperture.json
```

or export the variable for one shell:

```shell
ANTHROPIC_BASE_URL=http://<aperture-hostname> claude
```

Leave `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN` and `apiKeyHelper` unset. Any of them replaces the subscription's OAuth token, which is the credential the gateway is meant to forward.

The gateway serves this snippet itself, filled in with its own address, at `http://<aperture-hostname>/api/agent-config` under `claude-sub`.

## What passes through

Only inference requests (`/v1/messages`) go through the gateway. Sign-in and token refresh go straight to Anthropic. The gateway authenticates each request by Tailscale identity, so the machine running Claude Code must be on the tailnet and covered by a grant.

## Check it

Send a message, then open the gateway's chat, go to **Sessions** and look for the request. A `401` means Claude Code is not signed in to the subscription or a key or token is set on the client. A `405` from the upstream means the base URL ends in `/v1`.
