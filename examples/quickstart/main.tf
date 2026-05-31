# Minimal end-to-end setup: a tailnet with one Aperture gateway,
# managed top-to-bottom from terraform.
#
# This is the smallest configuration that's actually correct — no
# "for the sake of the example" placeholder values that you have to
# remember to replace.

terraform {
  required_providers {
    tailscale = {
      source  = "tailscale/tailscale"
      version = "~> 0.28"
    }
    aperture = {
      source  = "langri-sha/aperture"
      version = "~> 0.3"
    }
  }
}

variable "tailnet" {
  description = "Your tailnet name, e.g. tail-scale.ts.net or your custom domain."
  type        = string
}

variable "openai_api_key" {
  description = "OpenAI API key. Pull from a secret store; do not inline."
  type        = string
  sensitive   = true
}

# 1. Tailscale provider — talks to the tailnet API on your behalf.
provider "tailscale" {
  # api_key from $TAILSCALE_API_KEY by default.
}

# 2. Tailnet ACL — owns tag:ai-aperture and lets group:developers
#    reach it on 443 (the data-plane port for LLM proxying).
resource "tailscale_acl" "policy" {
  acl = jsonencode({
    tagOwners = {
      "tag:ai-aperture" = ["group:developers"]
    }
    groups = {
      "group:developers" = ["filip@example.com"]
    }
    acls = [
      {
        action = "accept"
        src    = ["group:developers"]
        dst    = ["tag:ai-aperture:443"]
      },
    ]
  })

  overwrite_existing_content = true
}

# 3. Aperture provider — points at the gateway's admin API. The endpoint
#    must use HTTPS; auth is by Tailscale identity, no api_key attribute.
#    The /aperture path suffix is mandatory for the provider's internal routing.
provider "aperture" {
  endpoint = "https://ai.${var.tailnet}/aperture"
}

# 4. The Aperture configuration document itself. One singleton, one
#    OpenAI provider, one grant for the developers group.
resource "aperture_config" "main" {
  providers = {
    openai = {
      baseurl = "https://api.openai.com/v1"
      models  = ["openai/gpt-5.5"]
      apikey  = var.openai_api_key
    }
  }

  grants = [
    {
      src = ["group:developers"]
      capabilities = [
        { role = "user" },
        { models = "**" },
      ]
    },
  ]
}
