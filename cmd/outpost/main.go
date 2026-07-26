package main

import (
	"fmt"
	"os"

	"github.com/goke/outpost/internal/cli"
	"github.com/goke/outpost/internal/output"
)

func main() {
	root := cli.New()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(output.ExitError)
	}
}
