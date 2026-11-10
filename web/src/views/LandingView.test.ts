import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import LandingView from './LandingView.vue'

async function mountAt(url: string) {
  setActivePinia(createPinia())
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/tanitim', component: LandingView }, { path: '/:rest(.*)*', component: { template: '<p/>' } }] })
  await router.push(url)
  const w = mount(LandingView, { global: { plugins: [router] } })
  await flushPromises()
  return { w, router }
}

describe('LandingView', () => {
  it('tells what matchwatch does: steps, every rule, the facts', async () => {
    const { w } = await mountAt('/tanitim')
    expect(w.find('h1').text()).toContain('Sözleşmede ne yazıyorsa')
    expect(w.findAll('#nasil li')).toHaveLength(3)
    expect(w.findAll('#kurallar li')).toHaveLength(9)
    expect(w.text()).toContain('Sözleşmeden yüksek fiyat')
    expect(w.text()).toContain('52 GİB tevkifat kodu') // jsdom has no IntersectionObserver: numbers show final values
    expect(w.find('svg[role=img] title').text()).toContain('470,00 TL')
    expect(w.find('form').exists()).toBe(false)
    expect(w.findAll('a[href="/giris"]').length).toBeGreaterThan(0)
    expect(w.findAll('a[href="/kayit"]').length).toBeGreaterThan(0)
    expect(w.findAll('[role=tab]')).toHaveLength(3)
    expect(w.findAll('#sss details').length).toBeGreaterThan(3)
  })
})
