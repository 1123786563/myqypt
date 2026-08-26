import '@testing-library/jest-dom/vitest'

import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// 未开启 vitest globals 时，需要手动注册 RTL 的自动清理
afterEach(cleanup)

// Radix 浮层（dropdown-menu 的 popper 定位）与 cmdk 列表高度测量依赖
// ResizeObserver，jsdom 未实现该接口，缺失时会抛 ReferenceError（质量评审已复现）。
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver
}

// cmdk 在选中项变化时对选中条目调用 scrollIntoView，jsdom 未实现该方法。
if (typeof Element.prototype.scrollIntoView !== 'function') {
  Element.prototype.scrollIntoView = () => {}
}

// jsdom 未实现 window.matchMedia（外观偏好的“跟随系统”主题需要它），
// 补一个恒为浅色的最小实现。
if (typeof window.matchMedia !== 'function') {
  window.matchMedia = (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })
}

// Node ≥26 自带实验性 localStorage 全局（未传 --localstorage-file 时求值为
// undefined），会在 vitest 的 jsdom 环境里遮蔽 jsdom 自己的存储实现。
// 此处把全局 localStorage 指回 jsdom 实际创建的存储对象（_localStorage），
// 使组件与测试代码可以照常读写；健康环境（浏览器）不受影响。
const jsdomStorage = (globalThis as Record<string, unknown>)._localStorage as Storage | undefined
if (jsdomStorage && typeof globalThis.localStorage === 'undefined') {
  Object.defineProperty(globalThis, 'localStorage', {
    value: jsdomStorage,
    configurable: true,
  })
}
