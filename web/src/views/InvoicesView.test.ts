import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import InvoicesView from './InvoicesView.vue'
import { api, type InvoiceRow } from '../api'
import { statusLabel } from '../labels'

const row = (n: number): InvoiceRow => ({
  id: `id-${n}`, vendor_id: 'v1', vendor_name: 'Demir Bağlantı A.Ş.', invoice_no: `FTR${n}`, issue_date: '2026-10-01',
  due_date: null, currency: 'TRY', payable_minor: 100000, audit_status: 'RED', open_findings: 2,
})

async function mountAt(url: string) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/faturalar', component: InvoicesView }, { path: '/faturalar/:id', component: { template: '<p/>' } }] })
  await router.push(url)
  const w = mount(InvoicesView, { global: { plugins: [router] } })
  await flushPromises()
  return { w, router }
}

describe('InvoicesView', () => {
  it('asks the server for the page and filters in the URL and pages from there', async () => {
    vi.spyOn(api, 'vendors').mockResolvedValue({ items: [] })
    const list = vi.spyOn(api, 'invoices').mockResolvedValue({ items: [row(51), row(52)], total: 120, counts: { RED: 120, GREEN: 30 } })
    const { w, router } = await mountAt('/faturalar?q=demir&status=RED&sort=issue_date&dir=desc&page=2&open=1')
    expect(list).toHaveBeenLastCalledWith({
      q: 'demir', status: 'RED', vendor_id: undefined, from: undefined, to: undefined, open: true, sort: 'issue_date', dir: 'desc', limit: 50, offset: 50,
    })
    expect(w.text()).toContain('51–100 / 120')
    expect(w.text()).toContain('Tümü 150')

    await w.findAll('button').find((b) => b.text() === 'Sonraki')!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.page).toBe('3')
    expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ offset: 100 }))

    // filtre değişince 1. sayfaya dönülür, gerisi korunur
    await w.findAll('button').find((b) => b.text().startsWith(statusLabel.GREEN))!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ q: 'demir', status: 'GREEN', sort: 'issue_date', dir: 'desc', open: '1' })

    await w.findAll('button').find((b) => b.text() === 'Filtreleri temizle')!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ sort: 'issue_date', dir: 'desc' })
  })

  it('ignores junk in the URL', async () => {
    vi.spyOn(api, 'vendors').mockResolvedValue({ items: [] })
    const list = vi.spyOn(api, 'invoices').mockResolvedValue({ items: [], total: 0, counts: {} })
    const { w } = await mountAt('/faturalar?status=BOGUS&sort=ettn&from=yesterday&page=-4&vendor=x')
    expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ status: undefined, sort: undefined, from: undefined, vendor_id: undefined, offset: 0 }))
    expect(w.text()).toContain('Henüz fatura yok')
  })
})
