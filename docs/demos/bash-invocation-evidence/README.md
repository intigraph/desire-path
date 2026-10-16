# Bash invocation capture demonstration

Trigger: an isolated `PostToolUse` payload reports exit zero but its output says
`Error: HTTP 502 Bad Gateway`. Base source `3abe720` records success without
command identity. Candidate implementation `b7a6e01` records an output-signature
error and `sd:create/true`. The clean and native-error controls remain correct.
These are synthetic hook payloads; no `sd` command or production database runs.

Build the two binaries from those revisions, then set `DP_DEMO_BASE` and
`DP_DEMO_HEAD` to their paths and run `python3 demo.py`. The retained recording
uses util-linux `script`; download both text files and replay with:

```sh
scriptreplay --log-out transcript.txt --log-timing timing.txt
```

The recording is retained with the source in Git. It requires downloading and
a compatible terminal replay tool; no inline player is claimed. Documentation
and recording commits after `b7a6e01` preserve its demonstrated Go blobs.
This proves candidate ingestion behavior, not an installed hook or fleet rollout.
