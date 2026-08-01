package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	ExitOK       = 0
	ExitError    = 1
	ExitUsage    = 2
	ExitSSH      = 3
	ExitAuth     = 4
	ExitConflict = 5
)

var (
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleStep    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

type Printer struct {
	JSON   bool
	Debug  bool
	Stdout io.Writer
	Stderr io.Writer
}

func New(json, debug bool) *Printer {
	return &Printer{
		JSON:   json,
		Debug:  debug,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

func (p *Printer) Step(msg string, args ...any) {
	if p == nil || p.JSON {
		return
	}
	text := fmt.Sprintf(msg, args...)
	if strings.TrimSpace(text) == "" {
		fmt.Fprintln(p.Stderr)
		return
	}
	if colorEnabled(p.Stderr) {
		fmt.Fprintf(p.Stderr, "%s %s\n", styleStep.Render("→"), text)
		return
	}
	fmt.Fprintf(p.Stderr, "→ %s\n", text)
}

func (p *Printer) Info(msg string, args ...any) {
	if p != nil && p.JSON {
		return
	}
	text := fmt.Sprintf(msg, args...)
	if p == nil {
		fmt.Println(text)
		return
	}
	if colorEnabled(p.Stdout) {
		fmt.Fprintf(p.Stdout, "%s\n", styleInfo.Render(text))
		return
	}
	fmt.Fprintf(p.Stdout, "%s\n", text)
}

func (p *Printer) Success(msg string, args ...any) {
	if p != nil && p.JSON {
		return
	}
	text := fmt.Sprintf(msg, args...)
	if p == nil {
		fmt.Println(text)
		return
	}
	if colorEnabled(p.Stdout) {
		fmt.Fprintf(p.Stdout, "%s %s\n", styleSuccess.Render("✓"), text)
		return
	}
	fmt.Fprintf(p.Stdout, "✓ %s\n", text)
}

func (p *Printer) Error(msg string, args ...any) {
	text := fmt.Sprintf(msg, args...)
	w := io.Writer(os.Stderr)
	if p != nil {
		w = p.Stderr
	}
	if p == nil || p.JSON {
		fmt.Fprintf(w, "error: %s\n", text)
		return
	}
	if colorEnabled(p.Stderr) {
		fmt.Fprintf(p.Stderr, "%s %s\n", styleError.Render("✗"), text)
		return
	}
	fmt.Fprintf(p.Stderr, "error: %s\n", text)
}

func (p *Printer) Debugf(format string, args ...any) {
	if p.Debug {
		fmt.Fprintf(p.Stderr, "debug: "+format+"\n", args...)
	}
}

func (p *Printer) PrintJSON(v any) error {
	enc := json.NewEncoder(p.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (p *Printer) Fatal(code int, msg string, args ...any) {
	p.Error(msg, args...)
	os.Exit(code)
}

func colorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if f, ok := w.(*os.File); ok {
		info, err := f.Stat()
		if err == nil && info.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}
