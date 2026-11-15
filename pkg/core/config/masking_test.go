package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
	"github.com/futuretea/clickhouse-mcp-server/pkg/core/masking"
)

func TestLoadConfigMaskingDefaultsOff(t *testing.T) {
	unsetMaskingEnv(t)
	v := viper.New()
	v.Set("clickhouse.address", "127.0.0.1:8123")
	cfg, err := config.LoadConfig("", v)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Masking.Enabled() {
		t.Fatal("masking enabled by default")
	}
	got, err := cfg.Masking.MaskText("synthetic-token-value", nil)
	if err != nil || got != "synthetic-token-value" {
		t.Fatalf("disabled MaskText = %q, %v", got, err)
	}
}

func TestLoadConfigMaskingEnvReplacesYAML(t *testing.T) {
	unsetMaskingEnv(t)
	const yamlSecret = "yaml-synthetic-token"
	const envSecret = "env-synthetic-token"
	t.Setenv("MCP_MASKING_JSON", `{"enabled":true,"values":["`+envSecret+`"]}`)
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := "clickhouse:\n  address: 127.0.0.1:8123\nmasking:\n  enabled: true\n  values: [\"" + yamlSecret + "\"]\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.LoadConfig(path, viper.New())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Masking.Enabled() {
		t.Fatal("env masking object was not enabled")
	}
	envMasked, err := cfg.Masking.MaskText(envSecret, nil)
	if err != nil || envMasked == envSecret {
		t.Fatalf("env value MaskText = %q, %v", envMasked, err)
	}
	yamlMasked, err := cfg.Masking.MaskText(yamlSecret, nil)
	if err != nil || yamlMasked != yamlSecret {
		t.Fatalf("YAML value remained active: %q, %v", yamlMasked, err)
	}
}

func TestLoadConfigRejectsInvalidMaskingWithoutEcho(t *testing.T) {
	unsetMaskingEnv(t)
	const secret = "synthetic-token-value"
	tests := []string{
		"",
		" ",
		`{"enabled":true}`,
		`{"enabled":true,"values":["` + secret + `"],"extra":"` + secret + `"}`,
		`{"enabled":true,"value_patterns":["a*"]}`,
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("MCP_MASKING_JSON", raw)
			v := viper.New()
			v.Set("clickhouse.address", "127.0.0.1:8123")
			_, err := config.LoadConfig("", v)
			if err == nil || err.Error() != masking.InvalidConfigMessage || strings.Contains(err.Error(), secret) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestLoadConfigInvalidYAMLDoesNotEchoDocument(t *testing.T) {
	unsetMaskingEnv(t)
	const secret = "synthetic-token-value"
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := "clickhouse: [" + secret + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := config.LoadConfig(path, viper.New())
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadConfigDottedMaskingEnvDoesNotEnable(t *testing.T) {
	unsetMaskingEnv(t)
	t.Setenv("MCP_MASKING_ENABLED", "true")
	v := viper.New()
	v.Set("clickhouse.address", "127.0.0.1:8123")
	cfg, err := config.LoadConfig("", v)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Masking.Enabled() {
		t.Fatal("MCP_MASKING_ENABLED enabled masking")
	}
}

func unsetMaskingEnv(t *testing.T) {
	t.Helper()
	previous, existed := os.LookupEnv("MCP_MASKING_JSON")
	if err := os.Unsetenv("MCP_MASKING_JSON"); err != nil {
		t.Fatalf("unset MCP_MASKING_JSON: %v", err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("MCP_MASKING_JSON", previous)
		}
	})
}
