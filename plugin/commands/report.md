---
description: "Full weather report: now, the week ahead, and today in your station's history"
argument-hint: ""
---

Produce a weather report from the user's station with the `tempestkeep` MCP tools.
Gather the data before writing. Do not stream raw tool results.

1. `current_conditions`: the now.
2. `forecast`: today plus the next few days.
3. `this_day_in_history`: this calendar day across every archived year.
4. `records`: to flag anything today is close to.

Format the report exactly like this:

```
## ⛅ <station name> · <weekday, local time>

**Now:** <temp, feels-like, conditions, wind, humidity; one sentence.>

**Next 3 days:** <one line per day: weekday, hi/lo, conditions, rain chance.>

**This day in history:** <2–3 sentences: typical temps for this date across the
archive, the record high/low for the date and which year, anything unusual.>

**Watch for:** <only if warranted: a record within ~3°F, gusts near the all-time
peak, first rain in N days. Omit the section entirely when nothing stands out.>
```

Use the units the tools return (°F, mph, in, inHg). Round temperatures to
whole degrees. If the archive tools are unavailable, produce the live-only sections
and note in one line that history needs an archive (`/tempestkeep:setup` builds one).
