package main

import (
	"fmt"
	"io"
	"net"
	"os"

	"github.com/hashicorp/yamux"
	"github.com/spf13/cobra"
)

func newTunnelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tunnel",
		Short: "Manage outbound tunnels to engress-edge",
	}
	cmd.AddCommand(newTunnelConnectCmd())
	return cmd
}

func newTunnelConnectCmd() *cobra.Command {
	var edgeHost string
	var edgePort int
	var localAddr string

	c := &cobra.Command{
		Use:   "connect",
		Short: "Open a TCP/yamux tunnel to Edge and proxy stdin/stdout for smoke tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			edgeAddr := fmt.Sprintf("%s:%d", edgeHost, edgePort)
			conn, err := net.Dial("tcp", edgeAddr)
			if err != nil {
				return fmt.Errorf("dial edge %s: %w", edgeAddr, err)
			}
			defer conn.Close()

			session, err := yamux.Client(conn, nil)
			if err != nil {
				return fmt.Errorf("yamux client: %w", err)
			}
			defer session.Close()

			stream, err := session.Open()
			if err != nil {
				return fmt.Errorf("open stream: %w", err)
			}
			defer stream.Close()

			if localAddr != "" {
				ln, err := net.Listen("tcp", localAddr)
				if err != nil {
					return fmt.Errorf("listen %s: %w", localAddr, err)
				}
				defer ln.Close()
				localConn, err := ln.Accept()
				if err != nil {
					return err
				}
				defer localConn.Close()
				go func() { _, _ = io.Copy(stream, localConn) }()
				_, err = io.Copy(localConn, stream)
				return err
			}

			go func() { _, _ = io.Copy(stream, os.Stdin) }()
			_, err = io.Copy(os.Stdout, stream)
			return err
		},
	}
	c.Flags().StringVar(&edgeHost, "edge-host", "127.0.0.1", "Edge public hostname")
	c.Flags().IntVar(&edgePort, "edge-port", 7443, "Edge tunnel port")
	c.Flags().StringVar(&localAddr, "local", "", "Optional local TCP addr to proxy (e.g. 127.0.0.1:18080)")
	return c
}
