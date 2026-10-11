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
	cmd := &cobra.Command{
		Args: cobra.NoArgs,
		Use:  "failure-families", Short: "Measure bounded repeated failure families",
		Long:    "Group recorded failures by tool, shell command-program shape and bounded error class. Output omits commands, arguments and errors. Unsupported command identities are counted as UNKNOWN. Turn-pattern observations are excluded. Peak counts use rolling windows, not clock-hour buckets.",
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
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			rows, err := s.ListDesires(context.Background(), store.ListOpts{Since: start})
			if err != nil {
				return err
			}
			filtered := rows[:0]
			for _, row := range rows {
				if !row.Timestamp.Before(start) && row.Timestamp.Before(end) {
					filtered = append(filtered, row)
				}
			}
			r := analyze.AggregateFailureFamilies(filtered, window, end)
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
	rootCmd.AddCommand(cmd)
}
