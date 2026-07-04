export interface ApiResponse<T> {
  success: boolean
  data?: T
  message?: string
  error?: {
    code: string
    message: string
  }
}

export class ApiRequestError extends Error {
  status: number
  code: string

  constructor(message: string, status: number, code: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

export interface AuthUser {
  username: string
}

export interface LoginResult {
  user: AuthUser
  token: string
  expiresAt: string
}

export interface SystemInfo {
  name: string
  version: string
  mode: string
  language: string
  dataDir: string
  databasePath: string
  authEnabled: boolean
  adminUsername: string
}

export interface RcloneStatus {
  installed: boolean
  version: string
  message: string
  configPath: string
  configExists: boolean
  importPath: string
  importPathExists: boolean
  cacheDir: string
}

export interface RcloneOperationResult {
  updated?: boolean
  imported?: boolean
  createdEmpty?: boolean
  output?: string
  status: RcloneStatus
}

export interface RcloneRemote {
  name: string
  type: string
  settings: Record<string, string>
  secretFields: string[]
}

export interface AppLogEntry {
  id: number
  time: string
  message: string
}

export interface BackendTemplate {
  id: string
  name: string
  repoExample: string
  fields: string[]
}

export interface Repository {
  id: string
  name: string
  backend: string
  repoUrl: string
  variables: Record<string, string>
  secretFields: string[]
  description: string
  createdAt: string
  updatedAt: string
}

export interface RepositoryInput {
  name: string
  backend: string
  repoUrl: string
  password: string
  variables: Record<string, string>
  description: string
}

export interface Retention {
  keepLast: number
  keepDaily: number
  keepWeekly: number
  keepMonthly: number
  keepYearly: number
  keepWithin: string
  prune: boolean
}

export interface ScriptGenerateInput {
  repositoryId: string
  scriptType: string
  secretMode: string
  mode: string
  sourceDirs: string[]
  tags: string[]
  cron: string
  options: BackupOptions
  retention: Retention
  restore: RestoreOptions
  notify: {
    enabled: boolean
    channel: string
    channelIds: string[]
    events: string[]
  }
}

export interface RestoreOptions {
  snapshotId: string
  targetDir: string
  includePaths: string[]
}

export interface BackupOptions {
  initIfMissing: boolean
  excludePatterns: string[]
  excludeExtensions: string[]
  excludeIfPresent: string[]
  excludeLargerThan: string
  excludeCaches: boolean
  excludeCloudFiles: boolean
  oneFileSystem: boolean
  useFsSnapshot: boolean
  compression: string
  uploadLimitKB: number
  downloadLimitKB: number
  readConcurrency: number
  host: string
  dryRun: boolean
}

export interface GeneratedFile {
  name: string
  language: string
  content: string
}

export interface ScriptGenerateResult {
  files: GeneratedFile[]
}

export interface Snapshot {
  id: string
  shortId?: string
  time: string
  tree?: string
  paths: string[]
  hostname?: string
  username?: string
  uid?: number
  gid?: number
  tags?: string[]
  programVersion?: string
  parent?: string
}

export interface InsightSummary {
  repositoryCount: number
  snapshotCount: number
  staleHours: number
  hosts: InsightItem[]
  tags: InsightItem[]
  paths: InsightItem[]
  failures: Array<{ repository: string; error: string }>
  message: string
}

export interface InsightItem {
  name: string
  lastBackup: string
  ageHours: number
  stale: boolean
}

export interface NotificationChannel {
  id: string
  name: string
  type: string
  settings: Record<string, string>
  secretFields: string[]
  createdAt: string
  updatedAt: string
}

export interface NotificationInput {
  name: string
  type: string
  settings: Record<string, string>
}

export interface NotificationTemplate {
  type: string
  name: string
  fields: string[]
  secretFields: string[]
}

export interface ConfigExport {
  formatVersion: number
  exportedAt: string
  repositories: RepositoryExport[]
  notifications: NotificationExport[]
  defaultVariables?: Record<string, string>
  rcloneConfig?: RcloneConfigExport
  client?: ClientConfigExport
}

export interface RepositoryExport {
  name: string
  backend: string
  repoUrl: string
  password: string
  variables: Record<string, string>
  description: string
}

export interface NotificationExport {
  name: string
  type: string
  settings: Record<string, string>
}

export interface RcloneConfigExport {
  included: boolean
  path: string
  content: string
}

export interface ClientConfigExport {
  generatedFiles?: Array<GeneratedFile & { savedAt: string }>
  selectedGeneratedFileName?: string
  theme?: string
  locale?: string
}

export interface ConfigImportResult {
  repositoriesCreated: number
  repositoriesUpdated: number
  repositoriesDeleted: number
  repositoriesSkipped: number
  notificationsCreated: number
  notificationsUpdated: number
  notificationsDeleted: number
  notificationsSkipped: number
  defaultVariablesRestored: number
  defaultVariablesDeleted: number
  rcloneConfigRestored: boolean
  rcloneConfigRemoved: boolean
}

export async function loginAdmin(username: string, password: string): Promise<LoginResult> {
  return request<LoginResult>('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password })
  })
}

export async function logoutAdmin(): Promise<void> {
  await request<{ loggedOut: boolean }>('/api/v1/auth/logout', { method: 'POST' })
}

export async function getCurrentUser(): Promise<AuthUser> {
  return request<AuthUser>('/api/v1/auth/me')
}

export async function getSystemInfo(): Promise<SystemInfo> {
  return request<SystemInfo>('/api/v1/system/info')
}

export async function getRcloneStatus(): Promise<RcloneStatus> {
  return request<RcloneStatus>('/api/v1/settings/rclone')
}

export async function updateRcloneBinary(): Promise<RcloneOperationResult> {
  return request<RcloneOperationResult>('/api/v1/settings/rclone/update', { method: 'POST' })
}

export async function importRcloneConfig(): Promise<RcloneOperationResult> {
  return request<RcloneOperationResult>('/api/v1/settings/rclone/import-config', { method: 'POST' })
}

export async function listRcloneRemotes(): Promise<RcloneRemote[]> {
  const response = await request<{ items: RcloneRemote[] }>('/api/v1/settings/rclone/remotes')
  return response.items
}

export async function listBackends(): Promise<BackendTemplate[]> {
  const response = await request<{ items: BackendTemplate[] }>('/api/v1/backends')
  return response.items
}

export async function listRepositories(): Promise<Repository[]> {
  const response = await request<{ items: Repository[] }>('/api/v1/repositories')
  return response.items
}

export async function createRepository(input: RepositoryInput): Promise<Repository> {
  return request<Repository>('/api/v1/repositories', jsonInit('POST', input))
}

export async function removeRepository(id: string): Promise<void> {
  await request<{ deleted: boolean }>(`/api/v1/repositories/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export async function generateScript(input: ScriptGenerateInput): Promise<ScriptGenerateResult> {
  return request<ScriptGenerateResult>('/api/v1/scripts/generate', jsonInit('POST', input))
}

export async function listSnapshots(repositoryId: string): Promise<Snapshot[]> {
  const response = await request<{ items: Snapshot[] }>(`/api/v1/snapshots?repositoryId=${encodeURIComponent(repositoryId)}`)
  return response.items
}

export async function removeSnapshot(repositoryId: string, snapshotId: string): Promise<void> {
  await request<{ deleted: boolean }>(`/api/v1/snapshots/${encodeURIComponent(snapshotId)}?repositoryId=${encodeURIComponent(repositoryId)}`, { method: 'DELETE' })
}

export async function getInsights(): Promise<InsightSummary> {
  return request<InsightSummary>('/api/v1/insights')
}

export async function listNotificationTemplates(): Promise<NotificationTemplate[]> {
  const response = await request<{ items: NotificationTemplate[] }>('/api/v1/notifications/templates')
  return response.items
}

export async function listNotifications(): Promise<NotificationChannel[]> {
  const response = await request<{ items: NotificationChannel[] }>('/api/v1/notifications')
  return response.items
}

export async function createNotification(input: NotificationInput): Promise<NotificationChannel> {
  return request<NotificationChannel>('/api/v1/notifications', jsonInit('POST', input))
}

export async function updateNotification(id: string, input: NotificationInput): Promise<NotificationChannel> {
  return request<NotificationChannel>(`/api/v1/notifications/${encodeURIComponent(id)}`, jsonInit('PUT', input))
}

export async function testNotification(id: string): Promise<void> {
  await request<{ tested: boolean }>(`/api/v1/notifications/${encodeURIComponent(id)}/test`, { method: 'POST' })
}

export async function removeNotification(id: string): Promise<void> {
  await request<{ deleted: boolean }>(`/api/v1/notifications/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export async function getDefaultVariables(): Promise<Record<string, string>> {
  const response = await request<{ variables: Record<string, string> }>('/api/v1/settings/default-variables')
  return response.variables
}

export async function saveDefaultVariables(variables: Record<string, string>): Promise<Record<string, string>> {
  const response = await request<{ variables: Record<string, string> }>('/api/v1/settings/default-variables', jsonInit('PUT', { variables }))
  return response.variables
}

export async function listAppLogs(query = '', limit = 500): Promise<AppLogEntry[]> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (query.trim()) params.set('query', query.trim())
  const response = await request<{ items: AppLogEntry[] }>(`/api/v1/settings/logs?${params.toString()}`)
  return response.items
}

export async function clearAppLogs(): Promise<void> {
  await request<{ cleared: boolean }>('/api/v1/settings/logs', { method: 'DELETE' })
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await request<{ changed: boolean }>('/api/v1/settings/password', jsonInit('POST', { currentPassword, newPassword }))
}

export async function exportConfig(): Promise<ConfigExport> {
  return request<ConfigExport>('/api/v1/settings/export')
}

export async function importConfig(config: ConfigExport): Promise<ConfigImportResult> {
  return request<ConfigImportResult>('/api/v1/settings/import', jsonInit('POST', config))
}

function jsonInit(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { ...init, credentials: 'same-origin' })
  const body = (await response.json().catch(() => ({ success: false }))) as ApiResponse<T>
  if (!response.ok || !body.success || body.data === undefined) {
    throw new ApiRequestError(body.error?.message || `Request failed: ${response.status}`, response.status, body.error?.code || 'REQUEST_FAILED')
  }
  return body.data
}
