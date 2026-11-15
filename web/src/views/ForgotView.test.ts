import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ForgotView from './ForgotView.vue'
import { api, ApiError } from '../api'

async function submit(email: string) {
  const w = mount(ForgotView, { global: { stubs: { RouterLink: true } } })
  await w.find('input[type=email]').setValue(email)
  await w.find('form').trigger('submit')
  await flushPromises()
  return w
}

describe('ForgotView', () => {
  it('asks for a reset mail and says the same thing whether or not the account exists', async () => {
    const forgot = vi.spyOn(api, 'forgot').mockResolvedValue()
    const w = await submit('ben@firma.com.tr')
    expect(forgot).toHaveBeenCalledWith('ben@firma.com.tr')
    expect(w.find('[role=status]').text()).toContain('ben@firma.com.tr ile bir hesap varsa')
  })

  it('says when there were too many requests', async () => {
    vi.spyOn(api, 'forgot').mockRejectedValue(new ApiError(429, 'TOO_MANY_RESETS', 'slow down'))
    const w = await submit('ben@firma.com.tr')
    expect(w.find('[role=alert]').text()).toContain('Çok fazla sıfırlama istendi')
  })
})
