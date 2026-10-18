import { test, expect } from '@playwright/test';
import { query, forecast } from './fixtures.mjs';

async function openApp(page, view = 'table') {
  await page.goto(`/?view=${view}`);
  await expect.poll(() => page.evaluate(() => window.fixtureHost?.initialized)).toBe(true);
  const app = page.frameLocator('iframe');
  await expect(app.locator('#content')).toHaveAttribute('aria-busy', 'false');
  return app;
}

async function refreshWith(page, app, data) {
  await page.evaluate(data => { window.fixtureHost.fixture.data = data; }, data);
  await app.locator('#refresh').click();
  await expect(app.locator('#refresh')).toBeEnabled();
  await expect(app.locator('#message')).toBeHidden();
}

async function exploreTable(app) {
  await app.locator('#search').fill('Complete');
  const sort = app.locator('thead th').nth(1).getByRole('button');
  await sort.click();
  await sort.click();
  await app.locator('#next').click();
}

test('refresh retains the current search, descending sort, and table page', async ({ page }, testInfo) => {
  const app = await openApp(page);
  await exploreTable(app);
  const first = await app.locator('tbody tr').first().locator('td').allTextContents();
  const rows = query.rows.map(row => row.map((value, i) => i === 1 && value !== null ? value + 100 : value));
  await refreshWith(page, app, { ...query, rows, note: 'Fresh compatible rows.' });
  await expect(app.locator('#content')).toContainText('Fresh compatible rows.');
  await expect(app.locator('#search')).toHaveValue('Complete');
  await expect(app.locator('thead th').nth(1)).toHaveAttribute('aria-sort', 'descending');
  await expect(app.locator('#page-info')).toHaveText('2 / 8');
  await expect(app.locator('tbody tr').first().locator('td')).toHaveText(first.map((value, i) => i === 1 ? String(Number(value) + 100) : value));
  await expect(app.locator('#refresh')).toBeFocused();
  await page.locator('iframe').screenshot({ path: testInfo.outputPath('refreshed-table-desktop.png'), animations: 'disabled' });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.locator('iframe').screenshot({ path: testInfo.outputPath('refreshed-table-narrow.png'), animations: 'disabled' });
});

test('refresh clamps the page to filtered matches, including no matches', async ({ page }) => {
  const app = await openApp(page);
  await exploreTable(app);
  await app.locator('#next').click();
  const rows = query.rows.slice(0, 5);
  await refreshWith(page, app, { ...query, rows, row_count: rows.length });
  await expect(app.locator('#search')).toHaveValue('Complete');
  await expect(app.locator('#page-info')).toHaveText('1 / 1');
  await expect(app.locator('tbody tr')).toHaveCount(5);
  await expect(app.locator('thead th').nth(1)).toHaveAttribute('aria-sort', 'descending');
  await refreshWith(page, app, { ...query, rows: [query.rows[5]], row_count: 1 });
  await expect(app.locator('#search')).toHaveValue('Complete');
  await expect(app.locator('tbody')).toContainText('No rows match your search.');
  await expect(app.locator('#page-info')).toHaveText('1 / 1');
  await expect(app.locator('#previous')).toBeDisabled();
  await expect(app.locator('#next')).toBeDisabled();
});

test('edits made during refresh retain search focus and text selection', async ({ page }) => {
  const app = await openApp(page);
  await page.evaluate(() => { window.fixtureHost.holdNextCall = true; });
  await app.locator('#refresh').click();
  await expect.poll(() => page.evaluate(() => typeof window.fixtureHost.releaseCall)).toBe('function');
  await app.locator('#search').fill('Partial');
  await app.locator('#search').evaluate(input => input.setSelectionRange(1, 4, 'forward'));
  await page.evaluate(() => window.fixtureHost.releaseCall());
  await expect(app.locator('#refresh')).toBeEnabled();
  await expect(app.locator('#search')).toHaveValue('Partial');
  await expect(app.locator('#search')).toBeFocused();
  expect(await app.locator('#search').evaluate(input => [input.selectionStart, input.selectionEnd, input.selectionDirection])).toEqual([1, 4, 'forward']);
  await expect(app.locator('tbody tr')).toHaveCount(1);
});

test('a refresh does not steal focus after the user leaves the app', async ({ page }) => {
  const app = await openApp(page);
  await page.evaluate(() => { window.fixtureHost.holdNextCall = true; });
  await app.locator('#refresh').click();
  await expect.poll(() => page.evaluate(() => typeof window.fixtureHost.releaseCall)).toBe('function');
  await app.locator('#search').fill('Partial');
  const outside = page.locator('header strong');
  await outside.evaluate(element => { element.tabIndex = 0; element.focus(); });
  await page.evaluate(() => window.fixtureHost.releaseCall());
  await expect(app.locator('#refresh')).toBeEnabled();
  await expect(app.locator('#search')).toHaveValue('Partial');
  await expect(outside).toBeFocused();
});

test('refresh follows the chosen dataset by name and resets if it disappears', async ({ page }) => {
  const app = await openApp(page, 'forecast');
  await app.getByRole('button', { name: 'daily', exact: true }).click();
  await app.locator('#search').fill('Partly');
  await app.locator('thead').getByRole('button', { name: /High temperature/ }).click();
  await app.locator('#chart-reading').press('End');
  await refreshWith(page, app, { hourly: forecast.hourly, current: forecast.current, daily: forecast.daily });
  await expect(app.getByRole('button', { name: 'daily', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(app.locator('#search')).toHaveValue('Partly');
  await expect(app.locator('th[aria-sort="ascending"]')).toHaveText('High temperature (°F)');
  await expect(app.locator('#chart-reading')).toHaveValue('0');
  await expect(app.locator('#chart-reading')).toHaveAttribute('max', '6');
  await refreshWith(page, app, { hourly: forecast.hourly, current: forecast.current });
  await expect(app.getByRole('button', { name: 'hourly', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(app.locator('#search')).toHaveValue('');
  await expect(app.locator('th[aria-sort="ascending"]')).toHaveCount(0);
});

test('SQL duplicate positions survive refresh but reordered or changed names reset controls', async ({ page }) => {
  const app = await openApp(page);
  const data = { columns: ['group', 'reading', 'reading'], rows: Array.from({ length: 24 }, (_, i) => ['keep', 24 - i, i]), row_count: 24 };
  await page.evaluate(data => window.fixtureHost.send(data), data);
  await expect(app.locator('thead th')).toHaveCount(3);
  await app.locator('#search').fill('keep');
  await app.locator('thead th').nth(2).getByRole('button').click();
  await app.locator('thead th').nth(2).getByRole('button').click();
  await app.locator('#next').click();
  await refreshWith(page, app, data);
  await expect(app.locator('#search')).toHaveValue('keep');
  await expect(app.locator('thead th').nth(2)).toHaveAttribute('aria-sort', 'descending');
  await expect(app.locator('tbody tr').first().locator('td').nth(2)).toHaveText('15');
  await refreshWith(page, app, { ...data, columns: ['reading', 'group', 'reading'], rows: data.rows.map(([group, a, b]) => [a, group, b]) });
  await expect(app.locator('#search')).toHaveValue('');
  await expect(app.locator('#page-info')).toHaveText('1 / 3');
  await expect(app.locator('th[aria-sort="descending"]')).toHaveCount(0);

  const prefix = 'column'.repeat(30);
  await refreshWith(page, app, { columns: [`${prefix}first`], rows: [['visible']], row_count: 1 });
  await app.locator('#search').fill('hidden');
  // Labels are capped for display; comparison must still use the complete name.
  await refreshWith(page, app, { columns: [`${prefix}second`], rows: [['visible']], row_count: 1 });
  await expect(app.locator('#search')).toHaveValue('');
  await expect(app.locator('tbody')).toHaveText('visible');
});

test('new tool input clears old filters and supersedes a pending refresh', async ({ page }) => {
  const app = await openApp(page);
  await exploreTable(app);
  await page.evaluate(() => { window.fixtureHost.holdNextCall = true; });
  await app.locator('#refresh').click();
  await expect.poll(() => page.evaluate(() => typeof window.fixtureHost.releaseCall)).toBe('function');
  await page.evaluate(async data => {
    await window.fixtureHost.bridge.sendToolInput({ arguments: { sql: 'SELECT new_context', max_rows: 5 } });
    await window.fixtureHost.send({ ...data, rows: data.rows.slice(0, 3), note: 'New request context.' });
    window.fixtureHost.releaseCall();
    await window.fixtureHost.bridge.request({ method: 'ping' });
  }, query);
  await expect(app.locator('#search')).toHaveValue('');
  await expect(app.locator('th[aria-sort="descending"]')).toHaveCount(0);
  await expect(app.locator('#page-info')).toHaveText('1 / 1');
  await expect(app.locator('tbody tr')).toHaveCount(3);
  await expect(app.locator('#content')).toContainText('New request context.');
});

test('failed refresh retains the filtered sorted page', async ({ page }) => {
  const app = await openApp(page);
  await exploreTable(app);
  const first = await app.locator('tbody tr').first().textContent();
  await page.evaluate(() => { window.fixtureHost.nextError = 'Synthetic refresh failure.'; });
  await app.locator('#refresh').click();
  await expect(app.locator('#message')).toContainText('Synthetic refresh failure.');
  await expect(app.locator('#search')).toHaveValue('Complete');
  await expect(app.locator('thead th').nth(1)).toHaveAttribute('aria-sort', 'descending');
  await expect(app.locator('#page-info')).toHaveText('2 / 8');
  await expect(app.locator('tbody tr').first()).toHaveText(first);
});
