#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

"$SCRIPT_DIR/services/network.sh"
"$SCRIPT_DIR/services/mosquitto.sh"
"$SCRIPT_DIR/services/catalog.sh"
"$SCRIPT_DIR/services/qrproxy.sh"
