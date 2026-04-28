package dynamic_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thoth-go/thoth-go/internal/checker"
	"github.com/thoth-go/thoth-go/internal/checker/dynamic"
)

// ── Test fixture helpers ──────────────────────────────────────────────────────

// newExerciseDir writes Go source files and a go.mod into a temp directory,
// returning its path.  A go.mod is required because `go build .` operates in
// module-aware mode by default.
func newExerciseDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	// Minimal go.mod — module name does not matter for a standalone binary.
	goMod := "module testexercise\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("writing go.mod: %v", err)
	}

	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll %q: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %q: %v", name, err)
		}
	}
	return dir
}

// fastRunner returns a Runner with a short per-case timeout, suitable for
// tests that exercise the timeout path.
func fastRunner(timeout time.Duration) *dynamic.Runner {
	return &dynamic.Runner{Timeout: timeout}
}

// ── Fixture programs ──────────────────────────────────────────────────────────

// helloWorldMain prints "Hello, World!\n" and exits 0.
const helloWorldMain = `package main

import "fmt"

func main() {
	fmt.Print("Hello, World!\n")
}
`

// echoArgsMain prints its first arg followed by a newline, exits 0.
const echoArgsMain = `package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		fmt.Printf("%s\n", os.Args[1])
	}
}
`

// echoStdinMain reads one line from stdin and prints it back.
const echoStdinMain = `package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		fmt.Println(scanner.Text())
	}
}
`

// exitOneMain exits with code 1 without printing anything.
const exitOneMain = `package main

import "os"

func main() {
	os.Exit(1)
}
`

// infiniteLoopMain sleeps for a very long time. We use time.Sleep rather than
// select{} because select{} with no cases triggers Go's deadlock detector,
// causing the runtime to exit immediately instead of blocking — which would
// defeat the purpose of the timeout test.
const infiniteLoopMain = `package main

import "time"

func main() {
	time.Sleep(1 * time.Hour)
}
`

// badSyntaxMain will not compile — used to test build-failure handling.
const badSyntaxMain = `package main

func main() {
	THIS IS NOT VALID GO SYNTAX
}
`

// ── Build ─────────────────────────────────────────────────────────────────────

func TestBuild_SucceedsOnValidSource(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": helloWorldMain})
	r := dynamic.New()

	binPath, cleanup, err := r.Build(dir)
	defer cleanup()

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if binPath == "" {
		t.Fatal("Build returned empty binPath")
	}
	if _, statErr := os.Stat(binPath); os.IsNotExist(statErr) {
		t.Errorf("binary %q does not exist after Build", binPath)
	}
}

func TestBuild_FailsOnBadSyntax(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": badSyntaxMain})
	r := dynamic.New()

	_, cleanup, err := r.Build(dir)
	defer cleanup()

	if err == nil {
		t.Fatal("expected Build to fail for bad syntax, got nil error")
	}
}

// ── Run — ModeExecutable ──────────────────────────────────────────────────────

func TestRun_StdoutMatch_Passes(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": helloWorldMain})
	cfg := &checker.ExerciseConfig{
		ID:    "hello-001",
		Title: "Hello World",
		Topic: "basics",
		Mode:  checker.ModeExecutable,
		TestCases: []checker.TestCase{
			{ExpectedStdout: "Hello, World!\n", ExpectedExitCode: 0},
		},
	}

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected Passed=true; ActualOut=%q RunError=%q", results[0].ActualOut, results[0].RunError)
	}
}

func TestRun_StdoutMismatch_Fails(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": helloWorldMain})
	cfg := &checker.ExerciseConfig{
		ID:    "hello-002",
		Title: "Hello World Wrong",
		Topic: "basics",
		Mode:  checker.ModeExecutable,
		TestCases: []checker.TestCase{
			{ExpectedStdout: "Goodbye, World!\n", ExpectedExitCode: 0},
		},
	}

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if results[0].Passed {
		t.Error("expected Passed=false for stdout mismatch")
	}
	if results[0].ActualOut != "Hello, World!\n" {
		t.Errorf("ActualOut = %q, want %q", results[0].ActualOut, "Hello, World!\n")
	}
}

func TestRun_ExitCodeMismatch_Fails(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": exitOneMain})
	cfg := &checker.ExerciseConfig{
		ID:    "exit-001",
		Title: "Exit Code",
		Topic: "basics",
		Mode:  checker.ModeExecutable,
		TestCases: []checker.TestCase{
			// We expect exit code 0 but the program exits 1.
			{ExpectedStdout: "", ExpectedExitCode: 0},
		},
	}

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if results[0].Passed {
		t.Error("expected Passed=false for exit code mismatch")
	}
	if results[0].ActualCode != 1 {
		t.Errorf("ActualCode = %d, want 1", results[0].ActualCode)
	}
}

func TestRun_CorrectExitCode_Passes(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": exitOneMain})
	cfg := &checker.ExerciseConfig{
		ID:    "exit-002",
		Title: "Expected Non-Zero Exit",
		Topic: "basics",
		Mode:  checker.ModeExecutable,
		TestCases: []checker.TestCase{
			{ExpectedStdout: "", ExpectedExitCode: 1},
		},
	}

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !results[0].Passed {
		t.Errorf("expected Passed=true; ActualCode=%d RunError=%q", results[0].ActualCode, results[0].RunError)
	}
}

func TestRun_CommandLineArgs_Passed(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": echoArgsMain})
	cfg := &checker.ExerciseConfig{
		ID:    "args-001",
		Title: "Echo Args",
		Topic: "basics",
		Mode:  checker.ModeExecutable,
		TestCases: []checker.TestCase{
			{Args: []string{"thoth"}, ExpectedStdout: "thoth\n", ExpectedExitCode: 0},
		},
	}

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !results[0].Passed {
		t.Errorf("expected Passed=true; ActualOut=%q", results[0].ActualOut)
	}
}

func TestRun_StdinPiped(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": echoStdinMain})
	cfg := &checker.ExerciseConfig{
		ID:    "stdin-001",
		Title: "Echo Stdin",
		Topic: "basics",
		Mode:  checker.ModeExecutable,
		TestCases: []checker.TestCase{
			{Stdin: "hello from stdin", ExpectedStdout: "hello from stdin\n", ExpectedExitCode: 0},
		},
	}

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !results[0].Passed {
		t.Errorf("expected Passed=true; ActualOut=%q", results[0].ActualOut)
	}
}

func TestRun_Timeout_SetsTimedOut(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": infiniteLoopMain})
	cfg := &checker.ExerciseConfig{
		ID:    "timeout-001",
		Title: "Infinite Loop",
		Topic: "basics",
		Mode:  checker.ModeExecutable,
		TestCases: []checker.TestCase{
			{ExpectedStdout: "", ExpectedExitCode: 0},
		},
	}

	// Inject a very short timeout so the test completes quickly.
	r := fastRunner(200 * time.Millisecond)
	results, err := r.Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !results[0].TimedOut {
		t.Error("expected TimedOut=true for infinite loop")
	}
	if results[0].Passed {
		t.Error("expected Passed=false for timed-out case")
	}
}

func TestRun_MultipleTestCases(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": echoArgsMain})
	cfg := &checker.ExerciseConfig{
		ID:    "multi-001",
		Title: "Multi Case",
		Topic: "basics",
		Mode:  checker.ModeExecutable,
		TestCases: []checker.TestCase{
			{Args: []string{"alpha"}, ExpectedStdout: "alpha\n", ExpectedExitCode: 0},
			{Args: []string{"beta"}, ExpectedStdout: "wrong\n", ExpectedExitCode: 0}, // deliberate fail
			{Args: []string{"gamma"}, ExpectedStdout: "gamma\n", ExpectedExitCode: 0},
		},
	}

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if !results[0].Passed {
		t.Errorf("case 0: expected pass")
	}
	if results[1].Passed {
		t.Errorf("case 1: expected fail (wrong expected stdout)")
	}
	if !results[2].Passed {
		t.Errorf("case 2: expected pass")
	}
}

func TestRun_BuildFailure_SurfacedAsResult(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": badSyntaxMain})
	cfg := &checker.ExerciseConfig{
		ID:    "build-fail-001",
		Title: "Build Fail",
		Topic: "basics",
		Mode:  checker.ModeExecutable,
		TestCases: []checker.TestCase{
			{ExpectedStdout: "anything", ExpectedExitCode: 0},
		},
	}

	// Run must NOT return an error — the build failure should be surfaced as a
	// TestResult with RunError set so the UI can render it.
	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Passed {
		t.Error("expected Passed=false for build failure")
	}
	if results[0].RunError == "" {
		t.Error("expected RunError to be set for build failure")
	}
}

func TestRun_EmptyTestCases_ReturnsNil(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": helloWorldMain})
	cfg := &checker.ExerciseConfig{
		ID:    "no-tests-001",
		Title: "No Tests",
		Topic: "basics",
		Mode:  checker.ModeExecutable,
		// Deliberately no TestCases.
	}

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty TestCases, got %v", results)
	}
}

// ── Run — ModeFunctionSignature (no test cases) ───────────────────────────────

// TestRun_ModeFunctionSignature_NoTestCases documents that function_signature
// mode with an empty TestCases slice returns nil results (static-only check),
// consistent with the ModeExecutable behaviour for zero test cases.
func TestRun_ModeFunctionSignature_NoTestCases(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": helloWorldMain})
	cfg := &checker.ExerciseConfig{
		ID:                "func-sig-001",
		Title:             "Function Sig No Cases",
		Topic:             "basics",
		Mode:              checker.ModeFunctionSignature,
		RequiredFunctions: []checker.FunctionSpec{{Name: "Greet", Signature: "func Greet(name string) string"}},
		// Deliberately no TestCases — the exercise relies on static checks only.
	}

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run (ModeFunctionSignature, no test cases): %v", err)
	}
	// No TestCases → no dynamic checks → nil slice (matches ModeExecutable behaviour).
	if results != nil {
		t.Errorf("expected nil results for empty TestCases, got %v", results)
	}
}

// ── TestResult index tracking ─────────────────────────────────────────────────

func TestRun_ResultIndexMatchesSlicePosition(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": echoArgsMain})
	cfg := &checker.ExerciseConfig{
		ID:    "index-001",
		Title: "Index Check",
		Topic: "basics",
		Mode:  checker.ModeExecutable,
		TestCases: []checker.TestCase{
			{Args: []string{"a"}, ExpectedStdout: "a\n"},
			{Args: []string{"b"}, ExpectedStdout: "b\n"},
			{Args: []string{"c"}, ExpectedStdout: "c\n"},
		},
	}

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for i, res := range results {
		if res.Index != i {
			t.Errorf("results[%d].Index = %d, want %d", i, res.Index, i)
		}
	}
}
