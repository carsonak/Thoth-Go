// Package cli wires together all cobra commands for the thoth-go CLI.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags.
var version = "dev"

// rootCmd is the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "thoth-go",
	Short: "Thoth-Go — a local CLI autograder for learning Go",
	Long: `Thoth-Go helps you master Go by providing curated exercises
with instant, config-driven feedback directly in your terminal.`,
	Version: version,
}

// Execute runs the root command and returns any error.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(
		newStartCmd(),
		newCheckCmd(),
		newSaveCmd(),
		newLoadCmd(),
		newFetchCmd(),
		newResetCmd(),
		newProgressCmd(),
	)
}

// placeholder commands — each will be fully implemented in subsequent phases.

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <exercise-id>",
		Short: "Open an exercise in your working directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "start: %s (not yet implemented)\n", args[0])
			return nil
		},
	}
}

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Run static analysis and tests against the current exercise",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "check: (not yet implemented)")
			return nil
		},
	}
}

func newSaveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "save",
		Short: "Save a snapshot of your current progress",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "save: (not yet implemented)")
			return nil
		},
	}
}

func newLoadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "load",
		Short: "Restore a previously saved snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "load: (not yet implemented)")
			return nil
		},
	}
}

func newFetchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch <topic>",
		Short: "Fetch exercises for a topic from the remote repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "fetch: %s (not yet implemented)\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolP("force", "f", false, "Force re-download even if already cached")
	return cmd
}

func newResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset <exercise-id>",
		Short: "Reset an exercise to its pristine cached state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "reset: %s (not yet implemented)\n", args[0])
			return nil
		},
	}
}

func newProgressCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "progress",
		Short: "Show your overall exercise progress",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "progress: (not yet implemented)")
			return nil
		},
	}
}
