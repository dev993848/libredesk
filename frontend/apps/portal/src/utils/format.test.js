import { describe, expect, it } from 'vitest'
import { formatDuration } from './format'

const labels = {
  'portal.time.hour': 'h',
  'portal.time.minute': 'm',
  'portal.time.lessThanMinute': 'less than a minute',
}
const t = (key) => labels[key] ?? key

describe('formatDuration', () => {
  it('renders zero explicit', () => {
    expect(formatDuration(0, t)).toBe('0 m')
  })

  it('renders sub-minute durations without rounding up to a minute', () => {
    expect(formatDuration(20, t)).toBe('less than a minute')
    expect(formatDuration(29, t)).toBe('less than a minute')
  })

  it('rounds to the nearest minute', () => {
    expect(formatDuration(89, t)).toBe('1 m')
    expect(formatDuration(1803, t)).toBe('30 m')
  })

  it('carries a rounded hour instead of showing sixty minutes', () => {
    expect(formatDuration(3599, t)).toBe('1 h')
  })

  it('renders hours and minutes together', () => {
    expect(formatDuration(5400, t)).toBe('1 h 30 m')
  })
})
