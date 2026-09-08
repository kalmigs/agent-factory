package ui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
)

var (
	TitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff00ff"))
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#51cf66"))
	WarnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd43b"))
	ErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b"))
	DimStyle     = lipgloss.NewStyle().Faint(true)
	CyanStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ffff"))
	BoldStyle    = lipgloss.NewStyle().Bold(true)
)

func PrintBanner() {
	fmt.Println()
	fmt.Println(TitleStyle.Render("    ╔═══════════════════════════════════════╗"))
	fmt.Println(TitleStyle.Render("    ║         🕹️  AGENT FACTORY  🕹️          ║"))
	fmt.Println(TitleStyle.Render("    ║   2D pixel art agent visualization    ║"))
	fmt.Println(TitleStyle.Render("    ╚═══════════════════════════════════════╝"))
	fmt.Println()
	fmt.Println(DimStyle.Render("  See your team's Claude agents working in a retro arcade"))
	fmt.Println()
}

func Info(msg string) {
	fmt.Printf("  %s %s\n", CyanStyle.Render(">"), msg)
}

func Success(msg string) {
	fmt.Printf("  %s %s\n", SuccessStyle.Render("✓"), msg)
}

func Warn(msg string) {
	fmt.Printf("  %s %s\n", WarnStyle.Render("!"), msg)
}

// stderrWarnStyle colours for stderr specifically. The package-level styles are
// built from lipgloss's default renderer, which decides whether to emit ANSI by
// looking at *stdout* -- so styling stderr with them puts escape codes into
// `2>log` whenever stdout happens to be a terminal, and strips them from what the
// user sees under `>out`.
var stderrWarnStyle = lipgloss.NewRenderer(os.Stderr).NewStyle().Foreground(lipgloss.Color("#ffd43b"))

// WarnErr writes a warning to stderr instead of stdout. A diagnostic that comes
// with a non-zero exit belongs there: a CI step that pipes stdout away and only
// surfaces stderr on failure would otherwise get the exit code and no reason.
func WarnErr(msg string) {
	fmt.Fprintf(os.Stderr, "  %s %s\n", stderrWarnStyle.Render("!"), msg)
}

func Error(msg string) {
	fmt.Printf("  %s %s\n", ErrorStyle.Render("✗"), msg)
}
