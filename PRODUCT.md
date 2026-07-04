# Urestic Product Definition

## 一句话定位

Urestic 是中文优先的 restic 易用化 Web 工具，用来生成备份脚本、管理仓库配置、查询仓库内容、分析备份健康状态和发送结果通知。

## 产品原则

- 备份在数据所在服务器本地执行。
- Urestic 不作为中央备份执行器。
- Web UI 是配置、生成、查询和分析入口。
- 默认中文界面，英文可选。
- 面向唯一管理员，不做多用户权限系统。
- 安全优先，不回显 secret。
- 先支持最常用云对象存储，再扩展其他 backend。

## 第一版用户流

1. 管理员登录 Urestic。
2. 创建仓库配置，选择 R2/B2/S3/rclone。
3. 配置备份源、tag、保留策略和通知渠道。
4. 选择脚本类型，默认 Python。
5. 生成脚本和配置文件。
6. 用户把脚本部署到目标服务器，由服务器本地 cron/systemd/计划任务执行。
7. Urestic 查询仓库快照。
8. Urestic 展示备份状态、过期分析和恢复命令。

## 页面结构

### 总览

- 仓库数量。
- 最近成功备份时间。
- 最近失败通知。
- 过期 host 数量。
- 过期 tag 数量。
- 需要关注的仓库。

### 仓库

- 保存 R2/B2/S3/rclone 仓库配置。
- 保存 restic repository URL。
- 加密保存仓库密码和云存储 secret。
- 测试仓库可访问性。
- 初始化命令生成。

### 脚本生成

- 脚本类型：Python、JS、sh、ps1、cron。
- 默认 Python。
- 选择仓库。
- 填写备份源目录。
- 设置 tag。
- 设置 retention。
- 设置通知渠道。
- 生成脚本和配置文件。
- 生成部署说明。

### 快照

- 查询 snapshots。
- 按仓库、host、tag、path、时间过滤。
- 展示快照 ID、时间、路径、主机、标签。
- 生成 restore 命令。

### 分析

- 最近备份时间分析。
- host 过期分析。
- tag 过期分析。
- 路径覆盖分析。
- retention 策略解释。
- prune 风险提示。

### 通知

- Telegram Bot。
- Email SMTP。
- Webhook。
- 通知模板。
- 测试发送。
- 生成脚本可用的通知配置。

### 设置

- 管理员安全配置。
- 默认语言。
- 默认脚本类型。
- 默认过期阈值。
- rclone 状态、更新和配置导入；Docker Compose 可挂载宿主机 rclone.conf，但必须在设置页手动复制才会导入运行配置。

## 支持后端

第一版必须支持：

- Cloudflare R2。
- Backblaze B2。
- S3 compatible。
- rclone。

第一版不要求支持：

- Azure Blob。
- Google Cloud Storage。
- Swift。
- SFTP。
- REST Server。
- 本地目录仓库。

## 脚本类型

### Python 默认脚本

必须支持：

- 本地 JSON 配置文件。
- restic init。
- restic unlock。
- restic backup。
- restic forget --prune。
- restic check。
- restic snapshots。
- 多渠道结果通知。
- 标准输出日志。
- 错误摘要。

### Shell 脚本

用于 Linux 简单场景。

### JS 脚本

用于已安装 Node.js 的简单场景，不替代 Python 的完整能力。

### PowerShell 脚本

用于 Windows 场景。

### Cron

只生成一行 crontab 命令，不产生脚本、JSON 或其它附加文件。

## 多渠道结果通知

第一版渠道：

- Telegram。
- Email SMTP。
- Webhook。

通知级别：

- success。
- warning。
- error。

通知事件：

- backup_success。
- backup_failed。
- forget_prune_success。
- forget_prune_failed。
- check_failed。
- unlock_performed。
- stale_backup_detected。

通知模板字段：

- repository。
- host。
- tags。
- paths。
- status。
- snapshot_id。
- files_new。
- files_changed。
- data_added。
- duration。
- error_summary。

禁止进入通知的字段：

- restic_password。
- access_key_secret。
- rclone token。
- bearer token。
- authorization header。

## 安全模型

- 唯一管理员。
- 登录后访问 Web UI。
- 仓库密码和 secret 加密保存。
- 查询仓库时不再要求用户重复输入密码。
- API 不返回明文 secret。
- 日志不保存明文 secret。
- 前端不持久化 token 到 localStorage。

## Agent

暂不做 Agent。

Agent 是安装在目标服务器上的常驻程序，用来接收中央控制台指令、执行备份、回传日志。它会显著增加安全、部署、升级和网络复杂度，因此不进入第一版。

## 第一版验收标准

- 可以创建 R2/B2/S3/rclone 仓库配置。
- 可以生成 Python 备份脚本和配置文件。
- Python 脚本可以执行 backup、forget/prune 和通知。
- 可以查询 restic snapshots 并聚合 host/tag/path。
- 可以展示最近备份时间和过期状态。
- 可以生成 restore 命令。
- UI 默认中文，交互清晰，避免旧式 CRUD 后台观感。
