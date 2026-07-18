# FRP Panel 更新中心离线版：文件手工部署指南

本文档适用于 `frp-panel-offline-v1.2.7.zip`，主流程使用包内已经生成的 Linux amd64/arm64 二进制手工安装和维护更新中心离线版，不使用一键安装脚本。只有需要修改或复现构建时才从源码重新编译。

## 1. 版本边界

这里的“离线版”只表示面板不连接发行方更新中心。使用 `offline` 构建标签后，以下功能会被编译为关闭状态：

- 不连接 `08642.xyz` 或其他配置的面板更新中心；
- 不注册面板实例；
- 不发送 5 秒存活心跳；
- 不请求 bootstrap、租约或在线功能状态；
- 不检查或下载面板新版本；
- 不注册 `/api/admin/update/*` 更新接口；
- 管理端不启动定时更新检查。

以下连接和功能保持可用：

- 从 GitHub 或配置的镜像下载 FRPS；
- 通过 SSH 管理 FRPS 节点；
- FRPS HTTP 插件回调面板进行用户 API Key 鉴权；
- Agent 指标上报、节点轮询、隧道和流量统计；
- 管理员配置的 SMTP、支付接口及其他业务连接。

## 2. 压缩包内容

解压后目录结构如下：

```text
frp-panel-offline-v1.2.7/
├── source/                         完整 v1.2.7 源码
├── bin/
│   ├── frp-panel-offline-linux-amd64
│   └── frp-panel-offline-linux-arm64
├── README-OFFLINE.md
├── PACKAGE-CONTENTS.txt
└── SHA256SUMS.txt
```

本文以 `bin/` 中已经生成并经过 CI 校验的二进制为主要部署文件。`source/` 用于代码审查、二次开发和可选的重新编译。

## 3. 环境要求

### 3.1 支持的目标系统

- Ubuntu 20.04、22.04、24.04；
- Debian 11、12；
- Alpine Linux 3.18 及以上；
- CPU 架构：Linux `amd64` 或 `arm64`；
- 建议至少 1 核 CPU、512 MB 内存、1 GB 可用磁盘；
- 生产环境建议 1 GB 以上内存，并为数据库和日志预留磁盘空间。

SQLite 使用纯 Go 实现，不要求安装系统 SQLite 开发库。当前项目也不要求 Redis 才能启动。

### 3.2 使用包内成品时的运行依赖

压缩包已经包含 CI 生成并校验过的 Linux amd64 和 arm64 二进制。直接手工部署成品时，不需要安装 Go、Node.js 或 npm。

目标服务器只需具备：

- CA 根证书；
- `curl` 或 `wget`，用于健康检查及后续按需下载 FRPS；
- systemd，或 Alpine OpenRC；
- 可选：`openssl`，用于生成随机配置密钥。

Ubuntu / Debian：

```sh
sudo apt-get update
sudo apt-get install -y ca-certificates curl openssl unzip file
```

Alpine（以下命令假设已进入 root shell，不使用 `sudo`）：

```sh
apk add --no-cache ca-certificates curl openssl unzip file
```

### 3.3 重新编译源码时的构建工具

源码构建机需要：

- Go `1.25.2`；
- Node.js `22.x`；
- npm（随 Node.js 安装）；
- Git、CA 根证书、`tar`、`unzip`；
- 可选：`openssl`，用于生成随机密钥。

首次构建时，Go 会访问模块代理，npm 会访问包仓库。离线版仅切断面板更新中心，不限制 Go、npm、GitHub 或 FRPS 下载。如果构建机完全隔离互联网，需要提前准备 Go 模块缓存和 npm 缓存。

检查版本：

```sh
go version
node --version
npm --version
```

Go 版本应显示 `go1.25.2`，Node.js 建议显示 `v22.x`。

### 3.4 Ubuntu / Debian 构建依赖

```sh
sudo apt-get update
sudo apt-get install -y ca-certificates curl git unzip tar openssl file build-essential
```

Node.js 22 和 Go 1.25.2 建议使用各自官方发行包安装，避免系统仓库版本过旧。安装完成后重新执行版本检查。

### 3.5 Alpine 构建依赖

```sh
apk add --no-cache bash ca-certificates curl git unzip tar openssl file build-base nodejs npm
```

Alpine 仓库里的 Go 版本不一定是 1.25.2。如果版本不符合要求，请安装 Go 官方 Linux 压缩包。

## 4. 校验客户压缩包

收到文件后先校验整个 ZIP：

```sh
sha256sum frp-panel-offline-v1.2.7.zip
```

将结果与交付方单独提供的 SHA-256 或 GitHub Release 资产摘要比较。不要使用压缩包内部文档记录的整包摘要，因为把文档加入 ZIP 后，ZIP 摘要本身会变化。

解压并进入包目录：

```sh
unzip frp-panel-offline-v1.2.7.zip
cd frp-panel-offline-v1.2.7
```

先验证包内两个成品二进制：

```sh
sha256sum -c SHA256SUMS.txt
```

## 5. 使用包内已生成二进制

这是推荐的客户部署方式：使用已生成文件手工配置，不运行一键安装脚本，也不需要在服务器上重新编译。

检查服务器架构：

```sh
uname -m
```

选择对应二进制并复制为统一文件名：

```sh
case "$(uname -m)" in
  x86_64|amd64)
    cp bin/frp-panel-offline-linux-amd64 frp-panel-offline
    ;;
  aarch64|arm64)
    cp bin/frp-panel-offline-linux-arm64 frp-panel-offline
    ;;
  *)
    echo "不支持的 CPU 架构：$(uname -m)" >&2
    exit 1
    ;;
esac

chmod 0755 frp-panel-offline
file frp-panel-offline
```

随后直接跳到第 7 节创建运行用户和目录。第 6 节仅供需要修改源码或自行复现构建的客户使用。

## 6. 可选：从源码重新编译离线版

```sh
cd source
```

### 6.1 安装前端依赖并构建

```sh
npm ci --prefix web/admin
npm run build --prefix web/admin

npm ci --prefix web/user
npm run build --prefix web/user
```

两个命令完成后应存在：

```text
web/admin/dist/index.html
web/user/dist/index.html
```

### 6.2 准备 Go 嵌入资源

面板使用 Go `embed` 打包网页和 Agent。必须先准备这些目录，再编译面板：

```sh
mkdir -p internal/api/dist/landing internal/api/dist/admin internal/api/dist/user internal/api/bin

cp web/landing/index.html internal/api/dist/landing/index.html
rm -rf internal/api/dist/admin/* internal/api/dist/user/*
cp -a web/admin/dist/. internal/api/dist/admin/
cp -a web/user/dist/. internal/api/dist/user/
```

### 6.3 编译 Agent

在目标 Linux 服务器本机编译时：

```sh
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
  -o internal/api/bin/agent ./cmd/agent/
```

嵌入的 Agent 架构与构建机架构一致。交叉编译时必须显式指定 `GOOS` 和 `GOARCH`。

### 6.4 编译面板离线版

关键参数是 `-tags offline`。遗漏此参数会生成错误版本，不能作为本离线包交付。

本机编译：

```sh
CGO_ENABLED=0 go build -tags offline -trimpath -ldflags="-s -w" \
  -o frp-panel-offline ./cmd/panel/
```

在 amd64 构建机上交叉编译 Linux arm64：

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
  -trimpath -ldflags="-s -w" \
  -o internal/api/bin/agent ./cmd/agent/

CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
  -tags offline -trimpath -ldflags="-s -w" \
  -o frp-panel-offline-linux-arm64 ./cmd/panel/

file frp-panel-offline-linux-arm64
```

每次切换目标架构，都必须先用相同的 `GOOS`、`GOARCH` 重编 `internal/api/bin/agent`，再编译面板。否则面板会嵌入错误架构的 Agent。

检查文件：

```sh
file frp-panel-offline
ls -lh frp-panel-offline
```

### 6.5 构建前验证

建议在交付前验证离线编译路径：

```sh
go test -tags offline ./...
go vet -tags offline ./...
```

## 7. 创建运行用户和目录

以下命令需要 root 权限。已经使用 root 登录时，请去掉命令前的 `sudo`；Alpine 最小系统通常没有预装 `sudo`。

### 7.1 Ubuntu / Debian

```sh
sudo useradd --system --home /var/lib/frp-panel --shell /usr/sbin/nologin frp-panel 2>/dev/null || true

sudo install -d -m 0755 /opt/frp-panel
sudo install -d -o frp-panel -g frp-panel -m 0750 /var/lib/frp-panel
sudo install -d -o root -g frp-panel -m 0750 /etc/frp-panel
```

### 7.2 Alpine

```sh
sudo addgroup -S frp-panel 2>/dev/null || true
sudo adduser -S -D -H -h /var/lib/frp-panel -s /sbin/nologin -G frp-panel frp-panel 2>/dev/null || true

sudo install -d -m 0755 /opt/frp-panel
sudo install -d -o frp-panel -g frp-panel -m 0750 /var/lib/frp-panel
sudo install -d -o root -g frp-panel -m 0750 /etc/frp-panel
```

安装刚编译的二进制：

```sh
sudo install -m 0755 frp-panel-offline /opt/frp-panel/frp-panel
```

## 8. 创建配置文件

生成随机值：

```sh
JWT_SECRET=$(openssl rand -hex 32)
SERVER_TOKEN=$(openssl rand -hex 24)
ADMIN_PASSWORD=$(openssl rand -hex 12)

printf 'JWT_SECRET=%s\n' "$JWT_SECRET"
printf 'SERVER_TOKEN=%s\n' "$SERVER_TOKEN"
printf 'ADMIN_PASSWORD=%s\n' "$ADMIN_PASSWORD"
```

安全保存管理员密码，不要把这些值提交到 Git 或发送到公开聊天。

创建 `/etc/frp-panel/config.yaml`：

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"

database:
  driver: "sqlite"
  dsn: "/var/lib/frp-panel/frp-panel.db"

redis:
  addr: "localhost:6379"
  password: ""
  db: 0

jwt:
  secret: "替换为 JWT_SECRET"
  expire_time: 24h
  issuer: "frp-panel"

frp:
  default_version: "0.68.0"
  github_mirror: ""
  download_timeout: 300
  plugin_webhook_url: ""
  poller_interval: 30
  server_token: "替换为 SERVER_TOKEN"

admin:
  email: "admin@example.com"
  password: "替换为 ADMIN_PASSWORD"

update:
  center_url: ""
  instance_key: ""
  instance_id: ""
  panel_version: "1.2.7-offline"
  panel_domain: ""
  heartbeat_enabled: false
  heartbeat_interval: 5s
  control_public_key: ""
  identity_key_file: "/var/lib/frp-panel/update-identity.key"
  lease_cache_file: "/var/lib/frp-panel/update-lease.json"
  bootstrap_cache_file: "/var/lib/frp-panel/update-bootstrap.json"
  bootstrap_urls: []
  anonymous_statistics: false
```

即使误填了更新中心地址，`offline` 构建也会在服务启动前清空整段更新配置。但仍建议保持上述空值，便于人工审计。

设置权限：

```sh
sudo chown root:frp-panel /etc/frp-panel/config.yaml
sudo chmod 0640 /etc/frp-panel/config.yaml
```

注意：`admin.password` 只在数据库中不存在超级管理员时用于首次创建账号。数据库已经初始化后，修改配置文件不会重置已有管理员密码。

## 9. systemd 部署（Ubuntu / Debian）

创建 `/etc/systemd/system/frp-panel.service`：

```ini
[Unit]
Description=FRP Panel Offline Edition
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=frp-panel
Group=frp-panel
WorkingDirectory=/var/lib/frp-panel
ExecStart=/opt/frp-panel/frp-panel -config /etc/frp-panel/config.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
```

加载并启动：

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now frp-panel
sudo systemctl status frp-panel --no-pager
```

查看日志：

```sh
sudo journalctl -u frp-panel -n 100 --no-pager
sudo journalctl -u frp-panel -f
```

## 10. OpenRC 部署（Alpine）

创建 `/etc/init.d/frp-panel`：

```sh
#!/sbin/openrc-run

name="FRP Panel Offline Edition"
command="/opt/frp-panel/frp-panel"
command_args="-config /etc/frp-panel/config.yaml"
command_user="frp-panel:frp-panel"
directory="/var/lib/frp-panel"
command_background="yes"
pidfile="/run/frp-panel.pid"
output_log="/var/log/frp-panel.log"
error_log="/var/log/frp-panel.log"

depend() {
  need net
}
```

启用并启动：

```sh
sudo chmod 0755 /etc/init.d/frp-panel
sudo rc-update add frp-panel default
sudo rc-service frp-panel start
sudo rc-service frp-panel status
```

查看日志：

```sh
sudo tail -n 100 /var/log/frp-panel.log
```

## 11. 防火墙和网络

面板默认监听 TCP `8080`。如果直接访问，需要放行该端口：

Ubuntu UFW：

```sh
sudo ufw allow 8080/tcp
```

使用云服务器时，还要在云安全组中放行对应端口。

FRPS 相关端口开在 FRPS 节点上，不一定与面板服务器相同：

- FRPS 客户端连接端口，例如 `7000/tcp`；
- 每个 TCP/UDP 代理的远程端口；
- HTTP/HTTPS 虚拟主机端口；
- FRPS Dashboard 端口建议只允许面板服务器访问。

FRPS 节点必须能访问面板的插件回调地址，否则用户 API Key 鉴权、用户组节点权限和服务端限速无法执行。

## 12. 可选 Nginx 反向代理

生产环境建议让面板只监听本机地址，并由 Nginx 提供 HTTPS。将配置中的 `server.host` 改为 `127.0.0.1`，示例 Nginx 配置：

```nginx
server {
    listen 80;
    server_name panel.example.com;

    client_max_body_size 64m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

检查并重载：

```sh
sudo nginx -t
sudo systemctl reload nginx
```

DNS、HTTPS 证书和 Nginx 不由面板自动配置，需要管理员单独完成。

## 13. 首次启动验证

本机健康检查：

```sh
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/api/settings/public
```

浏览器访问：

```text
http://服务器IP:8080/admin/login
```

使用配置文件中的 `admin.email` 和首次启动时的 `admin.password` 登录。

确认离线构建没有更新路由：

```sh
curl -sS -o /dev/null -w '%{http_code}\n' \
  http://127.0.0.1:8080/api/admin/update/check
```

应返回：

```text
404
```

检查日志中没有更新中心请求：

```sh
sudo journalctl -u frp-panel --since '10 minutes ago' --no-pager \
  | grep -Ei '08642\.xyz|bootstrap|lease/renew|api/v1/heartbeat'
```

正常情况下没有输出。节点轮询、SSH、GitHub FRPS 下载或 Agent 上报属于保留功能，不应被当成更新中心连接。

## 14. 添加 FRPS 节点前的检查

面板通过 SSH 在目标服务器部署 FRPS。目标服务器至少需要：

- Linux amd64 或 arm64；
- 直接以 root 身份登录的 SSH 用户；
- `curl` 或 `wget`、`tar`、CA 根证书；
- 能访问 GitHub Release 或配置的下载镜像；
- 防火墙和安全组已放行所需 FRPS 端口。

离线版不会禁用 GitHub 下载。如果 GitHub 下载异常，可在 `frp.github_mirror` 中填写可用镜像，或预先手工安装 FRPS。

面板生成的 FRPS 自动部署脚本会检查 `id -u`，当前必须为 `0`，不会自动调用 `sudo`。只有普通 sudo 用户时，请先手工安装和配置 FRPS，或者为面板提供受控的 root SSH 登录方式。

## 15. 数据备份、升级与回滚

重要文件：

```text
/opt/frp-panel/frp-panel             面板二进制
/etc/frp-panel/config.yaml           配置文件
/var/lib/frp-panel/frp-panel.db      SQLite 数据库
```

### 15.1 一致性备份

为了保证 SQLite 文件一致，先停止服务：

```sh
sudo systemctl stop frp-panel
sudo cp -a /etc/frp-panel/config.yaml /etc/frp-panel/config.yaml.backup
sudo cp -a /var/lib/frp-panel/frp-panel.db /var/lib/frp-panel/frp-panel.db.backup
sudo systemctl start frp-panel
```

Alpine 将 `systemctl` 替换为 `rc-service frp-panel`。

### 15.2 使用新客户包手工升级

解压新客户包，根据 `uname -m` 从新包的 `bin/` 目录选择 amd64 或 arm64 成品，并复制为 `frp-panel-offline`。选择方法与第 5 节相同。

只有需要修改源码时，才按第 6 节重新构建并得到 `frp-panel-offline`。

准备好新二进制后执行：

```sh
sudo cp -a /opt/frp-panel/frp-panel /opt/frp-panel/frp-panel.backup
sudo install -m 0755 frp-panel-offline /opt/frp-panel/frp-panel.new
sudo mv /opt/frp-panel/frp-panel.new /opt/frp-panel/frp-panel
sudo systemctl restart frp-panel
sudo systemctl status frp-panel --no-pager
```

不要覆盖 `/etc/frp-panel/config.yaml` 或 `/var/lib/frp-panel/frp-panel.db`。

Alpine 请在 root shell 中执行对应命令：

```sh
cp -a /opt/frp-panel/frp-panel /opt/frp-panel/frp-panel.backup
install -m 0755 frp-panel-offline /opt/frp-panel/frp-panel.new
mv /opt/frp-panel/frp-panel.new /opt/frp-panel/frp-panel
rc-service frp-panel restart
rc-service frp-panel status
```

### 15.3 回滚二进制

```sh
sudo cp -a /opt/frp-panel/frp-panel.backup /opt/frp-panel/frp-panel
sudo systemctl restart frp-panel
```

Alpine root shell：

```sh
cp -a /opt/frp-panel/frp-panel.backup /opt/frp-panel/frp-panel
rc-service frp-panel restart
rc-service frp-panel status
```

如果新版执行了数据库迁移且需要回滚数据库，应停止服务后同时恢复与旧二进制同一时间点的数据库备份。

## 16. 常见问题

### 编译时报 `pattern ... no matching files found`

说明 `internal/api/dist` 或 `internal/api/bin/agent` 没准备完整。重新执行第 6.1 到 6.3 节，再编译面板。

### `npm ci` 失败

检查 Node.js 是否为 22.x、系统时间和 CA 证书是否正确，并确认能访问 npm 仓库。不要删除或手工改写 `package-lock.json`。

### Go 模块下载失败

检查 Go 1.25.2、DNS、CA 证书和 `GOPROXY`。可根据所在网络配置可信的 Go 模块镜像。

### 启动时报数据库只读或 `permission denied`

```sh
sudo chown -R frp-panel:frp-panel /var/lib/frp-panel
sudo chmod 0750 /var/lib/frp-panel
```

配置文件应为 `root:frp-panel`、权限 `0640`。

### 启动时报 `exec format error`

二进制架构错误。使用 `uname -m` 检查目标服务器：

- `x86_64` 使用 amd64；
- `aarch64` 使用 arm64。

### 更新接口返回非 404（常见为 401）

只要返回值不是 404（常见错误值为 401），就说明拿错了二进制，或自行编译时遗漏了 `-tags offline`。选择包内正确架构的离线成品，或者重新按第 6 节编译，然后替换二进制并重启。

### FRPS 下载失败

这与更新中心离线模式无关。检查目标节点的 GitHub 网络、镜像配置、CA 证书、DNS、系统架构和 FRP 版本号。

### FRPC 能连接 FRPS，但面板权限或限速没有生效

检查 FRPS 配置中是否启用了面板 HTTP 插件，并确认 FRPS 能访问面板回调地址。用户 API Key 由 FRPS 插件交给面板验证；面板通过后，FRPS 才会接受代理并在服务端应用带宽限制。

## 17. 交付前检查清单

- ZIP 和包内二进制 SHA-256 已核对；
- 使用包内成品时，已根据 `uname -m` 选择正确架构；
- 重新编译源码时，Go 1.25.2、Node.js 22 和 npm 版本正确；
- 重新编译源码时，管理端、用户端、Agent 和 `-tags offline` 面板按顺序构建；
- `go test -tags offline ./...` 和 `go vet -tags offline ./...` 均通过；
- 配置中的 JWT、管理员密码和 FRPS Token 已替换；
- 配置、数据库目录权限正确；
- systemd 或 OpenRC 服务设置为开机启动；
- `/healthz` 和登录页面可访问；
- `/api/admin/update/check` 返回 404；
- 日志中没有更新中心注册、心跳、bootstrap 或租约请求；
- GitHub FRPS 下载和 SSH 节点管理按客户需求保持可用；
- 已完成配置、数据库和旧二进制备份。
