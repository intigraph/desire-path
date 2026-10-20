# desire-path

**Desire Path (`dp`) is a CLI and a set of agent hooks for developers who want
fewer repeated tool-call failures. It stores calls locally, ranks recurring
failures, and records mappings from mistaken tool names to real ones.**
The name comes from the trails people wear through grass: evidence of where a path is needed.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/scbrown/desire-path/main/install.sh | sh
```

Python 3.9+, curl, and a Linux or macOS shell. Downloads the latest published archive, verifies SHA-256 and the executable
version before publishing command links in `~/.local/bin`. Add it to `PATH`.
The complete versioned bundle stays in `~/.local/lib/desire-path/TAG` for rollback.
Published targets: Linux and macOS, x86_64 and arm64.

Set `STACK_INSTALL_ROOT` on the receiving shell for another prefix.
Pinned releases, alternatives, configuration and removal are in the
[installation and usage reference](docs/installation-reference.md).

## Try it

Use a temporary database and one synthetic failed call; Python 3 formats the result:

```bash
DP_DEMO=$(mktemp -d)
printf '%s\n' '{"tool_name":"read_file","error":"unknown tool","session_id":"demo"}' | dp --db "$DP_DEMO/desires.db" ingest --source claude-code --json > "$DP_DEMO/record.json"
dp --db "$DP_DEMO/desires.db" paths --json | python3 -c 'import json,sys; p=json.load(sys.stdin)[0]; print(p["pattern"], p["count"])'
```

Expected stdout:

```text
read_file 1
```

The failure became a queryable pattern without configuring an agent or touching your usual database.
An optional-metrics diagnostic may also appear on stderr; see [troubleshooting](https://scbrown.github.io/desire-path/getting-started.html#troubleshooting).

## Next steps

- [Tambo](https://github.com/scbrown/tambo): guided stack installation and verification.
- [Camayoc](https://github.com/scbrown/camayoc): governed memory and vocabulary.
- [Detailed examples and agent setup](docs/installation-reference.md).

**Load this repository’s graph:** public repository-pendant assets have not yet been verified. When a release supplies the archive, checksum and provenance, follow the [verified import workflow](https://scbrown.github.io/quipu/sharing/index.html): extract the share directory and use `quipu import SHARE_DIR --db repository.db`. Import stages knowledge; ROOT stays unchanged.

## The stack

<!-- stack-table:start -->

The Quipu stack is built around a governed, bitemporal knowledge store: shared
facts, decisions and work history your agents can query. Quipu can check writes
against your rules, records where each fact came from, and answers what was true
when.

| tool | what it gives your agents |
|---|---|
| [tambo](https://github.com/scbrown/tambo) | 🧰 Agent Platform Toolkit - installs and configures the quipu stack |
| [quipu](https://github.com/scbrown/quipu) | stores shared facts with provenance and history; can check writes against your rules |
| [camayoc](https://github.com/scbrown/camayoc) | prepares knowledge and vocabulary for Quipu, with checks before it is admitted |
| [quechua](https://github.com/scbrown/quechua) | shared vocabulary and shapes for interoperable knowledge |
| [chaski](https://github.com/scbrown/chaski) | reacts to graph changes and delivers governed notifications |
| [bobbin](https://github.com/scbrown/bobbin) | searches your code and can include related knowledge from Quipu |
| [yupana](https://github.com/scbrown/yupana) | shows code impact and can read policies from Quipu to check changes |
| [desire-path](https://github.com/scbrown/desire-path) | turns agent mistakes into feature requests; works on its own without Quipu |
| [seeds](https://github.com/scbrown/seeds) | a work item tracker backed by the Quipu knowledge base |
| [shantytown](https://github.com/scbrown/shantytown) | runs crews of AI agents on work items, triggered by hand, on a schedule or by events; can share work and memory through Quipu |
| [rikuy](https://github.com/scbrown/rikuy) | live brief: status, graphs, watches and signed decisions over Quipu |
| [shuttle](https://github.com/scbrown/shuttle) | runs signed agent workflows and keeps their records in Quipu |

<!-- stack-table:end -->

[License](LICENSE).
