# MOBv3 Deployment Draft

This directory contains deployment configuration and generators. Application
code stays under `source/`; generated runtime files stay under
`deploy/generated/` and are ignored by git.

Layout:

```text
deploy/config/     versioned machine-specific config inputs
deploy/lib/        generator helpers
deploy/services/   per-service generators
deploy/generated/  ignored generated service env files and Quadlet units
deploy/mosquitto/  Mosquitto runtime config
deploy/tls/        TLS generation and verification helpers
```

Versioned deployment config lives in `deploy/config/machine.env`. It contains
only values that must be chosen on the target machine:

```sh
$EDITOR deploy/config/machine.env
```

The generator intentionally fails loudly if `HOST_BIND_IP` or `USB_PRINTER` is
blank. They are physical machine facts and should be filled by the operator.

Everything else is project-owned and hardcoded in the generator layer: ports,
service names, image tags, cert paths, credential paths, topics, and in-container
paths.

Generate self-contained service env files and Quadlet units:

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
