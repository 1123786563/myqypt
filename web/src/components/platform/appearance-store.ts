import { createContext, useContext } from 'react'

// 控制台外观偏好的唯一持久化键（版本化）：只存主题、密度与侧边栏折叠
// 三个字段，绝不包含身份、令牌或其他任何内容。
export const APPEARANCE_STORAGE_KEY = 'myqypt.appearance.v1'

export type ThemePreference = 'light' | 'dark' | 'system'
export type DensityPreference = 'comfortable' | 'compact'

export type Appearance = {
  theme: ThemePreference
  density: DensityPreference
  sidebarCollapsed: boolean
}

const DEFAULT_APPEARANCE: Appearance = {
  theme: 'system',
  density: 'comfortable',
  sidebarCollapsed: false,
}

function isAppearance(value: unknown): value is Appearance {
  if (typeof value !== 'object' || value === null) return false
  const candidate = value as Record<string, unknown>
  return (
    (candidate.theme === 'light' || candidate.theme === 'dark' || candidate.theme === 'system') &&
    (candidate.density === 'comfortable' || candidate.density === 'compact') &&
    typeof candidate.sidebarCollapsed === 'boolean'
  )
}

// 读取外观偏好：版本化新键优先；未写入过新键时回退读取落地页的 ui-theme，
// 再退回默认值。解析或校验失败时静默使用默认值。
// 经 window 显式访问本地存储：浏览器中与裸 localStorage 等价，
// 测试环境的 Node 实验性 localStorage 全局则不可用。
export function loadAppearance(): Appearance {
  try {
    const raw = window.localStorage.getItem(APPEARANCE_STORAGE_KEY)
    if (raw !== null) {
      const parsed: unknown = JSON.parse(raw)
      if (isAppearance(parsed)) return parsed
    }
    const legacy = window.localStorage.getItem('ui-theme')
    if (legacy === 'light' || legacy === 'dark') {
      return { ...DEFAULT_APPEARANCE, theme: legacy }
    }
  } catch {
    // 浏览器禁用本地存储时使用默认值
  }
  return DEFAULT_APPEARANCE
}

export function saveAppearance(appearance: Appearance) {
  try {
    window.localStorage.setItem(APPEARANCE_STORAGE_KEY, JSON.stringify(appearance))
  } catch {
    // 浏览器禁用本地存储时静默忽略
  }
}

export type AppearanceContextValue = {
  appearance: Appearance
  setAppearance: (patch: Partial<Appearance>) => void
}

export const AppearanceContext = createContext<AppearanceContextValue | null>(null)

export function useAppearance(): AppearanceContextValue {
  const value = useContext(AppearanceContext)
  if (value === null) {
    throw new Error('useAppearance 必须在 AppearanceProvider 内使用')
  }
  return value
}
