package main

import (
	"fmt"
	"os"

	"github.com/degoke/outpost/internal/cli"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
)

func main() {
	root := cli.New()
	if err := root.Execute(); err != nil {
		if code, ok := transport.ExitStatus(err); ok {
			os.Exit(code)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(output.ExitError)
	}
}
