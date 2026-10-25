package analyze

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
)

func TestFamilyReasonAccountingDistinguishesUnsupportedKinds(t *testing.T) {
	end := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	raw := func(command string) json.RawMessage {
		b, _ := json.Marshal(map[string]string{"command": command})
		return b
	}
	common := []model.Desire{{ToolName: "Bash", ToolInput: raw("printf private-argument"), Error: "exit code 1 private-error", Timestamp: end.Add(-time.Minute)}}
	for i := 0; i < 2; i++ {
		common = append(common, model.Desire{Category: model.CategoryTurnPattern, ToolName: "invalid tool!", Timestamp: end.Add(-time.Minute)})
	}
	var previous []FailureFamily
	for _, tt := range []struct {
		name, tool string
		input      json.RawMessage
		reason     string
	}{
		{"tool name", "invalid tool!", raw("private-command"), "invalid_tool_name"},
		{"command shape", "Bash", raw("timeout 1 private-command"), "unsupported_command_shape"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rows := append([]model.Desire{}, common...)
			for i := 0; i < 3; i++ {
				rows = append(rows, model.Desire{ToolName: tt.tool, ToolInput: tt.input, Timestamp: end.Add(-time.Minute)})
			}
			report := AggregateFailureFamilies(rows, time.Hour, end)
			if report.Unknown != 3 || report.Excluded != 2 || len(report.Families) != 1 || report.Families[0].Count != 1 {
				t.Fatalf("legacy accounting changed: %+v", report)
			}
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("sanitized report: %s", data)
			var decoded struct {
				Unknown  map[string]int `json:"unknown_by_reason"`
				Excluded map[string]int `json:"excluded_by_reason"`
			}
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded.Unknown, map[string]int{tt.reason: 3}) || !reflect.DeepEqual(decoded.Excluded, map[string]int{"turn_pattern": 2}) {
				t.Fatalf("missing or incorrect reason counts: %s", data)
			}
			for _, secret := range []string{"private-argument", "private-error", "private-command", "invalid tool!"} {
				if strings.Contains(string(data), secret) {
					t.Fatalf("raw fixture value leaked: %q", secret)
				}
			}
			if previous != nil && !reflect.DeepEqual(previous, report.Families) {
				t.Fatal("diagnostics changed normalized identity or counts")
			}
			previous = report.Families
		})
	}
}

func TestFamilyReasonAccountingOmitsEmptyDiagnostics(t *testing.T) {
	report := AggregateFailureFamilies(nil, time.Hour, time.Now())
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "_by_reason") {
		t.Fatalf("empty diagnostics should be omitted: %s", data)
	}
}
