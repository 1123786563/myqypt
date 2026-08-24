# shadcn-admin + go-admin 白名单抽取设计

> 状态：待最终审阅
>
> 日期：2026-08-24
>
> 范围：Stage 1 前后端工程基础，不包含业务功能实现

## 1. 结论

平台不完整 Fork `shadcn-admin` 或 `go-admin`，而是执行白名单抽取：

- 从 `shadcn-admin` 抽取 React UI、交互模式、控制台布局和前端工程配置；
- 从 `go-admin` 抽取 Go 进程、命令、HTTP Transport 和通用基础设施组织方式；
- 不抽取任一上游的认证、权限、租户、计费或演示业务模型；
- 所有公开 HTTP 契约仍以 OpenAPI 3.1 为唯一事实源；
- 所有租户访问仍遵循 `Tenant -> Membership -> PostgreSQL active -> OpenFGA allowed -> Platform Context`；
- 复制实质代码时保留 MIT License，并记录固定上游 commit、复制范围和本地修改。

核心原则是：抽取可以形成深 Module 的实现，不抽取只会把上游领域假设扩散到调用方的浅封装。

## 2. 背景与约束

当前仓库只有领域、架构、ADR 和 Stage 1 实施计划，尚无业务运行代码。已接受的约束包括：

- Platform 是面向个人与小企业的公共多租户 SaaS；
- Tenant 是安全、数据、用量和计费的硬边界；
- Keycloak 拥有登录身份，Platform 拥有 User、Membership 和 Product Access；
- PostgreSQL 是 Membership、Platform Role 和 Product Access 的业务事实源；
- OpenFGA 是授权投影和求值器；
- Product 仅信任由可信边缘签发的短期、audience-bound Platform Context；
- Portal 同时承载官网、产品目录、价格页、登录入口和租户控制台；
- 生产服务端全部使用 Go，React 只生成静态资源并通过 CDN/Higress 发布；
- 产品和价格变更可以触发数分钟内完成的静态重建；
- Platform API 需要正式开放给第三方客户端和 Product Adapter。

这些约束优先于两个上游项目的默认实现。

## 3. 上游基线

设计审查固定以下上游快照：

| 上游 | 审查 commit | 许可证 | 可转移能力 | 不可直接转移能力 |
| --- | --- | --- | --- | --- |
| `satnaing/shadcn-admin` | `e16c87f213a5ba5e45964e9b67c792105ec74d26` | MIT | React 19、TypeScript、Vite、Tailwind 4、shadcn、TanStack Query/Table、控制台布局、浏览器测试配置 | Clerk 示例认证、本地 access token、演示领域、纯 SPA 路由假设 |
| `go-admin-team/go-admin` | `1b7dcd843ce38fddc8c280fe3139e02735cf7574` | MIT | Gin、Cobra、多命令组织、HTTP 中间件、进程启动、对象存储 Adapter 的实现思路 | JWT、Casbin、Host-based Tenant、SysUser/SysDept/SysRole/SysMenu、GORM 模型、Swaggo、通用 CRUD 与代码生成器 |

上游 commit 只是来源证明，不代表将来自动同步。任何上游更新都必须重新执行差异审查，不能直接覆盖本地 Module。

## 4. 目标架构

```text
Browser / Third-party Client
            |
            v
         Higress
            |
            +-- /, /products/*, /pricing --> CDN static React
            +-- /app/* ------------------> CDN SPA fallback
            +-- /portal-api/* -----------> Go Portal BFF
            +-- /api/v1/* ---------------> Go Public API
            +-- /webhooks/* -------------> Go Provider endpoints

Go Transport
  Gin + oapi-codegen strict server
            |
            v
Application Modules
            |
            +-- Identity port ------> Keycloak Adapter
            +-- Authorization port -> OpenFGA Adapter
            +-- Repository port ----> PostgreSQL Adapter
            +-- Workflow port ------> Temporal Adapter
            +-- ObjectStore port ---> OSS/OBS-compatible Adapter
```

Gin 只存在于 HTTP Transport Adapter。Application 和 Domain Module 的 Interface 不得暴露 `*gin.Context`、`*gorm.DB`、JWT Claims 或 Casbin Enforcer。

## 5. 前端白名单抽取

### 5.1 直接保留并本地化

从 `shadcn-admin` 保留以下工程能力，并根据平台命名和品牌进行本地化：

- React 19、TypeScript、Vite 和 Tailwind CSS 4 的构建配置；
- shadcn 的 CSS Variables、颜色、字体、间距和暗色模式；
- 响应式 Sidebar、Header 和 Top Navigation；
- Command Palette 和全局搜索外壳；
- Theme Switch 与用户菜单的纯展示部分；
- Dialog、Sheet、Dropdown、Form Field 等基础交互模式；
- TanStack Query 和 TanStack Table；
- Vitest Browser、Playwright、ESLint、Prettier 和 Knip 配置；
- 401、403、404、500、503 状态页的布局结构。

shadcn 基础组件优先通过官方 CLI 生成干净版本。只有上游确有平台需要的修改时，才从 `shadcn-admin` 移植差异，避免无意继承其 RTL 或演示项目定制。

### 5.2 改造成平台 Module

| 上游模式 | 目标 Module | Interface 与隐藏行为 |
| --- | --- | --- |
| Sidebar/Menu | `AppShell` | 接收已经过授权过滤的导航模型；隐藏响应式折叠、移动端 Sheet、焦点恢复和布局状态 |
| Tasks/Users Table 示例 | `PlatformDataTable` | 接收列、服务端分页、排序、筛选和行操作；隐藏 URL 同步、加载、空状态、错误、选择和批量行为 |
| Dialog 状态 Hook | `DialogController` | 暴露 `open`、`close` 和当前 Dialog；隐藏互斥与恢复规则 |
| Theme/Font Store | `AppearancePreferences` | 暴露主题、字号和密度；隐藏持久化和系统主题监听 |
| Search Command | `CommandPalette` | 接收搜索项和动作；隐藏快捷键、分组、过滤和导航行为 |
| Error Pages | `RouteErrorBoundary` | 接收标准错误分类；隐藏状态页选择、重试和追踪 ID 展示 |

### 5.3 路由与静态发布

不直接复制 `shadcn-admin` 当前 TanStack Router 路由树。原因是它以控制台 SPA 为目标，而本平台还要求公开页面的稳定 SEO 和构建期预渲染。

目标方案为：

- 使用 React Router Framework Mode；
- 设置运行期 `ssr: false`；
- 构建期预渲染 `/`、产品目录、产品详情、价格和公共政策页面；
- `/app/*` 生成 SPA fallback；
- Catalog 或价格发布事件触发静态构建；
- 构建从 Go 的只读“已发布 Catalog 快照”获取内容；
- CDN 使用带内容哈希的不可变静态资产，HTML 采用短缓存或发布时主动失效；
- 发布采用原子版本切换，失败时继续服务上一版本。

### 5.4 前端明确拒绝清单

以下内容不得进入目标工程：

- `@clerk/react` 与所有 `/clerk/*` 路由；
- `auth-store` 中的 access token；
- `localStorage` 或 `sessionStorage` Token；
- 演示登录、注册、OTP 和忘记密码实现；
- Tasks、Chats、Users、Apps 的假数据与演示领域模型；
- Clerk User Management；
- 上游品牌图片、Logo 和营销文案；
- 上游自动生成的 `routeTree.gen.ts`；
- 带默认认证或错误约定的 Axios 实例；
- 上游完整 lockfile 和未被目标 Module 使用的依赖。

## 6. 后端白名单抽取

### 6.1 工程运行骨架

从 `go-admin` 抽取或重写以下工程能力：

- Gin Router 和中间件组合方式；
- Cobra 多命令入口；
- `platform-api`、`platform-worker`、`migrate` 和 `version` 命令形态；
- HTTP Server timeout、信号监听和优雅关闭；
- Request ID、Panic Recovery、安全响应头与 CORS；
- 配置加载、环境覆盖和启动失败的组织方式；
- Dockerfile、Makefile 和开发 Compose 的结构思路；
- 版本信息注入；
- `/livez` 与 `/readyz`；
- OSS、OBS、Qiniu 等对象存储 Adapter 的实现思路。

对象存储只在文件功能进入实施时抽取。它是一个真实 seam：生产可以有多个云 Adapter，测试使用本地或内存 Adapter。Stage 1 基础搭建不提前引入未使用的云 SDK。

### 6.2 HTTP Transport

公开 HTTP 契约以 OpenAPI 3.1 文件为唯一事实源。使用 `oapi-codegen` 的 `gin-server + strict-server` 生成：

- 请求和响应传输类型；
- Strict Server Interface；
- Gin 路由绑定；
- TypeScript 客户端的上游契约输入。

生成文件只属于 Transport，不作为 Domain Model、数据库 Model 或跨 Module 共享模型。OpenAPI request validation middleware 负责结构和格式校验；业务不变量仍由 Application Module 校验。

### 6.3 必须重写的实现

| `go-admin` 默认实现 | 平台目标实现 |
| --- | --- |
| Logrus/Zap 与全局 Logger | `slog` + OpenTelemetry，依赖显式注入 |
| GORM 与多数据库 Driver | PostgreSQL + pgx/sqlc |
| `AutoMigrate` | 显式、追加式、版本化 SQL migration |
| Swaggo 注解 | OpenAPI 3.1 contract-first |
| 通用 JSON Response | OpenAPI 响应类型 + RFC Problem Details |
| 全局 `sdk.Runtime` | Composition Root 显式构造依赖 |
| Redis 初始化 | Valkey Adapter |
| 动态通用排序/过滤 | OpenAPI 明确允许的字段白名单 |
| 内建对象存储选择 | `ObjectStore` port + 按环境注入 Adapter |

### 6.4 后端明确拒绝清单

以下代码或概念不得进入目标工程：

- `app/admin`、`app/demo` 和 `app/other` 业务代码；
- `SysUser`、`SysDept`、`SysPost`、`SysRole` 和 `SysMenu`；
- 自有登录、验证码、JWT 和 Refresh Token；
- Casbin 规则、Enforcer 和权限中间件；
- 根据 Host、Header、Query 或 Cookie 自动选择 Tenant DB；
- `common/actions` 通用 CRUD；
- `common/apis` 中的全局 GORM 注入；
- `common/global`；
- GORM Model、Hook 和 `AutoMigrate`；
- 数据表驱动的代码生成器和 Form Builder；
- Swaggo 文档生成；
- 上游定时任务、服务监控和内容管理业务；
- 默认管理员、默认密码和演示数据；
- MySQL、SQLite、SQL Server 等未使用数据库 Driver；
- 与平台事实模型冲突的 Tenant、Organization、Department 或 Data Scope。

## 7. 关键 Module 与 Seam

### 7.1 Session Module

浏览器不持有 Keycloak Access Token。Go Portal BFF 完成 OIDC Authorization Code 流程，并向浏览器签发 `HttpOnly + Secure + SameSite` Session Cookie。

Session Module 的 Interface 包含登录开始、回调完成、当前会话查询和登出。Keycloak 是生产 Adapter，测试使用内存 Identity Adapter。

### 7.2 Tenant Authorization Module

Tenant ID 可以由用户在可信边缘显式选择，但不能直接成为内部可信身份。Module 必须：

1. 解析已认证 Principal；
2. 加载 Membership 业务状态；
3. 要求 Membership 为 active；
4. 执行 OpenFGA Check；
5. 删除客户端提供的内部身份 Header；
6. 返回短期、audience-bound Platform Context；
7. 任一依赖不可用或结论不完整时 fail closed。

### 7.3 Public API Module

`/api/v1` 面向第三方客户端与 Product Adapter，承担版本兼容和 SDK 承诺。`/portal-api` 面向自有 Portal，可以执行页面级聚合，但不得绕过相同的 Application Module 和授权规则。

### 7.4 ObjectStore Module

Interface 只暴露平台需要的 `Put`、`Get`、`Delete` 和签名访问能力，并明确内容类型、大小限制、校验和、Tenant Scope 与错误分类。云厂商 SDK、重试、分片上传和 endpoint 差异隐藏在 Adapter 内。

## 8. 数据流

### 8.1 Portal 登录与租户访问

```text
React -> /portal-api/auth/login
      -> Go BFF -> Keycloak
Keycloak callback -> Go BFF
      -> Session Cookie
React -> /portal-api/session
      -> select tenant
      -> Membership active
      -> OpenFGA allowed
      -> Application Module
      -> tenant-scoped PostgreSQL query
```

### 8.2 第三方 API

```text
Third-party Client
  -> Higress
  -> /api/v1
  -> bearer/API credential verification
  -> Tenant Authorization Module
  -> OpenAPI strict handler
  -> Application Module
```

第三方客户端也不能仅凭 `X-Tenant-ID` 获得 Tenant 权限。Tenant 选择必须与已认证主体、Membership 和授权结果绑定。

### 8.3 Catalog 静态发布

```text
Catalog publish transaction
  -> outbox event
  -> frontend build job
  -> read published Catalog snapshot
  -> prerender public routes
  -> validate HTML/metadata/links
  -> upload versioned artifact
  -> atomic CDN switch
```

构建失败不影响上一版静态站点。价格页展示 Catalog 版本或生效时间，避免构建窗口内的短暂陈旧造成误解。

## 9. 错误处理

- Public API 使用稳定的 RFC Problem Details 结构；
- 错误至少包含稳定 code、HTTP status、可展示 title、request/trace ID；
- 不向客户端暴露数据库、Keycloak、OpenFGA、Temporal 或支付 Provider 原始错误；
- 身份缺失返回 401，Membership/Authorization 拒绝返回 403；
- 无效请求返回 400 或 422，冲突返回 409；
- 依赖不可用按接口语义返回 503，保护请求必须 fail closed；
- Portal BFF 可以把稳定错误映射成页面文案，但不得重新定义业务结果；
- React Error Boundary 区分路由不存在、权限不足、会话过期、暂时不可用和未知错误；
- 所有重试都必须遵守幂等性约束，写接口不得由通用前端拦截器无条件重试。

## 10. 安全与多租户护栏

- 禁止浏览器保存 Access Token；
- 禁止从未验证 Header、Query、Cookie、Host 或 Product ID 推导可信 Tenant；
- 任何 Tenant-owned 表的查询都必须显式携带 Tenant Scope；
- PostgreSQL 对关键共享表使用 RLS 或等效数据库级隔离；
- 普通 Repository 不提供仅凭全局 ID 访问 Tenant-owned 对象的 Interface；
- 访问必须同时满足 PostgreSQL business relationship active 和 OpenFGA allowed；
- 不缓存 allow；OpenFGA 不可用时保护请求拒绝；
- 日志、指标和错误不得包含 Secret、Token、Prompt、Document body 或原始支付 payload；
- 对象存储 key、向量过滤、后台任务和缓存 key 都必须包含受信 Tenant Scope；
- 上游默认用户、密码、演示 Secret 和配置文件不得复制。

## 11. 目标目录

```text
myqypt/
|-- api/
|   `-- openapi/
|-- cmd/
|   |-- platform-api/
|   |-- platform-worker/
|   |-- migrate/
|   `-- version/
|-- internal/
|   |-- platform/
|   |-- identity/
|   |-- tenant/
|   |-- authorization/
|   |-- product/
|   |-- transport/http/
|   `-- adapters/
|       |-- postgres/
|       |-- openfga/
|       |-- keycloak/
|       `-- objectstore/
|-- web/
|   `-- src/
|       |-- routes/public/
|       |-- routes/app/
|       |-- components/ui/
|       |-- components/app-shell/
|       |-- components/platform-table/
|       |-- features/session/
|       `-- features/tenant/
`-- THIRD_PARTY_NOTICES.md
```

## 12. Foundation 验收标准

### 12.1 前端基础

- `/` 构建产物包含无需执行 JavaScript 即可读取的标题、正文、canonical 和基本 metadata；
- `/app` 能加载控制台 AppShell；
- 响应式 Sidebar、主题和标准错误页可用；
- 至少一个 `PlatformDataTable` 示例证明服务端分页、排序、筛选和 URL 同步；
- Session 通过 Go BFF Cookie 工作，前端不存在 Token 存储；
- 仓库中不存在 Clerk、演示 Token 和假业务数据；
- lint、format check、type-check、unit test、browser component test 和静态构建分别执行并记录结果；
- Playwright smoke 覆盖公开预渲染页、SPA fallback、会话过期和权限不足。

### 12.2 后端基础

- `platform-api` 可以启动、接收终止信号并优雅退出；
- `/livez` 只证明进程存活，`/readyz` 检查必要依赖且不会泄露配置；
- OpenAPI 生成的 Gin strict handler 可以处理一个无业务副作用的契约端点；
- PostgreSQL migration 与连接检查可执行；
- Request ID、结构化日志、Panic Recovery、Server timeout 和安全响应头生效；
- 未配置 Keycloak/OpenFGA 时保护接口 fail closed；
- 测试证明伪造 Tenant Header、跨 Tenant ID 和被撤销 Membership 均被拒绝；
- 仓库扫描确认不存在 Casbin、上游 JWT、自有验证码、GORM、Swaggo、Host-based Tenant Resolver、默认管理员和默认密码；
- Go unit、contract、PostgreSQL integration 和跨 Tenant attack tests 分别执行并记录结果。

健康检查、静态扫描或少量 focused tests 不能替代上述完整 Foundation 验收。

## 13. 来源、许可证与升级

- 创建 `THIRD_PARTY_NOTICES.md`，记录上游仓库、commit、许可证、复制文件和修改说明；
- 保留复制代码中要求保留的版权声明；
- 不把上游仓库作为运行期 Git submodule；
- 不自动同步上游 main/master；
- 每次引入上游更新都先生成 diff，再审查认证、Tenant、网络、存储和依赖变化；
- shadcn 官方组件更新与 `shadcn-admin` 定制更新分开处理；
- 生成代码、上游代码和本地业务代码使用清晰目录与文件头区分；
- 上游安全公告不会自动修复本地副本，依赖与复制代码都必须进入本平台的更新流程。

## 14. 权衡与失效条件

### 14.1 获得的收益

- 避免从零设计控制台交互和 Go 进程骨架；
- 保留 React/shadcn 的品牌控制力；
- 保留 Gin 生态，同时通过 strict OpenAPI 限制 Transport 漂移；
- 不继承第二套身份、权限和 Tenant 事实模型；
- 上游更新不会直接破坏平台安全边界。

### 14.2 付出的成本

- 白名单移植比直接 Fork 需要更多初始判断；
- 后续上游更新必须人工合并；
- `go-admin` 的大部分现成功能不会被复用；
- shadcn 组件源码由本团队维护；
- Gin、React Router 与两个上游的默认组合不同，需要自己的集成测试。

### 14.3 何时该设计不再适用

- 如果产品退化为仅内部使用的单租户管理后台，完整 `go-admin` Fork 会更快；
- 如果 Portal 不再需要 SEO，保留 `shadcn-admin` 的 TanStack Router SPA 会更简单；
- 如果决定放弃 Keycloak/OpenFGA 并采用自有 JWT/RBAC，需要重新进行安全和领域设计，不能通过本抽取方案顺带改变；
- 如果团队无法承担上游差异维护，应选择稳定框架的干净脚手架，而不是继续扩大复制范围。

## 15. 实施边界

本设计只授权后续计划搭建前后端 Foundation。它不包含：

- Tenant、Membership、Product Binding、Billing 或 Usage 的完整业务实现；
- Keycloak、OpenFGA、Temporal、OpenMeter 的生产集成；
- 微信、支付宝支付链；
- WeKnora Shared Cell 加固；
- 生产 Kubernetes、HA 或灾难恢复；
- 上游演示功能迁移。

后续实施计划必须把前端 Foundation、后端 Foundation 和最小 Session/API 集成拆成独立可审查任务，并分别提供测试与许可证证据。
