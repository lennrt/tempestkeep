import { test, expect } from '@playwright/test';
import { rainSummary } from './fixtures.mjs';
import { rainSummaryDisplay } from '../../internal/mcpapp/ui/data.js';

async function openRain(page, view = 'rain') {
  await page.goto(`/?view=${view}`);
  await expect.poll(() => page.evaluate(() => window.fixtureHost?.initialized)).toBe(true);
  const app = page.frameLocator('iframe');
  await expect(app.locator('#content')).toHaveAttribute('aria-busy', 'false');
  return app;
}

test('missing rain keeps a rainfall headline and leaves rain statistics unavailable', async ({ page }, testInfo) => {
  const app = await openRain(page);
  await expect(app.locator('.hero-label').first()).toHaveText('Rainfall');
  await expect(app.locator('.hero-value').first()).toHaveText('—');
  await expect(app.locator('.hero-value').nth(1)).toHaveText('3');
  await expect(app.locator('.hero-value').nth(2)).toHaveText('—');
  await expect(app.locator('#rain-coverage')).toContainText('0 of 12 stored observations');
  await expect(app.locator('#rain-coverage')).toContainText('No rain readings');
  await expect(app.locator('#rain-coverage')).toContainText('not continuous coverage');
  await expect(app.locator('#result-details dt', { hasText: /^longest dry spell days$/ }).locator('+ dd')).toHaveText('—');
  await expect(app.locator('#result-details dt', { hasText: /^longest wet spell days$/ }).locator('+ dd')).toHaveText('—');
  await expect(app.locator('#result-details dt', { hasText: /^observations$/ }).locator('+ dd')).toHaveText('12');
  await expect(app.locator('#result-details dt', { hasText: /^rain readings$/ }).locator('+ dd')).toHaveText('0');
  await expect(app.locator('#data-panel')).toHaveCount(0);
  await expect(app.locator('#chart-panel')).toHaveCount(0);
  expect(await page.evaluate(() => window.fixtureHost.calls)).toEqual([]);
  await page.locator('iframe').screenshot({ path: testInfo.outputPath('rain-missing-desktop.png'), animations: 'disabled' });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await app.locator('html').evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.locator('iframe').screenshot({ path: testInfo.outputPath('rain-missing-narrow.png'), animations: 'disabled' });
});

test('an empty rain range retains zero coverage counts without implying dry weather', async ({ page }) => {
  const app = await openRain(page);
  await page.evaluate(data => window.fixtureHost.send(data), { ...rainSummary, observations: 0, days_observed: 0 });
  await expect(app.locator('.hero-value')).toHaveText(['—', '0', '—']);
  await expect(app.locator('#rain-coverage')).toContainText('No observations in this range');
  await expect(app.locator('#rain-coverage')).toContainText('0 of 0 stored observations');
  await expect(app.locator('#result-details dt', { hasText: /^rain readings$/ }).locator('+ dd')).toHaveText('0');
  await expect(app.locator('#result-details dt', { hasText: /^longest dry spell days$/ }).locator('+ dd')).toHaveText('—');
});

test('measured zero rain and its observed dry spell remain numeric', async ({ page }) => {
  const app = await openRain(page);
  await page.evaluate(data => window.fixtureHost.send(data), { ...rainSummary, total_in: 0, rain_readings: 12, longest_dry_spell_days: 3 });
  await expect(app.locator('.hero-value')).toHaveText(['0in', '3', '0']);
  await expect(app.locator('#rain-coverage')).toContainText('12 of 12 stored observations');
  await expect(app.locator('#rain-coverage')).not.toContainText(/partial|unavailable/i);
  await expect(app.locator('#result-details dt', { hasText: /^longest dry spell days$/ }).locator('+ dd')).toHaveText('3');
});

test('partial positive and zero totals retain their values with an explicit sample warning', async ({ page }, testInfo) => {
  const app = await openRain(page);
  for (const total of [1.25, 0]) {
    await page.evaluate(data => window.fixtureHost.send(data), { ...rainSummary, total_in: total, rain_readings: 4, rainy_days: total > 0 ? 1 : 0 });
    await expect(app.locator('.hero-value').first()).toHaveText(`${total}in`);
    await expect(app.locator('#rain-coverage')).toContainText('Partial rainfall total: includes reported readings only.');
    await expect(app.locator('#rain-coverage')).toContainText('4 of 12 stored observations');
    await expect(app.locator('#rain-coverage')).toContainText('not continuous coverage');
  }
  await page.locator('iframe').screenshot({ path: testInfo.outputPath('rain-partial-desktop.png'), animations: 'disabled' });
});

test('rain display copies do not change raw results or reinterpret absent and invalid counts', () => {
  const source = Object.freeze({ ...rainSummary, total_in: 0, wettest_day_in: 0 });
  const display = rainSummaryDisplay(source, 'rain_stats');
  expect(display).not.toBe(source);
  expect(display).toMatchObject({ total_in: null, rainy_days: null, longest_dry_spell_days: null, longest_wet_spell_days: null, wettest_day_in: null, observations: 12, rain_readings: 0, days_observed: 3 });
  expect(source).toEqual({ ...rainSummary, total_in: 0, wettest_day_in: 0 });
  for (const tool of ['query_sql', 'daily_summary', 'period_summary', 'current_conditions']) expect(rainSummaryDisplay(source, tool)).toBe(source);
  for (const count of [undefined, null, '0', -1, 0.5]) {
    const legacy = { ...source, rain_readings: count };
    expect(rainSummaryDisplay(legacy, 'rain_stats')).toBe(legacy);
  }
});

test('legacy count-less rain totals and statistics keep their numeric meaning', async ({ page }) => {
  const app = await openRain(page);
  const { observations, rain_readings, ...legacy } = rainSummary;
  await page.evaluate(data => window.fixtureHost.send(data), { ...legacy, total_in: 0 });
  await expect(app.locator('.hero-value')).toHaveText(['0in', '3', '0']);
  await expect(app.locator('#rain-coverage')).toHaveCount(0);
  await expect(app.locator('#result-details dt', { hasText: /^longest dry spell days$/ }).locator('+ dd')).toHaveText('0');
});

test('SQL columns with rain summary names remain literal values', async ({ page }) => {
  const app = await openRain(page, 'table');
  await page.evaluate(() => window.fixtureHost.send({ columns: ['total_in', 'rain_readings', 'observations', 'rainy_days'], rows: [[0, 0, 12, 0]], row_count: 1 }));
  await expect(app.locator('tbody tr td')).toHaveText(['0', '0', '12', '0']);
  await expect(app.locator('#rain-coverage')).toHaveCount(0);
});

test('rain refresh updates values and replaces old coverage notices', async ({ page }) => {
  const app = await openRain(page);
  for (const [readings, total] of [[4, 1.25], [12, 0], [0, null]]) {
    await page.evaluate(data => { window.fixtureHost.fixture.data = data; }, { ...rainSummary, total_in: total, rain_readings: readings });
    await app.locator('#refresh').click();
    await expect(app.locator('#refresh')).toBeEnabled();
    await expect(app.locator('.hero-label').first()).toHaveText('Rainfall');
    await expect(app.locator('.hero-value').first()).toHaveText(total === null ? '—' : `${total}in`);
    await expect(app.locator('#rain-coverage')).toHaveCount(1);
    await expect(app.locator('#rain-coverage')).toContainText(`${readings} of 12 stored observations`);
    if (readings === 4) await expect(app.locator('#rain-coverage')).toContainText('Partial rainfall total');
    else await expect(app.locator('#rain-coverage')).not.toContainText('Partial rainfall total');
  }
  expect(await page.evaluate(() => window.fixtureHost.calls.map(({ name, arguments: args }) => ({ name, arguments: args })))).toEqual(Array.from({ length: 3 }, () => ({ name: 'rain_stats', arguments: { start: '2026-06-01', end: '2026-06-03' } })));
});
