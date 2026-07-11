package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoginCommand_CompletesSession(t *testing.T) {
	var pollCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/link/sessions":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "sess-abc"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/link/sessions/sess-abc":
			pollCount++
			if pollCount < 2 {
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "completed", "token": "egt_secret"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	cmd := newLoginCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--api-url", srv.URL, "--link-base", "https://engress.io/link", "--poll-interval", "1ms"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "https://engress.io/link/sess-abc") {
		t.Fatalf("output missing link URL: %q", output)
	}
	if !strings.Contains(output, "egt_secret") {
		t.Fatalf("output missing token: %q", output)
	}
}

func TestLoginCommand_TimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/link/sessions" {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "sess-pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "pending"})
	}))
	defer srv.Close()

	cmd := newLoginCmd()
	cmd.SetArgs([]string{
		"--api-url", srv.URL,
		"--poll-interval", "5ms",
		"--timeout", (50 * time.Millisecond).String(),
	})
	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want timeout error")
	}
}
