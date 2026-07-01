#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cert_dir=""

usage() {
  echo "usage: deploy/tls/verify_tls.sh --cert-dir <path>" >&2
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

required=(
  "$cert_dir/ca.crt"
  "$cert_dir/mosquitto_server.crt"
  "$cert_dir/catalog_server.crt"
  "$cert_dir/client.crt"
  "$cert_dir/mosquitto_server.key"
  "$cert_dir/catalog_server.key"
  "$cert_dir/client.key"
)

for path in "${required[@]}"; do
  if [ ! -r "$path" ]; then
    echo "missing or unreadable TLS file: $path" >&2
    exit 1
  fi
done

openssl verify -CAfile "$cert_dir/ca.crt" "$cert_dir/mosquitto_server.crt" "$cert_dir/catalog_server.crt" "$cert_dir/client.crt" >/dev/null

mosquitto_server_key_hash="$(openssl pkey -in "$cert_dir/mosquitto_server.key" -pubout -outform DER | sha256sum | awk '{print $1}')"
mosquitto_server_cert_hash="$(openssl x509 -in "$cert_dir/mosquitto_server.crt" -pubkey -noout | openssl pkey -pubin -outform DER | sha256sum | awk '{print $1}')"
catalog_server_key_hash="$(openssl pkey -in "$cert_dir/catalog_server.key" -pubout -outform DER | sha256sum | awk '{print $1}')"
catalog_server_cert_hash="$(openssl x509 -in "$cert_dir/catalog_server.crt" -pubkey -noout | openssl pkey -pubin -outform DER | sha256sum | awk '{print $1}')"
client_key_hash="$(openssl pkey -in "$cert_dir/client.key" -pubout -outform DER | sha256sum | awk '{print $1}')"
client_cert_hash="$(openssl x509 -in "$cert_dir/client.crt" -pubkey -noout | openssl pkey -pubin -outform DER | sha256sum | awk '{print $1}')"

if [ "$mosquitto_server_key_hash" != "$mosquitto_server_cert_hash" ]; then
  echo "mosquitto_server.crt does not match mosquitto_server.key" >&2
  exit 1
fi

if [ "$catalog_server_key_hash" != "$catalog_server_cert_hash" ]; then
  echo "catalog_server.crt does not match catalog_server.key" >&2
  exit 1
fi

if [ "$client_key_hash" != "$client_cert_hash" ]; then
  echo "client.crt does not match client.key" >&2
  exit 1
fi

echo "TLS verification OK"
