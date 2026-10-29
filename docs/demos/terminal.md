# Turn a failed call into guidance

This 20-second recording runs four native commands against a new local database. Its failure is a synthetic public fixture; the correction is written by the operator. It does not claim automatic discovery or a live agent integration. The animation’s reading holds are deliberate; actual command durations and executable identity are in `terminal/manifest.json`.

It was recorded with `dp 0.10.0` at source `0681233`; the manifest pins the executable SHA-256. Use that version to reproduce the same output. UUIDs and execution timings may differ. The vendored renderer is byte-identical to Tambo’s shared `stack-terminal-24` harness at `b0765b5d8f16a6a26acb3595552f17826d965b44`, SHA-256 `bebbe000565314f18a58731f2a1a4690577db127b3673fa10a45ce74eef0f23c`.

From the repository root, prepare a new, absent workspace with the public fixture and recorder sentinel:

```sh
mkdir /tmp/desire-path-demo
cp docs/demos/terminal/fixture/failed-read.json /tmp/desire-path-demo/
touch /tmp/desire-path-demo/.terminal-demo-sandbox
python3 docs/demos/terminal/render.py docs/demos/terminal/demo.tape.json \
  --workspace /tmp/desire-path-demo --tool dp="$(command -v dp)" \
  --output /tmp/desire-path-render
```

The shared recorder isolates HOME and XDG paths and rejects private output. Do not reuse a workspace with an existing `demo.db`, install hooks, or configure a network endpoint. Read `terminal/transcript.txt` for a static alternative.
