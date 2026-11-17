import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import AliasesView from './AliasesView.vue'
import { api, type AliasRow } from '../api'

const VENDOR = '0f6c3d1e-8a5b-4c2d-9e7f-1a2b3c4d5e6f'
const alias: AliasRow = {
  id: 'a1', vendor_id: VENDOR, vendor_name: 'Demir Bağlantı A.Ş.', name_key: 'm8 20mm inox civata', uom: 'ADET',
  created_at: '2026-11-05T11:09:27Z',
  contract_item: { id: 'ci1', contract_no: 'S-2026-001', item_code: 'CV-M8-20', name: 'M8x20 paslanmaz civata', unit_price: '1.250000',
    currency: 'TRY', uom: 'ADET', source_page: 3, source_clause_text: '' },
}

async function mountAt(url: string) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/eslesmeler', component: AliasesView }, { path: '/:rest(.*)*', component: { template: '<p/>' } }] })
  await router.push(url)
  const w = mount(AliasesView, { global: { plugins: [router] } })
  await flushPromises()
  return w
}
const button = (w: Awaited<ReturnType<typeof mountAt>>, text: string) => w.findAll('button').find((b) => b.text() === text)

describe('AliasesView', () => {
  it('lists the matches for the vendor in the URL and removes one after confirming', async () => {
    vi.spyOn(api, 'vendors').mockResolvedValue({ items: [] })
    const list = vi.spyOn(api, 'aliases').mockResolvedValueOnce({ items: [alias] }).mockResolvedValueOnce({ items: [] })
    const del = vi.spyOn(api, 'deleteAlias').mockResolvedValue({ reaudited: ['i1', 'i2'] })
    const w = await mountAt(`/eslesmeler?vendor=${VENDOR}`)
    expect(list).toHaveBeenCalledWith(VENDOR)
    expect(w.text()).toContain('m8 20mm inox civata (ADET)')
    expect(w.text()).toContain('Sözleşme S-2026-001, kod CV-M8-20, 1,25 TL / ADET')

    await button(w, 'Kaldır')!.trigger('click')
    expect(del).not.toHaveBeenCalled() // ilk tıklama yalnızca sorar
    await button(w, 'Vazgeç')!.trigger('click')
    expect(button(w, 'Evet, kaldır')).toBeUndefined()

    await button(w, 'Kaldır')!.trigger('click')
    await button(w, 'Evet, kaldır')!.trigger('click')
    await flushPromises()
    expect(del).toHaveBeenCalledWith('a1')
    expect(w.find('[role=status]').text()).toBe('“m8 20mm inox civata” eşleşmesi kaldırıldı; 2 fatura yeniden denetlendi.')
    expect(w.text()).toContain('Bu tedarikçi için eşleşme yok')
  })

  it('ignores a vendor id that is not a UUID', async () => {
    vi.spyOn(api, 'vendors').mockResolvedValue({ items: [] })
    const list = vi.spyOn(api, 'aliases').mockResolvedValue({ items: [] })
    await mountAt('/eslesmeler?vendor=nope')
    expect(list).toHaveBeenCalledWith(undefined)
  })
})
