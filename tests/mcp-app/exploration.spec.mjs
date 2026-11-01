import { test, expect } from '@playwright/test';

async function open(page, view) {
  await page.goto(`/?view=${view}`);
  await expect.poll(() => page.evaluate(() => window.fixtureHost?.initialized)).toBe(true);
  const app = page.frameLocator('iframe');
  await expect(app.locator('#data-table')).toBeVisible();
  return app;
}

test('dataset selection preserves keyboard focus on the replacement control', async ({ page }) => {
  const app = await open(page, 'forecast');
  const daily = app.getByRole('button', { name: 'daily', exact: true });
  await daily.focus();
  await daily.press('Enter');
  await expect(daily).toBeFocused();
  await expect(daily).toHaveAttribute('aria-pressed', 'true');
  await daily.press('Tab');
  await expect(app.getByRole('button', { name: 'hourly', exact: true })).toBeFocused();
  await app.getByRole('button', { name: 'hourly', exact: true }).press('Space');
  await expect(app.getByRole('button', { name: 'hourly', exact: true })).toBeFocused();
  expect(await page.evaluate(() => window.fixtureHost.calls)).toEqual([]);
});

test('daily forecasts summarize high low chance and days without inventing missing values', async ({ page }) => {
  const app = await open(page, 'forecast');
  await page.evaluate(async () => {
    const data = structuredClone(window.fixtureHost.fixture.data);
    data.daily = [{ date: '2026-06-01', high_f: 82, low_f: 52, precip_probability: 0 }, { date: '2026-06-02', high_f: null, low_f: 48, precip_probability: 65 }];
    await window.fixtureHost.send(data);
  });
  await app.getByRole('button', { name: 'daily', exact: true }).click();
  await expect(app.locator('.metrics')).toContainText('Highest forecast');
  await expect(app.locator('.metric-value')).toHaveText(['82°F', '48°F', '65%', '2']);
  await expect(app.locator('.metrics')).not.toContainText('Hourly');
  await page.locator('iframe').screenshot({ path: test.info().outputPath('daily-forecast-summary.png'), animations: 'disabled' });
  await page.evaluate(async () => {
    const data = structuredClone(window.fixtureHost.fixture.data);
    data.daily = [{ date: '2026-06-01', high_f: null, low_f: null, precip_probability: null }];
    await window.fixtureHost.send(data);
  });
  await expect(app.locator('.metric-value')).toHaveText(['—', '—', '—', '1']);
  await app.getByRole('button', { name: 'hourly', exact: true }).click();
  await expect(app.locator('.metrics')).toContainText('Hourly outlook');
  expect(await page.evaluate(() => window.fixtureHost.calls)).toEqual([]);
});

test('SQL long cells can be searched and inspected with keyboard and hostile content stays text', async ({ page }) => {
  const app = await open(page, 'table');
  const value = 'x'.repeat(2050) + '<img src=x onerror="window.injected=true">NEEDLE';
  await page.evaluate(data => window.fixtureHost.send(data), { columns: ['value'], rows: [[value], [null], [0]], row_count: 3 });
  await app.locator('#search').fill('NEEDLE');
  await expect(app.locator('tbody tr')).toHaveCount(1);
  const inspect = app.getByRole('button', { name: /Inspect full value/ });
  await inspect.focus();
  await inspect.press('Enter');
  const dialog = app.getByRole('dialog', { name: 'Cell value' });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator('pre')).toHaveText(value);
  await expect(app.locator('img')).toHaveCount(0);
  await dialog.press('Escape');
  await expect(dialog).not.toBeVisible();
  await expect(inspect).toBeFocused();
  expect(await page.evaluate(() => window.fixtureHost.calls)).toEqual([]);
});

test('long cell inspection and search stop at their disclosed limit', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const app = await open(page, 'table');
  await page.evaluate(() => window.fixtureHost.send({ columns: ['value'], rows: [['a'.repeat(65536) + 'BEYOND_LIMIT']], row_count: 1 }));
  await expect(app.locator('#data-panel')).toContainText('65,536');
  await app.getByRole('button', { name: /Inspect full value/ }).click();
  const dialog = app.getByRole('dialog', { name: 'Cell value' });
  await expect(dialog.locator('pre')).toHaveText('a'.repeat(65536));
  await expect(dialog).toContainText('65,536');
  expect(await app.locator('html').evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.locator('iframe').screenshot({ path: test.info().outputPath('cell-inspector-narrow.png'), animations: 'disabled' });
  await dialog.getByRole('button', { name: 'Close' }).click();
  await app.locator('#search').fill('BEYOND_LIMIT');
  await expect(app.locator('tbody')).toContainText('No rows match');
});

test('new host results dismiss an inspector without leaving stale text', async ({ page }) => {
  const app = await open(page, 'table');
  await page.evaluate(() => window.fixtureHost.send({ columns: ['value'], rows: [['a'.repeat(3000)]], row_count: 1 }));
  await app.getByRole('button', { name: /Inspect full value/ }).click();
  await expect(app.getByRole('dialog')).toBeVisible();
  await page.evaluate(() => window.fixtureHost.send({ columns: ['value'], rows: [['new result']], row_count: 1 }));
  await expect(app.getByRole('dialog')).toHaveCount(0);
  await expect(app.locator('tbody')).toHaveText('new result');
});
