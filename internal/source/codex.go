package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// knownCodexNotifyFields lists JSON keys from the Codex notify hook payload
// that map to universal Fields. Everything else goes into Extra.
var knownCodexNotifyFields = map[string]bool{
	"cwd": true,
}

// codexCLI implements Source for OpenAI Codex CLI.
type codexCLI struct{}

func init() {
	Register(&codexCLI{})
}

// Name returns "codex".
func (c *codexCLI) Name() string { return "codex" }

// Description returns a short human-readable description of this source.
func (c *codexCLI) Description() string {
	return "OpenAI Codex CLI Stop hook, legacy notify and exec events"
}

// Extract parses Codex CLI JSON and maps fields to the universal Fields struct.
// It handles three formats:
//
//  1. Native Stop hook: {"hook_event_name":"Stop","session_id":"...","turn_id":"..."}
//  2. Notify hook payload: {"type":"agent-turn-complete","thread-id":"...","cwd":"..."}
//  3. Item event from codex exec --json: {"type":"item.completed","item":{"type":"command_execution",...}}
//
// For notify payloads, ToolName is set to "agent_turn" (a synthetic tool name).
// For item events, ToolName is set to the item's type (e.g., "command_execution").
func (c *codexCLI) Extract(raw []byte) (*Fields, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("codex: parsing JSON: %w", err)
	}

	// Native hooks deliver JSON on stdin, without notify's argv size limit.
	if hook, ok := m["hook_event_name"]; ok {
		var event string
		if err := json.Unmarshal(hook, &event); err != nil || event != "Stop" {
			return nil, fmt.Errorf("codex: unsupported hook_event_name (expected Stop)")
		}
		return c.extractStop(m)
	}

	// Detect format by "type" field.
	typeRaw, ok := m["type"]
	if !ok {
		return nil, fmt.Errorf("codex: missing required field: type")
	}

	var eventType string
	if err := json.Unmarshal(typeRaw, &eventType); err != nil {
		return nil, fmt.Errorf("codex: parsing type: %w", err)
	}
	if eventType == "" {
		return nil, fmt.Errorf("codex: missing required field: type")
	}

	if eventType == "agent-turn-complete" {
		return c.extractNotify(m, eventType)
	}

	if eventType == "item.completed" || eventType == "item.started" {
		return c.extractItem(m, eventType)
	}

	return nil, fmt.Errorf("codex: unsupported event type: %q", eventType)
}

// extractStop records turn identity, not assistant prose or transcript contents.
// The legacy notify parser below remains compatible with existing producers.
func (c *codexCLI) extractStop(m map[string]json.RawMessage) (*Fields, error) {
	f := &Fields{ToolName: "agent_turn", Extra: map[string]json.RawMessage{"event_type": json.RawMessage(`"Stop"`)}}
	for _, key := range []string{"session_id", "cwd", "turn_id"} {
		raw, ok := m[key]
		if !ok {
			if key == "session_id" {
				return nil, fmt.Errorf("codex: Stop missing required field: session_id")
			}
			continue
		}
		var value *string
		if err := json.Unmarshal(raw, &value); err != nil || value == nil {
			return nil, fmt.Errorf("codex: Stop %s must be a string", key)
		}
		switch key {
		case "session_id":
			if *value == "" {
				return nil, fmt.Errorf("codex: Stop missing required field: session_id")
			}
			f.InstanceID = *value
		case "cwd":
			f.CWD = *value
		case "turn_id":
			f.Extra[key] = raw
		}
	}
	return f, nil
}

// extractNotify handles the Codex notify hook payload format.
func (c *codexCLI) extractNotify(m map[string]json.RawMessage, eventType string) (*Fields, error) {
	var f Fields
	f.ToolName = "agent_turn"

	// thread-id → InstanceID
	if v, ok := m["thread-id"]; ok {
		if err := json.Unmarshal(v, &f.InstanceID); err != nil {
			return nil, fmt.Errorf("codex: parsing thread-id: %w", err)
		}
	}

	// cwd → CWD
	if v, ok := m["cwd"]; ok {
		if err := json.Unmarshal(v, &f.CWD); err != nil {
			return nil, fmt.Errorf("codex: parsing cwd: %w", err)
		}
	}

	// Collect non-universal fields into Extra.
	extra := make(map[string]json.RawMessage)
	for k, v := range m {
		if !knownCodexNotifyFields[k] && k != "thread-id" && k != "type" {
			extra[k] = v
		}
	}
	// Always include event_type in Extra for downstream consumers.
	etJSON, _ := json.Marshal(eventType)
	extra["event_type"] = etJSON

	if len(extra) > 0 {
		f.Extra = extra
	}

	return &f, nil
}

// extractItem handles the codex exec --json item event format.
func (c *codexCLI) extractItem(m map[string]json.RawMessage, eventType string) (*Fields, error) {
	itemRaw, ok := m["item"]
	if !ok {
		return nil, fmt.Errorf("codex: %s event missing required field: item", eventType)
	}

	var item map[string]json.RawMessage
	if err := json.Unmarshal(itemRaw, &item); err != nil {
		return nil, fmt.Errorf("codex: parsing item: %w", err)
	}

	var f Fields

	// item.type → ToolName (e.g., "command_execution", "file_change")
	itemTypeRaw, ok := item["type"]
	if !ok {
		return nil, fmt.Errorf("codex: item missing required field: type")
	}
	if err := json.Unmarshal(itemTypeRaw, &f.ToolName); err != nil {
		return nil, fmt.Errorf("codex: parsing item type: %w", err)
	}
	if f.ToolName == "" {
		return nil, fmt.Errorf("codex: item missing required field: type")
	}

	// item.command → ToolInput for command_execution items
	if cmd, ok := item["command"]; ok {
		f.ToolInput = cmd
	}

	// item.status → Error if status indicates failure
	if statusRaw, ok := item["status"]; ok {
		var status string
		if err := json.Unmarshal(statusRaw, &status); err == nil {
			if status == "failed" || status == "error" {
				f.Error = "item " + status
			}
		}
	}

	// Collect item-level fields (except "type") into Extra.
	extra := make(map[string]json.RawMessage)
	for k, v := range item {
		if k != "type" && k != "command" && k != "status" {
			extra[k] = v
		}
	}

	// Include top-level event fields.
	etJSON, _ := json.Marshal(eventType)
	extra["event_type"] = etJSON

	// Include item status in Extra even though we derived Error from it.
	if statusRaw, ok := item["status"]; ok {
		extra["status"] = statusRaw
	}

	if len(extra) > 0 {
		f.Extra = extra
	}

	return &f, nil
}

// dpCodexNotifyCommand is the retired argv signature, retained only to identify
// the exact old dp-owned notify command during migration. It is never installed.
const dpCodexNotifyCommand = `printf '%s' "$1" | dp ingest --source codex`

// dpCodexNotifyScript retains the old complete argv, including its boundaries.
var dpCodexNotifyScript = []string{"bash", "-c", dpCodexNotifyCommand, "--"}

// Install configures the native Codex Stop stdin hook in ~/.codex/config.toml.
// Only the exact legacy dp notify argv is retired; foreign configuration stays.
func (c *codexCLI) Install(opts InstallOpts) error {
	configPath := opts.SettingsPath
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("determine home directory: %w", err)
		}
		configPath = filepath.Join(home, ".codex", "config.toml")
	}
	return installCodexStop(configPath)
}

// IsInstalled checks whether the native Stop hook is configured without the
// exact retired dp notify command in the
// Codex CLI config file. configDir should point to the directory containing
// config.toml (e.g., ~/.codex).
func (c *codexCLI) IsInstalled(configDir string) (bool, error) {
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, fmt.Errorf("determine home directory: %w", err)
		}
		configDir = filepath.Join(home, ".codex")
	}
	configPath := filepath.Join(configDir, "config.toml")

	config, err := readCodexConfig(configPath)
	if err != nil {
		return false, err
	}

	return codexStopInstalled(config) && !legacyCodexNotify(config["notify"]), nil
}

// legacyCodexNotify recognizes only the exact argv previously installed by dp.
func legacyCodexNotify(raw any) bool {
	arr, ok := raw.([]interface{})
	if !ok || len(arr) != len(dpCodexNotifyScript) {
		return false
	}
	for i, want := range dpCodexNotifyScript {
		if arr[i] != want {
			return false
		}
	}
	return true
}

func codexStopInstalled(config map[string]interface{}) bool {
	hooks, _ := config["hooks"].(map[string]interface{})
	groups, _ := hooks["Stop"].([]interface{})
	for _, raw := range groups {
		group, _ := raw.(map[string]interface{})
		// A matcher must not narrow this once-per-turn event.
		if matcher, exists := group["matcher"]; exists && matcher != "" {
			continue
		}
		commands, _ := group["hooks"].([]interface{})
		for _, raw := range commands {
			command, _ := raw.(map[string]interface{})
			if command["type"] == "command" && command["command"] == "dp ingest --source codex" {
				return true
			}
		}
	}
	return false
}

func installCodexStop(configPath string) error {
	config, err := readCodexConfig(configPath)
	if err != nil {
		return err
	}
	hooks, ok := config["hooks"].(map[string]interface{})
	if !ok {
		if _, exists := config["hooks"]; exists {
			return fmt.Errorf("codex: hooks must be a table")
		}
		hooks = map[string]interface{}{}
		config["hooks"] = hooks
	}
	groups, ok := hooks["Stop"].([]interface{})
	if !ok && hooks["Stop"] != nil {
		return fmt.Errorf("codex: hooks.Stop must be an array")
	}
	changed := false
	if !codexStopInstalled(config) {
		hooks["Stop"] = append(groups, map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": "dp ingest --source codex"}}})
		changed = true
	}
	if legacyCodexNotify(config["notify"]) {
		delete(config, "notify")
		changed = true
	}
	if !changed {
		return nil
	}
	return writeCodexConfig(configPath, config)
}

// readCodexConfig reads and parses a Codex TOML config file, returning an
// empty map if the file does not exist.
func readCodexConfig(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]interface{}), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var config map[string]interface{}
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return config, nil
}

// writeCodexConfig writes config back to the TOML file, creating the parent
// directory if needed.
func writeCodexConfig(path string, config map[string]interface{}) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	data, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
