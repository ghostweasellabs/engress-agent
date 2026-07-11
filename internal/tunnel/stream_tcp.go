package tunnel

import (
	"context"
	"io"
	"net"
)

func proxyStreamTCP(ctx context.Context, stream net.Conn, localAddr string) {
	d := net.Dialer{}
	backend, err := d.DialContext(ctx, "tcp", localAddr)
	if err != nil {
		return
	}
	defer backend.Close()

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(backend, stream)
		close(done)
	}()
	_, _ = io.Copy(stream, backend)
	<-done
}
