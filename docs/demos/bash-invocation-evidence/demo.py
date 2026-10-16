#!/usr/bin/env python3
"""Compare two supplied dp binaries using only isolated synthetic invocations."""
import json
import os
import subprocess
import tempfile

fixtures = [
    ("success", "sd show example --json", {"stdout": "{}", "exit_code": 0}, ""),
    ("masked", "sd create example || true", {"stdout": "Error: HTTP 502 Bad Gateway", "exit_code": 0}, ""),
    ("native", "sd create example", None, "Exit code 1: HTTP 502 Bad Gateway"),
]

for label, variable in [("base 3abe720", "DP_DEMO_BASE"), ("candidate b7a6e01", "DP_DEMO_HEAD")]:
    print(label, flush=True)
    with tempfile.TemporaryDirectory() as directory:
        env = {**os.environ, "HOME": directory, "XDG_CONFIG_HOME": directory + "/config", "DP_QUIPU_REVIEW": "0"}
        for name, command, response, error in fixtures:
            payload = {"tool_name": "Bash", "tool_input": {"command": command},
                       "hook_event_name": "PostToolUseFailure" if error else "PostToolUse"}
            if error:
                payload["error"] = error
            if response is not None:
                payload["tool_response"] = response
            result = subprocess.run([os.environ[variable], "--db", directory + "/probe.db",
                                     "--json", "ingest", "--source", "claude-code"],
                                    input=json.dumps(payload), env=env, text=True,
                                    capture_output=True, check=True)
            invocation = json.loads(result.stdout)
            metadata = invocation.get("metadata", {})
            print(json.dumps({"fixture": name, "submitted_command": command,
                              "is_error": invocation["is_error"],
                              "command_shape": metadata.get("command_shape"),
                              "failure_evidence": metadata.get("failure_evidence")}), flush=True)
