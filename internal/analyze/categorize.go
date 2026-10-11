package analyze

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/scbrown/desire-path/internal/model"
)

// Only diagnostic identities are evidence of a missing command. A bare exit
// status, missing file, or missing interpreter cannot identify an install need.
var missingCommandPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^(?:bash|sh|dash|zsh|/(?:usr/)?bin/(?:bash|sh|dash|zsh)):(?: (?:line )?[0-9]+:)?\s+([^\s:]+):\s+(?:command )?not found\s*$`),
	regexp.MustCompile(`(?m)^(?:zsh: )?command not found:\s+([^\s:]+)\s*$`),
	regexp.MustCompile(`(?m)^([^\s:]+):\s+not found in PATH\s*$`),
}
var commandIdentity = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+@-]*$`)

// CategorizeDesire leaves ambiguous failures unclassified.
func CategorizeDesire(toolName, errorMsg string, toolInput json.RawMessage) string {
	if toolName == "Bash" && EnvNeedCommand(errorMsg, toolInput) != "" {
		return model.CategoryEnvNeed
	}
	return ""
}

// EnvNeedCommand uses the failed command named by an execution diagnostic.
// It never attributes a later failure to the leading token of a shell batch.
func EnvNeedCommand(errorMsg string, _ json.RawMessage) string {
	found := ""
	for _, pattern := range missingCommandPatterns {
		for _, match := range pattern.FindAllStringSubmatch(errorMsg, -1) {
			candidate := strings.Trim(match[1], `"'`)
			if !commandIdentity.MatchString(candidate) {
				continue
			}
			if found != "" && found != candidate {
				return ""
			}
			found = candidate
		}
	}
	return found
}
