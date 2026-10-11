package analyze

import (
	"encoding/json"
	"github.com/scbrown/desire-path/internal/model"
	"testing"
	"time"
)

func TestFamilyPrograms(t *testing.T) {
	for _, tt := range []struct {
		a, b  string
		equal bool
	}{
		{"git -C /tmp/a status", "git -C /tmp/b status", true},
		{"git status", "git fetch", false},
		{"git status && cargo test", "git status && cargo build", false},
		{"printf 'secret value'", "printf 'different secret'", true},
	} {
		raw := func(s string) json.RawMessage { b, _ := json.Marshal(map[string]string{"command": s}); return b }
		a, okA := commandShape(raw(tt.a))
		b, okB := commandShape(raw(tt.b))
		if !okA || !okB || (a == b) != tt.equal {
			t.Fatalf("shape mismatch %q/%q", a, b)
		}
	}
	for _, command := range []string{"$TARGET arg", "env KEY=value git status", "timeout 5 git status", "git --unknown secret", "bash -c 'git status'", "printf 'unterminated"} {
		raw, _ := json.Marshal(map[string]string{"command": command})
		if _, ok := commandShape(raw); ok {
			t.Fatalf("ambiguous command accepted")
		}
	}
}

func TestFamilyRollingWindow(t *testing.T) {
	base := time.Unix(10000, 0)
	rows := []model.Desire{}
	for _, offset := range []time.Duration{0, 59 * time.Minute, 60 * time.Minute, 61 * time.Minute} {
		rows = append(rows, model.Desire{ToolName: "Read", Error: "Exit code: 1", Timestamp: base.Add(offset)})
	}
	rows = append(rows, model.Desire{Category: model.CategoryTurnPattern}, model.Desire{ToolName: "Bash"})
	r := AggregateFailureFamilies(rows, time.Hour, base.Add(62*time.Minute))
	if len(r.Families) != 1 || r.Families[0].Peak != 3 || r.Families[0].Current != 3 || r.Unknown != 1 || r.Excluded != 1 {
		t.Fatalf("%+v", r)
	}
	data, _ := json.Marshal(r)
	if string(data) == "" {
		t.Fatal("missing report")
	}
}
