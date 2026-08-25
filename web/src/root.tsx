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

// 在首帧绘制前根据 ui-theme 偏好与系统配色设置暗色模式，避免主题闪烁（FOUC）。
const themeInitScript = `
;(function () {
  var stored = null
  try {
    stored = localStorage.getItem('ui-theme')
  } catch (_error) {}
  var theme =
    stored === 'light' || stored === 'dark'
      ? stored
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
