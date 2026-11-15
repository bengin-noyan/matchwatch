import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import PasswordLinkView from './PasswordLinkView.vue'
import { api, ApiError } from '../api'

async function mountAt(url: string) {
  setActivePinia(createPinia())
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/davet', component: PasswordLinkView }, { path: '/sifre', component: PasswordLinkView }, { path: '/:rest(.*)*', component: { template: '<p/>' } }],
  })
  await router.push(url)
  const w = mount(PasswordLinkView, { global: { plugins: [router] } })
  await flushPromises()
  return { w, router }
}

describe('PasswordLinkView', () => {
  it('opens an invited account with the token from the fragment', async () => {
    const info = vi.spyOn(api, 'linkInfo').mockResolvedValue({ kind: 'INVITE', email: 'yeni@firma.com.tr', tenant_name: 'Firma A.Ş.' })
    const redeem = vi.spyOn(api, 'redeem').mockResolvedValue({ email: 'yeni@firma.com.tr', tenant_name: 'Firma A.Ş.' })
    const { w, router } = await mountAt('/davet#abc123')
    expect(info).toHaveBeenCalledWith('abc123')
    expect(w.text()).toContain('Firma A.Ş. seni matchwatch')
    await w.find('input[autocomplete=new-password]').setValue('uzun-bir-sifre')
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(redeem).toHaveBeenCalledWith('abc123', 'uzun-bir-sifre')
    expect(router.currentRoute.value.fullPath).toBe('/')
  })

  it('says when a link is used or expired', async () => {
    vi.spyOn(api, 'linkInfo').mockRejectedValue(new ApiError(404, 'INVALID_LINK', 'gone'))
    const { w } = await mountAt('/sifre#eski')
    expect(w.text()).toContain('Bağlantı geçersiz')
    expect(w.find('form').exists()).toBe(false)
  })

  it('treats a link without a token as invalid', async () => {
    const info = vi.spyOn(api, 'linkInfo')
    const { w } = await mountAt('/sifre')
    expect(info).not.toHaveBeenCalled()
    expect(w.text()).toContain('Bağlantı geçersiz')
  })
})
