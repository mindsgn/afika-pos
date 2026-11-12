export type Discount =
  | { type: 'none' }
  | { type: 'percent'; value: number }
  | { type: 'fixed'; value: number }; // fixed value in cents

export const NO_DISCOUNT: Discount = { type: 'none' };

export function discountAmount(subtotal: number, d: Discount): number {
  switch (d.type) {
    case 'none':
      return 0;
    case 'percent': {
      const pct = Math.min(100, Math.max(0, d.value));
      return Math.round((subtotal * pct) / 100);
    }
    case 'fixed':
      return Math.min(Math.max(0, Math.round(d.value)), subtotal);
  }
}

/** Splits `total` cents across `weights` proportionally (largest remainder), always summing exactly to `total`. */
export function allocate(total: number, weights: number[]): number[] {
  const sum = weights.reduce((a, b) => a + b, 0);
  if (total <= 0 || sum <= 0) return weights.map(() => 0);
  const raw = weights.map((w) => (total * w) / sum);
  const floors = raw.map((r) => Math.floor(r));
  let remainder = total - floors.reduce((a, b) => a + b, 0);
  const order = raw
    .map((r, i) => ({ i, frac: r - Math.floor(r) }))
    .sort((a, b) => b.frac - a.frac);
  for (let k = 0; remainder > 0 && k < order.length; k++, remainder--) {
    floors[order[k].i] += 1;
  }
  return floors;
}
