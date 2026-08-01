//go:build integration

package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestIntegrationConfigCheckConnect(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	root := NewRootCommand(IOStreams{In: strings.NewReader(""), Out: &out, ErrOut: &errOut})
	root.SetArgs([]string{"config", "check", "--connect"})

	if err := root.Execute(); err != nil {
		t.Fatalf("config check --connect: %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "ClickHouse connection verified.") {
		t.Fatalf("config check output = %q, want connection verification", out.String())
	}
}
