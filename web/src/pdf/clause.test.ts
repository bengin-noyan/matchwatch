import { describe, expect, it } from 'vitest'
import { clauseItems } from './clause'

const items = (...s: string[]) => s.map((str) => ({ str }))

describe('clauseItems', () => {
  it('spans the items a clause crosses, ignoring case and spacing', () => {
    const page = items('1. Taraflar', 'Madde 4.1 Birim fiyat', ' 1.250,00 TL', '+ KDV olarak', 'uygulanır.', 'Madde 5')
    expect(clauseItems(page, 'birim fiyat 1.250,00 TL + KDV')).toEqual([1, 2, 3])
  })

  it('handles Turkish capitals and text split mid-word', () => {
    expect(clauseItems(items('ÖDEME VADESİ FATURA TARİ', 'HİNDEN İTİBAREN 60 GÜNDÜR'), 'ödeme vadesi fatura tarihinden itibaren 60 gündür')).toEqual([0, 1])
  })

  it('returns nothing when the clause is not on the page or is blank', () => {
    expect(clauseItems(items('Madde 4.1 Birim fiyat 1.250,00 TL'), 'Birim fiyat 1.300,00 TL')).toEqual([])
    expect(clauseItems(items('Madde 4.1'), '  ')).toEqual([])
  })
})
