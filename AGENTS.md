# Urestic

本文件合并原 `AGENTS.md` 与 `PRODUCT.md`，作为项目方向、产品范围和协作约束的唯一说明。

## 定位

Urestic 是中文优先的 restic 易用化 Web 工具，不再定位为“GUI for restic”，而是定位为：

```text
easy use for restic
```

备份动作由各服务器自行执行。Urestic 负责生成可部署脚本、保存仓库配置、查询仓库快照、分析备份健康情况，并生成恢复命令。

## 产品原则

- 备份在数据所在服务器本地执行。
- Urestic 不作为中央备份执行器。
- Web UI 是配置、生成、查询和分析入口。
- 默认中文界面，英文可选。
- 面向唯一管理员，不做多用户权限系统。
- 安全优先，不回显 secret。
- 先支持最常用云对象存储，再扩展其他 backend。

## 核心目标

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
- 不默认读取宿主机 `~/.config/rclone/rclone.conf`。

Agent 是安装在目标服务器上的常驻程序，用来接收中央控制台指令、执行备份、回传日志。它会显著增加安全、部署、升级和网络复杂度，因此不进入第一版。

## 用户流

1. 管理员登录 Urestic。
2. 创建仓库配置，选择 R2/B2/S3/rclone。
3. 配置备份源、tag、保留策略和通知渠道。
4. 选择脚本类型，默认 Python。
5. 生成脚本和配置文件。
6. 用户把脚本部署到目标服务器，由服务器本地 cron/systemd/计划任务执行。
7. Urestic 查询仓库快照。
8. Urestic 展示备份状态、过期分析和恢复命令。

## 页面结构

当前主菜单：

```text
总览
仓库管理
脚本管理
通知插入
设置
```

历史规划中的“快照”和“分析”能力已合入仓库管理和总览/健康分析相关区域，不再保持独立主菜单。

### 总览

- 仓库数量。
- 最近备份状态。
- 过期 host 数量。
- 过期 tag 数量。
- 需要关注的仓库。

### 仓库管理

- 保存 R2/B2/S3/rclone 仓库配置。
- 保存 restic repository URL。
- 加密保存仓库密码和云存储 secret。
- 测试仓库可访问性。
- 查询 snapshots。
- 按仓库、host、tag、path、时间过滤。
- 展示快照 ID、时间、路径、主机、标签。
- 删除指定快照。
- 生成恢复命令。

### 脚本管理

- 脚本类型：Python、JS、sh、ps1、cron。
- 默认 Python。
- 选择仓库。
- 填写备份源目录。
- 设置 tag。
- 设置 retention。
- 设置通知渠道。
- 生成脚本和配置文件。
- 管理已生成脚本列表。
- 下载单个文件或下载全部文件。

### 通知插入

- Telegram Bot。
- Email SMTP。
- Webhook。
- 测试发送。
- 生成脚本可用的通知配置。
- 通知渠道不做启用/停用，是否写入脚本由脚本管理页勾选决定。

### 设置

- 修改 Web UI 管理员密码。
- 导入/导出恢复包。
- rclone 状态、更新和配置导入。
- Docker Compose 可挂载宿主机 rclone.conf，但必须在设置页手动复制才会导入运行配置。
- rclone 配置项只读展示 remote 数量和名称，不在页面编辑 secret。

## UI 原则

- 中文优先，英文作为辅助语言。
- 不做廉价后台 CRUD 风格。
- 以任务流为中心：配置仓库 -> 生成脚本 -> 部署脚本 -> 查询结果 -> 分析健康状态。
- 表单必须解释字段用途，尤其是 repository、tag、retention、prune。
- Cron 字段只在 cron 类型下出现。

## 支持后端

第一版优先支持：

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

其他 restic backend 后续再扩展。

## 脚本生成

脚本类型：

- `python`，默认，优先支持完整能力。
- `js`。
- `sh`。
- `ps1`。
- `cron`，只生成一行 crontab 命令，不附带脚本或 JSON。

脚本管理页默认生成备份脚本；恢复模式默认关闭，开启后生成对应的恢复脚本，不写入 cron、保留策略或通知。

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
- 恢复脚本。
- 配置文件。
- 初始化命令。
- 快照查询命令。
- 恢复命令模板。
- cron 类型只输出一行 crontab 命令。

## 仓库观察台

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

## 分析

第一版分析能力：

- 哪些 host 没有最近备份。
- 哪些 tag 长时间未更新。
- 某仓库最近一次快照时间。
- 快照覆盖了哪些路径。
- retention 策略解释和建议。
- prune 是否开启的影响说明。

## 多渠道结果通知

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

- backup_success。
- backup_failed。
- forget_prune_success。
- forget_prune_failed。
- check_failed。
- unlock_performed。
- stale_backup_detected。

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
- bearer token。
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
- 恢复包必须用用户输入的恢复包密码加密导出，密码不保存；忘记密码无法导入。
- 恢复包包含加密凭据，不能上传到公开仓库。
- 管理员密码不进入恢复包，通过容器内 `urestic reset-admin-password` 单独重置。

## rclone 策略

- rclone 是可选能力。
- 容器可以包含 `rclone` 二进制。
- Urestic 默认 rclone 运行配置路径是 `/app/data/rclone/rclone.conf`。
- Docker Compose 默认把宿主机 conf 只读挂载到 `/host-rclone/rclone.conf`。
- 必须在设置页手动复制，宿主机 conf 才会导入运行配置。
- API 和 UI 不返回 rclone secret 原文。
- 恢复包可以包含运行配置中的 rclone.conf，因此恢复包即使加密也必须作为敏感文件保存。

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

## 恢复包

恢复包导出必须要求输入恢复包密码，输出 `formatVersion=3` 加密包。加密 payload 解密后应覆盖可以迁移和恢复功能状态的内容：

- 仓库配置。
- 仓库密码。
- 云存储 key。
- 通知配置。
- 通知 token。
- 默认变量。
- `/app/data/rclone/rclone.conf`。
- 浏览器本地已生成脚本。
- 主题、语言等前端偏好。

恢复包不包含：

- Web UI 管理员密码。
- SQLite 原始数据库文件。
- snapshots 实时查询结果。
- 备份源数据。
- restic 仓库数据。

导入恢复包必须要求输入恢复包密码。解密后的 `formatVersion=2` 内容应尽量恢复到导出时状态：同名项覆盖，恢复包中不存在的仓库、通知、默认变量和 rclone.conf 可被删除。

## 技术栈

- 后端：Go + Gin。
- 前端：Vue 3 + TypeScript + Vite。
- 默认语言：`zh-CN`。
- 第二语言：`en-US`。
- 数据库：SQLite 优先。
- 容器：Docker / Docker Compose。

## 命令执行

restic 和 rclone 是外部命令，所有命令执行都必须按安全敏感操作处理。

- 不拼接 shell 命令字符串执行。
- 使用 `exec.CommandContext` 参数数组。
- 校验路径、tag、repository ID 和用户提供的选项。
- 日志输出必须避免 secret。
- 长时间运行命令应支持 context timeout 或取消。

## 第一版验收标准

- 可以创建 R2/B2/S3/rclone 仓库配置。
- 可以生成 Python 备份脚本和配置文件。
- Python 脚本可以执行 backup、forget/prune 和通知。
- 可以查询 restic snapshots 并聚合 host/tag/path。
- 可以展示最近备份时间和过期状态。
- 可以生成 restore 命令。
- 可以导出和导入恢复包。
- 可以在容器内重置管理员密码。
- UI 默认中文，交互清晰，避免旧式 CRUD 后台观感。

## 当前状态

项目方向已从旧的 “restic Web GUI” 调整为 “easy use for restic”。旧的计划任务执行型设计不再作为主线。
