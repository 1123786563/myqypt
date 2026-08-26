import type { NavigationGroup } from '../../components/platform/app-shell'

// 控制台静态示例导航（issue #107 Task 2）：仅包含「总览」一个入口。
// 动态授权导航在 F10–F12 引入；本模型代表调用方已完成授权过滤的结果，
// AppShell 不自行判定权限。
export const consoleNavigation: readonly NavigationGroup[] = [
  {
    id: 'console',
    label: '控制台',
    items: [{ id: 'overview', label: '总览', href: '/app/overview' }],
  },
]
