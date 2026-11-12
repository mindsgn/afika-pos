import { formatZAR } from './money';
import { CryptoMethod, formatCrypto } from './payment';

export interface ReceiptInput {
  receiptNo: string;
  createdAt: number;
  status: string;
  customerName?: string | null;
  items: { name: string; qty: number; unitPrice: number; lineDiscount: number }[];
  subtotal: number;
  discountAmount: number;
  pointsDiscount: number;
  total: number;
  paymentMethod: string;
  cryptoAmount: number;
  txHash?: string | null;
  failureReason?: string | null;
  pointsEarned: number;
  pointsRedeemed: number;
  refundedTotal: number;
}

export interface ReceiptOptions {
  width?: number;
  merchant?: string;
  /** Merchant website printed under the business name. */
  website?: string;
  /** Merchant address printed under the business name. */
  address?: string;
}

const pad2 = (n: number) => n.toString().padStart(2, '0');

export function formatDateTime(ts: number): string {
  const d = new Date(ts);
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

function twoCol(left: string, right: string, width: number): string {
  const space = width - right.length - 1;
  const l = left.length > space ? left.slice(0, Math.max(0, space - 1)) + '…' : left;
  return l + ' '.repeat(Math.max(1, width - l.length - right.length)) + right;
}

const center = (s: string, width: number) =>
  ' '.repeat(Math.max(0, Math.floor((width - s.length) / 2))) + s;

const STATUS_BANNER: Record<string, string> = {
  pending: '*** AWAITING PAYMENT ***',
  voided: '*** VOIDED ***',
  refunded: '*** REFUNDED ***',
  partially_refunded: '* PARTIALLY REFUNDED *',
  failed: '*** PAYMENT FAILED ***',
};

export function buildReceipt(r: ReceiptInput, opts: ReceiptOptions = {}): string {
  const width = opts.width ?? 40;
  const rule = '-'.repeat(width);
  const out: string[] = [];
  out.push(center((opts.merchant ?? 'CRYPTO POS').toUpperCase(), width));
  if (opts.address) out.push(center(opts.address, width));
  if (opts.website) out.push(center(opts.website, width));
  out.push(center('Receipt', width));
  out.push(rule);
  out.push(twoCol('Receipt', r.receiptNo, width));
  out.push(twoCol('Date', formatDateTime(r.createdAt), width));
  if (r.customerName) out.push(twoCol('Customer', r.customerName, width));
  if (STATUS_BANNER[r.status]) out.push(center(STATUS_BANNER[r.status], width));
  out.push(rule);
  for (const it of r.items) {
    out.push(twoCol(`${it.qty} x ${it.name}`, formatZAR(it.qty * it.unitPrice), width));
    if (it.lineDiscount > 0) out.push(twoCol('   discount', `-${formatZAR(it.lineDiscount)}`, width));
  }
  out.push(rule);
  out.push(twoCol('Subtotal', formatZAR(r.subtotal), width));
  if (r.discountAmount > 0) out.push(twoCol('Discount', `-${formatZAR(r.discountAmount)}`, width));
  if (r.pointsDiscount > 0) {
    out.push(twoCol(`Points (${r.pointsRedeemed})`, `-${formatZAR(r.pointsDiscount)}`, width));
  }
  out.push(twoCol('TOTAL', formatZAR(r.total), width));
  if (r.refundedTotal > 0) {
    out.push(twoCol('Refunded', `-${formatZAR(r.refundedTotal)}`, width));
    out.push(twoCol('Net', formatZAR(r.total - r.refundedTotal), width));
  }
  out.push(rule);
  const method = r.paymentMethod as CryptoMethod;
  out.push(twoCol('Paid with', formatCrypto(r.cryptoAmount, method), width));
  if (r.txHash) {
    out.push('Tx:');
    for (let i = 0; i < r.txHash.length; i += width) out.push(r.txHash.slice(i, i + width));
  }
  if (r.failureReason) out.push(`Reason: ${r.failureReason}`);
  if (r.status !== 'failed' && r.status !== 'voided') {
    out.push(rule);
    out.push(twoCol('Points earned', String(r.pointsEarned), width));
  }
  out.push(rule);
  out.push(center('Thank you!', width));
  return out.join('\n');
}

export interface RefundReceiptInput {
  receiptNo: string;
  refundedAt: number;
  items: { name: string; qty: number; amount: number }[];
  amount: number;
  reason?: string | null;
  restocked: boolean;
}

export function buildRefundReceipt(r: RefundReceiptInput, opts: ReceiptOptions = {}): string {
  const width = opts.width ?? 40;
  const rule = '-'.repeat(width);
  const out: string[] = [];
  out.push(center((opts.merchant ?? 'CRYPTO POS').toUpperCase(), width));
  if (opts.address) out.push(center(opts.address, width));
  if (opts.website) out.push(center(opts.website, width));
  out.push(center('REFUND / RETURN', width));
  out.push(rule);
  out.push(twoCol('Original receipt', r.receiptNo, width));
  out.push(twoCol('Date', formatDateTime(r.refundedAt), width));
  out.push(rule);
  for (const it of r.items) out.push(twoCol(`${it.qty} x ${it.name}`, formatZAR(it.amount), width));
  out.push(rule);
  out.push(twoCol('TOTAL REFUND', formatZAR(r.amount), width));
  out.push(twoCol('Returned to stock', r.restocked ? 'Yes' : 'No', width));
  if (r.reason) out.push(`Reason: ${r.reason}`);
  return out.join('\n');
}
