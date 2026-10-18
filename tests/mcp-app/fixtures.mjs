// Deliberately synthetic observations. Never substitute a private archive here.
const day = (index) => `2026-06-${String(index + 1).padStart(2, '0')}`;
const rounded = (n) => Math.round(n * 100) / 100;

export const observations = {
  from: '2026-06-14T00:00:00-07:00',
  to: '2026-06-14T23:59:59-07:00',
  bucket_minutes: 30,
  points: Array.from({ length: 48 }, (_, i) => {
    const hour = String(Math.floor(i / 2)).padStart(2, '0');
    const time = `2026-06-14T${hour}:${i % 2 ? '30' : '00'}:00-07:00`;
    const temp = 64 + 11 * Math.sin((i - 13) * Math.PI / 30) + Math.sin(i * 1.7) * 0.7;
    return {
      epoch: Date.parse(time) / 1000, time,
      temp_avg_f: rounded(temp), temp_min_f: rounded(temp - 0.8), temp_max_f: rounded(temp + 1.2),
      humidity_pct: rounded(64 - (temp - 60) * 1.4), pressure_inhg: rounded(30.12 - i * 0.002),
      wind_mph: rounded(3 + Math.max(0, Math.sin((i - 15) / 10)) * 6),
      gust_mph: rounded(6 + Math.max(0, Math.sin((i - 15) / 10)) * 9),
      rain_in: i >= 39 && i <= 42 ? 0.02 : 0,
      rain_readings: 30,
      uv_max: rounded(Math.max(0, Math.sin((i - 12) * Math.PI / 26)) * (i < 40 ? 7 : 0)),
      solar_avg_wm2: rounded(Math.max(0, Math.sin((i - 12) * Math.PI / 26)) * (i < 40 ? 780 : 0)),
      lightning_strikes: 0, lightning_readings: 30, obs: 30,
    };
  }),
};

export const daily = {
  from: day(0), to: day(13),
  days: Array.from({ length: 14 }, (_, i) => ({
    day: day(i), temp_min_f: rounded(51 + Math.sin(i) * 4),
    temp_max_f: rounded(73 + Math.sin(i / 3) * 8), temp_avg_f: rounded(63 + Math.sin(i / 3) * 5),
    rain_in: i % 5 === 1 ? 0.23 : 0, peak_gust_mph: rounded(8 + Math.abs(Math.sin(i) * 13)), obs: 1440,
  })),
};

export const query = {
  columns: ['day', 'temperature_c', 'humidity_pct', 'rain_mm', 'peak_wind_mps', 'observations', 'coverage'],
  rows: Array.from({ length: 64 }, (_, i) => [
    new Date(Date.UTC(2026, 3, i + 1)).toISOString().slice(0, 10),
    i === 5 ? null : rounded(17 + Math.sin(i / 5) * 6),
    rounded(60 + Math.cos(i / 4) * 14), i % 7 === 2 ? 4.8 : 0,
    rounded(3 + Math.abs(Math.sin(i)) * 7), i === 5 ? 960 : 1440,
    i === 5 ? 'Partial' : 'Complete',
  ]),
  row_count: 64,
  truncated: true,
  note: 'Synthetic daily aggregates. SQL values keep their selected units; more rows exist beyond this result.',
};

export const forecast = {
  station: 'Demo garden · synthetic station',
  current: { time: observations.points[30].time, conditions: 'Partly cloudy', temp_f: 72.4, feels_like_f: 71.8, humidity_pct: 52 },
  daily: daily.days.slice(0, 7).map((d, i) => ({ date: d.day, conditions: i === 2 ? 'Light rain' : 'Partly cloudy', high_f: d.temp_max_f, low_f: d.temp_min_f, precip_probability: i === 2 ? 65 : 10, sunrise: '05:42', sunset: '20:31' })),
  hourly: observations.points.filter((_, i) => i % 2 === 0).map((p) => ({ time: p.time, conditions: 'Partly cloudy', temp_f: p.temp_avg_f, feels_like_f: p.temp_avg_f, precip_probability: 10, wind_mph: p.wind_mph, gust_mph: p.gust_mph, wind_dir: 'SW' })),
};

const sectors = ['N', 'NNE', 'NE', 'ENE', 'E', 'ESE', 'SE', 'SSE', 'S', 'SSW', 'SW', 'WSW', 'W', 'WNW', 'NW', 'NNW'];
export const wind = {
  from: day(0), to: day(13), calm_pct: 12.5, calm_threshold_mph: 1.118, obs: 16000,
  sectors: sectors.map((sector, i) => ({ sector, count: i < 8 ? 200 : 1550, pct: (i < 8 ? 200 : 1550) / 140, avg_mph: rounded(4 + Math.sin(i) * 2), max_gust_mph: rounded(11 + Math.sin(i) * 4) })),
};

export const stations = {
  stations: [
    { station_id: 100001, name: 'Demo garden', latitude: 0, longitude: 0, timezone: 'UTC', devices: [
      { device_id: 123456, device_type: 'ST', serial_number: 'ST-SYNTHETIC-GARDEN' },
      { device_id: 234567, device_type: 'HB', serial_number: 'HB-SYNTHETIC-GARDEN' },
    ] },
    { station_id: 200002, name: 'Demo roof', latitude: 0, longitude: 0, timezone: 'UTC', devices: Array.from({ length: 10 }, (_, i) => ({
      device_id: 456700 + i, device_type: 'ST', serial_number: `ST-SYNTHETIC-ROOF-${i}`,
    })) },
  ],
};

export const rainSummary = {
  from: day(0), to: day(2), total_in: null, observations: 12, rain_readings: 0,
  days_observed: 3, rainy_days: 0, longest_dry_spell_days: 0, longest_wet_spell_days: 0,
};

export const fixtures = {
  weather: { tool: 'get_observations', args: { start: day(13), end: day(13), bucket_minutes: 30 }, data: observations },
  table: { tool: 'query_sql', args: {
    sql: "SELECT date(epoch, 'unixepoch') AS day, AVG(air_temp_c) AS temperature_c, AVG(humidity) AS humidity_pct, SUM(rain_mm) AS rain_mm, MAX(wind_gust) AS peak_wind_mps, COUNT(*) AS observations, CASE WHEN COUNT(*) >= 1440 THEN 'Complete' ELSE 'Partial' END AS coverage FROM obs_st GROUP BY day ORDER BY day",
    max_rows: 64,
  }, data: query },
  daily: { tool: 'daily_summary', args: { start: day(0), end: day(13) }, data: daily },
  forecast: { tool: 'forecast', args: { hours: 24, days: 7 }, data: forecast },
  wind: { tool: 'wind_rose', args: { start: day(0), end: day(13) }, data: wind },
  stations: { tool: 'list_stations', args: {}, data: stations },
  rain: { tool: 'rain_stats', args: { start: day(0), end: day(2) }, data: rainSummary },
  conditions: { tool: 'current_conditions', args: {}, data: { source: 'archive', time: observations.points[30].time, age_seconds: 900, temp_f: 72.4, humidity_pct: 0, rain_today_in: 0, wind_mph: 4.6, gust_mph: 8.2, note: 'Synthetic archive fallback.' } },
};

export function toolResult(data) {
  return { content: [{ type: 'text', text: JSON.stringify(data) }], structuredContent: data, isError: false };
}
