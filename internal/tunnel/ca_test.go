package tunnel

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTunnelCA_fromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ca.pem")
	want := []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	got, err := ResolveTunnelCA(path, "https://example.test")
	if err != nil {
		t.Fatalf("ResolveTunnelCA: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveTunnelCA_fetchAndCache(t *testing.T) {
	want := []byte("-----BEGIN CERTIFICATE-----\nfetched\n-----END CERTIFICATE-----\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(want)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	t.Setenv("HOME", cacheDir)
	t.Setenv("ENGRESS_TUNNEL_CA", "")

	got, err := ResolveTunnelCA("", srv.URL)
	if err != nil {
		t.Fatalf("ResolveTunnelCA: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %q, want %q", got, want)
	}

	cachePath := filepath.Join(cacheDir, ".config", "engress", "tunnel-ca.pem")
	cached, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if string(cached) != string(want) {
		t.Fatalf("cached %q, want %q", cached, want)
	}
}
