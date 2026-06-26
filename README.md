# MOBv3

MOBv3 is the next architecture sketch for Medunka OP Barcode.

## Shape

```text
Station -> MQTT/mTLS -> mosquitto -> catalog_service -> Turso DB file
   ^                                      |
   +-------------- MQTT response ---------+

mobctl -> HTTPS/mTLS raw stream -> catalog_service -> MDB convert -> Turso import

Pohoda -> qrproxy -> USB printer
                 +-> MQTT/mTLS -> QTerm
```

## Main Changes From MOB2

- `catalog_service` replaces `http_db_update_server`, `mqtt_product_api`, and PostgreSQL.
- Turso Database via `tursogo` owns local embedded catalog storage.
- Import is one blocking raw HTTP request, not multipart and not job polling.
- External surfaces keep certificate auth: HTTPS/mTLS import and MQTT/mTLS.
- Internal service-to-database auth disappears because the DB is an embedded file owned by `catalog_service`.
- Mosquitto remains separate because it is an infrastructure boundary, not catalog logic.

## Import API

```http
POST /catalog/import
Content-Type: application/gzip
X-Filename: catalog.mdb.gz
```

The response is final:

```json
{"status":"completed","imported":12345,"duration":"1m12s"}
```

Errors return non-2xx with:

```json
{"status":"failed","error":"..."}
```

## Operator CLI

```sh
mobctl --file .env upd --mdb catalog.mdb
mobctl --file .env ss
mobctl --file .env sw
```

The default `mobctl` build contains only `upd`, `ss`, and `sw`.

Build the admin variant to include OTA commands:

```sh
go build -tags admin ./cmd/mobctl
mobctl --file .env qterm-ota qterm.bin
mobctl --file .env barcode-ota station.bin
```

`barcode-ota` publishes one shared OTA stream to `station/ota/control` and `station/ota/chunk`. Stations report `started` on `station/ota/status` with their MAC-derived device ID; `mobctl` discovers responders during the begin window and requires every discovered station to acknowledge chunks and `done` for that session.

## Catalog Import Strategy

Import writes into `products_next`, indexes it, then atomically swaps it into `products` in one transaction. Lookups keep querying `products`.

## Support Scripts

Generate TLS material and Podman secrets:

```sh
conf/conf_gen.sh --server-name <host-or-ip> --cert-dir <cert-dir>
```

Use the IP/DNS name that clients will actually connect to. It is written into the server certificate SAN.

Use `--force` to replace existing generated Podman secrets:

```sh
conf/conf_gen.sh --server-name <host-or-ip> --cert-dir <cert-dir> --force
```

Verify TLS files against each other without regenerating anything:

```sh
conf/verify_tls.sh --cert-dir <cert-dir>
```

Start selected Compose services and wait for them to become ready:

```sh
systemd/mobv3-compose-start.sh mosquitto_broker catalog_service
```

Run the integration acceptance test:

```sh
tests/integration.sh --cert-dir <cert-dir>
```

The integration test generates local test TLS material if needed, starts the required services, imports `tests/data/catalog_5_products.mdb`, verifies MQTT lookup, and checks concurrent import rejection.

## Autostart

Register the user systemd unit from `systemd/mobv3-compose.service`. It calls `systemd/mobv3-compose-start.sh`, which launches the configured Compose services and waits for readiness.

```sh
systemctl --user enable --now mobv3-compose.service
systemctl --user status mobv3-compose.service
```
