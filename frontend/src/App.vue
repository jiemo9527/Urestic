<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  ApiRequestError,
  changePassword,
  createNotification,
  createRepository,
  exportConfig,
  generateScript,
  getCurrentUser,
  getInsights,
  getRcloneStatus,
  getSystemInfo,
  importRcloneConfig,
  importConfig,
  listBackends,
  listNotificationTemplates,
  listNotifications,
  listRcloneRemotes,
  listRepositories,
  listSnapshots,
  loginAdmin,
  logoutAdmin,
  removeNotification,
  removeRepository,
  removeSnapshot,
  testNotification,
  updateRcloneBinary,
  updateNotification,
  type AuthUser,
  type BackendTemplate,
  type ConfigExport,
  type GeneratedFile,
  type InsightSummary,
  type NotificationChannel,
  type NotificationTemplate,
  type Repository,
  type RcloneRemote,
  type RcloneStatus,
  type Snapshot,
  type SystemInfo
} from './api'

type View = 'dashboard' | 'repositories' | 'builder' | 'notifications' | 'settings'
type Theme = 'dark' | 'light'
type RepositoryCheck = { status: 'valid' | 'invalid' | 'checking'; message: string; count?: number; checkedAt?: string }
type SavedGeneratedFile = GeneratedFile & { savedAt: string }

const defaultSourceDirCandidates = 'C:\\Users\\Administrator\\Downloads\\,/www/wwwroot/,/srv/metube/,/root/downloads'

const { t, locale } = useI18n()
const authChecked = ref(false)
const authenticated = ref(false)
const loading = ref(false)
const pending = ref('')
const errorMessage = ref('')
const feedback = ref('')
const activeView = ref<View>(routeToView(window.location.pathname))
const theme = ref<Theme>(localStorage.getItem('urestic.theme') === 'light' ? 'light' : 'dark')

const authUser = ref<AuthUser | null>(null)
const systemInfo = ref<SystemInfo | null>(null)
const rcloneStatus = ref<RcloneStatus | null>(null)
const rcloneRemotes = ref<RcloneRemote[]>([])
const backends = ref<BackendTemplate[]>([])
const repositories = ref<Repository[]>([])
const snapshots = ref<Snapshot[]>([])
const insights = ref<InsightSummary | null>(null)
const notificationTemplates = ref<NotificationTemplate[]>([])
const notifications = ref<NotificationChannel[]>([])
const generatedFiles = ref<SavedGeneratedFile[]>([])
const selectedGeneratedFileName = ref('')
const repositoryChecks = ref<Record<string, RepositoryCheck>>({})
const activeRepositoryModal = ref<Repository | null>(null)
const importFileInput = ref<HTMLInputElement | null>(null)
const editingNotificationId = ref('')
const snapshotSearch = ref('')
const snapshotPage = ref(1)
const expandedSnapshotIds = ref<Set<string>>(new Set())

const loginForm = reactive({ username: 'admin', password: '' })
const repositoryForm = reactive({
  name: '',
  backend: 'r2',
  repoUrl: '',
  password: '',
  description: '',
  variables: {} as Record<string, string>
})
const builderForm = reactive({
  repositoryId: '',
  scriptType: 'python',
  secretMode: 'inline',
  sourceDirsText: '',
  sourceDirCandidatesText: defaultSourceDirCandidates,
  tagsText: '',
  excludeExtensionsText: '',
  excludePatternsText: '',
  excludeIfPresentText: '',
  excludeLargerThan: '',
  cron: '0 2 * * *',
  initIfMissing: true,
  excludeCaches: false,
  excludeCloudFiles: false,
  oneFileSystem: false,
  useFsSnapshot: false,
  compression: 'auto',
  uploadLimitKB: 0,
  downloadLimitKB: 0,
  readConcurrency: 0,
  host: '',
  dryRun: false,
  keepLast: 15,
  keepDaily: 7,
  keepWeekly: 3,
  keepMonthly: 2,
  keepYearly: 0,
  keepWithin: '',
  prune: true,
  notifyChannelIds: [] as string[],
  notifyOnSuccess: true,
  notifyOnBackupFailed: true,
  notifyOnPruneFailed: true
})
const snapshotForm = reactive({ repositoryId: '' })
const notificationForm = reactive({
  name: '',
  type: 'telegram',
  settings: {} as Record<string, string>
})
const passwordForm = reactive({ currentPassword: '', newPassword: '', confirmPassword: '' })

const menuItems: Array<{ key: View; labelKey: string }> = [
  { key: 'dashboard', labelKey: 'dashboard' },
  { key: 'repositories', labelKey: 'repositories' },
  { key: 'builder', labelKey: 'builder' },
  { key: 'notifications', labelKey: 'notifications' },
  { key: 'settings', labelKey: 'settings' }
]

const activeTitle = computed(() => t(`app.${menuItems.find((item) => item.key === activeView.value)?.labelKey || 'dashboard'}`))
const activeBackend = computed(() => backends.value.find((item) => item.id === repositoryForm.backend))
const activeNotificationTemplate = computed(() => notificationTemplates.value.find((item) => item.type === notificationForm.type))
const selectedGeneratedFile = computed(() => generatedFiles.value.find((file) => file.name === selectedGeneratedFileName.value) || generatedFiles.value[0] || null)
const activeSnapshotRepository = computed(() => activeRepositoryModal.value || repositories.value.find((item) => item.id === snapshotForm.repositoryId) || null)
const staleHosts = computed(() => (insights.value?.hosts || []).filter((item) => item.stale))
const sourceDirCandidates = computed(() => parseList(builderForm.sourceDirCandidatesText))
const notifyEvents = computed(() => {
  const events: string[] = []
  if (builderForm.notifyOnSuccess) events.push('backup_success')
  if (builderForm.notifyOnBackupFailed) events.push('backup_failed')
  if (builderForm.notifyOnPruneFailed) events.push('forget_prune_failed')
  return events
})
const filteredSnapshots = computed(() => {
  const query = snapshotSearch.value.trim().toLowerCase()
  if (!query) return snapshots.value
  const terms = query.split(/\s+/).filter(Boolean)
  return snapshots.value.filter((snapshot) => {
    const haystack = [
      snapshot.id,
      snapshot.shortId || '',
      snapshot.time,
      snapshot.tree || '',
      snapshot.hostname || '',
      snapshot.username || '',
      snapshot.programVersion || '',
      snapshot.parent || '',
      ...(snapshot.tags || []),
      ...snapshot.paths
    ].join(' ').toLowerCase()
    return terms.every((term) => haystack.includes(term))
  })
})
const snapshotPageSize = 20
const snapshotPageCount = computed(() => Math.max(1, Math.ceil(filteredSnapshots.value.length / snapshotPageSize)))
const pagedSnapshots = computed(() => {
  const page = Math.min(snapshotPage.value, snapshotPageCount.value)
  const start = (page - 1) * snapshotPageSize
  return filteredSnapshots.value.slice(start, start + snapshotPageSize)
})
function routeToView(path: string): View {
  switch (path.replace(/\/+$/, '') || '/') {
    case '/overview':
    case '/':
      return 'dashboard'
    case '/repositories':
      return 'repositories'
    case '/scripts':
      return 'builder'
    case '/notifications':
      return 'notifications'
    case '/settings':
      return 'settings'
    default:
      return 'dashboard'
  }
}

function viewPath(view: View) {
  switch (view) {
    case 'repositories':
      return '/repositories'
    case 'builder':
      return '/scripts'
    case 'notifications':
      return '/notifications'
    case 'settings':
      return '/settings'
    default:
      return '/overview'
  }
}

function switchLanguage(value: string) {
  locale.value = value
  localStorage.setItem('urestic.locale', value)
}

function go(view: View) {
  activeView.value = view
  feedback.value = ''
  errorMessage.value = ''
  const path = viewPath(view)
  if (window.location.pathname !== path) {
    window.history.pushState({ view }, '', path)
  }
}

function applyTheme(value: Theme) {
  document.documentElement.dataset.theme = value
  localStorage.setItem('urestic.theme', value)
}

function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  applyTheme(theme.value)
}

function parseList(value: string): string[] {
  return Array.from(new Set(value.split(',').map((item) => item.trim()).filter(Boolean)))
}

function addSourceDirCandidate(value: string) {
  const items = parseList(builderForm.sourceDirsText)
  if (!items.includes(value)) {
    items.push(value)
  }
  builderForm.sourceDirsText = items.join(',')
}

function backendClass(backend: string) {
  return `backend-${backend}`
}

function selectGeneratedFile(file: GeneratedFile) {
  selectedGeneratedFileName.value = file.name
}

function formatDate(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function checkClass(item: Repository) {
  const status = repositoryChecks.value[item.id]?.status
  return status ? `check-${status}` : 'check-unknown'
}

function checkText(item: Repository) {
  const check = repositoryChecks.value[item.id]
  if (!check) return '未检测'
  return check.message
}

function loadSavedGeneratedFiles() {
  try {
    const saved = JSON.parse(localStorage.getItem('urestic.generatedFiles') || '[]') as SavedGeneratedFile[]
    generatedFiles.value = saved.filter((file) => file.name && typeof file.content === 'string')
    selectedGeneratedFileName.value = generatedFiles.value[0]?.name || ''
  } catch {
    generatedFiles.value = []
  }
}

function persistGeneratedFiles() {
  localStorage.setItem('urestic.generatedFiles', JSON.stringify(generatedFiles.value))
}

function saveGeneratedFiles(files: GeneratedFile[]) {
  const now = new Date().toISOString()
  const incoming = files.map((file) => ({ ...file, savedAt: now }))
  const rest = generatedFiles.value.filter((file) => !incoming.some((item) => item.name === file.name))
  generatedFiles.value = [...incoming, ...rest]
  selectedGeneratedFileName.value = incoming[0]?.name || generatedFiles.value[0]?.name || ''
  persistGeneratedFiles()
}

function updateGeneratedFileContent(content: string) {
  const name = selectedGeneratedFile.value?.name
  if (!name) return
  generatedFiles.value = generatedFiles.value.map((file) => (file.name === name ? { ...file, content, savedAt: new Date().toISOString() } : file))
  persistGeneratedFiles()
}

function deleteGeneratedFile(file: GeneratedFile) {
  generatedFiles.value = generatedFiles.value.filter((item) => item.name !== file.name)
  if (selectedGeneratedFileName.value === file.name) {
    selectedGeneratedFileName.value = generatedFiles.value[0]?.name || ''
  }
  persistGeneratedFiles()
}

function clearGeneratedFiles() {
  generatedFiles.value = []
  selectedGeneratedFileName.value = ''
  persistGeneratedFiles()
}

function snapshotMatchesExpanded(snapshot: Snapshot) {
  return expandedSnapshotIds.value.has(snapshot.id)
}

function toggleSnapshotExpanded(snapshot: Snapshot) {
  const next = new Set(expandedSnapshotIds.value)
  if (next.has(snapshot.id)) {
    next.delete(snapshot.id)
  } else {
    next.add(snapshot.id)
  }
  expandedSnapshotIds.value = next
}

function setSnapshotSearch(value: string) {
  snapshotSearch.value = value
  snapshotPage.value = 1
}

function changeSnapshotPage(delta: number) {
  snapshotPage.value = Math.min(snapshotPageCount.value, Math.max(1, snapshotPage.value + delta))
}

async function initializeAuth() {
  try {
    authUser.value = await getCurrentUser()
    authenticated.value = true
    await loadData()
  } catch (error) {
    if (error instanceof ApiRequestError && error.status === 401) {
      authenticated.value = false
      return
    }
    throw error
  }
}

async function submitLogin() {
  pending.value = 'login'
  errorMessage.value = ''
  try {
    const result = await loginAdmin(loginForm.username, loginForm.password)
    authUser.value = result.user
    authenticated.value = true
    loginForm.password = ''
    await loadData()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function logout() {
  await logoutAdmin()
  authenticated.value = false
  authUser.value = null
}

async function loadData() {
  loading.value = true
  try {
    const [info, rclone, rcloneRemoteItems, backendItems, repositoryItems, notificationTemplateItems, notificationItems] = await Promise.all([
      getSystemInfo(),
      getRcloneStatus(),
      listRcloneRemotes(),
      listBackends(),
      listRepositories(),
      listNotificationTemplates(),
      listNotifications()
    ])
    systemInfo.value = info
    rcloneStatus.value = rclone
    rcloneRemotes.value = rcloneRemoteItems
    backends.value = backendItems
    repositories.value = repositoryItems
    notificationTemplates.value = notificationTemplateItems
    notifications.value = notificationItems
    builderForm.notifyChannelIds = builderForm.notifyChannelIds.filter((id) => notificationItems.some((item) => item.id === id))
    if (!builderForm.repositoryId && repositoryItems[0]) {
      builderForm.repositoryId = repositoryItems[0].id
    }
  } finally {
    loading.value = false
  }
  void refreshInsights()
}

async function refreshInsights() {
  try {
    insights.value = await getInsights()
  } catch (error) {
    if (!errorMessage.value) {
      errorMessage.value = error instanceof Error ? error.message : String(error)
    }
  }
}

async function refreshRcloneStatus() {
  pending.value = 'rclone-status'
  errorMessage.value = ''
  feedback.value = ''
  try {
    rcloneStatus.value = await getRcloneStatus()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function updateRclone() {
  pending.value = 'rclone-update'
  errorMessage.value = ''
  feedback.value = ''
  try {
    const result = await updateRcloneBinary()
    rcloneStatus.value = result.status
    feedback.value = result.output || 'rclone 已更新。'
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function copyHostRcloneConfig() {
  pending.value = 'rclone-import'
  errorMessage.value = ''
  feedback.value = ''
  try {
    const result = await importRcloneConfig()
    rcloneStatus.value = result.status
    rcloneRemotes.value = await listRcloneRemotes()
    feedback.value = result.createdEmpty ? '未读到宿主机 rclone.conf，已创建空的 Urestic rclone.conf。' : '已复制宿主机 rclone.conf 到 Urestic 数据目录。'
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function refreshRcloneRemotes() {
  pending.value = 'rclone-remotes'
  errorMessage.value = ''
  feedback.value = ''
  try {
    rcloneRemotes.value = await listRcloneRemotes()
    feedback.value = 'rclone remote 列表已刷新。'
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function refresh() {
  errorMessage.value = ''
  feedback.value = ''
  try {
    await loadData()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  }
}

async function submitRepository() {
  pending.value = 'repository'
  errorMessage.value = ''
  feedback.value = ''
  try {
    await createRepository({
      name: repositoryForm.name,
      backend: repositoryForm.backend,
      repoUrl: repositoryForm.repoUrl,
      password: repositoryForm.password,
      variables: { ...repositoryForm.variables },
      description: repositoryForm.description
    })
    repositoryForm.name = ''
    repositoryForm.password = ''
    repositoryForm.description = ''
    repositories.value = await listRepositories()
    feedback.value = '仓库已保存'
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function deleteRepository(item: Repository) {
  if (!window.confirm('确认删除该仓库配置？')) return
  await removeRepository(item.id)
  repositories.value = repositories.value.filter((repository) => repository.id !== item.id)
  if (activeRepositoryModal.value?.id === item.id) {
    activeRepositoryModal.value = null
    snapshots.value = []
  }
}

async function openRepositorySnapshots(repository: Repository) {
  activeRepositoryModal.value = repository
  snapshotSearch.value = ''
  snapshotPage.value = 1
  expandedSnapshotIds.value = new Set()
  await querySnapshots(repository.id)
}

function closeRepositorySnapshots() {
  activeRepositoryModal.value = null
}

async function detectAllRepositories() {
  if (repositories.value.length === 0) return
  pending.value = 'detect-all'
  errorMessage.value = ''
  feedback.value = ''
  const checking = { ...repositoryChecks.value }
  for (const repository of repositories.value) {
    checking[repository.id] = { status: 'checking', message: '检测中...' }
  }
  repositoryChecks.value = checking
  try {
    const results = await Promise.all(repositories.value.map(async (repository) => {
      try {
        const items = await listSnapshots(repository.id)
        return [repository.id, {
          status: 'valid',
          message: `有效，${items.length} 个快照`,
          count: items.length,
          checkedAt: new Date().toISOString()
        } satisfies RepositoryCheck] as const
      } catch (error) {
        return [repository.id, {
          status: 'invalid',
          message: error instanceof Error ? error.message : String(error),
          checkedAt: new Date().toISOString()
        } satisfies RepositoryCheck] as const
      }
    }))
    repositoryChecks.value = { ...repositoryChecks.value, ...Object.fromEntries(results) }
    feedback.value = '仓库检测完成'
  } finally {
    pending.value = ''
  }
}

async function submitGenerate() {
  pending.value = 'generate'
  errorMessage.value = ''
  feedback.value = ''
  try {
    const includeNotify = builderForm.scriptType !== 'cron' && builderForm.notifyChannelIds.length > 0
    const result = await generateScript({
      repositoryId: builderForm.repositoryId,
      scriptType: builderForm.scriptType,
      secretMode: builderForm.secretMode,
      sourceDirs: parseList(builderForm.sourceDirsText),
      tags: parseList(builderForm.tagsText),
      cron: builderForm.scriptType === 'cron' ? builderForm.cron : '',
      options: {
        initIfMissing: builderForm.initIfMissing,
        excludePatterns: parseList(builderForm.excludePatternsText),
        excludeExtensions: parseList(builderForm.excludeExtensionsText),
        excludeIfPresent: parseList(builderForm.excludeIfPresentText),
        excludeLargerThan: builderForm.excludeLargerThan,
        excludeCaches: builderForm.excludeCaches,
        excludeCloudFiles: builderForm.excludeCloudFiles,
        oneFileSystem: builderForm.oneFileSystem,
        useFsSnapshot: builderForm.useFsSnapshot,
        compression: builderForm.compression,
        uploadLimitKB: Number(builderForm.uploadLimitKB) || 0,
        downloadLimitKB: Number(builderForm.downloadLimitKB) || 0,
        readConcurrency: Number(builderForm.readConcurrency) || 0,
        host: builderForm.host,
        dryRun: builderForm.dryRun
      },
      retention: {
        keepLast: Number(builderForm.keepLast) || 0,
        keepDaily: Number(builderForm.keepDaily) || 0,
        keepWeekly: Number(builderForm.keepWeekly) || 0,
        keepMonthly: Number(builderForm.keepMonthly) || 0,
        keepYearly: Number(builderForm.keepYearly) || 0,
        keepWithin: builderForm.keepWithin,
        prune: builderForm.prune
      },
      notify: {
        enabled: includeNotify,
        channel: '',
        channelIds: includeNotify ? [...builderForm.notifyChannelIds] : [],
        events: includeNotify ? notifyEvents.value : []
      }
    })
    saveGeneratedFiles(result.files)
    feedback.value = t('app.copied')
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function querySnapshots(repositoryId = snapshotForm.repositoryId) {
  if (!repositoryId) return
  snapshotForm.repositoryId = repositoryId
  pending.value = 'snapshots'
  errorMessage.value = ''
  feedback.value = ''
  try {
    snapshots.value = await listSnapshots(repositoryId)
    snapshotPage.value = 1
  } catch (error) {
    snapshots.value = []
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function deleteSnapshot(snapshot: Snapshot) {
  if (!snapshotForm.repositoryId) return
  if (!window.confirm(`确认删除快照 ${snapshot.shortId || snapshot.id}？此操作会从仓库忘记该快照。`)) return
  pending.value = `delete-snapshot-${snapshot.id}`
  errorMessage.value = ''
  feedback.value = ''
  try {
    await removeSnapshot(snapshotForm.repositoryId, snapshot.id)
    snapshots.value = snapshots.value.filter((item) => item.id !== snapshot.id)
    snapshotPage.value = Math.min(snapshotPage.value, snapshotPageCount.value)
    feedback.value = '快照已删除'
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function submitNotification() {
  pending.value = 'notification'
  errorMessage.value = ''
  try {
    const input = {
      name: notificationForm.name,
      type: notificationForm.type,
      settings: { ...notificationForm.settings }
    }
    if (editingNotificationId.value) {
      await updateNotification(editingNotificationId.value, input)
    } else {
      await createNotification(input)
    }
    resetNotificationForm()
    notifications.value = await listNotifications()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

function resetNotificationForm() {
  editingNotificationId.value = ''
  notificationForm.name = ''
  notificationForm.type = 'telegram'
  notificationForm.settings = {}
}

function editNotification(item: NotificationChannel) {
  editingNotificationId.value = item.id
  notificationForm.name = item.name
  notificationForm.type = item.type
  notificationForm.settings = { ...item.settings }
}

async function sendTestNotification(item: NotificationChannel) {
  pending.value = `test-notification-${item.id}`
  errorMessage.value = ''
  feedback.value = ''
  try {
    await testNotification(item.id)
    feedback.value = `通知渠道 ${item.name} 测试发送成功`
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function deleteNotification(item: NotificationChannel) {
  if (!window.confirm('确认删除该通知渠道？')) return
  await removeNotification(item.id)
  notifications.value = notifications.value.filter((channel) => channel.id !== item.id)
}

function downloadTextFile(name: string, content: string, type = 'text/plain;charset=utf-8', delay = 0) {
  window.setTimeout(() => {
    const blob = new Blob([content], { type })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = name
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.setTimeout(() => URL.revokeObjectURL(url), 2000)
  }, delay)
}

function downloadGeneratedFile(file: GeneratedFile, delay = 0) {
  const type = file.language === 'json' ? 'application/json;charset=utf-8' : 'text/plain;charset=utf-8'
  downloadTextFile(file.name, file.content, type, delay)
}

function downloadGeneratedFiles() {
  generatedFiles.value.forEach((file, index) => downloadGeneratedFile(file, index * 250))
}

async function exportSettings() {
  pending.value = 'export-config'
  errorMessage.value = ''
  feedback.value = ''
  try {
    const config = await exportConfig()
    config.client = {
      generatedFiles: generatedFiles.value,
      selectedGeneratedFileName: selectedGeneratedFileName.value,
      theme: theme.value,
      locale: locale.value,
      sourceDirCandidatesText: builderForm.sourceDirCandidatesText
    }
    downloadTextFile(`urestic-config-${new Date().toISOString().slice(0, 10)}.json`, JSON.stringify(config, null, 2) + '\n', 'application/json;charset=utf-8')
    feedback.value = '恢复包已导出。文件包含 secret、rclone.conf 和已生成脚本，请按敏感文件保存。'
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

function chooseImportFile() {
  importFileInput.value?.click()
}

async function importSettings(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  pending.value = 'import-config'
  errorMessage.value = ''
  feedback.value = ''
  try {
    const parsed = JSON.parse(await file.text()) as ConfigExport
    if (parsed.formatVersion >= 2 && !window.confirm('导入恢复包会覆盖同名配置，并删除恢复包中不存在的仓库、通知、默认变量和 rclone.conf。确认继续？')) return
    const result = await importConfig(parsed)
    await loadData()
    applyClientConfig(parsed)
    feedback.value = `恢复完成：仓库新增 ${result.repositoriesCreated}，覆盖 ${result.repositoriesUpdated}，删除 ${result.repositoriesDeleted}；通知新增 ${result.notificationsCreated}，覆盖 ${result.notificationsUpdated}，删除 ${result.notificationsDeleted}；默认变量恢复 ${result.defaultVariablesRestored}，删除 ${result.defaultVariablesDeleted}；rclone.conf ${result.rcloneConfigRestored ? '已恢复' : result.rcloneConfigRemoved ? '已移除' : '未变化'}`
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

function applyClientConfig(config: ConfigExport) {
  const client = config.client
  if (!client) return
  if (Array.isArray(client.generatedFiles)) {
    generatedFiles.value = client.generatedFiles.filter((file) => file.name && typeof file.content === 'string').map((file) => ({
      name: file.name,
      language: file.language || 'text',
      content: file.content,
      savedAt: file.savedAt || new Date().toISOString()
    }))
    selectedGeneratedFileName.value = generatedFiles.value.some((file) => file.name === client.selectedGeneratedFileName) ? client.selectedGeneratedFileName || '' : generatedFiles.value[0]?.name || ''
    persistGeneratedFiles()
  }
  if (client.theme === 'dark' || client.theme === 'light') {
    theme.value = client.theme
    applyTheme(theme.value)
  }
  if (client.locale === 'zh-CN' || client.locale === 'en-US') {
    switchLanguage(client.locale)
  }
  if (typeof client.sourceDirCandidatesText === 'string') {
    builderForm.sourceDirCandidatesText = client.sourceDirCandidatesText
  }
}

async function submitPasswordChange() {
  pending.value = 'password'
  errorMessage.value = ''
  feedback.value = ''
  if (passwordForm.newPassword !== passwordForm.confirmPassword) {
    pending.value = ''
    errorMessage.value = '两次输入的新密码不一致'
    return
  }
  try {
    await changePassword(passwordForm.currentPassword, passwordForm.newPassword)
    passwordForm.currentPassword = ''
    passwordForm.newPassword = ''
    passwordForm.confirmPassword = ''
    authenticated.value = false
    feedback.value = '密码已修改，请重新登录'
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

function handlePopState() {
  activeView.value = routeToView(window.location.pathname)
  feedback.value = ''
  errorMessage.value = ''
}

onMounted(async () => {
  applyTheme(theme.value)
  loadSavedGeneratedFiles()
  window.addEventListener('popstate', handlePopState)
  if (window.location.pathname === '/' || routeToView(window.location.pathname) === 'dashboard' && window.location.pathname !== '/overview') {
    window.history.replaceState({ view: activeView.value }, '', viewPath(activeView.value))
  }
  try {
    await initializeAuth()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    authChecked.value = true
  }
})

onUnmounted(() => {
  window.removeEventListener('popstate', handlePopState)
})
</script>

<template>
  <section v-if="!authChecked" class="login-shell">
    <article class="login-card loading-card">
      <div class="logo-mark">U</div>
      <p class="eyebrow">loading</p>
    </article>
  </section>

  <section v-else-if="!authenticated" class="login-shell">
    <form class="login-card" @submit.prevent="submitLogin">
      <div class="logo-mark">U</div>
      <p class="eyebrow">{{ t('app.loginTitle') }}</p>
      <h1>{{ t('app.title') }}</h1>
      <p>{{ t('app.loginHint') }}</p>
      <label>{{ t('app.username') }}<input v-model="loginForm.username" required autocomplete="username" /></label>
      <label>{{ t('app.password') }}<input v-model="loginForm.password" required type="password" autocomplete="current-password" /></label>
      <button class="primary" type="submit" :disabled="pending === 'login'">{{ t('app.login') }}</button>
      <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
    </form>
  </section>

  <div v-else class="app-shell">
    <aside class="sidebar">
      <button class="logo-mark" type="button" title="Urestic" @click="go('dashboard')">U</button>
      <nav class="sidebar-nav">
        <button v-for="item in menuItems" :key="item.key" :class="{ active: activeView === item.key }" type="button" :title="`${t(`app.${item.labelKey}`)} · ${viewPath(item.key)}`" @click="go(item.key)">
          <span>{{ t(`app.${item.labelKey}`).slice(0, 2) }}</span>
        </button>
      </nav>
      <div class="sidebar-bottom">
        <select :value="locale" title="Language" @change="switchLanguage(($event.target as HTMLSelectElement).value)">
          <option value="zh-CN">中</option>
          <option value="en-US">EN</option>
        </select>
        <button class="sidebar-user logout-button" type="button" :title="`${t('app.logout')} · ${authUser?.username || ''}`" @click="logout">退出</button>
      </div>
    </aside>

    <main class="workspace">
      <header class="topbar">
        <div>
          <p class="breadcrumb">Urestic / <span>{{ activeTitle }}</span> <b class="path-chip">{{ viewPath(activeView) }}</b></p>
          <h1>{{ activeTitle }}</h1>
        </div>
        <div class="topbar-actions">
          <button class="ghost theme-toggle" type="button" @click="toggleTheme">{{ theme === 'dark' ? '白色主题' : '深色主题' }}</button>
          <button class="ghost" type="button" :disabled="loading" @click="refresh">{{ t('app.refresh') }}</button>
        </div>
      </header>

      <div v-if="feedback || errorMessage" class="message-stack">
        <p v-if="feedback" class="success">{{ feedback }}</p>
        <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
      </div>

      <section v-if="activeView === 'dashboard'" class="dashboard-page">
        <div class="metric-grid">
          <article class="metric"><span>{{ repositories.length }}</span><p>仓库配置</p></article>
          <article class="metric"><span>{{ insights?.snapshotCount || 0 }}</span><p>快照记录</p></article>
          <article class="metric"><span>{{ insights?.failures.length || 0 }}</span><p>查询失败</p></article>
          <article class="metric"><span>{{ staleHosts.length }}</span><p>48h 未更新主机</p></article>
        </div>
        <div class="panel-grid">
          <article class="card">
            <h3>最近仓库</h3>
            <div v-for="repository in repositories.slice(0, 5)" :key="repository.id" class="mini-row">
              <span>{{ repository.name }}</span><b :class="['backend-badge', backendClass(repository.backend)]">{{ repository.backend }}</b>
            </div>
            <p v-if="repositories.length === 0" class="muted">{{ t('app.noRepositories') }}</p>
          </article>
          <article class="card">
            <h3>需要关注</h3>
            <div v-for="item in staleHosts.slice(0, 5)" :key="item.name" class="mini-row"><span>{{ item.name }}</span><b class="bad">{{ Math.round(item.ageHours) }}h</b></div>
            <div v-for="item in insights?.failures || []" :key="item.repository" class="mini-row"><span>{{ item.repository }}</span><b class="bad">查询失败</b></div>
            <p v-if="staleHosts.length === 0 && (insights?.failures.length || 0) === 0" class="muted">暂无需要关注的仓库。</p>
          </article>
        </div>
      </section>

      <section v-if="activeView === 'repositories'" class="repositories-page">
        <form class="card repository-form" @submit.prevent="submitRepository">
          <div class="section-title"><p class="eyebrow">backend</p><h2>新建仓库</h2></div>
          <label>{{ t('app.repositoryName') }}<input v-model="repositoryForm.name" required placeholder="joplin-r2" /></label>
          <label>{{ t('app.backend') }}
            <select v-model="repositoryForm.backend">
              <option v-for="backend in backends" :key="backend.id" :value="backend.id">{{ backend.name }}</option>
            </select>
          </label>
          <label>{{ t('app.repoUrl') }}<input v-model="repositoryForm.repoUrl" required /></label>
          <p v-if="activeBackend" class="hint">示例：{{ activeBackend.repoExample }}。脚本生成会替换 <code>&lt;r2_s3_api&gt;</code>、<code>&lt;bucket&gt;</code>、<code>&lt;prefix&gt;</code> 等占位符。</p>
          <label>{{ t('app.resticPassword') }}<input v-model="repositoryForm.password" required type="password" /></label>
          <div v-if="activeBackend?.fields.length" class="variable-box">
            <label v-for="field in activeBackend.fields" :key="field">{{ field }}<input v-model="repositoryForm.variables[field]" :type="field.includes('secret') || field.includes('key') ? 'password' : 'text'" /></label>
          </div>
          <label>{{ t('app.description') }}<textarea v-model="repositoryForm.description" rows="3"></textarea></label>
          <button class="primary" type="submit" :disabled="pending === 'repository'">{{ t('app.save') }}</button>
        </form>
        <section class="repo-list">
          <div class="section-toolbar">
            <div><p class="eyebrow">saved</p><h2>已创建仓库</h2><p class="hint">双击仓库卡片打开 snapshots 操作。</p></div>
            <button class="ghost" type="button" :disabled="pending === 'detect-all' || repositories.length === 0" @click="detectAllRepositories">{{ pending === 'detect-all' ? '检测中...' : '检测全部' }}</button>
          </div>
          <div class="repo-grid">
            <article v-for="repository in repositories" :key="repository.id" class="repo-card" @dblclick="openRepositorySnapshots(repository)">
              <div class="card-head">
                <span :class="['backend-badge', backendClass(repository.backend)]">{{ repository.backend }}</span>
                <span :class="['status-dot', checkClass(repository)]"></span>
              </div>
              <h3>{{ repository.name }}</h3>
              <p class="repo-url">{{ repository.repoUrl }}</p>
              <p :class="['check-line', checkClass(repository)]">{{ checkText(repository) }}</p>
              <p v-if="repository.description" class="muted">{{ repository.description }}</p>
              <div class="card-actions"><button class="ghost" type="button" @click.stop="openRepositorySnapshots(repository)">快照</button><button class="ghost" type="button" @click.stop="deleteRepository(repository)">{{ t('app.delete') }}</button></div>
            </article>
            <p v-if="repositories.length === 0" class="empty-state">{{ t('app.noRepositories') }}</p>
          </div>
        </section>
      </section>

      <section v-if="activeView === 'builder'" class="script-generator">
        <form class="config-panel" @submit.prevent="submitGenerate">
          <div class="section-title"><p class="eyebrow">script</p><h2>生成备份脚本</h2><p>默认即插即用，会在生成时写入凭据；如需安全占位文件，请切换为占位符模式。</p></div>
          <label>目标仓库<select v-model="builderForm.repositoryId" required><option value="" disabled>选择仓库</option><option v-for="repository in repositories" :key="repository.id" :value="repository.id">{{ repository.name }} ({{ repository.backend }})</option></select></label>
          <div class="field-grid compact">
            <label>{{ t('app.scriptType') }}<select v-model="builderForm.scriptType"><option>python</option><option>js</option><option>sh</option><option>ps1</option><option>cron</option></select></label>
            <label>凭证模式<select v-model="builderForm.secretMode"><option value="inline">inline 即插即用</option><option value="placeholder">占位符</option></select></label>
          </div>
          <p v-if="builderForm.secretMode === 'inline'" class="warning">inline 会把仓库密码、云存储 key、通知 token 写进生成文件。文件本身必须按敏感文件处理。</p>
          <label>{{ t('app.sourceDirs') }}<textarea v-model="builderForm.sourceDirsText" rows="3" placeholder="留空不使用默认值，例如 /root/joplin,/var/www"></textarea></label>
          <div class="source-candidate-box">
            <label>备份源候选<input v-model="builderForm.sourceDirCandidatesText" /></label>
            <div class="source-candidates">
              <button v-for="candidate in sourceDirCandidates" :key="candidate" class="ghost" type="button" @click="addSourceDirCandidate(candidate)">{{ candidate }}</button>
            </div>
          </div>
          <div class="field-grid compact">
            <label>{{ t('app.tags') }}<input v-model="builderForm.tagsText" placeholder="daily,server-a" /></label>
            <label v-if="builderForm.scriptType === 'cron'">{{ t('app.cron') }}<input v-model="builderForm.cron" /></label>
            <p v-if="builderForm.scriptType === 'cron'" class="hint">cron 类型只生成一行 crontab 命令，不生成脚本、JSON 或通知配置。</p>
          </div>

          <details class="option-panel" open>
            <summary>备份选项</summary>
            <div class="option-grid">
              <label>排除文件类型<input v-model="builderForm.excludeExtensionsText" placeholder="tmp,log,cache" /></label>
              <label>排除路径/模式<input v-model="builderForm.excludePatternsText" placeholder="node_modules,*.bak,/tmp/**" /></label>
              <label>目录标记排除<input v-model="builderForm.excludeIfPresentText" placeholder=".nobackup,CACHEDIR.TAG" /></label>
              <label>排除大文件<input v-model="builderForm.excludeLargerThan" placeholder="2G, 500M" /></label>
              <label>Host 名称<input v-model="builderForm.host" placeholder="默认 hostname" /></label>
              <label>压缩<select v-model="builderForm.compression"><option value="auto">auto</option><option value="off">off</option><option value="max">max</option><option value="">不指定</option></select></label>
              <label>上传限速 KiB/s<input v-model.number="builderForm.uploadLimitKB" type="number" min="0" /></label>
              <label>下载限速 KiB/s<input v-model.number="builderForm.downloadLimitKB" type="number" min="0" /></label>
              <label>读取并发<input v-model.number="builderForm.readConcurrency" type="number" min="0" /></label>
            </div>
            <div class="checkbox-grid">
              <label class="checkbox"><input v-model="builderForm.initIfMissing" type="checkbox" /> 仓库不存在时自动 restic init</label>
              <label class="checkbox"><input v-model="builderForm.excludeCaches" type="checkbox" /> 排除缓存目录</label>
              <label class="checkbox"><input v-model="builderForm.excludeCloudFiles" type="checkbox" /> 排除云盘占位文件</label>
              <label class="checkbox"><input v-model="builderForm.oneFileSystem" type="checkbox" /> 不跨文件系统</label>
              <label class="checkbox"><input v-model="builderForm.useFsSnapshot" type="checkbox" /> Windows VSS</label>
              <label class="checkbox"><input v-model="builderForm.dryRun" type="checkbox" /> dry-run 预演</label>
            </div>
          </details>

          <details class="option-panel" open>
            <summary>保留策略与通知</summary>
            <div class="retention-grid">
              <label>keepLast<input v-model.number="builderForm.keepLast" type="number" min="0" /></label>
              <label>keepDaily<input v-model.number="builderForm.keepDaily" type="number" min="0" /></label>
              <label>keepWeekly<input v-model.number="builderForm.keepWeekly" type="number" min="0" /></label>
              <label>keepMonthly<input v-model.number="builderForm.keepMonthly" type="number" min="0" /></label>
              <label>keepYearly<input v-model.number="builderForm.keepYearly" type="number" min="0" /></label>
              <label>keepWithin<input v-model="builderForm.keepWithin" placeholder="30d" /></label>
            </div>
            <p class="hint">默认保留最近 15 个快照，允许当天多次；再保留最近 7 天、3 周、2 月的代表快照。</p>
            <div class="checkbox-grid">
              <label class="checkbox"><input v-model="builderForm.prune" type="checkbox" /> prune 后回收空间</label>
            </div>
            <div v-if="builderForm.scriptType !== 'cron'" class="notify-options">
              <div class="section-title"><p class="eyebrow">notify</p><h3>通知选项</h3><p>勾选的通知渠道会写入生成脚本配置；通知由各服务器运行脚本时发送。</p></div>
              <div class="checkbox-grid">
                <label v-for="channel in notifications" :key="channel.id" class="checkbox"><input v-model="builderForm.notifyChannelIds" type="checkbox" :value="channel.id" /> {{ channel.name }} · {{ channel.type }}</label>
              </div>
              <p v-if="notifications.length === 0" class="hint">还没有通知渠道，请先到通知页添加。</p>
              <div class="checkbox-grid">
                <label class="checkbox"><input v-model="builderForm.notifyOnSuccess" type="checkbox" /> 成功通知</label>
                <label class="checkbox"><input v-model="builderForm.notifyOnBackupFailed" type="checkbox" /> 备份异常通知</label>
                <label class="checkbox"><input v-model="builderForm.notifyOnPruneFailed" type="checkbox" /> forget/prune 异常通知</label>
              </div>
            </div>
          </details>
          <button class="primary submit-btn" type="submit" :disabled="pending === 'generate' || repositories.length === 0">{{ pending === 'generate' ? '生成中...' : t('app.generate') }}</button>
        </form>

        <section class="code-panel">
          <textarea v-if="selectedGeneratedFile" class="code-editor" :value="selectedGeneratedFile.content" spellcheck="false" @input="updateGeneratedFileContent(($event.target as HTMLTextAreaElement).value)"></textarea>
          <div v-else class="empty-state">填写左侧配置并生成，此处显示当前选中文件。</div>
        </section>

        <section class="generated-list-panel">
          <div class="section-toolbar"><div><p class="eyebrow">generated</p><h2>已生成脚本列表</h2></div><div class="action-row"><button v-if="generatedFiles.length" class="ghost" type="button" @click="clearGeneratedFiles">清空</button><button v-if="generatedFiles.length" class="primary" type="button" @click="downloadGeneratedFiles">下载全部</button></div></div>
          <div class="generated-file-grid">
            <article v-for="file in generatedFiles" :key="file.name" :class="['generated-file-card', { active: selectedGeneratedFile?.name === file.name }]">
              <button type="button" @click="selectGeneratedFile(file)"><span>{{ file.name }}</span><small>{{ formatDate(file.savedAt) }}</small></button>
              <button class="ghost" type="button" @click="downloadGeneratedFile(file)">下载</button>
              <button class="ghost" type="button" @click="deleteGeneratedFile(file)">删除</button>
            </article>
          </div>
          <p v-if="generatedFiles.length === 0" class="empty-state">暂无已生成文件。</p>
        </section>
      </section>

      <section v-if="activeView === 'notifications'" class="two-column">
        <form class="card" @submit.prevent="submitNotification">
          <div class="section-title"><p class="eyebrow">notify</p><h2>{{ editingNotificationId ? '修改通知渠道' : t('app.createNotification') }}</h2><p>是否写入脚本只由脚本管理页的通知渠道勾选决定；测试会立即发送一条测试消息。</p></div>
          <label>名称<input v-model="notificationForm.name" required placeholder="telegram-main" /></label>
          <label>类型<select v-model="notificationForm.type"><option v-for="template in notificationTemplates" :key="template.type" :value="template.type">{{ template.name }}</option></select></label>
          <div class="variable-box"><label v-for="field in activeNotificationTemplate?.fields || []" :key="field">{{ field }}<input v-model="notificationForm.settings[field]" :type="field.includes('token') || field === 'password' ? 'password' : 'text'" /></label></div>
          <div class="action-row settings-actions"><button class="primary" type="submit" :disabled="pending === 'notification'">{{ editingNotificationId ? '保存修改' : t('app.save') }}</button><button v-if="editingNotificationId" class="ghost" type="button" @click="resetNotificationForm">取消编辑</button></div>
        </form>
        <section class="stack">
          <article class="card notification-content">
            <div class="section-title"><p class="eyebrow">remote script</p><h2>通知内容</h2><p>通知由各服务器上运行的生成脚本发出，不是 Urestic Web 服务端主动发送。脚本管理页勾选的渠道会写入配置。</p></div>
            <div class="notification-events">
              <div><b>backup_success</b><span>备份完成。正文包含仓库、主机、备份源、标签、耗时。</span></div>
              <div><b>backup_failed</b><span>初始化、解锁或 backup 失败。正文包含仓库、主机、备份源、标签、错误。</span></div>
              <div><b>forget_prune_failed</b><span>备份成功但保留清理失败。正文包含仓库、主机、备份源、标签、错误。</span></div>
            </div>
            <p class="hint">Telegram 使用纯文本：标题 + 换行 + 正文；Email 使用标题作为 Subject、正文作为 Body；Webhook POST JSON：<code>{ event, title, details }</code>。</p>
          </article>
          <article v-for="item in notifications" :key="item.id" class="list-card"><div><h3>{{ item.name }}</h3><p>{{ item.type }}</p></div><div class="action-row"><button class="ghost" type="button" @click="editNotification(item)">修改</button><button class="ghost" type="button" :disabled="pending === `test-notification-${item.id}`" @click="sendTestNotification(item)">{{ pending === `test-notification-${item.id}` ? '测试中...' : '测试有效' }}</button><button class="ghost" type="button" @click="deleteNotification(item)">{{ t('app.delete') }}</button></div></article>
          <p v-if="notifications.length === 0" class="empty-state">{{ t('app.noNotifications') }}</p>
        </section>
      </section>

      <section v-if="activeView === 'settings'" class="settings-page">
        <article class="card">
          <div class="section-title"><p class="eyebrow">system</p><h2>系统</h2></div>
          <dl v-if="systemInfo" class="meta">
            <div><dt>Mode</dt><dd>{{ systemInfo.mode }}</dd></div>
            <div><dt>Version</dt><dd>{{ systemInfo.version }}</dd></div>
            <div><dt>Data</dt><dd>{{ systemInfo.dataDir }}</dd></div>
            <div><dt>DB</dt><dd>{{ systemInfo.databasePath }}</dd></div>
          </dl>
        </article>
        <article class="card settings-panel">
          <div class="section-title"><p class="eyebrow">rclone</p><h2>rclone 环境</h2><p>rclone 可选；Urestic 使用隔离配置，不默认读取宿主机 rclone.conf。</p></div>
          <dl v-if="rcloneStatus" class="meta">
            <div><dt>Binary</dt><dd>{{ rcloneStatus.installed ? (rcloneStatus.version || '已安装') : '未检测到 rclone' }}</dd></div>
            <div><dt>Config</dt><dd>{{ rcloneStatus.configPath }} · {{ rcloneStatus.configExists ? '已存在' : '未创建' }}</dd></div>
            <div><dt>Host Import</dt><dd>{{ rcloneStatus.importPath }} · {{ rcloneStatus.importPathExists ? '已挂载' : '未找到' }}</dd></div>
            <div><dt>Cache</dt><dd>{{ rcloneStatus.cacheDir }}</dd></div>
          </dl>
          <div class="action-row settings-actions">
            <button class="ghost" type="button" :disabled="pending === 'rclone-status'" @click="refreshRcloneStatus">{{ pending === 'rclone-status' ? '刷新中...' : '刷新状态' }}</button>
            <button class="ghost" type="button" :disabled="pending === 'rclone-update' || !rcloneStatus?.installed" @click="updateRclone">{{ pending === 'rclone-update' ? '更新中...' : '更新 rclone' }}</button>
            <button class="primary" type="button" :disabled="pending === 'rclone-import'" @click="copyHostRcloneConfig">{{ pending === 'rclone-import' ? '处理中...' : '复制/新建 conf' }}</button>
          </div>
          <p class="hint">Compose 默认挂载 <code>/root/.config/rclone/rclone.conf:/host-rclone/rclone.conf:ro</code>。如果导入源不存在、是目录或空文件，会创建空的 <code>/app/data/rclone/rclone.conf</code>。</p>
        </article>
        <article class="card settings-panel">
          <div class="section-title"><p class="eyebrow">rclone config</p><h2>rclone 配置项</h2><p>只读展示当前 Urestic rclone.conf 中的 remote 数量和名称。</p></div>
          <div class="action-row settings-actions"><button class="ghost" type="button" :disabled="pending === 'rclone-remotes'" @click="refreshRcloneRemotes">{{ pending === 'rclone-remotes' ? '刷新中...' : '刷新列表' }}</button></div>
          <section class="rclone-remote-summary">
            <p>共 {{ rcloneRemotes.length }} 个 remote</p>
            <div v-if="rcloneRemotes.length" class="rclone-name-list">
              <code v-for="remote in rcloneRemotes" :key="remote.name">{{ remote.name }}</code>
            </div>
            <p v-if="rcloneRemotes.length === 0" class="empty-state">暂无 rclone remote。可先复制/新建 conf，或在容器内使用 rclone config 管理。</p>
          </section>
        </article>
        <article class="card settings-panel">
          <div class="section-title"><p class="eyebrow">backup</p><h2>导入 / 导出恢复包</h2><p>导出包含仓库、通知、默认变量、rclone.conf、已生成脚本和前端偏好；导入会按恢复包覆盖同名项并删除多余项。</p></div>
          <div class="action-row settings-actions">
            <button class="primary" type="button" :disabled="pending === 'export-config'" @click="exportSettings">{{ pending === 'export-config' ? '导出中...' : '导出恢复包 JSON' }}</button>
            <button class="ghost" type="button" :disabled="pending === 'import-config'" @click="chooseImportFile">{{ pending === 'import-config' ? '导入中...' : '导入恢复包 JSON' }}</button>
            <input ref="importFileInput" class="hidden-file" type="file" accept="application/json,.json" @change="importSettings" />
          </div>
          <p class="warning">恢复包等同于明文凭据备份，且导入会覆盖当前配置；不要上传到公开仓库或发给不可信对象。</p>
        </article>
        <form class="card settings-panel" @submit.prevent="submitPasswordChange">
          <h3>修改 WebUI 密码</h3>
          <div class="field-grid compact">
            <label>当前密码<input v-model="passwordForm.currentPassword" type="password" required autocomplete="current-password" /></label>
            <label>新密码<input v-model="passwordForm.newPassword" type="password" required minlength="8" autocomplete="new-password" /></label>
            <label>再次输入新密码<input v-model="passwordForm.confirmPassword" type="password" required minlength="8" autocomplete="new-password" /></label>
          </div>
          <button class="primary" type="submit" :disabled="pending === 'password'">保存新密码</button>
        </form>
      </section>

      <div v-if="activeRepositoryModal" class="modal-backdrop" @click.self="closeRepositorySnapshots">
        <section class="modal-card snapshot-modal">
          <header class="modal-head">
            <div class="section-title"><p class="eyebrow">snapshots</p><h2>{{ activeSnapshotRepository?.name }}</h2><p>{{ activeSnapshotRepository?.repoUrl }}</p></div>
            <div class="action-row"><button class="primary" type="button" :disabled="pending === 'snapshots'" @click="querySnapshots()">{{ pending === 'snapshots' ? '查询中...' : t('app.refresh') }}</button><button class="ghost" type="button" @click="closeRepositorySnapshots">关闭</button></div>
          </header>
          <div class="snapshot-summary">
            <span>{{ filteredSnapshots.length }} / {{ snapshots.length }} 个快照</span>
            <span>{{ activeSnapshotRepository?.backend }}</span>
          </div>
          <div class="snapshot-tools">
            <input :value="snapshotSearch" placeholder="搜索 ID / host / user / tag / path / 时间" @input="setSnapshotSearch(($event.target as HTMLInputElement).value)" />
            <div class="action-row"><button class="ghost" type="button" :disabled="snapshotPage <= 1" @click="changeSnapshotPage(-1)">上一页</button><span class="page-indicator">{{ snapshotPage }} / {{ snapshotPageCount }}</span><button class="ghost" type="button" :disabled="snapshotPage >= snapshotPageCount" @click="changeSnapshotPage(1)">下一页</button></div>
          </div>
          <section class="snapshot-list">
            <article v-for="snapshot in pagedSnapshots" :key="snapshot.id" class="snapshot-row">
              <button class="snapshot-row-main" type="button" @click="toggleSnapshotExpanded(snapshot)">
                <span class="snapshot-id">{{ snapshot.shortId || snapshot.id }}</span>
                <span>{{ formatDate(snapshot.time) }}</span>
                <span>{{ snapshot.hostname || '-' }}</span>
                <span>{{ (snapshot.tags || []).join(', ') || '无标签' }}</span>
                <span>{{ snapshot.paths[0] || '-' }}</span>
              </button>
              <div v-if="snapshotMatchesExpanded(snapshot)" class="snapshot-expanded">
                <div class="card-head snapshot-card-head">
                  <div><p class="eyebrow">snapshot</p><h3>{{ snapshot.shortId || snapshot.id }}</h3></div>
                  <button class="ghost" type="button" :disabled="pending === `delete-snapshot-${snapshot.id}`" @click="deleteSnapshot(snapshot)">{{ pending === `delete-snapshot-${snapshot.id}` ? '删除中...' : t('app.delete') }}</button>
                </div>
                <dl class="snapshot-meta">
                  <div><dt>时间</dt><dd>{{ formatDate(snapshot.time) }}</dd></div>
                  <div><dt>Host</dt><dd>{{ snapshot.hostname || '-' }}</dd></div>
                  <div><dt>User</dt><dd>{{ snapshot.username || '-' }}</dd></div>
                  <div><dt>UID/GID</dt><dd>{{ snapshot.uid ?? '-' }} / {{ snapshot.gid ?? '-' }}</dd></div>
                  <div><dt>Tree</dt><dd><code>{{ snapshot.tree || '-' }}</code></dd></div>
                  <div><dt>ID</dt><dd><code>{{ snapshot.id }}</code></dd></div>
                  <div v-if="snapshot.programVersion"><dt>版本</dt><dd>{{ snapshot.programVersion }}</dd></div>
                  <div v-if="snapshot.parent"><dt>Parent</dt><dd><code>{{ snapshot.parent }}</code></dd></div>
                </dl>
                <div class="tag-row"><span v-for="tag in snapshot.tags || []" :key="tag">{{ tag }}</span><span v-if="!snapshot.tags?.length" class="muted">无标签</span></div>
                <div class="path-list"><code v-for="path in snapshot.paths" :key="path">{{ path }}</code></div>
              </div>
            </article>
          </section>
          <p v-if="filteredSnapshots.length === 0 && pending !== 'snapshots'" class="empty-state">{{ t('app.noSnapshots') }}</p>
        </section>
      </div>
    </main>
  </div>
</template>
