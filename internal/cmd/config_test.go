package cmd

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
)

func TestConfigCheckDisplaysEnvironmentValuesAndSources(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("MCP_CLICKHOUSE_ADDRESS", "environment.example:9443")
	t.Setenv("MCP_CLICKHOUSE_DATABASE", "environment_database")
	t.Setenv("MCP_CLICKHOUSE_SECURE", "false")
	t.Setenv("MCP_CLICKHOUSE_USERNAME", "environment_user")
	t.Setenv("MCP_CLICKHOUSE_PASSWORD", "environment password")
	t.Setenv("MCP_PORT", "8088")
	t.Setenv("MCP_LISTEN", "0.0.0.0")
	t.Setenv("MCP_SSE_BASE_URL", "https://events.example/sse")
	t.Setenv("MCP_LOG_LEVEL", "debug")
	t.Setenv("MCP_ENABLED_TOOLS", "environment_tool")
	t.Setenv("MCP_DISABLED_TOOLS", "disabled_environment_tool")
	t.Setenv("MCP_ENABLED_DOMAINS", "environment_domain")
	t.Setenv("MCP_DISABLED_DOMAINS", "disabled_environment_domain")

	out, errOut, err := runConfigCheck(t, "config", "check")
	if err != nil {
		t.Fatalf("config check: %v", err)
	}

	assertConfigCheckFields(t, out, []configCheckField{
		{"clickhouse.address", "environment.example:9443", "environment"},
		{"clickhouse.database", "environment_database", "environment"},
		{"clickhouse.secure", "false", "environment"},
		{"clickhouse.username", "environment_user", "environment"},
		{"clickhouse.password_status", "set", "environment"},
		{"port", "8088", "environment"},
		{"listen", "0.0.0.0", "environment"},
		{"sse_base_url", "https://events.example/sse", "environment"},
		{"log_level", "debug", "environment"},
		{"enabled_tools", "environment_tool", "environment"},
		{"disabled_tools", "disabled_environment_tool", "environment"},
		{"enabled_domains", "environment_domain", "environment"},
		{"disabled_domains", "disabled_environment_domain", "environment"},
	})
	assertNoCredentialForms(t, err, out, errOut, "environment_user", "environment password")
}

func TestConfigCheckDisplaysYAMLValuesAndConfigFlagSource(t *testing.T) {
	clearConfigEnvironment(t)
	configPath := writeConfigCheckYAML(t, `
clickhouse:
  address: yaml.example:8123
  database: yaml_database
  secure: true
  username: yaml_user
  password: yaml password
port: 8089
listen: 127.0.0.2
sse_base_url: https://yaml-events.example/sse
log_level: warn
enabled_tools: [yaml_tool]
disabled_tools: [yaml_disabled_tool]
enabled_domains: [yaml_domain]
disabled_domains: [yaml_disabled_domain]
`)

	out, errOut, err := runConfigCheck(t, "--config", configPath, "config", "check")
	if err != nil {
		t.Fatalf("config check: %v", err)
	}

	assertConfigCheckFields(t, out, []configCheckField{
		{"clickhouse.address", "yaml.example:8123", "YAML"},
		{"clickhouse.database", "yaml_database", "YAML"},
		{"clickhouse.secure", "true", "YAML"},
		{"clickhouse.username", "yaml_user", "YAML"},
		{"clickhouse.password_status", "set", "YAML"},
		{"port", "8089", "YAML"},
		{"listen", "127.0.0.2", "YAML"},
		{"sse_base_url", "https://yaml-events.example/sse", "YAML"},
		{"log_level", "warn", "YAML"},
		{"enabled_tools", "yaml_tool", "YAML"},
		{"disabled_tools", "yaml_disabled_tool", "YAML"},
		{"enabled_domains", "yaml_domain", "YAML"},
		{"disabled_domains", "yaml_disabled_domain", "YAML"},
	})
	assertLineContains(t, out, configPath, "source", "flag")
	assertNoCredentialForms(t, err, out, errOut, "yaml_user", "yaml password")
}

func TestConfigCheckDisplaysDefaultsAndDatabaseFallbackSources(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		clearConfigEnvironment(t)
		setConfigCheckAddress(t, "defaults.example:8123")

		out, _, err := runConfigCheck(t, "config", "check")
		if err != nil {
			t.Fatalf("config check: %v", err)
		}
		assertConfigCheckFields(t, out, []configCheckField{
			{"clickhouse.address", "defaults.example:8123", "environment"},
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
		})
		assertLineContains(t, out, "config", "source", "default")
	})

	t.Run("empty YAML database", func(t *testing.T) {
		clearConfigEnvironment(t)
		configPath := writeConfigCheckYAML(t, "clickhouse:\n  address: yaml-empty.example:8123\n  database: \"\"\n")

		out, _, err := runConfigCheck(t, "--config", configPath, "config", "check")
		if err != nil {
			t.Fatalf("config check: %v", err)
		}
		assertConfigCheckField(t, out, configCheckField{"clickhouse.database", "default", "default"})
	})

	t.Run("blank environment database", func(t *testing.T) {
		clearConfigEnvironment(t)
		setConfigCheckAddress(t, "environment-empty.example:8123")
		t.Setenv("MCP_CLICKHOUSE_DATABASE", " \t ")

		out, _, err := runConfigCheck(t, "config", "check")
		if err != nil {
			t.Fatalf("config check: %v", err)
		}
		assertConfigCheckField(t, out, configCheckField{"clickhouse.database", "default", "default"})
	})
}

func TestConfigCheckReportsRootFilterFlagsAsSources(t *testing.T) {
	clearConfigEnvironment(t)
	setConfigCheckAddress(t, "flags.example:8123")

	out, _, err := runConfigCheck(t,
		"--enabled-tools", "flag_tool",
		"--disabled-tools", "flag_disabled_tool",
		"--enable-domains", "flag_domain",
		"--disable-domains", "flag_disabled_domain",
		"config", "check",
	)
	if err != nil {
		t.Fatalf("config check: %v", err)
	}
	assertConfigCheckFields(t, out, []configCheckField{
		{"enabled_tools", "flag_tool", "flag"},
		{"disabled_tools", "flag_disabled_tool", "flag"},
		{"enabled_domains", "flag_domain", "flag"},
		{"disabled_domains", "flag_disabled_domain", "flag"},
	})
}

func TestConfigCheckRejectsMissingOrCredentialBearingAddressWithoutLeaking(t *testing.T) {
	t.Run("missing address", func(t *testing.T) {
		clearConfigEnvironment(t)

		_, _, err := runConfigCheck(t, "config", "check")
		if err == nil || !strings.Contains(err.Error(), "clickhouse.address is required") {
			t.Fatalf("config check error = %v, want required address error", err)
		}
	})

	t.Run("credential-bearing address", func(t *testing.T) {
		clearConfigEnvironment(t)
		const username = "address-user"
		const password = "address password"
		setConfigCheckAddress(t, username+":"+password+"@localhost:8123")

		out, errOut, err := runConfigCheck(t, "config", "check")
		if err == nil || !strings.Contains(err.Error(), "clickhouse.address") {
			t.Fatal("config check did not retain an invalid address diagnostic")
		}
		assertNoCredentialForms(t, err, out, errOut, username, password)
	})
}

func TestCommandsRejectCredentialBearingAddressWithoutLeaking(t *testing.T) {
	const (
		username = "address-user"
		password = "address /?@: password"
	)
	forms := []string{
		password,
		url.QueryEscape(password),
		url.PathEscape(password),
		url.UserPassword(username, password).String(),
	}
	for _, form := range forms {
		t.Run(form, func(t *testing.T) {
			clearConfigEnvironment(t)
			setConfigCheckAddress(t, username+":"+form+"@localhost:8123")
			for _, args := range [][]string{{"mcp"}, {"tools", "list"}} {
				out, errOut, err := runCommand(t, args...)
				if err == nil || !strings.Contains(err.Error(), "clickhouse.address") {
					t.Fatalf("%v error = %v, want invalid address error", args, err)
				}
				assertNoCredentialForms(t, err, out, errOut, username, password)
			}
		})
	}
}

func TestSafeSSEBaseURLRedactsCredentialsWithoutMutatingConfig(t *testing.T) {
	const username = "sse-user"
	const password = "SSE password"
	raw := credentialBearingURL(username, password)
	cfg := config.StaticConfig{SSEBaseURL: raw}

	if got, want := safeSSEBaseURL(cfg.SSEBaseURL), "https://sse-user:xxxxx@events.example/sse"; got != want {
		t.Fatalf("safeSSEBaseURL() = %q, want %q", got, want)
	}
	if cfg.SSEBaseURL != raw {
		t.Fatal("safeSSEBaseURL mutated StaticConfig.SSEBaseURL")
	}
	if got := safeSSEBaseURL(":// malformed"); got != "configured" {
		t.Fatalf("safeSSEBaseURL(malformed) = %q, want configured", got)
	}
}

func TestConfigCheckRedactsSSEBaseURLFromEnvironmentAndYAML(t *testing.T) {
	const username = "sse-user"
	const password = "SSE password"
	raw := credentialBearingURL(username, password)
	const displayed = "https://sse-user:xxxxx@events.example/sse"

	tests := []struct {
		name   string
		source string
		setup  func(*testing.T) []string
	}{
		{
			name:   "environment",
			source: "environment",
			setup: func(t *testing.T) []string {
				setConfigCheckAddress(t, "sse-environment.example:8123")
				t.Setenv("MCP_SSE_BASE_URL", raw)
				return []string{"config", "check"}
			},
		},
		{
			name:   "YAML",
			source: "YAML",
			setup: func(t *testing.T) []string {
				configPath := writeConfigCheckYAML(t, fmt.Sprintf("clickhouse:\n  address: sse-yaml.example:8123\nsse_base_url: %q\n", raw))
				return []string{"--config", configPath, "config", "check"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnvironment(t)
			args := tt.setup(t)

			out, errOut, err := runConfigCheck(t, args...)
			if err != nil {
				t.Fatalf("config check: %v", err)
			}
			assertConfigCheckField(t, out, configCheckField{"sse_base_url", displayed, tt.source})
			assertNoCredentialForms(t, err, out, errOut, username, password)
		})
	}
}

func TestConfigCheckRedactsPasswordWhenItMatchesAnotherDisplayValue(t *testing.T) {
	clearConfigEnvironment(t)
	setConfigCheckAddress(t, "matching-value.example:8123")
	const sharedValue = "shared password"
	t.Setenv("MCP_CLICKHOUSE_USERNAME", sharedValue)
	t.Setenv("MCP_CLICKHOUSE_PASSWORD", sharedValue)

	out, errOut, err := runConfigCheck(t, "config", "check")
	if err != nil {
		t.Fatalf("config check: %v", err)
	}
	assertConfigCheckField(t, out, configCheckField{"clickhouse.username", "[redacted]", "environment"})
	assertNoCredentialForms(t, err, out, errOut, sharedValue, sharedValue)
}

func TestConfigCheckKeepsPasswordStatusAsSet(t *testing.T) {
	clearConfigEnvironment(t)
	setConfigCheckAddress(t, "password-status.example:8123")
	t.Setenv("MCP_CLICKHOUSE_PASSWORD", "set")

	out, _, err := runConfigCheck(t, "config", "check")
	if err != nil {
		t.Fatalf("config check: %v", err)
	}
	assertConfigCheckField(t, out, configCheckField{"clickhouse.password_status", "set", "environment"})
}

func TestConfigCheckDoesNotOpenProbeWithoutConnect(t *testing.T) {
	clearConfigEnvironment(t)
	setConfigCheckAddress(t, "no-probe.example:8123")
	restoreOpenClickHouseProbe := openClickHouseProbe
	t.Cleanup(func() { openClickHouseProbe = restoreOpenClickHouseProbe })
	opened := false
	openClickHouseProbe = func(*ch.Options) (clickHouseProbe, error) {
		opened = true
		return nil, errors.New("probe must not open without --connect")
	}

	_, _, err := runConfigCheck(t, "config", "check")
	if err != nil {
		t.Fatalf("config check: %v", err)
	}
	if opened {
		t.Fatal("config check opened a ClickHouse probe without --connect")
	}
}

func TestConfigCheckConnectUsesHTTPOptionsAndClosesProbe(t *testing.T) {
	clearConfigEnvironment(t)
	configPath := writeConfigCheckYAML(t, `
clickhouse:
  address: secure.example:9443
  database: analytics
  secure: true
  username: probe-user
  password: probe password
`)
	restoreOpenClickHouseProbe := openClickHouseProbe
	t.Cleanup(func() { openClickHouseProbe = restoreOpenClickHouseProbe })
	calls := []string{}
	probe := &recordingClickHouseProbe{calls: &calls}
	var received *ch.Options
	openClickHouseProbe = func(options *ch.Options) (clickHouseProbe, error) {
		calls = append(calls, "open")
		received = options
		return probe, nil
	}

	out, errOut, err := runConfigCheck(t, "--config", configPath, "config", "check", "--connect")
	if err != nil {
		t.Fatalf("config check --connect: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"open", "ping", "close"}) {
		t.Fatalf("probe calls = %v, want Open -> Ping -> Close", calls)
	}
	if received == nil {
		t.Fatal("config check --connect did not pass ClickHouse options to the probe")
	}
	if received.Protocol != ch.HTTP || !reflect.DeepEqual(received.Addr, []string{"secure.example:9443"}) {
		t.Fatalf("probe options transport = %#v, want HTTP secure.example:9443", received)
	}
	if received.Auth != (ch.Auth{Database: "analytics", Username: "probe-user", Password: "probe password"}) {
		t.Fatal("probe options did not preserve parsed ClickHouse authentication")
	}
	if received.TLS == nil || received.TLS.MinVersion != tls.VersionTLS12 {
		t.Fatalf("probe options TLS = %#v, want TLS 1.2 or newer configuration", received.TLS)
	}
	if !strings.Contains(strings.ToLower(out), "connection") || !strings.Contains(strings.ToLower(out), "verified") {
		t.Fatalf("config check --connect success output = %q, want connection verification", out)
	}
	assertNoCredentialForms(t, err, out, errOut, "probe-user", "probe password")
}

func TestConfigCheckConnectRedactsCredentialVariantsFromLifecycleFailures(t *testing.T) {
	const username = "probe-user"
	const password = "probe password"

	tests := []struct {
		name      string
		stage     string
		wantCalls []string
	}{
		{name: "open", stage: "open", wantCalls: []string{"open"}},
		{name: "ping", stage: "ping", wantCalls: []string{"open", "ping", "close"}},
		{name: "close", stage: "close", wantCalls: []string{"open", "ping", "close"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnvironment(t)
			setConfigCheckAddress(t, "failure.example:8123")
			t.Setenv("MCP_CLICKHOUSE_USERNAME", username)
			t.Setenv("MCP_CLICKHOUSE_PASSWORD", password)
			restoreOpenClickHouseProbe := openClickHouseProbe
			t.Cleanup(func() { openClickHouseProbe = restoreOpenClickHouseProbe })

			failure := fmt.Errorf(
				"connection unavailable during %s: raw=%s query=%s path=%s userinfo=%s",
				tt.stage,
				password,
				url.QueryEscape(password),
				url.PathEscape(password),
				url.UserPassword(username, password).String(),
			)
			calls := []string{}
			probe := &recordingClickHouseProbe{calls: &calls}
			switch tt.stage {
			case "ping":
				probe.pingErr = failure
			case "close":
				probe.closeErr = failure
			}
			openClickHouseProbe = func(*ch.Options) (clickHouseProbe, error) {
				calls = append(calls, "open")
				if tt.stage == "open" {
					return nil, failure
				}
				return probe, nil
			}

			out, errOut, err := runConfigCheck(t, "config", "check", "--connect")
			if err == nil || !strings.Contains(err.Error(), "connection unavailable") {
				t.Fatal("config check --connect did not retain the ordinary failure diagnostic")
			}
			if !reflect.DeepEqual(calls, tt.wantCalls) {
				t.Fatalf("probe calls = %v, want %v", calls, tt.wantCalls)
			}
			if strings.Contains(strings.ToLower(out), "connection verified") {
				t.Fatalf("config check reported a successful connection after %s failure: %q", tt.stage, out)
			}
			assertNoCredentialForms(t, err, out, errOut, username, password)
		})
	}
}

type configCheckField struct {
	name   string
	value  string
	source string
}

func assertConfigCheckFields(t *testing.T, output string, fields []configCheckField) {
	t.Helper()
	for _, field := range fields {
		assertConfigCheckField(t, output, field)
	}
}

func assertConfigCheckField(t *testing.T, output string, want configCheckField) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, want.name) {
			continue
		}
		if want.value != "" && !strings.Contains(line, want.value) {
			t.Errorf("%s output line = %q, want value %q", want.name, line, want.value)
		}
		if !strings.Contains(strings.ToLower(line), "source") || !strings.Contains(line, want.source) {
			t.Errorf("%s output line = %q, want source %q", want.name, line, want.source)
		}
		return
	}
	t.Errorf("config check output missing %s", want.name)
}

func assertLineContains(t *testing.T, output string, values ...string) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		matched := true
		for _, value := range values {
			if !strings.Contains(line, value) {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}
	t.Errorf("config check output has no line containing %q", values)
}

func assertNoCredentialForms(t *testing.T, err error, out, errOut, username, password string) {
	t.Helper()
	values := map[string]string{
		"returned error": fmt.Sprint(err),
		"stdout":         out,
		"stderr":         errOut,
	}
	forms := []string{password, url.QueryEscape(password), url.PathEscape(password), url.UserPassword(username, password).String()}
	for channel, value := range values {
		for _, form := range forms {
			if strings.Contains(value, form) {
				t.Errorf("%s leaked a configured credential", channel)
			}
		}
	}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"MCP_CLICKHOUSE_ADDRESS", "MCP_CLICKHOUSE_DATABASE", "MCP_CLICKHOUSE_SECURE", "MCP_CLICKHOUSE_USERNAME", "MCP_CLICKHOUSE_PASSWORD",
		"MCP_PORT", "MCP_LISTEN", "MCP_SSE_BASE_URL", "MCP_LOG_LEVEL",
		"MCP_ENABLED_TOOLS", "MCP_DISABLED_TOOLS", "MCP_ENABLED_DOMAINS", "MCP_DISABLED_DOMAINS",
	} {
		t.Setenv(key, "")
	}
}

func setConfigCheckAddress(t *testing.T, address string) {
	t.Helper()
	t.Setenv("MCP_CLICKHOUSE_ADDRESS", address)
}

func writeConfigCheckYAML(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func credentialBearingURL(username, password string) string {
	return (&url.URL{
		Scheme:   "https",
		Host:     "events.example",
		Path:     "/sse",
		RawQuery: "token=" + url.QueryEscape(password),
		Fragment: "fragment-" + password,
		User:     url.UserPassword(username, password),
	}).String()
}

func runConfigCheck(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return runCommand(t, args...)
}

func runCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	root := NewRootCommand(IOStreams{In: strings.NewReader(""), Out: &out, ErrOut: &errOut})
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

type recordingClickHouseProbe struct {
	calls    *[]string
	pingErr  error
	closeErr error
}

func (p *recordingClickHouseProbe) Ping(context.Context) error {
	*p.calls = append(*p.calls, "ping")
	return p.pingErr
}

func (p *recordingClickHouseProbe) Close() error {
	*p.calls = append(*p.calls, "close")
	return p.closeErr
}
