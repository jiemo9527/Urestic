# Urestic 使用说明

Urestic 是一个 Docker 优先的 restic 辅助 Web 工具，用来管理仓库配置、生成备份脚本、配置通知并查询快照。

## 准备

需要先安装：

- Docker
- Docker Compose

Urestic 容器内会安装 `restic` 和 `rclone`。

## 启动

直接使用 Docker Hub 镜像启动：

```bash
docker compose up -d
```

也可以手动创建 `docker-compose.yml`：

```yaml
services:
  urestic:
    image: wanxve0000/urestic:latest
    container_name: urestic
    ports:
      - "8085:8085"
    environment:
      - TZ=Asia/Shanghai
    volumes:
      - ./data:/app/data
      - ./backups:/backups
      - ./sources:/sources:ro
      - ./restore:/restore
    restart: unless-stopped
```

打开 Web UI：

```text
http://localhost:8085
```

默认管理员账号是 `admin`。首次启动如果没有设置管理员密码，Urestic 会自动生成初始密码并写入 `/app/data/admin_password.sha256`，明文初始密码只会打印到容器日志：

```bash
docker logs urestic
```

如果要预先指定密码，可以通过环境变量设置 `URESTIC_ADMIN_PASSWORD`，否则不需要 `.env`。

## 重置管理员密码

如果忘记 Web UI 管理员密码，可以在容器内重置：

```bash
docker compose exec urestic urestic reset-admin-password 'new-strong-password'
```

也可以通过标准输入传入，避免密码进入 shell history：

```bash
printf '%s' 'new-strong-password' | docker compose exec -T urestic urestic reset-admin-password --stdin
```

重置会写入 `/app/data/admin_password.sha256`。服务端会在登录时重新读取该文件，通常无需重启容器。

## 数据目录

默认持久化目录：

```text
./data -> /app/data
./backups -> /backups
./sources -> /sources:ro
./restore -> /restore
```

不要把 `.env`、`data/`、`backups/`、`sources/`、`restore/` 提交到公开仓库。

## 基本使用流程

1. 登录 Web UI。
2. 进入“仓库管理”，新增 restic 仓库配置。
3. 填写 repository URL、restic password 和云存储参数。
4. 点击“检测”或“检测全部”确认仓库和凭据可用。
5. 进入“通知插入”，按需添加 Telegram、Email 或 Webhook。
6. 进入“脚本管理”，选择仓库、脚本类型、备份源、tag、保留策略和通知渠道。
7. 生成脚本，下载脚本和配置文件。
8. 把生成文件放到实际需要备份的服务器上运行。
9. 回到“仓库管理”，打开仓库快照列表查看备份结果。

## 生成脚本

支持脚本类型：

- `python`
- `js`
- `sh`
- `ps1`
- `cron`

`python` 和 `js` 会生成备份脚本和 `repo-config.json`。

`sh` 和 `ps1` 在勾选通知渠道时会额外生成 Python helper，并由 wrapper 调用。

`cron` 只生成一行 crontab 命令，不生成脚本或配置文件。

## 在目标服务器运行脚本

Python 示例：

```bash
python3 repo-backup.py
```

Shell 示例：

```bash
chmod +x repo-backup.sh
./repo-backup.sh
```

PowerShell 示例：

```powershell
pwsh ./repo-backup.ps1
```

运行脚本的服务器需要能访问对应的备份源目录和 restic 仓库。

## rclone 配置

Urestic 默认使用容器内隔离配置：

```text
/app/data/rclone/rclone.conf
```

Docker Compose 默认把宿主机 rclone 配置只读挂载到：

```text
/host-rclone/rclone.conf
```

需要导入时，进入“设置”页面点击“复制/新建 conf”。Urestic 只展示 remote 数量和名称，不在页面编辑 remote secret。

## 恢复包导入导出

进入“设置”页面可以导出或导入 Urestic 恢复包。

恢复包包含：

- 仓库配置和 restic password。
- 云存储 key。
- 通知渠道和通知 token。
- 默认变量。
- `/app/data/rclone/rclone.conf` 内容。
- 浏览器本地已生成脚本列表。
- 主题、语言、备份源候选等前端偏好。

恢复包不包含 Web UI 管理员密码。需要恢复管理员密码时使用 `urestic reset-admin-password`。

导入 `formatVersion=2` 恢复包时会覆盖同名仓库和通知，并删除恢复包中不存在的仓库、通知、默认变量和 rclone.conf，以尽量恢复到导出时状态。

恢复包等同于明文凭据备份，请按敏感文件保存，不要上传到公开仓库或发给不可信对象。

## 常用命令

查看容器状态：

```bash
docker compose ps
```

查看日志：

```bash
docker compose logs -f urestic
```

停止服务：

```bash
docker compose down
```

更新并重建：

```bash
git pull
docker compose up -d --build
```
