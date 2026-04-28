package repository_test

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/thoth-go/thoth-go/internal/repository"
)

// buildZip creates an in-memory zip archive from a map of relative-path →
// content entries and returns the raw bytes.
func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("buildZip Create %q: %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("buildZip Write %q: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("buildZip Close: %v", err)
	}
	return buf.Bytes()
}

// newTestServer returns an httptest server that serves the given topic zip once
// and records how many times it was called.
func newTestServer(t *testing.T, topic string, zipData []byte) (*httptest.Server, *int) {
	t.Helper()
	calls := new(int)
	mux := http.NewServeMux()
	mux.HandleFunc("/topics/"+topic+".zip", func(w http.ResponseWriter, r *http.Request) {
		*calls++
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(zipData)
	})
	return httptest.NewServer(mux), calls
}

// ── Fetch ────────────────────────────────────────────────────────────────────

func TestFetch_DownloadsAndExtractsFiles(t *testing.T) {
	zipData := buildZip(t, map[string]string{
		"loops-001/exercise.yaml": "id: loops-001\n",
		"loops-001/main.go":       "package main\n",
	})

	srv, _ := newTestServer(t, "loops", zipData)
	defer srv.Close()

	cacheDir := t.TempDir()
	m := repository.NewManager(cacheDir, srv.URL, nil)

	if err := m.Fetch("loops", false); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// Verify extracted files.
	got, err := os.ReadFile(filepath.Join(cacheDir, "loops", "loops-001", "exercise.yaml"))
	if err != nil {
		t.Fatalf("reading cached exercise.yaml: %v", err)
	}
	if string(got) != "id: loops-001\n" {
		t.Errorf("exercise.yaml = %q, want %q", got, "id: loops-001\n")
	}
}

func TestFetch_CacheHit_SkipsDownload(t *testing.T) {
	zipData := buildZip(t, map[string]string{"loops-001/main.go": "package main\n"})
	srv, calls := newTestServer(t, "loops", zipData)
	defer srv.Close()

	cacheDir := t.TempDir()
	m := repository.NewManager(cacheDir, srv.URL, nil)

	// Prime the cache by creating the topic directory manually.
	if err := os.MkdirAll(filepath.Join(cacheDir, "loops"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := m.Fetch("loops", false); err != nil {
		t.Fatalf("Fetch (cache hit): %v", err)
	}
	if *calls != 0 {
		t.Errorf("server called %d times, want 0 (cache hit)", *calls)
	}
}

func TestFetch_Force_ReDownloads(t *testing.T) {
	zipData := buildZip(t, map[string]string{"loops-001/main.go": "package main\n"})
	srv, calls := newTestServer(t, "loops", zipData)
	defer srv.Close()

	cacheDir := t.TempDir()
	m := repository.NewManager(cacheDir, srv.URL, nil)

	// First download.
	if err := m.Fetch("loops", false); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	// Force re-download.
	if err := m.Fetch("loops", true); err != nil {
		t.Fatalf("forced Fetch: %v", err)
	}
	if *calls != 2 {
		t.Errorf("server called %d times, want 2", *calls)
	}
}

func TestFetch_ServerError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	m := repository.NewManager(t.TempDir(), srv.URL, nil)
	if err := m.Fetch("unknown-topic", false); err == nil {
		t.Fatal("expected error from 404 response, got nil")
	}
}

func TestFetch_EmptyTopic_ReturnsError(t *testing.T) {
	m := repository.NewManager(t.TempDir(), "http://localhost", nil)
	if err := m.Fetch("", false); err == nil {
		t.Fatal("expected error for empty topic, got nil")
	}
}

func TestFetch_ZipSlipRejected(t *testing.T) {
	// Craft a zip containing a path-traversal entry.
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("../../evil.txt")
	_, _ = f.Write([]byte("owned"))
	_ = w.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	m := repository.NewManager(t.TempDir(), srv.URL, nil)
	if err := m.Fetch("evil", false); err == nil {
		t.Fatal("expected zip-slip error, got nil")
	}
}

// ── Reset ────────────────────────────────────────────────────────────────────

func TestReset_CopiesCachedFilesToDst(t *testing.T) {
	cacheDir := t.TempDir()
	// Manually populate cache: cacheDir/loops/loops-001/
	exerciseCache := filepath.Join(cacheDir, "loops", "loops-001")
	if err := os.MkdirAll(exerciseCache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exerciseCache, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	m := repository.NewManager(cacheDir, "http://localhost", nil)

	if err := m.Reset("loops-001", dst); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dst, "main.go"))
	if err != nil {
		t.Fatalf("reading reset file: %v", err)
	}
	if string(got) != "package main\n" {
		t.Errorf("main.go = %q, want %q", got, "package main\n")
	}
}

func TestReset_ExerciseNotInCache_ReturnsError(t *testing.T) {
	cacheDir := t.TempDir()
	// Populate one topic but not the requested exercise.
	if err := os.MkdirAll(filepath.Join(cacheDir, "loops", "loops-001"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := repository.NewManager(cacheDir, "http://localhost", nil)
	if err := m.Reset("nonexistent-999", t.TempDir()); err == nil {
		t.Fatal("expected error for missing exercise, got nil")
	}
}

func TestReset_CacheDirMissing_ReturnsError(t *testing.T) {
	m := repository.NewManager("/nonexistent/cache/dir", "http://localhost", nil)
	if err := m.Reset("loops-001", t.TempDir()); err == nil {
		t.Fatal("expected error for missing cache dir, got nil")
	}
}

func TestReset_EmptyExerciseID_ReturnsError(t *testing.T) {
	m := repository.NewManager(t.TempDir(), "http://localhost", nil)
	if err := m.Reset("", t.TempDir()); err == nil {
		t.Fatal("expected error for empty exerciseID, got nil")
	}
}

func TestDefaultCacheDir_ContainsThothGo(t *testing.T) {
	dir, err := repository.DefaultCacheDir()
	if err != nil {
		t.Fatalf("DefaultCacheDir: %v", err)
	}
	if dir == "" {
		t.Fatal("DefaultCacheDir returned empty string")
	}
	if base := filepath.Base(dir); base != "topics" {
		t.Errorf("base = %q, want %q", base, "topics")
	}
	if parent := filepath.Base(filepath.Dir(dir)); parent != "cache" {
		t.Errorf("parent = %q, want %q", parent, "cache")
	}
	if grandparent := filepath.Base(filepath.Dir(filepath.Dir(dir))); grandparent != ".thoth-go" {
		t.Errorf("grandparent = %q, want %q", grandparent, ".thoth-go")
	}
}
