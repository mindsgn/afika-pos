import { resolve } from 'node:path';
import { expect, test } from '@playwright/test';


test('merchant creates a checkout and customer can choose an asset and see paid status', async ({ page, request }) => {
  const response = await request.post('/v1/pos/payments', {
    headers: { Authorization: 'Bearer playwright-test-token' },
    data: {
      receiptNo: 'POS-E2E-001',
      amountCents: 12345,
      currency: 'ZAR',
      asset: 'usdc-base',
      receiveAddress: '0x1111111111111111111111111111111111111111',
      merchant: { name: 'Kofi Coffee', website: 'https://kofi.example', address: '12 Long St, Cape Town' },
    },
  });
  expect(response.status()).toBe(201);
  const { data: payment } = await response.json();

  await page.goto(payment.paymentUrl);
  // Load the same HTMX version locally so the browser test does not depend on the CDN.
  await page.addScriptTag({ path: resolve('node_modules/htmx.org/dist/htmx.min.js') });
  await expect(page.getByText('R123.45')).toBeVisible();
  await expect(page.getByText('Kofi Coffee')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Choose how to pay' })).toBeVisible();
  await page.getByRole('button', { name: /EURC.*Ethereum/ }).click();
  await expect(page.getByText('6.141791 EURC')).toBeVisible();
  await page.addScriptTag({ path: resolve('node_modules/htmx.org/dist/htmx.min.js') });
  await expect(page.getByText('ethereum', { exact: true })).toBeVisible();

  await expect(page.locator('.payment-poll')).toHaveAttribute('hx-get', `/pay/${payment.id}/status`);

  const confirm = await request.post(`/v1/pos/payments/${payment.id}/confirm`, {
    headers: { Authorization: 'Bearer playwright-test-token' },
    data: { txHash: `0x${'a'.repeat(64)}` },
  });
  expect(confirm.status()).toBe(200);
  await expect(page.getByText('Payment received. Thank you!')).toBeVisible({ timeout: 12_000 });
  await expect(page.getByText('POS-E2E-001')).toBeVisible();
});
