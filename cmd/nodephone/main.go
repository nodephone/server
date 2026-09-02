// Package main provides the entry point executable for the NodePhone Server.
package main

import (
	"fmt"
	"os"

	"github.com/nodephone/server/internal/kernel"
)

func main() {
	if err := kernel.Boot(); err != nil {
		fmt.Fprintf(os.Stderr, "Error booting NodePhone server kernel: %v\n", err)
		os.Exit(1)
	}
}
