package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/hashicorp/yamux"
)

const smokeResponseBody = "r3-agent-ok"

// ServeConfig configures an outbound tunnel that registers with Edge and proxies HTTP.
type ServeConfig struct {
	EdgeHost   string
	EdgePort   int
	EndpointID string
	LocalAddr  string // empty → built-in smoke response for any request
}

// Serve dials Edge, registers the endpoint, and proxies HTTP from yamux streams to local.
func Serve(ctx context.Context, cfg ServeConfig) error {
	if cfg.EndpointID == "" {
		return fmt.Errorf("endpoint_id is required")
	}

	edgeAddr := fmt.Sprintf("%s:%d", cfg.EdgeHost, cfg.EdgePort)
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", edgeAddr)
	if err != nil {
		return fmt.Errorf("dial edge %s: %w", edgeAddr, err)
	}
	defer conn.Close()

	session, err := yamux.Client(conn, nil)
	if err != nil {
		return fmt.Errorf("yamux client: %w", err)
	}
	defer session.Close()

	go func() {
		<-ctx.Done()
		_ = session.Close()
	}()

	regStream, err := session.Open()
	if err != nil {
		return fmt.Errorf("open register stream: %w", err)
	}
	if _, err := fmt.Fprintf(regStream, "REGISTER %s\n", cfg.EndpointID); err != nil {
		_ = regStream.Close()
		return fmt.Errorf("write register: %w", err)
	}
	if err := regStream.Close(); err != nil {
		return fmt.Errorf("close register stream: %w", err)
	}

	for {
		stream, err := session.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("accept stream: %w", err)
		}
		go handleStream(ctx, stream, cfg.LocalAddr)
	}
}

func handleStream(ctx context.Context, stream net.Conn, localAddr string) {
	defer stream.Close()

	br := bufio.NewReader(stream)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	defer req.Body.Close()
	req = req.WithContext(ctx)

	if localAddr == "" {
		_, _ = io.WriteString(stream, smokeResponse())
		return
	}

	backend, err := net.Dial("tcp", localAddr)
	if err != nil {
		return
	}
	defer backend.Close()

	if err := req.Write(backend); err != nil {
		return
	}

	resp, err := http.ReadResponse(bufio.NewReader(backend), req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_ = resp.Write(stream)
}

func smokeResponse() string {
	body := smokeResponseBody
	return fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
		len(body),
		body,
	)
}
