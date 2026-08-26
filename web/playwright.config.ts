import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: 'tests',
  // Vitest owns tests/**/*.test.tsx; Playwright owns only the .spec.ts smoke files.
  testMatch: '**/*.spec.ts',
  retries: 0,
  reporter: [['list']],
  outputDir: 'test-results',
  use: {
    baseURL: 'http://127.0.0.1:4173',
  },
  webServer: {
    command: 'node scripts/serve.mjs',
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: false,
    timeout: 30_000,
  },
  projects: [
    {
      // 桌面项目：跳过带 @mobile 标签的移动视口用例
      name: 'chromium',
      grepInvert: /@mobile/,
      use: { browserName: 'chromium' },
    },
    {
      // 移动项目复用 iPhone 13 视口（390×844、触摸、isMobile），仅跑 chromium，
      // 跳过带 @desktop 标签的桌面用例；无标签用例（如首页冒烟）两个项目都会跑。
      name: 'chromium-mobile',
      grepInvert: /@desktop/,
      use: { ...devices['iPhone 13'], browserName: 'chromium' },
    },
  ],
})
