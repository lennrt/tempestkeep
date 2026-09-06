# Reproduce the MCP demo

The README leads with [agent.gif](agent.gif). The matching [MP4 clip](agent.mp4)
is easier to pause and share. Both are recorded by Charm's
[VHS](https://github.com/charmbracelet/vhs) from [agent.tape](agent.tape).

## What the recording demonstrates

The Go client in [internal/demo/agentdemo](../internal/demo/agentdemo/main.go)
starts the real TempestKeep binary and uses the official MCP Go SDK over stdio.
Questions and narration are scripted. Counts, tool results, and summaries come
from actual protocol exchanges against synthetic weather data. There is no LLM
in the recording and no claim about a particular model's tool selection.

1. Discover the server, 29 tools, 2 resources, and 3 prompts.
2. Build 45 days of synthetic history through repeated bounded backfill calls.
3. Select the strongest gust in the last 30 days, then inspect that day's hourly
   observations. The comparison uses hourly means, not instantaneous maxima.
4. Close the session and reconnect without an API token using --read-only.
   Confirm live and write tools are absent, then query the same local archive.

The model generates repeatable daily patterns relative to the recording date.
Exact dates and counts may vary with recording time. Wind-sector percentages
refer to non-calm samples; the calm percentage refers to all wind observations.

## Run and verify

With Go 1.27.0 installed:

```sh
make mcp-demo       # paced terminal demonstration
make demo-smoke     # the same real MCP session, no presentation delays
```

[scripts/demo-agent.sh](../scripts/demo-agent.sh) creates a private temporary
directory, starts the synthetic API on an available loopback port, waits for its
readiness file, and runs the client. It overrides inherited weather credentials
and endpoint settings. It uses UTC for matching calendar boundaries and avoids
loading an existing .env or archive. Cleanup stops the API and removes the
synthetic archive and temporary files, including after a failed run.

The smoke check also supplies conflicting ambient settings to verify isolation.
It checks discovery, resumption, tool chaining, the second session's restricted
capabilities, and successful cleanup.

## Record the clip

Install VHS v0.11.0, ttyd, and ffmpeg. VHS also needs a Chromium browser; it can
locate or download one. Then run:

```sh
make demo-agent
```

If VHS is outside PATH, set its location with make demo-agent VHS=/path/to/vhs.
The tape writes docs/agent.gif and docs/agent.mp4. It waits for the successful
cleanup marker; short pauses between scenes give readers time to read results.
Review all four scenes before committing updated media. Keep headings, prompts,
arguments, results, and the synthetic-data label fully visible.

The manual Demo qualification workflow runs the smoke check, records the tapes,
and retains the GIFs and MP4 as workflow artifacts. It does not publish media
elsewhere. The older setup, dashboard, and explorer recordings remain separate
examples of the CLI.
