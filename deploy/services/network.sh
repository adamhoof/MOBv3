#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/../lib/config.sh"

prepare_outputs

cat >"$quadlet_out/mobv3.network" <<EOF
[Network]
NetworkName=mobv3_network
EOF

log "Generated network Quadlet unit"
