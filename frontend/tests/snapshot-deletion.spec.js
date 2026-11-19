import { test, expect } from '@playwright/test'

const browserErrors = new WeakMap()
test.beforeEach(async ({ page }) => {
  const errors = []
  browserErrors.set(page, errors)
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
})
test.afterEach(async ({ page }) => {
  expect(browserErrors.get(page)).toEqual([])
})

const repository = { id: 'fixture-repo', name: 'QA repository', backend: 'b2', repoUrl: 'b2:fixture:repo', variables: {}, secretFields: [] }
const seed = Array.from({ length: 25 }, (_, i) => ({ id: (i + 1).toString(16).padStart(64, '0'), shortId: (i + 1).toString(16).padStart(8, '0'), time: '2026-11-01T00:00:00Z', hostname: 'fixture-host', tags: [i < 2 ? 'match' : 'other'], paths: ['/fixture'] }))

async function setup(page, failPrune = false, loseResponse = false) {
  let snapshots = [...seed]
  const submissions = []
  let task = null
  let polls = 0
  await page.route('**/api/v1/**', async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    let data = {}
    if (path.endsWith('/auth/me')) data = { username: 'fixture' }
    else if (path.endsWith('/system/info')) data = { name: 'Urestic fixture', version: 'test' }
    else if (path.endsWith('/settings/rclone')) data = { installed: false }
    else if (path.endsWith('/repositories')) data = { items: [repository] }
    else if (path.endsWith('/insights')) data = { repositoryCount: 1, snapshotCount: snapshots.length, hosts: [], tags: [], paths: [], failures: [] }
    else if (path.endsWith('/snapshots/delete')) {
      const payload = request.postDataJSON()
      submissions.push(payload)
      polls = 0
      task = { id: 'fixture-job', repositoryId: repository.id, status: 'verifying', count: payload.snapshotIds.length, forgotten: false }
      data = task
      if (loseResponse) {
        await route.fulfill({ status: 503, json: { success: false, error: { code: 'FIXTURE_LOST_RESPONSE', message: 'fixture lost response' } } })
        return
      }
    } else if (path.endsWith('/snapshot-deletions')) data = { job: task }
    else if (path.includes('/snapshot-deletions/')) {
      polls++
      task = { ...task, status: polls < 2 ? 'pruning' : failPrune ? 'failed' : 'succeeded', forgotten: true, message: polls >= 2 && failPrune ? '快照已删除，但空间清理失败' : '' }
      if (polls >= 2) snapshots = snapshots.filter(item => !submissions.at(-1).snapshotIds.includes(item.id))
      data = task
    } else if (path.endsWith('/snapshots')) data = { items: snapshots }
    else data = { items: [] }
    await route.fulfill({ status: path.endsWith('/snapshots/delete') ? 202 : 200, json: { success: true, data } })
  })
  await page.goto('/repositories')
  await page.getByRole('button', { name: '快照', exact: true }).click()
  await expect(page.getByRole('button', { name: '清空当前仓库快照', exact: true })).toBeVisible()
  return submissions
}

test('selection spans pages and submits one batch, then refreshes after prune', async ({ page }) => {
  const submissions = await setup(page)
  await page.getByRole('checkbox', { name: '选择快照 00000001', exact: true }).check()
  await page.getByRole('button', { name: '下一页', exact: true }).click()
  await page.getByRole('checkbox', { name: '选择快照 00000015', exact: true }).check()
  await expect(page.getByText('已选 2 个', { exact: true })).toBeVisible()
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: '删除所选', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('统一回收空间')
  await expect(page.getByRole('button', { name: '清空当前仓库快照', exact: true })).toBeDisabled()
  await expect(page.getByRole('status')).toContainText('空间清理完成')
  expect(submissions).toHaveLength(1)
  expect(submissions[0].snapshotIds).toEqual([seed[0].id, seed[20].id])
  await expect(page.getByText('已选 0 个', { exact: true })).toBeVisible()
})

test('filtered select is limited to matches but clear targets entire repository', async ({ page }) => {
  const submissions = await setup(page)
  await page.locator('.snapshot-tools input').fill('match')
  await page.getByRole('button', { name: '全选筛选结果（跨页）', exact: true }).click()
  await expect(page.getByText('已选 2 个', { exact: true })).toBeVisible()
  const dialogs = []
  page.on('dialog', async dialog => {
    dialogs.push(dialog.message())
    await dialog.accept(dialog.type() === 'prompt' ? repository.name : undefined)
  })
  await page.getByRole('button', { name: '清空当前仓库快照', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('空间清理完成')
  expect(dialogs).toHaveLength(2)
  expect(submissions).toHaveLength(1)
  expect(submissions[0]).toMatchObject({ clearAll: true, confirmation: repository.name })
  expect(submissions[0].snapshotIds).toHaveLength(25)
})

test('prune failure is not shown as success and refreshes deleted snapshots', async ({ page }) => {
  await setup(page, true)
  await page.getByRole('checkbox', { name: '选择快照 00000001', exact: true }).check()
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: '删除所选', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('快照已删除，但空间清理失败')
  await expect(page.getByRole('status')).not.toContainText('空间清理完成')
  await expect(page.getByRole('checkbox', { name: '选择快照 00000001', exact: true })).toHaveCount(0)
})

test('task polling resumes after page reload', async ({ page }) => {
  const submissions = await setup(page)
  await page.getByRole('checkbox', { name: '选择快照 00000001', exact: true }).check()
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: '删除所选', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('统一回收空间')
  await page.reload()
  await expect(page.getByRole('status')).toContainText('空间清理完成')
  expect(submissions).toHaveLength(1)
})

test('selecting every snapshot also requires repository-name confirmation', async ({ page }) => {
  const submissions = await setup(page)
  await page.getByRole('button', { name: '全选筛选结果（跨页）', exact: true }).click()
  const dialogs = []
  page.on('dialog', async dialog => { dialogs.push(dialog.type()); await dialog.accept(dialog.type() === 'prompt' ? 'wrong' : undefined) })
  await page.getByRole('button', { name: '删除所选', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('仓库名不匹配')
  expect(dialogs).toEqual(['confirm', 'prompt'])
  expect(submissions).toHaveLength(0)
})

test('lost submission response can be recovered by reopening the repository', async ({ page }) => {
  const submissions = await setup(page, false, true)
  await page.getByRole('checkbox', { name: '选择快照 00000001', exact: true }).check()
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: '删除所选', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('fixture lost response')
  expect(browserErrors.get(page).every(message => message.includes('503'))).toBe(true)
  browserErrors.get(page).length = 0
  await page.getByRole('button', { name: '关闭', exact: true }).click()
  await page.getByRole('button', { name: '快照', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('空间清理完成')
  expect(submissions).toHaveLength(1)
})

test('wrong repository confirmation does not submit deletion', async ({ page }) => {
  const submissions = await setup(page)
  page.on('dialog', dialog => dialog.accept(dialog.type() === 'prompt' ? 'wrong-name' : undefined))
  await page.getByRole('button', { name: '清空当前仓库快照', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('仓库名不匹配')
  expect(submissions).toHaveLength(0)
})
