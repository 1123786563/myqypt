import * as React from 'react'
import type { PropsWithChildren } from 'react'

import { Button } from '../ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '../ui/dropdown-menu'
import {
  AppearanceContext,
  loadAppearance,
  saveAppearance,
  useAppearance,
  type Appearance,
  type DensityPreference,
  type ThemePreference,
} from './appearance-store'

// 外观偏好的状态容器：挂载时恢复，用户显式修改后才写回存储，
// 避免仅到访控制台就覆盖落地页仍在使用的 ui-theme 键。
export function AppearanceProvider({ children }: PropsWithChildren) {
  const [appearance, setAppearanceState] = React.useState<Appearance>(loadAppearance)
  const hydratedRef = React.useRef(false)

  React.useEffect(() => {
    if (!hydratedRef.current) {
      hydratedRef.current = true
      return
    }
    saveAppearance(appearance)
  }, [appearance])

  // 主题应用到 <html> 的 dark 类；跟随系统时按当前配色求值
  React.useEffect(() => {
    const root = document.documentElement
    if (appearance.theme === 'dark') root.classList.add('dark')
    else if (appearance.theme === 'light') root.classList.remove('dark')
    else root.classList.toggle('dark', window.matchMedia('(prefers-color-scheme: dark)').matches)
  }, [appearance.theme])

  // 密度应用到 <html> 的 data-density 属性（app.css 提供紧凑档样式钩子）
  React.useEffect(() => {
    document.documentElement.dataset.density = appearance.density
  }, [appearance.density])

  const value = React.useMemo<ReturnType<typeof useAppearance>>(
    () => ({
      appearance,
      setAppearance: (patch: Partial<Appearance>) =>
        setAppearanceState((current) => ({ ...current, ...patch })),
    }),
    [appearance],
  )

  return <AppearanceContext.Provider value={value}>{children}</AppearanceContext.Provider>
}

// 控制台外观设置菜单：主题（浅色/深色/跟随系统）与密度（舒适/紧凑）
export function AppearancePreferences() {
  const { appearance, setAppearance } = useAppearance()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" aria-label="外观设置">
          外观
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>主题</DropdownMenuLabel>
        <DropdownMenuRadioGroup
          value={appearance.theme}
          onValueChange={(value) => setAppearance({ theme: value as ThemePreference })}
        >
          <DropdownMenuRadioItem value="light">浅色</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="dark">深色</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="system">跟随系统</DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
        <DropdownMenuSeparator />
        <DropdownMenuLabel>密度</DropdownMenuLabel>
        <DropdownMenuRadioGroup
          value={appearance.density}
          onValueChange={(value) => setAppearance({ density: value as DensityPreference })}
        >
          <DropdownMenuRadioItem value="comfortable">舒适</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="compact">紧凑</DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
