import js from '@eslint/js'
import { defineConfig, globalIgnores } from 'eslint/config'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'

export default defineConfig([
  globalIgnores([
    'build',
    'node_modules',
    'playwright-report',
    'test-results',
    'pnpm-lock.yaml',
    '.react-router',
  ]),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [js.configs.recommended, tseslint.configs.recommended],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
      parserOptions: {
        ecmaFeatures: { jsx: true },
      },
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      // React Router 路由模块与 shadcn 组件需要同时导出组件与路由/变体常量
      'react-refresh/only-export-components': [
        'warn',
        {
          allowConstantExport: true,
          allowExportNames: ['meta', 'links', 'buttonVariants'],
        },
      ],
    },
  },
  {
    // 组件双层边界（issue #107）：components/ui 是纯 shadcn 原语层，
    // 禁止反向依赖 features、routes、API 客户端或会话模块。
    files: ['src/components/ui/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          // 按“路径段”匹配 features / routes 目录（相对导入与别名导入均命中），
          // 以及 api-client / session 模块名。
          patterns: [
            {
              regex: '(^|[/.])(?:features|routes|api-client|session)(?:/|$)',
              message: 'UI 原语必须保持纯净：不得导入 features、routes、API 客户端或会话模块。',
            },
          ],
        },
      ],
    },
  },
])
