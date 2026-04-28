package dynamic_test

// harness_test.go tests the ModeFunctionSignature evaluation strategy
// (internal/checker/dynamic/harness.go).
//
// # Test philosophy
//
// These tests write small Go *library* packages (not main packages) to temp
// directories, construct an ExerciseConfig with ModeFunctionSignature, and
// invoke dynamic.Runner.Run. They exercise the full harness pipeline:
//
//   1. Package name detection via go/parser.
//   2. harness_test.go generation.
//   3. `go test -json` invocation.
//   4. JSON event stream parsing → []TestResult.
//
// Because the harness compiles real Go code, these tests are integration-style
// and take a few seconds each (one `go test` subprocess per test). They are
// worth the cost: end-to-end coverage is the only way to verify that the
// generated Go code is syntactically and semantically valid.

import (
	"testing"

	"github.com/thoth-go/thoth-go/internal/checker"
	"github.com/thoth-go/thoth-go/internal/checker/dynamic"
)

// ── Source fixtures ───────────────────────────────────────────────────────────

// additionSrc is a correct implementation of Add(a, b int) int.
const additionSrc = `package addition

// Add returns the sum of a and b.
func Add(a, b int) int {
	return a + b
}
`

// additionWrongSrc has a bug: uses subtraction instead of addition.
const additionWrongSrc = `package addition

// Add has a bug: returns a - b instead of a + b.
func Add(a, b int) int {
	return a - b
}
`

// additionBadSyntaxSrc does not compile — tests build-failure path.
const additionBadSyntaxSrc = `package addition

INVALID GO SYNTAX HERE
`

// greetSrc implements Greet(name string) string (returns a string).
const greetSrc = `package greet

// Greet returns a personalised greeting.
func Greet(name string) string {
	return "Hello, " + name + "!"
}
`

// ── Helpers ──────────────────────────────────────────────────────────────────

// funcSigCfg builds a ModeFunctionSignature ExerciseConfig.
func funcSigCfg(funcName, sig string, cases []checker.TestCase) *checker.ExerciseConfig {
	return &checker.ExerciseConfig{
		ID:    "fs-test",
		Title: "Function Sig Test",
		Topic: "test",
		Mode:  checker.ModeFunctionSignature,
		RequiredFunctions: []checker.FunctionSpec{
			{Name: funcName, Signature: sig},
		},
		TestCases: cases,
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestFunctionSignature_CorrectImplementation_Passes(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"add.go": additionSrc})
	cfg := funcSigCfg("Add", "func Add(a, b int) int", []checker.TestCase{
		{Args: []string{"3", "4"}, ExpectedStdout: "7\n"},
		{Args: []string{"-1", "1"}, ExpectedStdout: "0\n"},
		{Args: []string{"0", "0"}, ExpectedStdout: "0\n"},
	})

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	for i, res := range results {
		if !res.Passed {
			t.Errorf("case %d: expected Passed=true; RunError=%q", i, res.RunError)
		}
	}
}

func TestFunctionSignature_WrongImplementation_Fails(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"add.go": additionWrongSrc})
	cfg := funcSigCfg("Add", "func Add(a, b int) int", []checker.TestCase{
		{Args: []string{"3", "4"}, ExpectedStdout: "7\n"},
	})

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Passed {
		t.Error("expected Passed=false for wrong implementation (a-b instead of a+b)")
	}
	if results[0].RunError == "" {
		t.Error("expected RunError to contain t.Errorf message on failure")
	}
}

func TestFunctionSignature_BuildFailure_SurfacedAsResult(t *testing.T) {
	// A file with syntax errors causes `go test` to fail with a build error.
	// The runner must NOT return a Go error; it must return TestResults with
	// RunError set so the UI can display the compiler output.
	dir := newExerciseDir(t, map[string]string{"add.go": additionBadSyntaxSrc})
	cfg := funcSigCfg("Add", "func Add(a, b int) int", []checker.TestCase{
		{Args: []string{"1", "2"}, ExpectedStdout: "3\n"},
	})

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
		t.Error("expected RunError to contain compiler output for build failure")
	}
}

func TestFunctionSignature_StringReturnValue_Passes(t *testing.T) {
	// Verifies that the %v\n formatting works correctly for string return values.
	// fmt.Sprintf("%v\n", "Hello, Alice!") == "Hello, Alice!\n"
	dir := newExerciseDir(t, map[string]string{"greet.go": greetSrc})
	cfg := funcSigCfg("Greet", "func Greet(name string) string", []checker.TestCase{
		// Args are Go source expressions — the string literals include quotes.
		{Args: []string{`"Alice"`}, ExpectedStdout: "Hello, Alice!\n"},
		{Args: []string{`"Bob"`}, ExpectedStdout: "Hello, Bob!\n"},
	})

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for i, res := range results {
		if !res.Passed {
			t.Errorf("case %d: expected Passed=true; RunError=%q", i, res.RunError)
		}
	}
}

func TestFunctionSignature_MultipleTestCases_MixedResults(t *testing.T) {
	// additionWrongSrc uses subtraction, so Add(3,4)=-1≠7 (fail)
	// but Add(0,0)=0==0 (pass because 0-0=0 accidentally matches).
	dir := newExerciseDir(t, map[string]string{"add.go": additionWrongSrc})
	cfg := funcSigCfg("Add", "func Add(a, b int) int", []checker.TestCase{
		{Args: []string{"3", "4"}, ExpectedStdout: "7\n"},   // fail: -1 ≠ 7
		{Args: []string{"0", "0"}, ExpectedStdout: "0\n"},   // pass: 0-0=0
		{Args: []string{"5", "3"}, ExpectedStdout: "8\n"},   // fail: 2 ≠ 8
	})

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if results[0].Passed {
		t.Error("case 0: expected fail (3-4=-1 ≠ 7)")
	}
	if !results[1].Passed {
		t.Errorf("case 1: expected pass (0-0=0 == 0); RunError=%q", results[1].RunError)
	}
	if results[2].Passed {
		t.Error("case 2: expected fail (5-3=2 ≠ 8)")
	}
}

func TestFunctionSignature_IndexTracking(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"add.go": additionSrc})
	cfg := funcSigCfg("Add", "func Add(a, b int) int", []checker.TestCase{
		{Args: []string{"1", "2"}, ExpectedStdout: "3\n"},
		{Args: []string{"4", "5"}, ExpectedStdout: "9\n"},
	})

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
