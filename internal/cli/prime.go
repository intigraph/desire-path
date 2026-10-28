package cli

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/scbrown/desire-path/internal/config"
	"github.com/spf13/cobra"
)

// primePrimer is the single source for CLI, book and skill orientation.
//
//go:embed primer.md
var primePrimer string

const primeDeadline = 650 * time.Millisecond

type primeDetails struct {
	Mode         string `json:"mode,omitempty"`
	Path         string `json:"path,omitempty"`
	URL          string `json:"url,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Next         string `json:"next,omitempty"`
	Bytes        int64  `json:"bytes,omitempty"`
	Modified     string `json:"modified,omitempty"`
	TotalDesires *int64 `json:"total_desires,omitempty"`
	UniquePaths  *int64 `json:"unique_paths,omitempty"`
}

type primeState struct {
	Status    string       `json:"status"`
	ElapsedMS int64        `json:"elapsed_ms"`
	Summary   primeDetails `json:"summary"`
}

type primeDocument struct {
	Sections []string `json:"sections"`
	Text     string   `json:"text,omitempty"`
}

type primeOutput struct {
	Schema  string        `json:"schema"`
	Tool    string        `json:"tool"`
	Version string        `json:"version"`
	Primer  primeDocument `json:"primer"`
	Live    primeState    `json:"live"`
	Status  string        `json:"status"`
}

type primeOptions struct {
	configPath, dbPath string
	dbOverride         bool
	target             chan primeDetails
}

type primeUsageError struct{ error }

// ExitCode preserves ordinary CLI failures while marking prime usage errors as 2.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var usage primeUsageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}

func init() { rootCmd.AddCommand(newPrimeCommand()) }

// newPrimeCommand also gives conformance tests a fresh command and flag state.
func newPrimeCommand() *cobra.Command {
	var brief, offline bool
	var section string
	cmd := &cobra.Command{
		Use:     "prime",
		Short:   "Orient agents with a primer and bounded read-only state",
		Long:    "Print the embedded primer and bounded read-only live state. Local mode only reads existing file metadata; remote mode makes one GET. --static skips live/config reads. --section selects a heading prefix; usage errors exit 2.",
		Example: "  dp prime\n  dp prime --brief --json\n  dp prime --section core --static",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return primeUsageError{err}
			}
			return nil
		},
		// Keep configuration reads inside the live deadline, and skip them for --static.
		PersistentPreRun: func(*cobra.Command, []string) {},
		SilenceUsage:     true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("section") && strings.TrimSpace(section) == "" {
				return primeUsageError{fmt.Errorf("--section needs a heading prefix")}
			}
			if brief && cmd.Flags().Changed("section") {
				return primeUsageError{fmt.Errorf("--brief and --section cannot be combined")}
			}
			text, sections, err := primeSection(primePrimer, section)
			if err != nil {
				return primeUsageError{err}
			}
			state := primeState{Status: "skipped", Summary: primeDetails{}}
			if !offline {
				state = primeSummary(cmd.Context(), primeOptions{configPath: configPath, dbPath: dbPath, dbOverride: cmd.Flags().Changed("db")})
			}
			version := Version
			if version == "" {
				version = "dev"
			}
			status := "ok"
			if state.Status != "ok" && state.Status != "skipped" {
				status = "degraded"
			}
			if jsonOutput {
				if brief {
					text = ""
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(primeOutput{"stack-prime/1", "dp", version, primeDocument{sections, text}, state, status})
			}
			if brief {
				return printPrimeBrief(cmd.OutOrStdout(), version, state)
			}
			if _, err := io.WriteString(cmd.OutOrStdout(), text); err != nil {
				return err
			}
			if !offline {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "\n## Live state\n\n%s\n", strings.Join(primeLiveLines(state), "\n"))
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&brief, "brief", false, "5 to 10 lines for stack aggregation")
	cmd.Flags().BoolVar(&offline, "static", false, "only print embedded guidance; skip config and state reads")
	cmd.Flags().StringVar(&section, "section", "", "one section by case-insensitive heading prefix")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return primeUsageError{err} })
	return cmd
}

func primeSection(primer, wanted string) (string, []string, error) {
	sections := []string{}
	blocks := []string{}
	for _, block := range strings.Split(primer, "\n## ")[1:] {
		heading := strings.SplitN(block, "\n", 2)[0]
		sections = append(sections, heading)
		blocks = append(blocks, "## "+block)
	}
	if wanted == "" {
		return primer, sections, nil
	}
	for i, heading := range sections {
		if strings.HasPrefix(strings.ToLower(heading), strings.ToLower(strings.TrimSpace(wanted))) {
			return blocks[i], sections, nil
		}
	}
	return "", sections, fmt.Errorf("unknown primer section %q; sections: %s", wanted, strings.Join(sections, ", "))
}

func primeLiveLines(state primeState) []string {
	s := state.Summary
	target := "Configuration: " + s.Path
	if s.Mode == "local" {
		target = "Store: " + s.Path
	}
	if s.Mode == "remote" {
		target = "Server: " + s.URL
	}
	lines := []string{target, "State: " + state.Status}
	if s.Reason != "" {
		lines[1] += " (" + s.Reason + ")"
	}
	if state.Status == "ok" && s.Mode == "local" {
		lines = append(lines, fmt.Sprintf("File metadata only: %d bytes; modified %s", s.Bytes, s.Modified))
	}
	if state.Status == "ok" && s.TotalDesires != nil {
		lines = append(lines, fmt.Sprintf("Desires: %d; unique paths: %d", *s.TotalDesires, *s.UniquePaths))
	}
	if s.Next != "" {
		lines = append(lines, "Next: "+s.Next)
	}
	for i := range lines {
		lines[i] = strings.NewReplacer("\n", "\\n", "\r", "\\r").Replace(lines[i])
	}
	return append(lines, fmt.Sprintf("Read: %d ms", state.ElapsedMS))
}

func printPrimeBrief(w io.Writer, version string, state primeState) error {
	lines := []string{"dp " + version + ": records failed tool calls and reveals repeated mistakes"}
	if state.Status == "skipped" {
		what, _, _ := primeSection(primePrimer, "What dp is")
		for _, line := range strings.Split(what, "\n")[1:] {
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
	} else {
		lines = append(lines, primeLiveLines(state)...)
	}
	lines = append(lines, "Run `dp prime` for the full primer.")
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}

func primeSummary(parent context.Context, opts primeOptions) primeState {
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, primeDeadline)
	defer cancel()
	result := make(chan primeState, 1)
	opts.target = make(chan primeDetails, 1)
	go func() { result <- readPrimeState(ctx, opts) }()
	var state primeState
	select {
	case state = <-result:
	case <-ctx.Done():
		details := primeDetails{Mode: "configuration", Path: opts.configPath, Next: "dp config"}
		select {
		case details = <-opts.target:
		default:
		}
		state = primeState{Status: "partial", Summary: details}
	}
	if ctx.Err() != nil {
		state.Status = "partial"
		state.Summary.Reason = "deadline"
	}
	state.ElapsedMS = time.Since(started).Milliseconds()
	return state
}

func readPrimeState(ctx context.Context, opts primeOptions) primeState {
	cfg, err := config.LoadFrom(opts.configPath)
	if err != nil {
		return primeState{Status: "unreadable", Summary: primeDetails{Mode: "configuration", Path: opts.configPath, Reason: "configuration", Next: "dp config"}}
	}
	if cfg.StoreMode == "remote" {
		details := primeDetails{Mode: "remote", URL: primeDisplayURL(cfg.RemoteURL), Next: "dp serve (on the server host)"}
		if opts.target != nil {
			opts.target <- details
		}
		return primeRemoteState(ctx, cfg.RemoteURL)
	}
	path := opts.dbPath
	if !opts.dbOverride && cfg.DBPath != "" {
		path = cfg.DBPath
	}
	details := primeDetails{Mode: "local", Path: path, Next: "dp --db " + primeShellQuote(path) + " stats"}
	if opts.target != nil {
		opts.target <- details
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		details.Reason = "database_absent"
		return primeState{Status: "absent", Summary: details}
	}
	if err != nil || !info.Mode().IsRegular() {
		details.Reason = "database_metadata"
		details.Next = "dp config db_path '/path/to/readable/desires.db'"
		return primeState{Status: "unreadable", Summary: details}
	}
	details.Bytes = info.Size()
	details.Modified = info.ModTime().UTC().Format(time.RFC3339Nano)
	details.Next = ""
	return primeState{Status: "ok", Summary: details}
}

func primeShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func primeDisplayURL(base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "(invalid configured URL)"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func primeRemoteState(ctx context.Context, base string) primeState {
	details := primeDetails{Mode: "remote", URL: primeDisplayURL(base), Reason: "response", Next: "dp config remote_url"}
	bad := primeState{Status: "unreadable", Summary: details}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		bad.Summary.Reason = "remote_configuration"
		return bad
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/stats"
	u.RawPath = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return bad
	}
	client := &http.Client{Timeout: primeDeadline, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		bad.Status = "unreachable"
		bad.Summary.Reason = "request"
		bad.Summary.Next = "dp serve (on the server host)"
		var timeout net.Error
		if ctx.Err() != nil || (errors.As(err, &timeout) && timeout.Timeout()) {
			bad.Status = "partial"
			bad.Summary.Reason = "deadline"
		}
		return bad
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return bad
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 {
		if ctx.Err() != nil {
			bad.Status = "partial"
			bad.Summary.Reason = "deadline"
		}
		return bad
	}
	var counts struct {
		Total  *int64 `json:"total_desires"`
		Unique *int64 `json:"unique_paths"`
	}
	if json.Unmarshal(body, &counts) != nil || counts.Total == nil || counts.Unique == nil || *counts.Total < 0 || *counts.Unique < 0 || *counts.Unique > *counts.Total {
		return bad
	}
	details.Reason = ""
	details.Next = ""
	details.TotalDesires = counts.Total
	details.UniquePaths = counts.Unique
	return primeState{Status: "ok", Summary: details}
}
