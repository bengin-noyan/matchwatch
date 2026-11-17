import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import AppShell from './AppShell.vue'

describe('AppShell', () => {
  it('keeps the phone menu behind a button and closes it on navigation or Escape', async () => {
    setActivePinia(createPinia())
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:rest(.*)*', component: { template: '<p/>' } }] })
    await router.push('/')
    const w = mount(AppShell, { global: { plugins: [router] } })
    const button = w.get('button[aria-controls=ana-menu]')
    const nav = () => w.get('#ana-menu')

    expect(button.attributes('aria-expanded')).toBe('false')
    expect(nav().classes()).toContain('hidden')
    expect(nav().findAll('a')).toHaveLength(8)

    await button.trigger('click')
    expect(button.attributes('aria-expanded')).toBe('true')
    expect(nav().classes()).not.toContain('hidden')

    await router.push('/faturalar')
    await flushPromises()
    expect(button.attributes('aria-expanded')).toBe('false')
    expect(nav().classes()).toContain('hidden')

    await button.trigger('click')
    await w.get('aside').trigger('keydown', { key: 'Escape' })
    expect(button.attributes('aria-expanded')).toBe('false')
  })
})
