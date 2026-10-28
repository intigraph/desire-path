package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrimeGeneratedCopiesCurrent(t *testing.T) {
	root := filepath.Join("..", "..")
	book, err := os.ReadFile(filepath.Join(root, "docs/book/src/commands/prime.md"))
	if err != nil {
		t.Fatal(err)
	}
	skill, err := os.ReadFile(filepath.Join(root, "skills/desire-path/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	const begin = "<!-- BEGIN dp prime -->\n"
	const end = "<!-- END dp prime -->"
	parts := strings.SplitN(string(skill), begin, 2)
	if len(parts) != 2 {
		t.Fatal("missing skill primer marker")
	}
	body := strings.SplitN(parts[1], end, 2)
	if len(body) != 2 || body[0] != primePrimer || string(book) != primePrimer {
		t.Fatal("primer copies stale: run python3 scripts/sync-primer.py")
	}
}

func TestPrimeLocalMetadataNeverCreatesOrMutates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "absent", "desires.db")
	opts := primeOptions{configPath: filepath.Join(dir, "no-config.toml"), dbPath: path}
	state := primeSummary(context.Background(), opts)
	if state.Status != "unreachable" || state.Reason != "database_absent" {
		t.Fatalf("%+v", state)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("created data directory")
	}
	// A non-SQLite file is still only metadata: never imply valid schema or capture health.
	path = filepath.Join(dir, "existing.db")
	original := []byte("not a sqlite database")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	entriesBefore, _ := os.ReadDir(dir)
	opts.dbPath = path
	state = primeSummary(context.Background(), opts)
	after, _ := os.Stat(path)
	content, _ := os.ReadFile(path)
	entriesAfter, _ := os.ReadDir(dir)
	if state.Status != "available" || state.Bytes != int64(len(original)) || state.TotalDesires != nil {
		t.Fatalf("%+v", state)
	}
	if !bytes.Equal(content, original) || !before.ModTime().Equal(after.ModTime()) || len(entriesBefore) != len(entriesAfter) {
		t.Fatal("metadata read changed file or created sidecars")
	}
	// Config is read without legacy migration; an explicit --db wins.
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte("db_path = 'missing.db'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	opts.configPath = cfg
	if s := primeSummary(context.Background(), opts); s.Status != "unreachable" {
		t.Fatalf("config ignored: %+v", s)
	}
	opts.dbOverride = true
	if s := primeSummary(context.Background(), opts); s.Status != "available" {
		t.Fatalf("override ignored: %+v", s)
	}
	opts.dbPath = dir
	if s := primeSummary(context.Background(), opts); s.Status != "unreadable" {
		t.Fatalf("directory accepted: %+v", s)
	}
}

func TestPrimeRemoteBoundsAndResponseValidation(t *testing.T) {
	cases := []struct {
		name, body string
		code       int
		want       string
	}{
		{"counts", `{"total_desires":7,"unique_paths":2,"tool_input":"private"}`, 200, "available"},
		{"zero", `{"total_desires":0,"unique_paths":0}`, 200, "available"},
		{"missing", `{}`, 200, "unreadable"},
		{"negative", `{"total_desires":-1,"unique_paths":0}`, 200, "unreadable"},
		{"inconsistent", `{"total_desires":1,"unique_paths":2}`, 200, "unreadable"},
		{"malformed", `not json`, 200, "unreadable"},
		{"oversize", strings.Repeat(" ", 65537), 200, "unreadable"},
		{"error", `private error`, 503, "unreadable"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "GET" || r.URL.Path != "/api/v1/stats" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				w.WriteHeader(tt.code)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			state := primeRemoteState(context.Background(), server.URL)
			encoded, _ := json.Marshal(state)
			if state.Status != tt.want || calls.Load() != 1 || bytes.Contains(encoded, []byte("private")) {
				t.Fatalf("%s calls=%d", encoded, calls.Load())
			}
		})
	}
	t.Run("no_redirect", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		}))
		defer server.Close()
		if s := primeRemoteState(context.Background(), server.URL); s.Status != "unreadable" || calls.Load() != 1 {
			t.Fatalf("redirect followed: %+v calls=%d", s, calls.Load())
		}
	})
	t.Run("deadline", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
		defer server.Close()
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config.toml")
		if err := os.WriteFile(cfg, []byte("store_mode = 'remote'\nremote_url = '"+server.URL+"'\n"), 0600); err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		state := primeSummary(context.Background(), primeOptions{configPath: cfg})
		if time.Since(started) >= time.Second || state.Status != "unreachable" {
			t.Fatalf("deadline failed: %+v %s", state, time.Since(started))
		}
	})
}

func TestPrimeStaticCLIAndFormats(t *testing.T) {
	// Save global Cobra state because existing CLI tests reuse the same root.
	oldDB, oldJSON, oldConfig := dbPath, jsonOutput, configPath
	oldOut := rootCmd.OutOrStdout()
	t.Cleanup(func() {
		dbPath, jsonOutput, configPath = oldDB, oldJSON, oldConfig
		rootCmd.SetOut(oldOut)
		rootCmd.SetArgs(nil)
		cmd, _, _ := rootCmd.Find([]string{"prime"})
		_ = cmd.Flags().Set("static", "false")
		_ = cmd.Flags().Set("brief", "false")
		for _, name := range []string{"static", "brief", "db", "json"} {
			if f := cmd.Flags().Lookup(name); f != nil {
				f.Changed = false
			}
		}
	})
	dir := t.TempDir()
	configPath = filepath.Join(dir, "broken.toml")
	if err := os.WriteFile(configPath, []byte("[invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"prime", "--static", "--brief", "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Tool, Primer string
		Live         primeState
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Tool != "dp" || result.Live.Status != "skipped" || strings.Contains(result.Primer, "## Core commands") {
		t.Fatalf("%s", out.String())
	}
	out.Reset()
	rootCmd.SetArgs([]string{"prime", "--static", "--brief=false", "--json=false"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "## Core commands") || !strings.Contains(out.String(), "Live summary: skipped") {
		t.Fatalf("%s", out.String())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("static command wrote state")
	}
}
