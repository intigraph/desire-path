---
name: desire-path
description: Orient agents and inspect failed tool-call evidence with dp.
---

<!-- BEGIN dp prime -->
# Desire Path agent primer

Desire Path (`dp`) records failed tool calls and turns repeated mistakes into
searchable patterns, aliases and signposts. In the Quipu stack, Quipu remembers
operational knowledge, Bobbin searches code, Yupana explains structure, and dp
captures the capabilities agents expected to find.

## Workflow

Query Quipu for prior decisions and incidents before acting. Use Bobbin for code
search and Yupana for impact. Inspect dp's evidence before proposing an alias;
a failed call is a signal, not authority to change infrastructure. Recording,
initialization and alias updates are explicit writes. `dp prime` only reads.

## Core commands

- `dp prime --brief`: short orientation and a bounded live summary.
- `dp prime --json`: primer plus structured state for agents.
- `dp prime --static`: orientation without reading configuration or live state.
- `dp list --since 7d`: recent recorded failures.
- `dp paths --top 10`: frequently requested tool names.
- `dp inspect TOOL`: evidence for one recorded pattern.
- `dp stats --invocations`: captured invocation statistics.
- `dp similar TOOL`: similar known tool names.
- `dp ingest --source claude-code`: record a supplied tool-call payload.
- `dp alias FROM TO`: explicitly create a tool-name correction.
- `dp init --source claude-code`: explicitly install recording integration.

The binary does not provide an MCP server; its CLI and hooks are the agent
surfaces. Hook installation is separate from orientation.

## Live summary

The live read has a 650 ms deadline. Local mode reports only the configured
existing database file's size and modification time; it never opens SQLite,
creates sidecars, or migrates a schema. File metadata does not establish valid
schema, row counts, capture coverage, or database health. Remote mode makes one
GET to the configured server's `/api/v1/stats`, with no retries or redirects and
a 64 KiB response cap. It reports only total desires and unique paths, not raw
calls or tool inputs. Unreachable, unreadable and skipped states are explicit;
the static primer remains usable. `--db` overrides the configured local path.

## Further reading

- Book: https://scbrown.github.io/desire-path/
- README: https://github.com/scbrown/desire-path#readme
- Skill: https://github.com/scbrown/desire-path/blob/main/skills/desire-path/SKILL.md
- Command help: `dp help` and `dp prime --help` (no shipped man page).
<!-- END dp prime -->
