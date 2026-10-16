package ingest

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/scbrown/desire-path/internal/analyze"
	"github.com/scbrown/desire-path/internal/source"
)

var outputErrorLine = regexp.MustCompile(`(?im)^[\t ]*(?:error:|fatal:|refused\b|HTTP(?:[\t ]+(?:error|status))?[\t ]+[45][0-9]{2}\b)`)

// bashEvidence copies fields before adding bounded, argument-free evidence.
// Output signatures remain distinct from native errors: expected negative
// controls can print errors too, and a shell program's failing member is unknown.
func bashEvidence(fields *source.Fields) *source.Fields {
	switch fields.ToolName {
	case "Bash", "exec_command", "functions.exec_command":
	default:
		return fields
	}
	f := *fields
	f.Extra = make(map[string]json.RawMessage, len(fields.Extra)+5)
	for key, value := range fields.Extra {
		f.Extra[key] = value
	}
	put := func(key string, value any) {
		f.Extra[key], _ = json.Marshal(value) // strings and []string cannot fail
	}
	shape, known := analyze.CommandProgram(f.ToolInput)
	put("command_capture", "unknown")
	if known {
		put("command_capture", "parsed-program")
		put("command_shape", shape)
		names := []string{}
		for _, command := range strings.Split(shape, "/") {
			name, _, _ := strings.Cut(command, ":")
			names = append(names, name)
		}
		put("command_argv0", names)
	}
	if f.Error != "" {
		put("failure_evidence", "native-error")
		return &f
	}
	response, ok := f.Extra["tool_response"]
	if !ok || len(response) > 128*1024 {
		put("output_scan", "unknown")
		return &f
	}
	var text string
	if string(response) == "null" {
		put("output_scan", "unknown")
		return &f
	}
	if json.Unmarshal(response, &text) != nil {
		var keys map[string]json.RawMessage
		if json.Unmarshal(response, &keys) != nil || (keys["stdout"] == nil && keys["stderr"] == nil && keys["output"] == nil) {
			put("output_scan", "unknown")
			return &f
		}
		var object struct {
			Stdout string `json:"stdout"`
			Stderr string `json:"stderr"`
			Output string `json:"output"`
		}
		if json.Unmarshal(response, &object) != nil {
			put("output_scan", "unknown")
			return &f
		}
		text = object.Stdout + "\n" + object.Stderr + "\n" + object.Output
	}
	put("output_scan", "scanned")
	if outputErrorLine.MatchString(text) {
		f.Error = "observed error signature in tool output; subprocess exit attribution unknown"
		put("failure_evidence", "output-signature")
	}
	return &f
}
