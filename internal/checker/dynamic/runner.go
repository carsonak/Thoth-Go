// Package dynamic implements the runtime execution engine for thoth-go.
//
// It compiles a learner's Go source directory into a temporary binary using
// the local Go toolchain and then executes that binary once per TestCase
// defined in the ExerciseConfig, capturing stdout and the exit code.
//
// # Layered Architecture
//
// The system is organised in strict layers:
//
//	UI → CLI → Engine → static / dynamic → checker (config types)
//
// dynamic sits at the lowest layer of the evaluation stack. It knows only
// about OS processes, the Go toolchain (`go build`), and the TestCase data
// from the config. It has no knowledge of UI formatting, progress state,
// or repository fetching. This separation means:
//
//   - The runner can be unit-tested by writing small Go programs to a temp dir
//     and asserting on the returned TestResult values — no CLI or UI involved.
//   - Swapping the dynamic strategy (e.g. sandboxed execution) only requires
//     changing this package and the Engine that calls it.
package dynamic

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/thoth-go/thoth-go/internal/checker"
)

const (
	// DefaultTimeout is the per-test-case execution budget.
	// Ten seconds is generous enough for simple I/O exercises while still
	// killing runaway infinite loops in a reasonable time.
	DefaultTimeout = 10 * time.Second

	// buildTimeout caps how long a single `go build` invocation may run.
	// A single-package exercise should never take longer than 30 s to compile
	// on reasonable hardware. The hard limit prevents CI hangs.
	buildTimeout = 30 * time.Second
)

// TestResult holds the outcome of a single dynamic test case execution.
//
// # Value Object
//
// TestResult is a plain data struct ("value object") — it carries information
// from the Runner upward to the Engine and UI but has no behaviour of its own.
// Keeping data and behaviour separate makes the struct trivially copyable,
// easy to assert against in tests, and straightforward to serialise if we ever
// want to persist run history.
type TestResult struct {
	// Index is the 0-based position of this case in cfg.TestCases, used by
	// the UI to display "Test 1", "Test 2", etc.
	Index int

	// TestCase is the original configuration entry. Embedding it here means
	// the UI does not need to keep a parallel slice — it has everything it needs
	// in one place (description, hidden flag, expected values).
	TestCase checker.TestCase

	// Passed is true when ActualOut == ExpectedStdout AND
	// ActualCode == ExpectedExitCode.
	Passed bool

	// ActualOut is the verbatim content written to stdout by the binary.
	ActualOut string

	// ActualCode is the exit code returned by the process.
	ActualCode int

	// TimedOut is true when the binary exceeded the per-case execution budget.
	// The UI uses this to show a distinct "timed out" message rather than a
	// generic diff, because the output will be truncated / empty.
	TimedOut bool

	// RunError is a human-readable description of a process-level failure
	// (build failure, binary not found, etc.). It is empty on clean runs.
	// By storing this as a string field instead of returning an error we keep
	// the Engine's control flow simple — it only needs to handle errors in
	// truly exceptional circumstances.
	RunError string
}

// Runner compiles and executes learner Go source code against the test cases
// defined in an ExerciseConfig.
//
// # Dependency Injection of Timeout
//
// Timeout is an explicit struct field rather than a global constant. This is
// a fundamental Go dependency-injection technique: callers (including tests)
// can override the timeout without recompiling the package or touching global
// state. Tests inject a short timeout (e.g. 200 ms) to make the "infinite
// loop" test case fast.
type Runner struct {
	// Timeout is the per-test-case execution budget.
	// Zero means use DefaultTimeout.
	Timeout time.Duration
}

// New returns a Runner configured with the default execution timeout.
func New() *Runner {
	return &Runner{Timeout: DefaultTimeout}
}

// effectiveTimeout returns the configured timeout, falling back to the default.
func (r *Runner) effectiveTimeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return DefaultTimeout
}

// Build compiles the Go package rooted at dir into a temporary binary.
// It returns the absolute path to the binary, a cleanup closure the caller
// must invoke (ideally with defer), and any compilation error.
//
// The binary is placed in an OS-managed temp directory.  The caller decides
// the binary's lifetime; this method does not own the resource after returning.
//
// # Returning a Cleanup Closure
//
// Returning a cleanup function keeps resource management explicit and
// co-located with the allocation.  Compare this to registering finalizers or
// relying on the OS to reclaim temp files — both of which make lifetime
// reasoning difficult.  Go's idiomatic pattern is:
//
//	binPath, cleanup, err := runner.Build(dir)
//	defer cleanup()
//
// which mirrors how database/sql uses rows.Close() and os.File.Close().
func (r *Runner) Build(dir string) (binPath string, cleanup func(), err error) {
	tmp, mkErr := os.MkdirTemp("", "thoth-go-build-*")
	if mkErr != nil {
		return "", func() {}, fmt.Errorf("dynamic: creating temp dir: %w", mkErr)
	}
	cleanup = func() { os.RemoveAll(tmp) }

	binPath = filepath.Join(tmp, "exercise")

	// context.WithTimeout ensures the subprocess is killed (SIGKILL via
	// exec.CommandContext) if go build hangs, preventing CI hangs.
	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()

	// `go build -o <binPath> .` compiles the package in dir.
	// We capture stderr so compilation errors can be surfaced to the learner
	// as readable messages rather than opaque runner errors.
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".") //nolint:gosec
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if runErr := cmd.Run(); runErr != nil {
		cleanup()
		cleanup = func() {} // idempotent: cleanup already ran
		if ctx.Err() != nil {
			return "", func() {}, fmt.Errorf("dynamic: build timed out after %s", buildTimeout)
		}
		return "", func() {}, fmt.Errorf("dynamic: compilation failed:\n%s", strings.TrimSpace(stderr.String()))
	}

	return binPath, cleanup, nil
}

// Run compiles dir and executes all TestCases defined in cfg.
//
// # Error vs. Failure
//
// Run distinguishes between errors and failures — a critical design choice:
//
//   - A returned error means a hard I/O problem (e.g. cannot create temp dir)
//     that prevents the runner from doing its job at all.
//   - A build failure or wrong output is NOT an error; it is represented as a
//     TestResult with Passed==false and RunError set.  This lets the Engine and
//     UI handle "wrong answer" and "broken code" as normal cases in their flow,
//     not as exceptional conditions that unwind the call stack.
//
// # Mode dispatch
//
//   - ModeExecutable / ModeBugFix: compile → run each test case.
//   - ModeFunctionSignature: returns a stub result (see comment inside).
func (r *Runner) Run(cfg *checker.ExerciseConfig, dir string) ([]TestResult, error) {
	// # Stub / Walking Skeleton for ModeFunctionSignature
	//
	// ModeFunctionSignature requires injecting a synthetic test-harness Go file
	// into the submission directory before compilation — a more complex flow
	// that will be implemented in a future phase.  Rather than panicking or
	// silently skipping, we return a clearly labelled stub result so:
	//
	//   1. The Engine and CLI work end-to-end (a "walking skeleton").
	//   2. Static checks still run and can fail correctly.
	//   3. The learner sees a clear "not yet implemented" message rather than a
	//      confusing absence of output.
	if cfg.Mode == checker.ModeFunctionSignature {
		return []TestResult{{
			Index:    0,
			Passed:   false,
			RunError: "function_signature dynamic execution is not yet implemented; static checks still apply",
		}}, nil
	}

	if len(cfg.TestCases) == 0 {
		return nil, nil
	}

	binPath, cleanup, err := r.Build(dir)
	defer cleanup()

	if err != nil {
		// Surface the build error as data so the UI can render the compiler
		// output in a friendly way.  We return one synthetic TestResult that
		// represents the whole "build" phase rather than one per test case,
		// because the learner only needs to see the compile error once.
		return []TestResult{{
			Index:    0,
			Passed:   false,
			RunError: err.Error(),
		}}, nil
	}

	results := make([]TestResult, len(cfg.TestCases))
	for i, tc := range cfg.TestCases {
		results[i] = r.runCase(binPath, i, tc)
	}
	return results, nil
}

// runCase executes a single test case against the pre-compiled binary and
// returns a populated TestResult.
func (r *Runner) runCase(binPath string, idx int, tc checker.TestCase) TestResult {
	result := TestResult{
		Index:    idx,
		TestCase: tc,
	}

	// Each test case gets its own fresh context so a timeout on case N does
	// not affect the budget of case N+1.  context.WithTimeout propagates
	// cleanly through exec.CommandContext, which sends SIGKILL when the
	// deadline fires — essential for stopping infinite loops in learner code.
	ctx, cancel := context.WithTimeout(context.Background(), r.effectiveTimeout())
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, tc.Args...) //nolint:gosec
	if tc.Stdin != "" {
		cmd.Stdin = strings.NewReader(tc.Stdin)
	}

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()
	result.ActualOut = stdout.String()

	// Check ctx.Err() before inspecting the run error.  After a deadline fires,
	// cmd.Run() returns a non-nil error AND ctx.Err() == DeadlineExceeded.
	// Checking the context is more reliable than inspecting the error message.
	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.RunError = fmt.Sprintf("timed out after %s", r.effectiveTimeout())
		return result
	}

	result.ActualCode = exitCode(err)
	result.Passed = result.ActualOut == tc.ExpectedStdout &&
		result.ActualCode == tc.ExpectedExitCode

	return result
}

// exitCode extracts the integer exit code from a cmd.Run() error.
// Returns 0 for nil (clean exit) and -1 for non-ExitError failures.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if isExitErr := func() bool {
		exitErr, _ = err.(*exec.ExitError)
		return exitErr != nil
	}(); isExitErr {
		return exitErr.ExitCode()
	}
	return -1
}
