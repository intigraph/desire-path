package analyze

import (
	"encoding/json"
	"testing"

	"github.com/scbrown/desire-path/internal/model"
)

func TestCategorizeDesire_EnvNeed(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		errMsg  string
		wantCat string
	}{
		{
			name:    "command not found",
			tool:    "Bash",
			errMsg:  "bash: cargo-insta: command not found",
			wantCat: model.CategoryEnvNeed,
		},
		{
			name:    "not found variant",
			tool:    "Bash",
			errMsg:  "/bin/sh: cargo-nextest: not found",
			wantCat: model.CategoryEnvNeed,
		},
		{
			name:    "exit code 127",
			tool:    "Bash",
			errMsg:  "exit code 127",
			wantCat: "",
		},
		{
			name:    "exit status 127",
			tool:    "Bash",
			errMsg:  "exit status 127",
			wantCat: "",
		},
		{
			name:    "exit 127",
			tool:    "Bash",
			errMsg:  "exit 127",
			wantCat: "",
		},
		{
			name:    "not found in PATH",
			tool:    "Bash",
			errMsg:  "cargo-insta: not found in PATH",
			wantCat: model.CategoryEnvNeed,
		},
		{
			name:    "not installed",
			tool:    "Bash",
			errMsg:  "cargo-nextest is not installed",
			wantCat: "",
		},
		{
			name:    "non-Bash tool ignored",
			tool:    "Read",
			errMsg:  "command not found",
			wantCat: "",
		},
		{
			name:    "generic Bash error not categorized",
			tool:    "Bash",
			errMsg:  "permission denied",
			wantCat: "",
		},
		{
			name:    "empty error",
			tool:    "Bash",
			errMsg:  "",
			wantCat: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CategorizeDesire(tt.tool, tt.errMsg, nil)
			if got != tt.wantCat {
				t.Errorf("CategorizeDesire(%q, %q) = %q, want %q", tt.tool, tt.errMsg, got, tt.wantCat)
			}
		})
	}
}

func TestEnvNeedCommand(t *testing.T) {
	tests := []struct {
		name      string
		errMsg    string
		toolInput json.RawMessage
		wantCmd   string
	}{
		{
			name:    "bash colon format",
			errMsg:  "bash: cargo-insta: command not found",
			wantCmd: "cargo-insta",
		},
		{
			name:    "sh colon format",
			errMsg:  "/bin/sh: cargo-nextest: not found",
			wantCmd: "cargo-nextest",
		},
		{
			name:    "command not found colon",
			errMsg:  "command not found: rg",
			wantCmd: "rg",
		},
		{
			name:      "from tool_input when error unclear",
			errMsg:    "exit code 127",
			toolInput: json.RawMessage(`{"command":"cargo-insta test"}`),
			wantCmd:   "",
		},
		{
			name:      "from tool_input with env vars",
			errMsg:    "exit code 127",
			toolInput: json.RawMessage(`{"command":"RUST_BACKTRACE=1 cargo-nextest run"}`),
			wantCmd:   "",
		},
		{
			name:    "no command extractable",
			errMsg:  "some other error",
			wantCmd: "",
		},
		{
			name:      "empty input",
			errMsg:    "",
			toolInput: nil,
			wantCmd:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnvNeedCommand(tt.errMsg, tt.toolInput)
			if got != tt.wantCmd {
				t.Errorf("EnvNeedCommand(%q, %s) = %q, want %q", tt.errMsg, tt.toolInput, got, tt.wantCmd)
			}
		})
	}
}

func TestMissingCommandEvidence(t *testing.T) {
	tests := []struct{ err, want string }{
		{"ls: cannot access 'missing': No such file or directory", ""},
		{"bash: ./script: cannot execute: required file not found", ""},
		{"/bin/sh: 1: ./script: not found", ""},
		{"/bin/sh: 1: absent-tool: not found", "absent-tool"},
		{"cat: absent-tool: not found", ""},
		{"Exit code 127", ""},
		{"dependency is not installed", ""},
		{"bash: line 12: absent-tool: command not found", "absent-tool"},
		{"zsh: command not found: absent-tool", "absent-tool"},
		{"bash: -C: command not found", ""},
		{"bash: one: command not found\nbash: two: command not found", ""},
		{"bash: absent-tool: command not found\n", "absent-tool"},
	}
	for _, tt := range tests {
		input := json.RawMessage(`{"command":"cd /tmp; date; absent-tool --help"}`)
		if got := EnvNeedCommand(tt.err, input); got != tt.want {
			t.Errorf("%q: got %q want %q", tt.err, got, tt.want)
		}
		cat := CategorizeDesire("Bash", tt.err, input)
		if (cat == model.CategoryEnvNeed) != (tt.want != "") {
			t.Errorf("%q: category %q", tt.err, cat)
		}
	}
}
