package state
package state_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thoth-go/thoth-go/internal/state"
)

// ---- ParseState tests ----

func TestParseState_Empty(t *testing.T) {
	raw := `{"schema_version":1,"exercises":{},"updated_at":"2024-01-01T00:00:00Z"}`
	ps, err := state.ParseState([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ps.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d", ps.SchemaVersion)
	}
	if len(ps.Exercises) != 0 {
		t.Errorf("Exercises len = %d, want 0", len(ps.Exercises))
	}
}

func TestParseState_NilExercisesBecomesMap(t *testing.T) {
	// If the JSON omits "exercises", the map must still be non-nil.
	raw := `{"schema_version":1,"updated_at":"2024-01-01T00:00:00Z"}`
	ps, err := state.ParseState([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ps.Exercises == nil {
		t.Error("Exercises map must not be nil after parse")
	}
}

func TestParseState_InvalidJSON(t *testing.T) {
	_, err := state.ParseState([]byte("{not json}"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ---- ProgressState mutation tests ----

func TestProgressState_NewIsEmpty(t *testing.T) {
	ps := state.NewProgressState()
	if ps.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", ps.SchemaVersion)
	}
	if len(ps.Exercises) != 0 {
		t.Errorf("Exercises not empty on creation")
	}
}

func TestProgressState_MarkStarted(t *testing.T) {
	ps := state.NewProgressState()
	ps.MarkStarted("loops-001")

	r := ps.Exercises["loops-001"]
	if r == nil {
		t.Fatal("record not created")
	}
	if r.Status != state.StatusInProgress {
		t.Errorf("Status = %q, want in_progress", r.Status)
	}
	if r.StartedAt.IsZero() {
		t.Error("StartedAt must be set")
	}
	if ps.ActiveExercise != "loops-001" {
		t.Errorf("ActiveExercise = %q", ps.ActiveExercise)
	}
}

func TestProgressState_MarkStarted_IdempotentStatus(t *testing.T) {
	ps := state.NewProgressState()
	ps.MarkStarted("loops-001")
	first := ps.Exercises["loops-001"].StartedAt

	// A second MarkStarted must not reset StartedAt.
	time.Sleep(time.Millisecond)
	ps.MarkStarted("loops-001")
	second := ps.Exercises["loops-001"].StartedAt

	if !first.Equal(second) {
		t.Errorf("StartedAt changed on second MarkStarted: %v → %v", first, second)
	}
}

func TestProgressState_MarkChecked(t *testing.T) {
	ps := state.NewProgressState()
	ps.MarkChecked("loops-001")
	ps.MarkChecked("loops-001")

	r := ps.Exercises["loops-001"]
	if r.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", r.Attempts)
	}
	if r.LastCheckedAt == nil {
		t.Error("LastCheckedAt must be set")
	}
}

func TestProgressState_MarkCompleted(t *testing.T) {
	ps := state.NewProgressState()
	ps.MarkCompleted("loops-001")

	r := ps.Exercises["loops-001"]
	if r.Status != state.StatusCompleted {
		t.Errorf("Status = %q, want completed", r.Status)
	}
	if r.CompletedAt == nil {
		t.Error("CompletedAt must be set")
	}
}

// ---- Save / Load round-trip tests ----

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	ps := state.NewProgressState()
	ps.MarkStarted("ex-001")
	ps.MarkChecked("ex-001")
	ps.MarkCompleted("ex-001")

	if err := ps.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := loaded.Exercises["ex-001"]
	if r == nil {
		t.Fatal("exercise record missing after load")
	}
	if r.Status != state.StatusCompleted {
		t.Errorf("Status = %q, want completed", r.Status)
	}
	if r.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", r.Attempts)
	}
}

func TestLoad_MissingFile_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	ps, err := state.Load(path)
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if ps == nil {
		t.Fatal("expected non-nil ProgressState")
	}
	if len(ps.Exercises) != 0 {
		t.Error("expected empty exercises for fresh state")
	}
}

func TestSave_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "state.json")

	ps := state.NewProgressState()
	if err := ps.Save(path); err != nil {
		t.Fatalf("Save with nested dirs: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestSave_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	ps := state.NewProgressState()
	if err := ps.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// File must be owner-only readable (0600).
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file perm = %o, want 0600", info.Mode().Perm())
	}
}

func TestSave_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	ps := state.NewProgressState()
	ps.MarkStarted("x")
	if err := ps.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Errorf("saved file is not valid JSON: %v", err)
	}
}
