import { Outlet } from 'react-router'

import { AppShell } from '../components/platform/app-shell'
import { consoleNavigation } from '../features/navigation/model'

// 静态占位用户区：纯展示，无任何认证逻辑（身份在 F10–F12 引入）
const userMenu = (
  <div className="flex items-center gap-2 rounded-md border px-2 py-1 text-sm text-muted-foreground">
    <span aria-hidden="true" className="size-2 rounded-full bg-muted-foreground/40" />
    示例用户
  </div>
)

export default function App() {
  return (
    <AppShell navigation={consoleNavigation} userMenu={userMenu}>
      <Outlet />
    </AppShell>
  )
}
