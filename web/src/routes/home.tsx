import { Link } from 'react-router'

import { ThemeToggle } from '../components/theme-toggle'
import { Button } from '../components/ui/button'

export default function Home() {
  return (
    <div className="flex min-h-svh flex-col">
      <header className="border-b">
        <div className="mx-auto flex h-16 w-full max-w-5xl items-center justify-between px-6">
          <Link to="/" className="text-lg font-semibold tracking-tight">
            myqypt
          </Link>
          <nav aria-label="主导航" className="flex items-center gap-4 text-sm">
            <Link
              to="/products"
              className="text-muted-foreground transition-colors hover:text-foreground"
            >
              产品
            </Link>
            <Link
              to="/pricing"
              className="text-muted-foreground transition-colors hover:text-foreground"
            >
              价格
            </Link>
            <Button asChild variant="outline">
              <Link to="/app">进入控制台</Link>
            </Button>
            <ThemeToggle />
          </nav>
        </div>
      </header>

      <main className="flex flex-1">
        <section className="mx-auto flex w-full max-w-3xl flex-col items-center px-6 py-24 text-center">
          <h1 className="text-4xl font-bold tracking-tight text-balance sm:text-5xl">
            一个入口，直达平台上的 AI 应用
          </h1>
          <p className="mt-6 max-w-xl text-lg text-muted-foreground">
            myqypt 为独立运营的 AI 产品提供统一平台：用一个账号管理身份与团队，
            一站式完成订阅、用量查看与产品生命周期管理，让个人与企业放心使用每一个 AI 应用。
          </p>
          <div className="mt-10 flex flex-wrap items-center justify-center gap-4">
            <Button asChild size="lg">
              <Link to="/app">立即开始</Link>
            </Button>
            <Button asChild size="lg" variant="outline">
              <Link to="/products">了解产品</Link>
            </Button>
          </div>
        </section>
      </main>

      <footer className="border-t">
        <div className="mx-auto flex w-full max-w-5xl items-center justify-between px-6 py-8 text-sm text-muted-foreground">
          <span>© myqypt</span>
          <span>AI 应用平台</span>
        </div>
      </footer>
    </div>
  )
}
