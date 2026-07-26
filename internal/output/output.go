package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const (
	ExitOK       = 0
	ExitError    = 1
	ExitUsage    = 2
	ExitSSH      = 3
	ExitAuth     = 4
	ExitConflict = 5
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

func (p *Printer) Info(msg string, args ...any) {
	fmt.Fprintf(p.Stdout, msg+"\n", args...)
}

func (p *Printer) Success(msg string, args ...any) {
	fmt.Fprintf(p.Stdout, msg+"\n", args...)
}

func (p *Printer) Error(msg string, args ...any) {
	fmt.Fprintf(p.Stderr, "error: "+msg+"\n", args...)
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
