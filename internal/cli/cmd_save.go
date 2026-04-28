package cli

// cmd_save.go implements the `thoth-go save` command.

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	appstate "github.com/thoth-go/thoth-go/internal/state"
	"github.com/thoth-go/thoth-go/internal/ui"
)

func newSaveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "save",
		Short: "Save a snapshot of your current working directory",
		Long: `Copy all files in the current directory into
~/.thoth-go/snapshots/<exercise-id>/. A previous snapshot for the same
exercise is replaced. Use 'thoth-go load' to restore.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			ps, _, err := loadOrNewState()
			if err != nil {
				return err
			}

			exerciseID := activeExerciseID(ps, cwd)
			if exerciseID == "" {
				return fmt.Errorf("no active exercise; run 'thoth-go start <exercise-id>' first")
			}

			snapshotDir, err := appstate.DefaultSnapshotDir()
			if err != nil {
				return err
			}

			if err := appstate.SaveSnapshot(exerciseID, cwd, snapshotDir); err != nil {
				ui.Default.Error("%v", err)
				return err
			}

			ui.Default.Success("Snapshot saved for %q.", exerciseID)
			return nil
		},
	}
}
