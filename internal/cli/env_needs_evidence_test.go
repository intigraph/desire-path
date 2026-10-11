package cli

import (
	"encoding/json"
	"github.com/scbrown/desire-path/internal/model"
	"testing"
)

func TestEnvNeedsRevalidatesHistoricalAttribution(t *testing.T) {
	input := json.RawMessage(`{"command":"cd /tmp; ls absent; missing-tool"}`)
	desires := []model.Desire{
		{Category: model.CategoryEnvNeed, Error: "ls: absent: No such file or directory", ToolInput: input},
		{Category: model.CategoryEnvNeed, Error: "Exit code 127", ToolInput: input},
		{Category: model.CategoryEnvNeed, Error: "bash: line 1: missing-tool: command not found", ToolInput: input},
	}
	needs := aggregateEnvNeeds(desires)
	if len(needs) != 1 || needs[0].Command != "missing-tool" || needs[0].Count != 1 {
		t.Fatalf("historical file/batch attribution survived: %#v", needs)
	}
}
