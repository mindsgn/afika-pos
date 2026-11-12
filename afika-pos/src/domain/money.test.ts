import { centsToInput, formatZAR, parseToCents } from './money';

describe('formatZAR', () => {
  it('formats cents with a space thousands separator and two decimals', () => {
    expect(formatZAR(0)).toBe('R0.00');
    expect(formatZAR(5)).toBe('R0.05');
    expect(formatZAR(123_456)).toBe('R1 234.56');
    expect(formatZAR(120_000_000)).toBe('R1 200 000.00');
  });

  it('rounds fractional cents and keeps the sign', () => {
    expect(formatZAR(99.6)).toBe('R1.00');
    expect(formatZAR(-12_305)).toBe('-R123.05');
  });
});

describe('parseToCents', () => {
  it('accepts rands with optional decimals and comma decimals', () => {
    expect(parseToCents('12')).toBe(1200);
    expect(parseToCents('12.5')).toBe(1250);
    expect(parseToCents('12,50')).toBe(1250);
    expect(parseToCents(' 0.07 ')).toBe(7);
  });

  it('rejects anything that is not a plain non-negative amount', () => {
    for (const bad of ['', 'abc', '-1', '1.234', '1,2,3', '.5', '1.', '1 000']) {
      expect(parseToCents(bad)).toBeNull();
    }
  });

  it('round-trips through centsToInput', () => {
    expect(centsToInput(1250)).toBe('12.50');
    expect(parseToCents(centsToInput(120_005))).toBe(120_005);
  });
});
