package ingest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/source"
)

func TestBashInvocationEvidence(t *testing.T) {
	for _, test := range []struct {
		name, command, response, native, evidence, shape string
		wantError                                        bool
	}{
		{"success", "sd show private-argument", `{"stdout":"{}","exit_code":0}`, "", "", "sd:show", false},
		{"masked", "cd private-path && sd create private-argument || true", `{"stdout":"Error: HTTP 502 Bad Gateway","exit_code":0}`, "", "output-signature", "cd/sd:create/true", true},
		{"stderr", "gh api private-argument", `{"stderr":"HTTP 403 Forbidden","exit_code":0}`, "", "output-signature", "gh:api", true},
		{"refusal", "br create private-argument", `"REFUSED: current policy"`, "", "output-signature", "br:create", true},
		{"native", "sd create private-argument", `"Error: other"`, "explicit native error", "native-error", "sd:create", true},
		{"example", "sd show private-argument", `"example: Error: HTTP 502\nNo errors"`, "", "", "sd:show", false},
		{"dynamic", "$CMD private-argument", `"done"`, "", "", "", false},
		{"wrapper", "bash -c private-argument", `"done"`, "", "", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, _ := json.Marshal(map[string]string{"command": test.command})
			fields := &source.Fields{ToolName: "Bash", ToolInput: input, Error: test.native,
				Extra: map[string]json.RawMessage{"tool_response": json.RawMessage(test.response)}}
			s := &fakeStore{}
			inv, err := IngestFields(context.Background(), s, fields, "claude-code")
			if err != nil {
				t.Fatal(err)
			}
			var meta struct {
				Capture  string   `json:"command_capture"`
				Shape    string   `json:"command_shape"`
				Argv0    []string `json:"command_argv0"`
				Evidence string   `json:"failure_evidence"`
			}
			if err := json.Unmarshal(inv.Metadata, &meta); err != nil {
				t.Fatal(err)
			}
			if inv.IsError != test.wantError || meta.Evidence != test.evidence || meta.Shape != test.shape {
				t.Fatalf("error=%v metadata=%+v", inv.IsError, meta)
			}
			if len(s.desires) != boolInt(test.wantError) {
				t.Fatal("error invocation/desire views disagree")
			}
			if strings.Contains(meta.Shape+strings.Join(meta.Argv0, "/"), "private-") {
				t.Fatal("command argument retained in attribution")
			}
			if fields.Error != test.native || len(fields.Extra) != 1 {
				t.Fatal("caller fields mutated")
			}
			if test.shape == "" && meta.Capture != "unknown" {
				t.Fatal("unsupported attribution not marked unknown")
			}
		})
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestBashEvidenceBounds(t *testing.T) {
	f := &source.Fields{ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"sd show x"}`),
		Extra: map[string]json.RawMessage{"tool_response": json.RawMessage(`"` + strings.Repeat("x", 128*1024) + `"`)}}
	got := bashEvidence(f)
	if got.Error != "" || string(got.Extra["output_scan"]) != `"unknown"` {
		t.Fatal("oversized output not marked unknown")
	}
}
