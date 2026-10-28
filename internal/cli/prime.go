package cli

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
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

// primeState deliberately excludes stored payloads, configuration secrets and errors.
type primeState struct {
	Status       string `json:"status"`
	Mode         string `json:"mode,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Bytes        int64  `json:"bytes,omitempty"`
	Modified     string `json:"modified,omitempty"`
	TotalDesires *int64 `json:"total_desires,omitempty"`
	UniquePaths  *int64 `json:"unique_paths,omitempty"`
}

type primeOptions struct {
	configPath, dbPath string
	dbOverride         bool
}

func init() {
	var brief, offline bool
	cmd := &cobra.Command{
		Use:     "prime",
		Short:   "Orient agents with a primer and bounded read-only state",
		Long:    "Print the embedded agent primer followed by a read-only live summary. Local mode only stats the existing database file; remote mode makes one bounded GET. Missing or unreadable state keeps the static primer available. No initialization, migration or hook installation is performed.",
		Example: "  dp prime\n  dp prime --brief --json\n  dp prime --static",
		Args:    cobra.NoArgs,
		// Keep all configuration reads inside the live deadline; --static reads none.
		PersistentPreRun: func(*cobra.Command, []string) {},
		RunE: func(cmd *cobra.Command, args []string) error {
			primer := primePrimer
			if brief {
				primer = strings.SplitN(primer, "## Core commands", 2)[0]
			}
			state := primeState{Status: "skipped", Reason: "static_only"}
			if !offline {
				state = primeSummary(cmd.Context(), primeOptions{configPath, dbPath, cmd.Flags().Changed("db")})
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					Tool   string     `json:"tool"`
					Primer string     `json:"primer"`
					Live   primeState `json:"live"`
				}{"dp", primer, state})
			}
			if _, err := fmt.Fprint(cmd.OutOrStdout(), primer); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "\nLive summary: %s", state.Status)
			if err != nil {
				return err
			}
			if state.Mode == "local" && state.Status == "available" {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), " (local file metadata only: %d bytes; modified %s)", state.Bytes, state.Modified)
			} else if state.Status == "available" && state.TotalDesires != nil {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), " (remote: %d desires; %d unique paths)", *state.TotalDesires, *state.UniquePaths)
			} else if state.Reason != "" {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), " (%s)", state.Reason)
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout())
			return err
		},
	}
	cmd.Flags().BoolVar(&brief, "brief", false, "short primer for stack-wide aggregation")
	cmd.Flags().BoolVar(&offline, "static", false, "only print static guidance; skip configuration and state reads")
	rootCmd.AddCommand(cmd)
}

func primeSummary(parent context.Context, opts primeOptions) primeState {
	ctx, cancel := context.WithTimeout(parent, primeDeadline)
	defer cancel()
	result := make(chan primeState, 1)
	go func() { result <- readPrimeState(ctx, opts) }()
	select {
	case state := <-result:
		return state
	case <-ctx.Done():
		return primeState{Status: "unreachable", Reason: "deadline"}
	}
}

func readPrimeState(ctx context.Context, opts primeOptions) primeState {
	cfg, err := config.LoadFrom(opts.configPath)
	if err != nil {
		return primeState{Status: "unreadable", Reason: "configuration"}
	}
	if cfg.StoreMode == "remote" {
		return primeRemoteState(ctx, cfg.RemoteURL)
	}
	path := opts.dbPath
	if !opts.dbOverride && cfg.DBPath != "" {
		path = cfg.DBPath
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return primeState{Status: "unreachable", Mode: "local", Reason: "database_absent"}
	}
	if err != nil || !info.Mode().IsRegular() {
		return primeState{Status: "unreadable", Mode: "local", Reason: "database_metadata"}
	}
	return primeState{Status: "available", Mode: "local", Bytes: info.Size(), Modified: info.ModTime().UTC().Format(time.RFC3339Nano)}
}

func primeRemoteState(ctx context.Context, base string) primeState {
	bad := primeState{Status: "unreadable", Mode: "remote", Reason: "response"}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		bad.Reason = "remote_configuration"
		return bad
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/stats"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return bad
	}
	client := &http.Client{Timeout: primeDeadline, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return primeState{Status: "unreachable", Mode: "remote", Reason: "request"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return bad
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 {
		return bad
	}
	var counts struct {
		Total  *int64 `json:"total_desires"`
		Unique *int64 `json:"unique_paths"`
	}
	if json.Unmarshal(body, &counts) != nil || counts.Total == nil || counts.Unique == nil || *counts.Total < 0 || *counts.Unique < 0 || *counts.Unique > *counts.Total {
		return bad
	}
	return primeState{Status: "available", Mode: "remote", TotalDesires: counts.Total, UniquePaths: counts.Unique}
}
