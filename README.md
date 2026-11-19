# Urestic 使用说明

Urestic 是一个 Docker 优先的 restic 辅助 Web 工具，用来管理仓库配置、生成备份脚本、配置通知并查询快照。

## 准备

需要先安装：

- Docker
- Docker Compose

Urestic 容器内会安装 `restic` 和固定版本 `rclone v1.74.3`。

## 启动

直接使用 Docker Hub 镜像启动：

```bash
docker compose up -d
```

也可以手动创建 `docker-compose.yml`：

```yaml
services:
  urestic:
    image: ghcr.io/jiemo9527/urestic:latest
    container_name: urestic
    ports:
      - "127.0.0.1:8085:8085"
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

端口默认只绑定 `127.0.0.1`，不对公网开放。Urestic 保存着仓库密码和云存储密钥，且 Docker 发布的端口会绕过 ufw 等主机防火墙，所以不建议改成 `"8085:8085"` 直接暴露。远程服务器上可以用 SSH 隧道访问：

```bash
ssh -L 8085:127.0.0.1:8085 <server>
```

保持连接后在本机浏览器打开 `http://localhost:8085`。确实需要公网访问时，请放在带 HTTPS 的反向代理后面，并只把 `127.0.0.1:8085` 作为上游。

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
7. 生成备份脚本，下载脚本和配置文件。
8. 把生成文件放到实际需要备份的服务器上运行。
9. 回到“仓库管理”，打开仓库快照列表查看备份结果。

## 快照批量删除与清空

在“仓库管理”打开仓库快照列表后：

- 勾选快照后点击“删除所选”；选择会跨分页保留。
- “全选筛选结果（跨页）”选择所有符合搜索条件的快照，不仅是当前页。
- “清空当前仓库快照”针对当前仓库全部快照，不受搜索或分页影响，需再次输入完整仓库名确认。不会删除仓库配置或备份源文件，但删除的备份无法再恢复。
- 一批快照统一执行一次 `forget` 和一次 `prune`，任务在后台运行，页面显示核对、删除、空间回收及最终结果；同一仓库不能重复提交并行删除。
- 服务端只删除明确确认的快照 ID，不使用无条件 `--all`。若清空前发现快照列表变化，任务失败并要求刷新确认；若删除期间新增快照，保留新增快照并在最终核验时提示未清空。通过全选删除全部快照同样要求输入仓库名。
- 页面断开不会取消后台任务；同一标签页刷新后会尝试恢复状态查询。提交响应丢失时，重新打开仓库快照窗口可找回该仓库最新任务。任务状态保存在服务进程内，服务重启后不自动重跑删除；应重新查询快照和仓库锁。
- 原单快照 DELETE 接口改为返回 `202` 和任务信息，调用方必须查询任务状态，不能把响应视作删除已完成；批量接口只接受完整 64 位十六进制快照 ID。
- 后台任务有 1 小时时限，Linux 容器超时先尝试中断 restic 并等待 30 秒退出，再强制终止。Windows 不支持该中断信号，会直接终止，且每批最多 400 个快照；Linux 每批最多 10,000 个。不要在清理期间重启容器；容器异常退出仍可能留下锁。
- 如果删除完成但 `prune` 失败，界面会明确提示空间尚未清理，不能把快照消失视为回收完成。共享数据不会被删除，B2 容量统计也可能稍后更新。

开发验证：后端 `go test ./...`；设置 `URESTIC_TEST_RESTIC` 为 restic 可执行文件路径可启用临时本地仓库集成测试。前端 `npm run build` 和 `npm run test:e2e`（先 `npx playwright install chromium`，或用 `PLAYWRIGHT_CHROMIUM_EXECUTABLE` 指定测试浏览器）。浏览器测试使用明确的模拟 API，不接触生产仓库。

## 生成脚本

支持脚本类型：

- `python`
- `js`
- `sh`
- `ps1`
- `cron`

默认是备份模式。`python` 和 `js` 会生成备份脚本和 `repo-config.json`。

`sh` 和 `ps1` 在勾选通知渠道时会额外生成 Python helper，并由 wrapper 调用。

`cron` 只生成一行 crontab 命令，不生成脚本或配置文件。

开启“恢复模式”后会生成恢复脚本，文件名为 `repo-restore.*`，配置文件为 `repo-restore-config.json`。恢复脚本只执行 `restic restore`，不会执行 backup、forget/prune、cron 或通知。

恢复模式字段：

- Snapshot ID：默认 `latest`，也可以填写具体快照 ID。
- 恢复目标目录：默认 `/restore`。
- 只恢复这些路径：可选，逗号分隔，会生成 `--include` 参数。
- Host 名称和标签：仅在 Snapshot ID 为 `latest` 时作为筛选条件写入恢复命令。

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

恢复脚本示例：

```bash
python3 repo-restore.py
```

恢复脚本会把数据恢复到配置里的目标目录，不会覆盖 Urestic Web 服务的数据目录，除非你手动把目标目录指向那里。

## rclone 配置

容器默认包含 `rclone v1.74.3`。

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

进入“设置”页面可以导出或导入 Urestic 加密恢复包。导出和导入都必须输入恢复包密码。

恢复包包含：

- 仓库配置和 restic password。
- 云存储 key。
- 通知渠道和通知 token。
- 默认变量。
- `/app/data/rclone/rclone.conf` 内容。
- 浏览器本地已生成脚本列表。
- 主题、语言等前端偏好。

恢复包不包含 Web UI 管理员密码。需要恢复管理员密码时使用 `urestic reset-admin-password`。

导出的文件是 `formatVersion=3` 加密包，内部 payload 解密后是 `formatVersion=2` 配置。Urestic 不保存恢复包密码，忘记密码将无法导入该恢复包。

导入恢复包时会覆盖同名仓库和通知，并删除恢复包中不存在的仓库、通知、默认变量和 rclone.conf，以尽量恢复到导出时状态。

恢复包内含加密凭据，请按敏感文件保存，不要上传到公开仓库或发给不可信对象。

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

更新 Docker Hub 镜像并重启：

```bash
docker compose pull
docker compose up -d
```
