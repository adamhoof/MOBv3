#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

compose=(podman compose)
client_env="$(mktemp)"
response_file="$(mktemp)"
cert_dir=""

usage() {
  echo "usage: tests/integration.sh --cert-dir <path>" >&2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --cert-dir)
      if [ "$#" -lt 2 ]; then
        usage
        exit 2
      fi
      cert_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [ -z "$cert_dir" ]; then
  usage
  exit 2
fi

if [[ "$cert_dir" != /* ]]; then
  cert_dir="$repo_root/$cert_dir"
fi

cleanup() {
  rm -f "$client_env" "$response_file"
  "${compose[@]}" down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

require_fixture() {
  if [ ! -f tests/data/catalog_5_products.mdb ]; then
    echo "missing tests/data/catalog_5_products.mdb" >&2
    exit 1
  fi
}

load_env() {
  if [ ! -f .env ]; then
    cp .env.example .env
  fi

  set -a
  source .env
  set +a
  export CERT_DIR="$cert_dir"
}

prepare_tls() {
  "${compose[@]}" down -v >/dev/null 2>&1 || true
  "$repo_root/conf/conf_gen.sh" --server-name localhost --cert-dir "$cert_dir" --force >/dev/null 2>&1
  "$repo_root/conf/verify_tls.sh" --cert-dir "$cert_dir"
}

start_stack() {
  MOBV3_APP_DIR="$repo_root" bash "$repo_root/systemd/mobv3-compose-start.sh" mosquitto_broker catalog_service
}

test_broker_persistence() {
  local topic="mobv3/integration/retained"
  local payload="retained-$(date +%s)"
  local received=""

  podman exec mobv3_mosquitto_broker sh -c 'test -w /mosquitto/data && test -w /mosquitto/log && test -r /run/secrets/mobv3_mosquitto_server_key'

  podman run --rm --network host \
    -v "$cert_dir:/certs:ro,z" \
    "$MOSQUITTO_IMAGE" \
    mosquitto_pub -h localhost -p 8883 \
    --cafile /certs/ca.crt \
    --cert /certs/client.crt \
    --key /certs/client.key \
    -t "$topic" -m "$payload" -r -q 1

  "${compose[@]}" restart mosquitto_broker

  received="$(podman run --rm --network host \
    -v "$cert_dir:/certs:ro,z" \
    "$MOSQUITTO_IMAGE" \
    mosquitto_sub -h localhost -p 8883 \
    --cafile /certs/ca.crt \
    --cert /certs/client.crt \
    --key /certs/client.key \
    -t "$topic" -C 1 -W 5)"

  if [ "$received" != "$payload" ]; then
    echo "retained message mismatch: got '$received', want '$payload'" >&2
    exit 1
  fi

  podman exec mobv3_mosquitto_broker sh -c 'test -s /mosquitto/data/mosquitto.db'
  echo "broker persistence OK"
}

write_client_env() {
  cat > "$client_env" <<EOF
MOBCTL_SERVER_URL=https://localhost:8443
CATALOG_IMPORT_ENDPOINT=/catalog/import
CATALOG_IMPORT_TIMEOUT=1m
TLS_CA_PATH=$cert_dir/ca.crt
TLS_CLIENT_CERT_PATH=$cert_dir/client.crt
TLS_CLIENT_KEY_PATH=$cert_dir/client.key
EOF
}

test_catalog_import_and_lookup() {
  local station="integration-station"
  local barcode="8590000000035"
  local response_topic="product/$station"
  local request_topic="product/$station/$barcode"

  write_client_env
  go run ./cmd/mobctl --file "$client_env" upd --mdb tests/data/catalog_5_products.mdb

  : > "$response_file"
  podman run --rm --network host \
    -v "$cert_dir:/certs:ro,z" \
    "$MOSQUITTO_IMAGE" \
    mosquitto_sub -h localhost -p 8883 \
    --cafile /certs/ca.crt \
    --cert /certs/client.crt \
    --key /certs/client.key \
    -t "$response_topic" -C 1 -W 10 \
    > "$response_file" &
  local sub_pid=$!

  sleep 1

  podman run --rm --network host \
    -v "$cert_dir:/certs:ro,z" \
    "$MOSQUITTO_IMAGE" \
    mosquitto_pub -h localhost -p 8883 \
    --cafile /certs/ca.crt \
    --cert /certs/client.crt \
    --key /certs/client.key \
    -t "$request_topic" -n -q 1

  wait "$sub_pid"

  python3 - "$response_file" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    payload = json.load(f)

expected = {
    "barcode": "8590000000035",
    "name": "Med lesni",
    "price": "149.00",
    "stock": "5",
    "unitOfMeasure": "ks",
    "unitOfMeasureCoef": "1",
    "valid": True,
}

for key, value in expected.items():
    if payload.get(key) != value:
        raise SystemExit(f"{key}: got {payload.get(key)!r}, want {value!r}; payload={payload!r}")
PY

  echo "catalog import and lookup OK"
}

test_concurrent_import_rejected() {
  local first_log="$(mktemp)"
  local second_log="$(mktemp)"
  local first_status=0
  local second_status=0

  podman exec -u 0 mobv3_catalog_service sh -c 'mv /app/mdb_to_csv.sh /app/mdb_to_csv.real && cat > /app/mdb_to_csv.sh <<'"'"'EOF'"'"'
#!/usr/bin/env sh
sleep 5
exec /app/mdb_to_csv.real "$@"
EOF
chmod +x /app/mdb_to_csv.sh'

  go run ./cmd/mobctl --file "$client_env" upd --mdb tests/data/catalog_5_products.mdb >"$first_log" 2>&1 &
  local first_pid=$!
  sleep 1

  if go run ./cmd/mobctl --file "$client_env" upd --mdb tests/data/catalog_5_products.mdb >"$second_log" 2>&1; then
    second_status=0
  else
    second_status=$?
  fi

  if wait "$first_pid"; then
    first_status=0
  else
    first_status=$?
  fi

  podman exec -u 0 mobv3_catalog_service sh -c 'mv /app/mdb_to_csv.real /app/mdb_to_csv.sh'

  if [ "$first_status" -ne 0 ]; then
    cat "$first_log" >&2
    echo "first import failed with status $first_status" >&2
    exit 1
  fi
  if [ "$second_status" -eq 0 ]; then
    cat "$second_log" >&2
    echo "second concurrent import unexpectedly succeeded" >&2
    exit 1
  fi
  if ! grep -q "409 Conflict: catalog import already running" "$second_log"; then
    cat "$second_log" >&2
    echo "second concurrent import did not report expected 409" >&2
    exit 1
  fi

  rm -f "$first_log" "$second_log"
  echo "concurrent import rejection OK"
}

require_fixture
load_env
prepare_tls
start_stack
test_broker_persistence
test_catalog_import_and_lookup
test_concurrent_import_rejected

echo "MOBv3 integration OK"
