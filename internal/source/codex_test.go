package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexName(t *testing.T) {
	c := &codexCLI{}
	if got := c.Name(); got != "codex" {
		t.Errorf("Name() = %q, want %q", got, "codex")
	}
}

func TestCodexDescription(t *testing.T) {
	c := &codexCLI{}
	got := c.Description()
	if got == "" {
		t.Error("Description() should not be empty")
	}
	if got != "OpenAI Codex CLI Stop hook, legacy notify and exec events" {
		t.Errorf("Description() = %q, want %q", got, "OpenAI Codex CLI Stop hook, legacy notify and exec events")
	}
}

func TestCodexExtract(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
		check   func(t *testing.T, f *Fields)
	}{
		{
			name:  "notify hook agent-turn-complete payload",
			input: `{"type":"agent-turn-complete","thread-id":"0199a213-81c0-7800-8aa1-bbab2a035a53","turn-id":"turn-001","cwd":"/home/user/project","input-messages":["fix the bug"],"last-assistant-message":"Done."}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "agent_turn" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "agent_turn")
				}
				if f.InstanceID != "0199a213-81c0-7800-8aa1-bbab2a035a53" {
					t.Errorf("InstanceID = %q, want %q", f.InstanceID, "0199a213-81c0-7800-8aa1-bbab2a035a53")
				}
				if f.CWD != "/home/user/project" {
					t.Errorf("CWD = %q, want %q", f.CWD, "/home/user/project")
				}
				if f.Error != "" {
					t.Errorf("Error = %q, want empty", f.Error)
				}

				// Extra should contain turn-specific fields.
				if f.Extra == nil {
					t.Fatal("Extra should not be nil")
				}
				for _, key := range []string{"turn-id", "input-messages", "last-assistant-message", "event_type"} {
					if _, ok := f.Extra[key]; !ok {
						t.Errorf("Extra should contain %q", key)
					}
				}

				// event_type should be "agent-turn-complete".
				var et string
				if err := json.Unmarshal(f.Extra["event_type"], &et); err != nil {
					t.Fatalf("parsing event_type: %v", err)
				}
				if et != "agent-turn-complete" {
					t.Errorf("event_type = %q, want %q", et, "agent-turn-complete")
				}

				// Universal fields should NOT be in Extra.
				for _, key := range []string{"cwd", "thread-id", "type"} {
					if _, ok := f.Extra[key]; ok {
						t.Errorf("Extra should not contain %q", key)
					}
				}
			},
		},
		{
			name:  "notify hook minimal payload",
			input: `{"type":"agent-turn-complete"}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "agent_turn" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "agent_turn")
				}
				if f.InstanceID != "" {
					t.Errorf("InstanceID = %q, want empty", f.InstanceID)
				}
				if f.CWD != "" {
					t.Errorf("CWD = %q, want empty", f.CWD)
				}
				// Extra should still have event_type.
				if f.Extra == nil {
					t.Fatal("Extra should not be nil (should have event_type)")
				}
			},
		},
		{
			name:  "item.completed command_execution event",
			input: `{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"bash -lc ls","status":"completed"}}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "command_execution" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "command_execution")
				}
				if f.Error != "" {
					t.Errorf("Error = %q, want empty", f.Error)
				}

				// ToolInput should be the command.
				var cmd string
				if err := json.Unmarshal(f.ToolInput, &cmd); err != nil {
					t.Fatalf("parsing ToolInput: %v", err)
				}
				if cmd != "bash -lc ls" {
					t.Errorf("ToolInput = %q, want %q", cmd, "bash -lc ls")
				}

				// Extra should contain item-level fields.
				if f.Extra == nil {
					t.Fatal("Extra should not be nil")
				}
				for _, key := range []string{"id", "event_type", "status"} {
					if _, ok := f.Extra[key]; !ok {
						t.Errorf("Extra should contain %q", key)
					}
				}
			},
		},
		{
			name:  "item.completed file_change event",
			input: `{"type":"item.completed","item":{"id":"item_2","type":"file_change","status":"completed"}}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "file_change" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "file_change")
				}
				if f.ToolInput != nil {
					t.Errorf("ToolInput = %s, want nil (file_change has no command)", f.ToolInput)
				}
			},
		},
		{
			name:  "item.completed agent_message event",
			input: `{"type":"item.completed","item":{"id":"item_3","type":"agent_message","text":"The task is complete."}}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "agent_message" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "agent_message")
				}
				// text should be in Extra.
				if f.Extra == nil {
					t.Fatal("Extra should not be nil")
				}
				if _, ok := f.Extra["text"]; !ok {
					t.Error("Extra should contain text")
				}
			},
		},
		{
			name:  "item.completed mcp_tool_call event",
			input: `{"type":"item.completed","item":{"id":"item_4","type":"mcp_tool_call","status":"completed"}}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "mcp_tool_call" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "mcp_tool_call")
				}
			},
		},
		{
			name:  "item.completed failed status sets Error",
			input: `{"type":"item.completed","item":{"id":"item_5","type":"command_execution","command":"bad-cmd","status":"failed"}}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "command_execution" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "command_execution")
				}
				if f.Error != "item failed" {
					t.Errorf("Error = %q, want %q", f.Error, "item failed")
				}
			},
		},
		{
			name:  "item.completed error status sets Error",
			input: `{"type":"item.completed","item":{"id":"item_6","type":"command_execution","command":"crash-cmd","status":"error"}}`,
			check: func(t *testing.T, f *Fields) {
				if f.Error != "item error" {
					t.Errorf("Error = %q, want %q", f.Error, "item error")
				}
			},
		},
		{
			name:  "item.started event also accepted",
			input: `{"type":"item.started","item":{"id":"item_7","type":"command_execution","command":"ls","status":"in_progress"}}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "command_execution" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "command_execution")
				}
			},
		},
		{
			name:    "missing type field",
			input:   `{"thread-id":"abc","cwd":"/tmp"}`,
			wantErr: "missing required field: type",
		},
		{
			name:    "empty type field",
			input:   `{"type":""}`,
			wantErr: "missing required field: type",
		},
		{
			name:    "unsupported event type",
			input:   `{"type":"thread.started","thread_id":"abc"}`,
			wantErr: "unsupported event type",
		},
		{
			name:    "turn.completed is unsupported",
			input:   `{"type":"turn.completed","usage":{"input_tokens":100}}`,
			wantErr: "unsupported event type",
		},
		{
			name:    "invalid JSON",
			input:   `not json`,
			wantErr: "parsing JSON",
		},
		{
			name:    "type is number",
			input:   `{"type":123}`,
			wantErr: "parsing type",
		},
		{
			name:    "item.completed missing item",
			input:   `{"type":"item.completed"}`,
			wantErr: "missing required field: item",
		},
		{
			name:    "item.completed item is string",
			input:   `{"type":"item.completed","item":"bad"}`,
			wantErr: "parsing item",
		},
		{
			name:    "item.completed missing item type",
			input:   `{"type":"item.completed","item":{"id":"item_1"}}`,
			wantErr: "item missing required field: type",
		},
		{
			name:    "item.completed empty item type",
			input:   `{"type":"item.completed","item":{"type":""}}`,
			wantErr: "item missing required field: type",
		},
		{
			name:    "cwd is array in notify",
			input:   `{"type":"agent-turn-complete","cwd":["a"]}`,
			wantErr: "parsing cwd",
		},
		{
			name:    "thread-id is number",
			input:   `{"type":"agent-turn-complete","thread-id":123}`,
			wantErr: "parsing thread-id",
		},
	}

	c := &codexCLI{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := c.Extract([]byte(tt.input))

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			tt.check(t, f)
		})
	}
}

func TestCodexRegistered(t *testing.T) {
	s := Get("codex")
	if s == nil {
		t.Fatal("codex source not found in registry")
	}
	if s.Name() != "codex" {
		t.Errorf("Name() = %q, want %q", s.Name(), "codex")
	}
}

func TestCodexInstall(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".codex", "config.toml")

	c := &codexCLI{}
	if err := c.Install(InstallOpts{SettingsPath: configPath}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	// Verify the file was written.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "dp ingest") {
		t.Errorf("config should contain dp ingest command, got:\n%s", content)
	}
	if !strings.Contains(content, "Stop") || strings.Contains(content, "notify") {
		t.Errorf("expected Stop without notify, got:\n%s", content)
	}
}

func TestCodexInstallIdempotent(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".codex", "config.toml")

	c := &codexCLI{}

	// Install twice.
	if err := c.Install(InstallOpts{SettingsPath: configPath}); err != nil {
		t.Fatalf("first Install() error: %v", err)
	}
	if err := c.Install(InstallOpts{SettingsPath: configPath}); err != nil {
		t.Fatalf("second Install() error: %v", err)
	}

	// Read and verify config has exactly the expected notify value.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	// The file should only have one notify entry.
	count := strings.Count(string(data), "dp ingest")
	if count != 1 {
		t.Errorf("expected 1 dp ingest reference, got %d in:\n%s", count, string(data))
	}
}

func TestCodexInstallPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	codexDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(codexDir, "config.toml")

	// Write existing config with other settings.
	existing := `model = "gpt-5.2-codex"

[tui]
notifications = true
`
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &codexCLI{}
	if err := c.Install(InstallOpts{SettingsPath: configPath}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	// Read back and verify existing settings preserved.
	config, err := readCodexConfig(configPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	if config["model"] != "gpt-5.2-codex" {
		t.Errorf("model = %v, want %q", config["model"], "gpt-5.2-codex")
	}

	// tui section should be preserved.
	tui, ok := config["tui"]
	if !ok {
		t.Error("tui section should be preserved")
	} else {
		tuiMap, ok := tui.(map[string]interface{})
		if !ok {
			t.Errorf("tui should be a map, got %T", tui)
		} else if tuiMap["notifications"] != true {
			t.Errorf("tui.notifications = %v, want true", tuiMap["notifications"])
		}
	}

	if !codexStopInstalled(config) {
		t.Error("Stop hook should be installed")
	}
}

func TestCodexInstallPreservesForeignNotify(t *testing.T) {
	dir := t.TempDir()
	codexDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(codexDir, "config.toml")

	// Write config with existing non-dp notify.
	existing := `notify = ["python3", "/path/to/custom-notify.py"]
`
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &codexCLI{}
	if err := c.Install(InstallOpts{SettingsPath: configPath}); err != nil {
		t.Fatal(err)
	}
	cfg, err := readCodexConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	notify := cfg["notify"].([]interface{})
	if len(notify) != 2 || notify[0] != "python3" || notify[1] != "/path/to/custom-notify.py" || !codexStopInstalled(cfg) {
		t.Fatalf("foreign config changed: %v", cfg)
	}

}

func TestCodexIsInstalledNoFile(t *testing.T) {
	dir := t.TempDir()
	c := &codexCLI{}

	installed, err := c.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if installed {
		t.Error("IsInstalled() should be false when config file does not exist")
	}
}

func TestCodexIsInstalledNoNotify(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`model = "gpt-5.2-codex"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &codexCLI{}
	installed, err := c.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if installed {
		t.Error("IsInstalled() should be false when no notify is configured")
	}
}

func TestCodexIsInstalledWithDPIngest(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	c := &codexCLI{}
	if err := c.Install(InstallOpts{SettingsPath: configPath}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	installed, err := c.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if !installed {
		t.Error("IsInstalled() should be true after Install()")
	}
}

func TestCodexIsInstalledOtherNotifyOnly(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	if err := os.WriteFile(configPath, []byte(`notify = ["python3", "/path/to/other.py"]`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &codexCLI{}
	installed, err := c.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if installed {
		t.Error("IsInstalled() should be false when only non-dp notify exists")
	}
}

func TestCodexImplementsInstaller(t *testing.T) {
	var _ Installer = &codexCLI{}
}

// Stop hooks stream their payload instead of appending the whole turn to argv.
func TestCodexStopHook(t *testing.T) {
	c := &codexCLI{}
	raw := `{"hook_event_name":"Stop","session_id":"session-123","cwd":"/fixture/project","turn_id":"turn-1","last_assistant_message":"` + strings.Repeat("x", 256*1024) + `"}`
	f, err := c.Extract([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if f.ToolName != "agent_turn" || f.InstanceID != "session-123" || f.CWD != "/fixture/project" {
		t.Fatalf("wrong identity: %+v", f)
	}
	if string(f.Extra["turn_id"]) != `"turn-1"` {
		t.Fatalf("turn_id lost: %v", f.Extra)
	}
	if _, ok := f.Extra["last_assistant_message"]; ok {
		t.Fatal("Stop retained assistant prose")
	}
	legacy, err := c.Extract([]byte(`{"type":"agent-turn-complete","thread-id":"session-123","cwd":"/fixture/project","turn-id":"turn-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if legacy.ToolName != f.ToolName || legacy.InstanceID != f.InstanceID || legacy.CWD != f.CWD || string(legacy.Extra["turn-id"]) != string(f.Extra["turn_id"]) {
		t.Fatal("Stop/notify record identity differs")
	}
}

func TestCodexStopHookRefusesInvalidMetadata(t *testing.T) {
	for _, raw := range []string{
		`{"hook_event_name":"PreToolUse","session_id":"s"}`,
		`{"hook_event_name":4,"session_id":"s"}`,
		`{"hook_event_name":"Stop"}`,
		`{"hook_event_name":"Stop","session_id":""}`,
		`{"hook_event_name":"Stop","session_id":3}`,
		`{"hook_event_name":"Stop","session_id":null}`,
		`{"hook_event_name":"Stop","session_id":"s","cwd":null}`,
		`{"hook_event_name":"Stop","session_id":"s","turn_id":null}`,
		`{"hook_event_name":"Stop","session_id":"s","cwd":3}`,
		`{"hook_event_name":"Stop","session_id":"s","turn_id":3}`,
	} {
		if _, err := (&codexCLI{}).Extract([]byte(raw)); err == nil {
			t.Errorf("accepted malformed Stop: %s", raw)
		}
	}
}

func TestCodexInstallMigratesOnlyExactLegacyNotify(t *testing.T) {
	for _, exact := range []bool{true, false} {
		t.Run(fmt.Sprint(exact), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.toml")
			argv := make([]interface{}, len(dpCodexNotifyScript))
			for i, value := range dpCodexNotifyScript {
				argv[i] = value
			}
			if !exact {
				argv[2] = dpCodexNotifyCommand + " ; other-hook"
			}
			cfg := map[string]interface{}{"notify": argv, "model": "preserved", "hooks": map[string]interface{}{"Stop": []interface{}{map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": "other hook"}}}}}}
			if err := writeCodexConfig(path, cfg); err != nil {
				t.Fatal(err)
			}
			c := &codexCLI{}
			if installed, err := c.IsInstalled(dir); err != nil || installed {
				t.Fatalf("legacy should not be current: %v/%v", installed, err)
			}
			if err := c.Install(InstallOpts{SettingsPath: path}); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(path)
			if err := c.Install(InstallOpts{SettingsPath: path}); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(path)
			if string(data) != string(after) {
				t.Fatal("install not idempotent")
			}
			cfg, err := readCodexConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			_, present := cfg["notify"]
			if present == exact {
				t.Fatalf("wrong notify migration: %v", cfg)
			}
			if !strings.Contains(string(after), "other hook") || cfg["model"] != "preserved" {
				t.Fatal("foreign hook/key lost")
			}
			if installed, err := c.IsInstalled(dir); err != nil || !installed {
				t.Fatalf("Stop not current: %v/%v", installed, err)
			}
		})
	}
}

func TestCodexInstallMalformedHooksLeavesConfigUntouched(t *testing.T) {
	for _, raw := range []string{`hooks = "foreign"`, `[hooks]
Stop = "foreign"`} {
		path := filepath.Join(t.TempDir(), "config.toml")
		os.WriteFile(path, []byte(raw), 0600)
		if err := (&codexCLI{}).Install(InstallOpts{SettingsPath: path}); err == nil {
			t.Fatal("accepted malformed hooks")
		}
		after, _ := os.ReadFile(path)
		if string(after) != raw {
			t.Fatal("modified refused config")
		}
	}
}
