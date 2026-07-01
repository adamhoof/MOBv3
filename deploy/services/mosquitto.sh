#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
set -a
# shellcheck disable=SC1091
source "$DEPLOY_DIR/env/mosquitto.env"
set +a

HOST_BIND_IP=${MOBV3_HOST_BIND_IP:-$HOST_BIND_IP}

if [[ -z "$HOST_BIND_IP" ]]; then
  printf 'ERROR: set HOST_BIND_IP in deploy/env/mosquitto.env or MOBV3_HOST_BIND_IP\n' >&2
  exit 1
fi

cat >"$QUADLET_OUT/mobv3-mosquitto.container" <<EOF
[Unit]
Description=MOBv3 Mosquitto broker
Wants=network-online.target
After=network-online.target

[Container]
ContainerName=mobv3_mosquitto_broker
Image=$MOSQUITTO_IMAGE
User=1883:0
EnvironmentFile=$APP_DIR/deploy/env/mosquitto.env
Network=mobv3.network
PublishPort=$HOST_BIND_IP:$MQTT_PORT:$MQTT_PORT
Volume=$APP_DIR/deploy/mosquitto:/mosquitto/config:ro,Z
Volume=mobv3_mosquitto_data:/mosquitto/data
Volume=mobv3_mosquitto_log:/mosquitto/log
Volume=$CERT_DIR/mosquitto_server.crt:/mosquitto/certs/mosquitto_server.crt:ro,Z
Volume=$CERT_DIR/ca.crt:/mosquitto/certs/ca.crt:ro,Z
Volume=\${CREDENTIALS_DIRECTORY}/mobv3_mosquitto_server_key:/run/secrets/mobv3_mosquitto_server_key:ro

[Service]
LoadCredentialEncrypted=mobv3_mosquitto_server_key:$CREDENTIAL_DIR/mobv3_mosquitto_server_key.cred
Restart=always
RestartSec=5s

[Install]
WantedBy=default.target
EOF

printf '==> Generated Mosquitto Quadlet unit\n'
