# ADR 0018: Preserve collection device and select newest live readings

Status: proposed

Collection without a flag or environment device selector previously discovered
the first live device on every run. Station reordering could break a scheduled
run against an archive already bound to another device.

Use existing archive identity before discovery. Positive explicit flags override
environment settings. A nonempty environment selector must also be positive.
Reject explicit nonpositive flags before loading configuration. A missing archive
or an empty unbound archive still discovers. Read-only archive inspection must
fail closed on invalid data. Legacy rows with device IDs retain that identity;
legacy archives without a usable identity require an explicit selector. Writer
binding remains the final ownership check, including concurrent changes.

Read persisted identity through the shared bounded metadata reader. This keeps
selection consistent with writer binding in UTF-16 archives and for legacy
metadata tables whose names differ only in letter case.

Live latest observations use the largest validated timestamp, independent of
response order. The first row wins a timestamp tie. Validate every row before
returning a result, so an older malformed row still rejects the response.

No schema, public signature, credential output, or runtime dependency changes.
Supplying zero explicitly now fails instead of requesting discovery; omit the
selector to use archive identity or discovery.
