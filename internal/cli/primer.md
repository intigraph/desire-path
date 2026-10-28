# Desire Path agent primer

## What dp is

Desire Path (`dp`) records failed tool calls and reveals repeated mistakes.
It turns that evidence into searchable patterns, aliases and signposts.
In the Quipu stack, Quipu remembers knowledge, Bobbin searches code,
Yupana explains structure, and dp captures capabilities agents expected.

## Agent workflow

1. Query Quipu for prior decisions and incidents before acting.
2. Use Bobbin for code search and Yupana for impact.
3. Inspect dp's recorded evidence before proposing an alias or correction.
4. Make recording, initialization and alias changes explicitly; `dp prime` only reads.

## Core commands

| Command | Use |
| --- | --- |
| `dp prime` | Full orientation followed by bounded read-only state |
| `dp prime --brief` | Short orientation for stack aggregation |
| `dp prime --json` | Structured `stack-prime/1` output for agents |
| `dp prime --static` | Embedded guidance without config or state reads |
| `dp prime --section core --static` | One section by case-insensitive heading prefix |
| `dp list --since 7d` | Recent recorded failures |
| `dp paths --top 10` | Frequently requested tool names |
| `dp inspect TOOL` | Evidence for a recorded pattern |
| `dp stats --invocations` | Captured invocation statistics |
| `dp similar TOOL` | Similar known tool names |
| `dp ingest --source claude-code` | Explicitly record a supplied payload |
| `dp alias FROM TO` | Explicitly create a tool-name correction |
| `dp init --source claude-code` | Explicitly install recording integration |

## MCP tools

None. The CLI and hooks are the agent surfaces. Hook installation is separate
from orientation.

## Reading live state

The live read has a 650 ms deadline. Local mode reports only the configured
existing database file's size and modification time; it never opens SQLite,
creates sidecars, or migrates a schema. File metadata does not establish valid
schema, row counts, capture coverage, or database health. Remote mode makes one
GET to the configured server's `/api/v1/stats`, with no retries or redirects and
a 64 KiB response cap. It reports only total desires and unique paths.

Status is `ok`, `partial` at the deadline, `absent`, `unreachable`, `unreadable`,
or `skipped` with `--static`. JSON includes elapsed milliseconds and reports
`degraded` for an incomplete live read; guidance remains usable and exit is 0.
Plain live state names the path or credential-free URL and a next command.
`--db` overrides the configured local path. Usage errors exit 2.

## Next steps

- [Book](https://scbrown.github.io/desire-path/)
- [README](https://github.com/scbrown/desire-path#readme)
- [Skill](https://github.com/scbrown/desire-path/blob/main/skills/desire-path/SKILL.md)
- Command help: `dp help` and `dp prime --help` (no shipped man page).
