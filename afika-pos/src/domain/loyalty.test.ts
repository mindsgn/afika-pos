import { maxRedeemablePoints, pointsEarned, pointsToReverse, pointsValue } from './loyalty';

describe('pointsEarned', () => {
  it('earns one point per R10 spent', () => {
    expect(pointsEarned(0)).toBe(0);
    expect(pointsEarned(999)).toBe(0);
    expect(pointsEarned(1000)).toBe(1);
    expect(pointsEarned(6_790)).toBe(6);
  });

  it('never earns points on a negative total', () => {
    expect(pointsEarned(-5000)).toBe(0);
  });
});

describe('pointsValue / maxRedeemablePoints', () => {
  it('values a point at 10 cents', () => {
    expect(pointsValue(0)).toBe(0);
    expect(pointsValue(6)).toBe(60);
  });

  it('caps redemption by both balance and payable amount', () => {
    expect(maxRedeemablePoints(100, 10_000)).toBe(100);
    expect(maxRedeemablePoints(100, 60)).toBe(6);
    expect(maxRedeemablePoints(0, 10_000)).toBe(0);
    expect(maxRedeemablePoints(-5, 10_000)).toBe(0);
    expect(maxRedeemablePoints(100, 0)).toBe(0);
  });
});

describe('pointsToReverse', () => {
  it('claws back the refunded share of the earned points', () => {
    expect(pointsToReverse(10, 5000, 10_000)).toBe(5);
    expect(pointsToReverse(10, 10_000, 10_000)).toBe(10);
  });

  it('returns nothing when the original sale had no total', () => {
    expect(pointsToReverse(10, 5000, 0)).toBe(0);
    expect(pointsToReverse(10, 5000, -1)).toBe(0);
  });
});
