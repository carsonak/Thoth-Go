package cli

// cmd_fetch.go implements the `thoth-go fetch <topic>` command.
//
// ARCHITECTURE NOTE — Single Responsibility:
// This file is responsible for exactly one command. It wires the CLI flag
// parsing (cobra) to the repository.Manager.Fetch call and the ui.Renderer
// output. No business logic lives here; Fetch is the single business action.

import (
	"github.com/spf13/cobra"

	"github.com/thoth-go/thoth-go/internal/repository"
	"github.com/thoth-go/thoth-go/internal/ui"
)

func newFetchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch <topic>",
		Short: "Fetch exercises for a topic from the remote repository",
		Long: `Download the zip bundle for a topic from the exercise server and
extract it into the local cache (~/.thoth-go/cache/topics/<topic>/).

If the topic is already cached, the command is a no-op unless --force is given.`,
		Args: cobra.ExactArgs(1),

		// ARCHITECTURE NOTE — RunE vs. Run:
		// All commands use RunE (returns error) rather than Run (no error).
		// This lets cobra propagate errors back to Execute(), which returns
		// them to main.go for consistent exit-code handling.
		RunE: func(cmd *cobra.Command, args []string) error {
			topic := args[0]
			force, _ := cmd.Flags().GetBool("force")

			cacheDir, err := repository.DefaultCacheDir()
			if err != nil {
				return err
			}

			m := repository.NewManager(cacheDir, baseURL(), nil)

			ui.Default.Info("Fetching topic %q…", topic)
			if err := m.Fetch(topic, force); err != nil {
				ui.Default.Error("%v", err)
				return err
			}

			ui.Default.Success("Topic %q is ready.", topic)
			return nil
		},
	}

	cmd.Flags().BoolP("force", "f", false, "Force re-download even if already cached")
	return cmd
}
