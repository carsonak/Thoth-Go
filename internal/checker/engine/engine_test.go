package engine_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thoth-go/thoth-go/internal/checker"
	"github.com/thoth-go/thoth-go/internal/checker/dynamic"
	"github.com/thoth-go/thoth-go/internal/checker/engine"
)

// ── Fixture helpers ───────────────────────────────────────────────────────────

// newExerciseDir writes source files + a go.mod into a temp directory.
// exercise.yaml is optional — pass it via files if needed; some tests use
// RunWithConfig directly and do not need it on disk.
func newExerciseDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	const goMod = "module testexercise\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("go.mod: %v", err)
	}

	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %q: %v", name, err)
		}
	}
	return dir
}

// shortTimeoutEngine returns an Engine with a 300 ms per-case timeout,
// used by tests that need the timeout path to complete quickly.
func shortTimeoutEngine(dir string) *engine.Engine {
	return engine.NewWithRunner(dir, &dynamic.Runner{Timeout: 300 * time.Millisecond})
}

// baseConfig returns a minimal valid ExerciseConfig in executable mode.
func baseConfig(id string, cases []checker.TestCase) *checker.ExerciseConfig {
	return &checker.ExerciseConfig{
		ID:        id,
		Title:     id,
		Topic:     "test",
		Mode:      checker.ModeExecutable,
		TestCases: cases,
	}
}

// ── Source fixtures ───────────────────────────────────────────────────────────

const helloMain = `package main

import "fmt"

func main() {
	fmt.Print("Hello, World!\n")
}
`

const forLoopMain = `package main

import "fmt"

func main() {
	for i := 0; i < 3; i++ {
		fmt.Println(i)
	}
}
`

const bannedImportMain = `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("hi")
	_ = os.Args
}
`

const funcSigMain = `package main

// Greet returns a greeting string.
func Greet(name string) string {
	return "Hello, " + name
}

func main() {}
`

const missingFuncMain = `package main

func main() {}
`

const infiniteSleepMain = `package main

import "time"

func main() {
	time.Sleep(1 * time.Hour)
}
`

// ── RunWithConfig tests ───────────────────────────────────────────────────────

func TestEngine_AllPass(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": helloMain})
	cfg := baseConfig("hello-001", []checker.TestCase{
		{ExpectedStdout: "Hello, World!\n", ExpectedExitCode: 0},
	})

	result, err := engine.New(dir).RunWithConfig(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.StaticPassed {
		t.Errorf("StaticPassed=false; violations: %v", result.StaticViolations)
	}
	if !result.DynamicPassed {
		t.Errorf("DynamicPassed=false; results: %v", result.TestResults)
	}
	if !result.AllPassed {
		t.Error("AllPassed should be true")
	}
}

func TestEngine_StaticFailure_HaltsBeforeDynamic(t *testing.T) {
	// The source uses `os` which will be in the banned list.
	dir := newExerciseDir(t, map[string]string{"main.go": bannedImportMain})

	cfg := &checker.ExerciseConfig{
		ID:    "banned-001",
		Title: "Banned Import",
		Topic: "test",
		Mode:  checker.ModeExecutable,
		Imports: checker.ImportRules{
			Banned: []string{"os"},
		},
		TestCases: []checker.TestCase{
			{ExpectedStdout: "hi\n", ExpectedExitCode: 0},
		},
	}

	result, err := engine.New(dir).RunWithConfig(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.StaticPassed {
		t.Error("StaticPassed should be false when banned import used")
	}
	// Engine must halt: no test results when static failed.
	if result.TestResults != nil {
		t.Errorf("TestResults should be nil on static failure, got %v", result.TestResults)
	}
	if result.AllPassed {
		t.Error("AllPassed should be false")
	}
}

func TestEngine_StaticBannedASTNode(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": forLoopMain})

	cfg := &checker.ExerciseConfig{
		ID:             "noloop-001",
		Title:          "No For Loop",
		Topic:          "test",
		Mode:           checker.ModeExecutable,
		BannedASTNodes: []string{"ast.ForStmt"},
	}

	result, err := engine.New(dir).RunWithConfig(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.StaticPassed {
		t.Error("StaticPassed should be false; for loop is banned")
	}
	if len(result.StaticViolations) == 0 {
		t.Error("expected at least one static violation")
	}
}

func TestEngine_DynamicFail_StdoutMismatch(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": helloMain})
	cfg := baseConfig("mismatch-001", []checker.TestCase{
		{ExpectedStdout: "Goodbye, World!\n", ExpectedExitCode: 0},
	})

	result, err := engine.New(dir).RunWithConfig(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.StaticPassed {
		t.Errorf("StaticPassed=false unexpectedly: %v", result.StaticViolations)
	}
	if result.DynamicPassed {
		t.Error("DynamicPassed should be false for stdout mismatch")
	}
	if result.AllPassed {
		t.Error("AllPassed should be false")
	}
}

func TestEngine_ModeFunctionSignature_MissingFunc(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": missingFuncMain})

	cfg := &checker.ExerciseConfig{
		ID:    "func-001",
		Title: "Greet Function",
		Topic: "test",
		Mode:  checker.ModeFunctionSignature,
		RequiredFunctions: []checker.FunctionSpec{
			{Name: "Greet", Signature: "func Greet(name string) string"},
		},
	}

	result, err := engine.New(dir).RunWithConfig(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.StaticPassed {
		t.Error("StaticPassed should be false when required function is missing")
	}
	if result.AllPassed {
		t.Error("AllPassed should be false")
	}
}

func TestEngine_ModeFunctionSignature_FuncPresent_StaticPasses(t *testing.T) {
	// funcSigMain declares Greet — static check should pass.
	// The config has no TestCases, so the dynamic phase returns nil (static-only
	// exercise) and AllPassed = true.
	dir := newExerciseDir(t, map[string]string{"main.go": funcSigMain})

	cfg := &checker.ExerciseConfig{
		ID:    "func-002",
		Title: "Greet Function",
		Topic: "test",
		Mode:  checker.ModeFunctionSignature,
		RequiredFunctions: []checker.FunctionSpec{
			{Name: "Greet", Signature: "func Greet(name string) string"},
		},
		// No TestCases: the exercise is graded by static checks only.
	}

	result, err := engine.New(dir).RunWithConfig(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.StaticPassed {
		t.Errorf("StaticPassed=false; violations: %v", result.StaticViolations)
	}
	// No TestCases → dynamic phase has nothing to do; TestResults is nil.
	if result.TestResults != nil {
		t.Errorf("expected nil TestResults for zero-test-case exercise, got %v", result.TestResults)
	}
	// AllPassed = StaticPassed && allPassed(nil) = true && true = true.
	if !result.AllPassed {
		t.Error("AllPassed should be true: static passed and no test cases to fail")
	}
}

func TestEngine_NoTestCases_StaticOnly(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": helloMain})
	cfg := &checker.ExerciseConfig{
		ID:    "static-only-001",
		Title: "Static Only",
		Topic: "test",
		Mode:  checker.ModeExecutable,
		// No TestCases — grading is purely by static rules.
	}

	result, err := engine.New(dir).RunWithConfig(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.StaticPassed {
		t.Errorf("StaticPassed=false; violations: %v", result.StaticViolations)
	}
	if result.TestResults != nil {
		t.Errorf("TestResults should be nil for empty test cases, got %v", result.TestResults)
	}
	if !result.AllPassed {
		t.Error("AllPassed should be true when static passes and no test cases")
	}
}

func TestEngine_DynamicTimeout_Detected(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": infiniteSleepMain})
	cfg := baseConfig("timeout-e-001", []checker.TestCase{
		{ExpectedStdout: "", ExpectedExitCode: 0},
	})

	eng := shortTimeoutEngine(dir)
	result, err := eng.RunWithConfig(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.TestResults) != 1 {
		t.Fatalf("len(TestResults)=%d, want 1", len(result.TestResults))
	}
	if !result.TestResults[0].TimedOut {
		t.Error("expected TimedOut=true for infinite sleep")
	}
	if result.DynamicPassed {
		t.Error("DynamicPassed should be false on timeout")
	}
}

// ── Run (loads exercise.yaml from disk) ──────────────────────────────────────

func TestEngine_Run_LoadsYAMLFromDisk(t *testing.T) {
	const yaml = `id: disk-001
title: Disk Test
topic: test
mode: executable
test_cases:
  - expected_stdout: "Hello, World!\n"
    expected_exit_code: 0
`
	dir := newExerciseDir(t, map[string]string{
		"main.go":       helloMain,
		"exercise.yaml": yaml,
	})

	result, err := engine.New(dir).Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.AllPassed {
		t.Errorf("AllPassed=false; violations=%v results=%v", result.StaticViolations, result.TestResults)
	}
}

func TestEngine_Run_MissingYAML_ReturnsError(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": helloMain})
	// Deliberately no exercise.yaml.

	_, err := engine.New(dir).Run()
	if err == nil {
		t.Fatal("expected error for missing exercise.yaml, got nil")
	}
}
