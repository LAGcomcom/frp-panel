#!/bin/sh
set -eu

INSTALL_DIR="${FRP_PANEL_INSTALL_DIR:-/opt/frp-panel}"
DATA_DIR="${FRP_PANEL_DATA_DIR:-/var/lib/frp-panel}"
CONFIG_DIR="${FRP_PANEL_CONFIG_DIR:-/etc/frp-panel}"
SYSTEMD_UNIT="${FRP_PANEL_SYSTEMD_UNIT:-/etc/systemd/system/frp-panel.service}"
OPENRC_SCRIPT="${FRP_PANEL_OPENRC_SCRIPT:-/etc/init.d/frp-panel}"
SERVICE_NAME="frp-panel"
MODE=""
ASSUME_YES="0"
PANEL_LANG="${FRP_PANEL_LANG:-zh}"

case "$0" in */*) SCRIPT_PARENT=${0%/*} ;; *) SCRIPT_PARENT="." ;; esac
SCRIPT_DIR=$(CDPATH= cd -- "$SCRIPT_PARENT" && pwd)

normalize_language() {
  case "$PANEL_LANG" in
    en|en_US|en-US) PANEL_LANG="en" ;;
    *) PANEL_LANG="zh" ;;
  esac
}

text() {
  key=$1
  case "$PANEL_LANG:$key" in
    zh:usage) echo "用法：install-offline.sh [--install|--update|--configure|--uninstall] [--yes] [--lang zh|en]" ;;
    zh:title) echo "FRP 面板部署管理" ;;
    zh:installed) echo "已安装" ;;
    zh:not_installed) echo "未安装" ;;
    zh:menu_install) echo "安装" ;;
    zh:menu_update) echo "更新" ;;
    zh:menu_configure) echo "修改配置" ;;
    zh:menu_uninstall) echo "卸载（保留配置和数据）" ;;
    zh:menu_language) echo "切换语言 / Language" ;;
    zh:menu_exit) echo "退出" ;;
    zh:select) echo "请选择操作 [0-5]：" ;;
    zh:invalid) echo "选择无效，请输入 0 到 5。" ;;
    zh:cancelled) echo "操作已取消。" ;;
    zh:root) echo "请使用 root 运行，例如：sudo sh install-offline.sh" ;;
    zh:linux) echo "该脚本仅支持 Linux。" ;;
    zh:init) echo "未检测到 systemd 或 OpenRC，无法创建系统服务。" ;;
    zh:arch) echo "不支持的处理器架构：" ;;
    zh:asset) echo "离线包缺少当前架构的面板文件：" ;;
    zh:checksums) echo "离线包缺少 SHA256SUMS.txt。" ;;
    zh:checksum_entry) echo "校验文件中缺少当前架构的记录：" ;;
    zh:hash_tool) echo "需要 sha256sum、shasum 或 openssl 才能校验文件。" ;;
    zh:hash_failed) echo "面板文件 SHA-256 校验失败，已停止操作。" ;;
    zh:update_missing) echo "尚未安装面板，不能执行更新。" ;;
    zh:config_missing) echo "尚未安装面板或配置文件不存在，不能修改配置。" ;;
    zh:port_prompt) echo "请输入面板监听端口 [8080]：" ;;
    zh:domain_prompt) echo "请输入访问域名（可留空，自动使用服务器 IP）：" ;;
    zh:email_prompt) echo "请输入管理员邮箱 [admin@example.com]：" ;;
    zh:frp_version_prompt) echo "默认 FRP 版本：" ;;
    zh:mirror_prompt) echo "GitHub 镜像地址（输入 - 清空）：" ;;
    zh:poller_prompt) echo "节点轮询间隔秒数：" ;;
    zh:webhook_prompt) echo "插件回调地址（输入 - 清空）：" ;;
    zh:port_invalid) echo "端口必须是 1 到 65535 之间的数字。" ;;
    zh:port_in_use) echo "所选端口已被其他程序占用，请更换端口后重试：" ;;
    zh:low_port_openrc) echo "OpenRC 使用 1 到 1023 端口需要 setcap；Alpine 可先安装 libcap，或者改用 1024 以上端口。" ;;
    zh:capability_failed) echo "无法授予面板绑定低端口的权限，请改用 1024 以上端口。" ;;
    zh:domain_invalid) echo "域名格式无效，请输入 example.com 或带 http/https 的地址。" ;;
    zh:email_invalid) echo "管理员邮箱格式无效。" ;;
    zh:frp_version_invalid) echo "FRP 版本格式无效，例如 0.68.0。" ;;
    zh:url_invalid) echo "URL 必须为空或以 http://、https:// 开头。" ;;
    zh:poller_invalid) echo "轮询间隔必须是 5 到 3600 秒。" ;;
    zh:password_invalid) echo "管理员密码只能包含字母、数字和 ._@%+=:-，且不能为空。" ;;
    zh:uninstall_prompt) echo "确定卸载面板程序和服务吗？配置与数据会保留。[y/N]：" ;;
    zh:confirm_needed) echo "非交互卸载必须增加 --yes。" ;;
    zh:service_failed) echo "服务启动失败，已经恢复原二进制和配置。" ;;
    zh:diagnostic_saved) echo "详细启动日志已保存到：" ;;
    zh:stop_failed) echo "无法确认旧服务已经停止，未修改任何程序或数据。" ;;
    zh:transaction_failed) echo "操作失败，已经恢复原程序、配置、数据库和服务。" ;;
    zh:done) echo "操作完成。" ;;
    zh:config_done) echo "配置已更新并重新加载。" ;;
    zh:config_failed) echo "配置修改失败，已经恢复原配置。" ;;
    zh:config) echo "配置文件：" ;;
    zh:data) echo "数据目录：" ;;
    zh:url) echo "访问地址：" ;;
    zh:admin) echo "管理员账号：" ;;
    zh:password) echo "管理员密码：" ;;
    zh:password_notice) echo "请立即保存该密码；仅首次安装时显示。" ;;
    zh:kept) echo "配置和数据未删除。" ;;
    en:usage) echo "Usage: install-offline.sh [--install|--update|--configure|--uninstall] [--yes] [--lang zh|en]" ;;
    en:title) echo "FRP Panel Offline Deployment Center" ;;
    en:installed) echo "installed" ;;
    en:not_installed) echo "not installed" ;;
    en:menu_install) echo "Install" ;;
    en:menu_update) echo "Update" ;;
    en:menu_configure) echo "Configure" ;;
    en:menu_uninstall) echo "Uninstall (keep config and data)" ;;
    en:menu_language) echo "Language / 切换语言" ;;
    en:menu_exit) echo "Exit" ;;
    en:select) echo "Select an operation [0-5]:" ;;
    en:invalid) echo "Invalid selection. Enter 0 through 5." ;;
    en:cancelled) echo "Cancelled." ;;
    en:root) echo "Run as root, for example: sudo sh install-offline.sh" ;;
    en:linux) echo "This script supports Linux only." ;;
    en:init) echo "Neither systemd nor OpenRC was detected." ;;
    en:arch) echo "Unsupported architecture:" ;;
    en:asset) echo "The package does not contain a panel binary for this architecture:" ;;
    en:checksums) echo "SHA256SUMS.txt is missing from the package." ;;
    en:checksum_entry) echo "No checksum entry exists for:" ;;
    en:hash_tool) echo "sha256sum, shasum, or openssl is required for verification." ;;
    en:hash_failed) echo "Panel binary SHA-256 verification failed." ;;
    en:update_missing) echo "The panel is not installed; update cannot continue." ;;
    en:config_missing) echo "The panel or its configuration is missing." ;;
    en:port_prompt) echo "Panel listen port [8080]:" ;;
    en:domain_prompt) echo "Access domain (leave empty to use the server IP):" ;;
    en:email_prompt) echo "Administrator email [admin@example.com]:" ;;
    en:frp_version_prompt) echo "Default FRP version:" ;;
    en:mirror_prompt) echo "GitHub mirror URL (enter - to clear):" ;;
    en:poller_prompt) echo "Node polling interval in seconds:" ;;
    en:webhook_prompt) echo "Plugin callback URL (enter - to clear):" ;;
    en:port_invalid) echo "Port must be a number from 1 to 65535." ;;
    en:port_in_use) echo "The selected port is already in use; choose another port:" ;;
    en:low_port_openrc) echo "OpenRC requires setcap for ports 1 through 1023; install libcap or use a port above 1023." ;;
    en:capability_failed) echo "Could not grant permission to bind a privileged port; use a port above 1023." ;;
    en:domain_invalid) echo "Invalid domain. Enter example.com or an http/https URL." ;;
    en:email_invalid) echo "Invalid administrator email." ;;
    en:frp_version_invalid) echo "Invalid FRP version; for example, use 0.68.0." ;;
    en:url_invalid) echo "The URL must be empty or begin with http:// or https://." ;;
    en:poller_invalid) echo "The polling interval must be 5 through 3600 seconds." ;;
    en:password_invalid) echo "The administrator password must be non-empty and use only letters, digits, or ._@%+=:-." ;;
    en:uninstall_prompt) echo "Remove the panel program and service? Config and data are kept. [y/N]:" ;;
    en:confirm_needed) echo "Non-interactive uninstall requires --yes." ;;
    en:service_failed) echo "Service startup failed; the previous binary and config were restored." ;;
    en:diagnostic_saved) echo "Startup details were saved to:" ;;
    en:stop_failed) echo "The existing service could not be confirmed stopped; no program or data was changed." ;;
    en:transaction_failed) echo "The operation failed; the previous program, config, database, and service were restored." ;;
    en:done) echo "Operation completed." ;;
    en:config_done) echo "Configuration updated and reloaded." ;;
    en:config_failed) echo "Configuration failed; the previous configuration was restored." ;;
    en:config) echo "Config:" ;;
    en:data) echo "Data:" ;;
    en:url) echo "Panel URL:" ;;
    en:admin) echo "Administrator:" ;;
    en:password) echo "Password:" ;;
    en:password_notice) echo "Store this password now; it is only shown on first install." ;;
    en:kept) echo "Configuration and data were not removed." ;;
    *) echo "$key" ;;
  esac
}

normalize_language

while [ "$#" -gt 0 ]; do
  case "$1" in
    --install|install) MODE="install" ;;
    --update|update) MODE="update" ;;
    --configure|configure|config) MODE="configure" ;;
    --uninstall|uninstall) MODE="uninstall" ;;
    --yes|-y) ASSUME_YES="1" ;;
    --lang)
      shift
      [ "$#" -gt 0 ] || { text usage >&2; exit 2; }
      PANEL_LANG=$1
      normalize_language
      ;;
    --help|-h) text usage; exit 0 ;;
    *) text usage >&2; exit 2 ;;
  esac
  shift
done

show_menu() {
  if [ ! -r /dev/tty ] || [ ! -w /dev/tty ]; then
    MODE="install"
    return
  fi
  while :; do
    if [ -x "$INSTALL_DIR/frp-panel" ]; then status=$(text installed); else status=$(text not_installed); fi
    printf '\n%s (%s)\n' "$(text title)" "$status" > /dev/tty
    printf '  1) %s\n  2) %s\n  3) %s\n  4) %s\n  5) %s\n  0) %s\n' \
      "$(text menu_install)" "$(text menu_update)" "$(text menu_configure)" \
      "$(text menu_uninstall)" "$(text menu_language)" "$(text menu_exit)" > /dev/tty
    printf '%s ' "$(text select)" > /dev/tty
    IFS= read -r choice < /dev/tty || choice="0"
    case "$choice" in
      1) MODE="install"; return ;;
      2) MODE="update"; return ;;
      3) MODE="configure"; return ;;
      4) MODE="uninstall"; return ;;
      5) if [ "$PANEL_LANG" = "zh" ]; then PANEL_LANG="en"; else PANEL_LANG="zh"; fi ;;
      0) text cancelled; exit 0 ;;
      *) text invalid >&2 ;;
    esac
  done
}

[ -n "$MODE" ] || show_menu

if [ "$(id -u)" -ne 0 ]; then text root >&2; exit 1; fi
if [ "$(uname -s)" != "Linux" ]; then text linux >&2; exit 1; fi

case "${FRP_PANEL_INIT:-auto}" in
  systemd) INIT="systemd" ;;
  openrc) INIT="openrc" ;;
  auto)
    if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
      INIT="systemd"
    elif command -v rc-service >/dev/null 2>&1; then
      INIT="openrc"
    else
      INIT="none"
    fi
    ;;
  *) text init >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) printf '%s %s\n' "$(text arch)" "$(uname -m)" >&2; exit 1 ;;
esac

ASSET_REL="bin/frp-panel-offline-linux-$ARCH"
ASSET="$SCRIPT_DIR/$ASSET_REL"
CHECKSUMS="$SCRIPT_DIR/SHA256SUMS.txt"

verify_asset() {
  [ -f "$ASSET" ] || { printf '%s %s\n' "$(text asset)" "$ASSET_REL" >&2; exit 1; }
  [ -f "$CHECKSUMS" ] || { text checksums >&2; exit 1; }
  expected=$(awk -v file="$ASSET_REL" '$2 == file || $2 == "*" file { print $1; exit }' "$CHECKSUMS")
  [ -n "$expected" ] || { printf '%s %s\n' "$(text checksum_entry)" "$ASSET_REL" >&2; exit 1; }
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$ASSET" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$ASSET" | awk '{print $1}')
  elif command -v openssl >/dev/null 2>&1; then
    actual=$(openssl dgst -sha256 "$ASSET" | awk '{print $NF}')
  else
    text hash_tool >&2
    exit 1
  fi
  [ "$actual" = "$expected" ] || { text hash_failed >&2; exit 1; }
}

confirm_uninstall() {
  [ "$ASSUME_YES" = "1" ] && return
  if [ ! -r /dev/tty ] || [ ! -w /dev/tty ]; then text confirm_needed >&2; exit 1; fi
  printf '%s ' "$(text uninstall_prompt)" > /dev/tty
  IFS= read -r answer < /dev/tty || answer=""
  case "$answer" in y|Y|yes|YES|是) ;; *) text cancelled; exit 0 ;; esac
}

stop_service() {
  if [ "$INIT" = "systemd" ]; then systemctl stop "$SERVICE_NAME" >/dev/null 2>&1 || true; fi
  if [ "$INIT" = "openrc" ]; then rc-service "$SERVICE_NAME" stop >/dev/null 2>&1 || true; fi
}

stop_service_required() {
  if [ "$INIT" = "systemd" ]; then
    systemctl stop "$SERVICE_NAME" || return 1
  else
    rc-service "$SERVICE_NAME" stop || return 1
  fi
  [ "${FRP_PANEL_SKIP_STOP_CHECK:-0}" = "1" ] && return 0
  attempts=0
  while [ "$attempts" -lt 5 ]; do
    if [ "$INIT" = "systemd" ]; then
      systemctl is-active --quiet "$SERVICE_NAME" || return 0
    else
      rc-service "$SERVICE_NAME" status >/dev/null 2>&1 || return 0
    fi
    sleep 1
    attempts=$((attempts + 1))
  done
  return 1
}

uninstall_panel() {
  confirm_uninstall
  stop_service
  if { [ "$INIT" = "systemd" ] || [ -f "$SYSTEMD_UNIT" ]; } && command -v systemctl >/dev/null 2>&1; then
    systemctl disable "$SERVICE_NAME" >/dev/null 2>&1 || true
    rm -f "$SYSTEMD_UNIT"
    systemctl daemon-reload >/dev/null 2>&1 || true
  fi
  if { [ "$INIT" = "openrc" ] || [ -f "$OPENRC_SCRIPT" ]; } && command -v rc-update >/dev/null 2>&1; then
    rc-update del "$SERVICE_NAME" default >/dev/null 2>&1 || true
    rm -f "$OPENRC_SCRIPT"
  fi
  rm -f "$INSTALL_DIR/frp-panel" "$INSTALL_DIR"/frp-panel.backup.*
  rmdir "$INSTALL_DIR" >/dev/null 2>&1 || true
  text done
  text kept
  printf '%s %s\n%s %s\n' "$(text config)" "$CONFIG_DIR/config.yaml" "$(text data)" "$DATA_DIR"
}

if [ "$MODE" = "uninstall" ]; then uninstall_panel; exit 0; fi
if [ "$MODE" = "update" ] && [ ! -x "$INSTALL_DIR/frp-panel" ]; then text update_missing >&2; exit 1; fi
if [ "$MODE" = "configure" ] && { [ ! -x "$INSTALL_DIR/frp-panel" ] || [ ! -f "$CONFIG_DIR/config.yaml" ]; }; then text config_missing >&2; exit 1; fi
if [ "$INIT" = "none" ]; then text init >&2; exit 1; fi

if [ "$MODE" != "configure" ]; then verify_asset; fi

random_hex() {
  bytes=$1
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$bytes"
  else
    od -An -N "$bytes" -tx1 /dev/urandom | tr -d ' \n'
  fi
}

prompt_value() {
  prompt=$1
  default=$2
  env_value=$3
  if [ -n "$env_value" ]; then printf '%s' "$env_value"; return; fi
  if [ "$ASSUME_YES" = "1" ]; then printf '%s' "$default"; return; fi
  if [ -r /dev/tty ] && [ -w /dev/tty ]; then
    printf '%s ' "$prompt" > /dev/tty
    IFS= read -r value < /dev/tty || value=""
    [ -n "$value" ] || value=$default
    printf '%s' "$value"
  else
    printf '%s' "$default"
  fi
}

detect_ip() {
  ip=$(hostname -I 2>/dev/null | tr ' ' '\n' | awk '/^[0-9]+\./ { print; exit }')
  [ -n "$ip" ] || ip="127.0.0.1"
  printf '%s' "$ip"
}

prepare_access_url() {
  domain=$1
  port=$2
  explicit="0"
  scheme=""
  case "$domain" in
    https://*) scheme="https"; explicit="1"; domain=${domain#https://} ;;
    http://*) scheme="http"; explicit="1"; domain=${domain#http://} ;;
  esac
  domain=$(printf '%s' "$domain" | sed 's#/$##')
  if [ -n "$domain" ] && ! printf '%s' "$domain" | grep -Eq '^[A-Za-z0-9.-]+(:[0-9]+)?$'; then
    text domain_invalid >&2
    exit 1
  fi
  if [ -z "$domain" ]; then domain=$(detect_ip); scheme="http"; fi
  if [ -z "$scheme" ]; then
    if [ "$port" = "443" ]; then scheme="https"; else scheme="http"; fi
  fi
  case "$domain" in
    *:*) ACCESS_URL="$scheme://$domain" ;;
    *)
      if [ "$explicit" = "1" ] || { [ "$scheme" = "http" ] && [ "$port" = "80" ]; } || { [ "$scheme" = "https" ] && [ "$port" = "443" ]; }; then
        ACCESS_URL="$scheme://$domain"
      else
        ACCESS_URL="$scheme://$domain:$port"
      fi
      ;;
  esac
}

validate_port() {
  port=$1
  case "$port" in ''|*[!0-9]*) text port_invalid >&2; exit 1 ;; esac
  if [ "$port" -lt 1 ] || [ "$port" -gt 65535 ]; then text port_invalid >&2; exit 1; fi
  if [ "$INIT" = "openrc" ] && [ "$port" -lt 1024 ] && ! command -v setcap >/dev/null 2>&1; then
    text low_port_openrc >&2
    exit 1
  fi
}

port_is_listening() {
  port=$1
  if command -v ss >/dev/null 2>&1; then
    ss -ltn 2>/dev/null | awk -v suffix=":$port" '$4 ~ suffix "$" { found=1 } END { exit !found }'
    return
  fi
  if command -v netstat >/dev/null 2>&1; then
    netstat -ltn 2>/dev/null | awk -v suffix=":$port" '$4 ~ suffix "$" { found=1 } END { exit !found }'
    return
  fi
  return 1
}

require_available_port() {
  port=$1
  if port_is_listening "$port"; then
    printf '%s %s\n' "$(text port_in_use)" "$port" >&2
    exit 1
  fi
}

prepare_binary_port_access() {
  port=$1
  if [ "$INIT" = "openrc" ] && [ "$port" -lt 1024 ]; then
    setcap cap_net_bind_service=+ep "$INSTALL_DIR/frp-panel" || return 1
  fi
}

create_user_and_dirs() {
  if ! id frp-panel >/dev/null 2>&1; then
    if command -v useradd >/dev/null 2>&1; then
      useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin frp-panel
    else
      addgroup -S frp-panel >/dev/null 2>&1 || true
      adduser -S -D -H -h "$DATA_DIR" -s /sbin/nologin -G frp-panel frp-panel
    fi
  fi
  mkdir -p "$INSTALL_DIR" "$DATA_DIR" "$CONFIG_DIR"
  chmod 0755 "$INSTALL_DIR"
  chmod 0750 "$DATA_DIR" "$CONFIG_DIR"
  chown frp-panel:frp-panel "$DATA_DIR"
  chown root:frp-panel "$CONFIG_DIR"
}

write_config_if_missing() {
  CONFIG="$CONFIG_DIR/config.yaml"
  CREATED_PASSWORD=""
  if [ -f "$CONFIG" ]; then return; fi

  PORT=$(prompt_value "$(text port_prompt)" "8080" "${FRP_PANEL_PORT:-}")
  validate_port "$PORT"
  if [ "${HAD_BINARY:-0}" = "0" ]; then require_available_port "$PORT"; fi

  PANEL_DOMAIN=$(prompt_value "$(text domain_prompt)" "" "${FRP_PANEL_DOMAIN:-}")
  ADMIN_EMAIL=$(prompt_value "$(text email_prompt)" "admin@example.com" "${FRP_PANEL_ADMIN_EMAIL:-}")
  if ! printf '%s' "$ADMIN_EMAIL" | grep -Eq '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+$'; then text email_invalid >&2; exit 1; fi
  prepare_access_url "$PANEL_DOMAIN" "$PORT"

  JWT_SECRET=$(random_hex 32)
  SERVER_TOKEN=$(random_hex 24)
  CREATED_PASSWORD="${FRP_PANEL_ADMIN_PASSWORD:-$(random_hex 12)}"
  case "$CREATED_PASSWORD" in ''|*[!A-Za-z0-9._@%+=:-]*) text password_invalid >&2; exit 1 ;; esac

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
  center_url: ""
  instance_key: ""
  instance_id: ""
  panel_version: "offline"
  panel_domain: "$PANEL_DOMAIN"
  heartbeat_enabled: false
  heartbeat_interval: 5s
  control_public_key: ""
  identity_key_file: "$DATA_DIR/update-identity.key"
  lease_cache_file: "$DATA_DIR/update-lease.json"
  bootstrap_cache_file: "$DATA_DIR/update-bootstrap.json"
  bootstrap_urls: []
  anonymous_statistics: false
EOF
  chmod 0640 "$CONFIG"
  chown root:frp-panel "$CONFIG"
}

read_access_url() {
  CONFIG="$CONFIG_DIR/config.yaml"
  PORT=$(awk '$1 == "port:" { gsub(/"/, "", $2); print $2; exit }' "$CONFIG")
  [ -n "$PORT" ] || PORT="8080"
  STORED_DOMAIN=$(awk '$1 == "panel_domain:" { sub(/^[^:]*:[[:space:]]*/, ""); gsub(/"/, ""); print; exit }' "$CONFIG")
  prepare_access_url "${FRP_PANEL_DOMAIN:-$STORED_DOMAIN}" "$PORT"
}

write_service() {
  if [ "$INIT" = "systemd" ]; then
    mkdir -p "${SYSTEMD_UNIT%/*}"
    cat > "$SYSTEMD_UNIT" <<EOF
[Unit]
Description=FRP Panel Offline Edition
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=frp-panel
Group=frp-panel
WorkingDirectory=$DATA_DIR
ExecStart=$INSTALL_DIR/frp-panel -config $CONFIG_DIR/config.yaml
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
Restart=on-failure
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME" >/dev/null
  else
    mkdir -p "${OPENRC_SCRIPT%/*}"
    cat > "$OPENRC_SCRIPT" <<EOF
#!/sbin/openrc-run
name="FRP Panel Offline Edition"
command="$INSTALL_DIR/frp-panel"
command_args="-config $CONFIG_DIR/config.yaml"
command_user="frp-panel:frp-panel"
directory="$DATA_DIR"
command_background="yes"
pidfile="/run/frp-panel.pid"
output_log="/var/log/frp-panel.log"
error_log="/var/log/frp-panel.log"
depend() { need net; }
EOF
    chmod 0755 "$OPENRC_SCRIPT"
    rc-update add "$SERVICE_NAME" default >/dev/null 2>&1 || true
  fi
}

start_service() {
  if [ "$INIT" = "systemd" ]; then systemctl restart "$SERVICE_NAME"; else rc-service "$SERVICE_NAME" restart; fi
}

service_healthy() {
  [ "${FRP_PANEL_SKIP_HEALTHCHECK:-0}" = "1" ] && return 0
  attempts=0
  while [ "$attempts" -lt 5 ]; do
    sleep 1
    if [ "$INIT" = "systemd" ]; then
      systemctl is-active --quiet "$SERVICE_NAME" && return 0
    else
      rc-service "$SERVICE_NAME" status >/dev/null 2>&1 && return 0
    fi
    attempts=$((attempts + 1))
  done
  return 1
}

save_service_diagnostics() {
  diagnostic="$DATA_DIR/install-error.log"
  if [ "$INIT" = "systemd" ] && command -v journalctl >/dev/null 2>&1; then
    journalctl -u "$SERVICE_NAME" -n 100 --no-pager > "$diagnostic" 2>&1 || true
  elif [ -f /var/log/frp-panel.log ]; then
    tail -n 100 /var/log/frp-panel.log > "$diagnostic" 2>&1 || true
  else
    printf '%s\n' "No service log was available." > "$diagnostic"
  fi
  chmod 0640 "$diagnostic" >/dev/null 2>&1 || true
  printf '%s %s\n' "$(text diagnostic_saved)" "$diagnostic" >&2
}

yaml_value() {
  section=$1
  key=$2
  awk -v section="$section" -v key="$key" '
    /^[^[:space:]#][^:]*:/ { current=$1; sub(/:$/, "", current) }
    current == section && $1 == key ":" {
      sub(/^[^:]*:[[:space:]]*/, "")
      gsub(/^"|"$/, "")
      print
      exit
    }
  ' "$CONFIG"
}

set_yaml_value() {
  section=$1
  key=$2
  replacement=$3
  output="$ROLLBACK_DIR/config.next"
  awk -v section="$section" -v key="$key" -v replacement="$replacement" '
    BEGIN { found=0 }
    /^[^[:space:]#][^:]*:/ { current=$1; sub(/:$/, "", current) }
    current == section && $1 == key ":" {
      print "  " key ": " replacement
      found=1
      next
    }
    { print }
    END { if (!found) exit 42 }
  ' "$CONFIG" > "$output" || { rm -f "$output"; return 1; }
  mv "$output" "$CONFIG"
}

valid_optional_url() {
  value=$1
  [ -z "$value" ] && return 0
  case "$value" in http://*|https://*) ;; *) return 1 ;; esac
  printf '%s' "$value" | grep -Eq '^https?://[A-Za-z0-9._~:/?#@!$&()*+,;=%-]+$'
}

configure_panel() {
  CONFIG="$CONFIG_DIR/config.yaml"
  ROLLBACK_DIR=$(mktemp -d)
  cleanup() { rm -rf "$ROLLBACK_DIR"; }
  trap cleanup 0 1 2 15

  current_port=$(yaml_value server port)
  current_domain=$(yaml_value update panel_domain)
  current_version=$(yaml_value frp default_version)
  current_mirror=$(yaml_value frp github_mirror)
  current_poller=$(yaml_value frp poller_interval)
  current_webhook=$(yaml_value frp plugin_webhook_url)

  new_port=$(prompt_value "$(text port_prompt) [$current_port]" "$current_port" "${FRP_PANEL_PORT:-}")
  new_domain=$(prompt_value "$(text domain_prompt) [$current_domain]" "$current_domain" "${FRP_PANEL_DOMAIN:-}")
  new_version=$(prompt_value "$(text frp_version_prompt) [$current_version]" "$current_version" "${FRP_PANEL_DEFAULT_FRP_VERSION:-}")
  new_mirror=$(prompt_value "$(text mirror_prompt) [$current_mirror]" "$current_mirror" "${FRP_PANEL_GITHUB_MIRROR:-}")
  new_poller=$(prompt_value "$(text poller_prompt) [$current_poller]" "$current_poller" "${FRP_PANEL_POLLER_INTERVAL:-}")
  new_webhook=$(prompt_value "$(text webhook_prompt) [$current_webhook]" "$current_webhook" "${FRP_PANEL_PLUGIN_WEBHOOK_URL:-}")

  [ "$new_domain" = "-" ] && new_domain=""
  [ "$new_mirror" = "-" ] && new_mirror=""
  [ "$new_webhook" = "-" ] && new_webhook=""

  validate_port "$new_port"
  if [ "$new_port" != "$current_port" ]; then require_available_port "$new_port"; fi
  if [ -n "$new_domain" ]; then prepare_access_url "$new_domain" "$new_port"; else ACCESS_URL="http://$(detect_ip):$new_port"; fi
  if ! printf '%s' "$new_version" | grep -Eq '^[0-9]+(\.[0-9]+){1,3}([._-][A-Za-z0-9.-]+)?$'; then text frp_version_invalid >&2; exit 1; fi
  valid_optional_url "$new_mirror" || { text url_invalid >&2; exit 1; }
  valid_optional_url "$new_webhook" || { text url_invalid >&2; exit 1; }
  case "$new_poller" in ''|*[!0-9]*) text poller_invalid >&2; exit 1 ;; esac
  if [ "$new_poller" -lt 5 ] || [ "$new_poller" -gt 3600 ]; then text poller_invalid >&2; exit 1; fi

  backup="$CONFIG.backup.$(date +%Y%m%d%H%M%S)"
  cp -p "$CONFIG" "$backup"
  if ! stop_service_required; then
    text stop_failed >&2
    exit 1
  fi
  if ! set_yaml_value server port "$new_port" || \
     ! set_yaml_value update panel_domain "\"$new_domain\"" || \
     ! set_yaml_value frp default_version "\"$new_version\"" || \
     ! set_yaml_value frp github_mirror "\"$new_mirror\"" || \
     ! set_yaml_value frp poller_interval "$new_poller" || \
     ! set_yaml_value frp plugin_webhook_url "\"$new_webhook\""; then
    cp -p "$backup" "$CONFIG"
    start_service >/dev/null 2>&1 || true
    text config_failed >&2
    exit 1
  fi
  chmod 0640 "$CONFIG"
  chown root:frp-panel "$CONFIG"
  if ! prepare_binary_port_access "$new_port"; then
    cp -p "$backup" "$CONFIG"
    start_service >/dev/null 2>&1 || true
    text capability_failed >&2
    exit 1
  fi
  if ! start_service || ! service_healthy; then
    save_service_diagnostics
    stop_service
    cp -p "$backup" "$CONFIG"
    start_service >/dev/null 2>&1 || true
    text config_failed >&2
    exit 1
  fi
  text config_done
  printf '%s %s\n%s %s\n' "$(text config)" "$CONFIG" "$(text url)" "$ACCESS_URL"
}

restore_failed_install() {
  stop_service

  if [ "$HAD_BINARY" = "1" ]; then cp -p "$BINARY_BACKUP" "$INSTALL_DIR/frp-panel"; else rm -f "$INSTALL_DIR/frp-panel"; fi
  if [ "$HAD_CONFIG" = "1" ]; then cp -p "$CONFIG_BACKUP" "$CONFIG"; else rm -f "$CONFIG"; fi
  if [ "$HAD_DB" = "1" ]; then
    cp -p "$DB_BACKUP" "$DATA_DIR/frp-panel.db"
  else
    rm -f "$DATA_DIR/frp-panel.db" "$DATA_DIR/frp-panel.db-wal" "$DATA_DIR/frp-panel.db-shm"
  fi

  if [ "$HAD_SYSTEMD_UNIT" = "1" ]; then
    cp -p "$ROLLBACK_DIR/systemd.service" "$SYSTEMD_UNIT"
  else
    rm -f "$SYSTEMD_UNIT"
    command -v systemctl >/dev/null 2>&1 && systemctl disable "$SERVICE_NAME" >/dev/null 2>&1 || true
  fi
  if [ "$HAD_OPENRC_SCRIPT" = "1" ]; then
    cp -p "$ROLLBACK_DIR/openrc.service" "$OPENRC_SCRIPT"
  else
    rm -f "$OPENRC_SCRIPT"
    command -v rc-update >/dev/null 2>&1 && rc-update del "$SERVICE_NAME" default >/dev/null 2>&1 || true
  fi
  command -v systemctl >/dev/null 2>&1 && systemctl daemon-reload >/dev/null 2>&1 || true

  if [ "$HAD_BINARY" = "1" ]; then start_service >/dev/null 2>&1 || true; fi
}

if [ "$MODE" = "configure" ]; then configure_panel; exit 0; fi

create_user_and_dirs
CONFIG="$CONFIG_DIR/config.yaml"
TIMESTAMP=$(date +%Y%m%d%H%M%S)
ROLLBACK_DIR=$(mktemp -d)
cleanup() { rm -rf "$ROLLBACK_DIR"; }
trap cleanup 0 1 2 15

HAD_BINARY="0"
HAD_CONFIG="0"
HAD_DB="0"
HAD_SYSTEMD_UNIT="0"
HAD_OPENRC_SCRIPT="0"
CONFIG_BACKUP=""
BINARY_BACKUP=""
DB_BACKUP=""

if [ -f "$CONFIG" ]; then
  HAD_CONFIG="1"
  CONFIG_BACKUP="$CONFIG.backup.$TIMESTAMP"
  cp -p "$CONFIG" "$CONFIG_BACKUP"
fi
if [ -f "$INSTALL_DIR/frp-panel" ]; then
  HAD_BINARY="1"
  BINARY_BACKUP="$INSTALL_DIR/frp-panel.backup.$TIMESTAMP"
  cp -p "$INSTALL_DIR/frp-panel" "$BINARY_BACKUP"
fi
if [ -f "$DATA_DIR/frp-panel.db" ]; then
  HAD_DB="1"
fi
if [ -f "$SYSTEMD_UNIT" ]; then
  HAD_SYSTEMD_UNIT="1"
  cp -p "$SYSTEMD_UNIT" "$ROLLBACK_DIR/systemd.service"
fi
if [ -f "$OPENRC_SCRIPT" ]; then
  HAD_OPENRC_SCRIPT="1"
  cp -p "$OPENRC_SCRIPT" "$ROLLBACK_DIR/openrc.service"
fi

if [ "$HAD_BINARY" = "1" ]; then
  if ! stop_service_required; then
    text stop_failed >&2
    exit 1
  fi
else
  stop_service
fi
if [ "$HAD_DB" = "1" ]; then
  DB_BACKUP="$DATA_DIR/frp-panel.db.backup.$TIMESTAMP"
  cp -p "$DATA_DIR/frp-panel.db" "$DB_BACKUP"
fi

write_config_if_missing
[ -n "${ACCESS_URL:-}" ] || read_access_url
install -m 0755 "$ASSET" "$INSTALL_DIR/frp-panel.new"
mv -f "$INSTALL_DIR/frp-panel.new" "$INSTALL_DIR/frp-panel"
if ! prepare_binary_port_access "$PORT"; then
  restore_failed_install
  text capability_failed >&2
  exit 1
fi
write_service

if ! start_service || ! service_healthy; then
  save_service_diagnostics
  restore_failed_install
  text service_failed >&2
  exit 1
fi

text done
printf '%s %s\n%s %s\n%s %s\n' \
  "$(text config)" "$CONFIG" "$(text data)" "$DATA_DIR" "$(text url)" "$ACCESS_URL"
if [ -n "${CREATED_PASSWORD:-}" ]; then
  printf '%s %s\n%s %s\n' "$(text admin)" "$ADMIN_EMAIL" "$(text password)" "$CREATED_PASSWORD"
  text password_notice
fi
