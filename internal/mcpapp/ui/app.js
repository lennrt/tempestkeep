import { App, applyDocumentTheme, applyHostStyleVariables } from "@modelcontextprotocol/ext-apps";
import { PAGE_SIZE, MAX_COLUMNS, number, object, text, pretty, label, unit, formatted, datasets, numericColumns, cell, compare, dateLabel, timeLabel, rainSummaryDisplay } from "./data.js";
import { seriesChart, windChart } from "./charts.js";

const TITLES = {
  current_conditions: "Current conditions", get_observations: "Weather in detail", daily_summary: "Daily weather", forecast: "Weather ahead", query_sql: "Explore your archive", wind_rose: "Where the wind comes from", list_stations: "Your stations", station_details: "Station details", station_info: "Station & archive", archive_status: "Archive health", records: "Weather records", this_day_in_history: "This day, through the years", period_summary: "Weather over time", climate_normals: "The shape of your seasons", rain_stats: "Rainfall patterns", pressure_trend: "Pressure tendency", climate_indices: "Climate indices", climatology: "Your daily rhythm", degree_days: "Degree days", lightning_activity: "Lightning activity", solar_stats: "Sun & solar", wind_stats: "Wind patterns", comfort_stats: "How the weather feels", sensor_health: "Sensor health", pressure_stats: "Pressure patterns", temperature_spells: "Heat waves & cold snaps", temperature_trend: "Temperature trend",
};
const tool = document.body.dataset.tool;
const validTool = Object.hasOwn(TITLES, tool);
const app = new App({ name: "TempestKeep Weather Explorer", version: __TEMPESTKEEP_VERSION__ }, {});
const byId = (id) => document.getElementById(id);
const content = byId("content"), message = byId("message"), refresh = byId("refresh");
const state = { data: null, tables: [], table: 0, search: "", sort: null, direction: 1, page: 0, metric: "", args: null, connected: false, busy: false, generation: 0 };

function el(tag, className, value) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (value !== undefined) node.textContent = String(value);
  return node;
}
function button(value, className, callback, tableFocus) {
  const node = el("button", className, value);
  node.type = "button"; node.addEventListener("click", callback);
  if (tableFocus) node.dataset.tableFocus = tableFocus;
  return node;
}
function status(value = "", error = false) {
  message.textContent = value;
  message.classList.toggle("error", error);
  message.setAttribute("role", error ? "alert" : "status");
  content.setAttribute("aria-busy", String(state.busy));
}
function updateRefresh() {
  const supported = Boolean(app.getHostCapabilities()?.serverTools);
  refresh.disabled = !validTool || !state.connected || !supported || !state.args || state.busy;
  refresh.title = !supported ? "This host does not support refreshing server tools." : !state.args ? "Waiting for the original tool arguments." : "Run this read-only tool again with its original arguments.";
}
function resetExploration() {
  state.table = 0; state.search = ""; state.sort = null; state.direction = 1; state.page = 0; state.metric = "";
}
function compatibleTable(previous, next, previousData, nextData) {
  return previous && next && previous.name === next.name && previous.sql === next.sql &&
    previous.columns.length === next.columns.length && previous.columns.every((column, i) =>
      column.key === next.columns[i].key && (previous.sql ? previousData.columns[i] === nextData.columns[i] : column.raw === next.columns[i].raw));
}
function tableFocus() {
  const active = document.activeElement;
  if (!document.hasFocus() || !active?.dataset.tableFocus || !byId("data-panel")?.contains(active)) return null;
  return { key: active.dataset.tableFocus, selection: active.id === "search" ? [active.selectionStart, active.selectionEnd, active.selectionDirection] : null };
}
function restoreTableFocus(focus) {
  if (!focus) return;
  let control = [...byId("data-panel").querySelectorAll("[data-table-focus]")].find(node => node.dataset.tableFocus === focus.key);
  if (control?.disabled) control = byId("search");
  control?.focus({ preventScroll: true });
  if (focus.selection && control?.id === "search") control.setSelectionRange(...focus.selection);
}
function hostContext(context) {
  if (!context) return;
  if (context.theme) applyDocumentTheme(context.theme);
  if (context.styles?.variables) applyHostStyleVariables(context.styles.variables);
  // External host fonts are deliberately not loaded; the resource has no network access.
}
function numberValue(value, key, sql = false) {
  const node = el("span", "", formatted(value, key, sql));
  if (value != null && unit(key) && !sql) node.append(el("small", "", unit(key)));
  return node;
}
function stat(labelText, value, key, note, hero = false) {
  const item = el("div", hero ? "hero-stat" : "metric");
  item.append(el("div", hero ? "hero-label" : "metric-label", labelText));
  const valueNode = el("div", hero ? "hero-value" : "metric-value");
  valueNode.append(numberValue(value, key)); item.append(valueNode);
  if (note) item.append(el("div", hero ? "hero-note" : "metric-note", note));
  return item;
}
function aggregate(rows, key, operation) {
  const values = rows.map(row => row[key]).filter(number);
  if (!values.length) return null;
  if (operation === "sum") return values.reduce((sum, value) => sum + value, 0);
  if (operation === "max") return Math.max(...values);
  if (operation === "min") return Math.min(...values);
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}
function heroBlock(data, primary, rows) {
  const hero = el("section", "hero"); hero.setAttribute("aria-label", "Weather overview");
  const main = el("div");
  const conditions = tool === "current_conditions" ? data : tool === "forecast" ? data.current : null;
  const temperature = conditions ? conditions.temp_f : aggregate(rows, "temp_avg_f");
  if (tool === "rain_stats") {
    main.append(el("div", "hero-label", "Rainfall"));
    const value = el("div", "hero-value"); value.append(numberValue(data.total_in, "total_in"));
    main.append(value, el("div", "hero-note", data.total_in == null ? "Rainfall unavailable" : "Total of reported rain readings"));
    hero.append(main,
      stat("Days observed", data.days_observed, "", "Days with stored observations", true),
      stat("Rainy days", data.rainy_days, "", "At least 0.01 in of reported rain", true));
  } else if (tool === "query_sql") {
    main.append(el("div", "hero-label", "A closer look at your data"), el("div", "hero-value", `${primary?.rows.length.toLocaleString() ?? 0} rows to explore`), el("div", "hero-note", "Search, sort, and discover patterns in your result."));
    hero.append(main, stat("Columns", primary?.columns.length ?? 0, "", "As selected by your query", true), stat("Result", data.truncated ? "Capped" : "Complete", "", data.truncated ? "More rows exist on the server" : "All returned rows available", true));
  } else if (conditions || temperature !== null) {
    main.append(el("div", "hero-label", conditions ? conditions.conditions || "Latest reading" : "Average of returned readings"));
    const value = el("div", "hero-value"); value.append(numberValue(temperature, "temp_f")); main.append(value);
    main.append(el("div", "hero-note", conditions ? (data.station || (data.source === "archive" ? "From your local archive" : "From your Tempest station")) : `${rows.length.toLocaleString()} ${pretty(primary?.name ?? "readings")} · ${data.bucket_minutes ? `${data.bucket_minutes}-minute buckets` : "Archive weather"}`));
    hero.append(main,
      stat(conditions ? "Feels like" : "Highest reading", conditions ? conditions.feels_like_f : aggregate(rows, "temp_max_f", "max"), "temp_f", conditions ? "Apparent temperature" : "In this returned series", true),
      stat(conditions ? "Humidity" : "Lowest reading", conditions ? conditions.humidity_pct : aggregate(rows, "temp_min_f", "min"), conditions ? "humidity_pct" : "temp_f", conditions ? "Relative humidity" : "In this returned series", true));
  } else {
    const total = object(data.total) ? data.total : data;
    const fields = Object.entries(total).filter(([key, value]) => number(value) && !/epoch|(^|_)id$/.test(key)).slice(0, 3);
    const [key, value] = fields[0] ?? [tool === "list_stations" ? primary?.name ?? "stations" : "Readings", primary?.rows.length ?? 0];
    main.append(el("div", "hero-label", label(key)));
    const big = el("div", "hero-value"); big.append(numberValue(value, key)); main.append(big, el("div", "hero-note", "From the returned tool result")); hero.append(main);
    fields.slice(1).forEach(([field, val]) => hero.append(stat(label(field), val, field, "", true)));
  }
  return hero;
}
function metricCards(data, rows) {
  const cards = el("section", "metrics"); cards.setAttribute("aria-label", "Summary metrics");
  if (tool === "query_sql") return cards;
  const conditions = tool === "current_conditions";
  const series = rows.some(row => Object.hasOwn(row, "temp_avg_f") || Object.hasOwn(row, "temp_f"));
  if (conditions || series) {
    const windKey = rows.some(row => Object.hasOwn(row, "peak_gust_mph")) ? "peak_gust_mph" : "gust_mph";
    const entries = conditions ? [
      ["Wind speed", data.wind_mph, "wind_mph", data.wind_dir || "Reported average"], ["Wind gust", data.gust_mph, "gust_mph", "Latest observation"], ["Rain today", data.rain_today_in, "rain_today_in", "Local calendar day"], ["Pressure", data.pressure_inhg, "pressure_inhg", data.pressure_trend || "Reported pressure"],
    ] : [
      ["Rainfall", aggregate(rows, "rain_in", "sum"), "rain_in", "Total in returned rows"], ["Peak gust", aggregate(rows, windKey, "max"), "gust_mph", "Highest returned reading"], ["Humidity", aggregate(rows, "humidity_pct"), "humidity_pct", "Average of returned readings"], ["Observations", aggregate(rows, "obs", "sum"), "obs", "Contributing samples"],
    ];
    // Forecasts have no rainfall totals or observation counts; show forecast data.
    if (tool === "forecast") entries.splice(0, entries.length,
      ["Rain chance", aggregate(rows, "precip_probability", "max"), "precip_probability", "Highest hourly probability"], ["Peak gust", aggregate(rows, "gust_mph", "max"), "gust_mph", "Hourly forecast maximum"], ["Wind speed", aggregate(rows, "wind_mph"), "wind_mph", "Average hourly forecast"], ["Hourly outlook", rows.length, "", "Hours returned"]);
    entries.forEach(args => cards.append(stat(...args)));
  }
  return cards;
}
function coverageNotice(table) {
  if (!table?.coverage?.length) return null;
  const bucketCount = count => `${count.toLocaleString()} ${count === 1 ? "bucket" : "buckets"}`;
  const descriptions = table.coverage.map(sensor => {
    const parts = [];
    if (sensor.missing) parts.push(`${bucketCount(sensor.missing)} without readings`);
    if (sensor.partial) parts.push(`${bucketCount(sensor.partial)} with partial readings`);
    return `${sensor.name}: ${parts.join(", ")}.`;
  });
  const notice = el("p", "note", `Sensor coverage is incomplete. ${descriptions.join(" ")} Totals include reported readings only; missing buckets display —. Reading counts describe samples, not continuous uptime.`);
  notice.id = "sensor-coverage";
  return notice;
}
function rainSummaryNotice(data) {
  const observations = data.observations, readings = data.rain_readings;
  if (![observations, readings].every(value => Number.isSafeInteger(value) && value >= 0) || readings > observations) return null;
  const condition = observations === 0 ? "No observations in this range. Rainfall is unavailable. " : readings === 0 ? "No rain readings were reported. Rainfall and rain statistics are unavailable. " : readings < observations ? "Partial rainfall total: includes reported readings only. " : "";
  const notice = el("p", "note", `${condition}Rain readings: ${formatted(readings)} of ${formatted(observations)} stored observations. Counts describe stored samples, not continuous coverage.`);
  notice.id = "rain-coverage";
  return notice;
}
function chartPanel(table, selectedIndex = 0) {
  const columns = numericColumns(table);
  if (!columns.length || !table.rows.length || table.name === "sectors" || ["stations", "devices", "gaps"].includes(table.name)) return null;
  if (!columns.some(column => column.key === state.metric)) state.metric = columns.find(column => ["temp_avg_f", "temp_f", "high_f", "coverage_pct"].includes(column.key))?.key ?? columns[0].key;
  const panel = el("section", "panel"); panel.id = "chart-panel";
  const head = el("div", "panel-head"), title = el("div");
  title.append(el("h2", "", label(state.metric)), el("p", "panel-description", `${table.rows.length.toLocaleString()} returned readings · ${unit(state.metric) || "Reported values"}`));
  const select = el("select", "metric-select"); select.id = "metric"; select.setAttribute("aria-label", "Chart metric");
  columns.forEach(column => { const option = el("option", "", column.title); option.value = column.key; select.append(option); }); select.value = state.metric;
  select.addEventListener("change", () => { state.metric = select.value; const replacement = chartPanel(table); panel.replaceWith(replacement); replacement.querySelector("select").focus(); });
  head.append(title, select); panel.append(head);
  const wrap = el("div", "chart-wrap"); wrap.append(seriesChart(table.rows, state.metric, tool === "get_observations" ? state.data.bucket_minutes : 0, selectedIndex)); panel.append(wrap);
  const foot = el("div", "chart-footer"); foot.append(el("span", "legend", label(state.metric)), el("span", "", "Missing values leave gaps · Only returned readings are shown")); panel.append(foot);
  return panel;
}
function windPanel(data, table) {
  const panel = el("section", "panel"); panel.id = "chart-panel";
  const head = el("div", "panel-head"); head.append(el("h2", "", "Wind direction frequency"), el("span", "badge", "16 compass sectors")); panel.append(head);
  const wrap = el("div", "wind-layout"); wrap.append(windChart(table.rows.slice(0, 16)));
  const strongest = table.rows.filter(row => number(row.pct) && row.pct > 0).reduce((best, row) => !best || row.pct > best.pct ? row : best, null);
  const note = el("div", "wind-note");
  note.append(el("div", "eyebrow", "Most frequent direction"), el("h3", "", strongest ? `${strongest.sector} · ${formatted(strongest.pct)}%` : "No directional readings"), el("p", "", `${formatted(data.calm_pct)}% calm below ${formatted(data.calm_threshold_mph)} mph.`), el("p", "", "Wedge area shows the share of directional observations. Exact frequencies and speeds are in the table."));
  wrap.append(note); panel.append(wrap); return panel;
}
function tablePanel() {
  const table = state.tables[state.table];
  const panel = el("section", "panel"); panel.id = "data-panel";
  const head = el("div", "panel-head"), title = el("div");
  title.append(el("h2", "", tool === "query_sql" ? "Query results" : tool === "list_stations" ? label(table.name) : "Explore the readings"), el("p", "panel-description", table.sql ? "Original column names and values · Units follow your SELECT" : tool === "list_stations" && table.name === "devices" ? "Devices supplied by each station · Search by ID, type, serial, or station" : "Click a column to sort · Search across all returned rows"));
  head.append(title, el("span", "badge", `${table.rows.length.toLocaleString()} rows`)); panel.append(head);
  const tools = el("div", "table-tools"), tabs = el("div", "tabs");
  tabs.setAttribute("aria-label", "Datasets");
  state.tables.forEach((dataset, i) => {
    const tab = button(pretty(dataset.name), "tab", () => {
      state.table = i; state.page = 0; state.search = ""; state.sort = null; state.metric = ""; render();
    }, `dataset:${dataset.name}`); tab.setAttribute("aria-pressed", String(i === state.table)); tabs.append(tab);
  });
  const searchWrap = el("label", "search-wrap"); searchWrap.append(el("span", "sr-only", "Search rows"));
  const search = el("input", "search"); search.id = "search"; search.type = "search"; search.placeholder = tool === "list_stations" ? `Search ${table.name}…` : "Search readings…"; search.value = state.search; search.maxLength = 200; search.dataset.tableFocus = "search";
  search.addEventListener("input", () => { state.search = search.value; state.page = 0; renderRows(); }); searchWrap.append(search); tools.append(tabs, searchWrap); panel.append(tools);
  const scroll = el("div", "table-scroll"); scroll.tabIndex = 0; scroll.dataset.tableFocus = "table"; scroll.setAttribute("role", "region"); scroll.setAttribute("aria-label", "Scrollable data table");
  const grid = el("table"); grid.id = "data-table";
  const caption = el("caption", "sr-only", `${TITLES[tool]}: ${pretty(table.name)}. Missing values are shown as an em dash.`); grid.append(caption);
  const thead = el("thead"), tr = el("tr");
  table.columns.forEach((column, index) => {
    const th = el("th"); th.scope = "col";
    if (table.rows.some(row => number(cell(row, column)))) th.className = "numeric";
    th.setAttribute("aria-sort", state.sort === index ? state.direction === 1 ? "ascending" : "descending" : "none");
    th.append(button(column.title, "", () => {
      state.direction = state.sort === index ? -state.direction : 1; state.sort = index; state.page = 0;
      [...thead.querySelectorAll("th")].forEach((node, i) => node.setAttribute("aria-sort", state.sort === i ? state.direction === 1 ? "ascending" : "descending" : "none")); renderRows();
    }, `sort:${index}`)); tr.append(th);
  }); thead.append(tr); grid.append(thead, el("tbody")); scroll.append(grid); panel.append(scroll);
  const bottom = el("div", "table-bottom"), info = el("span"); info.id = "row-count"; info.setAttribute("aria-live", "polite");
  const pagination = el("div", "pagination"), pageInfo = el("span"); pageInfo.id = "page-info";
  const previous = button("‹", "button", () => { state.page--; renderRows(); }, "previous"); previous.id = "previous"; previous.setAttribute("aria-label", "Previous page");
  const next = button("›", "button", () => { state.page++; renderRows(); }, "next"); next.id = "next"; next.setAttribute("aria-label", "Next page");
  pagination.append(previous, pageInfo, next); bottom.append(info, pagination); panel.append(bottom);
  if (table.limited) panel.append(el("p", "note", table.limitNotice ?? "Display limited to 2,000 rows and 64 columns. Narrow the tool request to inspect more."));
  return panel;
}
function renderRows() {
  const table = state.tables[state.table];
  if (!table) return;
  const query = state.search.trim().toLocaleLowerCase();
  const rows = table.rows.filter(row => !query || table.columns.some(column => formatted(cell(row, column), column.raw, table.sql).toLocaleLowerCase().includes(query)));
  if (state.sort !== null) { const column = table.columns[state.sort]; rows.sort((a, b) => compare(cell(a, column), cell(b, column), state.direction)); }
  const pages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE)); state.page = Math.max(0, Math.min(state.page, pages - 1));
  const body = byId("data-table").querySelector("tbody"); body.replaceChildren();
  for (const row of rows.slice(state.page * PAGE_SIZE, (state.page + 1) * PAGE_SIZE)) {
    const tr = el("tr");
    table.columns.forEach(column => {
      const value = cell(row, column), td = el("td", number(value) ? "numeric" : value == null ? "null" : "", formatted(value, column.raw, table.sql));
      td.title = value == null ? "Missing reading" : ["device_id", "station_id"].includes(column.raw) ? formatted(value, column.raw, table.sql) : text(value); tr.append(td);
    }); body.append(tr);
  }
  if (!rows.length) { const tr = el("tr"), td = el("td", "empty", query ? "No rows match your search." : tool === "list_stations" ? `No ${table.name} returned.` : "No readings returned for this range."); td.colSpan = Math.max(1, table.columns.length); tr.append(td); body.append(tr); }
  byId("row-count").textContent = rows.length ? `${state.page * PAGE_SIZE + 1}–${Math.min((state.page + 1) * PAGE_SIZE, rows.length)} of ${rows.length.toLocaleString()} rows${query ? ` · ${table.rows.length} before filtering` : ""}` : "0 rows";
  byId("page-info").textContent = `${state.page + 1} / ${pages}`;
  byId("previous").disabled = state.page === 0; byId("next").disabled = state.page >= pages - 1;
}
function detailsPanel(data) {
  const entries = Object.entries(data).slice(0, MAX_COLUMNS).flatMap(([key, value]) => {
    if (Array.isArray(value) || key === "note") return [];
    if (object(value)) return Object.entries(value).slice(0, MAX_COLUMNS).filter(([, item]) => !object(item) && !Array.isArray(item)).map(([field, item]) => [`${key}.${field}`, field, item]);
    return [[key, key, value]];
  }).slice(0, 128);
  if (!entries.length) return null;
  const details = el("details", "panel details"); details.id = "result-details";
  details.open = state.tables.length === 0;
  details.append(el("summary", "", `Result details · ${entries.length} fields`));
  const list = el("dl");
  for (const [name, key, value] of entries) { list.append(el("dt", "", pretty(name)), el("dd", value == null ? "null" : "", `${formatted(value, key)}${value != null && unit(key) ? ` ${unit(key)}` : ""}`)); }
  details.append(list); return details;
}
function render() {
  if (!state.data) return;
  const data = rainSummaryDisplay(state.data, tool);
  const table = state.tables[state.table], rows = table?.sql ? [] : table?.rows ?? [];
  byId("subtitle").textContent = data.from || data.to ? `${timeLabel(data.from ?? "Archive start")} — ${timeLabel(data.to ?? "Latest reading")}` : data.station || data.name || (data.time ? timeLabel(data.time) : "Your tool result, ready to explore.");
  byId("eyebrow").textContent = tool === "query_sql" ? "YOUR ARCHIVE, YOUR QUESTIONS" : tool === "forecast" ? "A LOOK AT WHAT'S AHEAD" : "YOUR WEATHER, IN CONTEXT";
  content.replaceChildren(heroBlock(data, table, rows));
  const cards = metricCards(data, rows); if (cards.childElementCount) content.append(cards);
  const coverage = coverageNotice(table); if (coverage) content.append(coverage);
  if (tool === "rain_stats") { const rainCoverage = rainSummaryNotice(data); if (rainCoverage) content.append(rainCoverage); }
  if (table) { const chart = tool === "wind_rose" && table.name === "sectors" ? windPanel(data, table) : chartPanel(table); if (chart) content.append(chart); content.append(tablePanel()); renderRows(); }
  if (data.truncated === true) content.append(el("p", "note", "Result truncated by the server. Search and pagination cover returned rows only. Narrow the query or increase max_rows within the server limit to see more."));
  if (typeof data.note === "string" && data.note) content.append(el("p", "note", text(data.note)));
  const details = detailsPanel(data); if (details) content.append(details);
  content.setAttribute("aria-busy", String(state.busy));
}
function receiveResult(result, preservePrevious = false) {
  state.busy = false;
  try {
    if (result.isError) {
      const detail = result.content?.find(item => item.type === "text")?.text;
      throw new Error(detail ? text(detail, 500) : "The tool could not complete this request.");
    }
    let data = result.structuredContent;
    if (data == null) {
      const source = result.content?.find(item => item.type === "text")?.text;
      if (typeof source !== "string" || source.length > 2_000_000) throw new Error("No supported weather data was returned.");
      data = JSON.parse(source);
    }
    if (!object(data)) throw new Error("The tool result is not a supported data object.");
    const previous = state.data ? state.tables[state.table] : null;
    const tables = datasets(data, tool);
    const selected = previous ? tables.findIndex(table => table.name === previous.name) : -1;
    const compatible = compatibleTable(previous, tables[selected], state.data, data);
    const focus = compatible ? tableFocus() : null;
    // Read the current controls now, so changes made while a refresh was pending
    // survive. Ordered identities keep duplicate SQL column positions distinct.
    if (!compatible) resetExploration();
    state.data = data; state.tables = tables;
    state.table = selected >= 0 ? selected : tool === "forecast" ? Math.max(0, tables.findIndex(table => table.name === "hourly")) : 0;
    render(); status();
    restoreTableFocus(focus);
  } catch (error) {
    if (!preservePrevious) { content.replaceChildren(); state.data = null; state.tables = []; resetExploration(); }
    const detail = error instanceof SyntaxError ? "The tool returned invalid JSON. Run the tool again to retry." : text(error.message, 500);
    status(`${detail}${preservePrevious && state.data ? " The previous result is still shown." : ""}`, true);
  }
  updateRefresh();
}

// Install every handler before connect: hosts can deliver input/results as soon
// as the handshake completes. No tools are called until the user hits Refresh.
app.ontoolinput = ({ arguments: args }) => {
  state.generation++;
  state.args = object(args) ? structuredClone(args) : args === undefined && tool !== "query_sql" ? {} : null;
  state.busy = true; content.replaceChildren(); state.data = null; state.tables = []; resetExploration();
  status("Loading your weather data…"); updateRefresh();
};
app.ontoolresult = (result) => { state.generation++; receiveResult(result); };
app.ontoolcancelled = () => { state.generation++; state.busy = false; content.replaceChildren(); state.data = null; state.tables = []; resetExploration(); status("This tool request was cancelled. Refresh to try again."); updateRefresh(); };
app.onhostcontextchanged = hostContext;
app.onerror = () => { state.busy = false; status("The host connection encountered an error. Run the tool again if the view does not recover.", true); updateRefresh(); };
app.onclose = () => { state.generation++; state.connected = false; state.busy = false; byId("connection").textContent = "Host disconnected"; status("The host disconnected. Run this tool again to reconnect.", true); updateRefresh(); };
app.onteardown = async () => { state.generation++; state.connected = false; updateRefresh(); return {}; };
let resizeFrame = 0;
window.addEventListener("resize", () => {
  cancelAnimationFrame(resizeFrame);
  resizeFrame = requestAnimationFrame(() => {
    const table = state.tables[state.table], old = byId("chart-panel");
    if (!old || !table || tool === "wind_rose") return;
    const slider = old.querySelector("#chart-reading");
    const focused = slider && document.activeElement === slider;
    const replacement = chartPanel(table, Number(slider?.value ?? 0));
    if (replacement) {
      old.replaceWith(replacement);
      if (focused) replacement.querySelector("#chart-reading")?.focus({ preventScroll: true });
    }
  });
});
refresh.addEventListener("click", async () => {
  if (refresh.disabled) return;
  const generation = ++state.generation;
  const focused = document.activeElement === refresh;
  state.busy = true; status("Refreshing your tool result…"); updateRefresh();
  try {
    const result = await app.callServerTool({ name: tool, arguments: structuredClone(state.args) }, { timeout: 30000 });
    if (generation === state.generation) receiveResult(result, true);
  } catch {
    if (generation !== state.generation) return;
    state.busy = false; status("Refresh failed. The previous result is still shown; try again when the host is ready.", true); updateRefresh();
  } finally {
    if (focused && generation === state.generation && !refresh.disabled && document.hasFocus() &&
        (document.activeElement === document.body || document.activeElement === refresh)) refresh.focus({ preventScroll: true });
  }
});
byId("title").textContent = validTool ? TITLES[tool] : "Weather explorer";
if (!validTool) {
  status("Open this view from a TempestKeep tool in an MCP Apps-capable host.", true);
} else {
  app.connect(undefined, { timeout: 15000 }).then(() => {
    state.connected = true; hostContext(app.getHostContext());
    byId("connection").textContent = "Connected to your host"; updateRefresh();
    if (!state.data && !state.busy) status("Ready. Waiting for your tool result…");
  }).catch(() => {
    state.busy = false; byId("connection").textContent = "Host unavailable";
    status("Connect through an MCP Apps-capable host, then run this TempestKeep tool again.", true); updateRefresh();
  });
}
