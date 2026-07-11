package tunnel

import (
	"bytes"
	"testing"
)

func TestUDPFrame_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("hello-datagram")
	if err := WriteFrame(&buf, payload); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFrame(&buf)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("%v %q", err, got)
	}
}

func TestUDPFrame_RejectsOversizedPayload(t *testing.T) {
	payload := make([]byte, maxFrameSize+1)
	if err := WriteFrame(&bytes.Buffer{}, payload); err == nil {
		t.Fatal("WriteFrame() error = nil, want error")
	}
}

func TestUDPFrame_RejectsOversizedLength(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, []byte("ok")); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	raw[0] = 0xff
	raw[1] = 0xff
	if _, err := ReadFrame(bytes.NewReader(raw)); err == nil {
		t.Fatal("ReadFrame() error = nil, want error")
	}
}
