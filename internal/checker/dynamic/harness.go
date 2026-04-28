// Package dynamic — harness.go: test harness generator for ModeFunctionSignature.
//
// # What is a Test Harness?
//
// A test harness is a piece of scaffolding code that wraps the code-under-test
// (here: the learner's function) so it can be invoked by a testing framework.
// Instead of running the learner's code directly (which would require knowing
// the function's ABI at compile time in the grader), we generate a Go source
// file at runtime that calls the learner's function and asserts on the result.
//
// # Meta-programming / Code Generation at Runtime
//
// "Meta-programming" is writing code that writes code. Here, the grader
// inspects the ExerciseConfig (test cases + required function names) and
// emits a valid Go test file (`harness_test.go`) before invoking `go test`.
// The generated file is compiled together with the learner's submission by the
// Go toolchain — the grader never links against the learner's package directly.
//
// Benefits:
//
//  1. Security isolation: The grader's core logic (engine, state, CLI) is
//     never compiled into the same binary as the learner's code. A malicious
//     or buggy submission cannot read grader internals, corrupt memory, or call
//     os.Exit() in a way that kills the grader process (the test runs in a
//     sub-process via `go test`).
//
//  2. Idiomatic testing: The harness uses the standard `testing` package, so
//     the output is parseable via `go test -json`, giving structured pass/fail
//     events per test case rather than a bespoke protocol.
//
//  3. Type safety via the Go compiler: If the learner's function signature does
//     not match what the harness calls, the compilation fails with a clear
//     error message — far better than a runtime panic or a cryptic diff.
//
// # Cleanup guarantee
//
// The harness file is written immediately before `go test` and removed
// immediately after via `defer os.Remove(harnessPath)`. This ensures:
//   - The harness never pollutes the learner's working directory between runs.
//   - Even if `go test` panics or returns an error, the file is removed.

package dynamic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thoth-go/thoth-go/internal/checker"
)

// goTestEvent mirrors the JSON objects emitted by `go test -json`.
//
// `go test -json` outputs one JSON object per line. Each object represents a
// single event in the test lifecycle. We only care about a subset of fields:
//
//   - Action: "run" | "output" | "pass" | "fail" | "skip" | "build-fail"
//   - Test: present for test-function-level events; absent for package events.
//   - Output: the log line for "output" events (e.g. t.Errorf messages).
//
// Parsing JSON lines (NDJSON) is more robust than parsing the human-readable
// text output: the format is stable across Go versions and does not depend on
// terminal width or colour settings.
type goTestEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

// runFunctionSignature implements the ModeFunctionSignature evaluation strategy.
//
// # Algorithm
//
//  1. Detect the package name of the learner's code via go/parser so the
//     generated harness file uses the correct package declaration.
//  2. Generate harness_test.go: one TestHarness_Case<n> function per TestCase,
//     each calling cfg.RequiredFunctions[0].Name with Args from the test case
//     and asserting that fmt.Sprintf("%v\n", result) == ExpectedStdout.
//  3. Run `go test -v -json -run TestHarness_` in the exercise directory.
//  4. Parse the JSON event stream to produce one TestResult per TestCase.
//  5. Remove harness_test.go (guaranteed via defer even on error).
//
// # Function call convention
//
// TestCase.Args contains Go source-code expression strings for the arguments.
// For example, a TestCase testing Add(3, 4) would have Args = ["3", "4"].
// The harness emits the literal call `Add(3, 4)` into the generated file, so
// the Go compiler enforces type-correctness — no strconv needed at harness-run
// time. Exercise designers must write Args as valid Go expressions.
//
// # Output comparison
//
// The harness uses fmt.Sprintf("%v\n", result) to convert the function's return
// value to a string and appends a newline, mirroring the convention for
// ModeExecutable (where programs print to stdout and end lines with "\n").
// This makes ExpectedStdout consistent across both modes.
func (r *Runner) runFunctionSignature(cfg *checker.ExerciseConfig, dir string) ([]TestResult, error) {
	// No test cases → static-only exercise; dynamic phase has nothing to do.
	if len(cfg.TestCases) == 0 {
		return nil, nil
	}

	// We need at least one required function to know what to call in the harness.
	if len(cfg.RequiredFunctions) == 0 {
		return []TestResult{{
			Index:  0,
			Passed: false,
			RunError: "function_signature mode requires at least one entry in " +
				"required_functions so the harness knows which function to call",
		}}, nil
	}

	// ── Step 1: Detect package name ──────────────────────────────────────────
	//
	// We scan the learner's directory for any non-test .go file and parse only
	// the package clause (PackageClauseOnly flag avoids parsing function bodies,
	// keeping it fast). The package name must match the harness declaration.
	pkgName, err := detectPackageName(dir)
	if err != nil {
		return nil, fmt.Errorf("function_signature: detecting package name: %w", err)
	}

	funcName := cfg.RequiredFunctions[0].Name

	// ── Step 2: Generate and write harness_test.go ───────────────────────────
	//
	// The generated file is placed directly in the learner's directory so Go's
	// test runner picks it up as part of the same package. A "_test.go" suffix
	// tells the Go toolchain to include it only during `go test`, not `go build`.
	harnessPath := filepath.Join(dir, "harness_test.go")
	harnessCode := generateHarness(pkgName, funcName, cfg.TestCases)

	if writeErr := os.WriteFile(harnessPath, []byte(harnessCode), 0o644); writeErr != nil {
		return nil, fmt.Errorf("function_signature: writing harness file: %w", writeErr)
	}
	// Guarantee cleanup: remove the harness file after this function returns,
	// regardless of whether `go test` succeeds, fails, or panics.
	defer os.Remove(harnessPath) //nolint:errcheck // best-effort cleanup

	// ── Step 3: Run go test -json ─────────────────────────────────────────────
	//
	// The overall timeout is: buildTimeout (compile) + numCases * perCaseBudget.
	// This is conservative; most harnesses compile in < 2 s and each test case
	// runs in microseconds (it is just a function call). The generous ceiling
	// prevents false timeouts on slow CI machines.
	totalTimeout := buildTimeout + time.Duration(len(cfg.TestCases))*r.effectiveTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), totalTimeout)
	defer cancel()

	// `-run TestHarness_` ensures only our generated tests execute, not any
	// existing tests the learner might have written in their directory.
	cmd := exec.CommandContext(ctx, "go", "test", "-v", "-json", "-run", "TestHarness_", ".") //nolint:gosec
	cmd.Dir = dir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	runErr := cmd.Run()

	// `go test` exits with code 1 when tests fail — that is expected and is
	// NOT an infrastructure error. We only propagate errors that are not
	// clean ExitErrors (e.g. ENOENT if `go` is not in PATH, I/O failures).
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			// True infrastructure failure — propagate upward.
			return nil, fmt.Errorf("function_signature: running go test: %w", runErr)
		}
		// exitErr means test failures or build failure; parse the output below.
	}

	// ── Step 4: Parse JSON event stream ─────────────────────────────────────
	return parseHarnessOutput(stdout.Bytes(), cfg.TestCases)
}

// detectPackageName reads the first non-test .go file in dir and returns its
// package name using go/parser with the PackageClauseOnly mode.
//
// PackageClauseOnly instructs the parser to stop after the package declaration,
// making this operation O(1) in the size of the source file.
func detectPackageName(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("reading directory %q: %w", dir, err)
	}

	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, parseErr := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.PackageClauseOnly)
		if parseErr != nil {
			continue // skip unparseable files; try the next one
		}
		if f.Name != nil && f.Name.Name != "" {
			return f.Name.Name, nil
		}
	}
	return "", fmt.Errorf("no parseable Go source files found in %q", dir)
}

// generateHarness produces the Go source code for the test harness file.
//
// # Design: string building over text/template
//
// We use fmt.Fprintf into a strings.Builder rather than text/template for two
// reasons:
//
//  1. The template is simple and linear — no iteration over complex nested
//     structures — so template syntax adds ceremony without clarity.
//  2. Keeping the generated format visible as plain format strings makes the
//     escaping rules obvious to a reader. The escaping guide below explains
//     each non-obvious case.
//
// # Escaping guide
//
// When using fmt.Fprintf to write Go source code, there are two layers of
// escaping to track:
//
//   - Go string literals in THIS source file (e.g. "\"" is one double-quote)
//   - fmt format verbs in the format string (e.g. %% is a literal %)
//
// Key examples used below:
//
//	"\tgot := fmt.Sprintf(\"%%v\\n\", %s(%s))\n"
//	  ├─ \t      → tab in output
//	  ├─ \"      → " in output (opening the inner string literal)
//	  ├─ %%      → % in output (fmt escaping; produces %v in generated code)
//	  ├─ \\n     → \n in output (backslash+n; in generated code it is the
//	  │            newline escape inside the string literal "%v\n")
//	  ├─ \"      → " in output (closing the inner string literal)
//	  └─ \n      → actual newline (ends the source line in the generated file)
func generateHarness(pkgName, funcName string, cases []checker.TestCase) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "// Code generated by thoth-go harness generator. DO NOT EDIT.\n")
	fmt.Fprintf(&sb, "// This file is automatically removed after test execution.\n")
	fmt.Fprintf(&sb, "package %s\n\n", pkgName)
	fmt.Fprintf(&sb, "import (\n\t\"fmt\"\n\t\"testing\"\n)\n\n")

	for i, tc := range cases {
		args := strings.Join(tc.Args, ", ")
		// %q wraps the expected string in Go double-quote syntax with proper
		// escaping (e.g. newlines become \n, quotes become \"). This ensures
		// the generated 'want' literal is syntactically valid regardless of
		// what characters appear in ExpectedStdout.
		fmt.Fprintf(&sb, "func TestHarness_Case%d(t *testing.T) {\n", i)
		fmt.Fprintf(&sb, "\tgot := fmt.Sprintf(\"%%v\\n\", %s(%s))\n", funcName, args)
		fmt.Fprintf(&sb, "\twant := %q\n", tc.ExpectedStdout)
		fmt.Fprintf(&sb, "\tif got != want {\n")
		// Do NOT embed args directly into the Errorf format string: if args
		// contain quote characters (e.g. string literal expressions like
		// `"Alice"`) they would produce invalid Go syntax inside the string.
		// Use a positional index instead for a safe, readable error label.
		fmt.Fprintf(&sb, "\t\tt.Errorf(\"case %d: got %%q; want %%q\", got, want)\n", i)
		fmt.Fprintf(&sb, "\t}\n")
		fmt.Fprintf(&sb, "}\n\n")
	}

	return sb.String()
}

// parseHarnessOutput interprets the NDJSON stream from `go test -json` and
// maps each TestHarness_Case<N> event back to a TestResult for index N.
//
// # NDJSON (Newline-Delimited JSON) parsing
//
// `go test -json` writes one JSON object per line. We use bufio.Scanner
// (with a generous buffer) to read line by line and json.Unmarshal each line
// independently. This is more robust than json.Decoder.Token() because a
// corrupted or extra line causes us to skip that line rather than aborting
// the entire parse.
//
// # Build failure handling
//
// If the submission does not compile (wrong function signature, syntax error,
// missing import), `go test -json` emits an `Action:"build-fail"` event
// followed by package-level `Action:"output"` events containing the compiler
// diagnostics. We collect these diagnostics and set RunError on all results
// so the UI can display the compiler error to the learner.
func parseHarnessOutput(jsonOutput []byte, cases []checker.TestCase) ([]TestResult, error) {
	results := make([]TestResult, len(cases))
	for i, tc := range cases {
		results[i] = TestResult{Index: i, TestCase: tc}
	}

	// Build a map from test name → pass/fail result and accumulated output.
	type testState struct {
		passed bool
		seen   bool
		output strings.Builder
	}
	byName := make(map[string]*testState)

	var buildFailed bool
	var buildOutput strings.Builder

	// Use a large scanner buffer (1 MB) to handle long compiler error messages
	// without hitting the default 64 KB limit.
	const maxScanBuf = 1 << 20 // 1 MB
	scanner := bufio.NewScanner(bytes.NewReader(jsonOutput))
	scanner.Buffer(make([]byte, maxScanBuf), maxScanBuf)

	for scanner.Scan() {
		var ev goTestEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue // skip malformed lines rather than aborting
		}

		switch ev.Action {
		case "build-fail":
			buildFailed = true

		case "output":
			if ev.Test == "" {
				// Package-level output (build errors, test binary output).
				buildOutput.WriteString(ev.Output)
			} else {
				// Test-function-level output (t.Errorf lines, t.Log, etc.).
				if byName[ev.Test] == nil {
					byName[ev.Test] = &testState{}
				}
				byName[ev.Test].output.WriteString(ev.Output)
			}

		case "pass":
			if ev.Test != "" {
				if byName[ev.Test] == nil {
					byName[ev.Test] = &testState{}
				}
				byName[ev.Test].passed = true
				byName[ev.Test].seen = true
			}

		case "fail":
			if ev.Test != "" {
				if byName[ev.Test] == nil {
					byName[ev.Test] = &testState{}
				}
				byName[ev.Test].passed = false
				byName[ev.Test].seen = true
			}
		}
	}

	// Build failure: mark every test case with the compiler error output.
	if buildFailed {
		errMsg := strings.TrimSpace(buildOutput.String())
		if errMsg == "" {
			errMsg = "build failed (no compiler output captured)"
		}
		for i := range results {
			results[i].Passed = false
			results[i].RunError = "compilation failed:\n" + errMsg
		}
		return results, nil
	}

	// Map TestHarness_Case<N> events back to TestResult[N].
	for i := range results {
		testName := "TestHarness_Case" + strconv.Itoa(i)
		st := byName[testName]
		if st == nil || !st.seen {
			// The test was not found in the output — likely because an earlier
			// test panicked and halted the test binary, or the test name does
			// not match (e.g. the harness was generated with different indices).
			results[i].RunError = fmt.Sprintf("test %q was not found in go test output; "+
				"the harness may have failed to compile or an earlier test panicked", testName)
			continue
		}
		results[i].Passed = st.passed
		if !st.passed {
			// Store the raw t.Errorf output as RunError so the UI can display
			// the actual-vs-expected diff to the learner.
			results[i].RunError = strings.TrimSpace(st.output.String())
		}
	}

	return results, nil
}
