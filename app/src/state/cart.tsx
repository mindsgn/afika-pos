import React, { createContext, useCallback, useContext, useState } from 'react';
import type { Customer } from '@/db/schema';
import { CartLine, CartProduct, StockError, addToCart, computeTotals, setLineQty } from '../domain/cart';
import { Discount, NO_DISCOUNT } from '../domain/discount';
import { DEFAULT_PAYMENT_OPTION_ID } from '../domain/payment';

interface CartContextValue {
  lines: CartLine[];
  discount: Discount;
  customer: Customer | null;
  redeemPoints: number;
  /** Payment option id, e.g. `usdc-base`. */
  method: string;
  totals: ReturnType<typeof computeTotals>;
  /** returns an error message, or null on success */
  add: (p: CartProduct) => string | null;
  setQty: (productId: number, qty: number) => string | null;
  setDiscount: (d: Discount) => void;
  setCustomer: (c: Customer | null) => void;
  setRedeemPoints: (n: number) => void;
  setMethod: (m: string) => void;
  clear: () => void;
}

const CartContext = createContext<CartContextValue | null>(null);

export function CartProvider({ children }: { children: React.ReactNode }) {
  const [lines, setLines] = React.useState<CartLine[]>([]);
  const [discount, setDiscount] = useState<Discount>(NO_DISCOUNT);
  const [customer, setCustomerState] = useState<Customer | null>(null);
  const [redeemPoints, setRedeemPoints] = useState(0);
  const [method, setMethod] = useState<string>(DEFAULT_PAYMENT_OPTION_ID);

  const add = useCallback((p: CartProduct) => {
    try {
      setLines((l) => addToCart(l, p));
      return null;
    } catch (e) {
      if (e instanceof StockError) return e.message;
      throw e;
    }
  }, []);

  const setQty = useCallback((productId: number, qty: number) => {
    try {
      setLines((l) => setLineQty(l, productId, qty));
      return null;
    } catch (e) {
      if (e instanceof StockError) return e.message;
      throw e;
    }
  }, []);

  const setCustomer = useCallback((c: Customer | null) => {
    setCustomerState(c);
    setRedeemPoints(0);
  }, []);

  const clear = useCallback(() => {
    setLines([]);
    setDiscount(NO_DISCOUNT);
    setCustomerState(null);
    setRedeemPoints(0);
  }, []);

  const totals = React.useMemo(() => computeTotals(lines, discount, redeemPoints), [lines, discount, redeemPoints]);

  const value: CartContextValue = {
    lines, discount, customer, redeemPoints, method, totals,
    add, setQty, setDiscount, setCustomer, setRedeemPoints, setMethod, clear,
  };
  return <CartContext.Provider value={value}>{children}</CartContext.Provider>;
}

export function useCart(): CartContextValue {
  const ctx = useContext(CartContext);
  if (!ctx) throw new Error('useCart must be used inside CartProvider');
  return ctx;
}
