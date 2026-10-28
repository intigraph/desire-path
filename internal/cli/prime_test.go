package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
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
	if state.Status != "absent" || state.Summary.Reason != "database_absent" {
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
	if state.Status != "ok" || state.Summary.Bytes != int64(len(original)) || state.Summary.TotalDesires != nil {
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
	if s := primeSummary(context.Background(), opts); s.Status != "absent" {
		t.Fatalf("config ignored: %+v", s)
	}
	opts.dbOverride = true
	if s := primeSummary(context.Background(), opts); s.Status != "ok" {
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
		{"counts", `{"total_desires":7,"unique_paths":2,"tool_input":"private"}`, 200, "ok"},
		{"zero", `{"total_desires":0,"unique_paths":0}`, 200, "ok"},
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
		if time.Since(started) >= time.Second || state.Status != "partial" || state.ElapsedMS < 0 || state.ElapsedMS >= 1000 {
			t.Fatalf("deadline failed: %+v %s", state, time.Since(started))
		}
	})
}

// primeTestRoot isolates Cobra flags from the reused global command tree.
func primeTestRoot(t *testing.T) *cobra.Command {
	t.Helper()
	oldDB, oldJSON, oldConfig := dbPath, jsonOutput, configPath
	t.Cleanup(func() { dbPath, jsonOutput, configPath = oldDB, oldJSON, oldConfig })
	configPath = filepath.Join(t.TempDir(), "missing-config.toml")
	root := &cobra.Command{Use: "dp", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().StringVar(&dbPath, "db", defaultDBPath(), "database")
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "JSON")
	root.AddCommand(newPrimeCommand())
	return root
}

func TestPrimeCLIContractFormatsSectionsAndUsage(t *testing.T) {
	cases := []struct {
		name string
		args []string
		code int
	}{
		{"missing", []string{"--json"}, 0},
		{"static_json", []string{"--static", "--json"}, 0},
		{"brief_json", []string{"--brief", "--json"}, 0},
		{"brief", []string{"--brief"}, 0},
		{"brief_static", []string{"--brief", "--static"}, 0},
		{"section", []string{"--section", "cOrE", "--static"}, 0},
		{"unknown_section", []string{"--section", "nonexistent"}, 2},
		{"bad_flag", []string{"--not-a-flag"}, 2},
		{"extra_argument", []string{"extra"}, 2},
		{"bad_combination", []string{"--brief", "--section", "core"}, 2},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			root := primeTestRoot(t)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"prime"}, tt.args...))
			err := root.Execute()
			if ExitCode(err) != tt.code {
				t.Fatalf("exit=%d error=%v output=%s", ExitCode(err), err, out.String())
			}
			if tt.code != 0 {
				if tt.name == "unknown_section" && !strings.Contains(err.Error(), "What dp is, Agent workflow, Core commands, MCP tools") {
					t.Fatalf("no section list: %v", err)
				}
				return
			}
			text := out.String()
			if strings.Contains(strings.Join(tt.args, " "), "--json") {
				var obj map[string]json.RawMessage
				if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"schema", "tool", "version", "primer", "live", "status"} {
					if _, ok := obj[key]; !ok {
						t.Fatalf("missing %s: %s", key, text)
					}
				}
				var live map[string]json.RawMessage
				_ = json.Unmarshal(obj["live"], &live)
				for _, key := range []string{"status", "elapsed_ms", "summary"} {
					if _, ok := live[key]; !ok {
						t.Fatalf("missing live.%s: %s", key, text)
					}
				}
				var value primeOutput
				if err := json.Unmarshal(out.Bytes(), &value); err != nil {
					t.Fatal(err)
				}
				if value.Schema != "stack-prime/1" || value.Tool != "dp" || value.Version == "" || len(value.Primer.Sections) < 5 {
					t.Fatalf("bad contract: %s", text)
				}
				var doc map[string]json.RawMessage
				_ = json.Unmarshal(obj["primer"], &doc)
				_, hasText := doc["text"]
				if hasText == (tt.name == "brief_json") {
					t.Fatalf("brief text presence wrong: %s", text)
				}
				if tt.name == "static_json" {
					if value.Status != "ok" || value.Live.Status != "skipped" {
						t.Fatalf("%s", text)
					}
				} else if value.Status != "degraded" || value.Live.Status != "absent" {
					t.Fatalf("%s", text)
				}
			} else if strings.HasPrefix(tt.name, "brief") {
				lines := strings.Split(strings.TrimSpace(text), "\n")
				if len(lines) < 5 || len(lines) > 10 || !strings.HasPrefix(lines[0], "dp ") || lines[len(lines)-1] != "Run `dp prime` for the full primer." {
					t.Fatalf("bad brief (%d): %s", len(lines), text)
				}
			} else if tt.name == "section" {
				if !strings.HasPrefix(text, "## Core commands") || strings.Contains(text, "## MCP tools") || strings.Contains(text, "## Live state") {
					t.Fatalf("bad selection: %s", text)
				}
			}
		})
	}
	if ExitCode(errors.New("ordinary failure")) != 1 {
		t.Fatal("changed other CLI exit codes")
	}
}

func TestPrimeRefusedServerDiffersFromDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte("store_mode = 'remote'\nremote_url = 'http://"+addr+"'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	state := primeSummary(context.Background(), primeOptions{configPath: cfg})
	if state.Status != "unreachable" || state.ElapsedMS >= 1000 || !strings.Contains(state.Summary.URL, addr) {
		t.Fatalf("%+v", state)
	}
	// Credential-bearing URLs are refused and the display value is scrubbed.
	state = primeRemoteState(context.Background(), "https://user:private@example.org/base?token=private#private")
	data, _ := json.Marshal(state)
	if state.Status != "unreadable" || bytes.Contains(data, []byte("private")) || bytes.Contains(data, []byte("user:")) {
		t.Fatalf("%s", data)
	}
}

func TestPrimePlainNamesTargetAndNextCommand(t *testing.T) {
	root := primeTestRoot(t)
	path := filepath.Join(t.TempDir(), "missing's.db")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"prime", "--db", path})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "## Live state") || !strings.Contains(out.String(), path) || !strings.Contains(out.String(), "State: absent") || !strings.Contains(out.String(), "Next: dp --db ") {
		t.Fatalf("%s", out.String())
	}
	if !strings.Contains(primeShellQuote(path), "'\"'\"'") {
		t.Fatal("path not shell-quoted")
	}
}
