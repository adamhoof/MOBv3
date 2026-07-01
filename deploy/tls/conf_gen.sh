#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: deploy/tls/conf_gen.sh --server-name <ip-or-dns> --cert-dir <path> [--force]

Generates MOBv3 TLS certs and creates Podman secrets:
  certs/ca.crt
  certs/mosquitto_server.crt
  certs/catalog_server.crt
  certs/client.crt
  mobv3_mosquitto_server_key secret
  mobv3_catalog_server_key secret
  mobv3_client_key secret

Private keys are generated in --cert-dir and imported into Podman secrets.
Keep *.key offline/private or remove them after verifying secrets were created.
EOF
}

server_name=""
cert_dir=""
force=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --server-name)
      if [ "$#" -lt 2 ]; then
        usage
        exit 2
      fi
      server_name="$2"
      shift 2
      ;;
    --cert-dir)
      if [ "$#" -lt 2 ]; then
        usage
        exit 2
      fi
      cert_dir="$2"
      shift 2
      ;;
    --force)
      force=1
      shift
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

if [ -z "$server_name" ] || [ -z "$cert_dir" ]; then
  usage
  exit 2
fi

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required" >&2
  exit 1
fi

if ! command -v podman >/dev/null 2>&1; then
  echo "podman is required" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ "$cert_dir" != /* ]]; then
  cert_dir="$repo_root/$cert_dir"
fi

mkdir -p "$cert_dir"
chmod 700 "$cert_dir"

base_san_entries=("DNS:localhost" "IP:127.0.0.1")
if [[ "$server_name" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || [[ "$server_name" == *:* ]]; then
  base_san_entries+=("IP:$server_name")
else
  base_san_entries+=("DNS:$server_name")
fi
mosquitto_san_entries=("${base_san_entries[@]}" "DNS:mobv3_mosquitto_broker")
catalog_san_entries=("${base_san_entries[@]}")
mosquitto_san="$(IFS=,; echo "${mosquitto_san_entries[*]}")"
catalog_san="$(IFS=,; echo "${catalog_san_entries[*]}")"

for secret in mobv3_mosquitto_server_key mobv3_catalog_server_key mobv3_client_key; do
  if podman secret inspect "$secret" >/dev/null 2>&1; then
    if [ "$force" -ne 1 ]; then
      echo "secret '$secret' already exists; use --force to replace it" >&2
      exit 1
    fi
    podman secret rm "$secret" >/dev/null
  fi
done

openssl req -x509 -newkey rsa:4096 -days 3650 -nodes \
  -keyout "$cert_dir/ca.key" \
  -out "$cert_dir/ca.crt" \
  -subj "/CN=MOBv3 CA"

openssl req -newkey rsa:4096 -nodes \
  -keyout "$cert_dir/mosquitto_server.key" \
  -out "$cert_dir/mosquitto_server.csr" \
  -subj "/CN=mobv3_mosquitto_broker" \
  -addext "subjectAltName=$mosquitto_san"

openssl x509 -req \
  -in "$cert_dir/mosquitto_server.csr" \
  -CA "$cert_dir/ca.crt" \
  -CAkey "$cert_dir/ca.key" \
  -CAcreateserial \
  -out "$cert_dir/mosquitto_server.crt" \
  -days 825 \
  -copy_extensions copy

openssl req -newkey rsa:4096 -nodes \
  -keyout "$cert_dir/catalog_server.key" \
  -out "$cert_dir/catalog_server.csr" \
  -subj "/CN=$server_name" \
  -addext "subjectAltName=$catalog_san"

openssl x509 -req \
  -in "$cert_dir/catalog_server.csr" \
  -CA "$cert_dir/ca.crt" \
  -CAkey "$cert_dir/ca.key" \
  -CAcreateserial \
  -out "$cert_dir/catalog_server.crt" \
  -days 825 \
  -copy_extensions copy

openssl req -newkey rsa:4096 -nodes \
  -keyout "$cert_dir/client.key" \
  -out "$cert_dir/client.csr" \
  -subj "/CN=mobv3-client"

openssl x509 -req \
  -in "$cert_dir/client.csr" \
  -CA "$cert_dir/ca.crt" \
  -CAkey "$cert_dir/ca.key" \
  -CAcreateserial \
  -out "$cert_dir/client.crt" \
  -days 825

podman secret create mobv3_mosquitto_server_key "$cert_dir/mosquitto_server.key" >/dev/null
podman secret create mobv3_catalog_server_key "$cert_dir/catalog_server.key" >/dev/null
podman secret create mobv3_client_key "$cert_dir/client.key" >/dev/null

"$repo_root/deploy/tls/verify_tls.sh" --cert-dir "$cert_dir"

cat <<EOF
Generated MOBv3 TLS assets for $server_name

Public/client-mounted files:
  $cert_dir/ca.crt
  $cert_dir/mosquitto_server.crt
  $cert_dir/catalog_server.crt
  $cert_dir/client.crt

Podman secrets:
  mobv3_mosquitto_server_key
  mobv3_catalog_server_key
  mobv3_client_key

Private working material:
  $cert_dir/ca.key
  $cert_dir/mosquitto_server.key
  $cert_dir/catalog_server.key
  $cert_dir/client.key
EOF
