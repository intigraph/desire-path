# Sanitized reason counts

Source fixtures, base `ca82165` versus candidate `ffbd121` plus test logging.
Three invalid tool names and three unsupported wrappers previously produced
identical totals. The candidate distinguishes their fixed reason keys, preserves
the normalized family, and emits no raw fixture inputs.

[Recorded output](output.log) and [timing](timing.log) are retained as a small
repository fixture demonstration. Replay after downloading both files:
`scriptreplay --log-out output.log --log-timing timing.log`. This is terminal
playback, not an inline video. Only the private launcher path in recorder metadata
was redacted; recorded command output is unchanged. The base portion displays
[retained actual before output](before.txt), not a second live base invocation.

Repeat the candidate probe: `go test ./internal/analyze -run TestFamilyReasonAccounting -count=1 -v`.
This demonstrates candidate source, not a release, installed producer or live data.
