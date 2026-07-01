#!/usr/bin/env bash

deploy_lib_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
deploy_dir="$(CDPATH= cd -- "$deploy_lib_dir/.." && pwd)"
repo_root="$(CDPATH= cd -- "$deploy_dir/.." && pwd)"
config_dir="$deploy_dir/config"
env_out="$deploy_dir/generated/env"
quadlet_out="$deploy_dir/generated/quadlet"

APP_DIR="$repo_root"
WORKDIR=/app
CERT_DIR="$repo_root/certs"
CREDENTIAL_DIR="$repo_root/deploy/credentials"

GO_IMAGE=docker.io/library/golang:1.26-alpine
ALPINE_IMAGE=docker.io/library/alpine:3.20
MOSQUITTO_IMAGE=docker.io/library/eclipse-mosquitto:2.1.2-alpine
TARGETARCH=amd64

MQTT_PROTOCOL=tcps
MQTT_PORT=8883
CATALOG_HTTP_PORT=8443
PROXY_PORT=9100

CATALOG_DB_PATH=/data/catalog.db
CATALOG_IMPORT_ENDPOINT=/catalog/import
MQTT_CATALOG_CLIENT_ID=catalog_service
MQTT_TOPIC_REQUEST=product/+/+
QRPROXY_MQTT_CLIENT_ID=qrproxy
QTERM_MQTT_TOPIC=qterm/payments

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

load_config() {
  local name path
  set -a
  for name in "$@"; do
    path="$config_dir/$name.env"
    [[ -f "$path" ]] || die "missing config file: $path"
    # shellcheck disable=SC1090
    source "$path"
  done
  set +a

  HOST_BIND_IP=${MOBV3_HOST_BIND_IP:-${HOST_BIND_IP:-}}
  USB_PRINTER=${MOBV3_USB_PRINTER:-${USB_PRINTER:-}}
}

require_vars() {
  local name
  for name in "$@"; do
    [[ -n "${!name:-}" ]] || die "missing required config value: $name"
  done
}

require_machine_config() {
  if [[ -z "${HOST_BIND_IP:-}" || -z "${USB_PRINTER:-}" ]]; then
    cat >&2 <<EOF
ERROR: deploy/config/machine.env is not filled in.

Set the target machine values before generating deployment files:

  HOST_BIND_IP=<mini-pc-lan-ip>
  USB_PRINTER=/dev/usb/lp0

These are intentionally not defaulted because they are physical machine facts.
EOF
    exit 1
  fi
}

prepare_outputs() {
  mkdir -p "$env_out" "$quadlet_out"
}
