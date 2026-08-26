import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { beforeEach, describe, expect, it } from 'vitest'

import { AppShell, type NavigationGroup } from '../src/components/platform/app-shell'
import App from '../src/routes/app'
import AppOverview from '../src/routes/app.overview'

const APPEARANCE_KEY = 'myqypt.appearance.v1'

// 位置探针：断言命令面板与导航链接触发的客户端跳转
function LocationProbe() {
  const location = useLocation()
  return <div data-testid="location-probe">{location.pathname}</div>
}

const testNavigation: readonly NavigationGroup[] = [
  {
    id: 'main',
    label: '主菜单',
    items: [
      { id: 'overview', label: '总览', href: '/app/overview' },
      { id: 'reports', label: '报表', href: '/app/reports' },
    ],
  },
]

function renderShell(navigation: readonly NavigationGroup[] = testNavigation) {
  return render(
    <MemoryRouter initialEntries={['/app']}>
      <LocationProbe />
      <AppShell navigation={navigation} userMenu={<div>用户菜单</div>}>
        <div>页面内容</div>
      </AppShell>
    </MemoryRouter>,
  )
}

// Radix DropdownMenu 触发器在 pointerdown（主键、无 Ctrl）时打开菜单
function openAppearanceMenu() {
  fireEvent.pointerDown(screen.getByRole('button', { name: '外观设置' }), {
    button: 0,
    ctrlKey: false,
  })
}

function readStoredAppearance() {
  return JSON.parse(window.localStorage.getItem(APPEARANCE_KEY) ?? '{}') as Record<string, unknown>
}

beforeEach(() => {
  window.localStorage.clear()
  document.documentElement.classList.remove('dark')
  document.documentElement.removeAttribute('data-density')
})

describe('AppShell 布局', () => {
  it('渲染授权传入的导航组、链接与调用方提供的用户区', () => {
    renderShell()
    const nav = screen.getByRole('navigation', { name: '主导航' })
    expect(nav).toBeVisible()
    expect(within(nav).getByText('主菜单')).toBeVisible()
    expect(within(nav).getByRole('link', { name: '总览' })).toHaveAttribute('href', '/app/overview')
    expect(within(nav).getByRole('link', { name: '报表' })).toHaveAttribute('href', '/app/reports')
    expect(screen.getByText('用户菜单')).toBeVisible()
    expect(screen.getByText('页面内容')).toBeVisible()
  })

  it('未传入 props 的导航项不得出现在 DOM（AppShell 不自行授予权限）', () => {
    renderShell([
      {
        id: 'main',
        label: '主菜单',
        items: [{ id: 'overview', label: '总览', href: '/app/overview' }],
      },
    ])
    expect(screen.queryByRole('link', { name: '报表' })).toBeNull()
    expect(screen.queryByText('报表')).toBeNull()
  })

  it('侧边栏链接可通过键盘聚焦', () => {
    renderShell()
    const links = within(screen.getByRole('navigation', { name: '主导航' })).getAllByRole('link')
    expect(links).toHaveLength(2)
    links[0].focus()
    expect(links[0]).toHaveFocus()
    links[1].focus()
    expect(links[1]).toHaveFocus()
  })

  it('点击侧边栏链接完成客户端导航', () => {
    renderShell()
    fireEvent.click(screen.getByRole('link', { name: '报表' }))
    expect(screen.getByTestId('location-probe')).toHaveTextContent('/app/reports')
  })

  it('桌面折叠按钮切换侧边栏状态并持久化', () => {
    renderShell()
    const collapse = screen.getByRole('button', { name: '折叠侧边栏' })
    expect(collapse).toHaveAttribute('aria-expanded', 'true')

    fireEvent.click(collapse)
    expect(document.querySelector('[data-sidebar="collapsed"]')).not.toBeNull()
    expect(readStoredAppearance().sidebarCollapsed).toBe(true)

    const expand = screen.getByRole('button', { name: '展开侧边栏' })
    expect(expand).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(expand)
    expect(document.querySelector('[data-sidebar="expanded"]')).not.toBeNull()
    expect(readStoredAppearance().sidebarCollapsed).toBe(false)
  })

  it('重新挂载后恢复已持久化的外观偏好', () => {
    window.localStorage.setItem(
      APPEARANCE_KEY,
      JSON.stringify({ theme: 'dark', density: 'compact', sidebarCollapsed: true }),
    )
    renderShell()
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(document.documentElement.getAttribute('data-density')).toBe('compact')
    expect(document.querySelector('[data-sidebar="collapsed"]')).not.toBeNull()
  })
})

describe('移动 Sheet 导航', () => {
  it('菜单按钮打开 Sheet，Escape 关闭并把焦点还给菜单按钮', async () => {
    renderShell()
    const menuButton = screen.getByRole('button', { name: '打开导航菜单' })
    // 真实浏览器里按下鼠标即聚焦按钮；fireEvent 不聚焦，这里先聚焦再点击，
    // 使 Radix 在挂载时捕获到触发按钮，关闭后才能把焦点还给它
    menuButton.focus()
    fireEvent.click(menuButton)

    const sheet = screen.getByRole('dialog', { name: '导航菜单' })
    expect(within(sheet).getByRole('link', { name: '总览' })).toBeVisible()

    fireEvent.keyDown(sheet, { key: 'Escape' })
    expect(screen.queryByRole('dialog', { name: '导航菜单' })).toBeNull()
    // Radix 在卸载后的宏任务里归还焦点，等待其完成
    await waitFor(() => expect(menuButton).toHaveFocus())
  })

  it('Sheet 内点击链接完成导航并关闭', () => {
    renderShell()
    fireEvent.click(screen.getByRole('button', { name: '打开导航菜单' }))
    const sheet = screen.getByRole('dialog', { name: '导航菜单' })
    fireEvent.click(within(sheet).getByRole('link', { name: '报表' }))
    expect(screen.getByTestId('location-probe')).toHaveTextContent('/app/reports')
    expect(screen.queryByRole('dialog', { name: '导航菜单' })).toBeNull()
  })
})

describe('命令面板', () => {
  it('Meta+K 打开，输入过滤，Enter 执行选中命令并关闭', () => {
    renderShell()
    fireEvent.keyDown(window, { key: 'k', metaKey: true })

    const dialog = screen.getByRole('dialog', { name: '命令面板' })
    const input = within(dialog).getByRole('combobox')
    expect(within(dialog).getByRole('option', { name: '总览' })).toBeVisible()
    expect(within(dialog).getByRole('option', { name: '报表' })).toBeVisible()

    fireEvent.change(input, { target: { value: '报表' } })
    expect(within(dialog).queryByRole('option', { name: '总览' })).toBeNull()
    expect(within(dialog).getByRole('option', { name: '报表' })).toBeVisible()

    fireEvent.change(input, { target: { value: '没有匹配的词' } })
    expect(within(dialog).getByText('没有匹配的结果')).toBeVisible()

    fireEvent.change(input, { target: { value: '报表' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(screen.getByTestId('location-probe')).toHaveTextContent('/app/reports')
    expect(screen.queryByRole('dialog', { name: '命令面板' })).toBeNull()
  })

  it('方向键循环切换选中项', () => {
    renderShell()
    fireEvent.keyDown(window, { key: 'k', metaKey: true })

    const dialog = screen.getByRole('dialog', { name: '命令面板' })
    const input = within(dialog).getByRole('combobox')
    const [first, second] = within(dialog).getAllByRole('option')
    expect(first).toHaveAttribute('aria-selected', 'true')

    fireEvent.keyDown(input, { key: 'ArrowDown' })
    expect(second).toHaveAttribute('aria-selected', 'true')

    fireEvent.keyDown(input, { key: 'ArrowUp' })
    expect(first).toHaveAttribute('aria-selected', 'true')
  })

  it('Ctrl+K 也可打开，Escape 关闭面板', () => {
    renderShell()
    fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
    const dialog = screen.getByRole('dialog', { name: '命令面板' })

    fireEvent.keyDown(dialog, { key: 'Escape' })
    expect(screen.queryByRole('dialog', { name: '命令面板' })).toBeNull()
  })
})

describe('外观偏好', () => {
  it('切换主题立即应用并只持久化外观字段', () => {
    renderShell()
    openAppearanceMenu()
    fireEvent.click(screen.getByRole('menuitemradio', { name: '深色' }))

    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(readStoredAppearance().theme).toBe('dark')

    openAppearanceMenu()
    fireEvent.click(screen.getByRole('menuitemradio', { name: '浅色' }))
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(readStoredAppearance().theme).toBe('light')

    // 持久化仅包含版本化外观键，且字段恰好为主题/密度/折叠三项
    expect(window.localStorage.length).toBe(1)
    expect(window.localStorage.key(0)).toBe(APPEARANCE_KEY)
    expect(Object.keys(readStoredAppearance()).sort()).toEqual([
      'density',
      'sidebarCollapsed',
      'theme',
    ])
  })

  it('切换密度写入 data-density 并持久化', () => {
    renderShell()
    openAppearanceMenu()
    fireEvent.click(screen.getByRole('menuitemradio', { name: '紧凑' }))
    expect(document.documentElement.getAttribute('data-density')).toBe('compact')
    expect(readStoredAppearance().density).toBe('compact')

    openAppearanceMenu()
    fireEvent.click(screen.getByRole('menuitemradio', { name: '舒适' }))
    expect(document.documentElement.getAttribute('data-density')).toBe('comfortable')
    expect(readStoredAppearance().density).toBe('comfortable')
  })

  it('卸载 AppShell 后清理 <html> 上的 dark 类与 data-density 属性', () => {
    const { unmount } = renderShell()
    openAppearanceMenu()
    fireEvent.click(screen.getByRole('menuitemradio', { name: '深色' }))
    openAppearanceMenu()
    fireEvent.click(screen.getByRole('menuitemradio', { name: '紧凑' }))
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(document.documentElement.getAttribute('data-density')).toBe('compact')

    unmount()
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(document.documentElement.getAttribute('data-density')).toBeNull()
  })
})

describe('/app 路由（静态示例导航）', () => {
  it('渲染 AppShell，且导航仅包含授权传入的总览入口', () => {
    render(
      <MemoryRouter initialEntries={['/app']}>
        <Routes>
          <Route path="/app" element={<App />}>
            <Route index element={<AppOverview />} />
            <Route path="overview" element={<AppOverview />} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )
    const nav = screen.getByRole('navigation', { name: '主导航' })
    expect(nav).toBeVisible()
    expect(within(nav).getAllByRole('link')).toHaveLength(1)
    expect(within(nav).getByRole('link', { name: '总览' })).toHaveAttribute('href', '/app/overview')
    expect(within(nav).queryByRole('link', { name: '报表' })).toBeNull()
    expect(screen.getByRole('heading', { level: 1, name: '总览' })).toBeVisible()
    expect(screen.getByText('示例用户')).toBeVisible()
  })
})
