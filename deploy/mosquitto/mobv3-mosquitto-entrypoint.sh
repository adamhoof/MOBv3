#!/bin/sh
set -eu

src=/run/secrets/mobv3_mosquitto_server_key
dst=/run/mobv3/mosquitto_server.key

mkdir -p /run/mobv3
cp "$src" "$dst"
chown mosquitto:mosquitto "$dst"
chmod 0400 "$dst"

exec "$@"
