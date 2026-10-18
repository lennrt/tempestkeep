import { test, expect } from '@playwright/test';
import { mkdir } from 'node:fs/promises';
import { observations, query } from './fixtures.mjs';
import { datasets } from '../../internal/mcpapp/ui/data.js';

async function openApp(page, view = 'weather', extra = '') {
  await page.goto(`/?view=${view}${extra}`);
  await expect.poll(() => page.evaluate(() => window.fixtureHost?.initialized)).toBe(true);
  const app = page.frameLocator('iframe');
  if (!extra.includes('loading')) await expect(app.locator('#content')).toHaveAttribute('aria-busy', 'false');
  return app;
}

test('official host bridge renders weather, changes metrics, and refreshes the original arguments', async ({ page }) => {
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const requests = [];
  page.on('request', (request) => requests.push(request.url()));
  const app = await openApp(page);
  await expect(app.locator('h1')).toContainText(/observations|weather|archive/i);
  await expect(app.locator('svg').first()).toBeVisible();
  await expect(app.locator('table tbody tr').first()).toBeVisible();
  const select = app.locator('.metric-select').first();
  const before = await app.locator('.chart').first().textContent();
  await select.selectOption({ index: 1 });
  await expect(app.locator('.chart').first()).not.toHaveText(before);
  await app.getByRole('button', { name: 'Refresh', exact: false }).click();
  await expect.poll(() => page.evaluate(() => window.fixtureHost.calls.length)).toBe(1);
  expect(await page.evaluate(() => window.fixtureHost.calls[0])).toMatchObject({
    name: 'get_observations', arguments: { start: '2026-06-14', end: '2026-06-14', bucket_minutes: 30 },
  });
  expect(requests.every((url) => url.startsWith('http://127.0.0.1:4177/'))).toBe(true);
  expect(errors).toEqual([]);
});

test('SQL table filters, sorts numerically, paginates, and shows truncation', async ({ page }) => {
  const app = await openApp(page, 'table');
  await expect(app.locator('table tbody tr')).toHaveCount(8);
  await expect(app.locator('#content')).toContainText(/more rows|truncat|limit/i);
  const next = app.getByRole('button', { name: /next page/i });
  await next.click();
  await expect(app.locator('table tbody tr').first()).toContainText('2026-04-09');
  await app.locator('input[type="search"]').fill('Partial');
  await expect(app.locator('table tbody tr')).toHaveCount(1);
  await expect(app.locator('table tbody tr').first()).toContainText('2026-04-06');
  await app.locator('input[type="search"]').fill('');
  await app.locator('thead th').nth(1).getByRole('button').click();
  await expect(app.locator('thead th').nth(1)).toHaveAttribute('aria-sort', 'ascending');
  const values = await app.locator('tbody tr td:nth-child(2)').allTextContents();
  const nums = values.map((value) => Number(value.replaceAll(',', ''))).filter(Number.isFinite);
  expect(nums).toEqual([...nums].sort((a, b) => a - b));
  await app.locator('thead th').nth(1).getByRole('button').click();
  await expect(app.locator('thead th').nth(1)).toHaveAttribute('aria-sort', 'descending');
  await app.locator('input[type="search"]').fill('does not exist');
  await expect(app.locator('#content')).toContainText(/no .*match|no results/i);
});

test('missing values remain distinct from zero and hostile cells remain text', async ({ page }) => {
  const app = await openApp(page, 'table');
  await page.evaluate(async () => window.fixtureHost.send({
    columns: ['label', 'reading'], rows: [['<img src=x onerror="window.injected=true">', null], ['measured zero', 0]], row_count: 2,
  }));
  await expect(app.locator('tbody tr')).toHaveCount(2);
  await expect(app.locator('tbody tr').first()).toContainText('<img src=x');
  await expect(app.locator('tbody tr').first().locator('td').nth(1)).toHaveText('—');
  await expect(app.locator('tbody tr').nth(1).locator('td').nth(1)).toHaveText('0');
  await expect(app.locator('img')).toHaveCount(0);
  expect(await page.frames()[1].evaluate(() => window.injected)).toBeUndefined();
});

test('query column order and duplicate labels retain every value', async ({ page }) => {
  const app = await openApp(page, 'table');
  await page.evaluate(() => window.fixtureHost.send({ columns: ['value', 'value', '__proto__'], rows: [[12, 34, 'safe']], row_count: 1 }));
  await expect(app.locator('tbody tr td')).toHaveText(['12', '34', 'safe']);
});

test('a missing temperature produces a gap instead of a zero reading', async ({ page }) => {
  const app = await openApp(page);
  const sparse = structuredClone(observations);
  sparse.points = [sparse.points[0], { ...sparse.points[1], temp_avg_f: null }, sparse.points[2]];
  await page.evaluate((data) => window.fixtureHost.send(data), sparse);
  await expect(app.locator('tbody tr')).toHaveCount(3);
  await expect(app.locator('tbody tr').nth(1)).toContainText('—');
  await expect(app.locator('.chart-line')).toHaveCount(2);
});

test('absent archive buckets break the plotted line', async ({ page }) => {
  const app = await openApp(page);
  const sparse = { ...observations, points: [observations.points[0], observations.points[1], observations.points[5], observations.points[6]] };
  await page.evaluate((data) => window.fixtureHost.send(data), sparse);
  await expect(app.locator('.chart-line')).toHaveCount(2);
});

test('dense charts expose all 2000 readings through pointer and keyboard selection', async ({ page }) => {
  const app = await openApp(page);
  const points = Array.from({ length: 2000 }, (_, i) => ({
    ...observations.points[0], epoch: observations.points[0].epoch + i * 60,
    time: new Date((observations.points[0].epoch + i * 60) * 1000).toISOString(), temp_avg_f: i / 10,
  }));
  await page.evaluate(data => window.fixtureHost.send(data), { ...observations, bucket_minutes: 1, points });
  const slider = app.getByRole('slider', { name: /Inspect a reading/ });
  await expect(slider).toHaveAttribute('max', '1999');
  await expect(app.locator('.reading-position')).toHaveText('1 of 2,000');
  const plot = await app.locator('.chart-hit').boundingBox();
  await page.mouse.move(plot.x + plot.width * 999 / 1999, plot.y + plot.height / 2);
  await expect(slider).toHaveValue('999');
  await expect(app.locator('.reading-value')).toHaveText('Average temperature (°F): 99.9');
  await expect(app.locator('.reading-time')).toHaveText(points[999].time.replace('T', ' · '));
  await slider.focus();
  await slider.press('End');
  await expect(slider).toHaveValue('1999');
  await expect(app.locator('.reading-time')).toHaveText(points[1999].time.replace('T', ' · '));
  await slider.press('ArrowLeft');
  await expect(slider).toHaveValue('1998');
  await slider.press('Home');
  await expect(slider).toHaveValue('0');
  expect(await page.evaluate(() => window.fixtureHost.calls)).toEqual([]);
});

test('pointer selection uses plotted time gaps instead of evenly spaced indices', async ({ page }) => {
  const app = await openApp(page);
  const points = [observations.points[0], observations.points[1], observations.points[10]];
  await page.evaluate(data => window.fixtureHost.send(data), { ...observations, points });
  await expect(app.locator('#chart-reading')).toHaveAttribute('max', '2');
  const plot = await app.locator('.chart-hit').boundingBox();
  // 70% of the time axis is nearest the final point. An index-based mapping
  // would select the middle row and hide the outage's actual spacing.
  await page.mouse.click(plot.x + plot.width * .7, plot.y + plot.height / 2);
  await expect(app.locator('#chart-reading')).toHaveValue('2');
  await expect(app.locator('.reading-time')).toHaveText(points[2].time.replace('T', ' · '));
  await expect(app.locator('.chart-line')).toHaveCount(2);
});

test('keyboard inspection preserves missing readings, measured zero, and source offsets', async ({ page }) => {
  const app = await openApp(page);
  const points = [
    { ...observations.points[0], epoch: 1793520000, time: '2026-11-01T01:00:00-07:00', temp_avg_f: null, rain_in: 0, rain_readings: 0 },
    { ...observations.points[1], epoch: 1793523600, time: '2026-11-01T01:00:00-08:00', temp_avg_f: 0, rain_in: 0, rain_readings: 30 },
  ];
  await page.evaluate(data => window.fixtureHost.send(data), { ...observations, points });
  const slider = app.locator('#chart-reading');
  await expect(app.locator('.reading-value')).toHaveText('Average temperature (°F): Missing reading');
  await expect(app.locator('.reading-time')).toHaveText('2026-11-01 · 01:00:00-07:00');
  await expect(app.locator('.chart-selection')).toHaveAttribute('visibility', 'hidden');
  await slider.focus();
  await slider.press('ArrowRight');
  await expect(app.locator('.reading-value')).toHaveText('Average temperature (°F): 0');
  await expect(app.locator('.reading-time')).toHaveText('2026-11-01 · 01:00:00-08:00');
  await expect(slider).toHaveAttribute('aria-valuetext', /Average temperature \(°F\): 0/);
  await expect(app.locator('.chart-selection')).toHaveAttribute('visibility', 'visible');
  await app.locator('#metric').selectOption('rain_in');
  await expect(app.locator('.reading-value')).toHaveText('Rainfall (in): Missing reading');
  await app.locator('#chart-reading').press('End');
  await expect(app.locator('.reading-value')).toHaveText('Rainfall (in): 0');
});

test('metric and result changes reset the inspector without stale readings', async ({ page }) => {
  const app = await openApp(page);
  await app.locator('#chart-reading').press('End');
  await expect(app.locator('#chart-reading')).toHaveValue('47');
  await app.locator('#metric').selectOption('rain_in');
  await expect(app.locator('#chart-reading')).toHaveValue('0');
  await expect(app.locator('.reading-value')).toHaveText('Rainfall (in): 0');
  await app.locator('#chart-reading').press('End');
  const point = { ...observations.points[0], time: '2026-07-01T12:00:00+02:00', rain_in: .7 };
  await page.evaluate(data => window.fixtureHost.send(data), { ...observations, points: [point] });
  await expect(app.locator('#chart-reading')).toHaveValue('0');
  await expect(app.locator('.reading-position')).toHaveText('1 of 1');
  await expect(app.locator('.reading-time')).toHaveText('2026-07-01 · 12:00:00+02:00');
  await expect(app.locator('.reading-value')).toHaveText('Rainfall (in): 0.7');
});

test('touch inspection stays readable within a narrow viewport', async ({ browser }, testInfo) => {
  const context = await browser.newContext({ hasTouch: true, viewport: { width: 390, height: 844 } });
  try {
    const page = await context.newPage();
    const app = await openApp(page);
    const plot = await app.locator('.chart-hit').boundingBox();
    await app.locator('.chart-hit').tap({ position: { x: plot.width - 1, y: plot.height / 2 } });
    await expect(app.locator('#chart-reading')).toHaveValue('47');
    await expect(app.locator('.reading-time')).toHaveText(observations.points[47].time.replace('T', ' · '));
    expect(await app.locator('html').evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    expect(await page.evaluate(() => window.fixtureHost.calls)).toEqual([]);
    await page.locator('iframe').screenshot({ path: testInfo.outputPath('chart-inspector-narrow.png'), animations: 'disabled' });
    await expect(app.locator('#chart-reading')).toHaveValue('47');
  } finally {
    await context.close();
  }
});

test('resizing preserves the inspected reading and keyboard focus', async ({ page }) => {
  const app = await openApp(page);
  const slider = app.locator('#chart-reading');
  await slider.focus();
  await slider.press('End');
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(slider).toHaveValue('47');
  await expect(slider).toBeFocused();
  await slider.press('ArrowLeft');
  await expect(slider).toHaveValue('46');
  await expect(app.locator('.reading-time')).toHaveText(observations.points[46].time.replace('T', ' · '));
});

test('event coverage derives display rows without changing the received result', () => {
  const original = Object.freeze({ rain_in: 0, rain_readings: 0, lightning_strikes: 0, lightning_readings: 0, obs: 5 });
  const data = Object.freeze({ points: Object.freeze([original]) });
  const table = datasets(data, 'get_observations')[0];
  expect(table.rows[0]).toMatchObject({ rain_in: null, lightning_strikes: null });
  expect(table.rows[0]).not.toBe(original);
  expect(original).toMatchObject({ rain_in: 0, lightning_strikes: 0 });
  expect(datasets(data, 'daily_summary')[0].rows[0]).toBe(original);
});

test('event coverage distinguishes missing, measured zero, and partial totals', async ({ page }) => {
  const app = await openApp(page);
  const data = { ...observations, points: observations.points.slice(0, 3).map((point, i) => ({
    ...point, rain_in: i === 2 ? 0.125 : 0, rain_readings: [0, 30, 10][i],
    lightning_strikes: i === 2 ? 4 : 0, lightning_readings: [0, 30, 20][i],
  })) };
  await page.evaluate((data) => window.fixtureHost.send(data), data);
  const headers = await app.locator('thead th').allTextContents();
  const rain = headers.indexOf('Rainfall (in)'), lightning = headers.indexOf('Lightning strikes');
  expect(rain).toBeGreaterThan(-1);
  expect(lightning).toBeGreaterThan(-1);
  for (const [column, expected] of [[rain, ['—', '0', '0.125']], [lightning, ['—', '0', '4']]]) {
    await expect(app.locator(`tbody tr td:nth-child(${column + 1})`)).toHaveText(expected);
  }
  await expect(app.locator('#sensor-coverage')).toContainText('Rain: 1 bucket without readings, 1 bucket with partial readings.');
  await expect(app.locator('#sensor-coverage')).toContainText('Lightning: 1 bucket without readings, 1 bucket with partial readings.');
  await expect(app.locator('.metric').filter({ hasText: 'Rainfall' }).locator('.metric-value')).toHaveText('0.125in');
  for (const metric of ['rain_in', 'lightning_strikes']) {
    await app.locator('#metric').selectOption(metric);
    await expect(app.locator('.chart-bar')).toHaveCount(2);
    await expect(app.locator('.chart-bar title').first()).toContainText(': 0');
  }
  await app.locator('#search').fill('no matching row');
  await expect(app.locator('#sensor-coverage')).toBeVisible();
});

test('wholly missing event sensors do not become zero summary metrics', async ({ page }) => {
  const app = await openApp(page);
  const data = { ...observations, points: observations.points.slice(0, 2).map(point => ({
    ...point, rain_in: 0, rain_readings: 0, lightning_strikes: 0, lightning_readings: 0,
  })) };
  await page.evaluate((data) => window.fixtureHost.send(data), data);
  await expect(app.locator('.metric').filter({ hasText: 'Rainfall' }).locator('.metric-value')).toHaveText('—');
  await expect(app.locator('#metric option[value="rain_in"]')).toHaveCount(0);
  await expect(app.locator('#metric option[value="lightning_strikes"]')).toHaveCount(0);
  await expect(app.locator('#sensor-coverage')).toContainText('Rain: 2 buckets without readings.');
});

test('older observation results without coverage counts keep their existing display', async ({ page }) => {
  const app = await openApp(page);
  const data = { ...observations, points: observations.points.slice(0, 2).map(point => {
    const legacy = { ...point, rain_in: 0, lightning_strikes: 0 };
    delete legacy.rain_readings;
    delete legacy.lightning_readings;
    return legacy;
  }) };
  await page.evaluate((data) => window.fixtureHost.send(data), data);
  await expect(app.locator('#sensor-coverage')).toHaveCount(0);
  await expect(app.locator('.metric').filter({ hasText: 'Rainfall' }).locator('.metric-value')).toHaveText('0in');
  await app.locator('#metric').selectOption('rain_in');
  await expect(app.locator('.chart-bar')).toHaveCount(2);
});

test('SQL columns named like event coverage fields retain their literal values', async ({ page }) => {
  const app = await openApp(page, 'table');
  await page.evaluate(() => window.fixtureHost.send({
    columns: ['rain_in', 'rain_readings', 'lightning_strikes', 'lightning_readings', 'obs'],
    rows: [[0, 0, 0, 0, 30]], row_count: 1,
  }));
  await expect(app.locator('tbody tr td')).toHaveText(['0', '0', '0', '0', '30']);
  await expect(app.locator('#sensor-coverage')).toHaveCount(0);
});

test('an empty wind rose does not invent a preferred direction', async ({ page }) => {
  const app = await openApp(page, 'wind');
  await page.evaluate(() => {
    const data = structuredClone(window.fixtureHost.fixture.data);
    data.obs = 0; data.calm_pct = 0;
    data.sectors.forEach((row) => { row.pct = 0; row.count = 0; });
    return window.fixtureHost.send(data);
  });
  await expect(app.locator('.wind-note')).toContainText('No directional readings');
});

for (const view of ['daily', 'forecast', 'wind', 'conditions']) {
  test(`${view} tool view renders its actual structured shape`, async ({ page }) => {
    const errors = [];
    page.on('pageerror', (error) => errors.push(error.message));
    const app = await openApp(page, view);
    await expect(app.locator('.hero')).toBeVisible();
    if (view !== 'conditions') await expect(app.locator('svg').first()).toBeVisible();
    if (view === 'conditions') await expect(app.locator('#content')).toContainText(/archive/i);
    expect(errors).toEqual([]);
  });
}

test('host theme updates and narrow screens keep the document in bounds', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const app = await openApp(page, 'table', '&theme=dark');
  await expect(app.locator('html')).toHaveAttribute('data-theme', 'dark');
  await page.evaluate(() => window.fixtureHost.setTheme('light'));
  await expect(app.locator('html')).toHaveAttribute('data-theme', 'light');
  expect(await page.frames()[1].evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await expect(app.getByRole('button', { name: /next page/i })).toBeVisible();
});

test('empty results and a failed refresh recover without losing the result', async ({ page }) => {
  const app = await openApp(page, 'table');
  await page.evaluate(() => { window.fixtureHost.nextError = 'Synthetic refresh failed'; });
  await app.getByRole('button', { name: /refresh/i }).click();
  await expect(app.locator('#message')).toContainText(/failed|error|could not/i);
  await expect(app.locator('tbody tr')).toHaveCount(8);
  await app.getByRole('button', { name: /refresh/i }).click();
  await expect(app.locator('#message')).toBeHidden();
  await page.evaluate(() => window.fixtureHost.send({ columns: ['day', 'rain_mm'], rows: [], row_count: 0 }));
  await expect(app.locator('#content')).toContainText(/no .*rows|no .*data|no .*results|no readings/i);
});

test('loading state performs no automatic tool calls', async ({ page }) => {
  const app = await openApp(page, 'weather', '&loading');
  await expect(app.locator('#message')).toBeVisible();
  expect(await page.evaluate(() => window.fixtureHost.calls)).toEqual([]);
  await page.evaluate((data) => window.fixtureHost.send(data), observations);
  await expect(app.locator('svg').first()).toBeVisible();
});

test('a host without tool capability cannot trigger refresh', async ({ page }) => {
  const app = await openApp(page, 'weather', '&noTools');
  await expect(app.locator('#refresh')).toBeDisabled();
  expect(await page.evaluate(() => window.fixtureHost.calls)).toEqual([]);
});

test('refresh waits for the original input instead of inventing arguments', async ({ page }) => {
  const app = await openApp(page, 'table', '&noInput');
  await expect(app.locator('#refresh')).toBeDisabled();
  expect(await page.evaluate(() => window.fixtureHost.calls)).toEqual([]);
});

test('plain JSON text content is a supported result fallback', async ({ page }) => {
  const app = await openApp(page, 'table');
  await page.evaluate((data) => window.fixtureHost.bridge.sendToolResult({ content: [{ type: 'text', text: JSON.stringify(data) }] }), { ...query, rows: query.rows.slice(0, 2), row_count: 2, truncated: false });
  await expect(app.locator('tbody tr')).toHaveCount(2);
});

test('large results are bounded before rendering and disclose the display limit', async ({ page }) => {
  const app = await openApp(page, 'table');
  await page.evaluate(() => window.fixtureHost.send({
    columns: Array.from({ length: 70 }, (_, i) => `column_${i}`),
    rows: Array.from({ length: 2100 }, (_, i) => Array.from({ length: 70 }, (_, j) => i + j)),
    row_count: 2100,
  }));
  await expect(app.locator('thead th')).toHaveCount(64);
  await expect(app.locator('tbody tr')).toHaveCount(8);
  await expect(app.locator('#content')).toContainText('Display limited to 2,000 rows and 64 columns');
});

test('a late refresh cannot replace a newer host-pushed result', async ({ page }) => {
  const app = await openApp(page, 'table');
  await page.evaluate(() => { window.fixtureHost.holdNextCall = true; });
  await app.locator('#refresh').click();
  await expect.poll(() => page.evaluate(() => typeof window.fixtureHost.releaseCall)).toBe('function');
  await expect(app.locator('#refresh')).toBeDisabled();
  await page.evaluate(async () => {
    await window.fixtureHost.send({ columns: ['newer'], rows: [['Latest host result']], row_count: 1 });
    window.fixtureHost.releaseCall();
    // A round-trip ensures queued bridge messages are delivered before assertions.
    await window.fixtureHost.bridge.request({ method: 'ping' });
  });
  await expect(app.locator('tbody tr')).toHaveCount(1);
  await expect(app.locator('tbody')).toContainText('Latest host result');
});

test('malformed results surface an error and recover on valid host data', async ({ page }) => {
  const app = await openApp(page, 'table');
  await page.evaluate(() => window.fixtureHost.bridge.sendToolResult({ content: [{ type: 'text', text: '{invalid JSON' }] }));
  await expect(app.locator('#message')).toContainText('invalid JSON');
  await expect(app.locator('table')).toHaveCount(0);
  await page.evaluate((data) => window.fixtureHost.send(data), query);
  await expect(app.locator('tbody tr')).toHaveCount(8);
});

test('documentation screenshots', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 1100 });
  for (const [view, name] of [['weather', 'mcp-app-weather.png'], ['table', 'mcp-app-table.png']]) {
    const app = await openApp(page, view);
    await expect(app.locator('tbody tr').first()).toBeVisible();
    if (process.env.CAPTURE_SCREENSHOTS === '1') {
      await mkdir('docs/screenshots', { recursive: true });
      await page.locator('iframe').screenshot({ path: `docs/screenshots/${name}`, animations: 'disabled' });
    }
  }
});
