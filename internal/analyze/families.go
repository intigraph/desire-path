package analyze

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/scbrown/desire-path/internal/model"
	"mvdan.cc/sh/v3/syntax"
)

// FailureFamily contains counts and bounded identifiers, never recorded inputs.
type FailureFamily struct {
	ID      string `json:"id"`
	Tool    string `json:"tool"`
	Class   string `json:"class"`
	Count   int    `json:"count"`
	Peak    int    `json:"peak_window"`
	Current int    `json:"current_window"`
}

// FamilyReport discloses records which cannot safely be normalized.
type FamilyReport struct {
	Families      []FailureFamily `json:"families"`
	Unknown       int             `json:"unknown"`
	Excluded      int             `json:"excluded"`
	WindowSeconds int64           `json:"window_seconds"`
}

var familyName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)
var familyTool = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.:-]{0,127}$`)
var exitClass = regexp.MustCompile(`(?i)exit (?:code|status)\s*:?\s*([0-9]{1,3})\b`)

func failureClass(message string) string {
	s := strings.ToLower(message)
	for _, entry := range []struct{ class, marker string }{
		{"permission", "permission denied"}, {"authentication", "unauthorized"},
		{"timeout", "timed out"}, {"timeout", "timeout"},
		{"missing", "command not found"}, {"missing", "no such file"},
		{"unavailable", "connection refused"}, {"usage", "unknown flag"},
	} {
		if strings.Contains(s, entry.marker) {
			return entry.class
		}
	}
	if m := exitClass.FindStringSubmatch(s); m != nil {
		return "exit-" + m[1]
	}
	return "unclassified"
}

func literal(word *syntax.Word) (string, bool) {
	// Quoting and expansion in the command identity are intentionally UNKNOWN.
	if len(word.Parts) != 1 {
		return "", false
	}
	p, ok := word.Parts[0].(*syntax.Lit)
	if !ok {
		return "", false
	}
	return p.Value, true
}

func commandShape(raw json.RawMessage) (string, bool) {
	var input struct {
		Command string `json:"command"`
		Cmd     string `json:"cmd"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return "", false
	}
	command := input.Command
	if command == "" {
		command = input.Cmd
	}
	if command == "" {
		return "", false
	}
	f, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		return "", false
	}
	parts := []string{}
	ok := true
	syntax.Walk(f, func(node syntax.Node) bool {
		if !ok {
			return false
		}
		call, isCall := node.(*syntax.CallExpr)
		if !isCall {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		name, simple := literal(call.Args[0])
		name = path.Base(name)
		if !simple || !familyName.MatchString(name) {
			ok = false
			return false
		}
		// Wrappers require an execution grammar; do not attribute their nested target.
		switch name {
		case "env", "timeout", "sudo", "sh", "bash", "eval", "exec", "xargs":
			ok = false
			return false
		}
		shape := name
		switch name {
		case "git", "cargo", "go", "gh", "npm", "docker", "dp", "sd", "st", "br":
			i := 1
			if name == "git" && len(call.Args) > 2 {
				flag, _ := literal(call.Args[1])
				if flag == "-C" {
					i = 3
				}
			}
			if i < len(call.Args) {
				verb, plain := literal(call.Args[i])
				if !plain || !familyName.MatchString(verb) {
					ok = false
					return false
				}
				shape += ":" + verb
			}
		}
		parts = append(parts, shape)
		return true
	})
	return strings.Join(parts, "/"), ok && len(parts) > 0
}

// AggregateFailureFamilies groups complete shell command-program shapes, not
// an inferred failing subcommand. Arguments, paths, errors and credentials are
// excluded from identity. Unsupported execution wrappers remain UNKNOWN.
func AggregateFailureFamilies(desires []model.Desire, window time.Duration, end time.Time) FamilyReport {
	r := FamilyReport{Families: []FailureFamily{}, WindowSeconds: int64(window / time.Second)}
	groups := map[string][]time.Time{}
	info := map[string]FailureFamily{}
	for _, d := range desires {
		if d.Category == model.CategoryTurnPattern {
			r.Excluded++
			continue
		}
		if !familyTool.MatchString(d.ToolName) {
			r.Unknown++
			continue
		}
		shape := d.ToolName
		if shape == "Bash" || shape == "exec_command" || shape == "functions.exec_command" {
			var ok bool
			shape, ok = commandShape(d.ToolInput)
			if !ok {
				r.Unknown++
				continue
			}
		}
		class := failureClass(d.Error)
		sum := sha256.Sum256([]byte(d.ToolName + "\x00" + shape + "\x00" + class))
		id := hex.EncodeToString(sum[:])
		groups[id] = append(groups[id], d.Timestamp)
		info[id] = FailureFamily{ID: id, Tool: d.ToolName, Class: class}
	}
	for id, times := range groups {
		sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
		f := info[id]
		f.Count = len(times)
		for _, stamp := range times {
			if stamp.After(end.Add(-window)) && stamp.Before(end) {
				f.Current++
			}
		}
		left := 0
		for right, stamp := range times {
			for left < right && !times[left].After(stamp.Add(-window)) {
				left++
			}
			if n := right - left + 1; n > f.Peak {
				f.Peak = n
			}
		}
		r.Families = append(r.Families, f)
	}
	sort.Slice(r.Families, func(i, j int) bool {
		if r.Families[i].Count != r.Families[j].Count {
			return r.Families[i].Count > r.Families[j].Count
		}
		return r.Families[i].ID < r.Families[j].ID
	})
	return r
}
