import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import LoginView from './LoginView.vue'
import { api, ApiError } from '../api'

async function mountAt(url: string) {
  setActivePinia(createPinia())
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/giris', component: LoginView }, { path: '/:rest(.*)*', component: { template: '<p/>' } }] })
  await router.push(url)
  const w = mount(LoginView, { global: { plugins: [router] } })
  await flushPromises()
  return { w, router }
}

describe('LoginView', () => {
  it('logs in and goes on to the page it was sent from', async () => {
    const login = vi.spyOn(api, 'login').mockResolvedValue({ user_id: 'u', email: 'a@b.c', tenant_id: 't', tenant_name: 'T' } as never)
    const { w, router } = await mountAt('/giris?next=/faturalar')
    await w.find('input[type=email]').setValue('a@b.c')
    await w.find('input[type=password]').setValue('dogru-sifre')
    await w.find('form#giris').trigger('submit')
    await flushPromises()
    expect(login).toHaveBeenCalledWith('a@b.c', 'dogru-sifre')
    expect(router.currentRoute.value.fullPath).toBe('/faturalar')
  })

  it('says why a login failed', async () => {
    vi.spyOn(api, 'login').mockRejectedValue(new ApiError(401, 'INVALID_LOGIN', 'invalid'))
    const { w } = await mountAt('/giris')
    await w.find('input[type=email]').setValue('a@b.c')
    await w.find('input[type=password]').setValue('yanlis')
    await w.find('form#giris').trigger('submit')
    await flushPromises()
    expect(w.find('[role=alert]').text()).toBe('E-posta ya da şifre yanlış.')
  })
})
