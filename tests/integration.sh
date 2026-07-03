#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

MOSQUITTO_IMAGE="${MOSQUITTO_IMAGE:-docker.io/library/eclipse-mosquitto:2.1.2-alpine}"
GO_IMAGE="${GO_IMAGE:-docker.io/library/golang:1.26-alpine}"
ALPINE_IMAGE="${ALPINE_IMAGE:-docker.io/library/alpine:3.20}"
TARGETARCH="${TARGETARCH:-$(go env GOARCH)}"
WORKDIR="${WORKDIR:-/app}"
MQTT_HOST_PORT="${MQTT_HOST_PORT:-18883}"
CATALOG_HOST_PORT="${CATALOG_HOST_PORT:-18443}"

run_id="${MOBV3_INTEGRATION_ID:-$$}"
network="mobv3_integration_${run_id}"
mosquitto_container="mobv3_integration_mosquitto_${run_id}"
catalog_container="mobv3_integration_catalog_${run_id}"
mosquitto_image="localhost/mobv3-integration-mosquitto:${run_id}"
catalog_image="localhost/mobv3-integration-catalog:${run_id}"
work_dir="$(mktemp -d)"
client_env="$work_dir/mobctl.env"
response_file="$work_dir/response.json"
mdb_fixture="$repo_root/tests/data/catalog_5_products.mdb"

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"; }

cleanup() {
  podman rm -f "$catalog_container" "$mosquitto_container" >/dev/null 2>&1 || true
  podman network rm "$network" >/dev/null 2>&1 || true
  rm -rf "$work_dir"
}
trap cleanup EXIT

wait_for_tcp() {
  local host=$1 port=$2
  for _ in $(seq 1 40); do
    if timeout 1 bash -c "</dev/tcp/$host/$port" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

mosquitto_client() {
  podman run --rm --network host \
    -v "$work_dir/certs:/certs:ro,z" \
    "$MOSQUITTO_IMAGE" "$@"
}

mobctl() {
  (cd "$repo_root/source" && go run ./cmd/mobctl --file "$client_env" "$@")
}

for cmd in bash go jq openssl podman systemd-creds; do
  need "$cmd"
done

[[ -f "$mdb_fixture" ]] || die "missing $mdb_fixture"

mkdir -p "$work_dir/certs" "$work_dir/credentials" "$work_dir/keys" "$work_dir/mosquitto-config" "$work_dir/mosquitto-data" "$work_dir/mosquitto-log" "$work_dir/catalog-data"
chmod 700 "$work_dir/credentials" "$work_dir/keys"

log "Generating temporary TLS material"
deploy/tls/conf_gen.sh --server-name localhost --cert-dir "$work_dir/certs" --credential-dir "$work_dir/credentials" --force >/dev/null 2>&1
for item in \
  mobv3_mosquitto_server_key:mosquitto_server.key \
  mobv3_catalog_server_key:catalog_server.key \
  mobv3_client_key:client.key
do
  cred="${item%%:*}"
  key="${item#*:}"
  systemd-creds --user decrypt --name="$cred" "$work_dir/credentials/$cred.cred" "$work_dir/keys/$key" >/dev/null
  chmod 600 "$work_dir/keys/$key"
done
cp "$work_dir/keys/client.key" "$work_dir/certs/client.key"

cat >"$work_dir/mosquitto-config/mosquitto.conf" <<EOF
listener 8883 0.0.0.0
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

log "Building integration images"
podman build -q -t "$mosquitto_image" \
  --build-arg MOSQUITTO_IMAGE="$MOSQUITTO_IMAGE" \
  -f deploy/mosquitto/Containerfile . >/dev/null
podman build -q -t "$catalog_image" \
  --build-arg GO_IMAGE="$GO_IMAGE" \
  --build-arg ALPINE_IMAGE="$ALPINE_IMAGE" \
  --build-arg TARGETARCH="$TARGETARCH" \
  --build-arg WORKDIR="$WORKDIR" \
  -f source/cmd/catalog_service/Containerfile source >/dev/null

log "Starting isolated integration stack"
podman network create "$network" >/dev/null
podman run -d --name "$mosquitto_container" \
  --network "$network" \
  --network-alias mosquitto_broker \
  -p "127.0.0.1:${MQTT_HOST_PORT}:8883" \
  -v "$work_dir/mosquitto-config:/mosquitto/config:ro,Z" \
  -v "$work_dir/mosquitto-data:/mosquitto/data:Z" \
  -v "$work_dir/mosquitto-log:/mosquitto/log:Z" \
  -v "$work_dir/certs/mosquitto_server.crt:/mosquitto/certs/mosquitto_server.crt:ro,Z" \
  -v "$work_dir/certs/ca.crt:/mosquitto/certs/ca.crt:ro,Z" \
  -v "$work_dir/keys/mosquitto_server.key:/run/secrets/mobv3_mosquitto_server_key:ro,Z" \
  "$mosquitto_image" >/dev/null
wait_for_tcp 127.0.0.1 "$MQTT_HOST_PORT" || die "Mosquitto did not open port $MQTT_HOST_PORT"

podman run -d --name "$catalog_container" \
  --network "$network" \
  -p "127.0.0.1:${CATALOG_HOST_PORT}:8443" \
  -e CATALOG_DB_PATH=/data/catalog.db \
  -e CATALOG_HTTP_HOST=0.0.0.0 \
  -e CATALOG_HTTP_PORT=8443 \
  -e CATALOG_IMPORT_ENDPOINT=/catalog/import \
  -e MQTT_PROTOCOL=tcps \
  -e MQTT_HOST=mosquitto_broker \
  -e MQTT_PORT=8883 \
  -e MQTT_CATALOG_CLIENT_ID=catalog_service \
  -e MQTT_TOPIC_REQUEST='product/+/+' \
  -e TLS_CA_PATH=/app/certs/ca.crt \
  -e CATALOG_TLS_SERVER_CERT_PATH=/app/certs/catalog_server.crt \
  -e CATALOG_TLS_SERVER_KEY_PATH=/run/secrets/mobv3_catalog_server_key \
  -e TLS_CLIENT_CERT_PATH=/app/certs/client.crt \
  -e TLS_CLIENT_KEY_PATH=/run/secrets/mobv3_client_key \
  -e XDG_CACHE_HOME=/data/.cache \
  -v "$work_dir/catalog-data:/data:Z" \
  -v "$work_dir/certs/catalog_server.crt:/app/certs/catalog_server.crt:ro,Z" \
  -v "$work_dir/certs/ca.crt:/app/certs/ca.crt:ro,Z" \
  -v "$work_dir/certs/client.crt:/app/certs/client.crt:ro,Z" \
  -v "$work_dir/keys/catalog_server.key:/run/secrets/mobv3_catalog_server_key:ro,Z" \
  -v "$work_dir/keys/client.key:/run/secrets/mobv3_client_key:ro,Z" \
  "$catalog_image" >/dev/null
wait_for_tcp 127.0.0.1 "$CATALOG_HOST_PORT" || die "Catalog did not open port $CATALOG_HOST_PORT"

cat >"$client_env" <<EOF
MOBCTL_SERVER_URL=https://127.0.0.1:$CATALOG_HOST_PORT
CATALOG_IMPORT_ENDPOINT=/catalog/import
CATALOG_IMPORT_TIMEOUT=1m
TLS_CA_PATH=$work_dir/certs/ca.crt
TLS_CLIENT_CERT_PATH=$work_dir/certs/client.crt
TLS_CLIENT_KEY_PATH=$work_dir/certs/client.key
EOF

log "Testing broker publish/subscribe"
topic="mobv3/integration/retained/${run_id}"
payload="retained-${run_id}"
mosquitto_client mosquitto_pub -h 127.0.0.1 -p "$MQTT_HOST_PORT" \
  --cafile /certs/ca.crt --cert /certs/client.crt --key /certs/client.key \
  -t "$topic" -m "$payload" -r -q 1
received="$(mosquitto_client mosquitto_sub -h 127.0.0.1 -p "$MQTT_HOST_PORT" \
  --cafile /certs/ca.crt --cert /certs/client.crt --key /certs/client.key \
  -t "$topic" -C 1 -W 5)"
[[ "$received" == "$payload" ]] || die "retained message mismatch"

log "Testing catalog import and lookup"
mobctl upd --mdb "$mdb_fixture"

response_topic="product/integration-station"
request_topic="product/integration-station/8590000000035"
: >"$response_file"
mosquitto_client mosquitto_sub -h 127.0.0.1 -p "$MQTT_HOST_PORT" \
  --cafile /certs/ca.crt --cert /certs/client.crt --key /certs/client.key \
  -t "$response_topic" -C 1 -W 10 >"$response_file" &
sub_pid=$!
sleep 1
mosquitto_client mosquitto_pub -h 127.0.0.1 -p "$MQTT_HOST_PORT" \
  --cafile /certs/ca.crt --cert /certs/client.crt --key /certs/client.key \
  -t "$request_topic" -n -q 1
wait "$sub_pid"

jq -e '
  .barcode == "8590000000035" and
  .name == "Med lesni" and
  .price == "149.00" and
  .stock == "5" and
  .unitOfMeasure == "ks" and
  .unitOfMeasureCoef == "1" and
  .valid == true
' "$response_file" >/dev/null || die "catalog lookup response mismatch"

log "Testing concurrent import rejection"
podman exec -u 0 "$catalog_container" sh -c 'mv /app/mdb_to_csv.sh /app/mdb_to_csv.real && cat > /app/mdb_to_csv.sh <<'"'"'EOF'"'"'
#!/usr/bin/env sh
sleep 5
exec /app/mdb_to_csv.real "$@"
EOF
chmod +x /app/mdb_to_csv.sh'

first_log="$work_dir/first-import.log"
second_log="$work_dir/second-import.log"
mobctl upd --mdb "$mdb_fixture" >"$first_log" 2>&1 &
first_pid=$!
sleep 1
if mobctl upd --mdb "$mdb_fixture" >"$second_log" 2>&1; then
  second_status=0
else
  second_status=$?
fi
wait "$first_pid"
podman exec -u 0 "$catalog_container" sh -c 'mv /app/mdb_to_csv.real /app/mdb_to_csv.sh'

if [[ "$second_status" -eq 0 ]]; then
  cat "$second_log" >&2
  die "second concurrent import unexpectedly succeeded"
fi
grep -q "409 Conflict: catalog import already running" "$second_log" || {
  cat "$second_log" >&2
  die "second concurrent import did not report expected 409"
}

log "MOBv3 integration OK"
