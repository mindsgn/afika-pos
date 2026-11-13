import { expect, test } from '@playwright/test';

test('merchant signs in with an email code and is sent to business onboarding', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('link', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/signin$/);
  await expect(page.getByRole('heading', { name: 'Sign in to Afika' })).toBeVisible();

  await page.getByLabel('Email').fill('merchant@example.com');
  await page.getByRole('button', { name: 'Send code' }).click();

  await expect(page.getByRole('heading', { name: 'Check your email' })).toBeVisible();
  await page.getByLabel('Authentication code').fill('000000');
  await page.getByRole('button', { name: 'Verify and continue' }).click();

  await expect(page).toHaveURL(/\/dashboard\/onboarding$/);
  await expect(page.getByRole('heading', { name: 'Set up your business' })).toBeVisible();

  await page.getByRole('button', { name: 'Sign out' }).click();
  await expect(page).toHaveURL(/\/$/);
});

test('wrong code is rejected and no session is created', async ({ page, context }) => {
  await page.goto('/signin');
  await page.getByLabel('Email').fill('merchant@example.com');
  await page.getByRole('button', { name: 'Send code' }).click();

  await page.getByLabel('Authentication code').fill('111111');
  await page.getByRole('button', { name: 'Verify and continue' }).click();

  await expect(page.getByText('That code was not accepted')).toBeVisible();
  await expect(page).toHaveURL(/\/signin\/verify$/);
  const cookies = await context.cookies();
  expect(cookies.some((cookie) => cookie.name === 'afika_session' && cookie.value !== '')).toBe(false);
});

test('account page requires a session', async ({ page }) => {
  await page.goto('/account');
  await expect(page).toHaveURL(/\/signin$/);
  await expect(page.getByRole('heading', { name: 'Sign in to Afika' })).toBeVisible();
});

test('dashboard requires a session', async ({ page }) => {
  await page.goto('/dashboard');
  await expect(page).toHaveURL(/\/signin$/);
});
