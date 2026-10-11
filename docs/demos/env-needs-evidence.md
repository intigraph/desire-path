# Missing-command evidence demo

This is an actual CLI transcript, not a recording (no terminal recorder was available). Each binary ingested the same three synthetic failures into a separate temporary database. The command input was `cd /tmp; missing-command --help`; diagnostics were a missing input file, a bare exit code127, and `bash: line 1: missing-command: command not found`.

Installed dp0.6.0 (65ef56b) reported:

```json
[{"command":"cd","count":3}]
```

The candidate reports:

```json
[{"command":"missing-command","count":1}]
```

The later failed command comes from the diagnostic. Missing files and ambiguous execution failures remain unclassified. This demonstrates candidate behavior in synthetic local databases; it does not establish installation or availability of commands on another host.

Repeat the candidate control with:

```sh
go test ./internal/analyze ./internal/cli -run 'TestMissingCommandEvidence|TestEnvNeedsRevalidatesHistoricalAttribution' -count=1
```

The separate private frozen-corpus probe contained44 unsupported recommendations across13 names before the change and zero afterward. Private commands and capture records are not included here.
