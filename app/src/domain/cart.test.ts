import type { CartLine, CartProduct } from './cart';
import { StockError, addToCart, computeTotals, removeLine, setLineQty } from './cart';
import { NO_DISCOUNT } from './discount';

const product = (over: Partial<CartProduct> = {}): CartProduct => ({
  id: 1,
  sku: 'DRK-001',
  name: 'Flat White',
  price: 3800,
  costPrice: 1200,
  stock: 10,
  ...over,
});

describe('addToCart', () => {
  it('adds a new line using the product catalogue values', () => {
    const lines = addToCart([], product());
    expect(lines).toEqual([
      { productId: 1, sku: 'DRK-001', name: 'Flat White', unitPrice: 3800, unitCost: 1200, qty: 1, stock: 10 },
    ]);
  });

  it('increments an existing line', () => {
    const lines = addToCart(addToCart([], product()), product(), 2);
    expect(lines).toHaveLength(1);
    expect(lines[0].qty).toBe(3);
  });

  it('refuses to sell more than the stock on hand', () => {
    expect(() => addToCart([], product({ stock: 2 }), 3)).toThrow(StockError);
    const lines = addToCart([], product({ stock: 2 }), 2);
    expect(() => addToCart(lines, product({ stock: 2 }))).toThrow(StockError);
  });

  it('does not mutate the input array', () => {
    const lines: CartLine[] = [];
    addToCart(lines, product());
    expect(lines).toEqual([]);
  });
});

describe('setLineQty', () => {
  it('updates a quantity within stock', () => {
    const lines = addToCart([], product());
    expect(setLineQty(lines, 1, 4)[0].qty).toBe(4);
  });

  it('removes the line when set to zero or below', () => {
    const lines = addToCart([], product());
    expect(setLineQty(lines, 1, 0)).toEqual([]);
    expect(setLineQty(lines, 1, -2)).toEqual([]);
  });

  it('ignores unknown products and rejects quantities over stock', () => {
    const lines = addToCart([], product());
    expect(setLineQty(lines, 99, 1)).toBe(lines);
    expect(() => setLineQty(lines, 1, 11)).toThrow(StockError);
  });
});

describe('removeLine', () => {
  it('drops only the requested product', () => {
    const lines = addToCart(
      addToCart([], product()),
      product({ id: 2, sku: 'DRK-002', name: 'Tea', price: 2500 }),
    );
    expect(removeLine(lines, 1).map((l) => l.productId)).toEqual([2]);
  });
});

describe('computeTotals', () => {
  it('sums lines and costs', () => {
    const lines = addToCart(
      addToCart([], product()),
      product({ id: 2, price: 2500, costPrice: 800, stock: 5 }),
      2,
    );
    expect(computeTotals(lines, NO_DISCOUNT)).toEqual({
      subtotal: 8800,
      discountAmount: 0,
      pointsDiscount: 0,
      total: 8800,
      costTotal: 2800,
      itemCount: 3,
    });
  });

  it('applies the discount before loyalty points', () => {
    const lines = addToCart([], product(), 2); // 7600
    const totals = computeTotals(lines, { type: 'percent', value: 10 }, 5); // 6840 - 50
    expect(totals.discountAmount).toBe(760);
    expect(totals.pointsDiscount).toBe(50);
    expect(totals.total).toBe(6790);
  });

  it('caps redeemed points at the amount still payable', () => {
    const lines = addToCart([], product({ price: 100, costPrice: 0 }));
    // R1.00 cart, R0.90 fixed discount, 1000 points (= R100) available
    const totals = computeTotals(lines, { type: 'fixed', value: 90 }, 1000);
    expect(totals.pointsDiscount).toBe(10);
    expect(totals.total).toBe(0);
  });

  it('ignores a negative points balance', () => {
    const lines = addToCart([], product());
    const totals = computeTotals(lines, NO_DISCOUNT, -50);
    expect(totals.pointsDiscount).toBe(0);
    expect(totals.total).toBe(3800);
  });

  it('handles an empty cart', () => {
    expect(computeTotals([], NO_DISCOUNT)).toEqual({
      subtotal: 0,
      discountAmount: 0,
      pointsDiscount: 0,
      total: 0,
      costTotal: 0,
      itemCount: 0,
    });
  });
});
