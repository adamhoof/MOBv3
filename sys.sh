#!/usr/bin/env bash
set -Eeuo pipefail

ADMIN_USER="${ADMIN_USER:-adamhoof}"
ADMIN_GROUP="${ADMIN_GROUP:-wheel}"
SSH_PORT="${SSH_PORT:-2002}"
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
MQTT_BROKER="8883"
HTTP_UPD_PORT="8443"
QRPROXY_PORT="9100"
PRINTER_GROUP="${PRINTER_GROUP:-lp}"
PRINTER_UDEV_RULE="${PRINTER_UDEV_RULE:-/etc/udev/rules.d/99-mobv3-usb-printer.rules}"
SSHD_DROPIN_DIR="${SSHD_DROPIN_DIR:-/etc/ssh/sshd_config.d}"
SSHD_DROPIN="${SSHD_DROPIN:-$SSHD_DROPIN_DIR/99-custom.conf}"
SSHD_SERVICE="${SSHD_SERVICE:-sshd}"
NOLOGIN_SHELL="${NOLOGIN_SHELL:-/usr/sbin/nologin}"
JOURNALD_DROPIN_DIR="${JOURNALD_DROPIN_DIR:-/etc/systemd/journald.conf.d}"
JOURNALD_DROPIN="${JOURNALD_DROPIN:-$JOURNALD_DROPIN_DIR/10-volatile.conf}"
JOURNALD_RUNTIME_MAX_USE="${JOURNALD_RUNTIME_MAX_USE:-64M}"
DISABLE_RSYSLOG="${DISABLE_RSYSLOG:-1}"
TOR_SERVICE="${TOR_SERVICE:-tor}"
TORRC="${TORRC:-/etc/tor/torrc}"
TORRC_DROPIN_DIR="${TORRC_DROPIN_DIR:-/etc/tor/torrc.d}"
TOR_SSH_DROPIN="${TOR_SSH_DROPIN:-$TORRC_DROPIN_DIR/ssh-onion.conf}"
TOR_SSH_HS_DIR="${TOR_SSH_HS_DIR:-/var/lib/tor/ssh}"
TOR_SSH_ONION_PORT="${TOR_SSH_ONION_PORT:-2002}"
TORADDR_EXPORT="${TORADDR_EXPORT:-$SCRIPT_DIR/toraddr}"

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }
need() { have "$1" || die "Missing required command: $1"; }

[[ "$SSH_PORT" =~ ^[0-9]+$ ]] && (( SSH_PORT >= 1 && SSH_PORT <= 65535 )) || die "SSH_PORT must be 1-65535; got '$SSH_PORT'"
[[ "$TOR_SSH_ONION_PORT" =~ ^[0-9]+$ ]] && (( TOR_SSH_ONION_PORT >= 1 && TOR_SSH_ONION_PORT <= 65535 )) || die "TOR_SSH_ONION_PORT must be 1-65535; got '$TOR_SSH_ONION_PORT'"

for cmd in chmod chown firewall-cmd getent grep id install ip jq loginctl openssl passwd podman sudo systemctl systemd-creds tar tee tor udevadm useradd usermod; do
  need "$cmd"
done

if have sshd; then
  SSHD_BIN="$(command -v sshd)"
elif [[ -x /usr/sbin/sshd ]]; then
  SSHD_BIN=/usr/sbin/sshd
else
  die "Missing required command: sshd"
fi

if [[ ! -x "$NOLOGIN_SHELL" && "$NOLOGIN_SHELL" == /usr/sbin/nologin && -x /sbin/nologin ]]; then
  NOLOGIN_SHELL=/sbin/nologin
fi

[[ -x "$NOLOGIN_SHELL" ]] || die "nologin shell does not exist or is not executable: $NOLOGIN_SHELL"
getent group "$ADMIN_GROUP" >/dev/null || die "Group '$ADMIN_GROUP' does not exist. Set ADMIN_GROUP=sudo on Debian/Ubuntu-like systems."
sudo firewall-cmd --state >/dev/null || die "firewalld is not running"

if id -u "$ADMIN_USER" >/dev/null 2>&1; then
  log "User '$ADMIN_USER' already exists"
else
  log "Creating user '$ADMIN_USER'"
  sudo useradd -m "$ADMIN_USER"
  sudo passwd "$ADMIN_USER"
fi

log "Ensuring '$ADMIN_USER' is in '$ADMIN_GROUP'"
sudo usermod -aG "$ADMIN_GROUP" "$ADMIN_USER"

log "Ensuring '$ADMIN_USER' can access USB receipt printer"
getent group "$PRINTER_GROUP" >/dev/null || die "Printer group '$PRINTER_GROUP' does not exist"
sudo usermod -aG "$PRINTER_GROUP" "$ADMIN_USER"

log "Enabling lingering user services for '$ADMIN_USER'"
sudo loginctl enable-linger "$ADMIN_USER"

admin_entry="$(getent passwd "$ADMIN_USER")"
IFS=: read -r _ _ _ _ _ admin_home _ <<< "$admin_entry"
[[ -n "$admin_home" ]] || die "User '$ADMIN_USER' has no home directory"
admin_group="$(id -gn "$ADMIN_USER")"
authorized_keys="$admin_home/.ssh/authorized_keys"

sudo install -d -m 700 -o "$ADMIN_USER" -g "$admin_group" "$admin_home/.ssh"

if ! sudo test -s "$authorized_keys"; then
  printf '%s\n' '------------------------------------------------'
  printf 'STOP: from your local PC, run:\n'
  printf '  ssh-keygen -t ed25519 -C "%s"\n' "$ADMIN_USER"
  printf '  ssh-copy-id %s@server_ip\n' "$ADMIN_USER"
  printf 'If SSH is already on port %s, run:\n' "$SSH_PORT"
  printf '  ssh-copy-id -p %s %s@server_ip\n' "$SSH_PORT" "$ADMIN_USER"
  printf '%s\n' '------------------------------------------------'

  [[ -t 0 ]] || die "No SSH key found at '$authorized_keys' and stdin is not interactive"
  read -r -p 'Press [Enter] only after the key is installed... '
  sudo test -s "$authorized_keys" || die "No SSH key found at '$authorized_keys'; refusing to disable password SSH"
fi

sudo chown "$ADMIN_USER:$admin_group" "$admin_home/.ssh" "$authorized_keys"
sudo chmod 700 "$admin_home/.ssh"
sudo chmod 600 "$authorized_keys"

log "Writing sshd drop-in"
sudo install -d -m 755 -o root -g root "$SSHD_DROPIN_DIR"
sudo tee "$SSHD_DROPIN" >/dev/null <<EOF
Port $SSH_PORT
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
EOF
sudo chown root:root "$SSHD_DROPIN"
sudo chmod 600 "$SSHD_DROPIN"

log "Validating sshd config"
sudo "$SSHD_BIN" -t

if have getenforce && [[ "$(getenforce)" != Disabled ]]; then
  need semanage
  log "Allowing sshd on tcp/$SSH_PORT in SELinux"
  sudo semanage port -a -t ssh_port_t -p tcp "$SSH_PORT" 2>/dev/null || sudo semanage port -m -t ssh_port_t -p tcp "$SSH_PORT"
fi

log "Opening firewall port tcp/$SSH_PORT"
sudo firewall-cmd --permanent --add-port="${SSH_PORT}/tcp"
sudo firewall-cmd --permanent --add-port="${HTTP_UPD_PORT}/tcp"
sudo firewall-cmd --permanent --add-port="${MQTT_BROKER}/tcp"
sudo firewall-cmd --permanent --add-port="${QRPROXY_PORT}/tcp"
sudo firewall-cmd --reload

log "Writing Tor SSH onion service drop-in"
sudo install -d -m 755 -o root -g root "$TORRC_DROPIN_DIR"
if ! sudo grep -Fxq "%include $TORRC_DROPIN_DIR/*.conf" "$TORRC"; then
  log "Enabling Tor drop-in config directory"
  printf '\n%%include %s/*.conf\n' "$TORRC_DROPIN_DIR" | sudo tee -a "$TORRC" >/dev/null
fi
sudo tee "$TOR_SSH_DROPIN" >/dev/null <<EOF
HiddenServiceDir $TOR_SSH_HS_DIR/
HiddenServicePort $TOR_SSH_ONION_PORT 127.0.0.1:$SSH_PORT
EOF
sudo chown root:root "$TOR_SSH_DROPIN"
sudo chmod 644 "$TOR_SSH_DROPIN"

log "Validating Tor config"
sudo tor --verify-config -f "$TORRC"

log "Enabling and restarting $TOR_SERVICE"
sudo systemctl enable --now "$TOR_SERVICE"
sudo systemctl restart "$TOR_SERVICE"
sudo systemctl is-active --quiet "$TOR_SERVICE" || die "$TOR_SERVICE did not become active after restart"

log "Exporting Tor SSH onion address to $TORADDR_EXPORT"
sudo test -s "$TOR_SSH_HS_DIR/hostname" || die "Tor onion hostname was not created at '$TOR_SSH_HS_DIR/hostname'"
sudo install -m 600 -o root -g root /dev/null "$TORADDR_EXPORT"
sudo sh -c 'cat "$1" > "$2"' sh "$TOR_SSH_HS_DIR/hostname" "$TORADDR_EXPORT"
sudo chown root:root "$TORADDR_EXPORT"
sudo chmod 000 "$TORADDR_EXPORT"
log "Tor SSH onion is exported at $TORADDR_EXPORT (root-owned, mode 000); connect with: torsocks ssh -p $TOR_SSH_ONION_PORT $ADMIN_USER@<address-from-file>"

log "Installing USB receipt printer udev rule"
sudo tee "$PRINTER_UDEV_RULE" >/dev/null <<EOF
SUBSYSTEM=="usbmisc", KERNEL=="lp[0-9]*", OWNER="$ADMIN_USER", GROUP="$PRINTER_GROUP", MODE="0660"
EOF
sudo chown root:root "$PRINTER_UDEV_RULE"
sudo chmod 644 "$PRINTER_UDEV_RULE"
sudo udevadm control --reload-rules
sudo udevadm trigger --subsystem-match=usbmisc || true

log "Configuring system journal logs in RAM only"
sudo install -d -m 755 -o root -g root "$JOURNALD_DROPIN_DIR"
sudo tee "$JOURNALD_DROPIN" >/dev/null <<EOF
[Journal]
Storage=volatile
RuntimeMaxUse=$JOURNALD_RUNTIME_MAX_USE
EOF
sudo chown root:root "$JOURNALD_DROPIN"
sudo chmod 644 "$JOURNALD_DROPIN"
sudo systemctl restart systemd-journald
sudo rm -rf /var/log/journal

if [[ "$DISABLE_RSYSLOG" == "1" ]] && systemctl list-unit-files rsyslog.service >/dev/null 2>&1; then
  log "Disabling rsyslog persistent log writer"
  sudo systemctl disable --now rsyslog.service || true
fi

log "Enabling and restarting $SSHD_SERVICE"
sudo systemctl enable --now "$SSHD_SERVICE"
sudo systemctl restart "$SSHD_SERVICE"
sudo systemctl is-active --quiet "$SSHD_SERVICE" || die "$SSHD_SERVICE did not become active after restart"

if (( SSH_PORT != 22 )); then
  log "Removing default firewall ssh service"
  sudo firewall-cmd --permanent --remove-service=ssh
  sudo firewall-cmd --reload
fi

log "Disabling root shell login"
sudo passwd -l root
sudo usermod -s "$NOLOGIN_SHELL" root

sudo firewall-cmd --list-all
log "System hardening completed successfully"
