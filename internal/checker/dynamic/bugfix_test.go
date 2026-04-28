package dynamic_test

// bugfix_test.go tests the ModeBugFix evaluation strategy
// (internal/checker/dynamic/bugfix.go).
//
// # Test design notes
//
// Bug-fix exercises have two independent validation layers:
//
//  1. Structural diff check: which lines did the learner modify?
//  2. Functional test cases: does the fixed code produce the correct output?
//
// The tests below cover each layer independently (unit-test style) and also
// test the composition (diff passes → tests run → all pass).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thoth-go/thoth-go/internal/checker"
	"github.com/thoth-go/thoth-go/internal/checker/dynamic"
)

// ── Fixtures ──────────────────────────────────────────────────────────────────

// loopBuggyMain has an off-by-one: the loop condition is i <= 3 (should be 5).
// This is the file the learner starts with and is expected to fix.
const loopBuggyMain = `package main

import "fmt"

func main() {
	for i := 1; i <= 3; i++ {
		fmt.Println(i)
	}
}
`

// loopFixedMain is the correct version (the "reference" / solution).
const loopFixedMain = `package main

import "fmt"

func main() {
	for i := 1; i <= 5; i++ {
		fmt.Println(i)
	}
}
`

// loopWrongLineMain has the correct loop condition but changed a different line
// (the fmt.Println call). This should trigger the "wrong line modified" error
// when RestrictedLines = [6].
const loopWrongLineMain = `package main

import "fmt"

func main() {
	for i := 1; i <= 5; i++ {
		fmt.Printf("%d\n", i)
	}
}
`

// loopExpectedOutput is "1\n2\n3\n4\n5\n".
const loopExpectedOutput = "1\n2\n3\n4\n5\n"

// ── writeRef writes a reference file into a subdirectory of dir ──────────────

func writeRef(t *testing.T, dir, subdir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, subdir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// bugFixCfg builds a minimal ModeBugFix ExerciseConfig.
func bugFixCfg(submitFile, refFile string, allowedLines []int, cases []checker.TestCase) *checker.ExerciseConfig {
	return &checker.ExerciseConfig{
		ID:    "bf-test",
		Title: "Bug Fix Test",
		Topic: "test",
		Mode:  checker.ModeBugFix,
		BugFix: checker.BugFixSpec{
			SubmitFile:      submitFile,
			ReferenceFile:   refFile,
			RestrictedLines: allowedLines,
		},
		TestCases: cases,
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestBugFix_ValidFixOnAllowedLine_Passes tests the golden path: the learner
// modifies exactly line 6 (the loop condition) which is the allowed line, and
// the fix produces the correct output.
func TestBugFix_ValidFixOnAllowedLine_Passes(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": loopFixedMain})
	writeRef(t, dir, "solution", "main.go", loopFixedMain)

	cfg := bugFixCfg("main.go", "solution/main.go", []int{6}, []checker.TestCase{
		{ExpectedStdout: loopExpectedOutput, ExpectedExitCode: 0},
	})

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if !results[0].Passed {
		t.Errorf("expected Passed=true for valid fix; RunError=%q", results[0].RunError)
	}
}

// TestBugFix_BuggySubmission_TestCasesFail checks that when the submitted file
// still has the bug (unchanged from starter), the diff passes (we compare buggy
// against itself via loopBuggyMain as reference), but the test cases fail
// because the output is wrong.
func TestBugFix_BuggySubmission_TestCasesFail(t *testing.T) {
	// Both submit and reference are the buggy version → diff shows no changes.
	dir := newExerciseDir(t, map[string]string{"main.go": loopBuggyMain})
	writeRef(t, dir, "solution", "main.go", loopBuggyMain)

	cfg := bugFixCfg("main.go", "solution/main.go", []int{6}, []checker.TestCase{
		{ExpectedStdout: loopExpectedOutput, ExpectedExitCode: 0},
	})

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Passed {
		t.Error("expected Passed=false: buggy submission produces wrong output")
	}
}

// TestBugFix_ModificationOutsideAllowedZone_Fails verifies the structural diff
// check. The learner changed line 7 (fmt.Println → fmt.Printf) but RestrictedLines
// only permits line 6.
func TestBugFix_ModificationOutsideAllowedZone_Fails(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": loopWrongLineMain})
	writeRef(t, dir, "solution", "main.go", loopFixedMain)

	cfg := bugFixCfg("main.go", "solution/main.go", []int{6}, []checker.TestCase{
		{ExpectedStdout: loopExpectedOutput, ExpectedExitCode: 0},
	})

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Passed {
		t.Error("expected Passed=false: modification is on wrong line")
	}
	if results[0].RunError == "" {
		t.Error("expected RunError to name the out-of-bounds line")
	}
}

// TestBugFix_EmptyRestrictedLines_AnyModificationAllowed verifies that an
// empty RestrictedLines slice disables the line-restriction check entirely.
func TestBugFix_EmptyRestrictedLines_AnyModificationAllowed(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": loopFixedMain})
	writeRef(t, dir, "solution", "main.go", loopFixedMain)

	// No RestrictedLines → any modification is allowed.
	cfg := bugFixCfg("main.go", "solution/main.go", nil, []checker.TestCase{
		{ExpectedStdout: loopExpectedOutput, ExpectedExitCode: 0},
	})

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !results[0].Passed {
		t.Errorf("expected Passed=true with no RestrictedLines; RunError=%q", results[0].RunError)
	}
}

// TestBugFix_MissingSubmitFile_ReturnsError checks that a hard error (missing
// file) is propagated as a Go error, not silently swallowed.
func TestBugFix_MissingSubmitFile_ReturnsError(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{})
	writeRef(t, dir, "solution", "main.go", loopFixedMain)

	cfg := bugFixCfg("main.go", "solution/main.go", []int{6}, []checker.TestCase{
		{ExpectedStdout: loopExpectedOutput},
	})

	_, err := dynamic.New().Run(cfg, dir)
	if err == nil {
		t.Fatal("expected error for missing submit file, got nil")
	}
}

// TestBugFix_DefaultSubmitFilename verifies that an empty SubmitFile defaults
// to "main.go" by successfully loading it when present.
func TestBugFix_DefaultSubmitFilename(t *testing.T) {
	dir := newExerciseDir(t, map[string]string{"main.go": loopFixedMain})
	writeRef(t, dir, "solution", "main.go", loopFixedMain)

	// BugFixSpec with empty SubmitFile — should default to "main.go".
	cfg := &checker.ExerciseConfig{
		ID:    "default-submit",
		Title: "Default Submit",
		Topic: "test",
		Mode:  checker.ModeBugFix,
		BugFix: checker.BugFixSpec{
			SubmitFile:    "", // empty → defaults to main.go
			ReferenceFile: "solution/main.go",
		},
		TestCases: []checker.TestCase{
			{ExpectedStdout: loopExpectedOutput},
		},
	}

	results, err := dynamic.New().Run(cfg, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !results[0].Passed {
		t.Errorf("expected Passed=true; RunError=%q", results[0].RunError)
	}
}
