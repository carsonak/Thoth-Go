package repository_test

// e2e_test.go: end-to-end test that exercises the full thoth-go pipeline
// using the local testdata curriculum without requiring a remote server.
//
// # What this tests
//
// The test simulates the three CLI commands a learner would run:
//
//  1. thoth-go fetch basics   → loads the packed zip into a local cache
//  2. thoth-go start <id>     → copies exercise files to a working dir
//  3. thoth-go check          → runs the engine over the working dir
//
// By driving these steps programmatically, we can verify end-to-end behaviour
// without spawning subprocesses or needing a real HTTP server.
//
// # Why here (repository package)?
//
// The repository package already depends on archive/zip and os. Placing the
// E2E test here keeps the dependency footprint small — we import only the
// packages that are actually exercised (repository, engine). Putting the test
// in a top-level e2e/ package would work too, but the test would need to be
// explicitly included with build tags; putting it here means `go test ./...`
// always covers it.
//
// # Local HTTP server pattern
//
// repository.Manager.Fetch downloads from a URL. We use httptest.NewServer
// to serve the local packed zip over HTTP so we can test Fetch without
// modifying the Manager's public API or introducing a "load from file"
// shortcut. This is the standard Go pattern for testing HTTP clients: inject
// a test server whose URL you control.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/thoth-go/thoth-go/internal/checker/engine"
	"github.com/thoth-go/thoth-go/internal/repository"
)

// packedZipPath returns the absolute path to testdata/packed/<topic>.zip,
// resolving relative to this test file's directory.
func packedZipPath(t *testing.T, topic string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = .../internal/repository/e2e_test.go
	// zip is at: .../testdata/packed/<topic>.zip
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	zipPath := filepath.Join(root, "testdata", "packed", topic+".zip")
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		t.Skipf("packed zip %q not found — run ./scripts/pack-topics.sh first", zipPath)
	}
	return zipPath
}

// serveZip starts a local HTTP server that serves a single .zip file at
// /topics/<topic>.zip and returns the server's base URL.
func serveZip(t *testing.T, topic, zipPath string) string {
	t.Helper()
	data, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("reading zip %q: %v", zipPath, err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/topics/" + topic + ".zip"
		if r.URL.Path != want {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		if _, writeErr := w.Write(data); writeErr != nil {
			t.Errorf("writing response: %v", writeErr)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// setupCache fetches the given topic into a temporary cache directory via a
// local HTTP server that serves the packed zip.
func setupCache(t *testing.T, topic string) (cacheDir string) {
	t.Helper()
	zipPath := packedZipPath(t, topic)
	baseURL := serveZip(t, topic, zipPath)

	cacheDir = t.TempDir()
	mgr := repository.NewManager(cacheDir, baseURL, nil)

	if err := mgr.Fetch(topic, false); err != nil {
		t.Fatalf("Fetch(%q): %v", topic, err)
	}
	return cacheDir
}

// ── E2E tests ─────────────────────────────────────────────────────────────────

// TestE2E_HelloWorld runs the full pipeline for the 01-hello-world exercise.
// Starter code has a TODO placeholder — the engine should report test failure.
// After injecting the correct solution, the engine should report AllPassed=true.
func TestE2E_HelloWorld(t *testing.T) {
	cacheDir := setupCache(t, "basics")
	mgr := repository.NewManager(cacheDir, "", nil)

	// Step 1: Reset exercise to a working directory (simulates `thoth-go start`).
	workDir := t.TempDir()
	if err := mgr.Reset("01-hello-world", workDir); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Step 2: Verify that the starter code (TODO placeholder) fails the test.
	result, err := engine.New(workDir).Run()
	if err != nil {
		t.Fatalf("Run (starter): %v", err)
	}
	if result.AllPassed {
		t.Error("starter code should NOT pass — it has a TODO placeholder")
	}

	// Step 3: Inject the correct solution.
	solution := []byte(`package main

import "fmt"

func main() {
	fmt.Print("Hello, World!\n")
}
`)
	if writeErr := os.WriteFile(filepath.Join(workDir, "main.go"), solution, 0o644); writeErr != nil {
		t.Fatalf("writing solution: %v", writeErr)
	}

	// Step 4: Re-run the engine — should now pass.
	result, err = engine.New(workDir).Run()
	if err != nil {
		t.Fatalf("Run (solution): %v", err)
	}
	if !result.AllPassed {
		for _, tr := range result.TestResults {
			t.Logf("test %d: passed=%v runError=%q", tr.Index, tr.Passed, tr.RunError)
		}
		t.Error("correct solution should AllPassed=true")
	}
}

// TestE2E_Addition runs the full pipeline for the 02-addition exercise.
func TestE2E_Addition(t *testing.T) {
	cacheDir := setupCache(t, "basics")
	mgr := repository.NewManager(cacheDir, "", nil)

	workDir := t.TempDir()
	if err := mgr.Reset("02-addition", workDir); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Starter code returns 0 — should fail.
	result, err := engine.New(workDir).Run()
	if err != nil {
		t.Fatalf("Run (starter): %v", err)
	}
	if result.AllPassed {
		t.Error("starter code (return 0) should NOT pass")
	}

	// Inject correct solution.
	solution := []byte(`package addition

func Add(a, b int) int {
	return a + b
}
`)
	if writeErr := os.WriteFile(filepath.Join(workDir, "addition.go"), solution, 0o644); writeErr != nil {
		t.Fatalf("writing solution: %v", writeErr)
	}

	result, err = engine.New(workDir).Run()
	if err != nil {
		t.Fatalf("Run (solution): %v", err)
	}
	if !result.AllPassed {
		for _, tr := range result.TestResults {
			t.Logf("test %d: passed=%v runError=%q", tr.Index, tr.Passed, tr.RunError)
		}
		t.Error("correct Add implementation should AllPassed=true")
	}
}

// TestE2E_FixLoop runs the full pipeline for the 03-fix-loop exercise.
func TestE2E_FixLoop(t *testing.T) {
	cacheDir := setupCache(t, "basics")
	mgr := repository.NewManager(cacheDir, "", nil)

	workDir := t.TempDir()
	if err := mgr.Reset("03-fix-loop", workDir); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Starter code has i <= 3 — test expects 1..5, should fail.
	result, err := engine.New(workDir).Run()
	if err != nil {
		t.Fatalf("Run (starter): %v", err)
	}
	if result.AllPassed {
		t.Error("buggy starter code should NOT pass")
	}

	// Apply the fix: change `i <= 3` to `i <= 5` on line 6.
	fixedMain := []byte(`package main

import "fmt"

func main() {
	for i := 1; i <= 5; i++ {
		fmt.Println(i)
	}
}
`)
	if writeErr := os.WriteFile(filepath.Join(workDir, "main.go"), fixedMain, 0o644); writeErr != nil {
		t.Fatalf("writing fix: %v", writeErr)
	}

	result, err = engine.New(workDir).Run()
	if err != nil {
		t.Fatalf("Run (fix): %v", err)
	}
	if !result.AllPassed {
		for _, tr := range result.TestResults {
			t.Logf("test %d: passed=%v runError=%q", tr.Index, tr.Passed, tr.RunError)
		}
		t.Error("fixed code should AllPassed=true")
	}
}

// TestE2E_FixLoop_WrongLine checks that modifying a line outside the allowed
// zone (RestrictedLines=[6]) produces a structural diff failure.
func TestE2E_FixLoop_WrongLine(t *testing.T) {
	cacheDir := setupCache(t, "basics")
	mgr := repository.NewManager(cacheDir, "", nil)

	workDir := t.TempDir()
	if err := mgr.Reset("03-fix-loop", workDir); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// "Fix" by changing the fmt.Println call (line 7) instead of the condition (line 6).
	// This produces the correct output, but on the wrong line — should be rejected.
	wrongLineMain := []byte(`package main

import "fmt"

func main() {
	for i := 1; i <= 3; i++ {
		fmt.Printf("%d\n", i+2) // "fix" on wrong line
	}
}
`)
	if writeErr := os.WriteFile(filepath.Join(workDir, "main.go"), wrongLineMain, 0o644); writeErr != nil {
		t.Fatalf("writing wrong-line fix: %v", writeErr)
	}

	result, err := engine.New(workDir).Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.AllPassed {
		t.Error("modification on wrong line should NOT pass the structural diff check")
	}
	if len(result.TestResults) == 0 {
		t.Error("expected at least one TestResult describing the violation")
	}
}
