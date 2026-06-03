#!/usr/bin/env bash
#
# Regenerate the provider documentation under docs/ from the live provider
# schema and the examples/ tree, using terraform-plugin-docs.
#
# Why this isn't just `tfplugindocs generate`: tfplugindocs builds the provider
# from the repo root, but this provider's entrypoint lives under cmd/. It also
# only looks the schema up under the bare name or the "hashicorp/" namespace, so
# we build the binary ourselves, export the schema through a throwaway
# "hashicorp/aperture" dev override, and hand it to tfplugindocs via
# --providers-schema (which skips its own build). The namespace is only a lookup
# key; the rendered docs use the real source from examples/provider/provider.tf.
#
# Requires: go, terraform. Run from anywhere — it resolves the repo root itself.

set -euo pipefail

cd "$(dirname "$0")/.."

PROVIDER_NAME=aperture
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

go build -o "$tmp/terraform-provider-${PROVIDER_NAME}" "./cmd/terraform-provider-${PROVIDER_NAME}"

cat > "$tmp/dev.tfrc" <<EOF
provider_installation {
  dev_overrides {
    "hashicorp/${PROVIDER_NAME}" = "$tmp"
  }
  direct {}
}
EOF

mkdir -p "$tmp/cfg"
cat > "$tmp/cfg/main.tf" <<EOF
terraform {
  required_providers {
    ${PROVIDER_NAME} = { source = "hashicorp/${PROVIDER_NAME}" }
  }
}
EOF

(
  cd "$tmp/cfg"
  TF_CLI_CONFIG_FILE="$tmp/dev.tfrc" terraform providers schema -json > "$tmp/schema.json"
)

go tool tfplugindocs generate \
  --provider-name "${PROVIDER_NAME}" \
  --providers-schema "$tmp/schema.json"
