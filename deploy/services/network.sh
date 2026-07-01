#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# Internal generator. deploy/generate.sh exports required values.

cat >"$QUADLET_OUT/mobv3.network" <<EOF
[Network]
NetworkName=mobv3_network
EOF

printf '==> Generated network Quadlet unit\n'
