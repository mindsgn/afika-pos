import { expect, test } from '@playwright/test';

async function signIn(page: import('@playwright/test').Page) {
  await page.goto('/signin');
  await page.getByLabel('Email').fill('dashboard-merchant@example.com');
  await page.getByRole('button', { name: 'Send code' }).click();
  await page.getByLabel('Authentication code').fill('000000');
  await page.getByRole('button', { name: 'Verify and continue' }).click();
  await expect(page).toHaveURL(/\/dashboard\/onboarding$/);
}

test('merchant onboards, manages catalogue and stock, checks out, refunds, voids and views reports', async ({ page }) => {
  await signIn(page);

  await page.getByLabel('Business name').fill('Dashboard Coffee');
  await page.getByLabel('Website').fill('dashboard.example');
  await page.getByLabel('Business address').fill('12 Long Street, Cape Town');
  await page.getByRole('button', { name: 'Continue to dashboard' }).click();
  await expect(page.getByRole('heading', { name: 'Dashboard Coffee' })).toBeVisible();

  await page.getByRole('link', { name: 'Categories' }).click();
  await page.getByLabel('Category name').fill('Coffee');
  await page.getByRole('button', { name: 'Add category' }).click();
  await expect(page.getByText('Coffee', { exact: true })).toBeVisible();

  await page.getByRole('link', { name: 'Products' }).click();
  await page.getByLabel('Product name').fill('Flat White');
  await page.getByLabel('SKU').fill('FW-001');
  await page.getByLabel('Category').selectOption({ label: 'Coffee' });
  await page.getByLabel('Price (cents)', { exact: true }).fill('3500');
  await page.getByLabel('Cost price (cents)', { exact: true }).fill('1200');
  await page.getByLabel('Opening stock').fill('10');
  await page.getByLabel('Reorder level').fill('2');
  await page.getByRole('button', { name: 'Add product' }).click();
  await expect(page.getByRole('cell', { name: 'Flat White' })).toBeVisible();
  await expect(page.getByRole('cell', { name: '10', exact: true })).toBeVisible();

  await page.getByRole('link', { name: 'Inventory' }).click();
  await page.getByLabel('Type').selectOption('restock');
  await page.getByLabel('Quantity change').fill('5');
  await page.getByLabel('Unit cost (cents, optional)').fill('1250');
  await page.getByLabel('Note').fill('E2E restock');
  await page.getByRole('button', { name: 'Save movement' }).click();
  await expect(page.getByText('E2E restock')).toBeVisible();

  await page.getByRole('link', { name: 'Loyalty' }).click();
  await page.getByLabel('Customer name').fill('Nandi');
  await page.getByLabel('Phone').fill('0820000000');
  await page.getByRole('button', { name: 'Add customer' }).click();
  await expect(page.getByRole('cell', { name: 'Nandi' })).toBeVisible();

  await page.getByRole('link', { name: 'Checkout' }).click();
  await page.getByRole('combobox', { name: 'Product 1' }).selectOption({ index: 1 });
  await page.getByLabel('Loyalty customer').selectOption({ index: 1 });
  await page.getByRole('button', { name: 'Complete sale' }).click();
  await expect(page.getByRole('heading', { name: /Sale R-/ })).toBeVisible();
  await expect(page.getByText('Status: completed')).toBeVisible();
  await expect(page.getByRole('link', { name: 'View receipt' })).toBeVisible();
  const firstSaleURL = page.url();
  await page.getByRole('link', { name: 'View receipt' }).click();
  await expect(page.getByRole('heading', { name: 'Receipt' })).toBeVisible();
  await page.goto(firstSaleURL);

  await page.getByRole('link', { name: 'Loyalty' }).click();
  await expect(page.getByRole('cell', { name: '3', exact: true })).toBeVisible();
  await page.goto(firstSaleURL);

  await page.getByLabel(/Quantity to refund/).fill('1');
  await page.getByLabel('Reason', { exact: true }).fill('Customer changed their mind');
  await page.getByRole('button', { name: 'Record refund' }).click();
  await expect(page.getByText('Status: refunded')).toBeVisible();
  await expect(page.getByRole('link', { name: 'refund receipt' })).toBeVisible();
  await page.getByRole('link', { name: 'refund receipt' }).click();
  await expect(page.getByRole('heading', { name: 'Receipt' })).toBeVisible();
  await page.goto(firstSaleURL);
  await page.getByRole('link', { name: 'Loyalty' }).click();
  await expect(page.getByRole('cell', { name: '0', exact: true })).toBeVisible();

  await page.getByRole('link', { name: 'Checkout' }).click();
  await page.getByRole('combobox', { name: 'Product 1' }).selectOption({ index: 1 });
  await page.getByRole('button', { name: 'Complete sale' }).click();
  await expect(page.getByRole('heading', { name: /Sale R-/ })).toBeVisible();
  await page.getByLabel('Void reason').fill('Duplicate sale');
  await page.getByRole('button', { name: 'Void sale' }).click();
  await expect(page.getByText('Status: voided')).toBeVisible();

  await page.getByRole('link', { name: 'Insights' }).click();
  await expect(page.getByRole('heading', { name: 'Insights' })).toBeVisible();
  await expect(page.getByText('Net profit today')).toBeVisible();
});
