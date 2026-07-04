# AGENTS.md

This file guides AI agents and contributors working in this repository.

## Project

Urestic is a Docker-first Web UI and API service for managing restic backups.

Primary goals:

- Manage restic repositories, snapshots, tags, jobs, schedules, and retention policies.
- Provide a Chinese-first UI with English support.
- Run cleanly in Docker with persistent data under `/app/data`.
- Treat rclone as an optional backend, not a required dependency.
- Keep container rclone configuration isolated from the host by default.
- Expose a stable `/api/v1` API for remote automation and future agents.

## Stack

- Backend: Go + Gin.
- Frontend: Vue 3 + TypeScript + Vite.
- Database target: SQLite first, PostgreSQL later if needed.
- Scheduler target: Go cron scheduler, single-node first.
- Container: Docker and Docker Compose.

Use Gin for the main backend. Do not switch to Fiber unless the maintainer explicitly requests it.

## Language

- UI default language: `zh-CN`.
- Secondary language: `en-US`.
- Public documentation should prefer Chinese first, with English docs where practical.
- API error codes must be stable English identifiers. User-facing text can be translated.

## rclone Policy

rclone is optional.

- The container may include the `rclone` binary.
- Urestic must not read the host `~/.config/rclone/rclone.conf` by default.
- Default rclone config path is `/app/data/rclone/rclone.conf`.
- Host rclone config can be imported later, but runtime config should remain isolated in Urestic data.
- Never expose rclone secrets, tokens, or config contents in API responses or logs.

## restic Backends

Support restic native backends first:

- Local directory.
- SFTP.
- REST server.
- S3 compatible storage.
- OpenStack Swift.
- Backblaze B2.
- Microsoft Azure Blob Storage.
- Google Cloud Storage.

Optional extension:

- rclone backend using `rclone:remote:path`.

## API Rules

- API prefix is `/api/v1`.
- Return JSON consistently.
- Do not return plaintext repository passwords, API tokens, rclone config, or environment secrets.
- Prefer explicit error codes over free-form strings.

Success response shape:

```json
{
  "success": true,
  "data": {},
  "message": "ok"
}
```

Error response shape:

```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human readable message"
  }
}
```

## Command Execution

restic and rclone are external commands. Treat all command execution as security-sensitive.

- Do not build command strings and run them through a shell.
- Use argument arrays with `exec.CommandContext`.
- Validate paths, tags, repository IDs, and user-provided options before execution.
- Stream stdout/stderr to job logs with secret redaction.
- Store exit code, start time, finish time, and duration for every run.
- Add cancellation support for long-running jobs.

## Docker Rules

- Persist application state under `/app/data`.
- Use `/backups` for mounted backup sources or local repositories.
- Use `/restore` for restore output.
- Do not mount the host root filesystem by default.
- Keep restore paths explicit to avoid overwriting source data.

## Security Rules

- High-risk operations must be auditable: restore, delete snapshot, forget, prune, unlock, delete repository, and custom command execution.
- Secrets must be encrypted at rest once persistence is implemented.
- Logs must redact repository passwords, API tokens, rclone tokens, and authorization headers.
- API tokens should use `Authorization: Bearer <token>`.
- Future remote agents must require token auth and should default to loopback binding.

## Code Style

- Keep changes small and direct.
- Prefer clear package boundaries over premature abstraction.
- Backend code must pass `gofmt`.
- Frontend code must use TypeScript.
- Avoid adding large frameworks unless they solve a current problem.
- Do not add backward-compatibility layers before the project has shipped persisted formats or stable APIs.

## Verification

Before completing backend changes, run when available:

```text
go test ./...
```

Before completing frontend changes, run when available:

```text
npm run build
```

For Docker changes, run when practical:

```text
docker compose config
```
