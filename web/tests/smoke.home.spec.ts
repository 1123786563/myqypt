import { expect, test } from '@playwright/test'

test.describe('JS 启用：完整渲染的落地页', () => {
  test('主标题、正文与三个导航入口可见且指向正确的路由', async ({ page }) => {
    await page.goto('/')

    const heading = page.getByRole('heading', { level: 1 })
    await expect(heading).toBeVisible()
    await expect(heading).toContainText('一个入口')

    const lead = page.getByRole('main').locator('p').first()
    await expect(lead).toBeVisible()
    await expect(lead).toContainText('统一平台')

    const nav = page.getByRole('navigation', { name: '主导航' })
    const products = nav.getByRole('link', { name: '产品', exact: true })
    await expect(products).toBeVisible()
    await expect(products).toHaveAttribute('href', '/products')

    const pricing = nav.getByRole('link', { name: '价格', exact: true })
    await expect(pricing).toBeVisible()
    await expect(pricing).toHaveAttribute('href', '/pricing')

    const consoleEntry = nav.getByRole('link', { name: '进入控制台' })
    await expect(consoleEntry).toBeVisible()
    await expect(consoleEntry).toBeEnabled()
    await expect(consoleEntry).toHaveAttribute('href', '/app')
  })
})

test.describe('JS 禁用：无需 JavaScript 的静态可读性', () => {
  test('主标题、正文、元数据与导航文本均可读', async ({ browser }) => {
    const context = await browser.newContext({ javaScriptEnabled: false })
    const page = await context.newPage()
    try {
      await page.goto('/')

      const heading = page.getByRole('heading', { level: 1 })
      await expect(heading).toBeVisible()
      await expect(heading).toContainText('一个入口')

      const lead = page.getByRole('main').locator('p').first()
      await expect(lead).toBeVisible()
      await expect(lead).toContainText('统一平台')

      const description = page.locator('meta[name="description"]')
      await expect(description).toHaveCount(1)
      await expect(description).toHaveAttribute('content', /.+/)

      const canonical = page.locator('link[rel="canonical"]')
      await expect(canonical).toHaveCount(1)
      await expect(canonical).toHaveAttribute('href', /.+/)

      const nav = page.getByRole('navigation', { name: '主导航' })
      await expect(nav.getByText('产品', { exact: true })).toBeVisible()
      await expect(nav.getByText('价格', { exact: true })).toBeVisible()
      await expect(nav.getByRole('link', { name: '进入控制台' })).toBeVisible()
    } finally {
      await context.close()
    }
  })
})
