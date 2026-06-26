#!/usr/bin/env bash
set -Eeuo pipefail

APP_DIR=${MOBV3_APP_DIR:-/home/adamhoof/MOBv3}
MQTT_TIMEOUT=${MOBV3_MQTT_TIMEOUT:-120}
CATALOG_TIMEOUT=${MOBV3_CATALOG_TIMEOUT:-120}

log() {
	printf '%s %s\n' "$(date --iso-8601=seconds)" "$*"
}

fail() {
	log "ERROR: $*" >&2
	exit 1
}

wait_for_container() {
	local name=$1
	local timeout=$2
	local deadline=$((SECONDS + timeout))

	while ((SECONDS < deadline)); do
		if [[ $(podman inspect -f '{{.State.Status}}' "$name" 2>/dev/null || true) == "running" ]]; then
			return 0
		fi
		sleep 1
	done

	fail "$name did not reach running state within ${timeout}s"
}

wait_for_mqtt() {
	local port=${MQTT_PORT:-8883}
	local deadline=$((SECONDS + MQTT_TIMEOUT))

	wait_for_container mobv3_mosquitto_broker "$MQTT_TIMEOUT"
	while ((SECONDS < deadline)); do
		if timeout 2 bash -c ":</dev/tcp/127.0.0.1/${port}" >/dev/null 2>&1; then
			log "mosquitto_broker is accepting connections on port ${port}"
			return 0
		fi
		sleep 1
	done

	fail "mosquitto_broker did not open port ${port} within ${MQTT_TIMEOUT}s"
}

wait_for_catalog() {
	local deadline=$((SECONDS + CATALOG_TIMEOUT))

	wait_for_container mobv3_catalog_service "$CATALOG_TIMEOUT"
	while ((SECONDS < deadline)); do
		local logs
		logs=$(podman logs mobv3_catalog_service 2>&1 || true)
		if [[ "$logs" == *"catalog_service listening"* ]]; then
			log "catalog_service is listening"
			return 0
		fi
		sleep 1
	done

	podman logs mobv3_catalog_service >&2 || true
	fail "catalog_service did not become ready within ${CATALOG_TIMEOUT}s"
}

require_usb_printer() {
	local printer=${USB_PRINTER:-/dev/usb/lp0}

	if [[ ! -e "$printer" ]]; then
		fail "USB printer device is missing: $printer"
	fi
	if [[ ! -c "$printer" ]]; then
		fail "USB printer path is not a character device: $printer"
	fi
	log "USB printer device is present: $printer"
}

cd "$APP_DIR"

if [[ -f .env ]]; then
	set -a
	# shellcheck disable=SC1091
	source .env
	set +a
else
	fail "missing .env in $APP_DIR"
fi

services=("$@")
if [[ ${#services[@]} -eq 0 ]]; then
	services=(mosquitto_broker catalog_service qrproxy)
fi
services_text="${services[*]}"

log "starting MOBv3 services: ${services_text}"
if [[ " ${services_text} " == *" qrproxy "* ]]; then
	require_usb_printer
fi

podman compose up -d --build "${services[@]}"

if [[ " ${services_text} " == *" mosquitto_broker "* ]]; then
	wait_for_mqtt
fi

if [[ " ${services_text} " == *" catalog_service "* ]]; then
	wait_for_catalog
fi

if [[ " ${services_text} " == *" qrproxy "* ]]; then
	wait_for_container mobv3_qrproxy 60
fi

log "MOBv3 stack is started"
