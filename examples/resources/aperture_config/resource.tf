# aperture_config is a singleton — one document per gateway. Multiple
# resource instances pointing at the same provider endpoint will stomp
# each other.
#
# Note: The provider endpoint must use HTTPS and include the /aperture path suffix.
# Example: endpoint = "https://ai.your-tailnet.ts.net/aperture"

resource "aperture_config" "main" {
  providers = {
    openai = {
      baseurl = "https://api.openai.com/v1"
      models  = ["openai/gpt-5.5", "openai/gpt-5.2"]
      apikey  = var.openai_api_key
    }
    anthropic = {
      baseurl = "https://api.anthropic.com/v1"
      models  = ["anthropic/claude-opus-4-7", "anthropic/claude-sonnet-4-6"]
      apikey  = var.anthropic_api_key
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
    {
      src = ["filip@example.com"]
      capabilities = [
        { role = "admin" },
      ]
    },
  ]

  quotas = {
    monthly = {
      capacity  = 100
      rate      = 100
      on_exceed = "block"
    }
  }

  auto_cost_basis = true
}

# Importing the existing live config:
#
#   terraform import aperture_config.main default
#
# The id is always "default" — Aperture has exactly one config per
# gateway. The post-import Read pulls everything from the API; you
# only need a stub `resource "aperture_config" "main" {}` block in
# your HCL before running import.
