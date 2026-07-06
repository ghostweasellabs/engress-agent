// Package cli implements the engress-agent command-line interface. R1 ships
// only version/help scaffolding; the tunnel-connect command lands in R2/R3
// once engress-edge accepts real tunnels.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ghostweasellabs/engress-sdk/observability"
)

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "engress-agent",
		Short: "Engress platform CLI connector",
	}
	root.AddCommand(newVersionCommand())
	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the engress-agent version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "engress-agent %s (commit %s)\n", observability.Version, observability.Commit)
			return err
		},
	}
}
