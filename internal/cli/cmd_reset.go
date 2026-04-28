package cli

// cmd_reset.go implements the `thoth-go reset <exercise-id>` command.
// It copies the pristine cached exercise files into the CWD, discarding
// any local modifications — without changing the progress state.

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/thoth-go/thoth-go/internal/repository"
	"github.com/thoth-go/thoth-go/internal/ui"
)

func newResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset <exercise-id>",
		Short: "Reset an exercise to its pristine cached state",
		Long: `Overwrite the current directory's exercise files with the originals from
the local cache. Your progress state is preserved (attempt count, start time,
etc.) — only the source files are reset.

Run 'thoth-go fetch <topic>' first if the exercise is not yet cached.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exerciseID := args[0]

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			cacheDir, err := repository.DefaultCacheDir()
			if err != nil {
				return err
			}
			m := repository.NewManager(cacheDir, baseURL(), nil)

			ui.Default.Warning("Resetting %q — local changes will be overwritten.", exerciseID)
			if err := m.Reset(exerciseID, cwd); err != nil {
				ui.Default.Error("%v", err)
				return err
			}

			ui.Default.Success("Exercise %q has been reset to the pristine state.", exerciseID)
			return nil
		},
	}
}
