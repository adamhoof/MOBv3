#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
set -a
# shellcheck disable=SC1091
source "$DEPLOY_DIR/env/catalog.env"
set +a

HOST_BIND_IP=${MOBV3_HOST_BIND_IP:-$HOST_BIND_IP}

if [[ -z "$HOST_BIND_IP" ]]; then
  printf 'ERROR: set HOST_BIND_IP in deploy/env/catalog.env or MOBV3_HOST_BIND_IP\n' >&2
  exit 1
fi

source_dir="$APP_DIR/source"

cat >"$QUADLET_OUT/mobv3-catalog.build" <<EOF
[Build]
ImageTag=localhost/mobv3-catalog_service:latest
File=$source_dir/cmd/catalog_service/Containerfile
SetWorkingDirectory=$source_dir
BuildArg=GO_IMAGE=$GO_IMAGE
BuildArg=ALPINE_IMAGE=$ALPINE_IMAGE
BuildArg=TARGETARCH=$TARGETARCH
BuildArg=WORKDIR=$WORKDIR
EOF

cat >"$QUADLET_OUT/mobv3-catalog.container" <<EOF
[Unit]
Description=MOBv3 catalog service
Wants=network-online.target mobv3-mosquitto.service
After=network-online.target mobv3-mosquitto.service

[Container]
ContainerName=mobv3_catalog_service
Image=mobv3-catalog.build
User=1000:0
EnvironmentFile=$APP_DIR/deploy/env/catalog.env
Network=mobv3.network
PublishPort=$HOST_BIND_IP:$CATALOG_HTTP_PORT:$CATALOG_HTTP_PORT
Volume=mobv3_catalog_data:/data
Volume=$CERT_DIR/catalog_server.crt:$WORKDIR/certs/catalog_server.crt:ro,Z
Volume=$CERT_DIR/ca.crt:$WORKDIR/certs/ca.crt:ro,Z
Volume=$CERT_DIR/client.crt:$WORKDIR/certs/client.crt:ro,Z
Volume=\${CREDENTIALS_DIRECTORY}/mobv3_catalog_server_key:/run/secrets/mobv3_catalog_server_key:ro
Volume=\${CREDENTIALS_DIRECTORY}/mobv3_client_key:/run/secrets/mobv3_client_key:ro

[Service]
LoadCredentialEncrypted=mobv3_catalog_server_key:$CREDENTIAL_DIR/mobv3_catalog_server_key.cred
LoadCredentialEncrypted=mobv3_client_key:$CREDENTIAL_DIR/mobv3_client_key.cred
Restart=always
RestartSec=5s

[Install]
WantedBy=default.target
EOF

printf '==> Generated catalog Quadlet units\n'
