package cli

// helpers.go provides small shared utilities used across the CLI command files.
//
// ARCHITECTURE NOTE — Thin Helpers, No Abstraction Layers:
// These are pure utility functions with no side-effects beyond reading
// environment variables or calling OS functions. They are intentionally small:
// we do not create a "service layer" or "application layer" here. The CLI
// package is an I/O boundary; it owns wiring, not business logic. Business
// logic lives in the packages it calls (engine, state, repository, ui).

import (
	"os"
	"path/filepath"

	"github.com/thoth-go/thoth-go/internal/checker"
	appstate "github.com/thoth-go/thoth-go/internal/state"
)

// defaultBaseURL is the canonical remote URL for exercise topic bundles.
// Operators and CI environments can override it via THOTH_GO_BASE_URL.
const defaultBaseURL = "https://exercises.thoth-go.dev"

// baseURL returns the exercise server base URL, preferring the environment
// variable THOTH_GO_BASE_URL over the compiled-in default.
//
// ARCHITECTURE NOTE — Configuration via Environment Variables:
// Environment variables are the idiomatic twelve-factor-app mechanism for
// injecting configuration that differs between deployments (local dev vs. CI
// vs. production). Hardcoding a fallback keeps the binary usable without any
// configuration while still allowing easy overrides.
func baseURL() string {
	if v := os.Getenv("THOTH_GO_BASE_URL"); v != "" {
		return v
	}
	return defaultBaseURL
}

// loadOrNewState loads the progress state from disk, returning a fresh empty
// state if the file does not yet exist.
func loadOrNewState() (*appstate.ProgressState, string, error) {
	path, err := appstate.DefaultStatePath()
	if err != nil {
		return nil, "", err
	}
	ps, err := appstate.Load(path)
	if err != nil {
		return nil, "", err
	}
	return ps, path, nil
}

// activeExerciseID returns the exercise the learner is currently working on.
//
// It tries two sources in order:
//  1. ps.ActiveExercise — set when `thoth-go start` was run.
//  2. exercise.yaml in dir — useful when the learner cloned an exercise
//     directly without using `thoth-go start`.
//
// ARCHITECTURE NOTE — Graceful Degradation:
// Rather than forcing the user into a strict workflow, we fall back to
// reading exercise.yaml directly. This makes the CLI usable even when the
// state file doesn't know about the current directory.
func activeExerciseID(ps *appstate.ProgressState, dir string) string {
	if ps.ActiveExercise != "" {
		return ps.ActiveExercise
	}
	cfg, err := checker.LoadExerciseConfig(filepath.Join(dir, "exercise.yaml"))
	if err != nil {
		return ""
	}
	return cfg.ID
}

// testCaseName returns a display label for a dynamic.TestResult.
// It prefers the description from the config, falling back to a positional label.
func testCaseName(idx int, description string) string {
	if description != "" {
		return description
	}
	return formatOrdinal(idx + 1)
}

// formatOrdinal returns "Test 1", "Test 2", etc.
func formatOrdinal(n int) string {
	switch n {
	case 1:
		return "Test 1"
	case 2:
		return "Test 2"
	case 3:
		return "Test 3"
	default:
		// For larger numbers, use a generic format.
		return "Test " + itoa(n)
	}
}

// itoa converts a non-negative integer to a string without importing strconv
// at the package level (keeps imports clean in this helper file).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
