import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import FindingPanel from './FindingPanel.vue'
import { api, type DetailLine, type Finding } from '../api'

const line = { id: 'line-uuid', line_id: '1', name: 'AYCICEK YAGI 5LT', uom: 'LT', qty: '10', unit_price: '190.00' } as DetailLine

function finding(over: Partial<Finding>): Finding {
  return {
    id: 'f1', line_id: '1', severity: 'CRITICAL', rule_code: 'PRICE_MISMATCH', expected: '1.250000 TRY', actual: '1.300000 TRY',
    financial_impact_minor: 5000, impact_currency: 'TRY', evidence: {}, resolution_status: 'OPEN', resolution_note: null, decisions: [], ...over,
  }
}

describe('FindingPanel', () => {
  it('shows both sides, the margin impact and records a dispute with its note', async () => {
    const resolve = vi.spyOn(api, 'resolve').mockResolvedValue({ id: 'f1', resolution_status: 'DISPUTED' })
    const w = mount(FindingPanel, {
      props: { finding: finding({ evidence: { contract_no: 'S-1', source_page: 3, source_clause_text: 'Madde 4.1' } }), line, invoiceCurrency: 'TRY', dueDate: null },
    })
    expect(w.text()).toContain('Sözleşme S-1, sayfa 3')
    expect(w.text()).toContain('1,25 TL')
    expect(w.text()).toContain('1,30 TL')
    expect(w.text()).toContain('+50,00 TL')
    await w.find('input').setValue('yazıyla bildirildi')
    await w.findAll('button').find((b) => b.text() === 'İtiraz et')!.trigger('click')
    await flushPromises()
    expect(resolve).toHaveBeenCalledWith('f1', 'DISPUTED', 'yazıyla bildirildi')
    expect(w.emitted('changed')).toHaveLength(1)
  })

  it('offers suggestions for an uncontracted line and confirms the chosen one', async () => {
    const match = vi.spyOn(api, 'matchLine').mockResolvedValue({ invoice_id: 'i', audit_status: 'RED' })
    const w = mount(FindingPanel, {
      props: {
        finding: finding({ rule_code: 'UNCONTRACTED_ITEM', financial_impact_minor: 0, evidence: {
          contract_no: 'S-1', suggestions: [{ contract_item_id: 'item-9', item_name: 'Ayçiçek Yağı 5 L', similarity: '0.8683' }] } }),
        line, invoiceCurrency: 'TRY', dueDate: null,
      },
    })
    expect(w.text()).toContain('Ayçiçek Yağı 5 L')
    expect(w.text()).toContain('benzerlik 0,8683')
    expect(w.text()).not.toContain('İtiraz et') // burada karar, eşleşmenin kendisi
    await w.findAll('button').find((b) => b.text() === 'Bu kalemle eşleştir')!.trigger('click')
    await flushPromises()
    expect(match).toHaveBeenCalledWith('line-uuid', 'item-9')
    expect(w.emitted('changed')).toHaveLength(1)
  })

  it('lists the decision trail oldest first, with who decided and the note', () => {
    const w = mount(FindingPanel, {
      props: {
        finding: finding({ resolution_status: 'OPEN', decisions: [
          { resolution_status: 'DISPUTED', note: 'tedarikçi iade edecek', decided_by_email: 'ayse@firma.com.tr', decided_at: '2026-11-05T11:09:27Z' },
          { resolution_status: 'OPEN', note: null, decided_by_email: null, decided_at: '2026-11-05T12:00:00Z' },
        ] }),
        line, invoiceCurrency: 'TRY', dueDate: null,
      },
    })
    const items = w.find('[aria-label="Karar geçmişi"]').findAll('li').map((li) => li.text())
    expect(items).toEqual([
      '05.11.2026 14:09 · ayse@firma.com.tr itiraz etti: tedarikçi iade edecek',
      '05.11.2026 15:00 · Geliştirici anahtarı kararı geri aldı',
    ])
  })

  it('shows no trail before the first decision', () => {
    const w = mount(FindingPanel, { props: { finding: finding({}), line, invoiceCurrency: 'TRY', dueDate: null } })
    expect(w.find('[aria-label="Karar geçmişi"]').exists()).toBe(false)
  })
})
