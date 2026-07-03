#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

export QUADLET_OUT="$SCRIPT_DIR/generated/quadlet"
export MOSQUITTO_CONFIG_OUT="$SCRIPT_DIR/generated/mosquitto"
export SYSTEMD_OUT="$SCRIPT_DIR/generated/systemd"
mkdir -p "$QUADLET_OUT" "$MOSQUITTO_CONFIG_OUT" "$SYSTEMD_OUT"

export APP_DIR="${APP_DIR:-/home/adamhoof/MOBv3}"
export WORKDIR="${WORKDIR:-/app}"
export CERT_DIR="${CERT_DIR:-$APP_DIR/certs}"
export CREDENTIAL_DIR="${CREDENTIAL_DIR:-$APP_DIR/deploy/credentials}"
export MOSQUITTO_IMAGE="${MOSQUITTO_IMAGE:-docker.io/library/eclipse-mosquitto:2.1.2-alpine}"
export GO_IMAGE="${GO_IMAGE:-docker.io/library/golang:1.26-alpine}"
export ALPINE_IMAGE="${ALPINE_IMAGE:-docker.io/library/alpine:3.20}"
export TARGETARCH="${TARGETARCH:-amd64}"
export USB_PRINTER="${USB_PRINTER:-/dev/usb/lp0}"

if [[ -z "${PUBLIC_BIND_IP:-}" ]]; then
  PUBLIC_BIND_IP="$(ip -j -4 route get 1.1.1.1 | jq -er 'select(length == 1)[0].prefsrc')" || {
    printf 'ERROR: could not detect public bind IP; set PUBLIC_BIND_IP\n' >&2
    exit 1
  }
fi
export PUBLIC_BIND_IP

printf '==> Using public bind IP %s\n' "$PUBLIC_BIND_IP"

"$SCRIPT_DIR/services/network.sh"
"$SCRIPT_DIR/services/mosquitto_config.sh"
"$SCRIPT_DIR/services/mosquitto.sh"
"$SCRIPT_DIR/services/catalog.sh"
"$SCRIPT_DIR/services/qrproxy.sh"
"$SCRIPT_DIR/services/target.sh"
