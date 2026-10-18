import { test, expect } from '@playwright/test';
import { stations } from './fixtures.mjs';
import { datasets } from '../../internal/mcpapp/ui/data.js';

async function openDevices(page) {
  await page.goto('/?view=stations');
  await expect.poll(() => page.evaluate(() => window.fixtureHost?.initialized)).toBe(true);
  const app = page.frameLocator('iframe');
  await expect(app.locator('#content')).toHaveAttribute('aria-busy', 'false');
  await expect(app.getByRole('button', { name: 'devices', exact: true })).toBeVisible();
  await app.getByRole('button', { name: 'devices', exact: true }).click();
  return app;
}

test('station devices expose exact searchable identities and their parent station', async ({ page }, testInfo) => {
  const requests = [];
  page.on('request', request => requests.push(request.url()));
  const app = await openDevices(page);
  await expect(app.locator('thead th')).toHaveText(['Station ID', 'Station name', 'Device ID', 'Device type', 'Serial number']);
  await app.locator('#search').fill('123456');
  await expect(app.locator('tbody tr')).toHaveCount(1);
  await expect(app.locator('tbody tr td')).toHaveText(['100001', 'Demo garden', '123456', 'ST', 'ST-SYNTHETIC-GARDEN']);
  await app.locator('#search').fill('ST-SYNTHETIC-ROOF-9');
  await expect(app.locator('tbody tr td')).toHaveText(['200002', 'Demo roof', '456709', 'ST', 'ST-SYNTHETIC-ROOF-9']);
  await app.locator('#search').fill('roof');
  await expect(app.locator('#row-count')).toContainText('10 rows');
  await app.locator('thead th').nth(2).getByRole('button').click();
  await app.locator('thead th').nth(2).getByRole('button').click();
  await expect(app.locator('tbody tr').first().locator('td').nth(2)).toHaveText('456709');
  await expect(app.locator('#chart-panel')).toHaveCount(0);
  expect(await page.evaluate(() => window.fixtureHost.calls)).toEqual([]);
  expect(requests.every(url => url.startsWith('http://127.0.0.1:4177/'))).toBe(true);
  await page.locator('iframe').screenshot({ path: testInfo.outputPath('devices-desktop.png'), animations: 'disabled' });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(app.locator('#search')).toBeVisible();
  expect(await app.locator('html').evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.locator('iframe').screenshot({ path: testInfo.outputPath('devices-narrow.png'), animations: 'disabled' });
});

test('unsafe numeric identifiers are marked without changing exact string identifiers', async ({ page }) => {
  const app = await openDevices(page);
  await page.evaluate(() => window.fixtureHost.send({ stations: [
    { station_id: Number.MAX_SAFE_INTEGER + 1, name: 'Numeric identity', devices: [{ device_id: Number.MAX_SAFE_INTEGER + 1, device_type: 'ST' }] },
    { station_id: '9007199254740993', name: 'String identity', devices: [{ device_id: '9007199254740995', device_type: 'ST' }] },
  ] }));
  const first = app.locator('tbody tr').first();
  await expect(first.locator('td').nth(0)).toHaveText('ID exceeds browser precision');
  await expect(first.locator('td').nth(2)).toHaveText('ID exceeds browser precision');
  await expect(first.locator('td').nth(2)).toHaveAttribute('title', 'ID exceeds browser precision');
  await expect(app.locator('tbody tr').last().locator('td').nth(0)).toHaveText('9007199254740993');
  await app.locator('#search').fill('9007199254740995');
  await expect(app.locator('tbody tr')).toHaveCount(1);
  await expect(app.locator('tbody tr td').nth(2)).toHaveText('9007199254740995');
  await app.locator('#search').fill('9007199254740992');
  await expect(app.locator('tbody')).toHaveText('No rows match your search.');
});

test('device rows retain duplicate parent associations, missing fields, and hostile text', async ({ page }) => {
  const app = await openDevices(page);
  const hostile = '<img src=x onerror="window.injected=true">';
  await page.evaluate(data => window.fixtureHost.send(data), { stations: [
    { station_id: 0, name: hostile, devices: [{ device_id: 123456, device_type: 'ST', serial_number: hostile }, { device_id: 123456, device_type: 'ST' }] },
    { station_id: 200002, name: 'Other synthetic station', devices: [{ device_id: 123456, device_type: 'ST' }, {}] },
    { name: 'No devices supplied', devices: null },
    { name: 'Empty devices', devices: [] },
  ] });
  await expect(app.locator('tbody tr')).toHaveCount(4);
  await expect(app.locator('tbody tr').first().locator('td')).toHaveText(['0', hostile, '123456', 'ST', hostile]);
  await expect(app.locator('tbody tr').nth(1).locator('td').last()).toHaveText('—');
  await expect(app.locator('tbody tr').last().locator('td')).toHaveText(['200002', 'Other synthetic station', '—', '—', '—']);
  await app.locator('#search').fill('123456');
  await expect(app.locator('tbody tr')).toHaveCount(3);
  await expect(app.locator('img')).toHaveCount(0);
  expect(await page.frames()[1].evaluate(() => window.injected)).toBeUndefined();
  expect(await page.evaluate(() => window.fixtureHost.calls)).toEqual([]);
});

test('device refresh retains controls through new and empty rows with a stable schema', async ({ page }) => {
  const app = await openDevices(page);
  await app.locator('#search').fill('ST');
  const sort = app.locator('thead th').nth(2).getByRole('button');
  await sort.click(); await sort.click();
  await app.locator('#next').click();
  const data = structuredClone(stations);
  data.stations[1].devices.push({ device_id: 456800, device_type: 'ST', serial_number: 'ST-SYNTHETIC-NEW' });
  await page.evaluate(data => { window.fixtureHost.fixture.data = data; }, data);
  await app.locator('#refresh').click();
  await expect(app.locator('#refresh')).toBeEnabled();
  await expect(app.getByRole('button', { name: 'devices', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(app.locator('#search')).toHaveValue('ST');
  await expect(app.locator('thead th').nth(2)).toHaveAttribute('aria-sort', 'descending');
  await expect(app.locator('#page-info')).toHaveText('2 / 2');
  await expect(app.locator('#row-count')).toContainText('12 rows');
  await expect(app.locator('tbody tr').first().locator('td').nth(2)).toHaveText('456702');
  await page.evaluate(() => { window.fixtureHost.fixture.data = { stations: [] }; });
  await app.locator('#refresh').click();
  await expect(app.locator('#refresh')).toBeEnabled();
  await expect(app.locator('#search')).toHaveValue('ST');
  await expect(app.locator('thead th')).toHaveCount(5);
  await expect(app.locator('thead th').nth(2)).toHaveAttribute('aria-sort', 'descending');
  await expect(app.locator('#page-info')).toHaveText('1 / 1');
  await expect(app.locator('tbody')).toHaveText('No rows match your search.');
  await app.locator('#search').fill('');
  await expect(app.locator('tbody')).toHaveText('No devices returned.');
  expect(await page.evaluate(() => window.fixtureHost.calls.map(({ name, arguments: args }) => ({ name, arguments: args })))).toEqual([
    { name: 'list_stations', arguments: {} }, { name: 'list_stations', arguments: {} },
  ]);
});

test('device traversal stops at both limits before inspecting omitted entries', () => {
  const tooManyStations = Array.from({ length: 2001 }, () => ({ devices: [] }));
  Object.defineProperty(tooManyStations, 2000, { get() { throw new Error('Read past the station bound'); } });
  expect(datasets({ stations: tooManyStations }, 'list_stations').find(table => table.name === 'devices')).toMatchObject({ rows: [], limited: true });

  const tooManyDevices = Array(2001).fill(null);
  Object.defineProperty(tooManyDevices, 2000, { get() { throw new Error('Read past the device bound'); } });
  const laterDevices = Array(1);
  Object.defineProperty(laterDevices, 0, { get() { throw new Error('Device budget restarted for another station'); } });
  const data = { stations: [{ devices: tooManyDevices }, { devices: laterDevices }] };
  expect(datasets(data, 'list_stations').find(table => table.name === 'devices')).toMatchObject({ rows: [], limited: true });
});

test('bounded device views disclose omissions while search and pagination stay local', async ({ page }) => {
  const app = await openDevices(page);
  await page.evaluate(() => window.fixtureHost.send({ stations: [{ station_id: 100001, name: 'Synthetic limit case', devices: Array.from({ length: 2100 }, (_, i) => ({ device_id: i + 100000, device_type: 'ST' })) }] }));
  await expect(app.locator('#content')).toContainText('Device inspection is limited to 2,000 stations and 2,000 device entries.');
  await expect(app.locator('thead th')).toHaveCount(5);
  await expect(app.locator('tbody tr')).toHaveCount(8);
  await expect(app.locator('#page-info')).toHaveText('1 / 250');
  await app.locator('#search').fill('102099');
  await expect(app.locator('tbody')).toHaveText('No rows match your search.');
  await expect(app.locator('#content')).toContainText('Search covers displayed entries only');
  await app.locator('#search').fill('101999');
  await expect(app.locator('tbody tr').first().locator('td').nth(2)).toHaveText('101999');
});

test('device derivation preserves raw results and does not interpret other tools or SQL', () => {
  const data = structuredClone(stations), original = structuredClone(data);
  for (const station of data.stations) {
    station.devices.forEach(Object.freeze); Object.freeze(station.devices); Object.freeze(station);
  }
  Object.freeze(data.stations); Object.freeze(data);
  const tables = datasets(data, 'list_stations');
  expect(data).toEqual(original);
  expect(tables.map(table => table.name)).toEqual(['stations', 'devices']);
  expect(tables[1].rows[0]).toEqual({ station_id: 100001, station_name: 'Demo garden', device_id: 123456, device_type: 'ST', serial_number: 'ST-SYNTHETIC-GARDEN' });
  expect(datasets(data, 'station_details').map(table => table.name)).toEqual(['stations']);
  const sql = datasets({ ...data, columns: ['device_id', 'station_id'], rows: [[123456, 100001]] }, 'query_sql');
  expect(sql.map(table => table.name)).toEqual(['Results']);
  expect(sql[0].rows).toEqual([[123456, 100001]]);
});
