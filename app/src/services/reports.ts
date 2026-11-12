import { and, eq, gte, inArray, lte } from 'drizzle-orm';
import { refundItems, refunds, sales } from '@/db/schema';
import type { PosDb } from '@/db/types';

export interface ProfitSummary {
  salesCount: number;
  grossRevenue: number;
  refundedTotal: number;
  netRevenue: number;
  cogs: number;
  profit: number;
  marginPct: number;
  refundCount: number;
  voidedCount: number;
  failedCount: number;
  pointsIssued: number;
  byMethod: { method: string; count: number; revenue: number }[];
}

const SOLD = ['completed', 'partially_refunded', 'refunded'];

/**
 * Profit = net revenue (after refunds) - cost of goods actually kept by customers.
 * Cost of restocked refund items is added back; cost of non-restocked (written-off) items is not.
 */
export function profitSummary(db: PosDb, range: { from?: number; to?: number } = {}): ProfitSummary {
  const conds = [];
  if (range.from != null) conds.push(gte(sales.createdAt, range.from));
  if (range.to != null) conds.push(lte(sales.createdAt, range.to));
  const all = db.select().from(sales).where(conds.length ? and(...conds) : undefined).all();

  const sold = all.filter((s) => SOLD.includes(s.status));
  const soldIds = sold.map((s) => s.id);

  const recovered = soldIds.length
    ? db
        .select({ cost: refundItems.costRecovered })
        .from(refundItems)
        .innerJoin(refunds, eq(refundItems.refundId, refunds.id))
        .where(inArray(refunds.saleId, soldIds))
        .all()
        .reduce((s, r) => s + r.cost, 0)
    : 0;
  const refundCount = soldIds.length
    ? db.select().from(refunds).where(inArray(refunds.saleId, soldIds)).all().length
    : 0;

  const grossRevenue = sold.reduce((s, x) => s + x.total, 0);
  const refundedTotal = sold.reduce((s, x) => s + x.refundedTotal, 0);
  const cogs = sold.reduce((s, x) => s + x.costTotal, 0) - recovered;
  const netRevenue = grossRevenue - refundedTotal;
  const profit = netRevenue - cogs;

  const methods = new Map<string, { count: number; revenue: number }>();
  for (const s of sold) {
    const m = methods.get(s.paymentMethod) ?? { count: 0, revenue: 0 };
    m.count += 1;
    m.revenue += s.total - s.refundedTotal;
    methods.set(s.paymentMethod, m);
  }

  return {
    salesCount: sold.length,
    grossRevenue,
    refundedTotal,
    netRevenue,
    cogs,
    profit,
    marginPct: netRevenue > 0 ? Math.round((profit / netRevenue) * 1000) / 10 : 0,
    refundCount,
    voidedCount: all.filter((s) => s.status === 'voided').length,
    failedCount: all.filter((s) => s.status === 'failed').length,
    pointsIssued: sold.reduce((s, x) => s + x.pointsEarned, 0),
    byMethod: [...methods.entries()].map(([method, v]) => ({ method, ...v })),
  };
}
