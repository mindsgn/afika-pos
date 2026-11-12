import { apiBaseUrl, checkoutBaseUrl, paymentUrlFor } from './payments';

const ORIGINAL_ENV = { ...process.env };

beforeEach(() => {
  delete process.env.EXPO_PUBLIC_API_URL;
  delete process.env.EXPO_PUBLIC_CHECKOUT_URL;
});

afterAll(() => {
  process.env = ORIGINAL_ENV;
});

describe('apiBaseUrl', () => {
  it('falls back to the deployed API', () => {
    expect(apiBaseUrl()).toBe('https://afika.phumzile.studio');
  });

  it('honours the configured URL and strips trailing slashes', () => {
    process.env.EXPO_PUBLIC_API_URL = 'http://localhost:8787///';
    expect(apiBaseUrl()).toBe('http://localhost:8787');
  });

  it('ignores a blank override', () => {
    process.env.EXPO_PUBLIC_API_URL = '   ';
    expect(apiBaseUrl()).toBe('https://afika.phumzile.studio');
  });
});

describe('checkoutBaseUrl', () => {
  it('defaults to the API host', () => {
    process.env.EXPO_PUBLIC_API_URL = 'http://localhost:8787';
    expect(checkoutBaseUrl()).toBe('http://localhost:8787');
  });

  it('can point at a separate web checkout host', () => {
    process.env.EXPO_PUBLIC_CHECKOUT_URL = 'https://pay.afika.app/';
    expect(checkoutBaseUrl()).toBe('https://pay.afika.app');
  });
});

describe('paymentUrlFor', () => {
  it('builds the shareable /pay/{id} link', () => {
    process.env.EXPO_PUBLIC_API_URL = 'https://afika.phumzile.studio';
    expect(paymentUrlFor('abc123')).toBe('https://afika.phumzile.studio/pay/abc123');
  });

  it('escapes ids that are not URL safe', () => {
    process.env.EXPO_PUBLIC_CHECKOUT_URL = 'https://pay.afika.app';
    expect(paymentUrlFor('a/b?c')).toBe('https://pay.afika.app/pay/a%2Fb%3Fc');
  });
});
