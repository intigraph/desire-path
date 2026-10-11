package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOnlyMissingDoesNotCreate(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "missing")
	if _, err := NewReadOnly(filepath.Join(parent, "data.db")); err == nil {
		t.Fatal("missing database became a readable empty store")
	}
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("reader created parent: %v", err)
	}
}

func TestReadOnlyCurrentSchemaRefusesWrites(t *testing.T) {
	// URI punctuation must be escaped as filename bytes.
	dir := t.TempDir()
	initial := filepath.Join(dir, "data.db")
	path := filepath.Join(dir, "data ? #.db")
	writable, err := New(initial)
	if err != nil {
		t.Fatal(err)
	}
	writable.Close()
	if err := os.Rename(initial, path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	readonly, err := NewReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readonly.ListDesires(context.Background(), ListOpts{}); err != nil {
		t.Fatal(err)
	}
	if _, err := readonly.db.Exec("UPDATE schema_version SET version = 0"); err == nil {
		t.Fatal("read-only store accepted mutation")
	}
	readonly.Close()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("read-only report changed database bytes")
	}
}

func TestReadOnlyIncompatibleSchemaDoesNotMigrate(t *testing.T) {
	for _, version := range []int{schemaVersion - 1, schemaVersion + 1} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "data.db")
			writable, err := New(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := writable.db.Exec("UPDATE schema_version SET version = ?", version); err != nil {
				t.Fatal(err)
			}
			writable.Close()
			before, _ := os.ReadFile(path)
			if _, err := NewReadOnly(path); err == nil {
				t.Fatal("reader accepted incompatible schema")
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("reader migrated incompatible schema")
			}
		})
	}
}
