import { test, expect } from '@playwright/test';
import { datasets } from '../../internal/mcpapp/ui/data.js';

const rows = [
  { day: '2026-06-01', temp_avg_f: 70, rain_in: null, obs: 2, rain_readings: 0 },
  { day: '2026-06-02', temp_avg_f: 71, rain_in: 0, obs: 2, rain_readings: 2 },
  { day: '2026-06-03', temp_avg_f: 72, rain_in: 1.25, obs: 2, rain_readings: 1 },
];

test('history coverage reuses original counts and never changes the received rows', () => {
  for (const [tool, key] of [['daily_summary', 'days'], ['period_summary', 'periods'], ['this_day_in_history', 'years']]) {
    const data = { [key]: rows.map(row => Object.freeze({ ...row, rainy_days: 0 })) };
    const before = JSON.stringify(data);
    const [table] = datasets(data, tool);
    expect(table.coverage).toEqual([{ name: 'Rain', missing: 1, partial: 1 }]);
    expect(table.rows.map(row => row.rain_in)).toEqual([null, 0, 1.25]);
    expect(table.rows[0].rainy_days).toBeNull();
    expect(table.rows[1].rainy_days).toBe(0);
    expect(JSON.stringify(data)).toBe(before);
  }
  const [unrelated] = datasets({ periods: rows }, 'query_sql');
  expect(unrelated.coverage).toBeUndefined();
});

test('daily rainfall charts and totals distinguish missing zero and partial samples', async ({ page }, testInfo) => {
  await page.goto('/?view=daily');
  await expect.poll(() => page.evaluate(() => window.fixtureHost?.initialized)).toBe(true);
  const app = page.frameLocator('iframe');
  await expect(app.locator('#content')).toHaveAttribute('aria-busy', 'false');
  await page.evaluate(days => window.fixtureHost.send({ days }), rows);
  await expect(app.locator('#sensor-coverage')).toContainText('1 row without readings');
  await expect(app.locator('#sensor-coverage')).toContainText('1 row with partial readings');
  await expect(app.locator('#sensor-coverage')).toContainText('not continuous uptime');
  const rain = app.locator('.metric').filter({ has: app.locator('.metric-label', { hasText: /^Rainfall$/ }) });
  await expect(rain.locator('.metric-value')).toHaveText('1.25in');
  await app.locator('#metric').selectOption('rain_in');
  await expect(app.locator('.chart-inspector')).toContainText('Missing');
  await expect(app.locator('tbody tr').first()).toContainText('—');
  await page.setViewportSize({ width: 390, height: 844 });
  await page.locator('iframe').screenshot({ path: testInfo.outputPath('history-rain-narrow.png'), animations: 'disabled' });
  await page.evaluate(days => { window.fixtureHost.fixture.data = { days }; }, rows.map(row => ({ ...row, rain_in: 0, rain_readings: 2 })));
  await app.locator('#refresh').click();
  await expect(app.locator('#sensor-coverage')).toHaveCount(0);
  await expect(rain.locator('.metric-value')).toHaveText('0in');
});

test('climate rain normals expose their contributing years without a completeness claim', async ({ page }) => {
  await page.goto('/?view=normals');
  await expect.poll(() => page.evaluate(() => window.fixtureHost?.initialized)).toBe(true);
  const app = page.frameLocator('iframe');
  await expect(app.locator('#normal-rain-coverage')).toContainText('rain_years');
  await expect(app.locator('#normal-rain-coverage')).toContainText('Partial months can contribute');
  await expect(app.locator('tbody tr').first()).toContainText('—');
  expect(await page.evaluate(() => window.fixtureHost.calls)).toEqual([]);
});
