import { type RouteConfig, index, route } from '@react-router/dev/routes'

export default [
  route('/', 'routes/home.tsx'),
  // /app 控制台壳：索引与 /app/overview 共用总览页（索引路由显式指定 id
  // 避免与 overview 路由按文件路径推导出重复 id），
  // 保证 /app 与深链都有真实内容（静态托管回退见 scripts/serve.mjs）
  route('app', 'routes/app.tsx', [
    index('routes/app.overview.tsx', { id: 'routes/app.overview-index' }),
    route('overview', 'routes/app.overview.tsx'),
  ]),
] satisfies RouteConfig
