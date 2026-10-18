import { number, formatted, label, unit, dateLabel, timeLabel } from "./data.js";

const NS = "http://www.w3.org/2000/svg";
export function svg(tag, attributes = {}, value) {
  const node = document.createElementNS(NS, tag);
  for (const [key, val] of Object.entries(attributes)) node.setAttribute(key, String(val));
  if (value !== undefined) node.textContent = String(value);
  return node;
}
export function seriesChart(rows, key, bucketMinutes = 0, selectedIndex = 0) {
  const viewport = document.documentElement.clientWidth;
  const mobile = viewport <= 480;
  const width = Math.max(240, Math.min(1000, viewport - (mobile ? 38 : viewport <= 740 ? 62 : 114)));
  const height = mobile ? 180 : 220;
  const chart = svg("svg", { viewBox: `0 0 ${width} ${height}`, role: "img", "aria-label": `${label(key)} chart; select a reading with the slider below`, class: "chart", preserveAspectRatio: "none" });
  const values = rows.map(row => row[key]).filter(number);
  if (!values.length) return chart;
  let low = Math.min(...values), high = Math.max(...values);
  const bars = /rain|precip|strikes|degree_days|_days$|coverage/.test(key);
  if (bars) low = Math.min(0, low);
  const span = high - low || Math.abs(high) * .1 || 1;
  low -= bars ? 0 : span * .18;
  high += span * .18;
  const left = mobile ? 38 : 48, right = width - 16, top = 20, bottom = height - 43;
  const y = (value) => bottom - (value - low) / (high - low) * (bottom - top);
  const times = rows.map(row => number(row.epoch) ? row.epoch * 1000 : typeof (row.time ?? row.day ?? row.date ?? row.period) === "string" ? Date.parse(row.time ?? row.day ?? row.date ?? row.period) : NaN);
  const realTime = times.every(number) && times.at(-1) > times[0] && times.every((value, i) => !i || value >= times[i - 1]);
  const x = (i) => rows.length === 1 ? (left + right) / 2 : left + (realTime ? (times[i] - times[0]) / (times.at(-1) - times[0]) : i / (rows.length - 1)) * (right - left);
  for (let i = 0; i <= 3; i++) {
    const value = low + (high - low) * i / 3;
    chart.append(svg("line", { x1: left, x2: right, y1: y(value), y2: y(value), class: "chart-grid" }));
    chart.append(svg("text", { x: left - 12, y: y(value) + 3, "text-anchor": "end" }, formatted(value, key)));
  }
  // Up to 2,000 points are returned by the server. Null samples break paths.
  // Fills use separate segments so absent readings never look like zero.
  const segments = [];
  let segment = [];
  rows.forEach((row, i) => {
    // Archive series omit completely empty buckets. A known bucket width lets
    // us break at those outages without inventing readings or calendar spacing.
    if (i && bucketMinutes > 0 && number(times[i]) && number(times[i - 1]) && times[i] - times[i - 1] > bucketMinutes * 60000) {
      if (segment.length) segments.push(segment);
      segment = [];
    }
    if (!number(row[key])) {
      if (segment.length) segments.push(segment);
      segment = [];
    } else segment.push([x(i), y(row[key]), i]);
  });
  if (segment.length) segments.push(segment);
  if (bars) {
    const width = Math.max(.5, Math.min(24, (right - left) / Math.max(rows.length, 1) * .65));
    rows.forEach((row, i) => {
      if (!number(row[key])) return;
      const bar = svg("rect", { x: x(i) - width / 2, y: Math.min(y(row[key]), y(0)), width, height: Math.max(.5, Math.abs(y(row[key]) - y(0))), rx: 2, class: "chart-bar" });
      bar.append(svg("title", {}, `${row.time ? timeLabel(row.time) : dateLabel(row)}: ${formatted(row[key], key)} ${unit(key)}`));
      chart.append(bar);
    });
  } else {
    const defs = svg("defs");
    const gradient = svg("linearGradient", { id: "chart-fill", x1: 0, y1: 0, x2: 0, y2: 1 });
    gradient.append(svg("stop", { offset: "0%", "stop-color": "var(--accent)", "stop-opacity": ".15" }), svg("stop", { offset: "100%", "stop-color": "var(--accent)", "stop-opacity": ".015" }));
    defs.append(gradient); chart.append(defs);
    for (const points of segments) {
      const line = points.map(([px, py], index) => `${index ? "L" : "M"}${px.toFixed(2)},${py.toFixed(2)}`).join(" ");
      if (points.length > 1) chart.append(svg("path", { d: `${line} L${points.at(-1)[0]},${bottom} L${points[0][0]},${bottom}Z`, fill: "url(#chart-fill)" }));
      chart.append(svg("path", { d: line, class: "chart-line" }));
      if (rows.length <= 60 || points.length === 1) {
        for (const [px, py, i] of points) {
          const point = svg("circle", { cx: px, cy: py, r: 2.5, class: "chart-dot" });
          point.append(svg("title", {}, `${rows[i].time ? timeLabel(rows[i].time) : dateLabel(rows[i])}: ${formatted(rows[i][key], key)} ${unit(key)}`));
          chart.append(point);
        }
      }
    }
  }
  const tickCount = Math.min(mobile ? 3 : 6, rows.length);
  const ticks = [...new Set(Array.from({ length: tickCount }, (_, i) => Math.round(i * (rows.length - 1) / Math.max(1, tickCount - 1))))];
  const spansDates = typeof rows[0]?.time === "string" && rows[0].time.slice(0, 10) !== rows.at(-1)?.time?.slice(0, 10);
  ticks.forEach((i, n) => chart.append(svg("text", { x: x(i), y: height - 13, "text-anchor": n === 0 ? "start" : n === ticks.length - 1 ? "end" : "middle" }, spansDates && typeof rows[i].time === "string" ? `${rows[i].time.slice(5, 10)} ${dateLabel(rows[i])}` : dateLabel(rows[i]))));
  const view = document.createElement("div");
  view.className = "series-view";
  const inspector = document.createElement("div");
  inspector.className = "chart-inspector";
  const control = document.createElement("label");
  control.htmlFor = "chart-reading";
  control.textContent = "Inspect a reading";
  const position = document.createElement("span");
  position.className = "reading-position";
  control.append(position);
  const range = document.createElement("input");
  range.type = "range"; range.id = "chart-reading";
  range.min = "0"; range.max = String(rows.length - 1); range.step = "1";
  range.setAttribute("aria-describedby", "chart-reading-help");
  const reading = document.createElement("div");
  reading.className = "chart-reading";
  const timestamp = document.createElement("div"), value = document.createElement("div");
  timestamp.className = "reading-time"; value.className = "reading-value";
  reading.append(timestamp, value);
  const help = document.createElement("p");
  help.id = "chart-reading-help";
  help.textContent = "Point or tap on the chart, or use the slider. Arrow keys move one reading; Home and End jump to the first and last.";
  inspector.append(control, range, reading, help);

  const cursor = svg("line", { y1: top, y2: bottom, class: "chart-cursor" });
  const marker = svg("circle", { r: 4.5, class: "chart-selection" });
  const target = svg("rect", { x: left, y: top, width: right - left, height: bottom - top, fill: "transparent", class: "chart-hit" });
  chart.append(cursor, marker, target);
  const positions = rows.map((_, index) => x(index));
  const metric = `${label(key)}${unit(key) ? ` (${unit(key)})` : ""}`;
  const select = index => {
    range.value = String(index);
    const row = rows[index];
    position.textContent = `${index + 1} of ${rows.length.toLocaleString("en-US")}`;
    timestamp.textContent = row.time ? timeLabel(row.time) : dateLabel(row);
    value.textContent = `${metric}: ${number(row[key]) ? formatted(row[key], key) : "Missing reading"}`;
    range.setAttribute("aria-valuetext", `Reading ${position.textContent}. ${timestamp.textContent}. ${value.textContent}`);
    cursor.setAttribute("x1", positions[index]); cursor.setAttribute("x2", positions[index]);
    marker.setAttribute("cx", positions[index]);
    marker.setAttribute("visibility", number(row[key]) ? "visible" : "hidden");
    if (number(row[key])) marker.setAttribute("cy", y(row[key]));
  };
  range.addEventListener("input", () => select(Number(range.value)));
  const point = event => {
    const bounds = chart.getBoundingClientRect();
    const px = (event.clientX - bounds.left) * width / bounds.width;
    // Use plotted coordinates, including real time gaps, rather than treating
    // unequal intervals as evenly spaced or interpolating a weather reading.
    let nearest = 0;
    positions.forEach((position, index) => {
      if (Math.abs(position - px) < Math.abs(positions[nearest] - px)) nearest = index;
    });
    select(nearest);
  };
  target.addEventListener("pointerdown", point);
  target.addEventListener("pointermove", event => {
    if (event.pointerType === "mouse" || event.buttons > 0) point(event);
  });
  select(Math.max(0, Math.min(selectedIndex, rows.length - 1)));
  view.append(chart, inspector);
  return view;
}

export function windChart(sectors) {
  const chart = svg("svg", { viewBox: "0 0 320 280", role: "img", "aria-label": "Wind direction frequency; percentages are in the table below", class: "chart wind-chart" });
  const cx = 160, cy = 139, radius = 99;
  const maximum = Math.max(...sectors.map(row => number(row.pct) ? row.pct : 0), 1);
  for (let ring = 1; ring <= 4; ring++) chart.append(svg("circle", { cx, cy, r: radius * ring / 4, fill: "none", class: "chart-grid" }));
  const directions = ["N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"];
  const polar = (angle, r) => [cx + Math.sin(angle) * r, cy - Math.cos(angle) * r];
  directions.forEach((direction, i) => {
    const angle = i * Math.PI / 8;
    const row = sectors.find(sector => sector.sector === direction);
    if (row && number(row.pct) && row.pct > 0) {
      const r = Math.sqrt(row.pct / maximum) * radius;
      const a = polar(angle - Math.PI / 19, r), b = polar(angle + Math.PI / 19, r);
      const wedge = svg("path", { d: `M${cx},${cy} L${a} A${r},${r} 0 0 1 ${b}Z`, fill: "var(--accent)", opacity: .55 + .4 * row.pct / maximum });
      wedge.append(svg("title", {}, `${direction}: ${formatted(row.pct)}%`)); chart.append(wedge);
    }
    if (i % 2 === 0) {
      const [x, y] = polar(angle, 121);
      chart.append(svg("text", { x, y: y + 3, "text-anchor": "middle" }, direction));
    }
  });
  chart.append(svg("circle", { cx, cy, r: 7, fill: "var(--surface)" }));
  return chart;
}
