package cmd

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfigCheckPrefersEnvironmentAndRootFlagsOverYAML(t *testing.T) {
	clearConfigEnvironment(t)
	configPath := writeConfigCheckYAML(t, `
clickhouse:
  address: yaml.example:8123
  database: yaml_database
  secure: true
  username: yaml_user
  password: yaml_password
port: 8089
listen: 127.0.0.2
sse_base_url: https://yaml-events.example/sse
log_level: warn
enabled_tools: [yaml_tool]
disabled_tools: [yaml_disabled]
enabled_domains: [yaml_domain]
disabled_domains: [yaml_disabled_domain]
`)
	t.Setenv("MCP_CLICKHOUSE_ADDRESS", "environment.example:8123")
	t.Setenv("MCP_CLICKHOUSE_DATABASE", "environment_database")
	t.Setenv("MCP_CLICKHOUSE_SECURE", "false")
	t.Setenv("MCP_CLICKHOUSE_USERNAME", "environment_user")
	t.Setenv("MCP_CLICKHOUSE_PASSWORD", "environment_password")
	t.Setenv("MCP_PORT", "8088")
	t.Setenv("MCP_LISTEN", "0.0.0.0")
	t.Setenv("MCP_SSE_BASE_URL", "https://environment-events.example/sse")
	t.Setenv("MCP_LOG_LEVEL", "debug")
	t.Setenv("MCP_ENABLED_TOOLS", "environment_tool")
	t.Setenv("MCP_DISABLED_TOOLS", "environment_disabled")
	t.Setenv("MCP_ENABLED_DOMAINS", "environment_domain")
	t.Setenv("MCP_DISABLED_DOMAINS", "environment_disabled_domain")

	out, _, err := runConfigCheck(t,
		"--config", configPath,
		"--enabled-tools", "flag_tool",
		"--disabled-tools", "flag_disabled",
		"--enable-domains", "flag_domain",
		"--disable-domains", "flag_disabled_domain",
		"config", "check",
	)
	if err != nil {
		t.Fatalf("config check: %v", err)
	}
	assertConfigCheckFields(t, out, []configCheckField{
		{"clickhouse.address", "environment.example:8123", "environment"},
		{"clickhouse.database", "environment_database", "environment"},
		{"clickhouse.secure", "false", "environment"},
		{"clickhouse.username", "environment_user", "environment"},
		{"clickhouse.password_status", "set", "environment"},
		{"port", "8088", "environment"},
		{"listen", "0.0.0.0", "environment"},
		{"sse_base_url", "https://environment-events.example/sse", "environment"},
		{"log_level", "debug", "environment"},
		{"enabled_tools", "flag_tool", "flag"},
		{"disabled_tools", "flag_disabled", "flag"},
		{"enabled_domains", "flag_domain", "flag"},
		{"disabled_domains", "flag_disabled_domain", "flag"},
	})
}

func TestConfigCheckEmitsTheConfirmedFieldsInOrder(t *testing.T) {
	clearConfigEnvironment(t)
	setConfigCheckAddress(t, "ordered.example:8123")

	out, _, err := runConfigCheck(t, "config", "check")
	if err != nil {
		t.Fatalf("config check: %v", err)
	}
	want := []configCheckField{
		{"clickhouse.address", "ordered.example:8123", "environment"},
		{"clickhouse.database", "default", "default"},
		{"clickhouse.secure", "false", "default"},
		{"clickhouse.username", "", "default"},
		{"clickhouse.password_status", "unset", "default"},
		{"port", "0", "default"},
		{"listen", "127.0.0.1", "default"},
		{"sse_base_url", "", "default"},
		{"log_level", "info", "default"},
		{"enabled_tools", "", "default"},
		{"disabled_tools", "", "default"},
		{"enabled_domains", "", "default"},
		{"disabled_domains", "", "default"},
		{"allow_unsafe_database_role", "false", "default"},
	}
	var got []configCheckField
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, "config=") {
			continue
		}
		name, remainder, found := strings.Cut(line, "=")
		value, source, sourceFound := strings.Cut(remainder, " source=")
		if !found || !sourceFound {
			t.Fatalf("config check field line = %q, want name=value source=source", line)
		}
		got = append(got, configCheckField{name: name, value: value, source: source})
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config check fields = %#v, want %#v", got, want)
	}
}

func TestConfigCheckRedactsPasswordFormsFromConfigPath(t *testing.T) {
	const username = "path-user"
	const password = "path password"
	forms := []string{
		password,
		url.QueryEscape(password),
		url.PathEscape(password),
		url.UserPassword(username, password).String(),
	}
	for _, form := range forms {
		t.Run(form, func(t *testing.T) {
			clearConfigEnvironment(t)
			path := filepath.Join(t.TempDir(), "contains-"+form+".yaml")
			contents := fmt.Sprintf("clickhouse:\n  address: config-path.example:8123\n  username: %q\n  password: %q\n", username, password)
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			out, errOut, err := runConfigCheck(t, "--config", path, "config", "check")
			if err != nil {
				t.Fatalf("config check: %v", err)
			}
			assertLineContains(t, out, "config=", "source=flag")
			assertNoCredentialForms(t, err, out, errOut, username, password)
		})
	}
}

func TestConfigCheckRedactsPasswordFormsFromConfigLoadError(t *testing.T) {
	const username = "load-error-user"
	const password = "load error password"
	forms := []string{
		password,
		url.QueryEscape(password),
		url.PathEscape(password),
		url.UserPassword(username, password).String(),
	}
	for _, form := range forms {
		t.Run(form, func(t *testing.T) {
			clearConfigEnvironment(t)
			setConfigCheckAddress(t, "load-error.example:8123")
			t.Setenv("MCP_CLICKHOUSE_USERNAME", username)
			t.Setenv("MCP_CLICKHOUSE_PASSWORD", password)
			path := filepath.Join(t.TempDir(), "missing-"+form+"-clickhouse.address is required.yaml")

			out, errOut, err := runConfigCheck(t, "--config", path, "config", "check")
			if err == nil || err.Error() != "load config: configuration is invalid" {
				t.Fatalf("config check error = %v, want generic config-load error", err)
			}
			assertNoCredentialForms(t, err, out, errOut, username, password)
		})
	}
}
