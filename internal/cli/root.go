// Package cli wires together all cobra commands for the thoth-go CLI.
//
// # Command Registration Pattern
//
// Each subcommand lives in its own file (cmd_check.go, cmd_fetch.go, …).
// root.go owns only the root cobra.Command and the init() that registers
// subcommands. This separation follows the Single Responsibility Principle:
// changing a command's flags or behaviour requires editing exactly one file,
// not hunting through a monolithic root.go.
package cli

import "github.com/spf13/cobra"

// version is set at build time via -ldflags "-X github.com/…/cli.version=v1.2.3".
var version = "dev"

// rootCmd is the base command invoked when no subcommand is given.
var rootCmd = &cobra.Command{
	Use:   "thoth-go",
	Short: "Thoth-Go — a local CLI autograder for learning Go",
	Long: `Thoth-Go helps you master Go by providing curated exercises
with instant, config-driven feedback directly in your terminal.`,
	Version: version,
}

// Execute runs the root command and returns any error.
// main.go calls this; the error is printed and os.Exit(1) is called there.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// ARCHITECTURE NOTE — Registration in init():
	// Cobra commands are registered in init() rather than inside Execute()
	// because init() runs once at program startup before any argument parsing.
	// This keeps Execute() a pure "run" function and makes the command tree
	// visible to cobra's help / completion machinery before any user input
	// is processed.
	rootCmd.AddCommand(
		newFetchCmd(),
		newStartCmd(),
		newCheckCmd(),
		newSaveCmd(),
		newLoadCmd(),
		newResetCmd(),
		newProgressCmd(),
	)
}
