package tunnel

import "testing"

func TestParseEndpointMapping_PortOnly(t *testing.T) {
	id, addr, err := ParseEndpointMapping("tcp-abc:5432")
	if err != nil {
		t.Fatal(err)
	}
	if id != "tcp-abc" || addr != "127.0.0.1:5432" {
		t.Fatalf("got id=%q addr=%q", id, addr)
	}
}

func TestParseEndpointMapping_HostPort(t *testing.T) {
	id, addr, err := ParseEndpointMapping("udp-xyz:10.0.0.5:9002")
	if err != nil {
		t.Fatal(err)
	}
	if id != "udp-xyz" || addr != "10.0.0.5:9002" {
		t.Fatalf("got id=%q addr=%q", id, addr)
	}
}

func TestParseEndpointMapping_Invalid(t *testing.T) {
	if _, _, err := ParseEndpointMapping("nocolon"); err == nil {
		t.Fatal("expected error")
	}
}
