import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UsersView from './UsersView.vue'
import { api, ApiError } from '../api'

const user = { id: 'u1', email: 'ben@firma.com.tr', created_at: '2026-11-01T09:00:00Z' }
const invite = { id: 'i1', email: 'yeni@firma.com.tr', created_at: '2026-11-15T09:00:00Z', expires_at: '2026-11-22T09:00:00Z' }

async function mountIt() {
  setActivePinia(createPinia())
  const w = mount(UsersView, { global: { stubs: { RouterLink: true } } })
  await flushPromises()
  return w
}
const button = (w: Awaited<ReturnType<typeof mountIt>>, text: string) => w.findAll('button').find((b) => b.text() === text)

describe('UsersView', () => {
  it('invites by e-mail and shows the one-time link', async () => {
    vi.spyOn(api, 'users').mockResolvedValueOnce({ users: [user], invites: [] }).mockResolvedValueOnce({ users: [user], invites: [invite] })
    const create = vi.spyOn(api, 'invite').mockResolvedValue({ id: 'i1', email: invite.email, token: 'tok', expires_at: invite.expires_at })
    const w = await mountIt()
    await w.find('input[type=email]').setValue(invite.email)
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(create).toHaveBeenCalledWith(invite.email)
    expect((w.find('input[aria-label=Bağlantı]').element as HTMLInputElement).value).toBe(`${location.origin}/davet#tok`)
    expect(w.text()).toContain('Davet edildi')
  })

  it('makes a reset link for a user and revokes an invite', async () => {
    vi.spyOn(api, 'users').mockResolvedValue({ users: [user], invites: [invite] })
    const reset = vi.spyOn(api, 'resetLink').mockResolvedValue({ email: user.email, token: 'rst', expires_at: '2026-11-15T10:00:00Z' })
    const revoke = vi.spyOn(api, 'revokeInvite').mockResolvedValue()
    const w = await mountIt()
    await button(w, 'Şifre sıfırlama bağlantısı')!.trigger('click')
    await flushPromises()
    expect(reset).toHaveBeenCalledWith('u1')
    expect((w.find('input[aria-label=Bağlantı]').element as HTMLInputElement).value).toBe(`${location.origin}/sifre#rst`)
    await button(w, 'Daveti iptal et')!.trigger('click')
    await flushPromises()
    expect(revoke).toHaveBeenCalledWith('i1')
  })

  it('says when the e-mail already has an account', async () => {
    vi.spyOn(api, 'users').mockResolvedValue({ users: [user], invites: [] })
    vi.spyOn(api, 'invite').mockRejectedValue(new ApiError(409, 'EMAIL_TAKEN', 'taken'))
    const w = await mountIt()
    await w.find('input[type=email]').setValue(user.email)
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(w.find('[role=alert]').text()).toBe('Bu e-postayla zaten bir hesap var.')
  })
})
