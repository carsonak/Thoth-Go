// Package state manages the user's progress and snapshots in ~/.thoth-go.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Status represents the completion status of a single exercise.
type Status string

const (
	StatusNotStarted Status = "not_started"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

// ExerciseRecord tracks the learner's progress on one exercise.
type ExerciseRecord struct {
	// ExerciseID matches ExerciseConfig.ID.
	ExerciseID string `json:"exercise_id"`
	// Status is the current completion state.
	Status Status `json:"status"`
	// Attempts is the total number of times `thoth-go check` was run.
	Attempts int `json:"attempts"`
	// StartedAt is when the exercise was first opened (zero value = never).
	StartedAt time.Time `json:"started_at,omitempty"`
	// CompletedAt is when the exercise first passed all checks.
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// LastCheckedAt is the timestamp of the most recent check run.
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
}

// ProgressState is the top-level structure written to ~/.thoth-go/state.json.
type ProgressState struct {
	// SchemaVersion allows forward-compatible migrations.
	SchemaVersion int `json:"schema_version"`
	// Exercises maps exercise ID → record.
	Exercises map[string]*ExerciseRecord `json:"exercises"`
	// ActiveExercise is the ID of the exercise currently being worked on.
	ActiveExercise string `json:"active_exercise,omitempty"`
	// UpdatedAt is set whenever the state is persisted.
	UpdatedAt time.Time `json:"updated_at"`
}

const currentSchemaVersion = 1

// NewProgressState returns an empty, initialised ProgressState.
func NewProgressState() *ProgressState {
	return &ProgressState{
		SchemaVersion: currentSchemaVersion,
		Exercises:     make(map[string]*ExerciseRecord),
		UpdatedAt:     time.Now(),
	}
}

// Record returns the ExerciseRecord for the given ID, creating one if absent.
func (p *ProgressState) Record(exerciseID string) *ExerciseRecord {
	r, ok := p.Exercises[exerciseID]
	if !ok {
		r = &ExerciseRecord{
			ExerciseID: exerciseID,
			Status:     StatusNotStarted,
		}
		p.Exercises[exerciseID] = r
	}
	return r
}

// MarkStarted transitions an exercise to InProgress if it hasn't been started.
func (p *ProgressState) MarkStarted(exerciseID string) {
	r := p.Record(exerciseID)
	if r.Status == StatusNotStarted {
		r.Status = StatusInProgress
		now := time.Now()
		r.StartedAt = now
	}
	p.ActiveExercise = exerciseID
	p.UpdatedAt = time.Now()
}

// MarkChecked increments the attempt counter and updates LastCheckedAt.
func (p *ProgressState) MarkChecked(exerciseID string) {
	r := p.Record(exerciseID)
	r.Attempts++
	now := time.Now()
	r.LastCheckedAt = &now
	p.UpdatedAt = time.Now()
}

// MarkCompleted transitions an exercise to Completed.
func (p *ProgressState) MarkCompleted(exerciseID string) {
	r := p.Record(exerciseID)
	r.Status = StatusCompleted
	now := time.Now()
	r.CompletedAt = &now
	p.UpdatedAt = time.Now()
}

// Save serialises the state to disk at the given path, creating parent
// directories if necessary.
func (p *ProgressState) Save(path string) error {
	p.UpdatedAt = time.Now()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("state save: creating dir: %w", err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("state save: marshalling: %w", err)
	}
	// Write atomically via a temp file then rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("state save: writing temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("state save: renaming: %w", err)
	}
	return nil
}

// Load reads and parses a state.json file from disk.
// If the file does not exist, a fresh ProgressState is returned.
func Load(path string) (*ProgressState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewProgressState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("state load: reading file: %w", err)
	}
	return ParseState(data)
}

// ParseState decodes JSON bytes into a ProgressState.
func ParseState(data []byte) (*ProgressState, error) {
	var ps ProgressState
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("state parse: %w", err)
	}
	if ps.Exercises == nil {
		ps.Exercises = make(map[string]*ExerciseRecord)
	}
	return &ps, nil
}

// DefaultStatePath returns the canonical path for state.json.
func DefaultStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".thoth-go", "state.json"), nil
}
