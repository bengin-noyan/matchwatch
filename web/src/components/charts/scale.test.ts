import { describe, expect, it } from 'vitest'
import { niceTicks, tickLabel, weeksBetween } from './scale'

describe('niceTicks', () => {
  it('rounds to 1/2/5 steps and covers the max', () => {
    expect(niceTicks(15086)).toEqual([0, 5000, 10000, 15000, 20000])
    expect(niceTicks(1.3)).toEqual([0, 0.5, 1, 1.5])
    expect(niceTicks(0)).toEqual([0])
  })
})

describe('tickLabel', () => {
  it('writes small values in full and large ones compact', () => {
    expect(tickLabel(5000, 9000)).toBe('5.000')
    expect(tickLabel(20000, 20000)).toMatch(/^20\s?B$/)
  })
})

describe('weeksBetween', () => {
  it('lists every Monday, filling empty weeks', () => {
    expect(weeksBetween('2026-03-01', '2026-03-17')).toEqual(['2026-02-23', '2026-03-02', '2026-03-09', '2026-03-16'])
  })
})
