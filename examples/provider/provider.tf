terraform {
  required_providers {
    aperture = {
      source  = "langri-sha/aperture"
      version = "~> 0.3"
    }
  }
}

provider "aperture" {
  # Full base URL of the Aperture admin API including the /aperture
  # path prefix. Auth is by Tailscale identity at the network layer,
  # so the caller must be on the tailnet with the admin role.
  endpoint = "http://ai.tail-scale.ts.net/aperture"
}
