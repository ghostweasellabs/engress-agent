package tunnel

import (
	"context"
	"io"
	"net"
)

func proxyStreamUDP(ctx context.Context, r io.Reader, w io.Writer, localAddr string) {
	host, port, err := net.SplitHostPort(localAddr)
	if err != nil {
		host = "127.0.0.1"
		port = localAddr
	}
	raddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, port))
	if err != nil {
		return
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return
	}
	defer conn.Close()

	errCh := make(chan error, 2)
	go func() {
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
			}
			payload, err := ReadFrame(r)
			if err != nil {
				errCh <- err
				return
			}
			if _, err := conn.Write(payload); err != nil {
				errCh <- err
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, maxFrameSize)
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
			}
			n, err := conn.Read(buf)
			if err != nil {
				errCh <- err
				return
			}
			if err := WriteFrame(w, buf[:n]); err != nil {
				errCh <- err
				return
			}
		}
	}()

	<-errCh
}
