import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import SignupView from './SignupView.vue'
import { api, ApiError } from '../api'

async function mountAt(url: string) {
  setActivePinia(createPinia())
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/kayit', component: SignupView }, { path: '/:rest(.*)*', component: { template: '<p/>' } }] })
  await router.push(url)
  const w = mount(SignupView, { global: { plugins: [router] } })
  await flushPromises()
  return { w, router }
}

async function fill(w: Awaited<ReturnType<typeof mountAt>>['w']) {
  await w.find('input[autocomplete=organization]').setValue('Yeni Firma A.Ş.')
  await w.find('input[inputmode=numeric]').setValue(' 1234567890 ')
  await w.find('input[type=email]').setValue('a@b.c')
  await w.find('input[autocomplete=new-password]').setValue('uzun-bir-sifre')
  await w.find('form#kayit').trigger('submit')
  await flushPromises()
}

describe('SignupView', () => {
  it('opens the account and goes to the desk', async () => {
    const signup = vi.spyOn(api, 'signup').mockResolvedValue({ email: 'a@b.c', tenant_name: 'Yeni Firma A.Ş.' } as never)
    const { w, router } = await mountAt('/kayit')
    expect(w.find('a[href="/giris"]').exists()).toBe(true)
    await fill(w)
    expect(signup).toHaveBeenCalledWith({ company: 'Yeni Firma A.Ş.', vkn: '1234567890', email: 'a@b.c', password: 'uzun-bir-sifre' })
    expect(router.currentRoute.value.fullPath).toBe('/')
  })

  it('says why a sign-up failed', async () => {
    vi.spyOn(api, 'signup').mockRejectedValue(new ApiError(409, 'EMAIL_TAKEN', 'taken'))
    const { w } = await mountAt('/kayit')
    await fill(w)
    expect(w.find('[role=alert]').text()).toContain('zaten bir hesap var')
  })
})
