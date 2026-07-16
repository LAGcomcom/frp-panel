#!/bin/sh
set -eu

REPO="${FRP_PANEL_REPO:-LAGcomcom/frp-panel}"
INSTALL_DIR="${FRP_PANEL_INSTALL_DIR:-/opt/frp-panel}"
DATA_DIR="${FRP_PANEL_DATA_DIR:-/var/lib/frp-panel}"
CONFIG_DIR="${FRP_PANEL_CONFIG_DIR:-/etc/frp-panel}"
SERVICE_NAME="frp-panel"
MODE=""
ASSUME_YES="0"

usage() {
  echo "Usage: install.sh [--install|--update|--uninstall] [--yes]"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --install|install) MODE="install" ;;
    --update|update) MODE="update" ;;
    --uninstall|uninstall) MODE="uninstall" ;;
    --yes|-y) ASSUME_YES="1" ;;
    --help|-h) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
  shift
done

show_menu() {
  if [ ! -r /dev/tty ] || [ ! -w /dev/tty ]; then
    MODE="install"
    return
  fi

  if [ -x "$INSTALL_DIR/frp-panel" ]; then
    status="installed"
  else
    status="not installed"
  fi

  printf '\nFRP Panel Manager (%s)\n' "$status" > /dev/tty
  printf '  1) Install\n  2) Update\n  3) Uninstall (keep config and data)\n  0) Exit\n' > /dev/tty
  printf 'Select an operation [0-3]: ' > /dev/tty
  IFS= read -r choice < /dev/tty || choice="0"
  case "$choice" in
    1) MODE="install" ;;
    2) MODE="update" ;;
    3) MODE="uninstall" ;;
    0) echo "Cancelled."; exit 0 ;;
    *) echo "Invalid selection." >&2; exit 2 ;;
  esac
}

[ -n "$MODE" ] || show_menu

if [ "$(id -u)" -ne 0 ]; then
  echo "Please run as root (for example: curl ... | sudo sh)." >&2
  exit 1
fi

case "$(uname -s)" in
  Linux) ;;
  *) echo "Only Linux is supported by this installer." >&2; exit 1 ;;
esac

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  INIT="systemd"
elif command -v rc-service >/dev/null 2>&1; then
  INIT="openrc"
else
  INIT="none"
fi

confirm_uninstall() {
  [ "$ASSUME_YES" = "1" ] && return
  if [ ! -r /dev/tty ] || [ ! -w /dev/tty ]; then
    echo "Uninstall requires an interactive confirmation or --yes." >&2
    exit 1
  fi
  printf 'Remove FRP Panel services and binaries? Config and data will be kept. [y/N]: ' > /dev/tty
  IFS= read -r answer < /dev/tty || answer=""
  case "$answer" in
    y|Y|yes|YES) ;;
    *) echo "Cancelled."; exit 0 ;;
  esac
}

uninstall_panel() {
  confirm_uninstall

  if [ "$INIT" = "systemd" ] || [ -f "/etc/systemd/system/$SERVICE_NAME.service" ]; then
    systemctl stop "$SERVICE_NAME" >/dev/null 2>&1 || true
    systemctl disable "$SERVICE_NAME" >/dev/null 2>&1 || true
    rm -f "/etc/systemd/system/$SERVICE_NAME.service"
    systemctl daemon-reload >/dev/null 2>&1 || true
  fi
  if [ "$INIT" = "openrc" ] || [ -f "/etc/init.d/$SERVICE_NAME" ]; then
    rc-service "$SERVICE_NAME" stop >/dev/null 2>&1 || true
    rc-update del "$SERVICE_NAME" default >/dev/null 2>&1 || true
    rm -f "/etc/init.d/$SERVICE_NAME"
  fi

  rm -f "$INSTALL_DIR/frp-panel" "$INSTALL_DIR"/frp-panel.backup.*
  rmdir "$INSTALL_DIR" >/dev/null 2>&1 || true

  echo "FRP Panel was uninstalled."
  echo "Config kept at: $CONFIG_DIR"
  echo "Data kept at:   $DATA_DIR"
}

if [ "$MODE" = "uninstall" ]; then
  uninstall_panel
  exit 0
fi

if [ "$MODE" = "update" ] && [ ! -x "$INSTALL_DIR/frp-panel" ]; then
  echo "FRP Panel is not installed. Select Install first." >&2
  exit 1
fi

if [ "$INIT" = "none" ]; then
  echo "Neither systemd nor OpenRC was detected." >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

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
  HEARTBEAT_ENABLED="${FRP_PANEL_HEARTBEAT_ENABLED:-}"
  PANEL_DOMAIN="${FRP_PANEL_DOMAIN:-}"
  if [ -z "$HEARTBEAT_ENABLED" ] && [ -r /dev/tty ] && [ -w /dev/tty ]; then
    printf 'Enable signed five-second health reporting to 08642.xyz? [y/N]: ' > /dev/tty
    IFS= read -r answer < /dev/tty || answer=""
    case "$answer" in y|Y|yes|YES) HEARTBEAT_ENABLED="true" ;; *) HEARTBEAT_ENABLED="false" ;; esac
  fi
  case "$HEARTBEAT_ENABLED" in
    1|true|TRUE|yes|YES) HEARTBEAT_ENABLED="true" ;;
    *) HEARTBEAT_ENABLED="false" ;;
  esac
  if [ "$HEARTBEAT_ENABLED" = "true" ]; then
    if [ -z "$PANEL_DOMAIN" ] && [ -r /dev/tty ] && [ -w /dev/tty ]; then
      printf 'Public panel domain (for example panel.example.com): ' > /dev/tty
      IFS= read -r PANEL_DOMAIN < /dev/tty || PANEL_DOMAIN=""
    fi
    PANEL_DOMAIN=$(printf '%s' "$PANEL_DOMAIN" | sed 's#^https\{0,1\}://##; s#/$##')
    if ! printf '%s' "$PANEL_DOMAIN" | grep -Eq '^[A-Za-z0-9.-]+(:[0-9]+)?$'; then
      echo "A valid FRP_PANEL_DOMAIN is required when health reporting is enabled." >&2
      exit 1
    fi
    PANEL_DOMAIN="https://$PANEL_DOMAIN"
  fi
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
update:
  center_url: "https://08642.xyz"
  instance_key: ""
  panel_version: "$VERSION"
  panel_domain: "$PANEL_DOMAIN"
  heartbeat_enabled: $HEARTBEAT_ENABLED
  heartbeat_interval: 5s
  control_public_key: "HU5iQjEL7v24v5aK0K+gHTctvWorW+iLq5bhpXf9lSU="
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
