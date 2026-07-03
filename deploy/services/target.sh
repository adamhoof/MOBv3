#!/usr/bin/env bash
set -Eeuo pipefail

cat >"$SYSTEMD_OUT/mobv3.target" <<EOF
[Unit]
Description=MOBv3 services
Requires=mobv3-mosquitto.service mobv3-catalog.service mobv3-qrproxy.service

[Install]
WantedBy=default.target
EOF

printf '==> Generated MOBv3 systemd target\n'
