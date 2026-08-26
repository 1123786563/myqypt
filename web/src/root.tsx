import type { LinksFunction, MetaFunction } from 'react-router'
import { Links, Meta, Outlet, Scripts, ScrollRestoration } from 'react-router'
import './styles/app.css'

export const links: LinksFunction = () => [{ rel: 'canonical', href: '/' }]

export const meta: MetaFunction = () => [
  { title: 'myqypt · AI 应用平台' },
  {
    name: 'description',
    content:
      'myqypt 是面向个人与企业的多租户 AI 应用平台，为独立运营的 AI 产品提供统一身份、订阅、用量可见性与生命周期管理。',
  },
]

// 在首帧绘制前解析主题偏好，避免主题闪烁（FOUC）：版本化外观键
// （myqypt.appearance.v1，含 theme/density/sidebarCollapsed）优先，
// 回退到落地页仍在使用的 ui-theme，再回退系统配色。
const themeInitScript = `
;(function () {
  var preference = null
  try {
    var saved = JSON.parse(localStorage.getItem('myqypt.appearance.v1') || 'null')
    if (saved && (saved.theme === 'light' || saved.theme === 'dark' || saved.theme === 'system')) {
      preference = saved.theme
    }
  } catch (_error) {}
  if (preference === null) {
    try {
      var legacy = localStorage.getItem('ui-theme')
      if (legacy === 'light' || legacy === 'dark') preference = legacy
    } catch (_error) {}
  }
  var theme =
    preference === 'light' || preference === 'dark'
      ? preference
      : window.matchMedia('(prefers-color-scheme: dark)').matches
        ? 'dark'
        : 'light'
  document.documentElement.classList.toggle('dark', theme === 'dark')
})()
`

export default function Root() {
  return (
    <html lang="zh-CN">
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <Meta />
        <Links />
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
      </head>
      <body>
        <Outlet />
        <ScrollRestoration />
        <Scripts />
      </body>
    </html>
  )
}
