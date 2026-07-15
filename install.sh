#!/bin/sh
set -eu

REPO="${FRP_PANEL_REPO:-LAGcomcom/frp-panel}"
INSTALL_DIR="${FRP_PANEL_INSTALL_DIR:-/opt/frp-panel}"
DATA_DIR="${FRP_PANEL_DATA_DIR:-/var/lib/frp-panel}"
CONFIG_DIR="${FRP_PANEL_CONFIG_DIR:-/etc/frp-panel}"
SERVICE_NAME="frp-panel"
MODE="install"

case "${1:-}" in
  --update) MODE="update" ;;
  "") ;;
  *) echo "Usage: install.sh [--update]" >&2; exit 2 ;;
esac

if [ "$(id -u)" -ne 0 ]; then
  echo "Please run as root (for example: curl ... | sudo sh)." >&2
  exit 1
fi

case "$(uname -s)" in
  Linux) ;;
  *) echo "Only Linux is supported by this installer." >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  INIT="systemd"
elif command -v rc-service >/dev/null 2>&1; then
  INIT="openrc"
else
  echo "Neither systemd nor OpenRC was detected." >&2
  exit 1
fi

TMP_DIR=$(mktemp -d)
BACKUP=""
CONFIG_BACKUP=""
cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT HUP INT TERM

download() {
  url=$1
  output=$2
  if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 --connect-timeout 15 -o "$output" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$output" --timeout=30 --tries=3 "$url"
  else
    echo "curl or wget is required." >&2
    exit 1
  fi
}

if [ -n "${FRP_PANEL_VERSION:-}" ]; then
  RELEASE_BASE="https://github.com/$REPO/releases/download/$FRP_PANEL_VERSION"
else
  RELEASE_BASE="https://github.com/$REPO/releases/latest/download"
fi

ASSET="frp-panel-linux-$ARCH"
download "$RELEASE_BASE/$ASSET" "$TMP_DIR/$ASSET"
download "$RELEASE_BASE/checksums.txt" "$TMP_DIR/checksums.txt"
download "$RELEASE_BASE/version.txt" "$TMP_DIR/version.txt"

expected=$(awk -v file="$ASSET" '$2 == file || $2 == "*" file { print $1; exit }' "$TMP_DIR/checksums.txt")
[ -n "$expected" ] || { echo "Checksum for $ASSET is missing." >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$TMP_DIR/$ASSET" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$TMP_DIR/$ASSET" | awk '{print $1}')
else
  echo "sha256sum or shasum is required." >&2
  exit 1
fi
[ "$actual" = "$expected" ] || { echo "SHA-256 verification failed." >&2; exit 1; }

VERSION=$(tr -d '\r\n' < "$TMP_DIR/version.txt")
[ -n "$VERSION" ] || { echo "Release version is empty." >&2; exit 1; }
case "$VERSION" in
  *[!A-Za-z0-9._-]*) echo "Release version contains unsafe characters." >&2; exit 1 ;;
esac

if ! id frp-panel >/dev/null 2>&1; then
  if command -v useradd >/dev/null 2>&1; then
    useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin frp-panel
  else
    adduser -S -H -h "$DATA_DIR" -s /sbin/nologin frp-panel
  fi
fi

mkdir -p "$INSTALL_DIR" "$DATA_DIR" "$CONFIG_DIR"
chmod 0750 "$DATA_DIR" "$CONFIG_DIR"
chown frp-panel:frp-panel "$DATA_DIR"
chown root:frp-panel "$CONFIG_DIR"

CONFIG="$CONFIG_DIR/config.yaml"
CREATED_PASSWORD=""
if [ ! -f "$CONFIG" ]; then
  random_hex() { od -An -N "$1" -tx1 /dev/urandom | tr -d ' \n'; }
  JWT_SECRET=$(random_hex 32)
  SERVER_TOKEN=$(random_hex 24)
  CREATED_PASSWORD="${FRP_PANEL_ADMIN_PASSWORD:-$(random_hex 12)}"
  ADMIN_EMAIL="${FRP_PANEL_ADMIN_EMAIL:-admin@example.com}"
  PORT="${FRP_PANEL_PORT:-8080}"
  AUTH_SERVER="${FRP_PANEL_AUTH_SERVER:-https://ymsq.movewellpro.fun}"
  cat > "$CONFIG" <<EOF
server:
  host: "0.0.0.0"
  port: $PORT
  mode: "release"
database:
  driver: "sqlite"
  dsn: "$DATA_DIR/frp-panel.db"
redis:
  addr: "localhost:6379"
  password: ""
  db: 0
jwt:
  secret: "$JWT_SECRET"
  expire_time: 24h
  issuer: "frp-panel"
frp:
  default_version: "0.68.0"
  github_mirror: ""
  download_timeout: 300
  plugin_webhook_url: ""
  poller_interval: 30
  server_token: "$SERVER_TOKEN"
admin:
  email: "$ADMIN_EMAIL"
  password: "$CREATED_PASSWORD"
license:
  auth_server: "$AUTH_SERVER"
update:
  center_url: ""
  instance_key: ""
  panel_version: "$VERSION"
  panel_domain: ""
  control_public_key: ""
  identity_key_file: "$DATA_DIR/update-identity.key"
  lease_cache_file: "$DATA_DIR/update-lease.json"
  bootstrap_cache_file: "$DATA_DIR/update-bootstrap.json"
  bootstrap_urls: []
  anonymous_statistics: false
EOF
  chmod 0640 "$CONFIG"
  chown root:frp-panel "$CONFIG"
else
  CONFIG_BACKUP="$TMP_DIR/config.yaml.backup"
  cp -p "$CONFIG" "$CONFIG_BACKUP"
  sed "s/^\([[:space:]]*panel_version:[[:space:]]*\).*/\1\"$VERSION\"/" "$CONFIG_BACKUP" > "$CONFIG"
fi

if [ -f "$INSTALL_DIR/frp-panel" ]; then
  BACKUP="$INSTALL_DIR/frp-panel.backup.$(date +%Y%m%d%H%M%S)"
  cp -p "$INSTALL_DIR/frp-panel" "$BACKUP"
fi
install -m 0755 "$TMP_DIR/$ASSET" "$INSTALL_DIR/frp-panel.new"
mv -f "$INSTALL_DIR/frp-panel.new" "$INSTALL_DIR/frp-panel"

if [ "$INIT" = "systemd" ]; then
  cat > /etc/systemd/system/frp-panel.service <<EOF
[Unit]
Description=FRP Panel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=frp-panel
Group=frp-panel
WorkingDirectory=$DATA_DIR
ExecStart=$INSTALL_DIR/frp-panel -config $CONFIG
Restart=on-failure
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable frp-panel >/dev/null
  if ! systemctl restart frp-panel; then
    [ -z "$CONFIG_BACKUP" ] || cp -p "$CONFIG_BACKUP" "$CONFIG"
    [ -z "$BACKUP" ] || { cp -p "$BACKUP" "$INSTALL_DIR/frp-panel"; systemctl restart frp-panel || true; }
    echo "Service failed to start; the previous binary was restored." >&2
    exit 1
  fi
else
  cat > /etc/init.d/frp-panel <<EOF
#!/sbin/openrc-run
name="FRP Panel"
command="$INSTALL_DIR/frp-panel"
command_args="-config $CONFIG"
command_user="frp-panel:frp-panel"
directory="$DATA_DIR"
command_background="yes"
pidfile="/run/frp-panel.pid"
output_log="/var/log/frp-panel.log"
error_log="/var/log/frp-panel.log"
depend() { need net; }
EOF
  chmod 0755 /etc/init.d/frp-panel
  rc-update add frp-panel default >/dev/null 2>&1 || true
  if ! rc-service frp-panel restart; then
    [ -z "$CONFIG_BACKUP" ] || cp -p "$CONFIG_BACKUP" "$CONFIG"
    [ -z "$BACKUP" ] || { cp -p "$BACKUP" "$INSTALL_DIR/frp-panel"; rc-service frp-panel restart || true; }
    echo "Service failed to start; the previous binary was restored." >&2
    exit 1
  fi
fi

echo "FRP Panel $VERSION $MODE completed."
echo "Config: $CONFIG"
echo "Data:   $DATA_DIR"
if [ -n "$CREATED_PASSWORD" ]; then
  echo "Admin:  $ADMIN_EMAIL"
  echo "Password: $CREATED_PASSWORD"
  echo "Store this password now; it is only printed during first installation."
fi
