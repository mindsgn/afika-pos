import { Discount, discountAmount } from './discount';
import { POINT_VALUE_CENTS } from './loyalty';

export interface CartProduct {
  id: number;
  sku: string;
  name: string;
  price: number;
  costPrice: number;
  stock: number;
}

export interface CartLine {
  productId: number;
  sku: string;
  name: string;
  unitPrice: number;
  unitCost: number;
  qty: number;
  stock: number;
}

export class StockError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'StockError';
  }
}

export function addToCart(lines: CartLine[], p: CartProduct, qty = 1): CartLine[] {
  const existing = lines.find((l) => l.productId === p.id);
  const next = (existing?.qty ?? 0) + qty;
  if (next > p.stock) throw new StockError(`Only ${p.stock} of ${p.name} in stock`);
  if (existing) {
    return lines.map((l) => (l.productId === p.id ? { ...l, qty: next, stock: p.stock } : l));
  }
  return [
    ...lines,
    {
      productId: p.id,
      sku: p.sku,
      name: p.name,
      unitPrice: p.price,
      unitCost: p.costPrice,
      qty,
      stock: p.stock,
    },
  ];
}

export function setLineQty(lines: CartLine[], productId: number, qty: number): CartLine[] {
  if (qty <= 0) return lines.filter((l) => l.productId !== productId);
  const line = lines.find((l) => l.productId === productId);
  if (!line) return lines;
  if (qty > line.stock) throw new StockError(`Only ${line.stock} of ${line.name} in stock`);
  return lines.map((l) => (l.productId === productId ? { ...l, qty } : l));
}

export const removeLine = (lines: CartLine[], productId: number): CartLine[] =>
  lines.filter((l) => l.productId !== productId);

export interface CartTotals {
  subtotal: number;
  discountAmount: number;
  pointsDiscount: number;
  total: number;
  costTotal: number;
  itemCount: number;
}

export function computeTotals(
  lines: CartLine[],
  discount: Discount,
  redeemPoints = 0,
): CartTotals {
  const subtotal = lines.reduce((s, l) => s + l.unitPrice * l.qty, 0);
  const costTotal = lines.reduce((s, l) => s + l.unitCost * l.qty, 0);
  const disc = discountAmount(subtotal, discount);
  const pointsDiscount = Math.min(Math.max(0, redeemPoints) * POINT_VALUE_CENTS, subtotal - disc);
  return {
    subtotal,
    discountAmount: disc,
    pointsDiscount,
    total: subtotal - disc - pointsDiscount,
    costTotal,
    itemCount: lines.reduce((n, l) => n + l.qty, 0),
  };
}
