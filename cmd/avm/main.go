package main

import (
	"fmt"
	"os"

	"github.com/MikD1/agent-vm/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
