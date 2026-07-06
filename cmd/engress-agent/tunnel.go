package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/yamux"
	"github.com/spf13/cobra"

	"github.com/ghostweasellabs/engress-agent/internal/tunnel"
)

func newTunnelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tunnel",
		Short: "Manage outbound tunnels to engress-edge",
	}
	cmd.AddCommand(newTunnelConnectCmd())
	cmd.AddCommand(newTunnelServeCmd())
	return cmd
}

func newTunnelServeCmd() *cobra.Command {
	var edgeHost string
	var edgePort int
	var endpointID string
	var localAddr string

	c := &cobra.Command{
		Use:   "serve",
		Short: "Register with Edge and proxy HTTP from yamux streams to a local backend",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()

			return tunnel.Serve(ctx, tunnel.ServeConfig{
				EdgeHost:   edgeHost,
				EdgePort:   edgePort,
				EndpointID: endpointID,
				LocalAddr:  localAddr,
			})
		},
	}
	c.Flags().StringVar(&edgeHost, "edge-host", "127.0.0.1", "Edge public hostname")
	c.Flags().IntVar(&edgePort, "edge-port", 7443, "Edge tunnel port")
	c.Flags().StringVar(&endpointID, "endpoint-id", "", "Endpoint ID to register with Edge")
	c.Flags().StringVar(&localAddr, "local", "", "Local TCP backend (e.g. 127.0.0.1:18080); empty uses built-in smoke handler")
	_ = c.MarkFlagRequired("endpoint-id")
	return c
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
