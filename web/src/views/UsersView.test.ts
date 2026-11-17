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
  it('sends an invite by e-mail and never shows a link', async () => {
    vi.spyOn(api, 'users').mockResolvedValueOnce({ users: [user], invites: [] }).mockResolvedValueOnce({ users: [user], invites: [invite] })
    const create = vi.spyOn(api, 'invite').mockResolvedValue(invite)
    const w = await mountIt()
    await w.find('input[type=email]').setValue(invite.email)
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(create).toHaveBeenCalledWith(invite.email)
    expect(w.find('[role=status]').text()).toContain('Davet yeni@firma.com.tr adresine gönderildi')
    expect(w.html()).not.toContain('#')
    expect(w.text()).toContain('Davet edildi')
    expect(button(w, 'Şifre sıfırlama bağlantısı')).toBeUndefined() // kimse bir iş arkadaşının şifresini sıfırlayamaz
  })

  it('revokes an invite', async () => {
    vi.spyOn(api, 'users').mockResolvedValue({ users: [user], invites: [invite] })
    const revoke = vi.spyOn(api, 'revokeInvite').mockResolvedValue()
    const w = await mountIt()
    await button(w, 'Daveti iptal et')!.trigger('click')
    await flushPromises()
    expect(revoke).toHaveBeenCalledWith('i1')
  })

  it('says when mail is off or the e-mail already has an account', async () => {
    vi.spyOn(api, 'users').mockResolvedValue({ users: [user], invites: [] })
    const create = vi.spyOn(api, 'invite')
    const w = await mountIt()
    for (const [err, text] of [
      [new ApiError(409, 'EMAIL_TAKEN', 'taken'), 'Bu e-postayla zaten bir hesap var.'],
      [new ApiError(503, 'MAIL_DISABLED', 'off'), 'Bu sunucuda e-posta gönderimi kapalı; davet gönderilemiyor.'],
    ] as const) {
      create.mockRejectedValueOnce(err)
      await w.find('input[type=email]').setValue(user.email)
      await w.find('form').trigger('submit')
      await flushPromises()
      expect(w.find('[role=alert]').text()).toBe(text)
    }
  })
})
