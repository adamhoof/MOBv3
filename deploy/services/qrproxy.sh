#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
set -a
# shellcheck disable=SC1091
source "$DEPLOY_DIR/env/qrproxy.env"
set +a

HOST_BIND_IP=${MOBV3_HOST_BIND_IP:-$HOST_BIND_IP}
USB_PRINTER=${MOBV3_USB_PRINTER:-$USB_PRINTER}

if [[ -z "$HOST_BIND_IP" || -z "$USB_PRINTER" ]]; then
  printf 'ERROR: set HOST_BIND_IP and USB_PRINTER in deploy/env/qrproxy.env or MOBV3_HOST_BIND_IP/MOBV3_USB_PRINTER\n' >&2
  exit 1
fi

source_dir="$APP_DIR/source"

cat >"$QUADLET_OUT/mobv3-qrproxy.build" <<EOF
[Build]
ImageTag=localhost/mobv3-qrproxy:latest
File=$source_dir/cmd/qrproxy/Containerfile
SetWorkingDirectory=$source_dir
BuildArg=GO_IMAGE=$GO_IMAGE
BuildArg=TARGETARCH=$TARGETARCH
EOF

cat >"$QUADLET_OUT/mobv3-qrproxy.container" <<EOF
[Unit]
Description=MOBv3 QRProxy
Wants=network-online.target mobv3-mosquitto.service
After=network-online.target mobv3-mosquitto.service

[Container]
ContainerName=mobv3_qrproxy
Image=mobv3-qrproxy.build
User=0:0
GroupAdd=keep-groups
SecurityLabelDisable=true
EnvironmentFile=$APP_DIR/deploy/env/qrproxy.env
Network=mobv3.network
PublishPort=$HOST_BIND_IP:$PROXY_PORT:$PROXY_PORT
Volume=$CERT_DIR/ca.crt:$WORKDIR/certs/ca.crt:ro,Z
Volume=$CERT_DIR/client.crt:$WORKDIR/certs/client.crt:ro,Z
Volume=\${CREDENTIALS_DIRECTORY}/mobv3_client_key:/run/secrets/mobv3_client_key:ro
AddDevice=$USB_PRINTER:$USB_PRINTER

[Service]
LoadCredentialEncrypted=mobv3_client_key:$CREDENTIAL_DIR/mobv3_client_key.cred
Restart=always
RestartSec=5s

[Install]
WantedBy=default.target
EOF

printf '==> Generated QRProxy Quadlet units\n'
