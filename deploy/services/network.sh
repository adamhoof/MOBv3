#!/usr/bin/env bash
set -Eeuo pipefail

cat >"$QUADLET_OUT/mobv3.network" <<EOF
[Network]
NetworkName=mobv3_network
EOF

printf '==> Generated network Quadlet unit\n'
