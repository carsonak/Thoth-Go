// Package ui provides vibrant, consistent terminal output for thoth-go using
// charmbracelet/lipgloss. All output should flow through this package rather
// than calling fmt.Println directly, so styling can be toggled centrally.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette holds the colour values used throughout the UI.
var Palette = struct {
	Success lipgloss.Color
	Error   lipgloss.Color
	Warning lipgloss.Color
	Info    lipgloss.Color
	Muted   lipgloss.Color
	Accent  lipgloss.Color
}{
	Success: lipgloss.Color("#22c55e"), // green-500
	Error:   lipgloss.Color("#ef4444"), // red-500
	Warning: lipgloss.Color("#f59e0b"), // amber-500
	Info:    lipgloss.Color("#3b82f6"), // blue-500
	Muted:   lipgloss.Color("#6b7280"), // gray-500
	Accent:  lipgloss.Color("#a855f7"), // purple-500
}

// Renderer is the primary UI engine. Create one via New and inject it where needed.
type Renderer struct {
	out io.Writer

	// pre-built styles
	successIcon lipgloss.Style
	errorIcon   lipgloss.Style
	warnIcon    lipgloss.Style
	infoIcon    lipgloss.Style

	successText lipgloss.Style
	errorText   lipgloss.Style
	warnText    lipgloss.Style
	infoText    lipgloss.Style
	mutedText   lipgloss.Style

	banner     lipgloss.Style
	boxSuccess lipgloss.Style
	boxError   lipgloss.Style
	label      lipgloss.Style
	diff       lipgloss.Style
}

// New creates a Renderer that writes to the given writer (pass os.Stdout for
// normal use or a bytes.Buffer in tests).
func New(out io.Writer) *Renderer {
	r := &Renderer{out: out}

	bold := lipgloss.NewStyle().Bold(true)

	r.successIcon = bold.Foreground(Palette.Success)
	r.errorIcon = bold.Foreground(Palette.Error)
	r.warnIcon = bold.Foreground(Palette.Warning)
	r.infoIcon = bold.Foreground(Palette.Info)

	r.successText = lipgloss.NewStyle().Foreground(Palette.Success)
	r.errorText = lipgloss.NewStyle().Foreground(Palette.Error)
	r.warnText = lipgloss.NewStyle().Foreground(Palette.Warning)
	r.infoText = lipgloss.NewStyle().Foreground(Palette.Info)
	r.mutedText = lipgloss.NewStyle().Foreground(Palette.Muted)

	r.banner = lipgloss.NewStyle().
		Bold(true).
		Foreground(Palette.Accent).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Palette.Accent).
		Padding(0, 2)

	r.boxSuccess = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(Palette.Success).
		Padding(0, 1)

	r.boxError = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(Palette.Error).
		Padding(0, 1)

	r.label = lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
		Background(Palette.Accent).
		Foreground(lipgloss.Color("#ffffff"))

	r.diff = lipgloss.NewStyle().Foreground(Palette.Muted)

	return r
}

// Default is a package-level renderer writing to os.Stdout.
// Tests and commands should prefer injecting their own Renderer.
var Default = New(os.Stdout)

// ── Top-level message helpers ────────────────────────────────────────────────

// Success prints a green success message with a ✓ prefix.
func (r *Renderer) Success(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintln(r.out, r.successIcon.Render("✓ ")+r.successText.Render(msg))
}

// Error prints a red error message with a ✗ prefix.
func (r *Renderer) Error(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintln(r.out, r.errorIcon.Render("✗ ")+r.errorText.Render(msg))
}

// Warning prints an amber warning message with a ⚠ prefix.
func (r *Renderer) Warning(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintln(r.out, r.warnIcon.Render("⚠ ")+r.warnText.Render(msg))
}

// Info prints a blue informational message with a ℹ prefix.
func (r *Renderer) Info(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintln(r.out, r.infoIcon.Render("ℹ ")+r.infoText.Render(msg))
}

// Muted prints a subdued message, useful for secondary details.
func (r *Renderer) Muted(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintln(r.out, r.mutedText.Render(msg))
}

// ── Structural elements ──────────────────────────────────────────────────────

// Banner prints a rounded-border accent box, used as the app header.
func (r *Renderer) Banner(text string) {
	fmt.Fprintln(r.out, r.banner.Render(text))
}

// SectionHeader prints a bold section label followed by a separator line.
func (r *Renderer) SectionHeader(title string) {
	styled := lipgloss.NewStyle().Bold(true).Foreground(Palette.Accent).Render(title)
	sep := r.mutedText.Render(strings.Repeat("─", 48))
	fmt.Fprintln(r.out, styled)
	fmt.Fprintln(r.out, sep)
}

// Label prints a small highlighted badge followed by the given message.
func (r *Renderer) Label(badge, message string) {
	fmt.Fprintf(r.out, "%s %s\n", r.label.Render(badge), message)
}

// ── Test result helpers ──────────────────────────────────────────────────────

// TestPass prints a single passing test case result.
func (r *Renderer) TestPass(name string) {
	fmt.Fprintln(r.out, r.successIcon.Render("  ✓ ")+r.mutedText.Render(name))
}

// TestFail prints a failing test case with expected vs actual details.
func (r *Renderer) TestFail(name, expected, got string) {
	fmt.Fprintln(r.out, r.errorIcon.Render("  ✗ ")+r.errorText.Render(name))
	exp := r.boxSuccess.Render("expected:\n" + indent(expected, 4))
	act := r.boxError.Render("got:\n" + indent(got, 4))
	fmt.Fprintln(r.out, exp)
	fmt.Fprintln(r.out, act)
}

// StaticError prints a single static-analysis violation.
func (r *Renderer) StaticError(rule, detail string) {
	badge := r.label.
		Background(Palette.Error).
		Render("STATIC")
	fmt.Fprintf(r.out, "%s %s — %s\n",
		badge,
		r.errorText.Render(rule),
		r.mutedText.Render(detail),
	)
}

// Summary prints the final pass/fail summary line.
func (r *Renderer) Summary(passed, total int) {
	ratio := fmt.Sprintf("%d/%d", passed, total)
	if passed == total {
		fmt.Fprintln(r.out, r.successIcon.Render("✓ All tests passed ")+
			r.mutedText.Render(ratio))
	} else {
		fmt.Fprintln(r.out, r.errorIcon.Render("✗ Some tests failed ")+
			r.mutedText.Render(ratio))
	}
}

// ── Utilities ────────────────────────────────────────────────────────────────

// indent prepends each line of s with n spaces.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}
