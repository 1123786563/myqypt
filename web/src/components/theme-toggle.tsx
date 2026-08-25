import { Button } from './ui/button'

const THEME_STORAGE_KEY = 'ui-theme'

export function ThemeToggle() {
  const toggleTheme = () => {
    const isDark = document.documentElement.classList.toggle('dark')
    try {
      localStorage.setItem(THEME_STORAGE_KEY, isDark ? 'dark' : 'light')
    } catch {
      // 浏览器禁用本地存储时静默忽略
    }
  }

  return (
    <Button variant="ghost" size="icon" aria-label="切换主题" onClick={toggleTheme}>
      <span aria-hidden="true">月</span>
    </Button>
  )
}
