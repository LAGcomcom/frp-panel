# FRP Panel

FRP Panel 是一个用于管理 FRPS 服务器、用户、套餐、代理和流量的 Go + Vue 面板。仓库包含面板后端、管理端、用户端、远程监控 Agent，以及 Ubuntu、Debian、Alpine 可用的安装更新脚本。

> 当前仓库公开源代码，但尚未附加开源许可证。未经明确许可，不代表授予复制、修改或再分发权利。

## 一个命令管理

在 SSH 中只需要执行这一条命令：

```sh
curl -fsSL https://github.com/LAGcomcom/frp-panel/releases/latest/download/install.sh | sudo sh
```

脚本会显示统一菜单：

```text
FRP 面板指令中心（未安装）
  1) 安装
  2) 更新
  3) 卸载（保留配置和数据）
  4) 切换语言 / Language
  0) 退出
```

指令面板默认使用中文，按 `4` 可即时切换 English。无人值守任务可设置 `FRP_PANEL_LANG=en`，不设置时仍默认中文。

支持 Linux `amd64`、`arm64`，自动识别 systemd 或 Alpine OpenRC。首次安装完成后终端会显示管理员账号和随机密码。默认监听 `8080`，配置位于 `/etc/frp-panel/config.yaml`，数据位于 `/var/lib/frp-panel`。

可通过环境变量覆盖首次安装参数：

```sh
curl -fsSL https://github.com/LAGcomcom/frp-panel/releases/latest/download/install.sh | \
  sudo FRP_PANEL_PORT=8080 FRP_PANEL_ADMIN_EMAIL=admin@example.com sh
```

交互安装会询问是否启用签名存活上报。无人值守安装可明确开启，并提供面板域名：

```sh
curl -fsSL https://github.com/LAGcomcom/frp-panel/releases/latest/download/install.sh | \
  sudo FRP_PANEL_HEARTBEAT_ENABLED=true FRP_PANEL_DOMAIN=panel.example.com sh -s -- --install
```

卸载只删除服务和程序文件，配置与数据库默认保留，重新安装后可以继续使用。

## 无人值守

脚本参数仅用于自动化任务，SSH 手动管理不需要记忆这些参数：

```sh
sudo sh install.sh --install
sudo sh install.sh --update
sudo sh install.sh --uninstall --yes
```

更新会验证 Release 的 SHA-256，保留现有配置与数据库，并在服务启动失败时恢复旧二进制。

## 从源码构建

需要 Go 1.25、Node.js 22、npm 和 GNU Make：

```sh
make build-linux
```

构建会先生成管理端和用户端静态资源，再把 Linux Agent 嵌入面板二进制。公开版不包含运行时许可证门。配置示例见 `config.example.yaml`。

## Docker

```sh
cp deploy/config.yaml.example deploy/config.yaml
docker compose -f deploy/docker-compose.yml up -d --build
```

生产使用前必须修改管理员密码、JWT 密钥和 FRPS Token。不要提交 `config.yaml`、数据库、密钥、日志或备份文件。

## 发布

推送 `v*` 标签后，GitHub Actions 会构建 Linux `amd64/arm64` 资源，生成 `checksums.txt`，并创建 GitHub Release。一键安装脚本只使用已发布 Release，不从 `main` 分支下载未经校验的二进制。
