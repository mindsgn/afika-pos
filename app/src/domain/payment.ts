/**
 * Payment options for the POS.
 *
 * A merchant can charge in Bitcoin or in a stablecoin on a specific chain. The
 * shareable checkout page lets the payer choose one of these options; the app
 * shows a QR code and waits for the receipt worker to observe the transaction.
 */

export type PaymentAsset = 'BTC' | 'USDC' | 'ZARP' | 'EURC';
export type PaymentNetwork = 'bitcoin' | 'base' | 'ethereum';

export interface PaymentOption {
  /** Stable identifier used in URLs and local rows, e.g. `usdc-base`. */
  id: string;
  /** Human label shown in the app and on the web checkout page. */
  label: string;
  asset: PaymentAsset;
  network: PaymentNetwork;
  /** Chain name used by the API/worker. */
  networkLabel: string;
  decimals: number;
}

export const PAYMENT_OPTIONS: PaymentOption[] = [
  { id: 'btc', label: 'Bitcoin', asset: 'BTC', network: 'bitcoin', networkLabel: 'Bitcoin', decimals: 8 },
  { id: 'usdc-base', label: 'USDC · Base', asset: 'USDC', network: 'base', networkLabel: 'Base', decimals: 2 },
  { id: 'usdc-ethereum', label: 'USDC · Ethereum', asset: 'USDC', network: 'ethereum', networkLabel: 'Ethereum', decimals: 6 },
  { id: 'zarp-base', label: 'ZARP · Base', asset: 'ZARP', network: 'base', networkLabel: 'Base', decimals: 2 },
  { id: 'zarp-ethereum', label: 'ZARP · Ethereum', asset: 'ZARP', network: 'ethereum', networkLabel: 'Ethereum', decimals: 6 },
  { id: 'eurc-base', label: 'Euro (EURC) · Base', asset: 'EURC', network: 'base', networkLabel: 'Base', decimals: 2 },
  { id: 'eurc-ethereum', label: 'Euro (EURC) · Ethereum', asset: 'EURC', network: 'ethereum', networkLabel: 'Ethereum', decimals: 6 },
];

export const DEFAULT_PAYMENT_OPTION_ID = 'usdc-base';

export function getPaymentOption(id: string | null | undefined): PaymentOption {
  return PAYMENT_OPTIONS.find((o) => o.id === id) ?? PAYMENT_OPTIONS[1];
}

/** ZAR value of one unit of each asset. A real build would read live rates. */
export const ZAR_PER_UNIT: Record<PaymentAsset, number> = {
  BTC: 1_200_000,
  USDC: 18.5,
  ZARP: 1,
  EURC: 20.1,
};

/** Converts a ZAR amount in cents to the asset amount, rounded to its display precision. */
export function toAssetAmount(amountCents: number, optionId: string): number {
  const option = getPaymentOption(optionId);
  const factor = 10 ** option.decimals;
  // A total can never be negative, so never quote a negative crypto amount.
  const cents = Math.max(0, amountCents);
  return Math.round((cents / 100 / ZAR_PER_UNIT[option.asset]) * factor) / factor;
}

/** Formats an asset amount using the option's symbol and precision. */
export function formatAssetAmount(amount: number, optionId: string): string {
  const option = getPaymentOption(optionId);
  return `${amount.toFixed(option.decimals)} ${option.asset}`;
}

// ---------------------------------------------------------------------------
// Backwards-compatible helpers. Older sales stored a symbol (`USDC`, `BTC`,
// `ETH`) rather than a payment option id, so these resolve that too.
// ---------------------------------------------------------------------------

export type CryptoMethod = string;

const SYMBOL_TO_OPTION: Record<string, string> = {
  BTC: 'btc',
  USDC: 'usdc-base',
  ETH: 'usdc-ethereum',
  ZARP: 'zarp-base',
  EURC: 'eurc-base',
};

/** Resolves a legacy symbol or an option id to an option id. */
export function resolveOptionId(method: string): string {
  if (PAYMENT_OPTIONS.some((o) => o.id === method)) return method;
  return SYMBOL_TO_OPTION[method.toUpperCase()] ?? DEFAULT_PAYMENT_OPTION_ID;
}

export function toCrypto(amountCents: number, method: string): number {
  return toAssetAmount(amountCents, resolveOptionId(method));
}

export function formatCrypto(amount: number, method: string): string {
  return formatAssetAmount(amount, resolveOptionId(method));
}
