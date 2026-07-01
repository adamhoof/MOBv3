# MOBv3 Deployment Draft

This directory contains deployment configuration and generators. Application
code stays under `source/`; generated Quadlet units stay under `deploy/generated/`
and are ignored by git.

Layout:

```text
deploy/env/        committed per-service deployment/runtime env files
deploy/services/   per-service generators
deploy/generated/  ignored generated Quadlet units
deploy/mosquitto/  Mosquitto runtime config
deploy/tls/        TLS generation and verification helpers
```

Deployment wiring lives in committed per-service env files:

```text
deploy/env/mosquitto.env
deploy/env/catalog.env
deploy/env/qrproxy.env
```

They contain no secrets and are loaded directly by the generated Quadlet units.
The only values that must be filled for a target box are physical machine facts:

```sh
MOBV3_HOST_BIND_IP=<mini-pc-lan-ip> MOBV3_USB_PRINTER=/dev/usb/lp0 deploy/generate.sh
```

The generator intentionally fails loudly if `HOST_BIND_IP` or `USB_PRINTER` is
blank. If preferred, fill them directly in the relevant files under `deploy/env/`.

Generate Quadlet units:

```sh
deploy/generate.sh
```

Install generated Quadlet units on the target host:

```sh
mkdir -p ~/.config/containers/systemd
cp deploy/generated/quadlet/* ~/.config/containers/systemd/
systemctl --user daemon-reload
```

Start services:

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
