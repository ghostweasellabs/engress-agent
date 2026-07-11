package tunnel

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/hashicorp/yamux"
)

const smokeResponseBody = "r3-agent-ok"

// ErrAuthRejected indicates the edge rejected credentials; callers must not retry.
var ErrAuthRejected = errors.New("tunnel auth rejected")

// ServeConfig configures an outbound TLS tunnel that authenticates with Edge and proxies HTTP.
type ServeConfig struct {
	EdgeHost      string
	EdgePort      int
	Token         string
	EndpointIDs   []string
	LocalAddrs    map[string]string // endpoint id → local addr; missing → smoke body
	TLSCAPEM      []byte            // pinned CA; nil → system roots
	TLSServerName string            // default EdgeHost
	InsecureSkip  bool              // lab escape hatch
}

// Serve dials Edge over TLS 1.3, performs AUTH, and proxies HTTP from yamux streams.
func Serve(ctx context.Context, cfg ServeConfig) error {
	if cfg.Token == "" {
		return fmt.Errorf("token is required")
	}
	if len(cfg.EndpointIDs) == 0 {
		return fmt.Errorf("endpoint_ids is required")
	}

	serverName := cfg.TLSServerName
	if serverName == "" {
		serverName = cfg.EdgeHost
	}

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		ServerName:         serverName,
		InsecureSkipVerify: cfg.InsecureSkip,
	}
	if len(cfg.TLSCAPEM) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.TLSCAPEM) {
			return fmt.Errorf("invalid tunnel CA pem")
		}
		tlsCfg.RootCAs = pool
	}

	edgeAddr := fmt.Sprintf("%s:%d", cfg.EdgeHost, cfg.EdgePort)
	dialer := &tls.Dialer{Config: tlsCfg}
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

	if err := authenticate(ctx, session, cfg); err != nil {
		return err
	}

	for {
		stream, err := session.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("accept stream: %w", err)
		}
		go handleStream(ctx, stream, cfg.LocalAddrs)
	}
}

func authenticate(ctx context.Context, session *yamux.Session, cfg ServeConfig) error {
	authStream, err := session.Open()
	if err != nil {
		return fmt.Errorf("open auth stream: %w", err)
	}
	defer authStream.Close()

	if _, err := fmt.Fprintf(authStream, "AUTH %s %s\n", cfg.Token, strings.Join(cfg.EndpointIDs, ",")); err != nil {
		return fmt.Errorf("write auth: %w", err)
	}

	br := bufio.NewReader(authStream)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read auth reply: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if after, ok := strings.CutPrefix(line, "OK "); ok {
			_ = after
			continue
		}
		if reason, ok := strings.CutPrefix(line, "ERR "); ok {
			switch reason {
			case "invalid-token", "forbidden":
				return fmt.Errorf("%w: %s", ErrAuthRejected, reason)
			default:
				return fmt.Errorf("auth error: %s", reason)
			}
		}
		return fmt.Errorf("unexpected auth reply: %s", line)
	}
}

func handleStream(ctx context.Context, stream net.Conn, localAddrs map[string]string) {
	defer stream.Close()

	br := bufio.NewReader(stream)
	head, err := br.Peek(7)
	if err == nil && string(head) == "STREAM " {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		proto, endpointID, err := parseStreamLine(line)
		if err != nil {
			return
		}
		localAddr, ok := lookupStreamLocalAddr(endpointID, localAddrs)
		if !ok {
			return
		}
		switch proto {
		case "tcp":
			proxyStreamTCP(ctx, br, stream, localAddr)
		case "udp":
			proxyStreamUDP(ctx, br, stream, localAddr)
		}
		return
	}

	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	defer req.Body.Close()
	req = req.WithContext(ctx)

	localAddr := resolveLocalAddr(req.Host, localAddrs)
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

func parseStreamLine(line string) (proto, endpointID string, err error) {
	line = strings.TrimSpace(line)
	rest, ok := strings.CutPrefix(line, "STREAM ")
	if !ok {
		return "", "", fmt.Errorf("not a stream line")
	}
	parts := strings.Fields(rest)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid STREAM line")
	}
	return parts[0], parts[1], nil
}

func lookupStreamLocalAddr(endpointID string, localAddrs map[string]string) (string, bool) {
	if localAddrs == nil {
		return "", false
	}
	addr, ok := localAddrs[endpointID]
	return addr, ok && addr != ""
}

func resolveLocalAddr(host string, localAddrs map[string]string) string {
	if len(localAddrs) == 0 {
		return ""
	}
	if len(localAddrs) == 1 {
		for _, addr := range localAddrs {
			return addr
		}
	}

	label := hostLabel(host)
	if label != "" {
		if addr, ok := localAddrs[label]; ok {
			return addr
		}
	}
	return ""
}

func hostLabel(host string) string {
	host, _, _ = net.SplitHostPort(host)
	if i := strings.IndexByte(host, '.'); i > 0 {
		return host[:i]
	}
	return host
}

func smokeResponse() string {
	body := smokeResponseBody
	return fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
		len(body),
		body,
	)
}
