import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import FindingsView from './FindingsView.vue'
import { api, type FindingListRow } from '../api'

const row = (n: number, over: Partial<FindingListRow> = {}): FindingListRow => ({
  id: `f-${n}`, invoice_id: `i-${n}`, invoice_no: `FTR${n}`, vendor_name: 'Demir Bağlantı A.Ş.', issue_date: '2026-10-01',
  currency: 'TRY', line_id: '1', item_name: 'M8x20 civata', severity: 'CRITICAL', rule_code: 'PRICE_MISMATCH',
  expected: '1.250000 TRY', actual: '1.350000 TRY', financial_impact_minor: 47000, impact_currency: 'TRY',
  resolution_status: 'OPEN', resolution_note: null, ...over,
})

async function mountAt(url: string) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/bulgular', component: FindingsView }, { path: '/faturalar/:id', component: { template: '<p/>' } }] })
  await router.push(url)
  const w = mount(FindingsView, { global: { plugins: [router] } })
  await flushPromises()
  return { w, router }
}
const button = (w: Awaited<ReturnType<typeof mountAt>>['w'], text: string) => w.findAll('button').find((b) => b.text().startsWith(text))!

describe('FindingsView', () => {
  it('lists open findings by default and filters from the URL', async () => {
    vi.spyOn(api, 'vendors').mockResolvedValue({ items: [] })
    const list = vi.spyOn(api, 'findings').mockResolvedValue({ items: [row(1)], total: 1, counts: { PRICE_MISMATCH: 1, FX_UNVERIFIED: 4 } })
    const { w, router } = await mountAt('/bulgular?page=2&vendor=nope')
    expect(list).toHaveBeenLastCalledWith({ resolution: 'OPEN', rule: undefined, vendor_id: undefined, limit: 50, offset: 50 })
    expect(w.text()).toContain('Bütün kurallar 5')
    expect(w.text()).toContain('470,00 TL') // the impact
    await button(w, 'Kur doğrulanamadı').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ rule: 'FX_UNVERIFIED', vendor: 'nope' }) // page resets; junk stays inert like on Faturalar
    await button(w, 'Tümü').trigger('click')
    await flushPromises()
    expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ resolution: undefined, rule: 'FX_UNVERIFIED', offset: 0 }))
  })

  it('decides on the ticked findings with one note and reloads', async () => {
    vi.spyOn(api, 'vendors').mockResolvedValue({ items: [] })
    const list = vi.spyOn(api, 'findings').mockResolvedValue({ items: [row(1), row(2), row(3)], total: 3, counts: { PRICE_MISMATCH: 3 } })
    const decide = vi.spyOn(api, 'decideMany').mockResolvedValue({ updated: 2 })
    const { w } = await mountAt('/bulgular')
    expect(w.find('form[aria-label="Seçili bulgular için karar"]').exists()).toBe(false)

    const boxes = w.findAll('tbody input[type=checkbox]')
    await boxes[0]!.setValue(true)
    await boxes[2]!.setValue(true)
    const head = w.find('thead input[type=checkbox]').element as HTMLInputElement
    expect(head.indeterminate).toBe(true)
    expect(w.text()).toContain('2 bulgu seçildi')
    expect(w.text()).not.toContain('Kararı geri al') // nothing selected is decided yet

    await w.find('form[aria-label="Seçili bulgular için karar"] input').setValue('sözleşme eki ile uyumlu')
    await button(w, 'İstisna olarak onayla').trigger('click')
    await flushPromises()
    expect(decide).toHaveBeenCalledWith(['f-1', 'f-3'], 'APPROVED_EXCEPTION', 'sözleşme eki ile uyumlu')
    expect(list).toHaveBeenCalledTimes(2)
    expect(w.find('[role=status]').text()).toBe('2 bulgu için karar kaydedildi.')
    expect(w.find('form[aria-label="Seçili bulgular için karar"]').exists()).toBe(false)

    // the header box ticks the whole page
    await w.find('thead input[type=checkbox]').setValue(true)
    expect(w.text()).toContain('3 bulgu seçildi')
  })
})
