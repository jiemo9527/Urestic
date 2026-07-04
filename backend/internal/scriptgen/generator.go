package scriptgen

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/urestic/urestic/backend/internal/repositories"
)

type Request struct {
	ScriptType string         `json:"scriptType"`
	SecretMode string         `json:"secretMode"`
	SourceDirs []string       `json:"sourceDirs"`
	Tags       []string       `json:"tags"`
	Cron       string         `json:"cron"`
	Options    BackupOptions  `json:"options"`
	Retention  Retention      `json:"retention"`
	Notify     NotifySettings `json:"notify"`
	// ResticPassword is filled server-side only when the caller explicitly
	// requests an inline, ready-to-run script package.
	ResticPassword string `json:"-"`
}

type BackupOptions struct {
	InitIfMissing     bool     `json:"initIfMissing"`
	ExcludePatterns   []string `json:"excludePatterns"`
	ExcludeExtensions []string `json:"excludeExtensions"`
	ExcludeIfPresent  []string `json:"excludeIfPresent"`
	ExcludeLargerThan string   `json:"excludeLargerThan"`
	ExcludeCaches     bool     `json:"excludeCaches"`
	ExcludeCloudFiles bool     `json:"excludeCloudFiles"`
	OneFileSystem     bool     `json:"oneFileSystem"`
	UseFsSnapshot     bool     `json:"useFsSnapshot"`
	Compression       string   `json:"compression"`
	UploadLimitKB     int      `json:"uploadLimitKB"`
	DownloadLimitKB   int      `json:"downloadLimitKB"`
	ReadConcurrency   int      `json:"readConcurrency"`
	Host              string   `json:"host"`
	DryRun            bool     `json:"dryRun"`
}

type Retention struct {
	KeepLast    int    `json:"keepLast"`
	KeepDaily   int    `json:"keepDaily"`
	KeepWeekly  int    `json:"keepWeekly"`
	KeepMonthly int    `json:"keepMonthly"`
	KeepYearly  int    `json:"keepYearly"`
	KeepWithin  string `json:"keepWithin"`
	Prune       bool   `json:"prune"`
}

type NotifySettings struct {
	Enabled    bool            `json:"enabled"`
	Channel    string          `json:"channel"`
	ChannelIDs []string        `json:"channelIds,omitempty"`
	Events     []string        `json:"events"`
	Channels   []NotifyChannel `json:"channels,omitempty"`
}

type NotifyChannel struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Settings map[string]string `json:"settings"`
}

type File struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Content  string `json:"content"`
}

type Result struct {
	Files []File `json:"files"`
}

func Generate(repository repositories.Repository, request Request) Result {
	scriptType := strings.TrimSpace(request.ScriptType)
	if scriptType == "" {
		scriptType = "python"
	}
	nameBase := generatedFileBase(repository)
	configName := nameBase + "-config.json"

	switch scriptType {
	case "js":
		return Result{Files: []File{
			{Name: nameBase + "-backup.js", Language: "javascript", Content: javaScript(configName)},
			{Name: configName, Language: "json", Content: pythonConfig(repository, request)},
		}}
	case "sh":
		files := []File{{Name: nameBase + "-backup.sh", Language: "bash", Content: shellScript(repository, request)}}
		if request.Notify.Enabled {
			helperName := nameBase + "-backup.py"
			files[0].Content = shellPythonWrapper(helperName)
			files = append(files, File{Name: helperName, Language: "python", Content: pythonScript(configName)})
		}
		files = append(files, File{Name: configName, Language: "json", Content: pythonConfig(repository, request)})
		return Result{Files: files}
	case "ps1":
		files := []File{{Name: nameBase + "-backup.ps1", Language: "powershell", Content: powershellScript(repository, request)}}
		if request.Notify.Enabled {
			helperName := nameBase + "-backup.py"
			files[0].Content = powershellPythonWrapper(helperName)
			files = append(files, File{Name: helperName, Language: "python", Content: pythonScript(configName)})
		}
		files = append(files, File{Name: configName, Language: "json", Content: pythonConfig(repository, request)})
		return Result{Files: files}
	case "cron":
		return Result{Files: []File{
			{Name: nameBase + "-crontab.txt", Language: "text", Content: cronLine(repository, request)},
		}}
	default:
		return Result{Files: []File{
			{Name: nameBase + "-backup.py", Language: "python", Content: pythonScript(configName)},
			{Name: configName, Language: "json", Content: pythonConfig(repository, request)},
		}}
	}
}

func generatedFileBase(repository repositories.Repository) string {
	name := slug(repository.Name)
	if name == "" {
		name = slug(repository.ID)
	}
	if name == "" {
		name = "repository"
	}
	return name
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	parts := make([]rune, 0, len(value))
	previousDash := false
	for _, item := range value {
		valid := (item >= 'a' && item <= 'z') || (item >= '0' && item <= '9') || item == '_'
		if valid {
			parts = append(parts, item)
			previousDash = false
			continue
		}
		if !previousDash {
			parts = append(parts, '-')
			previousDash = true
		}
	}
	return strings.Trim(string(parts), "-")
}

func pythonConfig(repository repositories.Repository, request Request) string {
	password := "<填写 restic 仓库密码>"
	if inlineSecrets(request) && strings.TrimSpace(request.ResticPassword) != "" {
		password = request.ResticPassword
	}
	config := map[string]any{
		"restic_repository": repositoryURL(repository, request),
		"source_dirs":       request.SourceDirs,
		"tags":              request.Tags,
		"options":           request.Options,
		"retention":         request.Retention,
		"notify":            request.Notify,
		"restic_password":   password,
	}
	for key, value := range visibleVariables(repository, request) {
		if value == "********" {
			config[key] = "<填写 " + key + ">"
			continue
		}
		config[key] = value
	}
	encoded, _ := json.MarshalIndent(config, "", "  ")
	return string(encoded) + "\n"
}

func pythonScript(configName string) string {
	script := `#!/usr/bin/env python3
# -*- coding: utf-8 -*-
from email.message import EmailMessage
import json
import os
import socket
import smtplib
import subprocess
import sys
import time
import urllib.error
import urllib.request

CONFIG_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), '__CONFIG_FILE__')

class CommandFailed(RuntimeError):
    pass

def load_config():
    with open(CONFIG_PATH, 'r', encoding='utf-8') as file:
        return json.load(file)

def configured(value):
    return bool(value) and not str(value).startswith('<填写 ') and value != '********'

def ensure_restic():
    if subprocess.run(['restic', 'version'], text=True, capture_output=True).returncode == 0:
        return
    print('未检测到 restic，尝试自动安装...')
    installers = [
        ['apt-get', 'update'],
        ['apt-get', 'install', '-y', 'restic'],
        ['dnf', 'install', '-y', 'restic'],
        ['yum', 'install', '-y', 'restic'],
        ['apk', 'add', 'restic'],
        ['brew', 'install', 'restic'],
    ]
    for command in installers:
        if shutil.which(command[0]) is None:
            continue
        result = subprocess.run(command, text=True)
        if result.returncode == 0 and subprocess.run(['restic', 'version'], text=True, capture_output=True).returncode == 0:
            return
    raise SystemExit('无法自动安装 restic，请手动安装后重试。')

def restic_env(config):
    env = os.environ.copy()
    if not configured(config.get('restic_password')):
        raise SystemExit('请先在 __CONFIG_FILE__ 中填写 restic_password。')
    env['RESTIC_PASSWORD'] = config['restic_password']
    if configured(config.get('r2_access_key_id')):
        env['AWS_ACCESS_KEY_ID'] = config['r2_access_key_id']
    if configured(config.get('r2_secret_access_key')):
        env['AWS_SECRET_ACCESS_KEY'] = config['r2_secret_access_key']
    if configured(config.get('s3_access_key_id')):
        env['AWS_ACCESS_KEY_ID'] = config['s3_access_key_id']
    if configured(config.get('s3_secret_access_key')):
        env['AWS_SECRET_ACCESS_KEY'] = config['s3_secret_access_key']
    if configured(config.get('s3_region')):
        env['AWS_DEFAULT_REGION'] = config['s3_region']
    if configured(config.get('b2_account_id')):
        env['B2_ACCOUNT_ID'] = config['b2_account_id']
    if configured(config.get('b2_account_key')):
        env['B2_ACCOUNT_KEY'] = config['b2_account_key']
    env.setdefault('AWS_DEFAULT_REGION', 'auto')
    return env

def normalized_extensions(values):
    result = []
    for value in values or []:
        value = str(value).strip().lstrip('.')
        if value:
            result.append(value)
    return result

def backup_args(config):
    options = config.get('options') or {}
    args = []
    for pattern in options.get('excludePatterns') or []:
        if str(pattern).strip():
            args.extend(['--exclude', str(pattern).strip()])
    for extension in normalized_extensions(options.get('excludeExtensions')):
        args.extend(['--exclude', f'*.{extension}'])
    for marker in options.get('excludeIfPresent') or []:
        if str(marker).strip():
            args.extend(['--exclude-if-present', str(marker).strip()])
    if str(options.get('excludeLargerThan') or '').strip():
        args.extend(['--exclude-larger-than', str(options['excludeLargerThan']).strip()])
    if options.get('excludeCaches'):
        args.append('--exclude-caches')
    if options.get('excludeCloudFiles'):
        args.append('--exclude-cloud-files')
    if options.get('oneFileSystem'):
        args.append('--one-file-system')
    if options.get('useFsSnapshot'):
        args.append('--use-fs-snapshot')
    compression = str(options.get('compression') or '').strip()
    if compression in ('auto', 'off', 'max'):
        args.extend(['--compression', compression])
    if int(options.get('uploadLimitKB') or 0) > 0:
        args.extend(['--limit-upload', str(int(options['uploadLimitKB']))])
    if int(options.get('downloadLimitKB') or 0) > 0:
        args.extend(['--limit-download', str(int(options['downloadLimitKB']))])
    if int(options.get('readConcurrency') or 0) > 0:
        args.extend(['--read-concurrency', str(int(options['readConcurrency']))])
    if str(options.get('host') or '').strip():
        args.extend(['--host', str(options['host']).strip()])
    if options.get('dryRun'):
        args.append('--dry-run')
    return args

def snapshot_selection_args(config):
    options = config.get('options') or {}
    args = []
    if str(options.get('host') or '').strip():
        args.extend(['--host', str(options['host']).strip()])
    if options.get('dryRun'):
        args.append('--dry-run')
    return args

def repository_exists(repo, env):
    return subprocess.run(['restic', '-r', repo, 'cat', 'config'], text=True, capture_output=True, env=env).returncode == 0

def ensure_repository(repo, env, config):
    if repository_exists(repo, env):
        return
    if (config.get('options') or {}).get('initIfMissing'):
        print('未检测到已初始化的 restic 仓库，执行 restic init...')
        run(['restic', '-r', repo, 'init'], env)
        return
    raise CommandFailed('restic 仓库尚未初始化。请启用 initIfMissing 或先手动运行 restic init。')

def notify_enabled(config, event):
    notify = config.get('notify', {})
    if not notify.get('enabled'):
        return False
    events = notify.get('events') or []
    return not events or event in events

def send_notifications(config, event, title, details):
    if not notify_enabled(config, event):
        return
    channels = config.get('notify', {}).get('channels') or []
    if not channels:
        print('通知已启用，但没有可用渠道。')
        return
    for channel in channels:
        try:
            send_notification(channel, event, title, details)
        except Exception as error:
            print(f"通知发送失败: {channel.get('name', channel.get('type', 'unknown'))}: {error}", file=sys.stderr)

def notification_details(config, extra=''):
    options = config.get('options') or {}
    host = str(options.get('host') or '').strip() or socket.gethostname()
    sources = ', '.join(config.get('source_dirs') or []) or '-'
    tags = ', '.join(config.get('tags') or []) or '-'
    lines = [
        f"仓库: {config.get('restic_repository', '-')}",
        f"主机: {host}",
        f"备份源: {sources}",
        f"标签: {tags}",
    ]
    if extra:
        lines.append(extra)
    return '\n'.join(lines)

def send_notification(channel, event, title, details):
    channel_type = channel.get('type')
    settings = channel.get('settings') or {}
    if channel_type == 'telegram':
        send_telegram(settings, title, details)
    elif channel_type == 'email':
        send_email(settings, title, details)
    elif channel_type == 'webhook':
        send_webhook(settings, event, title, details)

def send_telegram(settings, title, details):
    token = settings.get('bot_token')
    chat_id = settings.get('chat_id')
    if not configured(token) or not configured(chat_id):
        print('Telegram 通知缺少 bot_token 或 chat_id，已跳过。')
        return
    payload = json.dumps({'chat_id': chat_id, 'text': f'{title}\n{details}'}).encode('utf-8')
    request = urllib.request.Request(
        f'https://api.telegram.org/bot{token}/sendMessage',
        data=payload,
        headers={'Content-Type': 'application/json'},
        method='POST',
    )
    with urllib.request.urlopen(request, timeout=20) as response:
        response.read()

def send_email(settings, title, details):
    host = settings.get('host')
    sender = settings.get('from') or settings.get('username')
    recipient = settings.get('to')
    if not configured(host) or not configured(sender) or not configured(recipient):
        print('Email 通知缺少 host/from/to，已跳过。')
        return
    port = int(settings.get('port') or 587)
    message = EmailMessage()
    message['Subject'] = title
    message['From'] = sender
    message['To'] = recipient
    message.set_content(details)
    with smtplib.SMTP(host, port, timeout=20) as smtp:
        smtp.starttls()
        username = settings.get('username')
        password = settings.get('password')
        if configured(username) and configured(password):
            smtp.login(username, password)
        smtp.send_message(message)

def send_webhook(settings, event, title, details):
    url = settings.get('url')
    if not configured(url):
        print('Webhook 通知缺少 url，已跳过。')
        return
    payload = json.dumps({'event': event, 'title': title, 'details': details}).encode('utf-8')
    headers = {'Content-Type': 'application/json'}
    token = settings.get('token')
    if configured(token):
        headers['Authorization'] = f'Bearer {token}'
    request = urllib.request.Request(url, data=payload, headers=headers, method='POST')
    with urllib.request.urlopen(request, timeout=20) as response:
        response.read()

def run(command, env):
    started = time.time()
    result = subprocess.run(command, text=True, capture_output=True, env=env)
    duration = time.time() - started
    print(result.stdout)
    if result.returncode != 0:
        print(result.stderr, file=sys.stderr)
        raise CommandFailed(result.stderr.strip() or f'命令失败: {command[0]}')
    print(f'完成: {duration:.2f}s')

def main():
    started = time.time()
    config = {}
    try:
        config = load_config()
        ensure_restic()
        repo = config['restic_repository']
        env = restic_env(config)
        ensure_repository(repo, env, config)
        tags = []
        for tag in config.get('tags', []):
            tags.extend(['--tag', tag])
        run(['restic', '-r', repo, 'unlock'], env)
        run(['restic', '-r', repo, 'backup'] + config['source_dirs'] + tags + backup_args(config) + ['--json'], env)
    except Exception as error:
        send_notifications(config, 'backup_failed', '❌ Urestic 备份失败', notification_details(config, f"错误: {error}"))
        raise

    try:
        retention = config.get('retention', {})
        forget = ['restic', '-r', repo, 'forget'] + tags + snapshot_selection_args(config)
        for key, flag in [('keepLast', '--keep-last'), ('keepDaily', '--keep-daily'), ('keepWeekly', '--keep-weekly'), ('keepMonthly', '--keep-monthly'), ('keepYearly', '--keep-yearly')]:
            if retention.get(key):
                forget.extend([flag, str(retention[key])])
        if retention.get('keepWithin'):
            forget.extend(['--keep-within', retention['keepWithin']])
        if retention.get('prune'):
            forget.append('--prune')
        run(forget, env)
    except Exception as error:
        send_notifications(config, 'forget_prune_failed', '⚠️ Urestic 保留清理失败', notification_details(config, f"错误: {error}"))
        raise

    send_notifications(config, 'backup_success', '✅ Urestic 备份成功', notification_details(config, f"耗时: {time.time() - started:.2f}s"))

if __name__ == '__main__':
    main()
`
	return strings.ReplaceAll(script, "__CONFIG_FILE__", configName)
}

func shellPythonWrapper(helperName string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PYTHON_SCRIPT="$SCRIPT_DIR/%s"

if command -v python3 >/dev/null 2>&1; then
  exec python3 "$PYTHON_SCRIPT"
fi
if command -v python >/dev/null 2>&1; then
  exec python "$PYTHON_SCRIPT"
fi
echo "已勾选通知渠道，该脚本需要 Python 运行通知版备份脚本。请安装 python3 后重试。" >&2
exit 1
`, helperName)
}

func powershellPythonWrapper(helperName string) string {
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$PythonScript = Join-Path $ScriptDir %s

if (Get-Command python -ErrorAction SilentlyContinue) {
  & python $PythonScript
  exit $LASTEXITCODE
}
if (Get-Command py -ErrorAction SilentlyContinue) {
  & py -3 $PythonScript
  exit $LASTEXITCODE
}
throw '已勾选通知渠道，该脚本需要 Python 运行通知版备份脚本。请安装 Python 后重试。'
`, psQuote(helperName))
}

func javaScript(configName string) string {
	script := `#!/usr/bin/env node
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawnSync } = require('child_process');

const CONFIG_PATH = path.join(__dirname, '__CONFIG_FILE__');

function loadConfig() {
  return JSON.parse(fs.readFileSync(CONFIG_PATH, 'utf8'));
}

function configured(value) {
  return Boolean(value) && !String(value).startsWith('<填写 ') && value !== '********';
}

function commandExists(name) {
  return spawnSync('sh', ['-c', 'command -v ' + name + ' >/dev/null 2>&1']).status === 0;
}

function ensureRestic() {
  if (spawnSync('restic', ['version'], { encoding: 'utf8' }).status === 0) return;
  console.log('未检测到 restic，尝试自动安装...');
  const installers = [
    ['apt-get', ['update']],
    ['apt-get', ['install', '-y', 'restic']],
    ['dnf', ['install', '-y', 'restic']],
    ['yum', ['install', '-y', 'restic']],
    ['apk', ['add', 'restic']],
    ['brew', ['install', 'restic']]
  ];
  for (const [command, args] of installers) {
    if (!commandExists(command)) continue;
    const result = spawnSync(command, args, { stdio: 'inherit' });
    if (result.status === 0 && spawnSync('restic', ['version'], { encoding: 'utf8' }).status === 0) return;
  }
  throw new Error('无法自动安装 restic，请手动安装后重试。');
}

function resticEnv(config) {
  const env = { ...process.env };
  if (!configured(config.restic_password)) {
    throw new Error('请先在 __CONFIG_FILE__ 中填写 restic_password。');
  }
  env.RESTIC_PASSWORD = config.restic_password;
  if (configured(config.r2_access_key_id)) env.AWS_ACCESS_KEY_ID = config.r2_access_key_id;
  if (configured(config.r2_secret_access_key)) env.AWS_SECRET_ACCESS_KEY = config.r2_secret_access_key;
  if (configured(config.s3_access_key_id)) env.AWS_ACCESS_KEY_ID = config.s3_access_key_id;
  if (configured(config.s3_secret_access_key)) env.AWS_SECRET_ACCESS_KEY = config.s3_secret_access_key;
  if (configured(config.s3_region)) env.AWS_DEFAULT_REGION = config.s3_region;
  if (configured(config.b2_account_id)) env.B2_ACCOUNT_ID = config.b2_account_id;
  if (configured(config.b2_account_key)) env.B2_ACCOUNT_KEY = config.b2_account_key;
  if (!env.AWS_DEFAULT_REGION) env.AWS_DEFAULT_REGION = 'auto';
  return env;
}

function normalizedExtensions(values) {
  return (values || []).map((value) => String(value).trim().replace(/^\.+/, '')).filter(Boolean);
}

function backupArgs(config) {
  const options = config.options || {};
  const args = [];
  for (const pattern of options.excludePatterns || []) {
    if (String(pattern).trim()) args.push('--exclude', String(pattern).trim());
  }
  for (const extension of normalizedExtensions(options.excludeExtensions)) {
    args.push('--exclude', '*.' + extension);
  }
  for (const marker of options.excludeIfPresent || []) {
    if (String(marker).trim()) args.push('--exclude-if-present', String(marker).trim());
  }
  if (String(options.excludeLargerThan || '').trim()) args.push('--exclude-larger-than', String(options.excludeLargerThan).trim());
  if (options.excludeCaches) args.push('--exclude-caches');
  if (options.excludeCloudFiles) args.push('--exclude-cloud-files');
  if (options.oneFileSystem) args.push('--one-file-system');
  if (options.useFsSnapshot) args.push('--use-fs-snapshot');
  if (['auto', 'off', 'max'].includes(String(options.compression || '').trim())) args.push('--compression', String(options.compression).trim());
  if (Number(options.uploadLimitKB || 0) > 0) args.push('--limit-upload', String(Number(options.uploadLimitKB)));
  if (Number(options.downloadLimitKB || 0) > 0) args.push('--limit-download', String(Number(options.downloadLimitKB)));
  if (Number(options.readConcurrency || 0) > 0) args.push('--read-concurrency', String(Number(options.readConcurrency)));
  if (String(options.host || '').trim()) args.push('--host', String(options.host).trim());
  if (options.dryRun) args.push('--dry-run');
  return args;
}

function snapshotSelectionArgs(config) {
  const options = config.options || {};
  const args = [];
  if (String(options.host || '').trim()) args.push('--host', String(options.host).trim());
  if (options.dryRun) args.push('--dry-run');
  return args;
}

function repositoryExists(repo, env) {
  return spawnSync('restic', ['-r', repo, 'cat', 'config'], { encoding: 'utf8', env }).status === 0;
}

function run(command, args, env) {
  const started = Date.now();
  const result = spawnSync(command, args, { encoding: 'utf8', env });
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.status !== 0) {
    if (result.stderr) process.stderr.write(result.stderr);
    throw new Error((result.stderr || '命令失败: ' + command).trim());
  }
  console.log('完成: ' + ((Date.now() - started) / 1000).toFixed(2) + 's');
}

function ensureRepository(repo, env, config) {
  if (repositoryExists(repo, env)) return;
  if ((config.options || {}).initIfMissing) {
    console.log('未检测到已初始化的 restic 仓库，执行 restic init...');
    run('restic', ['-r', repo, 'init'], env);
    return;
  }
  throw new Error('restic 仓库尚未初始化。请启用 initIfMissing 或先手动运行 restic init。');
}

function notifyEnabled(config, event) {
  const notify = config.notify || {};
  if (!notify.enabled) return false;
  const events = notify.events || [];
  return events.length === 0 || events.includes(event);
}

async function sendNotifications(config, event, title, details) {
  if (!notifyEnabled(config, event)) return;
  const channels = (config.notify || {}).channels || [];
  if (channels.length === 0) {
    console.log('通知已启用，但没有可用渠道。');
    return;
  }
  for (const channel of channels) {
    try {
      await sendNotification(channel, event, title, details);
    } catch (error) {
      console.error('通知发送失败: ' + (channel.name || channel.type || 'unknown') + ': ' + error.message);
    }
  }
}

async function sendNotification(channel, event, title, details) {
  const settings = channel.settings || {};
  if (channel.type === 'telegram') return sendTelegram(settings, title, details);
  if (channel.type === 'webhook') return sendWebhook(settings, event, title, details);
  if (channel.type === 'email') {
    console.error('JS 脚本暂不支持 Email SMTP 通知，请使用 Python 脚本。');
  }
}

async function sendTelegram(settings, title, details) {
  if (!configured(settings.bot_token) || !configured(settings.chat_id)) {
    console.log('Telegram 通知缺少 bot_token 或 chat_id，已跳过。');
    return;
  }
  if (typeof fetch !== 'function') throw new Error('当前 Node.js 没有 fetch，请使用 Node 18+。');
  const response = await fetch('https://api.telegram.org/bot' + settings.bot_token + '/sendMessage', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ chat_id: settings.chat_id, text: title + '\n' + details })
  });
  if (!response.ok) throw new Error('Telegram HTTP ' + response.status);
}

async function sendWebhook(settings, event, title, details) {
  if (!configured(settings.url)) {
    console.log('Webhook 通知缺少 url，已跳过。');
    return;
  }
  if (typeof fetch !== 'function') throw new Error('当前 Node.js 没有 fetch，请使用 Node 18+。');
  const headers = { 'Content-Type': 'application/json' };
  if (configured(settings.token)) headers.Authorization = 'Bearer ' + settings.token;
  const response = await fetch(settings.url, {
    method: 'POST',
    headers,
    body: JSON.stringify({ event, title, details })
  });
  if (!response.ok) throw new Error('Webhook HTTP ' + response.status);
}

function notificationDetails(config, extra) {
  const options = config.options || {};
  const host = String(options.host || '').trim() || os.hostname();
  const sources = (config.source_dirs || []).join(', ') || '-';
  const tags = (config.tags || []).join(', ') || '-';
  const lines = [
    '仓库: ' + (config.restic_repository || '-'),
    '主机: ' + host,
    '备份源: ' + sources,
    '标签: ' + tags
  ];
  if (extra) lines.push(extra);
  return lines.join('\n');
}

async function main() {
  const started = Date.now();
  let config = {};
  let repo = '';
  let env = process.env;
  let tags = [];
  try {
    config = loadConfig();
    ensureRestic();
    repo = config.restic_repository;
    env = resticEnv(config);
    ensureRepository(repo, env, config);
    tags = (config.tags || []).flatMap((tag) => ['--tag', String(tag)]);
    run('restic', ['-r', repo, 'unlock'], env);
    run('restic', ['-r', repo, 'backup', ...(config.source_dirs || []).map(String), ...tags, ...backupArgs(config), '--json'], env);
  } catch (error) {
    await sendNotifications(config, 'backup_failed', '❌ Urestic 备份失败', notificationDetails(config, '错误: ' + error.message));
    throw error;
  }

  try {
    const retention = config.retention || {};
    const forget = ['-r', repo, 'forget', ...tags, ...snapshotSelectionArgs(config)];
    for (const [key, flag] of [['keepLast', '--keep-last'], ['keepDaily', '--keep-daily'], ['keepWeekly', '--keep-weekly'], ['keepMonthly', '--keep-monthly'], ['keepYearly', '--keep-yearly']]) {
      if (retention[key]) forget.push(flag, String(retention[key]));
    }
    if (retention.keepWithin) forget.push('--keep-within', retention.keepWithin);
    if (retention.prune) forget.push('--prune');
    run('restic', forget, env);
  } catch (error) {
    await sendNotifications(config, 'forget_prune_failed', '⚠️ Urestic 保留清理失败', notificationDetails(config, '错误: ' + error.message));
    throw error;
  }

  await sendNotifications(config, 'backup_success', '✅ Urestic 备份成功', notificationDetails(config, '耗时: ' + ((Date.now() - started) / 1000).toFixed(2) + 's'));
}

main().catch((error) => {
  console.error(error.message || error);
  process.exit(1);
});
`
	return strings.ReplaceAll(script, "__CONFIG_FILE__", configName)
}

func shellScript(repository repositories.Repository, request Request) string {
	tags := shellTags(request.Tags)
	sources := shellQuoteAll(request.SourceDirs)
	backupArgs := shellBackupArgs(request.Options)
	selectionArgs := shellSelectionArgs(request.Options)
	initBlock := shellInitBlock(request.Options)
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

ensure_restic() {
  if command -v restic >/dev/null 2>&1; then return 0; fi
  echo "未检测到 restic，尝试自动安装..."
  if command -v apt-get >/dev/null 2>&1; then apt-get update && apt-get install -y restic; return; fi
  if command -v dnf >/dev/null 2>&1; then dnf install -y restic; return; fi
  if command -v yum >/dev/null 2>&1; then yum install -y restic; return; fi
  if command -v apk >/dev/null 2>&1; then apk add restic; return; fi
  if command -v brew >/dev/null 2>&1; then brew install restic; return; fi
  echo "无法自动安装 restic，请手动安装后重试。" >&2
  exit 1
}

export RESTIC_REPOSITORY=%s
export RESTIC_PASSWORD=%s
%s

ensure_restic
%s
restic unlock
restic backup %s %s %s
restic forget %s %s %s
`, shellQuote(repositoryURL(repository, request)), shellQuote(resticPassword(request)), shellEnvExports(repository, request), initBlock, sources, tags, backupArgs, tags, selectionArgs, retentionArgs(request.Retention))
}

func powershellScript(repository repositories.Repository, request Request) string {
	sources := psQuoteAll(request.SourceDirs)
	tags := strings.Join(psTags(request.Tags), " ")
	backupArgs := powershellBackupArgs(request.Options)
	selectionArgs := powershellSelectionArgs(request.Options)
	initBlock := powershellInitBlock(request.Options)
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$env:RESTIC_REPOSITORY = %s
$env:RESTIC_PASSWORD = %s
%s

if (-not (Get-Command restic -ErrorAction SilentlyContinue)) {
  Write-Host '未检测到 restic，尝试自动安装...'
  if (Get-Command winget -ErrorAction SilentlyContinue) { winget install restic.restic --silent }
  elseif (Get-Command choco -ErrorAction SilentlyContinue) { choco install restic -y }
  else { throw '无法自动安装 restic，请手动安装后重试。' }
}

%s
restic unlock
restic backup %s %s %s
restic forget %s %s %s
`, psQuote(repositoryURL(repository, request)), psQuote(resticPassword(request)), powershellEnvExports(repository, request), initBlock, sources, tags, backupArgs, tags, selectionArgs, retentionArgs(request.Retention))
}

func cronLine(repository repositories.Repository, request Request) string {
	cron := strings.TrimSpace(request.Cron)
	if cron == "" {
		cron = "0 2 * * *"
	}

	commands := []string{
		"set -eu",
		"export RESTIC_REPOSITORY=" + shellQuote(repositoryURL(repository, request)),
		"export RESTIC_PASSWORD=" + shellQuote(resticPassword(request)),
	}
	commands = append(commands, shellEnvExportCommands(repository, request)...)
	commands = append(commands,
		cronInitCommand(request.Options),
		"restic unlock",
		strings.Join(nonEmpty([]string{"restic backup", shellQuoteAll(request.SourceDirs), shellTags(request.Tags), shellBackupArgs(request.Options)}), " "),
		strings.Join(nonEmpty([]string{"restic forget", shellTags(request.Tags), shellSelectionArgs(request.Options), retentionArgs(request.Retention)}), " "),
	)
	command := "/bin/sh -c " + shellQuote(strings.Join(nonEmpty(commands), "; ")) + " >> /var/log/urestic-backup.log 2>&1"
	return cron + " " + cronEscape(command) + "\n"
}

func shellEnvExportCommands(repository repositories.Repository, request Request) []string {
	env := strings.TrimSpace(shellEnvExports(repository, request))
	if env == "" {
		return nil
	}
	return strings.Split(env, "\n")
}

func cronInitCommand(options BackupOptions) string {
	if options.InitIfMissing {
		return "restic cat config >/dev/null 2>&1 || restic init"
	}
	return "restic cat config >/dev/null 2>&1"
}

func nonEmpty(values []string) []string {
	result := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func cronEscape(value string) string {
	return strings.ReplaceAll(value, "%", `\%`)
}

func shellTags(tags []string) string {
	parts := []string{}
	for _, tag := range tags {
		if strings.TrimSpace(tag) != "" {
			parts = append(parts, "--tag "+shellQuote(tag))
		}
	}
	return strings.Join(parts, " ")
}

func psTags(tags []string) []string {
	parts := []string{}
	for _, tag := range tags {
		if strings.TrimSpace(tag) != "" {
			parts = append(parts, "--tag", "'"+strings.ReplaceAll(tag, "'", "''")+"'")
		}
	}
	return parts
}

func shellQuoteAll(values []string) string {
	quoted := []string{}
	for _, value := range values {
		quoted = append(quoted, shellQuote(value))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func visibleVariables(repository repositories.Repository, request Request) map[string]string {
	secretFields := map[string]struct{}{}
	for _, field := range repository.SecretFields {
		secretFields[field] = struct{}{}
	}

	variables := map[string]string{}
	for key, value := range repository.Variables {
		_, secret := secretFields[key]
		if (secret || secretVariable(key)) && !inlineSecrets(request) {
			if strings.TrimSpace(value) != "" {
				variables[key] = "<填写 " + key + ">"
			}
			continue
		}
		variables[key] = value
	}
	return variables
}

func inlineSecrets(request Request) bool {
	return strings.EqualFold(strings.TrimSpace(request.SecretMode), "inline")
}

func repositoryURL(repository repositories.Repository, request Request) string {
	return RepositoryURL(repository, visibleVariables(repository, request))
}

func RepositoryURL(repository repositories.Repository, variables map[string]string) string {
	url := strings.TrimSpace(repository.RepoURL)
	bucket := ""
	prefix := ""
	endpoint := ""
	switch repository.Backend {
	case "r2":
		bucket = variables["r2_bucket"]
		prefix = variables["r2_prefix"]
		endpoint = variables["r2_s3_api"]
	case "s3":
		bucket = variables["s3_bucket"]
		prefix = variables["s3_prefix"]
		endpoint = variables["s3_endpoint"]
	case "b2":
		bucket = variables["b2_bucket"]
		prefix = variables["b2_prefix"]
	}
	replacements := map[string]string{
		"<r2_s3_api>":   trimRightSlash(endpoint),
		"<accountid>":   variables["r2_account_id"],
		"<account_id>":  variables["r2_account_id"],
		"<s3_endpoint>": trimRightSlash(endpoint),
		"<endpoint>":    trimRightSlash(endpoint),
		"<bucket>":      strings.TrimSpace(bucket),
		"<prefix>":      trimSlash(prefix),
	}
	for placeholder, value := range replacements {
		if value != "" || placeholder == "<prefix>" {
			url = strings.ReplaceAll(url, placeholder, value)
		}
	}
	return strings.TrimRight(url, "/:")
}

func trimRightSlash(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func trimSlash(value string) string {
	return strings.Trim(strings.TrimSpace(value), "/")
}

func secretVariable(key string) bool {
	switch key {
	case "r2_secret_access_key", "s3_secret_access_key", "b2_account_key":
		return true
	default:
		return false
	}
}

func shellEnvExports(repository repositories.Repository, request Request) string {
	variables := visibleVariables(repository, request)
	lines := []string{}
	if variables["r2_access_key_id"] != "" {
		lines = append(lines, "export AWS_ACCESS_KEY_ID="+shellQuote(variables["r2_access_key_id"]))
	}
	if variables["r2_secret_access_key"] != "" {
		lines = append(lines, "export AWS_SECRET_ACCESS_KEY="+shellQuote(variables["r2_secret_access_key"]))
	}
	if variables["s3_access_key_id"] != "" {
		lines = append(lines, "export AWS_ACCESS_KEY_ID="+shellQuote(variables["s3_access_key_id"]))
	}
	if variables["s3_secret_access_key"] != "" {
		lines = append(lines, "export AWS_SECRET_ACCESS_KEY="+shellQuote(variables["s3_secret_access_key"]))
	}
	if variables["s3_region"] != "" {
		lines = append(lines, "export AWS_DEFAULT_REGION="+shellQuote(variables["s3_region"]))
	}
	if variables["b2_account_id"] != "" {
		lines = append(lines, "export B2_ACCOUNT_ID="+shellQuote(variables["b2_account_id"]))
	}
	if variables["b2_account_key"] != "" {
		lines = append(lines, "export B2_ACCOUNT_KEY="+shellQuote(variables["b2_account_key"]))
	}
	if len(lines) > 0 && variables["s3_region"] == "" {
		lines = append(lines, "export AWS_DEFAULT_REGION=auto")
	}
	return strings.Join(lines, "\n")
}

func powershellEnvExports(repository repositories.Repository, request Request) string {
	variables := visibleVariables(repository, request)
	lines := []string{}
	if variables["r2_access_key_id"] != "" {
		lines = append(lines, "$env:AWS_ACCESS_KEY_ID = "+psQuote(variables["r2_access_key_id"]))
	}
	if variables["r2_secret_access_key"] != "" {
		lines = append(lines, "$env:AWS_SECRET_ACCESS_KEY = "+psQuote(variables["r2_secret_access_key"]))
	}
	if variables["s3_access_key_id"] != "" {
		lines = append(lines, "$env:AWS_ACCESS_KEY_ID = "+psQuote(variables["s3_access_key_id"]))
	}
	if variables["s3_secret_access_key"] != "" {
		lines = append(lines, "$env:AWS_SECRET_ACCESS_KEY = "+psQuote(variables["s3_secret_access_key"]))
	}
	if variables["s3_region"] != "" {
		lines = append(lines, "$env:AWS_DEFAULT_REGION = "+psQuote(variables["s3_region"]))
	}
	if variables["b2_account_id"] != "" {
		lines = append(lines, "$env:B2_ACCOUNT_ID = "+psQuote(variables["b2_account_id"]))
	}
	if variables["b2_account_key"] != "" {
		lines = append(lines, "$env:B2_ACCOUNT_KEY = "+psQuote(variables["b2_account_key"]))
	}
	if len(lines) > 0 && variables["s3_region"] == "" {
		lines = append(lines, "$env:AWS_DEFAULT_REGION = 'auto'")
	}
	return strings.Join(lines, "\n")
}

func psQuoteAll(values []string) string {
	quoted := []string{}
	for _, value := range values {
		quoted = append(quoted, psQuote(value))
	}
	return strings.Join(quoted, " ")
}

func psQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func resticPassword(request Request) string {
	if inlineSecrets(request) && strings.TrimSpace(request.ResticPassword) != "" {
		return request.ResticPassword
	}
	return "<填写 restic 仓库密码>"
}

func shellInitBlock(options BackupOptions) string {
	if options.InitIfMissing {
		return `if ! restic cat config >/dev/null 2>&1; then
  echo "未检测到已初始化的 restic 仓库，执行 restic init..."
  restic init
fi`
	}
	return `if ! restic cat config >/dev/null 2>&1; then
  echo "restic 仓库尚未初始化。请启用 initIfMissing 或先手动运行 restic init。" >&2
  exit 1
fi`
}

func powershellInitBlock(options BackupOptions) string {
	if options.InitIfMissing {
		return `restic cat config *> $null
if ($LASTEXITCODE -ne 0) {
  Write-Host '未检测到已初始化的 restic 仓库，执行 restic init...'
  restic init
  if ($LASTEXITCODE -ne 0) { throw 'restic init 失败。' }
}`
	}
	return `restic cat config *> $null
if ($LASTEXITCODE -ne 0) {
  throw 'restic 仓库尚未初始化。请启用 initIfMissing 或先手动运行 restic init。'
}`
}

func shellBackupArgs(options BackupOptions) string {
	return backupArgs(options, shellQuote)
}

func powershellBackupArgs(options BackupOptions) string {
	return backupArgs(options, psQuote)
}

func shellSelectionArgs(options BackupOptions) string {
	return selectionArgs(options, shellQuote)
}

func powershellSelectionArgs(options BackupOptions) string {
	return selectionArgs(options, psQuote)
}

func backupArgs(options BackupOptions, quote func(string) string) string {
	parts := []string{}
	for _, pattern := range cleanList(options.ExcludePatterns) {
		parts = append(parts, "--exclude", quote(pattern))
	}
	for _, extension := range cleanExtensions(options.ExcludeExtensions) {
		parts = append(parts, "--exclude", quote("*."+extension))
	}
	for _, marker := range cleanList(options.ExcludeIfPresent) {
		parts = append(parts, "--exclude-if-present", quote(marker))
	}
	if largerThan := strings.TrimSpace(options.ExcludeLargerThan); largerThan != "" {
		parts = append(parts, "--exclude-larger-than", quote(largerThan))
	}
	if options.ExcludeCaches {
		parts = append(parts, "--exclude-caches")
	}
	if options.ExcludeCloudFiles {
		parts = append(parts, "--exclude-cloud-files")
	}
	if options.OneFileSystem {
		parts = append(parts, "--one-file-system")
	}
	if options.UseFsSnapshot {
		parts = append(parts, "--use-fs-snapshot")
	}
	if compression := cleanCompression(options.Compression); compression != "" {
		parts = append(parts, "--compression", quote(compression))
	}
	if options.UploadLimitKB > 0 {
		parts = append(parts, "--limit-upload", fmt.Sprint(options.UploadLimitKB))
	}
	if options.DownloadLimitKB > 0 {
		parts = append(parts, "--limit-download", fmt.Sprint(options.DownloadLimitKB))
	}
	if options.ReadConcurrency > 0 {
		parts = append(parts, "--read-concurrency", fmt.Sprint(options.ReadConcurrency))
	}
	if host := strings.TrimSpace(options.Host); host != "" {
		parts = append(parts, "--host", quote(host))
	}
	if options.DryRun {
		parts = append(parts, "--dry-run")
	}
	return strings.Join(parts, " ")
}

func selectionArgs(options BackupOptions, quote func(string) string) string {
	parts := []string{}
	if host := strings.TrimSpace(options.Host); host != "" {
		parts = append(parts, "--host", quote(host))
	}
	if options.DryRun {
		parts = append(parts, "--dry-run")
	}
	return strings.Join(parts, " ")
}

func cleanList(values []string) []string {
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func cleanExtensions(values []string) []string {
	result := []string{}
	for _, value := range values {
		value = strings.TrimPrefix(strings.TrimSpace(value), ".")
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func cleanCompression(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "off", "max":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func retentionArgs(retention Retention) string {
	parts := []string{}
	if retention.KeepLast > 0 {
		parts = append(parts, "--keep-last", fmt.Sprint(retention.KeepLast))
	}
	if retention.KeepDaily > 0 {
		parts = append(parts, "--keep-daily", fmt.Sprint(retention.KeepDaily))
	}
	if retention.KeepWeekly > 0 {
		parts = append(parts, "--keep-weekly", fmt.Sprint(retention.KeepWeekly))
	}
	if retention.KeepMonthly > 0 {
		parts = append(parts, "--keep-monthly", fmt.Sprint(retention.KeepMonthly))
	}
	if retention.KeepYearly > 0 {
		parts = append(parts, "--keep-yearly", fmt.Sprint(retention.KeepYearly))
	}
	if retention.KeepWithin != "" {
		parts = append(parts, "--keep-within", shellQuote(retention.KeepWithin))
	}
	if retention.Prune {
		parts = append(parts, "--prune")
	}
	return strings.Join(parts, " ")
}
