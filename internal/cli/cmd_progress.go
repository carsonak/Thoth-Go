package cli

// cmd_progress.go implements the `thoth-go progress` command.
//
// ARCHITECTURE NOTE — Read-Only Command:
// progress is a pure read command — it loads state and renders it. It writes
// nothing, so there is no risk of corrupting state if it fails mid-way. This
// is a useful property to keep in mind when designing commands: separate
// read (query) commands from write (mutation) commands wherever possible.

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	appstate "github.com/thoth-go/thoth-go/internal/state"
	"github.com/thoth-go/thoth-go/internal/ui"
)

func newProgressCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "progress",
		Short: "Show your overall exercise progress",
		Long:  `Display a summary of all started and completed exercises.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			statePath, err := appstate.DefaultStatePath()
			if err != nil {
				return err
			}
			ps, err := appstate.Load(statePath)
			if err != nil {
				return err
			}

			ui.Default.Banner("  Your Progress  ")

			if len(ps.Exercises) == 0 {
				ui.Default.Muted("No exercises started yet.")
				ui.Default.Muted("Run `thoth-go fetch <topic>` then `thoth-go start <exercise-id>`.")
				return nil
			}

			// Sort exercise IDs for stable, readable output.
			ids := make([]string, 0, len(ps.Exercises))
			for id := range ps.Exercises {
				ids = append(ids, id)
			}
			sort.Strings(ids)

			completed := 0
			for _, id := range ids {
				rec := ps.Exercises[id]
				icon, label := statusDisplay(rec.Status)
				line := fmt.Sprintf("%s  %-30s  %s  (%d attempt",
					icon, id, label, rec.Attempts)
				if rec.Attempts != 1 {
					line += "s"
				}
				line += ")"
				if rec.Status == appstate.StatusCompleted {
					ui.Default.Success("%s", line)
					completed++
				} else if rec.Status == appstate.StatusInProgress {
					ui.Default.Info("%s", line)
				} else {
					ui.Default.Muted("%s", line)
				}
			}

			ui.Default.SectionHeader("")
			ui.Default.Summary(completed, len(ids))
			return nil
		},
	}
}

// statusDisplay returns a (icon, label) pair for a given exercise status.
func statusDisplay(s appstate.Status) (icon, label string) {
	switch s {
	case appstate.StatusCompleted:
		return "✓", "completed  "
	case appstate.StatusInProgress:
		return "→", "in progress"
	default:
		return "○", "not started"
	}
}
