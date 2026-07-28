package main

import (
	"fmt"
	"os"

	"github.com/degoke/outpost/internal/cli"
	"github.com/degoke/outpost/internal/output"
)

func main() {
	root := cli.New()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(output.ExitError)
	}
}
