import '@testing-library/jest-dom/vitest'

import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// 未开启 vitest globals 时，需要手动注册 RTL 的自动清理
afterEach(cleanup)
