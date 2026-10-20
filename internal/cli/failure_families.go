package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/scbrown/desire-path/internal/analyze"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

func init() {
	var since, until string
	var window time.Duration
	var maxRecords int
	cmd := &cobra.Command{
		Args: cobra.NoArgs,
		Use:  "failure-families", Short: "Measure bounded repeated failure families",
		Long:    "Group recorded failures by tool, shell command-program shape and bounded error class. Output omits commands, arguments and errors. Unsupported command identities are counted as UNKNOWN. Turn-pattern observations are excluded. Peak counts use rolling windows, not clock-hour buckets. Refuses rather than returning partial counts when --max-records is exceeded.",
		Example: "  dp failure-families --since 2026-01-01T00:00:00Z --until 2026-01-08T00:00:00Z --window 1h --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if since == "" || until == "" || window <= 0 {
				return fmt.Errorf("positive --window and explicit --since/--until required")
			}
			start, err := time.Parse(time.RFC3339Nano, since)
			if err != nil {
				return err
			}
			end, err := time.Parse(time.RFC3339Nano, until)
			if err != nil {
				return err
			}
			if !end.After(start) {
				return fmt.Errorf("until must follow since")
			}
			s, err := openReadOnlyStore()
			if err != nil {
				return err
			}
			defer s.Close()
			r, err := boundedFailureFamilies(context.Background(), s, start, end, window, maxRecords)
			if err != nil {
				return err
			}
			if jsonOutput {
				return json.NewEncoder(os.Stdout).Encode(r)
			}
			table := NewTable(os.Stdout, "FAMILY", "TOOL", "CLASS", "COUNT", "PEAK")
			for _, f := range r.Families {
				table.Row(f.ID, f.Tool, f.Class, fmt.Sprint(f.Count), fmt.Sprint(f.Peak))
			}
			table.Flush()
			fmt.Fprintf(os.Stderr, "UNKNOWN=%d excluded=%d window_seconds=%d\n", r.Unknown, r.Excluded, r.WindowSeconds)
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "inclusive RFC3339 start")
	cmd.Flags().StringVar(&until, "until", "", "exclusive RFC3339 end")
	cmd.Flags().DurationVar(&window, "window", time.Hour, "rolling count window")
	cmd.Flags().IntVar(&maxRecords, "max-records", 10000, "maximum candidate records (1..100000); overflow refuses without partial output")
	rootCmd.AddCommand(cmd)
}

// boundedFamilyReport identifies the exact measured interval. Complete is true
// only after the candidate inventory fit the requested cap; Unknown still
// describes normalization coverage, not proof of an absent failure family.
type boundedFamilyReport struct {
	analyze.FamilyReport
	Since        string `json:"since"`
	Until        string `json:"until"`
	MaxRecords   int    `json:"max_records"`
	InputRecords int    `json:"input_records"`
	Complete     bool   `json:"complete"`
}

func boundedFailureFamilies(ctx context.Context, s store.Store, start, end time.Time, window time.Duration, maxRecords int) (boundedFamilyReport, error) {
	if maxRecords < 1 || maxRecords > 100000 {
		return boundedFamilyReport{}, fmt.Errorf("max-records must be between 1 and 100000")
	}
	// Limit in the existing store query, before materializing or parsing inputs.
	// One sentinel distinguishes a complete inventory from a truncated one. The
	// store has no Until selector: future records conservatively consume this
	// cap too, rather than allowing them to hide an overfull source interval.
	rows, err := s.ListDesires(ctx, store.ListOpts{Since: start, Limit: maxRecords + 1})
	if err != nil {
		return boundedFamilyReport{}, err
	}
	if len(rows) > maxRecords {
		return boundedFamilyReport{}, fmt.Errorf("failure-family candidate record cap exceeded; counts UNKNOWN")
	}
	filtered := rows[:0]
	for _, row := range rows {
		if !row.Timestamp.Before(start) && row.Timestamp.Before(end) {
			filtered = append(filtered, row)
		}
	}
	return boundedFamilyReport{
		FamilyReport: analyze.AggregateFailureFamilies(filtered, window, end),
		Since:        start.UTC().Format(time.RFC3339Nano), Until: end.UTC().Format(time.RFC3339Nano),
		MaxRecords: maxRecords, InputRecords: len(filtered), Complete: true,
	}, nil
}
