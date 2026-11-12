import {
  DEFAULT_PAYMENT_OPTION_ID,
  PAYMENT_OPTIONS,
  ZAR_PER_UNIT,
  formatAssetAmount,
  formatCrypto,
  getPaymentOption,
  resolveOptionId,
  toAssetAmount,
  toCrypto,
} from './payment';

describe('payment options', () => {
  it('exposes bitcoin plus every stablecoin/chain combination', () => {
    const ids = PAYMENT_OPTIONS.map((o) => o.id);
    expect(ids).toEqual([
      'btc',
      'usdc-base',
      'usdc-ethereum',
      'zarp-base',
      'zarp-ethereum',
      'eurc-base',
      'eurc-ethereum',
    ]);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('gives every option a positive rate and sane precision', () => {
    for (const option of PAYMENT_OPTIONS) {
      expect(ZAR_PER_UNIT[option.asset]).toBeGreaterThan(0);
      expect(option.decimals).toBeGreaterThanOrEqual(2);
      expect(option.decimals).toBeLessThanOrEqual(8);
      expect(option.label.length).toBeGreaterThan(0);
    }
  });

  it('falls back to the default option for unknown ids', () => {
    expect(getPaymentOption('nope').id).toBe('usdc-base');
    expect(getPaymentOption(undefined).id).toBe(DEFAULT_PAYMENT_OPTION_ID);
    expect(getPaymentOption('btc').asset).toBe('BTC');
  });
});

describe('toAssetAmount', () => {
  it('converts ZAR cents into the asset at the published rate', () => {
    // R370.00 at R18.50 per USDC
    expect(toAssetAmount(37_000, 'usdc-base')).toBe(20);
    // R1 200 000.00 buys 1 BTC
    expect(toAssetAmount(120_000_000, 'btc')).toBe(1);
    // ZARP tracks the rand one to one
    expect(toAssetAmount(12_345, 'zarp-base')).toBe(123.45);
    expect(toAssetAmount(2_010, 'eurc-base')).toBe(1);
  });

  it('rounds to the precision advertised by the option', () => {
    // R0.01 is a third of a satoshi at R1.2m/BTC, so it rounds up to 1 satoshi
    expect(toAssetAmount(1, 'btc')).toBe(0.00000001);
    expect(toAssetAmount(10, 'btc')).toBe(0.00000008);
    // R0.01 is half a USDC cent, which rounds down to zero at two decimals
    expect(toAssetAmount(1, 'usdc-base')).toBe(0);
    expect(toAssetAmount(1, 'usdc-ethereum')).toBe(0.000541);
  });

  it('treats a zero or negative amount as zero', () => {
    expect(toAssetAmount(0, 'btc')).toBe(0);
    expect(Object.is(toAssetAmount(0, 'btc'), -0)).toBe(false);
    expect(toAssetAmount(-500, 'btc')).toBe(0);
    expect(toAssetAmount(-500, 'zarp-base')).toBe(0);
  });
});

describe('formatAssetAmount', () => {
  it('prints the symbol with the option precision', () => {
    expect(formatAssetAmount(20, 'usdc-base')).toBe('20.00 USDC');
    expect(formatAssetAmount(0.5, 'btc')).toBe('0.50000000 BTC');
    expect(formatAssetAmount(123.456, 'zarp-ethereum')).toBe('123.456000 ZARP');
  });
});

describe('legacy payment methods', () => {
  it('resolves stored symbols to an option id', () => {
    expect(resolveOptionId('BTC')).toBe('btc');
    expect(resolveOptionId('USDC')).toBe('usdc-base');
    expect(resolveOptionId('eth')).toBe('usdc-ethereum');
    expect(resolveOptionId('ZARP')).toBe('zarp-base');
    expect(resolveOptionId('EURC')).toBe('eurc-base');
  });

  it('passes option ids through and defaults anything else', () => {
    expect(resolveOptionId('zarp-ethereum')).toBe('zarp-ethereum');
    expect(resolveOptionId('DOGE')).toBe(DEFAULT_PAYMENT_OPTION_ID);
    expect(resolveOptionId('')).toBe(DEFAULT_PAYMENT_OPTION_ID);
  });

  it('keeps toCrypto/formatCrypto working for old receipts', () => {
    expect(toCrypto(37_000, 'USDC')).toBe(20);
    expect(formatCrypto(1, 'BTC')).toBe('1.00000000 BTC');
  });
});
