# MOBv3 Deployment Draft

This directory contains deployment configuration and generators. Application
code stays under `source/`; generated files stay under `deploy/generated/` and
are ignored by git.

Layout:

```text
deploy/env/        committed per-service runtime/config env files
deploy/services/   per-service generators
deploy/generated/  ignored generated Quadlet units, systemd target, and Mosquitto config
deploy/tls/        TLS generation and verification helpers
```

Runtime service config lives in committed per-service env files:

```text
deploy/env/mosquitto.env
deploy/env/catalog.env
deploy/env/qrproxy.env
```

They contain no secrets and are loaded directly by the generated Quadlet units
where the service needs runtime environment. Host deployment facts such as
`APP_DIR`, `CERT_DIR`, `CREDENTIAL_DIR`, image tags, `PUBLIC_BIND_IP`, and
`USB_PRINTER` are generator variables with defaults in `deploy/generate.sh`.
Override them only when needed:

```sh
PUBLIC_BIND_IP=<mini-pc-lan-ip> USB_PRINTER=/dev/usb/lp0 deploy/generate.sh
```

The generator auto-detects `PUBLIC_BIND_IP` from the host's default IPv4 route
when it is not set. `USB_PRINTER` defaults to `/dev/usb/lp0`.

Generate Quadlet units:

```sh
deploy/generate.sh
```

Install generated Quadlet units on the target host:

```sh
mkdir -p ~/.config/containers/systemd
cp deploy/generated/quadlet/* ~/.config/containers/systemd/
mkdir -p ~/.config/systemd/user
cp deploy/generated/systemd/* ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now mobv3.target
```

Start services manually without enabling autostart:

```sh
systemctl --user enable --now mobv3-mosquitto.service
systemctl --user enable --now mobv3-catalog.service
systemctl --user enable --now mobv3-qrproxy.service
```

The generated Quadlet units include `.build` units for local application images,
so there is no separate image build script.

Generate public certs and encrypted credentials:

```sh
deploy/tls/conf_gen.sh --server-name <host-or-ip> --cert-dir certs
```

Public certs are written to `certs/`. Private keys are generated in a temporary
directory, verified, encrypted with `systemd-creds --user`, and removed.
Encrypted credentials are written to `deploy/credentials/` by default and are
ignored by git.

Generated Quadlet units use `LoadCredentialEncrypted=` and read-only `%d/...`
mounts to expose decrypted keys to containers at `/run/secrets/...`.
