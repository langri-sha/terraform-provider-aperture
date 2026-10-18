# aperture_config is a singleton — one document per gateway. Multiple
# resource instances pointing at the same provider endpoint will stomp
# each other.
#
# Note: The provider endpoint must include the /aperture path suffix.
# Example: endpoint = "http://ai.your-tailnet.ts.net/aperture"

resource "aperture_config" "main" {
  providers = {
    # Passthrough: clients bring their own Claude subscription login and
    # the gateway forwards it. No API key is stored anywhere.
    anthropic = {
      baseurl       = "https://api.anthropic.com"
      authorization = "x-api-key"
      auth_mode     = "passthrough"
      models        = ["claude-opus-5", "claude-sonnet-5", "claude-haiku-5"]
      compatibility = {
        anthropic_messages = true
        openai_chat        = false
      }
    }

    # Passthrough for a ChatGPT subscription, through the ChatGPT backend.
    codex = {
      baseurl   = "https://chatgpt.com/backend-api/codex"
      auth_mode = "passthrough"
      models    = ["gpt-5.5", "gpt-5.4", "gpt-5.4-mini"]
      compatibility = {
        openai_chat      = false
        openai_responses = true
      }
    }

    # A shared key held by the gateway instead of the client's own login.
    openai = {
      baseurl = "https://api.openai.com"
      models  = ["gpt-5.5"]
      apikey  = var.openai_api_key
    }
  }

  grants = [
    {
      src          = ["filip@example.com"]
      capabilities = [{ role = "admin" }]
    },
    {
      src = ["group:developers"]
      capabilities = [
        { role = "user", models = "**" },
        { connectors = ["aperture/**"] },
        { quotas = [{ bucket = "daily:<user>" }] },
      ]
    },
  ]

  quotas = {
    "daily:<user>" = {
      capacity  = "$10.00"
      rate      = "$5.00/day"
      on_exceed = "reject"
    }
  }

  hooks = {
    audit = {
      url         = "https://hooks.example.com/aperture"
      apikey      = var.audit_hook_api_key
      timeout     = "10s"
      fail_policy = "fail_open"
    }
  }

  auto_cost_basis = true
}

# Importing the existing live config:
#
#   terraform import aperture_config.main default
#
# The id is always "default" — Aperture has exactly one config per
# gateway. The post-import Read pulls everything from the API except
# apikeys, which the gateway redacts: declare each one in HCL, or remove
# the provider or hook that holds it. See the "Import an existing gateway"
# guide.
