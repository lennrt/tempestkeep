# Interactive MCP Apps

TempestKeep displays weather and archive results inside MCP clients that support
the [MCP Apps extension][apps]. The client is the host: it renders the app and
routes its read-only requests to the existing TempestKeep server.

The app ships inside the Go executable. It needs no web server, Node.js runtime,
or separate TUI. Clients without MCP Apps support still receive normal text and
structured tool results.

## Connect and explore

Use the [MCP connection instructions](../README.md#connect-your-agent) to configure
`tempestkeep mcp`. Configure a token for live weather or an existing archive for
local queries. The same capability rules apply to the app and ordinary tools.

1. Reconnect the client after installing the updated binary.
2. Ask for a forecast, daily summary, or another available read-only result.
3. Open the app view that the host associates with that tool call.

The host decides where the view appears. Some clients require you to expand
the tool result. Host support and permission controls vary by version. Use the
[official MCP Apps documentation][apps] to find current host support.
This repository does not certify a particular host or version.

| Request | What to explore |
|---|---|
| "Show the forecast." | Supported weather series and their returned readings. |
| "Show daily weather for the past month." | Trends and a table of daily summaries. |
| "Show the windiest days in my archive." | Ranked rows from a summary or read-only SQL result. |
| "Show the archive status." | Coverage and the available structured details. |
| "List my stations and devices." | Stations and a Devices table with parent station names and exact IDs. |

All 27 read-only tools have views when live and archive sources are available.
Only tools supported by the current configuration appear. `backfill_archive`
and `sync_archive` retain their ordinary MCP results and have no app controls.

## Read charts and tables

Charts show supported weather series with a metric selector. Point or tap on a
chart to inspect the nearest returned reading, including its full source time,
timezone when supplied, metric, and units. The slider selects every displayed
point, including dense series of up to 2,000 readings. Focus the slider and use
arrow keys for adjacent readings, or Home and End for the first and last.
Missing readings are labeled explicitly. Selection follows the plotted spacing
and does not interpolate across gaps or fetch data. Changing the metric or
replacing the result resets selection to the first reading.
Wind results include a 16-sector rose with exact frequencies in the table.
Dataset tabs separate collections
such as hourly and daily forecasts. Tables provide text search, numeric-aware
column sorting, and eight-row pages. These controls operate on the received
result. They do not run a new archive query or retrieve rows beyond a server limit.

The `list_stations` view includes a Devices table with station ID, station name,
device ID, device type, and serial number. Search an exact ID or serial number to
find its parent station. Numeric station and device IDs use ungrouped digits so
they can be copied into configuration. Duplicate entries and missing fields stay
visible. The view uses only the received station result and leaves it unchanged.
If a numeric ID exceeds the browser's exact integer range, the cell reports
`ID exceeds browser precision` instead of presenting a rounded ID for copying.
Use `tempestkeep list-devices` to read that ID; text IDs remain unchanged.
It examines at most 2,000 stations and 2,000 device entries,
with a notice when either limit omits data. Its fixed columns also let table
controls survive a refresh that temporarily returns no devices.

The view displays at most 2,000 rows and 64 columns per table. It reports when
these display limits omit data. The original structured tool result remains
available to the host. Long cell text is also bounded in the display.

If a result is truncated, narrow its dates, reduce its selected columns, or use
an aggregate query. The truncation notice remains visible during table
interaction. A table page is not evidence that the archive is complete.

Missing readings remain distinct from measured zero. Labels and field units
come from the tool result. Raw SQL retains SI units unless the query explicitly
converts them. SQL column names and values remain in their original order, even
when names repeat. The app does not infer units from arbitrary SQL column names.
For observation series, `rain_readings` and `lightning_readings` count the samples
that supplied each event measurement. A zero count displays the corresponding
total as missing in charts, tables, and summaries. A positive count preserves a
measured zero. If a count is below `obs`, the total covers only some samples.
The view reports missing and partial buckets, and keeps their counts in the table.
These counts describe samples, not continuous sensor uptime. Original tool
results remain intact. Older results without counts retain their existing display.

The Rainfall patterns view for `rain_stats` keeps rainfall as its headline even
when no measurement is available. It shows missing rainfall and unavailable
rain statistics as `—`, while measured zero stays zero. The sample-count notice
distinguishes an empty range, absent rain readings, and a partial reported total.
Counts describe stored samples, not continuous coverage. Refresh updates the
total and notice together; original tool results and other views stay unchanged.
See the [query guide](querying.md) for date boundaries, aggregation, and incomplete
sensor coverage.

Refresh repeats the current read-only tool with the same arguments through the
host. Live tools retain the configured API cache behavior. Archive views read
the archive's current contents. Refresh does not sync or backfill history.
A compatible refresh keeps the selected dataset, search text, sort column and
direction, and page. If fewer filtered rows remain, the page moves to the last
available page. Edits made while the call is pending are retained, including
search focus and text selection. A failed refresh leaves the prior result and
controls available.

A new tool input resets table controls. A missing selected dataset or a changed
ordered column structure also resets them, so stale sorting cannot select a
different column. Empty results without explicit column metadata also reset
controls because their structure is unknown. This state lasts only in the current
app instance. Chart reading selection still resets when a result is replaced.
For a different range or query, ask the agent to call the tool with new arguments.

The view follows the host's theme and fits narrow containers. Loading, empty
results, and errors have explicit states. If the host denies refresh, the app
reports the unavailable action. It does not open another connection to the API.

## Privacy and resource boundaries

The host supplies tool inputs and results to the app. These data can include
station details, weather history, and query text. Configure trusted hosts and
review their retention and model-provider policies.

The frontend receives no WeatherFlow token and does not open SQLite files.
The existing Go tool handlers enforce read-only queries, limits, and deadlines.
The app renders external labels and cell values as text.

Scripts and styles are bundled into the HTML resource. Its content security
policy requests no external connection, resource, or frame origins. It requests
no camera, microphone, location, or other device permissions. The host supplies
the sandbox and controls tool-call permissions. See the
[threat model](threat-model.md) for the full boundary and residual risks.

## Discovery contract

An app-capable client advertises the `io.modelcontextprotocol/ui` extension
with `text/html;profile=mcp-app` in its supported MIME types. TempestKeep then
includes `_meta.ui.resourceUri` on each available read-only tool.

The URI is `ui://tempestkeep/<tool-name>.html`. `resources/read` returns HTML with
MIME type `text/html;profile=mcp-app`. Resources follow the data-source capability
rules and remain available for inspection by ordinary MCP clients. With every
source available, there are 27 UI resources and two archive documentation resources.

Clients that do not advertise the extension see their ordinary tool metadata.
The tool's input schema, annotations, text content, and structured result stay
the same in either mode. See the [service specification](../openspec/specs/mcp-service/spec.md)
and [ADR 0006](adr/0006-mcp-apps.md) for the proposed contract.

## Troubleshooting

| Symptom | Action |
|---|---|
| Only text appears | Make sure that the client version supports MCP Apps, then reconnect and expand the tool result. |
| An expected view is absent | Inspect tool discovery. Live-only tools need a token, and archive tools need an archive. |
| Refresh is unavailable | Use the host's tool-call controls or ask the agent to repeat the same read-only tool. |
| Archive data does not become newer | Run the normal sync or collection workflow, then refresh the view. |
| A table search finds no rows | Clear the search and inspect the range, returned row count, and truncation notice. |
| The view is empty or reports an error | Inspect the tool result and the server's redacted stderr diagnostics. |

Do not add an HTTP listener or expose your token to the browser to fix a host
compatibility problem. Ordinary MCP results remain available. See the
[MCP troubleshooting guide](troubleshooting.md) for connection and archive issues.

## Develop and test the app

Use Node.js 20.19.0 or later for frontend and specification development. The
lockfile pins the SDK, bundler, browser tooling, and OpenSpec dependencies.
From the repository root, install dependencies and the test browser:

```sh
npm ci --ignore-scripts --no-audit --no-fund
npx playwright install chromium
```

On a fresh Linux machine, Playwright can also need system browser dependencies.
Use `npx playwright install --with-deps chromium`, as the CI workflow does.
Building the ordinary Go executable uses the committed HTML and does not run npm.

```sh
npm run app:build
npm run app:check
npm run app:test
npm run spec:check
make docs-check
```

The build creates the self-contained HTML embedded by Go. Commit source changes
with the rebuilt bundle. The generated-asset check detects stale output.
The bundle also retains third-party license and notice text. See
[NOTICE.md](../NOTICE.md) for the source and generation process.
Browser tests use a local test host with the official MCP Apps bridge and
synthetic fixtures. They do not need a WeatherFlow token or a model provider.

## Reproduce the screenshots

After installing the development dependencies and Chromium, run:

```sh
npm run app:build
npm run app:screenshots
```

Playwright captures the embedded app in the local synthetic test host. The
generated files are [the weather preview](screenshots/mcp-app-weather.png) and
[the table preview](screenshots/mcp-app-table.png). Inspect both images before
committing them with a UI change.

These screenshots show fixture data and the app's layout. They are not captures
of a production station or evidence of compatibility with a named third-party
host. Keep credentials, personal paths, station identifiers, and real weather
archives out of fixtures and screenshots.

[apps]: https://modelcontextprotocol.io/extensions/apps/overview
