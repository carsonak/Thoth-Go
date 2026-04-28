// Package engine implements the master orchestrator (the "Engine") for
// thoth-go's grading pipeline.
//
// # Facade / Orchestrator Pattern
//
// The Engine acts as a Facade over two subsystems — static analysis and
// dynamic execution — and as an Orchestrator that decides the order in which
// those subsystems are invoked.
//
//	                 ┌─────────────────────────────────┐
//	                 │            Engine               │
//	                 │                                 │
//	CLI / tests ────▶│  1. Load exercise.yaml          │
//	                 │  2. static.Checker.AnalyzeDir   │
//	                 │     (halt on violation)         │
//	                 │  3. dynamic.Runner.Run          │
//	                 │  4. Return CheckResult          │
//	                 └─────────────────────────────────┘
//	                        │              │
//	                 ┌──────┘              └──────┐
//	                 ▼                            ▼
//	         static.Checker               dynamic.Runner
//	      (AST + go/types rules)      (go build + exec)
//
// # Why a Separate Package?
//
// The Engine imports both `internal/checker/static` and
// `internal/checker/dynamic`, which in turn both import
// `internal/checker` (config types). Putting the Engine in the
// `internal/checker` package would create an import cycle:
//
//	checker → static → checker (CYCLE)
//
// A dedicated `engine` sub-package breaks the cycle while keeping the
// coordinator logic separate from the low-level subsystems — a clean
// application of the Dependency Inversion Principle.
package engine

import (
	"fmt"
	"path/filepath"

	"github.com/thoth-go/thoth-go/internal/checker"
	"github.com/thoth-go/thoth-go/internal/checker/dynamic"
	"github.com/thoth-go/thoth-go/internal/checker/static"
)

// CheckResult is the complete outcome of a single `thoth-go check` run.
//
// # Separation of Static and Dynamic Results
//
// Keeping StaticViolations and TestResults as distinct fields (rather than
// merging them into one list) lets the UI render them differently — static
// violations appear as rule badges, while test results appear as pass/fail
// diffs — without needing to inspect a type field on every item.
type CheckResult struct {
	// StaticViolations are the rule violations found by the AST/types checker.
	// Non-empty means the submission broke at least one exercise rule
	// (banned import, banned node, missing function, etc.).
	StaticViolations []static.Violation

	// TestResults are the outcomes of each black-box test case executed against
	// the compiled binary. Nil means the engine halted before the dynamic phase
	// (e.g. static violations were found, or there were no test cases).
	TestResults []dynamic.TestResult

	// StaticPassed is true when no static violations were found.
	StaticPassed bool

	// DynamicPassed is true when every TestResult has Passed==true.
	// Meaningless when TestResults is nil.
	DynamicPassed bool

	// AllPassed is true when both static and dynamic phases succeeded.
	// This is the single "did the learner complete the exercise?" signal
	// consumed by the state manager and the UI summary.
	AllPassed bool
}

// Engine orchestrates a complete check run for a single exercise directory.
//
// # Dependency Injection of the Runner
//
// The runner field holds a *dynamic.Runner rather than calling dynamic.New()
// inside Run. This lets tests inject a Runner with a short timeout without
// modifying global state. It follows the same DI pattern used throughout
// thoth-go: explicit dependencies, no hidden singletons.
type Engine struct {
	// dir is the absolute path to the exercise working directory.
	dir string
	// runner is the dynamic execution engine; injected for testability.
	runner *dynamic.Runner
}

// New creates an Engine for the exercise in dir.
// Pass dir as the absolute path to the directory containing exercise.yaml
// and the learner's Go source files.
func New(dir string) *Engine {
	return &Engine{
		dir:    dir,
		runner: dynamic.New(),
	}
}

// NewWithRunner creates an Engine with a custom Runner.
// This is exported for use by tests and higher-level integrations that need
// to inject a Runner with a specific timeout or mock behaviour.
func NewWithRunner(dir string, runner *dynamic.Runner) *Engine {
	return &Engine{dir: dir, runner: runner}
}

// Run executes the full grading pipeline for the engine's directory:
//
//  1. Load and validate exercise.yaml.
//  2. Run static analysis (import rules, banned AST nodes, banned functions,
//     required functions for function_signature mode).
//  3. If static analysis passes, run the dynamic test suite.
//  4. Return a CheckResult that the CLI can render and the state manager can
//     use to update progress.
//
// # Halt-on-Static-Failure Strategy
//
// The engine halts after Step 2 if any static violations are found. This is a
// deliberate design choice: running the dynamic tests against code that already
// breaks the exercise rules wastes time and produces confusing output
// (e.g. "your binary output was correct, but you used a for loop when
// recursion was required"). The learner should fix rule violations first.
//
// Run returns a non-nil error only for hard infrastructure failures (e.g. the
// exercise.yaml file is missing or unreadable). Static violations and test
// failures are returned as data in CheckResult, not as errors.
func (e *Engine) Run() (*CheckResult, error) {
	// ── Step 1: Load exercise configuration ─────────────────────────────────
	//
	// exercise.yaml is the single source of truth for all grading rules.
	// ParseExerciseConfig validates the config (required fields, valid mode,
	// cross-field constraints) so the rest of the pipeline can trust it.
	cfg, err := checker.LoadExerciseConfig(filepath.Join(e.dir, "exercise.yaml"))
	if err != nil {
		return nil, fmt.Errorf("engine: loading exercise config: %w", err)
	}

	return e.RunWithConfig(cfg)
}

// RunWithConfig runs the pipeline using an already-parsed config.
// This exists so the engine test suite can construct configs programmatically
// without writing YAML to disk — another example of injecting dependencies
// through explicit parameters rather than file I/O.
func (e *Engine) RunWithConfig(cfg *checker.ExerciseConfig) (*CheckResult, error) {
	result := &CheckResult{}

	// ── Step 2: Static analysis ──────────────────────────────────────────────
	//
	// The static.Checker is constructed from the config (config-driven, not
	// hardcoded). AnalyzeDir applies import rules, banned AST nodes, and banned
	// functions in one pass. CheckRequiredFunctions is an additional pass only
	// needed for function_signature mode exercises.
	sc := static.New(cfg)

	violations, err := sc.AnalyzeDir(e.dir)
	if err != nil {
		return nil, fmt.Errorf("engine: static analysis: %w", err)
	}

	// For function_signature mode, also check that required exported functions
	// are declared. This is a structural check (does the function exist at all?)
	// — the dynamic phase (once implemented) will verify that it behaves
	// correctly.
	if cfg.Mode == checker.ModeFunctionSignature {
		rfViolations, rfErr := sc.CheckRequiredFunctions(e.dir)
		if rfErr != nil {
			return nil, fmt.Errorf("engine: required function check: %w", rfErr)
		}
		violations = append(violations, rfViolations...)
	}

	result.StaticViolations = violations
	result.StaticPassed = len(violations) == 0

	// Halt-on-static-failure: do not run tests if the code already violates
	// the exercise rules. See the Run doc-comment for rationale.
	if !result.StaticPassed {
		return result, nil
	}

	// ── Step 3: Dynamic execution ────────────────────────────────────────────
	//
	// The Runner compiles the source, runs each TestCase in the config, and
	// returns structured results. Build failures (compilation errors) are
	// represented as a TestResult with RunError set — not as a returned error —
	// so the UI can render compiler output in a friendly way.
	testResults, dynErr := e.runner.Run(cfg, e.dir)
	if dynErr != nil {
		return nil, fmt.Errorf("engine: dynamic runner: %w", dynErr)
	}

	result.TestResults = testResults
	result.DynamicPassed = allPassed(testResults)
	result.AllPassed = result.StaticPassed && result.DynamicPassed

	return result, nil
}

// allPassed returns true when every TestResult in the slice has Passed==true.
// An empty (nil) slice is treated as "passed" because exercises with no test
// cases are considered fully static (graded by rule compliance alone).
func allPassed(results []dynamic.TestResult) bool {
	for _, r := range results {
		if !r.Passed {
			return false
		}
	}
	return true
}
