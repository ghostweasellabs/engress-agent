package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultAPIURL   = "https://api.engress.io"
	defaultLinkBase = "https://engress.io/link"
	envAPIURL       = "ENGRESS_API_URL"
	envLinkBase     = "ENGRESS_LINK_BASE"
)

type linkSessionCreateResponse struct {
	ID string `json:"id"`
}

type linkSessionPollResponse struct {
	Status string `json:"status"`
	Token  string `json:"token"`
}

func newLoginCmd() *cobra.Command {
	var apiURL string
	var linkBase string
	var pollInterval time.Duration
	var timeout time.Duration

	c := &cobra.Command{
		Use:   "login",
		Short: "Start a browser link session and print a connect token",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runLogin(ctx, cmd.OutOrStdout(), apiURL, linkBase, pollInterval, timeout)
		},
	}

	c.Flags().StringVar(&apiURL, "api-url", envOrDefault(envAPIURL, defaultAPIURL), "Customer API base URL")
	c.Flags().StringVar(&linkBase, "link-base", envOrDefault(envLinkBase, defaultLinkBase), "Browser link base URL")
	c.Flags().DurationVar(&pollInterval, "poll-interval", 2*time.Second, "Poll interval while waiting for browser completion")
	c.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Maximum time to wait for session completion")
	return c
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runLogin(ctx context.Context, out io.Writer, apiURL, linkBase string, pollInterval, timeout time.Duration) error {
	client := &http.Client{Timeout: 15 * time.Second}

	sessionID, err := createLinkSession(ctx, client, apiURL)
	if err != nil {
		return err
	}

	linkURL := fmt.Sprintf("%s/%s", trimTrailingSlash(linkBase), sessionID)
	if _, err := fmt.Fprintf(out, "%s\n", linkURL); err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for link session %s", sessionID)
		}

		status, token, done, err := pollLinkSession(ctx, client, apiURL, sessionID)
		if err != nil {
			return err
		}
		if done {
			if _, err := fmt.Fprintln(out, token); err != nil {
				return err
			}
			return nil
		}
		if status != "pending" {
			return fmt.Errorf("unexpected link session status %q", status)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func createLinkSession(ctx context.Context, client *http.Client, apiURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/v1/link/sessions", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("create link session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create link session: unexpected status %s", resp.Status)
	}

	var body linkSessionCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode create link session: %w", err)
	}
	if body.ID == "" {
		return "", fmt.Errorf("create link session: empty id")
	}
	return body.ID, nil
}

func pollLinkSession(ctx context.Context, client *http.Client, apiURL, sessionID string) (status, token string, done bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/v1/link/sessions/"+sessionID, nil)
	if err != nil {
		return "", "", false, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", false, fmt.Errorf("poll link session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", "", false, fmt.Errorf("link session %s not found", sessionID)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", false, fmt.Errorf("poll link session: unexpected status %s", resp.Status)
	}

	var body linkSessionPollResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", false, fmt.Errorf("decode poll link session: %w", err)
	}
	if body.Status == "completed" {
		if body.Token == "" {
			return "", "", false, fmt.Errorf("completed link session missing token")
		}
		return body.Status, body.Token, true, nil
	}
	return body.Status, "", false, nil
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
