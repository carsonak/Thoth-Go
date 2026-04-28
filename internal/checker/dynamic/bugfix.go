// Package dynamic — bugfix.go: diff-based validator for ModeBugFix exercises.
//
// # The Bug-Fix Pattern
//
// In a bug-fix exercise the learner is given a file with a deliberate defect
// (e.g. an off-by-one error, a wrong operator, a missing return) and must
// correct it. The challenge has two layers:
//
//  1. Structural integrity: the learner must only touch the lines the exercise
//     allows — they must not rewrite the entire solution or remove unrelated
//     code. This is enforced by the diff check.
//
//  2. Correctness: after the fix, the binary must produce the expected output.
//     This is enforced by running the TestCases (same as ModeExecutable).
//
// Separating these two concerns matters pedagogically: a learner who submits
// the correct output but rewrites the whole function has not learned how to
// identify and isolate a bug — which is the skill the exercise is designed to
// teach.
//
// # Structural vs. Semantic Diff
//
// We use a structural (line-level) diff rather than a semantic (AST-level)
// diff for two reasons:
//
//  1. Simplicity: line diffs are language-agnostic and easy to explain to
//     learners. "You changed line 7 but only line 5 is allowed" is
//     immediately actionable.
//
//  2. Robustness: AST-level diffing requires a complete, parseable file.
//     A learner who introduces a syntax error while debugging can still
//     receive the "wrong line modified" feedback via a text diff.
//
// # diffmatchpatch
//
// The github.com/sergi/go-diff/diffmatchpatch library provides the Myers diff
// algorithm (the same algorithm used by `git diff`). We use its line-level
// mode (DiffLinesToChars → DiffMain → DiffCharsToLines) which is O(n) in the
// number of lines rather than O(n²) in the number of characters.

package dynamic

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/thoth-go/thoth-go/internal/checker"
)

// runBugFix implements the ModeBugFix evaluation strategy:
//
//  1. Load the learner's submitted file and the reference (correct) file.
//  2. Compute a line-level diff of learner → reference.
//  3. Validate that every modified line falls within cfg.BugFix.RestrictedLines.
//     If not, return an immediate failure explaining which line was out-of-bounds.
//  4. If the diff is valid, fall back to the executable pipeline: compile the
//     learner's package and run each TestCase.
//
// # Fail-Fast Principle
//
// We perform the diff check before compilation and test execution. This is the
// "fail fast" principle: surface the cheapest, most actionable error first.
// Compiling a file with the wrong lines changed would produce either a
// compilation error (if the out-of-bounds change broke syntax) or a
// passing-but-wrong test — neither of which explains the real problem to the
// learner. The diff check gives a precise, targeted error message.
func (r *Runner) runBugFix(cfg *checker.ExerciseConfig, dir string) ([]TestResult, error) {
	// Determine which file the learner is submitting.
	// BugFixSpec.SubmitFile is optional; "main.go" is the conventional default
	// for single-file exercises.
	submitFile := cfg.BugFix.SubmitFile
	if submitFile == "" {
		submitFile = "main.go"
	}

	learnerPath := filepath.Join(dir, submitFile)
	referencePath := filepath.Join(dir, cfg.BugFix.ReferenceFile)

	learnerBytes, err := os.ReadFile(learnerPath)
	if err != nil {
		return nil, fmt.Errorf("bugfix: reading submit file %q: %w", submitFile, err)
	}
	referenceBytes, err := os.ReadFile(referencePath)
	if err != nil {
		return nil, fmt.Errorf("bugfix: reading reference file %q: %w", cfg.BugFix.ReferenceFile, err)
	}

	// ── Structural diff check ────────────────────────────────────────────────
	//
	// computeModifiedLines returns the 1-based line numbers in the learner's
	// file that differ from the reference. "Modified" here means lines present
	// in the learner's file that do not appear in the same position in the
	// reference (Insert operations in the diff).
	modifiedLines := computeModifiedLines(string(learnerBytes), string(referenceBytes))

	// Validate only when RestrictedLines is non-empty.  An empty slice means
	// "all lines are freely modifiable" — useful for exercises that want to
	// test the diff-plus-run pipeline without restricting which lines change.
	if len(cfg.BugFix.RestrictedLines) > 0 {
		// Build a fast lookup set from the allowed line numbers.
		allowed := make(map[int]struct{}, len(cfg.BugFix.RestrictedLines))
		for _, l := range cfg.BugFix.RestrictedLines {
			allowed[l] = struct{}{}
		}

		for _, line := range modifiedLines {
			if _, ok := allowed[line]; !ok {
				return []TestResult{{
					Index:  0,
					Passed: false,
					RunError: fmt.Sprintf(
						"line %d was modified but falls outside the allowed fix zone %v; "+
							"only modify the lines indicated by the exercise description",
						line, cfg.BugFix.RestrictedLines,
					),
				}}, nil
			}
		}
	}

	// ── Run test cases ───────────────────────────────────────────────────────
	//
	// The diff is structurally valid; now verify the fix is functionally
	// correct by running the standard executable pipeline. The reference file
	// lives in a subdirectory (e.g. solution/) so `go build .` only compiles
	// the learner's files in the root directory.
	return r.runExecutable(cfg, dir)
}

// computeModifiedLines returns the 1-based line numbers in learner's text that
// differ from reference, using a line-level Myers diff.
//
// # How DiffLinesToChars works
//
// diffmatchpatch's DiffLinesToChars converts each unique line to a single
// Unicode character, reducing a large text diff to a short character diff.
// DiffMain then runs the Myers algorithm on these short character strings —
// O(n·d) where n = number of unique lines and d = edit distance.
// DiffCharsToLines converts the result back to human-readable line text.
//
// # Direction convention
//
// We call DiffMain(reference, learner) — transforming the correct reference
// into the learner's submission. In the resulting diff:
//
//   - DiffEqual:  line is the same in both → not modified.
//   - DiffDelete: line is in reference but not in learner → learner deleted it.
//   - DiffInsert: line is in learner but not in reference → learner added/changed it.
//
// "Modified lines" in the learner's file are the Insert chunks, because those
// are the lines present in the learner's file that did not come from the
// reference. We track their 1-based line numbers in the learner file.
//
// Note: files are assumed to use Unix line endings (\n). Most Go tooling
// (including gofmt) normalises line endings, so this is safe in practice.
func computeModifiedLines(learner, reference string) []int {
	dmp := diffmatchpatch.New()

	// Encode lines as single characters for efficient diffing.
	refChars, learnerChars, lineArray := dmp.DiffLinesToChars(reference, learner)
	diffs := dmp.DiffMain(refChars, learnerChars, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)

	var modified []int
	learnerLine := 1 // 1-based cursor tracking our position in the learner's file

	for _, d := range diffs {
		// Count the number of complete lines in this diff chunk.
		// Each line (including trailing \n) contributes exactly one to the count.
		n := strings.Count(d.Text, "\n")

		switch d.Type {
		case diffmatchpatch.DiffEqual:
			// Lines identical in both files — advance learner cursor past them.
			learnerLine += n

		case diffmatchpatch.DiffInsert:
			// Lines present in learner but not reference — these are the
			// learner's added/changed lines. Record their line numbers.
			for i := 0; i < n; i++ {
				modified = append(modified, learnerLine+i)
			}
			learnerLine += n

		case diffmatchpatch.DiffDelete:
			// Lines from reference that do not appear in learner — the learner
			// deleted them. These lines do not exist in the learner's file so
			// they have no learner line number to record.
			// Do NOT advance learnerLine.
		}
	}

	return modified
}
