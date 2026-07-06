package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand_PrintsVersion(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "engress-agent") {
		t.Fatalf("output = %q, want it to contain \"engress-agent\"", out.String())
	}
}
