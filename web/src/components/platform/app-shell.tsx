import * as React from 'react'
import type { PropsWithChildren, ReactNode } from 'react'
import { NavLink, useNavigate } from 'react-router'

import { cn } from '../../lib/utils'
import { Button } from '../ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '../ui/sheet'
import { AppearancePreferences, AppearanceProvider } from './appearance-preferences'
import { useAppearance } from './appearance-store'
import { CommandPalette, type CommandItem } from './command-palette'

// 导航模型：AppShell 只消费调用方提供的（已授权过滤的）模型，
// 自身不做任何权限判定——不在 props 里的项目不会出现在 DOM 中。
export type NavigationItem = {
  id: string
  label: string
  href: string
  icon?: ReactNode
}

export type NavigationGroup = {
  id: string
  label: string
  items: readonly NavigationItem[]
}

export type AppShellProps = PropsWithChildren<{
  navigation: readonly NavigationGroup[]
  userMenu: ReactNode
}>

// 与 lucide-react 同几何的最小内联图标（本仓库不依赖该包）
function MenuIcon(props: React.ComponentProps<'svg'>) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M4 6h16" />
      <path d="M4 12h16" />
      <path d="M4 18h16" />
    </svg>
  )
}

function ChevronLeftIcon(props: React.ComponentProps<'svg'>) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="m15 18-6-6 6-6" />
    </svg>
  )
}

function ChevronRightIcon(props: React.ComponentProps<'svg'>) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="m9 18 6-6-6-6" />
    </svg>
  )
}

export function AppShell({ navigation, userMenu, children }: AppShellProps) {
  return (
    <AppearanceProvider>
      <AppShellFrame navigation={navigation} userMenu={userMenu}>
        {children}
      </AppShellFrame>
    </AppearanceProvider>
  )
}

function AppShellFrame({ navigation, userMenu, children }: AppShellProps) {
  const { appearance, setAppearance } = useAppearance()
  const navigate = useNavigate()
  const [paletteOpen, setPaletteOpen] = React.useState(false)
  const [mobileOpen, setMobileOpen] = React.useState(false)
  const collapsed = appearance.sidebarCollapsed

  // 全局 Meta/Ctrl+K 开关命令面板
  React.useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        setPaletteOpen((open) => !open)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  // 命令面板的条目由导航模型派生：执行即跳转并收起面板
  const commands = React.useMemo<CommandItem[]>(
    () =>
      navigation.flatMap((group) =>
        group.items.map((item) => ({
          id: item.id,
          label: item.label,
          group: group.label,
          keywords: [item.id],
          onSelect: () => {
            navigate(item.href)
            setPaletteOpen(false)
          },
        })),
      ),
    [navigation, navigate],
  )

  const renderNavigation = (onNavigate?: () => void) =>
    navigation.map((group) => (
      <div key={group.id} className="flex flex-col gap-1">
        <span className="px-2 text-xs font-medium text-muted-foreground">{group.label}</span>
        {group.items.map((item) => (
          <NavLink
            key={item.id}
            to={item.href}
            onClick={onNavigate}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground',
                isActive && 'bg-accent text-accent-foreground',
              )
            }
          >
            {item.icon}
            <span className="truncate">{item.label}</span>
          </NavLink>
        ))}
      </div>
    ))

  return (
    <div data-sidebar={collapsed ? 'collapsed' : 'expanded'} className="flex min-h-svh w-full">
      {/* 桌面侧边栏：宽度随 data-sidebar 折叠状态过渡（app.css） */}
      <aside className="app-sidebar hidden shrink-0 flex-col gap-2 overflow-hidden border-r p-2 whitespace-nowrap lg:flex">
        <nav id="app-sidebar-nav" aria-label="主导航" className="flex flex-1 flex-col gap-6 py-2">
          {renderNavigation()}
        </nav>
        <Button
          variant="ghost"
          size="icon"
          className="self-center"
          aria-label={collapsed ? '展开侧边栏' : '折叠侧边栏'}
          aria-expanded={!collapsed}
          aria-controls="app-sidebar-nav"
          onClick={() => setAppearance({ sidebarCollapsed: !collapsed })}
        >
          {collapsed ? (
            <ChevronRightIcon className="size-4" />
          ) : (
            <ChevronLeftIcon className="size-4" />
          )}
        </Button>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 items-center gap-2 border-b px-4">
          {/* 移动导航：Sheet 关闭（含 Escape）后焦点回到触发按钮 */}
          <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
            <SheetTrigger asChild>
              <Button variant="ghost" size="icon" className="lg:hidden" aria-label="打开导航菜单">
                <MenuIcon className="size-5" />
              </Button>
            </SheetTrigger>
            <SheetContent side="left" className="w-72 p-0">
              <SheetHeader className="border-b">
                <SheetTitle>导航菜单</SheetTitle>
                <SheetDescription>平台导航入口</SheetDescription>
              </SheetHeader>
              <nav className="flex flex-col gap-6 p-4">
                {renderNavigation(() => setMobileOpen(false))}
              </nav>
            </SheetContent>
          </Sheet>

          <Button
            variant="outline"
            size="sm"
            aria-label="打开命令面板"
            onClick={() => setPaletteOpen(true)}
          >
            <span aria-hidden="true">⌘K</span>
          </Button>

          <div className="ml-auto flex items-center gap-2">
            <AppearancePreferences />
            {userMenu}
          </div>
        </header>

        <main className="flex-1 p-4 md:p-6">{children}</main>
      </div>

      <CommandPalette commands={commands} open={paletteOpen} onOpenChange={setPaletteOpen} />
    </div>
  )
}
