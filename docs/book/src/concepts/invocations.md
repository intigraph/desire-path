# Invocations

**Invocations** track ALL tool calls, not just failures. When enabled, desire_path becomes a comprehensive telemetry system for AI tool usage.

## Signposting sibling mode

Signposting preserves a deterministic search exactly. A PreToolUse sibling may
start a detached semantic prefetch, while PostToolUse offers its pointer only
when the literal result is empty or too large and a warm result is available
inside the production budget. A signpost is a runnable command, not a
replacement result set.
Following that command is adoption. Hook timeouts and failures are dropped, so
the unmodified search remains the worst-case behavior.

V1 covers Bash-mediated `grep` and `rg`; native agent Grep/Glob tools are not
intercepted. The evaluation and event contract lives in
`docs/plans/009-signposting-eval.md`.

## The Difference

- **Desires**: Only failed tool calls
- **Invocations**: Every tool call (success or failure)

Invocations give you the full picture: success rates, usage patterns, which tools get hammered, which never get touched, and how sessions unfold over time.

## Enabling Invocations

Turn on full tracking when initializing:

```bash
dp init --source claude-code --track-all
```

The `--track-all` flag activates invocation recording. Now every tool call flows into your desire_path database, not just the failures.

## What Gets Captured

Each invocation records:

- **source**: Which AI system made the call (e.g., "claude-code")
- **instance_id**: Specific AI session or conversation
- **host_id**: Machine where the call happened
- **tool_name**: The tool that was called
- **is_error**: Boolean — did it succeed or fail?
- **error**: Error message if `is_error` is true (null otherwise)
- **cwd**: Working directory during the call
- **timestamp**: When it happened
- **metadata**: Additional context as JSON (optional)

### Bash command and output evidence

New Bash invocations include `command_capture`, `command_shape`, and ordered
`command_argv0` metadata. Identities omit arguments and paths. For example,
`cd project && sd show item || true` records `cd/sd:show/true`; this describes
the submitted program, not which branches executed or which command failed.
Dynamic commands, unsupported execution wrappers, and oversized inputs record
`command_capture=unknown`. Older rows have no command attribution; they are not
backfilled or counted as successful captures.

Line-anchored `Error:`, `Fatal:`, `REFUSED`, and HTTP 4xx/5xx output signatures
also produce error invocations and desires when the caller masks its exit.
`failure_evidence=output-signature` distinguishes these from `native-error`.
An expected negative control can print an error, so signature counts require
interpretation; they are not independently verified subprocess failures.
`output_scan=unknown` discloses absent, unsupported, or oversized responses.
The original response remains available in the existing metadata.

This SQLite query counts weekly **captured invocations containing a tool**,
deduplicating repeated mentions within a program. It separates native errors
from output signatures rather than assigning a chain's failure to `cd`:

```sql
WITH programs AS (
  SELECT DISTINCT i.id, strftime('%Y-%W', i.timestamp) AS week,
         c.value AS command, i.is_error,
         json_extract(i.metadata, '$.failure_evidence') AS evidence
  FROM invocations AS i, json_each(i.metadata, '$.command_argv0') AS c
  WHERE i.timestamp >= '2026-10-01' AND i.timestamp < '2026-11-01'
    AND c.value IN ('bd', 'br', 'sd', 'gh')
)
SELECT week, command, count(*) AS captured_invocations,
       sum(is_error = 1 AND evidence = 'native-error') AS native_errors,
       sum(is_error = 1 AND evidence = 'output-signature') AS output_signatures,
       round(100.0 * sum(is_error) / count(*), 2) AS flagged_percent
FROM programs GROUP BY week, command ORDER BY week, command;
```

Always report capture coverage over the same date interval too:

```sql
SELECT coalesce(json_extract(metadata, '$.command_capture'), 'historical-unknown')
         AS capture, count(*) AS invocations
FROM invocations
WHERE tool_name IN ('Bash', 'exec_command', 'functions.exec_command')
  AND timestamp >= '2026-10-01' AND timestamp < '2026-11-01'
GROUP BY capture;
```

Dates are explicit example bounds; choose the intended reporting interval.
Use an existing database through a read-only SQLite connection. Masked exits
cannot be recovered when the caller suppresses both its status and output;
preserve command exits in scripts and include errors in tool responses.

## Viewing Invocation Stats

Get aggregated statistics:

```bash
dp stats --invocations
```

This might show:

```
TOOL NAME       TOTAL   SUCCESS   FAILED   SUCCESS RATE
Read            324     320       4        98.8%
Bash            156     142       14       91.0%
Glob            89      89        0        100.0%
Edit            67      63        4        94.0%
read_file       47      0         47       0.0%
```

Insights immediately visible:
- `Read` works reliably (98.8% success)
- `Bash` has issues (14 failures worth investigating)
- `read_file` fails every time (needs aliasing or building)

## Exporting Invocation Data

Pull raw data for deeper analysis:

```bash
dp export --type invocations --format json > invocations.json
```

Now you can:
- Load into analytics tools
- Build custom dashboards
- Track trends over time
- Correlate with other metrics

## The Source Plugin System

Invocations use desire_path's plugin architecture. Each AI tool has its own parser:

```bash
dp init --source claude-code --track-all   # Claude Code invocations
dp init --source aider --track-all          # Aider invocations
dp init --source custom-ai --track-all      # Your custom AI tool
```

The `source` plugin handles:
- Parsing that AI tool's specific output format
- Extracting tool call data
- Recording to the desire_path database

Different AI tools structure their telemetry differently. Plugins normalize everything into a common schema.

## Use Cases

### Success Rate Monitoring

Which tools are fragile? If `Bash` fails 10% of the time, maybe error handling needs work.

### Usage Patterns

Which tools actually get used? You might discover Claude Code calls `Read` 10x more than anything else — worth optimizing.

### Session Timelines

Replay how a conversation unfolded: "First it read the file, then globbed for tests, then ran bash commands." Debug AI reasoning by seeing the sequence.

### Hallucination Detection

Tools with 0% success rate are hallucinations. But you already knew that from desires. What's new: tools with 50% success rate might have naming collisions or API confusion.

## When To Enable Invocations

### Don't Enable If...

- You only care about failures (desires are enough)
- Storage or performance is constrained (invocations generate more data)
- You're just getting started (start simple, expand later)

### Do Enable If...

- You're building production AI tooling (comprehensive telemetry matters)
- You want to optimize tool implementations (need success data to measure improvements)
- You're analyzing AI behavior patterns (full session data reveals reasoning flows)
- You're running experiments (A/B testing tool changes requires success metrics)

## The Full Picture

Desires tell you what's broken. Invocations tell you what's working, how often, and why.

Together, they turn desire_path from a failure tracker into an AI observability platform. You're not just fixing problems — you're understanding how AI assistants interact with your system at every level.

More data. Richer insights. Better tools.
