import { describe, expect, it } from 'vitest'
import { formatAmountText, formatDate, formatDecimal, formatMinor } from './money'

describe('formatMinor', () => {
  it('formats kuruş without float math', () => {
    expect(formatMinor(21412, 'TRY')).toBe('214,12 TL')
    expect(formatMinor(5, 'TRY')).toBe('0,05 TL')
    expect(formatMinor(0, 'USD')).toBe('0,00 USD')
    expect(formatMinor(123456789012, 'TRY')).toBe('1.234.567.890,12 TL')
    expect(formatMinor(-500, 'EUR')).toBe('-5,00 EUR')
  })
})

describe('formatDecimal', () => {
  it('drops trailing zeros and groups thousands', () => {
    expect(formatDecimal('1.250000')).toBe('1,25')
    expect(formatDecimal('1000.000000')).toBe('1.000')
    expect(formatDecimal('0.037500')).toBe('0,0375')
    expect(formatDecimal('1000.000000', 2)).toBe('1.000,00')
    expect(formatDecimal('-12.5')).toBe('-12,5')
    expect(formatDecimal('abc')).toBe('abc')
  })
})

describe('formatAmountText', () => {
  it('formats API "<decimal> <CUR>" strings', () => {
    expect(formatAmountText('1.300000 TRY')).toBe('1,30 TL')
    expect(formatAmountText('0.032500 USD')).toBe('0,0325 USD')
    expect(formatAmountText('1000 ADET')).toBe('1000 ADET')
  })
})

describe('formatDate', () => {
  it('turns ISO dates into dd.mm.yyyy', () => {
    expect(formatDate('2026-03-01')).toBe('01.03.2026')
    expect(formatDate(null)).toBe('')
  })
})

describe('formatValue', () => {
  it('humanizes finding values', async () => {
    const { formatValue } = await import('./money')
    expect(formatValue('60 days')).toBe('60 gün')
    expect(formatValue('1.300000 TRY')).toBe('1,30 TL')
    expect(formatValue('1000 ADET')).toBe('1.000 ADET')
    expect(formatValue('1.2500')).toBe('1,25')
    expect(formatValue('601')).toBe('601')
    expect(formatValue('')).toBe('')
  })
})
