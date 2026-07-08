package tunnel

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
)

func TestServe_AUTHHandshakeAndSmokeResponse(t *testing.T) {
	certPEM, serverTLS := testTLSCert(t, "127.0.0.1")

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("tls listen: %v", err)
	}
	defer ln.Close()

	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	ready := make(chan *yamux.Session, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		session, err := yamux.Server(conn, nil)
		if err != nil {
			_ = conn.Close()
			return
		}
		authStream, err := session.Accept()
		if err != nil {
			_ = session.Close()
			return
		}
		line, err := bufio.NewReader(authStream).ReadString('\n')
		if err != nil {
			_ = authStream.Close()
			_ = session.Close()
			return
		}
		if line != "AUTH egt_x http-a\n" {
			t.Errorf("auth line = %q, want AUTH egt_x http-a\\n", line)
		}
		_, _ = io.WriteString(authStream, "OK http-a\n")
		_ = authStream.Close()
		ready <- session
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ServeConfig{
			EdgeHost:      "127.0.0.1",
			EdgePort:      mustAtoi(portStr),
			Token:         "egt_x",
			EndpointIDs:   []string{"http-a"},
			TLSCAPEM:      certPEM,
			TLSServerName: "127.0.0.1",
		})
	}()

	var session *yamux.Session
	select {
	case session = <-ready:
	case err := <-errCh:
		t.Fatalf("serve exited early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for fake edge auth")
	}

	stream, err := session.Open()
	if err != nil {
		t.Fatalf("open proxy stream: %v", err)
	}
	defer stream.Close()

	_, err = io.WriteString(stream, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
	if err != nil {
		t.Fatalf("write request: %v", err)
	}

	resp, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !strings.Contains(string(resp), smokeResponseBody) {
		t.Fatalf("response %q missing smoke body %q", resp, smokeResponseBody)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not exit after cancel")
	}
}

func TestServe_ERRInvalidToken(t *testing.T) {
	certPEM, serverTLS := testTLSCert(t, "127.0.0.1")

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("tls listen: %v", err)
	}
	defer ln.Close()

	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		session, err := yamux.Server(conn, nil)
		if err != nil {
			_ = conn.Close()
			return
		}
		authStream, err := session.Accept()
		if err != nil {
			_ = session.Close()
			return
		}
		_, _ = io.WriteString(authStream, "ERR invalid-token\n")
		_ = authStream.Close()
		_ = session.Close()
	}()

	ctx := context.Background()
	err = Serve(ctx, ServeConfig{
		EdgeHost:      "127.0.0.1",
		EdgePort:      mustAtoi(portStr),
		Token:         "egt_bad",
		EndpointIDs:   []string{"http-a"},
		TLSCAPEM:      certPEM,
		TLSServerName: "127.0.0.1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrAuthRejected) {
		t.Fatalf("error = %v, want ErrAuthRejected", err)
	}
}

func testTLSCert(t *testing.T, serverName string) ([]byte, *tls.Config) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: serverName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{serverName},
		IPAddresses:  []net.IP{net.ParseIP(serverName)},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("key pair: %v", err)
	}

	return certPEM, &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS13,
	}
}

func mustAtoi(s string) int {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		panic(err)
	}
	return n
}
