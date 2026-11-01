# Design

Keep dataset reset semantics and restore focus only after user dataset selection,
using existing table-control identities. Do not steal focus on initial results.

Keep 2,048-character table previews. SQL string search and an explicit native
modal inspector use at most 65,536 characters per cell. Longer cells disclose
this bound in the table and inspector. Values remain text nodes. The dialog
supports keyboard scrolling, Escape, a close button, and focus restoration.
Existing table row/column limits apply before this work. The original payload
remains available to the host without mutation.

Daily forecast cards use available daily high/low temperatures and maximum
rain probability, with missing values distinct from zero. Hourly summaries
retain hourly wording. All operations remain local to returned rows.

Use synthetic browser regressions for keyboard focus, daily/hourly switching,
null and zero values, suffix search, hostile content, limits, and narrow layout.
