package tunnel

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultAPIBase      = "https://engress.io"
	tunnelCAPathRel     = ".config/engress/tunnel-ca.pem"
	tunnelCAFetchSuffix = "/api/v1/tunnel-ca"
)

// ResolveTunnelCA returns the CA PEM: caFile if set, else $ENGRESS_TUNNEL_CA file,
// else fetch https://{apiBase}/api/v1/tunnel-ca and cache at
// ~/.config/engress/tunnel-ca.pem (0644). apiBase defaults to https://engress.io.
func ResolveTunnelCA(caFile, apiBase string) ([]byte, error) {
	if caFile != "" {
		return os.ReadFile(caFile)
	}
	if env := os.Getenv("ENGRESS_TUNNEL_CA"); env != "" {
		return os.ReadFile(env)
	}
	if apiBase == "" {
		apiBase = defaultAPIBase
	}
	url := strings.TrimRight(apiBase, "/") + tunnelCAFetchSuffix

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch tunnel CA: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch tunnel CA: HTTP %d", resp.StatusCode)
	}
	pemBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read tunnel CA: %w", err)
	}

	cachePath, err := tunnelCACachePath()
	if err != nil {
		return pemBytes, nil
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return pemBytes, nil
	}
	_ = os.WriteFile(cachePath, pemBytes, 0o644)

	return pemBytes, nil
}

func tunnelCACachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, tunnelCAPathRel), nil
}
