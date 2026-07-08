package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	var token string
	var endpointIDs []string
	var localAddr string
	var tlsCA string
	var tlsServerName string
	var apiBase string
	var insecureSkip bool

	c := &cobra.Command{
		Use:   "serve",
		Short: "Authenticate with Edge and proxy HTTP from yamux streams to a local backend",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()

			if insecureSkip {
				_, _ = fmt.Fprintln(os.Stderr, "warning: --insecure-skip-verify disables TLS certificate verification")
			}

			localAddrs := map[string]string{}
			if localAddr != "" {
				for _, id := range endpointIDs {
					localAddrs[id] = localAddr
				}
			}

			var caPEM []byte
			if !insecureSkip {
				var err error
				caPEM, err = tunnel.ResolveTunnelCA(tlsCA, apiBase)
				if err != nil {
					return err
				}
			}

			cfg := tunnel.ServeConfig{
				EdgeHost:      edgeHost,
				EdgePort:      edgePort,
				Token:         token,
				EndpointIDs:   endpointIDs,
				LocalAddrs:    localAddrs,
				TLSCAPEM:      caPEM,
				TLSServerName: tlsServerName,
				InsecureSkip:  insecureSkip,
			}

			return serveWithBackoff(ctx, cfg)
		},
	}
	c.Flags().StringVar(&edgeHost, "edge-host", "127.0.0.1", "Edge public hostname")
	c.Flags().IntVar(&edgePort, "edge-port", 7443, "Edge tunnel port")
	c.Flags().StringVar(&token, "token", "", "Tunnel auth token")
	c.Flags().StringSliceVar(&endpointIDs, "endpoint-id", nil, "Endpoint ID to register (repeatable)")
	c.Flags().StringVar(&localAddr, "local", "", "Local TCP backend (e.g. 127.0.0.1:18080); empty uses built-in smoke handler")
	c.Flags().StringVar(&tlsCA, "tls-ca", "", "Path to tunnel CA PEM (default: fetch from API)")
	c.Flags().StringVar(&tlsServerName, "tls-server-name", "", "TLS SNI server name (default: edge-host)")
	c.Flags().StringVar(&apiBase, "api-base", "https://engress.io", "API base URL for tunnel CA fetch")
	c.Flags().BoolVar(&insecureSkip, "insecure-skip-verify", false, "Skip TLS certificate verification (lab only)")
	_ = c.Flags().MarkHidden("insecure-skip-verify")
	_ = c.MarkFlagRequired("token")
	_ = c.MarkFlagRequired("endpoint-id")
	return c
}

func serveWithBackoff(ctx context.Context, cfg tunnel.ServeConfig) error {
	delay := time.Second
	const maxDelay = 60 * time.Second

	for {
		start := time.Now()
		err := tunnel.Serve(ctx, cfg)
		if errors.Is(err, tunnel.ErrAuthRejected) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if time.Since(start) >= maxDelay {
			delay = time.Second
		}

		wait := jitteredDelay(delay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}

		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

func jitteredDelay(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	// ±20% jitter
	jitter := 0.8 + rand.Float64()*0.4
	return time.Duration(float64(base) * jitter)
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
