#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: deploy/tls/conf_gen.sh --server-name <ip-or-dns> --cert-dir <path> [--credential-dir <path>] [--force]

Generates MOBv3 public TLS certs and systemd encrypted credentials:
  certs/ca.crt
  certs/mosquitto_server.crt
  certs/catalog_server.crt
  certs/client.crt
  deploy/credentials/ca_key.cred
  deploy/credentials/mobv3_mosquitto_server_key.cred
  deploy/credentials/mobv3_catalog_server_key.cred
  deploy/credentials/mobv3_client_key.cred

Private keys are generated in a temporary directory, verified, encrypted with
systemd-creds --user, and then removed.
EOF
}

server_name=""
cert_dir=""
credential_dir=""
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
    --credential-dir)
      if [ "$#" -lt 2 ]; then
        usage
        exit 2
      fi
      credential_dir="$2"
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

if ! command -v systemd-creds >/dev/null 2>&1; then
  echo "systemd-creds is required" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ "$cert_dir" != /* ]]; then
  cert_dir="$repo_root/$cert_dir"
fi
if [ -z "$credential_dir" ]; then
  credential_dir="$repo_root/deploy/credentials"
elif [[ "$credential_dir" != /* ]]; then
  credential_dir="$repo_root/$credential_dir"
fi

mkdir -p "$cert_dir"
mkdir -p "$credential_dir"
chmod 755 "$cert_dir"
chmod 700 "$credential_dir"

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

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

for cred in ca_key mobv3_mosquitto_server_key mobv3_catalog_server_key mobv3_client_key; do
  if [ -e "$credential_dir/$cred.cred" ]; then
    if [ "$force" -ne 1 ]; then
      echo "credential '$credential_dir/$cred.cred' already exists; use --force to replace it" >&2
      exit 1
    fi
    rm -f "$credential_dir/$cred.cred"
  fi
done

openssl req -x509 -newkey rsa:4096 -days 3650 -nodes \
  -keyout "$work_dir/ca.key" \
  -out "$work_dir/ca.crt" \
  -subj "/CN=MOBv3 CA"

openssl req -newkey rsa:4096 -nodes \
  -keyout "$work_dir/mosquitto_server.key" \
  -out "$work_dir/mosquitto_server.csr" \
  -subj "/CN=mobv3_mosquitto_broker" \
  -addext "subjectAltName=$mosquitto_san"

openssl x509 -req \
  -in "$work_dir/mosquitto_server.csr" \
  -CA "$work_dir/ca.crt" \
  -CAkey "$work_dir/ca.key" \
  -CAcreateserial \
  -out "$work_dir/mosquitto_server.crt" \
  -days 825 \
  -copy_extensions copy

openssl req -newkey rsa:4096 -nodes \
  -keyout "$work_dir/catalog_server.key" \
  -out "$work_dir/catalog_server.csr" \
  -subj "/CN=$server_name" \
  -addext "subjectAltName=$catalog_san"

openssl x509 -req \
  -in "$work_dir/catalog_server.csr" \
  -CA "$work_dir/ca.crt" \
  -CAkey "$work_dir/ca.key" \
  -CAcreateserial \
  -out "$work_dir/catalog_server.crt" \
  -days 825 \
  -copy_extensions copy

openssl req -newkey rsa:4096 -nodes \
  -keyout "$work_dir/client.key" \
  -out "$work_dir/client.csr" \
  -subj "/CN=mobv3-client"

openssl x509 -req \
  -in "$work_dir/client.csr" \
  -CA "$work_dir/ca.crt" \
  -CAkey "$work_dir/ca.key" \
  -CAcreateserial \
  -out "$work_dir/client.crt" \
  -days 825

"$repo_root/deploy/tls/verify_tls.sh" --cert-dir "$work_dir"

install -m 644 "$work_dir/ca.crt" "$cert_dir/ca.crt"
install -m 644 "$work_dir/mosquitto_server.crt" "$cert_dir/mosquitto_server.crt"
install -m 644 "$work_dir/catalog_server.crt" "$cert_dir/catalog_server.crt"
install -m 644 "$work_dir/client.crt" "$cert_dir/client.crt"

systemd-creds --user encrypt --name=ca_key "$work_dir/ca.key" "$credential_dir/ca_key.cred"
systemd-creds --user encrypt --name=mobv3_mosquitto_server_key "$work_dir/mosquitto_server.key" "$credential_dir/mobv3_mosquitto_server_key.cred"
systemd-creds --user encrypt --name=mobv3_catalog_server_key "$work_dir/catalog_server.key" "$credential_dir/mobv3_catalog_server_key.cred"
systemd-creds --user encrypt --name=mobv3_client_key "$work_dir/client.key" "$credential_dir/mobv3_client_key.cred"
chmod 600 "$credential_dir"/*.cred

cat <<EOF
Generated MOBv3 TLS assets for $server_name

Public/client-mounted files:
  $cert_dir/ca.crt
  $cert_dir/mosquitto_server.crt
  $cert_dir/catalog_server.crt
  $cert_dir/client.crt

Systemd encrypted credentials:
  $credential_dir/ca_key.cred
  $credential_dir/mobv3_mosquitto_server_key.cred
  $credential_dir/mobv3_catalog_server_key.cred
  $credential_dir/mobv3_client_key.cred

Private working material was removed after encryption.
EOF
