package cli

// cmd_start.go implements the `thoth-go start <exercise-id>` command.
//
// Responsibilities:
//  1. Copy the pristine exercise files from the local cache into the CWD.
//  2. Mark the exercise as "in_progress" in the progress state.
//
// ARCHITECTURE NOTE — Orchestration, Not Logic:
// The command is a thin orchestrator: it calls repository.Manager.Reset to
// copy files and state.MarkStarted to update progress. Neither of those
// functions knows about the other. The command is the only place they are
// composed together — a classic example of the Orchestrator pattern, where
// the coordinator (this command) has no logic of its own beyond sequencing
// calls to specialised components.

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/thoth-go/thoth-go/internal/repository"
	"github.com/thoth-go/thoth-go/internal/ui"
)

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <exercise-id>",
		Short: "Copy an exercise into the current directory and mark it started",
		Long: `Copy the pristine exercise files from the local cache into your current
working directory and record the exercise as active in ~/.thoth-go/state.json.

Run 'thoth-go fetch <topic>' first if the exercise is not yet cached.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exerciseID := args[0]

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			// Step 1: copy pristine exercise files from the cache into CWD.
			cacheDir, err := repository.DefaultCacheDir()
			if err != nil {
				return err
			}
			m := repository.NewManager(cacheDir, baseURL(), nil)

			ui.Default.Info("Setting up exercise %q in %s…", exerciseID, cwd)
			if err := m.Reset(exerciseID, cwd); err != nil {
				ui.Default.Error("%v", err)
				return err
			}

			// Step 2: record the exercise as active in the progress state.
			ps, statePath, err := loadOrNewState()
			if err != nil {
				return err
			}
			ps.MarkStarted(exerciseID)
			if err := ps.Save(statePath); err != nil {
				return err
			}

			ui.Default.Success("Exercise %q started. Edit the files here and run `thoth-go check`.", exerciseID)
			return nil
		},
	}
}
