<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  ApiRequestError,
  changePassword,
  clearAppLogs,
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
  listAppLogs,
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
  type AppLogEntry,
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
type SettingSection = 'system' | 'rclone' | 'recovery' | 'password' | 'logs'
type RepositoryCheck = { status: 'valid' | 'invalid' | 'checking'; message: string; count?: number; checkedAt?: string }
type SavedGeneratedFile = GeneratedFile & { savedAt: string }

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
const activeSettingsSection = ref<SettingSection>('system')
const logEntries = ref<AppLogEntry[]>([])
const logKeyword = ref('')
const logAutoRefresh = ref(false)
let logRefreshTimer: ReturnType<typeof window.setInterval> | undefined

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
  restoreMode: false,
  sourceDirsText: '',
  restoreSnapshotId: 'latest',
  restoreTargetDir: '/restore',
  restorePathsText: '',
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
  notifyOnSuccess: false,
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
const settingsSections: Array<{ key: SettingSection; labelKey: string }> = [
  { key: 'system', labelKey: 'settingsSystem' },
  { key: 'rclone', labelKey: 'settingsRclone' },
  { key: 'recovery', labelKey: 'settingsRecovery' },
  { key: 'password', labelKey: 'settingsPassword' },
  { key: 'logs', labelKey: 'settingsLogs' }
]

const activeTitle = computed(() => t(`app.${menuItems.find((item) => item.key === activeView.value)?.labelKey || 'dashboard'}`))
const activeBackend = computed(() => backends.value.find((item) => item.id === repositoryForm.backend))
const activeNotificationTemplate = computed(() => notificationTemplates.value.find((item) => item.type === notificationForm.type))
const selectedGeneratedFile = computed(() => generatedFiles.value.find((file) => file.name === selectedGeneratedFileName.value) || generatedFiles.value[0] || null)
const activeSnapshotRepository = computed(() => activeRepositoryModal.value || repositories.value.find((item) => item.id === snapshotForm.repositoryId) || null)
const staleHosts = computed(() => (insights.value?.hosts || []).filter((item) => item.stale))
const scriptTypeOptions = computed(() => builderForm.restoreMode ? ['python', 'js', 'sh', 'ps1'] : ['python', 'js', 'sh', 'ps1', 'cron'])
const languageToggleLabel = computed(() => locale.value === 'zh-CN' ? 'EN' : '中')
const languageToggleTitle = computed(() => locale.value === 'zh-CN' ? t('app.switchToEnglish') : t('app.switchToChinese'))
const themeToggleLabel = computed(() => theme.value === 'dark' ? t('app.lightTheme') : t('app.darkTheme'))
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
const displayedLogEntries = computed(() => {
  const query = logKeyword.value.trim().toLowerCase()
  if (!query) return logEntries.value
  return logEntries.value.filter((item) => `${item.time} ${item.message}`.toLowerCase().includes(query))
})

watch(() => builderForm.restoreMode, (enabled) => {
  if (enabled && builderForm.scriptType === 'cron') {
    builderForm.scriptType = 'python'
  }
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

function toggleLanguage() {
  switchLanguage(locale.value === 'zh-CN' ? 'en-US' : 'zh-CN')
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
  if (!check) return t('app.notChecked')
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
    feedback.value = result.output || t('app.rcloneUpdated')
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
    feedback.value = result.createdEmpty ? t('app.rcloneEmptyCreated') : t('app.rcloneConfigCopied')
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
    feedback.value = t('app.rcloneRemotesRefreshed')
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
    feedback.value = t('app.repositorySaved')
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function deleteRepository(item: Repository) {
  if (!window.confirm(t('app.confirmDeleteRepository'))) return
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

async function detectRepository(repository: Repository) {
  pending.value = `detect-repository-${repository.id}`
  errorMessage.value = ''
  feedback.value = ''
  repositoryChecks.value = {
    ...repositoryChecks.value,
    [repository.id]: { status: 'checking', message: t('app.checking') }
  }
  try {
    const items = await listSnapshots(repository.id)
    repositoryChecks.value = {
      ...repositoryChecks.value,
      [repository.id]: {
        status: 'valid',
        message: t('app.validSnapshotCount', { count: items.length }),
        count: items.length,
        checkedAt: new Date().toISOString()
      }
    }
    feedback.value = t('app.repositoryDetected', { name: repository.name })
  } catch (error) {
    repositoryChecks.value = {
      ...repositoryChecks.value,
      [repository.id]: {
        status: 'invalid',
        message: error instanceof Error ? error.message : String(error),
        checkedAt: new Date().toISOString()
      }
    }
  } finally {
    pending.value = ''
  }
}

async function detectAllRepositories() {
  if (repositories.value.length === 0) return
  pending.value = 'detect-all'
  errorMessage.value = ''
  feedback.value = ''
  const checking = { ...repositoryChecks.value }
  for (const repository of repositories.value) {
    checking[repository.id] = { status: 'checking', message: t('app.checking') }
  }
  repositoryChecks.value = checking
  try {
    const results = await Promise.all(repositories.value.map(async (repository) => {
      try {
        const items = await listSnapshots(repository.id)
        return [repository.id, {
          status: 'valid',
          message: t('app.validSnapshotCount', { count: items.length }),
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
    feedback.value = t('app.repositoryDetectionDone')
  } finally {
    pending.value = ''
  }
}

function setSettingsSection(section: SettingSection) {
  activeSettingsSection.value = section
  if (section === 'logs') {
    void refreshLogs()
  }
  syncLogAutoRefresh()
}

async function refreshLogs() {
  pending.value = 'logs-refresh'
  errorMessage.value = ''
  try {
    logEntries.value = await listAppLogs('', 500)
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function clearRuntimeLogs() {
  pending.value = 'logs-clear'
  errorMessage.value = ''
  feedback.value = ''
  try {
    await clearAppLogs()
    logEntries.value = []
    logKeyword.value = ''
    feedback.value = t('app.logsCleared')
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

function syncLogAutoRefresh() {
  if (logRefreshTimer) {
    window.clearInterval(logRefreshTimer)
    logRefreshTimer = undefined
  }
  if (logAutoRefresh.value && activeSettingsSection.value === 'logs') {
    logRefreshTimer = window.setInterval(() => void refreshLogs(), 5000)
  }
}

async function submitGenerate() {
  pending.value = 'generate'
  errorMessage.value = ''
  feedback.value = ''
  try {
    const includeNotify = !builderForm.restoreMode && builderForm.scriptType !== 'cron' && builderForm.notifyChannelIds.length > 0
    const result = await generateScript({
      repositoryId: builderForm.repositoryId,
      scriptType: builderForm.restoreMode && builderForm.scriptType === 'cron' ? 'python' : builderForm.scriptType,
      secretMode: builderForm.secretMode,
      mode: builderForm.restoreMode ? 'restore' : 'backup',
      sourceDirs: builderForm.restoreMode ? [] : parseList(builderForm.sourceDirsText),
      tags: parseList(builderForm.tagsText),
      cron: !builderForm.restoreMode && builderForm.scriptType === 'cron' ? builderForm.cron : '',
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
      restore: {
        snapshotId: builderForm.restoreSnapshotId,
        targetDir: builderForm.restoreTargetDir,
        includePaths: parseList(builderForm.restorePathsText)
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
  if (!window.confirm(t('app.confirmDeleteSnapshot', { id: snapshot.shortId || snapshot.id }))) return
  pending.value = `delete-snapshot-${snapshot.id}`
  errorMessage.value = ''
  feedback.value = ''
  try {
    await removeSnapshot(snapshotForm.repositoryId, snapshot.id)
    snapshots.value = snapshots.value.filter((item) => item.id !== snapshot.id)
    snapshotPage.value = Math.min(snapshotPage.value, snapshotPageCount.value)
    feedback.value = t('app.snapshotDeleted')
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
    feedback.value = t('app.notificationTestSuccess', { name: item.name })
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    pending.value = ''
  }
}

async function deleteNotification(item: NotificationChannel) {
  if (!window.confirm(t('app.confirmDeleteNotification'))) return
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
      locale: locale.value
    }
    downloadTextFile(`urestic-config-${new Date().toISOString().slice(0, 10)}.json`, JSON.stringify(config, null, 2) + '\n', 'application/json;charset=utf-8')
    feedback.value = t('app.configExported')
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
    if (parsed.formatVersion >= 2 && !window.confirm(t('app.confirmImportConfig'))) return
    const result = await importConfig(parsed)
    await loadData()
    applyClientConfig(parsed)
    feedback.value = t('app.importConfigSuccess', {
      repositoriesCreated: result.repositoriesCreated,
      repositoriesUpdated: result.repositoriesUpdated,
      repositoriesDeleted: result.repositoriesDeleted,
      notificationsCreated: result.notificationsCreated,
      notificationsUpdated: result.notificationsUpdated,
      notificationsDeleted: result.notificationsDeleted,
      defaultVariablesRestored: result.defaultVariablesRestored,
      defaultVariablesDeleted: result.defaultVariablesDeleted,
      rcloneStatus: result.rcloneConfigRestored ? t('app.restored') : result.rcloneConfigRemoved ? t('app.removed') : t('app.unchanged')
    })
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
}

async function submitPasswordChange() {
  pending.value = 'password'
  errorMessage.value = ''
  feedback.value = ''
  if (passwordForm.newPassword !== passwordForm.confirmPassword) {
    pending.value = ''
    errorMessage.value = t('app.passwordMismatch')
    return
  }
  try {
    await changePassword(passwordForm.currentPassword, passwordForm.newPassword)
    passwordForm.currentPassword = ''
    passwordForm.newPassword = ''
    passwordForm.confirmPassword = ''
    authenticated.value = false
    feedback.value = t('app.passwordChanged')
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
  if (logRefreshTimer) {
    window.clearInterval(logRefreshTimer)
  }
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
      <div class="brand-block">
        <button class="logo-mark" type="button" title="Urestic" @click="go('dashboard')">U</button>
        <span class="version-chip">v{{ systemInfo?.version || '260705' }}</span>
      </div>
      <nav class="sidebar-nav">
        <button v-for="item in menuItems" :key="item.key" :class="{ active: activeView === item.key }" type="button" :title="`${t(`app.${item.labelKey}`)} · ${viewPath(item.key)}`" @click="go(item.key)">
          <span>{{ t(`app.${item.labelKey}`) }}</span>
        </button>
      </nav>
      <div class="sidebar-bottom">
        <button class="sidebar-user language-toggle" type="button" :title="languageToggleTitle" @click="toggleLanguage">{{ languageToggleLabel }}</button>
        <button class="sidebar-user logout-button" type="button" :title="`${t('app.logout')} · ${authUser?.username || ''}`" @click="logout">{{ t('app.logout') }}</button>
      </div>
    </aside>

    <main class="workspace">
      <header class="topbar">
        <div>
          <p class="breadcrumb">Urestic / <span>{{ activeTitle }}</span> <b class="path-chip">{{ viewPath(activeView) }}</b></p>
          <h1>{{ activeTitle }}</h1>
        </div>
        <div class="topbar-actions">
          <button class="ghost theme-toggle" type="button" @click="toggleTheme">{{ themeToggleLabel }}</button>
          <button class="ghost" type="button" :disabled="loading" @click="refresh">{{ t('app.refresh') }}</button>
        </div>
      </header>

      <div v-if="feedback || errorMessage" class="message-stack">
        <p v-if="feedback" class="success">{{ feedback }}</p>
        <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
      </div>

      <section v-if="activeView === 'dashboard'" class="dashboard-page">
        <div class="metric-grid">
          <article class="metric"><span>{{ repositories.length }}</span><p>{{ t('app.repositoryConfigs') }}</p></article>
          <article class="metric"><span>{{ insights?.snapshotCount || 0 }}</span><p>{{ t('app.snapshotRecords') }}</p></article>
          <article class="metric"><span>{{ insights?.failures.length || 0 }}</span><p>{{ t('app.queryFailures') }}</p></article>
          <article class="metric"><span>{{ staleHosts.length }}</span><p>{{ t('app.staleHosts48h') }}</p></article>
        </div>
        <div class="panel-grid">
          <article class="card">
            <h3>{{ t('app.recentRepositories') }}</h3>
            <div v-for="repository in repositories.slice(0, 5)" :key="repository.id" class="mini-row">
              <span>{{ repository.name }}</span><b :class="['backend-badge', backendClass(repository.backend)]">{{ repository.backend }}</b>
            </div>
            <p v-if="repositories.length === 0" class="muted">{{ t('app.noRepositories') }}</p>
          </article>
          <article class="card">
            <h3>{{ t('app.needsAttention') }}</h3>
            <div v-for="item in staleHosts.slice(0, 5)" :key="item.name" class="mini-row"><span>{{ item.name }}</span><b class="bad">{{ Math.round(item.ageHours) }}h</b></div>
            <div v-for="item in insights?.failures || []" :key="item.repository" class="mini-row"><span>{{ item.repository }}</span><b class="bad">{{ t('app.queryFailed') }}</b></div>
            <p v-if="staleHosts.length === 0 && (insights?.failures.length || 0) === 0" class="muted">{{ t('app.noAttentionNeeded') }}</p>
          </article>
        </div>
      </section>

      <section v-if="activeView === 'repositories'" class="repositories-page">
        <form class="card repository-form" @submit.prevent="submitRepository">
          <div class="section-title"><p class="eyebrow">backend</p><h2>{{ t('app.newRepository') }}</h2></div>
          <label>{{ t('app.repositoryName') }}<input v-model="repositoryForm.name" required placeholder="joplin-r2" /></label>
          <label>{{ t('app.backend') }}
            <select v-model="repositoryForm.backend">
              <option v-for="backend in backends" :key="backend.id" :value="backend.id">{{ backend.name }}</option>
            </select>
          </label>
          <label>{{ t('app.repoUrl') }}<input v-model="repositoryForm.repoUrl" required /></label>
          <p v-if="activeBackend" class="hint">{{ t('app.repoUrlExamplePrefix') }}{{ activeBackend.repoExample }}{{ t('app.repoUrlExampleSuffix') }} <code>&lt;r2_s3_api&gt;</code>, <code>&lt;bucket&gt;</code>, <code>&lt;prefix&gt;</code>.</p>
          <label>{{ t('app.resticPassword') }}<input v-model="repositoryForm.password" required type="password" /></label>
          <div v-if="activeBackend?.fields.length" class="variable-box">
            <label v-for="field in activeBackend.fields" :key="field">{{ field }}<input v-model="repositoryForm.variables[field]" :type="field.includes('secret') || field.includes('key') ? 'password' : 'text'" /></label>
          </div>
          <label>{{ t('app.description') }}<textarea v-model="repositoryForm.description" rows="3"></textarea></label>
          <button class="primary" type="submit" :disabled="pending === 'repository'">{{ t('app.save') }}</button>
        </form>
        <section class="repo-list">
          <div class="section-toolbar">
            <div><p class="eyebrow">saved</p><h2>{{ t('app.savedRepositories') }}</h2><p class="hint">{{ t('app.repositorySnapshotHint') }}</p></div>
            <button class="ghost" type="button" :disabled="pending === 'detect-all' || repositories.length === 0" @click="detectAllRepositories">{{ pending === 'detect-all' ? t('app.checking') : t('app.checkAll') }}</button>
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
              <div class="card-actions"><button class="ghost" type="button" :disabled="pending === `detect-repository-${repository.id}`" @click.stop="detectRepository(repository)">{{ pending === `detect-repository-${repository.id}` ? t('app.checking') : t('app.detect') }}</button><button class="ghost" type="button" @click.stop="openRepositorySnapshots(repository)">{{ t('app.snapshots') }}</button><button class="ghost" type="button" @click.stop="deleteRepository(repository)">{{ t('app.delete') }}</button></div>
            </article>
            <p v-if="repositories.length === 0" class="empty-state">{{ t('app.noRepositories') }}</p>
          </div>
        </section>
      </section>

      <section v-if="activeView === 'builder'" class="script-generator">
        <form class="config-panel" @submit.prevent="submitGenerate">
          <div class="section-title"><p class="eyebrow">script</p><h2>{{ builderForm.restoreMode ? t('app.generateRestoreScript') : t('app.generateBackupScript') }}</h2><p>{{ builderForm.restoreMode ? t('app.generateRestoreScriptHint') : t('app.generateScriptHint') }}</p></div>
          <label>{{ t('app.targetRepository') }}<select v-model="builderForm.repositoryId" required><option value="" disabled>{{ t('app.selectRepository') }}</option><option v-for="repository in repositories" :key="repository.id" :value="repository.id">{{ repository.name }} ({{ repository.backend }})</option></select></label>
          <div class="field-grid compact">
            <label>{{ t('app.scriptType') }}<select v-model="builderForm.scriptType"><option v-for="type in scriptTypeOptions" :key="type" :value="type">{{ type }}</option></select></label>
            <label>{{ t('app.secretMode') }}<select v-model="builderForm.secretMode"><option value="inline">{{ t('app.inlineReady') }}</option><option value="placeholder">{{ t('app.placeholder') }}</option></select></label>
          </div>
          <label class="checkbox mode-switch"><input v-model="builderForm.restoreMode" type="checkbox" /> {{ t('app.restoreMode') }}</label>
          <p class="hint">{{ builderForm.restoreMode ? t('app.restoreModeHint') : t('app.backupModeHint') }}</p>
          <p v-if="builderForm.secretMode === 'inline'" class="warning">{{ t('app.inlineSecretWarning') }}</p>
          <label v-if="!builderForm.restoreMode">{{ t('app.sourceDirs') }}<textarea v-model="builderForm.sourceDirsText" rows="3" :placeholder="t('app.sourceDirsPlaceholder')"></textarea></label>
          <div v-if="builderForm.restoreMode" class="restore-config-box">
            <div class="field-grid compact">
              <label>{{ t('app.restoreSnapshotId') }}<input v-model="builderForm.restoreSnapshotId" placeholder="latest" /></label>
              <label>{{ t('app.restoreTargetDir') }}<input v-model="builderForm.restoreTargetDir" placeholder="/restore" /></label>
            </div>
            <label>{{ t('app.restoreIncludePaths') }}<textarea v-model="builderForm.restorePathsText" rows="3" :placeholder="t('app.restoreIncludePathsPlaceholder')"></textarea></label>
            <label>{{ t('app.hostName') }}<input v-model="builderForm.host" :placeholder="t('app.restoreHostPlaceholder')" /></label>
            <p class="hint">{{ t('app.restoreLatestFilterHint') }}</p>
          </div>
          <div class="field-grid compact">
            <label>{{ t('app.tags') }}<input v-model="builderForm.tagsText" placeholder="daily,server-a" /></label>
            <label v-if="!builderForm.restoreMode && builderForm.scriptType === 'cron'">{{ t('app.cron') }}<input v-model="builderForm.cron" /></label>
            <p v-if="!builderForm.restoreMode && builderForm.scriptType === 'cron'" class="hint">{{ t('app.cronOnlyHint') }}</p>
          </div>

          <details v-if="!builderForm.restoreMode" class="option-panel">
            <summary>{{ t('app.backupOptions') }}</summary>
            <div class="option-grid">
              <label>{{ t('app.excludeExtensions') }}<input v-model="builderForm.excludeExtensionsText" placeholder="tmp,log,cache" /></label>
              <label>{{ t('app.excludePatterns') }}<input v-model="builderForm.excludePatternsText" placeholder="node_modules,*.bak,/tmp/**" /></label>
              <label>{{ t('app.excludeIfPresent') }}<input v-model="builderForm.excludeIfPresentText" placeholder=".nobackup,CACHEDIR.TAG" /></label>
              <label>{{ t('app.excludeLargerThan') }}<input v-model="builderForm.excludeLargerThan" placeholder="2G, 500M" /></label>
              <label>{{ t('app.hostName') }}<input v-model="builderForm.host" :placeholder="t('app.defaultHostname')" /></label>
              <label>{{ t('app.compression') }}<select v-model="builderForm.compression"><option value="auto">auto</option><option value="off">off</option><option value="max">max</option><option value="">{{ t('app.unspecified') }}</option></select></label>
              <label>{{ t('app.uploadLimit') }}<input v-model.number="builderForm.uploadLimitKB" type="number" min="0" /></label>
              <label>{{ t('app.downloadLimit') }}<input v-model.number="builderForm.downloadLimitKB" type="number" min="0" /></label>
              <label>{{ t('app.readConcurrency') }}<input v-model.number="builderForm.readConcurrency" type="number" min="0" /></label>
            </div>
            <div class="checkbox-grid">
              <label class="checkbox"><input v-model="builderForm.initIfMissing" type="checkbox" /> {{ t('app.initIfMissing') }}</label>
              <label class="checkbox"><input v-model="builderForm.excludeCaches" type="checkbox" /> {{ t('app.excludeCaches') }}</label>
              <label class="checkbox"><input v-model="builderForm.excludeCloudFiles" type="checkbox" /> {{ t('app.excludeCloudFiles') }}</label>
              <label class="checkbox"><input v-model="builderForm.oneFileSystem" type="checkbox" /> {{ t('app.oneFileSystem') }}</label>
              <label class="checkbox"><input v-model="builderForm.useFsSnapshot" type="checkbox" /> Windows VSS</label>
              <label class="checkbox"><input v-model="builderForm.dryRun" type="checkbox" /> {{ t('app.dryRun') }}</label>
            </div>
          </details>

          <details v-if="!builderForm.restoreMode" class="option-panel" open>
            <summary>{{ t('app.retentionAndNotifications') }}</summary>
            <div class="retention-grid">
              <label>keepLast<input v-model.number="builderForm.keepLast" type="number" min="0" /></label>
              <label>keepDaily<input v-model.number="builderForm.keepDaily" type="number" min="0" /></label>
              <label>keepWeekly<input v-model.number="builderForm.keepWeekly" type="number" min="0" /></label>
              <label>keepMonthly<input v-model.number="builderForm.keepMonthly" type="number" min="0" /></label>
              <label>keepYearly<input v-model.number="builderForm.keepYearly" type="number" min="0" /></label>
              <label>keepWithin<input v-model="builderForm.keepWithin" placeholder="30d" /></label>
            </div>
            <p class="hint">{{ t('app.defaultRetentionHint') }}</p>
            <div class="checkbox-grid">
              <label class="checkbox"><input v-model="builderForm.prune" type="checkbox" /> {{ t('app.pruneAfterForget') }}</label>
            </div>
            <div v-if="builderForm.scriptType !== 'cron'" class="notify-options">
              <div class="section-title"><p class="eyebrow">notify</p><h3>{{ t('app.notificationOptions') }}</h3><p>{{ t('app.notificationOptionsHint') }}</p></div>
              <div class="checkbox-grid">
                <label v-for="channel in notifications" :key="channel.id" class="checkbox"><input v-model="builderForm.notifyChannelIds" type="checkbox" :value="channel.id" /> {{ channel.name }} · {{ channel.type }}</label>
              </div>
              <p v-if="notifications.length === 0" class="hint">{{ t('app.addNotificationFirst') }}</p>
              <div class="checkbox-grid">
                <label class="checkbox"><input v-model="builderForm.notifyOnSuccess" type="checkbox" /> {{ t('app.successNotification') }}</label>
                <label class="checkbox"><input v-model="builderForm.notifyOnBackupFailed" type="checkbox" /> {{ t('app.backupFailedNotification') }}</label>
                <label class="checkbox"><input v-model="builderForm.notifyOnPruneFailed" type="checkbox" /> {{ t('app.pruneFailedNotification') }}</label>
              </div>
            </div>
          </details>
          <button class="primary submit-btn" type="submit" :disabled="pending === 'generate' || repositories.length === 0">{{ pending === 'generate' ? t('app.generating') : t('app.generate') }}</button>
        </form>

        <section class="code-panel">
          <textarea v-if="selectedGeneratedFile" class="code-editor" :value="selectedGeneratedFile.content" spellcheck="false" @input="updateGeneratedFileContent(($event.target as HTMLTextAreaElement).value)"></textarea>
          <div v-else class="empty-state">{{ t('app.generatedFileEmptyHint') }}</div>
        </section>

        <section class="generated-list-panel">
          <div class="section-toolbar"><div><p class="eyebrow">generated</p><h2>{{ t('app.generatedScripts') }}</h2></div><div class="action-row"><button v-if="generatedFiles.length" class="ghost" type="button" @click="clearGeneratedFiles">{{ t('app.clear') }}</button><button v-if="generatedFiles.length" class="primary" type="button" @click="downloadGeneratedFiles">{{ t('app.downloadAll') }}</button></div></div>
          <div class="generated-file-grid">
            <article v-for="file in generatedFiles" :key="file.name" :class="['generated-file-card', { active: selectedGeneratedFile?.name === file.name }]">
              <button type="button" @click="selectGeneratedFile(file)"><span>{{ file.name }}</span><small>{{ formatDate(file.savedAt) }}</small></button>
              <button class="ghost" type="button" @click="downloadGeneratedFile(file)">{{ t('app.download') }}</button>
              <button class="ghost" type="button" @click="deleteGeneratedFile(file)">{{ t('app.delete') }}</button>
            </article>
          </div>
          <p v-if="generatedFiles.length === 0" class="empty-state">{{ t('app.noGeneratedFiles') }}</p>
        </section>
      </section>

      <section v-if="activeView === 'notifications'" class="two-column">
        <form class="card" @submit.prevent="submitNotification">
          <div class="section-title"><p class="eyebrow">notify</p><h2>{{ editingNotificationId ? t('app.editNotification') : t('app.createNotification') }}</h2><p>{{ t('app.notificationFormHint') }}</p></div>
          <label>{{ t('app.name') }}<input v-model="notificationForm.name" required /></label>
          <label>{{ t('app.type') }}<select v-model="notificationForm.type"><option v-for="template in notificationTemplates" :key="template.type" :value="template.type">{{ template.name }}</option></select></label>
          <div class="variable-box"><label v-for="field in activeNotificationTemplate?.fields || []" :key="field">{{ field }}<input v-model="notificationForm.settings[field]" :type="field.includes('token') || field === 'password' ? 'password' : 'text'" /></label></div>
          <div class="action-row settings-actions"><button class="primary" type="submit" :disabled="pending === 'notification'">{{ editingNotificationId ? t('app.saveChanges') : t('app.save') }}</button><button v-if="editingNotificationId" class="ghost" type="button" @click="resetNotificationForm">{{ t('app.cancelEdit') }}</button></div>
        </form>
        <section class="stack">
          <article class="card notification-content">
            <div class="section-title"><p class="eyebrow">remote script</p><h2>{{ t('app.notificationContent') }}</h2><p>{{ t('app.notificationContentHint') }}</p></div>
            <div class="notification-events">
              <div><b>backup_success</b><span>{{ t('app.backupSuccessEvent') }}</span></div>
              <div><b>backup_failed</b><span>{{ t('app.backupFailedEvent') }}</span></div>
              <div><b>forget_prune_failed</b><span>{{ t('app.pruneFailedEvent') }}</span></div>
            </div>
            <p class="hint">{{ t('app.notificationPayloadHint') }} <code>{ event, title, details }</code>.</p>
          </article>
          <article v-for="item in notifications" :key="item.id" class="list-card"><div><h3>{{ item.name }}</h3><p>{{ item.type }}</p></div><div class="action-row"><button class="ghost" type="button" @click="editNotification(item)">{{ t('app.edit') }}</button><button class="ghost" type="button" :disabled="pending === `test-notification-${item.id}`" @click="sendTestNotification(item)">{{ pending === `test-notification-${item.id}` ? t('app.testing') : t('app.testSend') }}</button><button class="ghost" type="button" @click="deleteNotification(item)">{{ t('app.delete') }}</button></div></article>
          <p v-if="notifications.length === 0" class="empty-state">{{ t('app.noNotifications') }}</p>
        </section>
      </section>

      <section v-if="activeView === 'settings'" class="settings-page">
        <nav class="settings-section-nav">
          <button v-for="section in settingsSections" :key="section.key" class="ghost" :class="{ active: activeSettingsSection === section.key }" type="button" @click="setSettingsSection(section.key)">{{ t(`app.${section.labelKey}`) }}</button>
        </nav>

        <article v-if="activeSettingsSection === 'system'" class="card settings-panel">
          <div class="section-title"><p class="eyebrow">system</p><h2>{{ t('app.system') }}</h2></div>
          <dl v-if="systemInfo" class="meta">
            <div><dt>Mode</dt><dd>{{ systemInfo.mode }}</dd></div>
            <div><dt>Version</dt><dd>{{ systemInfo.version }}</dd></div>
            <div><dt>Data</dt><dd>{{ systemInfo.dataDir }}</dd></div>
            <div><dt>DB</dt><dd>{{ systemInfo.databasePath }}</dd></div>
          </dl>
        </article>

        <article v-if="activeSettingsSection === 'rclone'" class="card settings-panel">
          <div class="section-title"><p class="eyebrow">rclone</p><h2>{{ t('app.rcloneEnvironment') }}</h2><p>{{ t('app.rcloneEnvironmentHint') }}</p></div>
          <dl v-if="rcloneStatus" class="meta">
            <div><dt>Binary</dt><dd>{{ rcloneStatus.installed ? (rcloneStatus.version || t('app.installed')) : t('app.rcloneNotDetected') }}</dd></div>
            <div><dt>Config</dt><dd>{{ rcloneStatus.configPath }} · {{ rcloneStatus.configExists ? t('app.exists') : t('app.notCreated') }}</dd></div>
            <div><dt>Host Import</dt><dd>{{ rcloneStatus.importPath }} · {{ rcloneStatus.importPathExists ? t('app.mounted') : t('app.notFound') }}</dd></div>
            <div><dt>Cache</dt><dd>{{ rcloneStatus.cacheDir }}</dd></div>
          </dl>
          <div class="action-row settings-actions">
            <button class="ghost" type="button" :disabled="pending === 'rclone-status'" @click="refreshRcloneStatus">{{ pending === 'rclone-status' ? t('app.refreshing') : t('app.refreshStatus') }}</button>
            <button class="ghost" type="button" :disabled="pending === 'rclone-update' || !rcloneStatus?.installed" @click="updateRclone">{{ pending === 'rclone-update' ? t('app.updating') : t('app.updateRclone') }}</button>
            <button class="primary" type="button" :disabled="pending === 'rclone-import'" @click="copyHostRcloneConfig">{{ pending === 'rclone-import' ? t('app.processing') : t('app.copyOrCreateConf') }}</button>
          </div>
          <p class="hint">{{ t('app.rcloneComposeHintPrefix') }} <code>/root/.config/rclone/rclone.conf:/host-rclone/rclone.conf:ro</code>. {{ t('app.rcloneComposeHintSuffix') }} <code>/app/data/rclone/rclone.conf</code>.</p>

          <details class="option-panel">
            <summary>{{ t('app.rcloneConfigItems') }}</summary>
            <div class="section-title"><p>{{ t('app.rcloneConfigItemsHint') }}</p></div>
            <div class="action-row settings-actions"><button class="ghost" type="button" :disabled="pending === 'rclone-remotes'" @click="refreshRcloneRemotes">{{ pending === 'rclone-remotes' ? t('app.refreshing') : t('app.refreshList') }}</button></div>
            <section class="rclone-remote-summary">
              <p>{{ t('app.remoteCount', { count: rcloneRemotes.length }) }}</p>
              <div v-if="rcloneRemotes.length" class="rclone-name-list">
                <code v-for="remote in rcloneRemotes" :key="remote.name">{{ remote.name }}</code>
              </div>
              <p v-if="rcloneRemotes.length === 0" class="empty-state">{{ t('app.noRcloneRemotes') }}</p>
            </section>
          </details>
        </article>

        <article v-if="activeSettingsSection === 'recovery'" class="card settings-panel">
          <div class="section-title"><p class="eyebrow">backup</p><h2>{{ t('app.importExportRecoveryPack') }}</h2><p>{{ t('app.recoveryPackHint') }}</p></div>
          <div class="action-row settings-actions">
            <button class="primary" type="button" :disabled="pending === 'export-config'" @click="exportSettings">{{ pending === 'export-config' ? t('app.exporting') : t('app.exportRecoveryJson') }}</button>
            <button class="ghost" type="button" :disabled="pending === 'import-config'" @click="chooseImportFile">{{ pending === 'import-config' ? t('app.importing') : t('app.importRecoveryJson') }}</button>
            <input ref="importFileInput" class="hidden-file" type="file" accept="application/json,.json" @change="importSettings" />
          </div>
          <p class="warning">{{ t('app.recoveryPackWarning') }}</p>
        </article>

        <form v-if="activeSettingsSection === 'password'" class="card settings-panel" @submit.prevent="submitPasswordChange">
          <h3>{{ t('app.changeWebPassword') }}</h3>
          <div class="field-grid compact">
            <label>{{ t('app.currentPassword') }}<input v-model="passwordForm.currentPassword" type="password" required autocomplete="current-password" /></label>
            <label>{{ t('app.newPassword') }}<input v-model="passwordForm.newPassword" type="password" required minlength="8" autocomplete="new-password" /></label>
            <label>{{ t('app.confirmNewPassword') }}<input v-model="passwordForm.confirmPassword" type="password" required minlength="8" autocomplete="new-password" /></label>
          </div>
          <button class="primary" type="submit" :disabled="pending === 'password'">{{ t('app.saveNewPassword') }}</button>
        </form>

        <article v-if="activeSettingsSection === 'logs'" class="card settings-panel log-panel">
          <div class="section-title"><p class="eyebrow">logs</p><h2>{{ t('app.systemLogs') }}</h2><p>{{ t('app.systemLogsHint') }}</p></div>
          <div class="log-toolbar">
            <label>{{ t('app.keyword') }}<input v-model="logKeyword" :placeholder="t('app.logKeywordPlaceholder')" /></label>
            <label class="checkbox log-autorefresh"><input v-model="logAutoRefresh" type="checkbox" @change="syncLogAutoRefresh" /> {{ t('app.autoRefresh') }}</label>
            <button class="ghost" type="button" :disabled="pending === 'logs-refresh'" @click="refreshLogs">{{ pending === 'logs-refresh' ? t('app.refreshing') : t('app.refreshLogs') }}</button>
            <button class="ghost" type="button" :disabled="pending === 'logs-clear'" @click="clearRuntimeLogs">{{ pending === 'logs-clear' ? t('app.processing') : t('app.clearLogs') }}</button>
          </div>
          <section class="log-viewer">
            <div v-for="entry in displayedLogEntries" :key="entry.id" class="log-line"><time>{{ formatDate(entry.time) }}</time><span>{{ entry.message }}</span></div>
            <p v-if="displayedLogEntries.length === 0" class="empty-state">{{ t('app.noLogs') }}</p>
          </section>
        </article>
      </section>

      <div v-if="activeRepositoryModal" class="modal-backdrop" @click.self="closeRepositorySnapshots">
        <section class="modal-card snapshot-modal">
          <header class="modal-head">
            <div class="section-title"><p class="eyebrow">snapshots</p><h2>{{ activeSnapshotRepository?.name }}</h2><p>{{ activeSnapshotRepository?.repoUrl }}</p></div>
            <div class="action-row"><button class="primary" type="button" :disabled="pending === 'snapshots'" @click="querySnapshots()">{{ pending === 'snapshots' ? t('app.querying') : t('app.refresh') }}</button><button class="ghost" type="button" @click="closeRepositorySnapshots">{{ t('app.close') }}</button></div>
          </header>
          <div class="snapshot-summary">
            <span>{{ t('app.snapshotCountSummary', { filtered: filteredSnapshots.length, total: snapshots.length }) }}</span>
            <span>{{ activeSnapshotRepository?.backend }}</span>
          </div>
          <div class="snapshot-tools">
            <input :value="snapshotSearch" :placeholder="t('app.snapshotSearchPlaceholder')" @input="setSnapshotSearch(($event.target as HTMLInputElement).value)" />
            <div class="action-row"><button class="ghost" type="button" :disabled="snapshotPage <= 1" @click="changeSnapshotPage(-1)">{{ t('app.previousPage') }}</button><span class="page-indicator">{{ snapshotPage }} / {{ snapshotPageCount }}</span><button class="ghost" type="button" :disabled="snapshotPage >= snapshotPageCount" @click="changeSnapshotPage(1)">{{ t('app.nextPage') }}</button></div>
          </div>
          <section class="snapshot-list">
            <article v-for="snapshot in pagedSnapshots" :key="snapshot.id" class="snapshot-row">
              <button class="snapshot-row-main" type="button" @click="toggleSnapshotExpanded(snapshot)">
                <span class="snapshot-id">{{ snapshot.shortId || snapshot.id }}</span>
                <span>{{ formatDate(snapshot.time) }}</span>
                <span>{{ snapshot.hostname || '-' }}</span>
                <span>{{ (snapshot.tags || []).join(', ') || t('app.noTags') }}</span>
                <span>{{ snapshot.paths[0] || '-' }}</span>
              </button>
              <div v-if="snapshotMatchesExpanded(snapshot)" class="snapshot-expanded">
                <div class="card-head snapshot-card-head">
                  <div><p class="eyebrow">snapshot</p><h3>{{ snapshot.shortId || snapshot.id }}</h3></div>
                  <button class="ghost" type="button" :disabled="pending === `delete-snapshot-${snapshot.id}`" @click="deleteSnapshot(snapshot)">{{ pending === `delete-snapshot-${snapshot.id}` ? t('app.deleting') : t('app.delete') }}</button>
                </div>
                <dl class="snapshot-meta">
                  <div><dt>{{ t('app.time') }}</dt><dd>{{ formatDate(snapshot.time) }}</dd></div>
                  <div><dt>Host</dt><dd>{{ snapshot.hostname || '-' }}</dd></div>
                  <div><dt>User</dt><dd>{{ snapshot.username || '-' }}</dd></div>
                  <div><dt>UID/GID</dt><dd>{{ snapshot.uid ?? '-' }} / {{ snapshot.gid ?? '-' }}</dd></div>
                  <div><dt>Tree</dt><dd><code>{{ snapshot.tree || '-' }}</code></dd></div>
                  <div><dt>ID</dt><dd><code>{{ snapshot.id }}</code></dd></div>
                  <div v-if="snapshot.programVersion"><dt>{{ t('app.version') }}</dt><dd>{{ snapshot.programVersion }}</dd></div>
                  <div v-if="snapshot.parent"><dt>Parent</dt><dd><code>{{ snapshot.parent }}</code></dd></div>
                </dl>
                <div class="tag-row"><span v-for="tag in snapshot.tags || []" :key="tag">{{ tag }}</span><span v-if="!snapshot.tags?.length" class="muted">{{ t('app.noTags') }}</span></div>
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
