import { expect, test } from '@playwright/test'

test.describe('/app 控制台（桌面）@desktop', () => {
  test('直接打开 /app 与 /app/overview 深链均可加载（静态托管回退）', async ({ page }) => {
    await page.goto('/app')
    await expect(page.getByRole('heading', { level: 1, name: '总览' })).toBeVisible()

    await page.goto('/app/overview')
    await expect(page.getByRole('heading', { level: 1, name: '总览' })).toBeVisible()
  })

  test('桌面侧边栏可见，导航链接跳转并标记当前页', async ({ page }) => {
    await page.goto('/app')
    const nav = page.getByRole('navigation', { name: '主导航' })
    await expect(nav).toBeVisible()

    const overview = nav.getByRole('link', { name: '总览' })
    await expect(overview).toHaveAttribute('href', '/app/overview')
    await overview.click()

    await expect(page).toHaveURL(/\/app\/overview$/)
    await expect(overview).toHaveAttribute('aria-current', 'page')
  })

  test('折叠按钮收起侧边栏并持久化，刷新后保持，再次点击展开', async ({ page }) => {
    await page.goto('/app')
    // 等待 SPA 水合完成（按钮行为随 AppShell 挂载）
    await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible()

    const sidebar = page.locator('.app-sidebar')
    await expect(sidebar).toBeVisible()
    const expandedWidth = await sidebar.evaluate((el) =>
      Number.parseFloat(getComputedStyle(el).width),
    )
    expect(expandedWidth).toBeGreaterThan(200)

    const collapse = page.getByRole('button', { name: '折叠侧边栏' })
    await expect(collapse).toHaveAttribute('aria-expanded', 'true')
    await collapse.click()

    await expect(page.locator('div[data-sidebar]')).toHaveAttribute('data-sidebar', 'collapsed')
    const expand = page.getByRole('button', { name: '展开侧边栏' })
    await expect(expand).toHaveAttribute('aria-expanded', 'false')
    // 宽度有 0.2s 过渡，轮询等待收窄到折叠宽度（~64px）
    await expect
      .poll(() => sidebar.evaluate((el) => Number.parseFloat(getComputedStyle(el).width)))
      .toBeLessThan(100)
    // 折叠态按设计仅裁剪文字而不移出可访问性树（见 app.css 注释），
    // Playwright 可见性不考虑祖先 overflow 裁剪，故用导航框宽度断言其收起
    const nav = page.getByRole('navigation', { name: '主导航' })
    await expect.poll(async () => (await nav.boundingBox())?.width ?? 0).toBeLessThan(100)

    // 折叠状态写入本地存储，整页刷新后仍保持
    await page.reload()
    const expandAfterReload = page.getByRole('button', { name: '展开侧边栏' })
    await expect(expandAfterReload).toBeVisible()
    await expect(page.locator('div[data-sidebar]')).toHaveAttribute('data-sidebar', 'collapsed')

    await expandAfterReload.click()
    await expect(page.locator('div[data-sidebar]')).toHaveAttribute('data-sidebar', 'expanded')
  })

  test('导航仅包含授权传入的总览入口', async ({ page }) => {
    await page.goto('/app')
    const nav = page.getByRole('navigation', { name: '主导航' })
    await expect(nav.getByRole('link')).toHaveCount(1)
  })

  test('Meta+K 打开命令面板：输入过滤、Enter 执行、Escape 关闭', async ({ page }) => {
    await page.goto('/app')
    // 等待 SPA 水合完成（全局快捷键监听随 AppShell 挂载），再触发键盘
    await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible()

    await page.keyboard.press('Meta+k')
    const dialog = page.getByRole('dialog', { name: '命令面板' })
    await expect(dialog).toBeVisible()
    await expect(dialog.getByRole('combobox')).toBeFocused()

    const input = dialog.getByRole('combobox')
    await input.fill('总览')
    await expect(dialog.getByRole('option', { name: '总览' })).toBeVisible()

    await input.fill('不存在的命令')
    await expect(dialog.getByText('没有匹配的结果')).toBeVisible()

    await input.fill('')
    await input.press('Enter')
    await expect(page).toHaveURL(/\/app\/overview$/)
    await expect(dialog).toBeHidden()

    await page.keyboard.press('Meta+k')
    await expect(dialog).toBeVisible()
    await dialog.press('Escape')
    await expect(dialog).toBeHidden()
  })

  test('外观菜单切换主题立即生效且刷新后可恢复', async ({ page }) => {
    await page.goto('/app')

    await page.getByRole('button', { name: '外观设置' }).click()
    await page.getByRole('menuitemradio', { name: '浅色' }).click()
    await expect(page.locator('html')).not.toHaveClass(/dark/)

    await page.getByRole('button', { name: '外观设置' }).click()
    await page.getByRole('menuitemradio', { name: '深色' }).click()
    await expect(page.locator('html')).toHaveClass(/dark/)

    await page.reload()
    await expect(page.locator('html')).toHaveClass(/dark/)
  })
})
