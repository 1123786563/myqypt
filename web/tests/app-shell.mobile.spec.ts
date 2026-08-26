import { expect, test } from '@playwright/test'

test.describe('/app 控制台（移动）@mobile', () => {
  test('深链 /app/overview 在移动视口可加载（静态托管回退）', async ({ page }) => {
    await page.goto('/app/overview')
    await expect(page.getByRole('heading', { level: 1, name: '总览' })).toBeVisible()
  })

  test('菜单按钮打开 Sheet，导航后关闭', async ({ page }) => {
    await page.goto('/app')

    const menuButton = page.getByRole('button', { name: '打开导航菜单' })
    await expect(menuButton).toBeVisible()
    // 桌面侧边栏在移动视口隐藏
    await expect(page.getByRole('navigation', { name: '主导航' })).toBeHidden()

    await menuButton.click()
    const sheet = page.getByRole('dialog', { name: '导航菜单' })
    await expect(sheet).toBeVisible()

    const overview = sheet.getByRole('link', { name: '总览' })
    await expect(overview).toBeVisible()
    await overview.click()

    await expect(page).toHaveURL(/\/app\/overview$/)
    await expect(sheet).toBeHidden()
  })

  test('Escape 关闭 Sheet 并把焦点还给菜单按钮', async ({ page }) => {
    await page.goto('/app')

    const menuButton = page.getByRole('button', { name: '打开导航菜单' })
    await menuButton.click()
    const sheet = page.getByRole('dialog', { name: '导航菜单' })
    await expect(sheet).toBeVisible()

    await sheet.press('Escape')
    await expect(sheet).toBeHidden()
    await expect(menuButton).toBeFocused()
  })

  test('减少动态效果偏好下 Sheet 仍可正常开关', async ({ page }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.goto('/app')

    const menuButton = page.getByRole('button', { name: '打开导航菜单' })
    await menuButton.click()
    const sheet = page.getByRole('dialog', { name: '导航菜单' })
    await expect(sheet).toBeVisible()

    // prefers-reduced-motion 时 app.css 规则禁用进入动画
    const animationName = await sheet.evaluate((el) => getComputedStyle(el).animationName)
    expect(animationName).toBe('none')

    await sheet.press('Escape')
    await expect(sheet).toBeHidden()
    await expect(menuButton).toBeFocused()
  })
})
