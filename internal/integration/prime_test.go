//go:build integration

package integration

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func primeTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%v:%d:%d", info.Mode(), info.Size(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += fmt.Sprintf(":%x", sha256.Sum256(body))
		}
		relative, _ := filepath.Rel(root, path)
		snapshot[relative] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestPrimeCompiledContractReadOnlyAndUsage(t *testing.T) {
	for _, flags := range [][]string{{}, {"--static"}, {"--brief"}, {"--brief", "--static"}, {"--json"}, {"--brief", "--json"}, {"--section", "CORE", "--static"}, {"--section", "unknown"}, {"--unknown"}} {
		t.Run(strings.Join(flags, "_"), func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			cwd := filepath.Join(root, "cwd")
			for _, dir := range []string{home, cwd} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			before := primeTree(t, root)
			cmd := exec.Command(dpBin, append([]string{"prime"}, flags...)...)
			cmd.Dir = cwd
			cmd.Env = append(os.Environ(), "HOME="+home)
			started := time.Now()
			out, err := cmd.CombinedOutput()
			if time.Since(started) >= time.Second {
				t.Fatalf("prime exceeded budget: %s", out)
			}
			invalid := strings.Contains(strings.Join(flags, " "), "unknown")
			if invalid {
				exit, ok := err.(*exec.ExitError)
				if !ok || exit.ExitCode() != 2 {
					t.Fatalf("error=%v output=%s", err, out)
				}
			} else {
				if err != nil {
					t.Fatalf("%v: %s", err, out)
				}
				if strings.Contains(strings.Join(flags, " "), "--json") {
					var obj struct {
						Schema, Tool, Version, Status string
						Primer                        struct {
							Sections []string
							Text     *string
						}
						Live struct {
							Status    string
							ElapsedMS int64 `json:"elapsed_ms"`
						}
					}
					if err := json.Unmarshal(out, &obj); err != nil {
						t.Fatal(err)
					}
					if obj.Schema != "stack-prime/1" || obj.Tool != "dp" || obj.Version == "" || len(obj.Primer.Sections) < 5 || obj.Live.Status != "absent" || obj.Status != "degraded" {
						t.Fatalf("%s", out)
					}
					if (obj.Primer.Text == nil) != strings.Contains(strings.Join(flags, " "), "--brief") {
						t.Fatalf("%s", out)
					}
				} else if len(flags) == 0 && (!strings.Contains(string(out), "# Desire Path agent primer") || !strings.Contains(string(out), "State: absent")) {
					t.Fatalf("%s", out)
				}
			}
			if after := primeTree(t, root); !reflect.DeepEqual(before, after) {
				t.Fatalf("prime wrote files or changed metadata: before=%v after=%v", before, after)
			}
		})
	}
}

func TestPrimeCompiledSlowBackendIsPartial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, ".dp"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := "store_mode = 'remote'\nremote_url = '" + server.URL + "'\n"
	if err := os.WriteFile(filepath.Join(home, ".dp/config.toml"), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	before := primeTree(t, root)
	cmd := exec.Command(dpBin, "prime", "--json")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+home)
	started := time.Now()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	var obj struct {
		Status string
		Live   struct {
			Status    string
			ElapsedMS int64 `json:"elapsed_ms"`
		}
	}
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatal(err)
	}
	if obj.Status != "degraded" || obj.Live.Status != "partial" || obj.Live.ElapsedMS < 0 || obj.Live.ElapsedMS >= 1000 || time.Since(started) >= time.Second {
		t.Fatalf("%s duration=%s", out, time.Since(started))
	}
	if after := primeTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("slow-backend read changed HOME/cwd")
	}
}

func TestPrimeCompiledRefusedServerKeepsFullPrimer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := server.URL
	server.Close()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, ".dp"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".dp/config.toml"), []byte("store_mode = 'remote'\nremote_url = '"+base+"'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := primeTree(t, root)
	for _, args := range [][]string{{"prime"}, {"prime", "--json"}} {
		cmd := exec.Command(dpBin, args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "HOME="+home)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		if len(args) == 1 {
			if !strings.Contains(string(out), "# Desire Path agent primer") || !strings.Contains(string(out), "## Next steps") || !strings.Contains(string(out), "State: unreachable") || !strings.Contains(string(out), base) || !strings.Contains(string(out), "Next: dp serve") {
				t.Fatalf("%s", out)
			}
		} else {
			var obj struct {
				Status string
				Primer struct{ Text string }
				Live   struct{ Status string }
			}
			if err := json.Unmarshal(out, &obj); err != nil {
				t.Fatal(err)
			}
			if obj.Status != "degraded" || obj.Live.Status != "unreachable" || !strings.Contains(obj.Primer.Text, "## Next steps") {
				t.Fatalf("%s", out)
			}
		}
	}
	if after := primeTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("refused-server read changed HOME/cwd")
	}
}
