import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import Home from '../src/routes/home'

describe('Home 落地页', () => {
  it('渲染一个可见的一级标题', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <Home />
      </MemoryRouter>,
    )
    expect(screen.getByRole('heading', { level: 1 })).toBeVisible()
  })

  it('导航包含产品与价格链接', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <Home />
      </MemoryRouter>,
    )
    expect(screen.getByRole('link', { name: '产品' })).toHaveAttribute('href', '/products')
    expect(screen.getByRole('link', { name: '价格' })).toHaveAttribute('href', '/pricing')
  })

  it('提供指向控制台的入口链接', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <Home />
      </MemoryRouter>,
    )
    expect(screen.getByRole('link', { name: /进入控制台/ })).toHaveAttribute('href', '/app')
  })
})
