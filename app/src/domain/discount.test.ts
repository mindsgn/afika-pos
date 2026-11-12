import { NO_DISCOUNT, allocate, discountAmount } from './discount';

describe('discountAmount', () => {
  it('returns nothing for no discount', () => {
    expect(discountAmount(10_000, NO_DISCOUNT)).toBe(0);
  });

  it('applies percentages rounded to the cent and clamps to 0-100', () => {
    expect(discountAmount(10_000, { type: 'percent', value: 10 })).toBe(1000);
    expect(discountAmount(9_995, { type: 'percent', value: 12.5 })).toBe(1249);
    expect(discountAmount(10_000, { type: 'percent', value: 250 })).toBe(10_000);
    expect(discountAmount(10_000, { type: 'percent', value: -5 })).toBe(0);
  });

  it('applies fixed discounts without exceeding the subtotal', () => {
    expect(discountAmount(10_000, { type: 'fixed', value: 2500 })).toBe(2500);
    expect(discountAmount(10_000, { type: 'fixed', value: 25_000 })).toBe(10_000);
    expect(discountAmount(10_000, { type: 'fixed', value: -100 })).toBe(0);
  });
});

describe('allocate', () => {
  it('splits a total proportionally and sums back exactly', () => {
    const parts = allocate(1000, [1, 1, 1]);
    expect(parts.reduce((a, b) => a + b, 0)).toBe(1000);
    expect(parts).toEqual([334, 333, 333]);
  });

  it('weights by line value', () => {
    expect(allocate(1000, [300, 700])).toEqual([300, 700]);
  });

  it('returns zeros for empty or non-positive inputs', () => {
    expect(allocate(0, [1, 2])).toEqual([0, 0]);
    expect(allocate(-100, [1, 2])).toEqual([0, 0]);
    expect(allocate(1000, [0, 0])).toEqual([0, 0]);
    expect(allocate(1000, [])).toEqual([]);
  });

  it('gives remainders to the largest fractional shares first', () => {
    expect(allocate(10, [1, 1, 1, 1])).toEqual([3, 3, 2, 2]);
  });
});
