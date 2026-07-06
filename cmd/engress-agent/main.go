package main

import (
	"os"

	"github.com/ghostweasellabs/engress-agent/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
