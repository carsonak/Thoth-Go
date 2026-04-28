package cli

// cmd_check.go implements the `thoth-go check` command.
//
// This is the most important command — it runs the full grading pipeline and
// is the primary feedback loop for the learner.
//
// ARCHITECTURE NOTE — UI as the Presentation Layer:
// The command does not format strings directly. Every line of output goes
// through ui.Default (a *ui.Renderer). This is the Presentation Layer
// principle: the command is a controller that translates domain results
// (CheckResult) into UI calls. Changing the output style only requires
// editing ui/ui.go, not hunting through command logic.

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/thoth-go/thoth-go/internal/checker/engine"
	"github.com/thoth-go/thoth-go/internal/ui"
)

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Run static analysis and tests against the current exercise",
		Long: `Run the full grading pipeline for the exercise in the current directory:

  1. Static analysis — import rules, banned AST nodes, banned functions.
  2. Dynamic tests  — compile the package and run each test case.

Results are printed to the terminal and your progress is saved automatically.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			ui.Default.Banner("  Thoth-Go  ")

			// Run the full grading pipeline.
			//
			// ARCHITECTURE NOTE — Engine as a Black Box:
			// The command passes the directory to the Engine and receives a
			// CheckResult. It does not know about static.Checker or
			// dynamic.Runner — those are implementation details of the engine
			// package. This is the Facade pattern in action: a complex
			// subsystem is exposed through a single, simple interface.
			eng := engine.New(cwd)
			result, err := eng.Run()
			if err != nil {
				ui.Default.Error("Check failed: %v", err)
				return err
			}

			// ── Static analysis results ──────────────────────────────────
			if !result.StaticPassed {
				ui.Default.SectionHeader("Static Analysis")
				for _, v := range result.StaticViolations {
					ui.Default.StaticError(v.Rule, v.Message)
				}
				ui.Default.Error("Fix the violations above before running tests.")
				// Return nil, not an error — a check failure is not a CLI
				// error (exit 1 would confuse scripts that check the exit code
				// to see if the command ran successfully).
				return nil
			}

			// ── Dynamic test results ─────────────────────────────────────
			ui.Default.SectionHeader("Test Cases")

			if len(result.TestResults) == 0 {
				ui.Default.Info("No test cases defined for this exercise.")
			} else {
				passed := 0
				for i, tr := range result.TestResults {
					name := testCaseName(i, tr.TestCase.Description)

					if tr.RunError != "" {
						// Build failure or runtime error — show as a special fail.
						ui.Default.TestFail(name, "(expected output)", tr.RunError)
						continue
					}

					if tr.TimedOut {
						ui.Default.TestFail(name, fmt.Sprintf("(exit within %s)", tr.TestCase.Description), "timed out")
						continue
					}

					if tr.Passed {
						passed++
						ui.Default.TestPass(name)
					} else {
						// Hidden test cases: do not reveal expected output.
						if tr.TestCase.Hidden {
							ui.Default.TestFail("(hidden test)", "—", "output did not match")
						} else {
							ui.Default.TestFail(name, tr.TestCase.ExpectedStdout, tr.ActualOut)
						}
					}
				}
				ui.Default.Summary(passed, len(result.TestResults))
			}

			// ── Update progress state ────────────────────────────────────
			//
			// State updates are best-effort: if the state file cannot be
			// written (e.g. disk full), we log a warning but do not fail the
			// check command — the learner already got their feedback.
			ps, statePath, stateErr := loadOrNewState()
			if stateErr == nil {
				exerciseID := activeExerciseID(ps, cwd)
				if exerciseID != "" {
					ps.MarkChecked(exerciseID)
					if result.AllPassed {
						ps.MarkCompleted(exerciseID)
						ui.Default.Success("Exercise complete! 🎉")
					}
					if saveErr := ps.Save(statePath); saveErr != nil {
						ui.Default.Warning("Could not save progress: %v", saveErr)
					}
				}
			}

			return nil
		},
	}
}
