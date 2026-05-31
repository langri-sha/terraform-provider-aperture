#!/usr/bin/env bash
#
# Build, sign, and upload a release of terraform-provider-aperture to a
# Terraform Cloud private provider registry.
#
# Required env vars (recommended to inject via `op run --env-file=...`):
#
#   TFC_TOKEN       Terraform Cloud user/team token with registry-providers
#                   write permission on $TFC_ORG.
#   TFC_ORG         Terraform Cloud organization (e.g. doc-ba).
#   GPG_KEY_ID      Long fingerprint of the signing key (40-char hex).
#                   The corresponding secret key must be importable by gpg
#                   on this machine, with a passphrase-less or agent-cached
#                   secret part.
#
# Optional:
#
#   PROVIDER_NAME   Defaults to "aperture".
#   VERSION         Defaults to the current `git describe --tags`. Must be
#                   semver without the leading "v" by the time it reaches
#                   TFC (TFC strips it but emits a warning). We strip here.
#   PROTOCOLS       Defaults to "6.0" (terraform-plugin-framework's wire
#                   protocol). Comma-separated.
#   DIST            Build output directory. Defaults to ./dist.
#   PLATFORMS       Space-separated GOOS/GOARCH list. Defaults below.
#
# Usage: scripts/tfc-release.sh [VERSION]

set -euo pipefail

# --- Config -----------------------------------------------------------------

PROVIDER_NAME=${PROVIDER_NAME:-aperture}
TFC_ORG=${TFC_ORG:?TFC_ORG is required (e.g. doc-ba)}
TFC_TOKEN=${TFC_TOKEN:?TFC_TOKEN is required}
GPG_KEY_ID=${GPG_KEY_ID:?GPG_KEY_ID is required (40-char fingerprint)}
PROTOCOLS=${PROTOCOLS:-6.0}
DIST=${DIST:-dist}

# Default platforms are the popular ones — TFC runners are linux/amd64;
# the rest cover dev workstations and self-hosted runners.
PLATFORMS=${PLATFORMS:-"linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64"}

VERSION_INPUT=${1:-${VERSION:-$(git describe --tags --abbrev=0 2>/dev/null || echo "")}}
if [[ -z "$VERSION_INPUT" ]]; then
  echo "error: VERSION is required (no tags found). Pass as arg or set VERSION." >&2
  exit 1
fi
VERSION=${VERSION_INPUT#v}

# --- Helpers ----------------------------------------------------------------

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
api() {
  local method=$1 path=$2 body=${3:-}
  local url="https://app.terraform.io${path}"
  local args=(-sS --fail-with-body
    -H "Authorization: Bearer $TFC_TOKEN"
    -H "Content-Type: application/vnd.api+json"
    -X "$method"
    "$url")
  if [[ -n "$body" ]]; then
    curl "${args[@]}" -d "$body"
  else
    curl "${args[@]}"
  fi
}
upload() {
  local url=$1 file=$2
  curl -sS --fail-with-body -X PUT --upload-file "$file" "$url"
}

# --- Build ------------------------------------------------------------------

log "Building $PROVIDER_NAME v$VERSION for: $PLATFORMS"
rm -rf "$DIST"
mkdir -p "$DIST"

binname="terraform-provider-${PROVIDER_NAME}_v${VERSION}"

for platform in $PLATFORMS; do
  goos=${platform%/*}
  goarch=${platform#*/}
  out="${DIST}/${binname}"
  [[ "$goos" == "windows" ]] && out="${out}.exe"

  log "  build $goos/$goarch"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -ldflags "-s -w -X main.version=$VERSION -X main.commit=$(git rev-parse --short HEAD)" \
    -o "$out" ./cmd/terraform-provider-aperture

  zipname="terraform-provider-${PROVIDER_NAME}_${VERSION}_${goos}_${goarch}.zip"
  (cd "$DIST" && zip -q "$zipname" "$(basename "$out")" && rm -f "$(basename "$out")")
done

# --- Sign -------------------------------------------------------------------

log "Generating SHA256SUMS"
shafile="terraform-provider-${PROVIDER_NAME}_${VERSION}_SHA256SUMS"
sigfile="${shafile}.sig"
(cd "$DIST" && sha256sum *.zip > "$shafile")

log "Signing SHA256SUMS with $GPG_KEY_ID"
rm -f "$DIST/$sigfile"
gpg --batch --yes --pinentry-mode loopback --passphrase '' \
  --local-user "$GPG_KEY_ID" \
  --detach-sign --output "$DIST/$sigfile" "$DIST/$shafile"
gpg --verify "$DIST/$sigfile" "$DIST/$shafile" >&2

# --- Upload: GPG key (idempotent) ------------------------------------------

log "Registering GPG key in TFC org $TFC_ORG"
ascii_armor=$(gpg --armor --export "$GPG_KEY_ID" | python3 -c 'import sys, json; print(json.dumps(sys.stdin.read()))')
gpg_body=$(cat <<EOF
{"data":{"type":"gpg-keys","attributes":{"namespace":"$TFC_ORG","ascii-armor":$ascii_armor}}}
EOF
)
gpg_resp=$(api POST "/api/registry/private/v2/gpg-keys" "$gpg_body" || true)
if echo "$gpg_resp" | grep -q '"key-id"'; then
  log "  registered"
else
  log "  already present (or error): $gpg_resp"
fi

# --- Upload: provider (idempotent) -----------------------------------------

log "Ensuring provider $TFC_ORG/$PROVIDER_NAME exists"
prov_body=$(cat <<EOF
{"data":{"type":"registry-providers","attributes":{"name":"$PROVIDER_NAME","namespace":"$TFC_ORG","registry-name":"private"}}}
EOF
)
api POST "/api/v2/organizations/$TFC_ORG/registry-providers" "$prov_body" >/dev/null 2>&1 || true

# --- Upload: version --------------------------------------------------------

log "Creating version v$VERSION"
key_id_short=${GPG_KEY_ID: -16}
ver_body=$(cat <<EOF
{"data":{"type":"registry-provider-versions","attributes":{"version":"$VERSION","key-id":"$key_id_short","protocols":["${PROTOCOLS//,/\",\"}"]}}}
EOF
)
ver_resp=$(api POST "/api/v2/organizations/$TFC_ORG/registry-providers/private/$TFC_ORG/$PROVIDER_NAME/versions" "$ver_body")
shasums_url=$(echo "$ver_resp" | python3 -c 'import json,sys;print(json.load(sys.stdin)["data"]["links"]["shasums-upload"])')
sig_url=$(echo "$ver_resp"     | python3 -c 'import json,sys;print(json.load(sys.stdin)["data"]["links"]["shasums-sig-upload"])')

log "  uploading SHA256SUMS"
upload "$shasums_url" "$DIST/$shafile"
log "  uploading SHA256SUMS.sig"
upload "$sig_url"     "$DIST/$sigfile"

# --- Upload: platforms ------------------------------------------------------

for platform in $PLATFORMS; do
  goos=${platform%/*}
  goarch=${platform#*/}
  zipname="terraform-provider-${PROVIDER_NAME}_${VERSION}_${goos}_${goarch}.zip"
  shasum=$(awk -v f="$zipname" '$2==f {print $1}' "$DIST/$shafile")

  log "Registering platform $goos/$goarch"
  plat_body=$(cat <<EOF
{"data":{"type":"registry-provider-version-platforms","attributes":{"os":"$goos","arch":"$goarch","shasum":"$shasum","filename":"$zipname"}}}
EOF
)
  plat_resp=$(api POST "/api/v2/organizations/$TFC_ORG/registry-providers/private/$TFC_ORG/$PROVIDER_NAME/versions/$VERSION/platforms" "$plat_body")
  zip_url=$(echo "$plat_resp" | python3 -c 'import json,sys;print(json.load(sys.stdin)["data"]["links"]["provider-binary-upload"])')

  log "  uploading $zipname"
  upload "$zip_url" "$DIST/$zipname"
done

log "Done. Source address: app.terraform.io/$TFC_ORG/$PROVIDER_NAME"
log "Pin in versions.tf as: source = \"app.terraform.io/$TFC_ORG/$PROVIDER_NAME\""
