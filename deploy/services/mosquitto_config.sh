#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
set -a
source "$DEPLOY_DIR/env/mosquitto.env"
set +a

cat >"$MOSQUITTO_CONFIG_OUT/mosquitto.conf" <<EOF
listener $MQTT_PORT 0.0.0.0
user mosquitto
cafile /mosquitto/certs/ca.crt
certfile /mosquitto/certs/mosquitto_server.crt
keyfile /run/mobv3/mosquitto_server.key
require_certificate true
use_identity_as_username true
allow_anonymous false
persistence true
persistence_location /mosquitto/data/
log_dest stdout
EOF

printf '==> Generated Mosquitto config\n'
