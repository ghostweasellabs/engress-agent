package main

import (
	"os"

	"github.com/ghostweasellabs/engress-agent/internal/cli"
)

func main() {
	root := cli.NewRootCommand()
	root.AddCommand(newTunnelCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
