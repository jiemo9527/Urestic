# Urestic

Urestic 是一个中文优先的 restic 易用化工具，不再定位为“GUI for restic”，而是定位为：

```text
easy use for restic
```

备份动作由各服务器自行执行。Urestic 负责生成可部署脚本、保存仓库配置、查询仓库快照、分析备份健康情况，并生成恢复命令。

## 新方向

Urestic 的核心目标：

- 让 restic + R2/B2/S3/rclone 更容易正确配置。
- 生成各服务器本地执行的备份脚本，而不是由中央容器直接扫描远程服务器。
- 一站式查看多个 restic 仓库的快照、主机、标签、路径和最近备份状态。
- 分析哪些备份过期、哪些 tag/host 长时间没有更新、保留策略是否合理。
- 生成恢复命令和初始化命令，降低误操作风险。
- 提供多渠道结果通知配置，让脚本执行结果能发送到 Telegram、Email、Webhook 等渠道。

## 不做什么

第一阶段明确不做：

- 不做中央服务器直接执行所有机器的备份。
- 不做 Agent。
- 不做多用户、多租户或复杂权限系统。
- 不做传统 CRUD 风格的“restic GUI”。
- 不默认读取或暴露 secret、仓库密码、rclone 配置内容。
- rclone 运行配置默认保存在 `/app/data/rclone/rclone.conf`；Docker Compose 默认把宿主机 conf 只读挂载到 `/host-rclone/rclone.conf`，但必须在设置页手动复制才会导入。

## 第一版范围

### Web UI

仍然保留 Web UI，但重做为现代企业工具风格，中文界面优先。

建议主菜单：

```text
总览
仓库
脚本生成
快照
分析
通知
设置
```

UI 原则：

- 中文优先，英文作为辅助语言。
- 不做廉价后台 CRUD 风格。
- 以任务流为中心：配置仓库 -> 生成脚本 -> 部署脚本 -> 查询结果 -> 分析健康状态。
- 表单必须解释字段用途，尤其是 repository、tag、retention、prune；cron 字段只在 cron 类型下出现。

### 脚本生成

脚本类型：

- `python`，默认，优先支持完整能力。
- `js`。
- `sh`。
- `ps1`。
- `cron`，只生成一行 crontab 命令，不附带脚本或 JSON。

Python 脚本优先支持：

- `restic init`。
- `restic unlock`。
- `restic backup`。
- `restic forget --prune`。
- `restic check`。
- `restic snapshots`。
- 本地配置文件读取。
- 执行结果通知。
- 日志输出。

生成内容：

- 备份脚本。
- 配置文件。
- 初始化命令。
- 快照查询命令。
- 恢复命令模板。
- cron 类型只输出一行 crontab 命令。

### 后端支持

第一版优先支持：

- Cloudflare R2。
- Backblaze B2。
- S3 compatible。
- rclone。

其他 restic backend 后续再扩展。

### 仓库观察台

Urestic 查询仓库内容，不负责远程执行备份。

查询能力：

- `restic snapshots --json`。
- 按 host 聚合。
- 按 tag 聚合。
- 按 path 聚合。
- 显示最近备份时间。
- 显示快照数量。
- 判断备份是否过期。
- 生成恢复命令。

### 分析

第一版分析能力：

- 哪些 host 没有最近备份。
- 哪些 tag 长时间未更新。
- 某仓库最近一次快照时间。
- 快照覆盖了哪些路径。
- retention 策略解释和建议。
- prune 是否开启的影响说明。

### 多渠道结果通知

Urestic 需要支持结果通知配置，并让生成脚本可以发送执行结果。

第一版通知渠道：

- Telegram Bot。
- Email SMTP。
- Webhook。

后续可扩展：

- Discord。
- Slack。
- Gotify。
- Bark。
- 企业微信。
- 飞书。

通知事件：

- 备份成功。
- 备份失败。
- forget/prune 成功。
- forget/prune 失败。
- check 失败。
- 仓库锁自动 unlock。
- 仓库长时间无新快照。

通知内容必须包含：

- 仓库名。
- 主机名。
- 标签。
- 备份路径。
- 成功/失败状态。
- 错误摘要。
- 新增数据量。
- 耗时。
- 快照 ID。

通知内容不得包含：

- restic password。
- R2/S3/B2 secret。
- rclone token。
- Authorization header。

## 安全策略

当前用户模型：唯一管理员。

- Web API 必须要求管理员登录。
- 不做多用户系统。
- 仓库密码和云存储 secret 可以加密保存。
- 查询仓库时不要求用户反复输入密码。
- API 响应不返回明文 secret。
- 日志和通知不输出 secret。
- 前端不把 token 保存到 localStorage。

## Cron 与保留策略

Cron 和 retention 不冲突。

- Cron 是运行时间，例如每天 02:00 执行脚本。
- Retention 是保留规则，例如保留最近 10 个、每天 7 个、每月 12 个。
- Prune 是空间回收，例如 forget 后删除不再引用的数据块。

换句话说：

```text
Cron = 什么时候跑
Retention = 保留哪些快照
Prune = 是否回收空间
```

## 技术栈

- 后端：Go + Gin。
- 前端：Vue 3 + TypeScript + Vite。
- 默认语言：`zh-CN`。
- 第二语言：`en-US`。
- 数据库：SQLite 优先。
- 容器：Docker / Docker Compose。

## 当前状态

项目方向已从旧的 “restic Web GUI” 调整为 “easy use for restic”。

后续应先重做产品结构和 UI，再重建功能实现。旧的计划任务执行型设计不再作为主线。
