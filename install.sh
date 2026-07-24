#!/bin/sh
set -eu

REPO="${FRP_PANEL_REPO:-LAGcomcom/frp-panel}"
INSTALL_DIR="${FRP_PANEL_INSTALL_DIR:-/opt/frp-panel}"
DATA_DIR="${FRP_PANEL_DATA_DIR:-/var/lib/frp-panel}"
CONFIG_DIR="${FRP_PANEL_CONFIG_DIR:-/etc/frp-panel}"
SERVICE_NAME="frp-panel"
MODE=""
ASSUME_YES="0"
PANEL_LANG="${FRP_PANEL_LANG:-zh}"

normalize_language() {
  case "$PANEL_LANG" in
    en|en_US|en-US) PANEL_LANG="en" ;;
    *) PANEL_LANG="zh" ;;
  esac
}

text() {
  key=$1
  case "$PANEL_LANG:$key" in
    zh:usage) echo "用法：install.sh [--install|--update|--uninstall] [--yes]" ;;
    zh:installed) echo "已安装" ;;
    zh:not_installed) echo "未安装" ;;
    zh:menu_title) echo "FRP 面板指令中心" ;;
    zh:menu_install) echo "安装" ;;
    zh:menu_update) echo "更新" ;;
    zh:menu_uninstall) echo "卸载（保留配置和数据）" ;;
    zh:menu_language) echo "切换语言 / Language" ;;
    zh:menu_exit) echo "退出" ;;
    zh:select_operation) echo "请选择操作 [0-4]：" ;;
    zh:cancelled) echo "操作已取消。" ;;
    zh:invalid_selection) echo "选择无效，请输入 0 到 4。" ;;
    zh:root_required) echo "请使用 root 运行，例如：curl ... | sudo sh" ;;
    zh:linux_only) echo "该安装程序仅支持 Linux。" ;;
    zh:uninstall_confirm_required) echo "卸载需要交互确认，或使用 --yes。" ;;
    zh:uninstall_prompt) echo "确定移除 FRP 面板服务和程序吗？配置与数据会保留。[y/N]：" ;;
    zh:uninstalled) echo "FRP 面板已卸载。" ;;
    zh:config_kept) echo "配置保留位置：" ;;
    zh:data_kept) echo "数据保留位置：" ;;
    zh:update_not_installed) echo "尚未安装 FRP 面板，请先选择安装。" ;;
    zh:init_missing) echo "未检测到 systemd 或 OpenRC，无法管理服务。" ;;
    zh:unsupported_arch) echo "不支持的处理器架构：" ;;
    zh:download_tool_missing) echo "需要安装 curl 或 wget 才能下载文件。" ;;
    zh:checksum_missing) echo "发布包缺少以下文件的校验值：" ;;
    zh:hash_tool_missing) echo "需要安装 sha256sum 或 shasum 才能校验文件。" ;;
    zh:hash_failed) echo "SHA-256 校验失败，已停止安装。" ;;
    zh:version_empty) echo "发布版本号为空，已停止安装。" ;;
    zh:version_unsafe) echo "发布版本号包含不安全字符。" ;;
    zh:port_prompt) echo "请输入面板监听端口 [8080]：" ;;
    zh:domain_prompt) echo "请输入访问域名（可留空，自动使用服务器 IP）：" ;;
    zh:invalid_port) echo "端口必须是 1 到 65535 之间的数字。" ;;
    zh:invalid_domain) echo "域名格式无效，请输入 example.com 或带协议的完整域名。" ;;
    zh:service_restore) echo "服务启动失败，已恢复之前的程序。" ;;
    zh:completed) echo "操作完成。" ;;
    zh:config_label) echo "配置文件：" ;;
    zh:data_label) echo "数据目录：" ;;
    zh:admin_label) echo "管理员账号：" ;;
    zh:password_label) echo "管理员密码：" ;;
    zh:access_label) echo "面板访问地址：" ;;
    zh:store_password) echo "请立即保存该密码；它只会在首次安装时显示。" ;;
    zh:mode_install) echo "安装" ;;
    zh:mode_update) echo "更新" ;;
    zh:mode_uninstall) echo "卸载" ;;
    en:usage) echo "Usage: install.sh [--install|--update|--uninstall] [--yes]" ;;
    en:installed) echo "installed" ;;
    en:not_installed) echo "not installed" ;;
    en:menu_title) echo "FRP Panel Command Center" ;;
    en:menu_install) echo "Install" ;;
    en:menu_update) echo "Update" ;;
    en:menu_uninstall) echo "Uninstall (keep config and data)" ;;
    en:menu_language) echo "Language / 切换语言" ;;
    en:menu_exit) echo "Exit" ;;
    en:select_operation) echo "Select an operation [0-4]:" ;;
    en:cancelled) echo "Cancelled." ;;
    en:invalid_selection) echo "Invalid selection. Enter a number from 0 to 4." ;;
    en:root_required) echo "Please run as root (for example: curl ... | sudo sh)." ;;
    en:linux_only) echo "Only Linux is supported by this installer." ;;
    en:uninstall_confirm_required) echo "Uninstall requires an interactive confirmation or --yes." ;;
    en:uninstall_prompt) echo "Remove FRP Panel services and binaries? Config and data will be kept. [y/N]:" ;;
    en:uninstalled) echo "FRP Panel was uninstalled." ;;
    en:config_kept) echo "Config kept at:" ;;
    en:data_kept) echo "Data kept at:" ;;
    en:update_not_installed) echo "FRP Panel is not installed. Select Install first." ;;
    en:init_missing) echo "Neither systemd nor OpenRC was detected." ;;
    en:unsupported_arch) echo "Unsupported architecture:" ;;
    en:download_tool_missing) echo "curl or wget is required." ;;
    en:checksum_missing) echo "Checksum is missing for:" ;;
    en:hash_tool_missing) echo "sha256sum or shasum is required." ;;
    en:hash_failed) echo "SHA-256 verification failed; installation stopped." ;;
    en:version_empty) echo "Release version is empty; installation stopped." ;;
    en:version_unsafe) echo "Release version contains unsafe characters." ;;
    en:port_prompt) echo "Panel listen port [8080]:" ;;
    en:domain_prompt) echo "Access domain (leave empty to use the server IP):" ;;
    en:invalid_port) echo "Port must be a number between 1 and 65535." ;;
    en:invalid_domain) echo "Invalid domain. Enter example.com or a full domain with a scheme." ;;
    en:service_restore) echo "Service failed to start; the previous binary was restored." ;;
    en:completed) echo " completed." ;;
    en:config_label) echo "Config:" ;;
    en:data_label) echo "Data:" ;;
    en:admin_label) echo "Admin:" ;;
    en:password_label) echo "Password:" ;;
    en:access_label) echo "Panel URL:" ;;
    en:store_password) echo "Store this password now; it is only printed during first installation." ;;
    en:mode_install) echo "install" ;;
    en:mode_update) echo "update" ;;
    en:mode_uninstall) echo "uninstall" ;;
    *) echo "$key" ;;
  esac
}

normalize_language

has_controlling_tty() {
  [ -r /dev/tty ] && [ -w /dev/tty ] && ( : < /dev/tty ) 2>/dev/null
}

usage() {
  text usage
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
  if ! has_controlling_tty; then
    MODE="install"
    return
  fi

  while :; do
    if [ -x "$INSTALL_DIR/frp-panel" ]; then
      status=$(text installed)
    else
      status=$(text not_installed)
    fi

    printf '\n%s (%s)\n' "$(text menu_title)" "$status" > /dev/tty
    printf '  1) %s\n  2) %s\n  3) %s\n  4) %s\n  0) %s\n' \
      "$(text menu_install)" "$(text menu_update)" "$(text menu_uninstall)" \
      "$(text menu_language)" "$(text menu_exit)" > /dev/tty
    printf '%s ' "$(text select_operation)" > /dev/tty
    IFS= read -r choice < /dev/tty || choice="0"
    case "$choice" in
      1) MODE="install"; return ;;
      2) MODE="update"; return ;;
      3) MODE="uninstall"; return ;;
      4) [ "$PANEL_LANG" = "zh" ] && PANEL_LANG="en" || PANEL_LANG="zh" ;;
      0) text cancelled; exit 0 ;;
      *) text invalid_selection >&2 ;;
    esac
  done
}

[ -n "$MODE" ] || show_menu

if [ "$(id -u)" -ne 0 ]; then
  text root_required >&2
  exit 1
fi

case "$(uname -s)" in
  Linux) ;;
  *) text linux_only >&2; exit 1 ;;
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
  if ! has_controlling_tty; then
    text uninstall_confirm_required >&2
    exit 1
  fi
  printf '%s ' "$(text uninstall_prompt)" > /dev/tty
  IFS= read -r answer < /dev/tty || answer=""
  case "$answer" in
    y|Y|yes|YES|是) ;;
    *) text cancelled; exit 0 ;;
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

  text uninstalled
  printf '%s %s\n' "$(text config_kept)" "$CONFIG_DIR"
  printf '%s %s\n' "$(text data_kept)" "$DATA_DIR"
}

if [ "$MODE" = "uninstall" ]; then
  uninstall_panel
  exit 0
fi

if [ "$MODE" = "update" ] && [ ! -x "$INSTALL_DIR/frp-panel" ]; then
  text update_not_installed >&2
  exit 1
fi

if [ "$INIT" = "none" ]; then
  text init_missing >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) printf '%s %s\n' "$(text unsupported_arch)" "$(uname -m)" >&2; exit 1 ;;
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
    text download_tool_missing >&2
    exit 1
  fi
}

detect_server_ip() {
  detected=""
  if command -v curl >/dev/null 2>&1; then
    detected=$(curl -fsS --connect-timeout 3 --max-time 5 https://api.ipify.org 2>/dev/null || true)
  elif command -v wget >/dev/null 2>&1; then
    detected=$(wget -qO- -T 5 https://api.ipify.org 2>/dev/null || true)
  fi
  if ! printf '%s' "$detected" | grep -Eq '^[0-9]+(\.[0-9]+){3}$'; then
    detected=$( (hostname -I 2>/dev/null || true) | tr ' ' '\n' | awk '/^[0-9]+\./ { print; exit }')
  fi
  [ -n "$detected" ] || detected="127.0.0.1"
  printf '%s' "$detected"
}

prepare_access_address() {
  raw_domain=$(printf '%s' "${PANEL_DOMAIN:-}" | sed 's#/$##')
  scheme="${FRP_PANEL_SCHEME:-}"
  explicit_scheme="0"
  case "$raw_domain" in
    https://*) scheme="https"; explicit_scheme="1"; raw_domain=${raw_domain#https://} ;;
    http://*) scheme="http"; explicit_scheme="1"; raw_domain=${raw_domain#http://} ;;
  esac
  if [ -n "$raw_domain" ] && ! printf '%s' "$raw_domain" | grep -Eq '^[A-Za-z0-9.-]+(:[0-9]+)?$'; then
    text invalid_domain >&2
    exit 1
  fi
  if [ -z "$raw_domain" ]; then
    raw_domain=$(detect_server_ip)
  fi
  if [ -z "$scheme" ]; then
    if [ "$PORT" = "443" ]; then scheme="https"; else scheme="http"; fi
  fi
  case "$scheme" in http|https) ;; *) text invalid_domain >&2; exit 1 ;; esac

  PANEL_DOMAIN="$scheme://$raw_domain"
  case "$raw_domain" in
    *:*) ACCESS_URL="$PANEL_DOMAIN" ;;
    *)
      if [ "$explicit_scheme" = "1" ] || { [ "$scheme" = "http" ] && [ "$PORT" = "80" ]; } || { [ "$scheme" = "https" ] && [ "$PORT" = "443" ]; }; then
        ACCESS_URL="$PANEL_DOMAIN"
      else
        ACCESS_URL="$PANEL_DOMAIN:$PORT"
      fi
      ;;
  esac
}

if [ -n "${FRP_PANEL_VERSION:-}" ]; then
  RELEASE_BASE="https://github.com/$REPO/releases/download/$FRP_PANEL_VERSION"
else
  RELEASE_BASE="https://github.com/$REPO/releases/latest/download"
fi

ASSET="frp-panel-linux-$ARCH"
SCRIPT_DIR=""
if [ -f "$0" ]; then
  SCRIPT_DIR=$(CDPATH= cd "$(dirname "$0")" 2>/dev/null && pwd || true)
fi
LOCAL_RELEASE_DIR="$SCRIPT_DIR/release"
if [ "$MODE" = "install" ] && [ -z "${FRP_PANEL_VERSION:-}" ] && [ -n "$SCRIPT_DIR" ] && [ -f "$LOCAL_RELEASE_DIR/$ASSET" ] && [ -f "$LOCAL_RELEASE_DIR/checksums.txt" ] && [ -f "$LOCAL_RELEASE_DIR/version.txt" ]; then
  cp "$LOCAL_RELEASE_DIR/$ASSET" "$TMP_DIR/$ASSET"
  cp "$LOCAL_RELEASE_DIR/checksums.txt" "$TMP_DIR/checksums.txt"
  cp "$LOCAL_RELEASE_DIR/version.txt" "$TMP_DIR/version.txt"
else
  download "$RELEASE_BASE/$ASSET" "$TMP_DIR/$ASSET"
  download "$RELEASE_BASE/checksums.txt" "$TMP_DIR/checksums.txt"
  download "$RELEASE_BASE/version.txt" "$TMP_DIR/version.txt"
fi

expected=$(awk -v file="$ASSET" '$2 == file || $2 == "*" file { print $1; exit }' "$TMP_DIR/checksums.txt")
[ -n "$expected" ] || { printf '%s %s\n' "$(text checksum_missing)" "$ASSET" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$TMP_DIR/$ASSET" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$TMP_DIR/$ASSET" | awk '{print $1}')
else
  text hash_tool_missing >&2
  exit 1
fi
[ "$actual" = "$expected" ] || { text hash_failed >&2; exit 1; }

VERSION=$(tr -d '\r\n' < "$TMP_DIR/version.txt")
[ -n "$VERSION" ] || { text version_empty >&2; exit 1; }
case "$VERSION" in
  *[!A-Za-z0-9._-]*) text version_unsafe >&2; exit 1 ;;
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
ACCESS_URL=""
if [ ! -f "$CONFIG" ]; then
  random_hex() { od -An -N "$1" -tx1 /dev/urandom | tr -d ' \n'; }
  JWT_SECRET=$(random_hex 32)
  SERVER_TOKEN=$(random_hex 24)
  CREATED_PASSWORD="${FRP_PANEL_ADMIN_PASSWORD:-$(random_hex 12)}"
  ADMIN_EMAIL="${FRP_PANEL_ADMIN_EMAIL:-admin@example.com}"
  PORT="${FRP_PANEL_PORT:-}"
  if [ -z "$PORT" ] && has_controlling_tty; then
    printf '%s ' "$(text port_prompt)" > /dev/tty
    IFS= read -r PORT < /dev/tty || PORT=""
  fi
  [ -n "$PORT" ] || PORT="8080"
  case "$PORT" in *[!0-9]*|'') text invalid_port >&2; exit 1 ;; esac
  if [ "$PORT" -lt 1 ] || [ "$PORT" -gt 65535 ]; then
    text invalid_port >&2
    exit 1
  fi
  HEARTBEAT_ENABLED="${FRP_PANEL_HEARTBEAT_ENABLED:-true}"
  PANEL_DOMAIN="${FRP_PANEL_DOMAIN:-}"
  if [ -z "$PANEL_DOMAIN" ] && has_controlling_tty; then
    printf '%s ' "$(text domain_prompt)" > /dev/tty
    IFS= read -r PANEL_DOMAIN < /dev/tty || PANEL_DOMAIN=""
  fi
  case "$HEARTBEAT_ENABLED" in
    1|true|TRUE|yes|YES) HEARTBEAT_ENABLED="true" ;;
    *) HEARTBEAT_ENABLED="false" ;;
  esac
  prepare_access_address
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

if [ -z "$ACCESS_URL" ]; then
  PORT=$(awk '$1 == "port:" { gsub(/\"/, "", $2); print $2; exit }' "$CONFIG")
  [ -n "$PORT" ] || PORT="8080"
  PANEL_DOMAIN=$(awk '$1 == "panel_domain:" { sub(/^[^:]*:[[:space:]]*/, ""); gsub(/\"/, ""); print; exit }' "$CONFIG")
  prepare_access_address
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
    text service_restore >&2
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
    text service_restore >&2
    exit 1
  fi
fi

operation=$(text "mode_$MODE")
printf 'FRP Panel %s %s%s\n' "$VERSION" "$operation" "$(text completed)"
printf '%s %s\n' "$(text config_label)" "$CONFIG"
printf '%s %s\n' "$(text data_label)" "$DATA_DIR"
printf '%s %s\n' "$(text access_label)" "$ACCESS_URL"
if [ -n "$CREATED_PASSWORD" ]; then
  printf '%s %s\n' "$(text admin_label)" "$ADMIN_EMAIL"
  printf '%s %s\n' "$(text password_label)" "$CREATED_PASSWORD"
  text store_password
fi
