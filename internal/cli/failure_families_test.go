package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

type cappedFamilyStore struct {
	store.Store
	opts store.ListOpts
	err  error
}

func (s *cappedFamilyStore) ListDesires(_ context.Context, opts store.ListOpts) ([]model.Desire, error) {
	s.opts = opts
	return nil, s.err
}

func TestFailureFamilyCapUsesStoreLimitAndPreservesErrors(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	want := errors.New("private store error")
	s := &cappedFamilyStore{err: want}
	_, err := boundedFailureFamilies(context.Background(), s, start, start.Add(time.Hour), time.Hour, 20)
	if !errors.Is(err, want) || s.opts.Limit != 21 || !s.opts.Since.Equal(start) {
		t.Fatalf("missing bounded query/error propagation: opts=%+v err=%v", s.opts, err)
	}
	for _, cap := range []int{-1, 0, 100001} {
		s.opts = store.ListOpts{}
		if _, err := boundedFailureFamilies(context.Background(), s, start, start.Add(time.Hour), time.Hour, cap); err == nil || s.opts.Limit != 0 {
			t.Fatalf("invalid cap reached the store: %d", cap)
		}
	}
}

func TestFailureFamilyReadOnlyExactCapOverflowAndInterval(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	db := filepath.Join(t.TempDir(), "private.db")
	w, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	for i, stamp := range []time.Time{start.Add(-time.Second), start.Add(time.Minute), start.Add(2 * time.Minute)} {
		if err := w.RecordDesire(ctx, model.Desire{ID: string(rune('a' + i)), ToolName: "Read", Error: "permission denied", Timestamp: stamp}); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	before, _ := os.ReadFile(db)
	info, _ := os.Stat(db)
	r, err := store.NewReadOnly(db)
	if err != nil {
		t.Fatal(err)
	}
	report, err := boundedFailureFamilies(ctx, r, start, end, time.Hour, 2)
	if err != nil || !report.Complete || report.InputRecords != 2 || report.MaxRecords != 2 || len(report.Families) != 1 || report.Families[0].Current != 2 {
		t.Fatalf("exact cap not complete: %+v %v", report, err)
	}
	if report.Since != "2026-01-01T00:00:00Z" || report.Until != "2026-01-01T01:00:00Z" {
		t.Fatalf("missing source window: %+v", report)
	}
	if _, err := boundedFailureFamilies(ctx, r, start, end, time.Hour, 1); err == nil {
		t.Fatal("overflow returned partial counts")
	}
	if err := r.RecordDesire(ctx, model.Desire{ID: "forbidden", ToolName: "Read", Timestamp: start}); err == nil {
		t.Fatal("report reader allowed mutation")
	}
	r.Close()
	after, _ := os.ReadFile(db)
	newInfo, _ := os.Stat(db)
	if !bytes.Equal(before, after) || !info.ModTime().Equal(newInfo.ModTime()) {
		t.Fatal("report changed database bytes or mtime")
	}
	// Future candidates consume the cap conservatively. They never hide overflow
	// or appear in the explicitly identified report interval.
	w, err = store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.RecordDesire(ctx, model.Desire{ID: "future", ToolName: "Read", Timestamp: end}); err != nil {
		t.Fatal(err)
	}
	w.Close()
	r, err = store.NewReadOnly(db)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := boundedFailureFamilies(ctx, r, start, end, time.Hour, 2); err == nil {
		t.Fatal("future record hid an incomplete candidate inventory")
	}
	report, err = boundedFailureFamilies(ctx, r, start, end, time.Hour, 3)
	if err != nil || report.InputRecords != 2 {
		t.Fatalf("exclusive end not preserved: %+v %v", report, err)
	}
	encoded, _ := json.Marshal(report)
	if bytes.Contains(encoded, []byte("permission denied")) || bytes.Contains(encoded, []byte("private.db")) {
		t.Fatal("raw errors or paths escaped report")
	}
}
