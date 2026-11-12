/**
 * Shareable online payments.
 *
 * The POS creates a pending sale, asks `backend/api` for a payment request and
 * shows the returned checkout URL as a QR code. A scheduled worker watches the
 * chain and flips the request to `paid`; this service polls the API and then
 * finalises the local sale exactly once.
 */
import { eq } from 'drizzle-orm';
import { business, paymentRequests } from '@/db/schema';
import type { PaymentRequest } from '@/db/schema';
import type { PosDb } from '@/db/types';
import { PaymentOption, ZAR_PER_UNIT, getPaymentOption, toAssetAmount } from '@/domain/payment';
import { PosError } from './errors';
import { completeSale, createPendingSale, failSale, CheckoutInput } from './checkout';

const DEFAULT_API_URL = 'https://afika.phumzile.studio';

export function apiBaseUrl(): string {
  const configured = process.env.EXPO_PUBLIC_API_URL?.trim();
  return (configured || DEFAULT_API_URL).replace(/\/+$/, '');
}

function apiToken(): string {
  return process.env.EXPO_PUBLIC_API_TOKEN?.trim() ?? '';
}

export function checkoutBaseUrl(): string {
  const configured = process.env.EXPO_PUBLIC_CHECKOUT_URL?.trim();
  return (configured || apiBaseUrl()).replace(/\/+$/, '');
}

export function paymentUrlFor(id: string): string {
  return `${checkoutBaseUrl()}/pay/${encodeURIComponent(id)}`;
}

export interface RemotePayment {
  id: string;
  receiptNo: string;
  amountCents: number;
  currency: string;
  status: 'pending' | 'paid' | 'expired' | 'failed';
  asset: string;
  network: string;
  address: string;
  cryptoAmount: string;
  paymentUrl: string;
  merchantName: string;
  txHash?: string;
  createdAt: number;
  updatedAt: number;
  expiresAt: number;
}

interface Envelope<T> {
  data: T;
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = apiToken();
  let response: Response;
  try {
    response = await fetch(`${apiBaseUrl()}${path}`, {
      ...init,
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(init.headers ?? {}),
      },
    });
  } catch {
    throw new PosError('Could not reach the payments service. Check your connection.');
  }
  const text = await response.text();
  let body: unknown = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = null;
    }
  }
  if (!response.ok) {
    const message =
      (body as { error?: { message?: string } } | null)?.error?.message ?? `Payments service error (${response.status})`;
    throw new PosError(message);
  }
  return (body as Envelope<T>)?.data as T;
}

export interface StartPaymentInput extends CheckoutInput {
  optionId: string;
  /** Merchant wallet that receives the funds, when known. */
  receiveAddress?: string;
}

export interface StartedPayment {
  payment: PaymentRequest;
  option: PaymentOption;
  offline: boolean;
}

function localId() {
  return `local_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 10)}`;
}

/**
 * Creates the pending sale and the shareable payment request. If the API is
 * unreachable the request is still recorded locally so the merchant can see
 * the flow; the checkout screen offers a manual confirmation in that case.
 */
export async function startPayment(db: PosDb, input: StartPaymentInput): Promise<StartedPayment> {
  const option = getPaymentOption(input.optionId);
  const sale = createPendingSale(db, input);
  const profile = db.select().from(business).where(eq(business.id, 1)).get();
  const receiveAddress = (input.receiveAddress || profile?.walletAddress || '').trim();
  const cryptoAmount = toAssetAmount(sale.total, option.id);
  const now = Date.now();

  let id = localId();
  let paymentUrl = paymentUrlFor(id);
  let address = receiveAddress;
  let offline = false;

  try {
    const created = await request<RemotePayment>('/v1/pos/payments', {
      method: 'POST',
      body: JSON.stringify({
        receiptNo: sale.receiptNo,
        amountCents: sale.total,
        currency: 'ZAR',
        asset: option.id,
        receiveAddress,
        merchant: {
          name: profile?.name ?? '',
          website: profile?.website ?? '',
          address: profile?.address ?? '',
        },
      }),
    });
    id = created.id;
    paymentUrl = created.paymentUrl || paymentUrlFor(created.id);
    address = created.address || address;
  } catch (error) {
    offline = true;
    if (error instanceof PosError) {
      // keep the sale local; the checkout screen surfaces the warning
    }
  }

  const row = db
    .insert(paymentRequests)
    .values({
      id,
      saleId: sale.saleId,
      receiptNo: sale.receiptNo,
      asset: option.id,
      network: option.network,
      address,
      amountCents: sale.total,
      cryptoAmount: String(cryptoAmount),
      paymentUrl,
      status: 'pending',
      createdAt: now,
      updatedAt: now,
    })
    .returning()
    .get();

  return { payment: row, option, offline };
}

export function getPayment(db: PosDb, id: string): PaymentRequest | undefined {
  return db.select().from(paymentRequests).where(eq(paymentRequests.id, id)).get();
}

/** Marks the local request paid and completes the sale exactly once. */
export function settlePayment(db: PosDb, id: string, txHash: string | null = null, now = Date.now()): PaymentRequest {
  const row = getPayment(db, id);
  if (!row) throw new PosError('Payment request not found');
  if (row.status === 'paid') return row;

  const option = getPaymentOption(row.asset);
  completeSale(db, row.saleId, {
    txHash,
    cryptoAmount: Number(row.cryptoAmount),
    cryptoRate: ZAR_PER_UNIT[option.asset],
    now,
  });
  db.update(paymentRequests)
    .set({ status: 'paid', txHash, updatedAt: now })
    .where(eq(paymentRequests.id, id))
    .run();
  return getPayment(db, id)!;
}

/** Marks the local request expired/failed and releases the pending sale. */
export function abandonPayment(db: PosDb, id: string, reason = 'Payment was not completed', now = Date.now()): PaymentRequest {
  const row = getPayment(db, id);
  if (!row) throw new PosError('Payment request not found');
  if (row.status === 'paid') return row;
  failSale(db, row.saleId, reason);
  db.update(paymentRequests)
    .set({ status: 'failed', updatedAt: now })
    .where(eq(paymentRequests.id, id))
    .run();
  return getPayment(db, id)!;
}

async function fetchRemote(id: string): Promise<RemotePayment | null> {
  if (id.startsWith('local_')) return null;
  try {
    return await request<RemotePayment>(`/v1/pos/payments/${encodeURIComponent(id)}`);
  } catch {
    return null;
  }
}

/**
 * Polls the API for the current status and settles the sale when paid. Returns
 * the freshest local row. Network failures leave the row untouched so the
 * progress screen can retry.
 */
export async function refreshPayment(db: PosDb, id: string): Promise<PaymentRequest> {
  const row = getPayment(db, id);
  if (!row) throw new PosError('Payment request not found');
  if (row.status !== 'pending') return row;

  const remote = await fetchRemote(id);
  if (!remote) return row;

  if (remote.status === 'paid') {
    return settlePayment(db, id, remote.txHash ?? null);
  }
  if (remote.status === 'expired' || remote.status === 'failed') {
    return abandonPayment(db, id, remote.status === 'expired' ? 'Payment link expired' : 'Payment failed');
  }
  return row;
}

/** Dev/demo shortcut: ask the API to mark the request paid, then settle locally. */
export async function simulatePayment(db: PosDb, id: string): Promise<PaymentRequest> {
  const row = getPayment(db, id);
  if (!row) throw new PosError('Payment request not found');
  if (row.status === 'paid') return row;

  if (!id.startsWith('local_')) {
    try {
      const remote = await request<RemotePayment>(`/v1/pos/payments/${encodeURIComponent(id)}/confirm`, {
        method: 'POST',
        body: JSON.stringify({ txHash: `0x${Date.now().toString(16).padStart(64, '0')}` }),
      });
      if (remote.status === 'paid') return settlePayment(db, id, remote.txHash ?? null);
    } catch {
      // fall through to local settlement
    }
  }
  return settlePayment(db, id, `0x${Date.now().toString(16).padStart(64, '0')}`);
}
