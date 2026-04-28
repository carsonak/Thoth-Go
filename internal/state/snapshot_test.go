package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thoth-go/thoth-go/internal/state"
)

// buildFixtureDir writes a small directory tree to base and returns the path.
func buildFixtureDir(t *testing.T, base string, files map[string]string) string {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("buildFixtureDir MkdirAll: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("buildFixtureDir WriteFile: %v", err)
		}
	}
	return base
}

// readFile is a test helper that reads a file and returns its contents as a string.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readFile %q: %v", path, err)
	}
	return string(data)
}

func TestSaveSnapshot_CreatesFiles(t *testing.T) {
	tmp := t.TempDir()
	src := buildFixtureDir(t, filepath.Join(tmp, "src"), map[string]string{
		"main.go":      "package main\n",
		"sub/helper.go": "package sub\n",
	})
	snapshotRoot := filepath.Join(tmp, "snapshots")

	if err := state.SaveSnapshot("ex-001", src, snapshotRoot); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	got := readFile(t, filepath.Join(snapshotRoot, "ex-001", "main.go"))
	if got != "package main\n" {
		t.Errorf("main.go content = %q, want %q", got, "package main\n")
	}
	got = readFile(t, filepath.Join(snapshotRoot, "ex-001", "sub", "helper.go"))
	if got != "package sub\n" {
		t.Errorf("sub/helper.go content = %q, want %q", got, "package sub\n")
	}
}

func TestSaveSnapshot_OverwritesPrevious(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	snapshotRoot := filepath.Join(tmp, "snapshots")

	// First snapshot — contains "old.go".
	buildFixtureDir(t, src, map[string]string{"old.go": "v1\n"})
	if err := state.SaveSnapshot("ex-001", src, snapshotRoot); err != nil {
		t.Fatalf("first SaveSnapshot: %v", err)
	}

	// Update src: remove old.go, add new.go.
	if err := os.Remove(filepath.Join(src, "old.go")); err != nil {
		t.Fatal(err)
	}
	buildFixtureDir(t, src, map[string]string{"new.go": "v2\n"})

	// Second snapshot — must not contain stale "old.go".
	if err := state.SaveSnapshot("ex-001", src, snapshotRoot); err != nil {
		t.Fatalf("second SaveSnapshot: %v", err)
	}

	if _, err := os.Stat(filepath.Join(snapshotRoot, "ex-001", "old.go")); !os.IsNotExist(err) {
		t.Error("stale old.go should have been removed from snapshot")
	}
	got := readFile(t, filepath.Join(snapshotRoot, "ex-001", "new.go"))
	if got != "v2\n" {
		t.Errorf("new.go = %q, want %q", got, "v2\n")
	}
}

func TestSaveSnapshot_SkipsGitDir(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	buildFixtureDir(t, src, map[string]string{
		"main.go":        "package main\n",
		".git/HEAD":      "ref: refs/heads/main\n",
		".git/config":    "[core]\n",
	})
	snapshotRoot := filepath.Join(tmp, "snapshots")

	if err := state.SaveSnapshot("ex-git", src, snapshotRoot); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	if _, err := os.Stat(filepath.Join(snapshotRoot, "ex-git", ".git")); !os.IsNotExist(err) {
		t.Error(".git directory must not be copied into the snapshot")
	}
}

func TestSaveSnapshot_EmptyExerciseID(t *testing.T) {
	tmp := t.TempDir()
	if err := state.SaveSnapshot("", tmp, tmp); err == nil {
		t.Error("expected error for empty exerciseID, got nil")
	}
}

func TestLoadSnapshot_RestoresFiles(t *testing.T) {
	tmp := t.TempDir()
	src := buildFixtureDir(t, filepath.Join(tmp, "src"), map[string]string{
		"main.go": "package main\nfunc main() {}\n",
	})
	snapshotRoot := filepath.Join(tmp, "snapshots")
	dst := filepath.Join(tmp, "dst")

	if err := state.SaveSnapshot("ex-002", src, snapshotRoot); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if err := state.LoadSnapshot("ex-002", snapshotRoot, dst); err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	got := readFile(t, filepath.Join(dst, "main.go"))
	want := "package main\nfunc main() {}\n"
	if got != want {
		t.Errorf("restored main.go = %q, want %q", got, want)
	}
}

func TestLoadSnapshot_OverwritesExistingFiles(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	snapshotRoot := filepath.Join(tmp, "snapshots")

	buildFixtureDir(t, src, map[string]string{"main.go": "original\n"})
	if err := state.SaveSnapshot("ex-003", src, snapshotRoot); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Mutate dst before loading.
	buildFixtureDir(t, dst, map[string]string{"main.go": "mutated\n"})

	if err := state.LoadSnapshot("ex-003", snapshotRoot, dst); err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	got := readFile(t, filepath.Join(dst, "main.go"))
	if got != "original\n" {
		t.Errorf("main.go = %q after load, want %q", got, "original\n")
	}
}

func TestLoadSnapshot_MissingSnapshot(t *testing.T) {
	tmp := t.TempDir()
	err := state.LoadSnapshot("nonexistent", tmp, filepath.Join(tmp, "dst"))
	if err == nil {
		t.Fatal("expected error for missing snapshot, got nil")
	}
}

func TestLoadSnapshot_EmptyExerciseID(t *testing.T) {
	tmp := t.TempDir()
	if err := state.LoadSnapshot("", tmp, tmp); err == nil {
		t.Error("expected error for empty exerciseID, got nil")
	}
}

func TestDefaultSnapshotDir_ContainsThothGo(t *testing.T) {
	dir, err := state.DefaultSnapshotDir()
	if err != nil {
		t.Fatalf("DefaultSnapshotDir: %v", err)
	}
	if dir == "" {
		t.Fatal("DefaultSnapshotDir returned empty string")
	}
	// Must be rooted under ~/.thoth-go/snapshots.
	if base := filepath.Base(dir); base != "snapshots" {
		t.Errorf("base = %q, want %q", base, "snapshots")
	}
	parent := filepath.Base(filepath.Dir(dir))
	if parent != ".thoth-go" {
		t.Errorf("parent = %q, want %q", parent, ".thoth-go")
	}
}
