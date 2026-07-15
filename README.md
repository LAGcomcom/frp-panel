# FRP Panel

FRP Panel 是一个用于管理 FRPS 服务器、用户、套餐、代理和流量的 Go + Vue 面板。仓库包含面板后端、管理端、用户端、远程监控 Agent，以及 Ubuntu、Debian、Alpine 可用的安装更新脚本。

> 当前仓库公开源代码，但尚未附加开源许可证。未经明确许可，不代表授予复制、修改或再分发权利。

## 一键安装

支持 Linux `amd64`、`arm64`，自动识别 systemd 或 Alpine OpenRC：

```sh
curl -fsSL https://github.com/LAGcomcom/frp-panel/releases/latest/download/install.sh | sudo sh
```

安装完成后终端会显示首次管理员账号和随机密码。默认监听 `8080`，配置位于 `/etc/frp-panel/config.yaml`，数据位于 `/var/lib/frp-panel`。

可通过环境变量覆盖首次安装参数：

```sh
curl -fsSL https://github.com/LAGcomcom/frp-panel/releases/latest/download/install.sh | \
  sudo FRP_PANEL_PORT=8080 FRP_PANEL_ADMIN_EMAIL=admin@example.com sh
```

## 一键更新

```sh
curl -fsSL https://github.com/LAGcomcom/frp-panel/releases/latest/download/install.sh | sudo sh -s -- --update
```

更新会验证 Release 的 SHA-256，保留现有配置与数据库，并在服务启动失败时恢复旧二进制。也可以指定版本：

```sh
curl -fsSL https://github.com/LAGcomcom/frp-panel/releases/latest/download/install.sh | \
  sudo FRP_PANEL_VERSION=v1.0.0 sh -s -- --update
```

## 服务管理

systemd：

```sh
sudo systemctl status frp-panel
sudo journalctl -u frp-panel -f
```

Alpine OpenRC：

```sh
sudo rc-service frp-panel status
sudo tail -f /var/log/frp-panel.log
```

## 从源码构建

需要 Go 1.25、Node.js 22、npm 和 GNU Make：

```sh
make build-linux
```

构建会先生成管理端和用户端静态资源，再把许可证页及 Linux Agent 嵌入面板二进制。配置示例见 `config.example.yaml`。

## Docker

```sh
cp deploy/config.yaml.example deploy/config.yaml
docker compose -f deploy/docker-compose.yml up -d --build
```

生产使用前必须修改管理员密码、JWT 密钥和 FRPS Token。不要提交 `config.yaml`、数据库、密钥、日志或备份文件。

## 发布

推送 `v*` 标签后，GitHub Actions 会构建 Linux `amd64/arm64` 资源，生成 `checksums.txt`，并创建 GitHub Release。一键安装脚本只使用已发布 Release，不从 `main` 分支下载未经校验的二进制。
