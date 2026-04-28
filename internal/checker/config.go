// Package checker defines the configuration structures parsed from exercise.yaml.
// All checker rules are data-driven — no hardcoded logic in the engine.
package checker

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Mode describes how an exercise is evaluated.
type Mode string

const (
	// ModeExecutable runs the compiled binary and checks stdout/exit-code.
	ModeExecutable Mode = "executable"
	// ModeFunctionSignature checks that required exported functions exist with
	// the correct signatures, then runs tests.
	ModeFunctionSignature Mode = "function_signature"
	// ModeBugFix diffs the submitted file against a reference patch and also
	// runs tests to confirm the fix is correct.
	ModeBugFix Mode = "bug_fix"
)

// ImportRules lists allowed and banned standard-library (or third-party) imports.
type ImportRules struct {
	// Allowed is a whitelist; if non-empty, only these imports are permitted.
	Allowed []string `yaml:"allowed"`
	// Banned imports cause an immediate static-check failure.
	Banned []string `yaml:"banned"`
}

// TestCase describes a single black-box test run against the compiled binary.
type TestCase struct {
	// Args are command-line arguments passed to the binary.
	Args []string `yaml:"args"`
	// Stdin is optional input piped to the binary's stdin.
	Stdin string `yaml:"stdin,omitempty"`
	// ExpectedStdout is the exact string the binary must print to stdout.
	ExpectedStdout string `yaml:"expected_stdout"`
	// ExpectedExitCode is the exit code the binary must return (default 0).
	ExpectedExitCode int `yaml:"expected_exit_code"`
	// Hidden test cases are not shown to the learner on failure.
	Hidden bool `yaml:"hidden"`
	// Description is an optional human-readable label for the test.
	Description string `yaml:"description,omitempty"`
}

// FunctionSpec describes a required exported function that must be present.
type FunctionSpec struct {
	// Name is the exact Go identifier (e.g. "Fibonacci").
	Name string `yaml:"name"`
	// Signature is a human-readable description used only in error messages.
	Signature string `yaml:"signature"`
}

// BugFixSpec configures the optional bug-fix diff mode.
type BugFixSpec struct {
	// ReferenceFile is a path (relative to the exercise root) to the corrected
	// reference source. Used only in ModeBugFix.
	ReferenceFile string `yaml:"reference_file"`
	// RestrictedLines lists 1-based line numbers the learner must not modify.
	RestrictedLines []int `yaml:"restricted_lines,omitempty"`
}

// ExerciseConfig is the top-level structure parsed from exercise.yaml.
type ExerciseConfig struct {
	// ID is the unique, URL-safe exercise identifier (e.g. "loops-001").
	ID string `yaml:"id"`
	// Title is the human-readable exercise name.
	Title string `yaml:"title"`
	// Topic groups related exercises (e.g. "control-flow", "concurrency").
	Topic string `yaml:"topic"`
	// Mode controls which evaluation strategy the checker uses.
	Mode Mode `yaml:"mode"`
	// Difficulty is an optional hint shown in progress output (1-5).
	Difficulty int `yaml:"difficulty,omitempty"`

	// Imports enforces import allowlists/blocklists.
	Imports ImportRules `yaml:"imports"`
	// BannedASTNodes lists go/ast type names that must not appear in the source.
	// Example: ["ast.ForStmt", "ast.GoStmt"]
	BannedASTNodes []string `yaml:"banned_ast_nodes"`
	// BannedFunctions lists fully-qualified function identifiers that must not
	// be called, verified via go/types to prevent aliasing.
	// Example: ["fmt.Println", "os.Exit"]
	BannedFunctions []string `yaml:"banned_functions"`

	// RequiredFunctions lists exported functions the submission must define.
	// Used in ModeFunctionSignature exercises.
	RequiredFunctions []FunctionSpec `yaml:"required_functions,omitempty"`

	// BugFix configures the diff-based checker for ModeBugFix.
	BugFix BugFixSpec `yaml:"bug_fix,omitempty"`

	// TestCases are the black-box input/output test cases run after compilation.
	TestCases []TestCase `yaml:"test_cases"`
}

// Validate performs semantic validation of the parsed config beyond what the
// YAML decoder can enforce (required fields, valid mode, etc.).
func (c *ExerciseConfig) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("exercise config: 'id' is required")
	}
	if c.Title == "" {
		return fmt.Errorf("exercise config: 'title' is required")
	}
	if c.Topic == "" {
		return fmt.Errorf("exercise config: 'topic' is required")
	}
	switch c.Mode {
	case ModeExecutable, ModeFunctionSignature, ModeBugFix:
		// valid
	case "":
		return fmt.Errorf("exercise config: 'mode' is required")
	default:
		return fmt.Errorf("exercise config: unknown mode %q (want executable|function_signature|bug_fix)", c.Mode)
	}
	if c.Mode == ModeBugFix && c.BugFix.ReferenceFile == "" {
		return fmt.Errorf("exercise config: bug_fix mode requires 'bug_fix.reference_file'")
	}
	return nil
}

// LoadExerciseConfig reads and parses an exercise.yaml file from disk.
func LoadExerciseConfig(path string) (*ExerciseConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading exercise config: %w", err)
	}
	return ParseExerciseConfig(data)
}

// ParseExerciseConfig decodes YAML bytes into an ExerciseConfig and validates it.
func ParseExerciseConfig(data []byte) (*ExerciseConfig, error) {
	var cfg ExerciseConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // reject unknown keys to surface config typos early
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing exercise config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}
