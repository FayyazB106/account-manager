#!/usr/bin/env bash
# Updates a Linux server install of Account Manager (the systemd setup from DEPLOY.md).
#
#   sudo bash scripts/update.sh v1.1.0            # download that release, verify, install
#   sudo bash scripts/update.sh latest            # same, for the newest release
#   sudo bash scripts/update.sh ./account-manager-linux-amd64.tar.gz   # use a file you copied over
#
# What it does: backs up the data folder and the current binary, stops the service,
# installs the new binary, starts it, and checks it answers. If it does not come up it
# puts the old binary and data back and starts the old version again.
# Your .env (master key) and data/ are never overwritten by the update itself.
#
# Settings (environment variables, all optional):
#   INSTALL_DIR   where the app lives            (default /opt/account-manager)
#   SERVICE       systemd service name           (default account-manager)
#   SERVICE_USER  user the service runs as       (default accountmgr)
#   PORT          port to health-check           (default 8080)
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/account-manager}"
SERVICE="${SERVICE:-account-manager}"
SERVICE_USER="${SERVICE_USER:-accountmgr}"
PORT="${PORT:-8080}"
REPO="FayyazB106/account-manager"
BIN_NAME="account-manager-linux-amd64"
ASSET="$BIN_NAME.tar.gz"

die() { echo "ERROR: $*" >&2; exit 1; }
say() { echo "==> $*"; }

[ "$(id -u)" -eq 0 ] || die "run as root (use sudo)"
[ $# -eq 1 ] || die "usage: sudo bash $0 <vX.Y.Z | latest | path/to/$ASSET>"
[ -x "$INSTALL_DIR/$BIN_NAME" ] || die "no $BIN_NAME in $INSTALL_DIR — set INSTALL_DIR if it is elsewhere"
command -v systemctl >/dev/null || die "systemctl not found (this script is for systemd installs)"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# ---- 1. get the new tarball ------------------------------------------------
ARG="$1"
if [ -f "$ARG" ]; then
  TARBALL="$ARG"
  say "using $TARBALL"
else
  if [ "$ARG" = "latest" ]; then
    BASE="https://github.com/$REPO/releases/latest/download"
  else
    case "$ARG" in v*) ;; *) die "'$ARG' is not a file, 'latest' or a version like v1.1.0";; esac
    BASE="https://github.com/$REPO/releases/download/$ARG"
  fi
  command -v curl >/dev/null || die "curl is needed to download a release"
  say "downloading $BASE/$ASSET"
  curl -fL --proto '=https' -o "$WORK/$ASSET" "$BASE/$ASSET" || die "download failed"
  curl -fL --proto '=https' -o "$WORK/SHA256SUMS" "$BASE/SHA256SUMS" || die "could not download SHA256SUMS to verify the release"
  say "verifying checksum"
  ( cd "$WORK" && grep " $ASSET\$" SHA256SUMS | sha256sum -c - ) || die "checksum mismatch — not installing"
  TARBALL="$WORK/$ASSET"
fi

say "unpacking"
mkdir "$WORK/new"
tar -xzf "$TARBALL" -C "$WORK/new" || die "could not unpack $TARBALL"
[ -f "$WORK/new/$BIN_NAME" ] || die "$BIN_NAME is not inside the tarball"

# ---- 2. back up, then swap ---------------------------------------------------
STAMP="$(date +%Y%m%d-%H%M%S)"
OLD_BIN="$INSTALL_DIR/$BIN_NAME.bak-$STAMP"
OLD_DATA="$INSTALL_DIR/data.bak-$STAMP"

say "stopping $SERVICE"
systemctl stop "$SERVICE"

say "backing up the binary and data to $INSTALL_DIR/*.bak-$STAMP"
cp -a "$INSTALL_DIR/$BIN_NAME" "$OLD_BIN"
if [ -d "$INSTALL_DIR/data" ]; then cp -a "$INSTALL_DIR/data" "$OLD_DATA"; fi

rollback() {
  echo "!! the new version did not start — rolling back" >&2
  systemctl stop "$SERVICE" || true
  cp -a "$OLD_BIN" "$INSTALL_DIR/$BIN_NAME"
  if [ -d "$OLD_DATA" ]; then rm -rf "$INSTALL_DIR/data"; cp -a "$OLD_DATA" "$INSTALL_DIR/data"; fi
  systemctl start "$SERVICE" || true
  echo "Rolled back to the previous version. Check: journalctl -u $SERVICE -n 50" >&2
  exit 1
}

say "installing the new binary"
install -m 0755 -o "$SERVICE_USER" -g "$SERVICE_USER" "$WORK/new/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME" \
  || { cp -a "$OLD_BIN" "$INSTALL_DIR/$BIN_NAME"; systemctl start "$SERVICE" || true; die "could not install the binary"; }

# ---- 3. start and check ----------------------------------------------------
say "starting $SERVICE"
systemctl start "$SERVICE" || rollback

say "waiting for it to answer on port $PORT"
ok=""
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1; then ok=1; break; fi
  systemctl is-active --quiet "$SERVICE" || break
  sleep 1
done
[ -n "$ok" ] || rollback

say "updated and running. Backups kept:"
echo "    $OLD_BIN"
[ -d "$OLD_DATA" ] && echo "    $OLD_DATA"
echo "Delete them once you are happy (they hold copies of your data)."
