package cli

// cmd_load.go implements the `thoth-go load` command.

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	appstate "github.com/thoth-go/thoth-go/internal/state"
	"github.com/thoth-go/thoth-go/internal/ui"
)

func newLoadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "load",
		Short: "Restore a previously saved snapshot into the current directory",
		Long: `Copy the files from ~/.thoth-go/snapshots/<exercise-id>/ back into the
current working directory, overwriting any conflicting files.`,
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

			if err := appstate.LoadSnapshot(exerciseID, snapshotDir, cwd); err != nil {
				ui.Default.Error("%v", err)
				return err
			}

			ui.Default.Success("Snapshot for %q restored.", exerciseID)
			return nil
		},
	}
}
