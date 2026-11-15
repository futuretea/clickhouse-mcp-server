package cmd

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
	"github.com/futuretea/clickhouse-mcp-server/pkg/core/masking"
	"github.com/futuretea/clickhouse-mcp-server/pkg/toolset"
	clickhousetoolset "github.com/futuretea/clickhouse-mcp-server/pkg/toolset/clickhouse"
)

func TestConfigCheckInvalidMaskingDoesNotEchoDocument(t *testing.T) {
	clearConfigEnvironment(t)
	setConfigCheckAddress(t, "masking.example:8123")
	const secret = "synthetic-token-value"
	t.Setenv("MCP_MASKING_JSON", `{"enabled":true,"extra":"`+secret+`"}`)

	out, errOut, err := runConfigCheck(t, "config", "check")
	if err == nil || err.Error() != masking.InvalidConfigMessage {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(out+errOut+err.Error(), secret) {
		t.Fatalf("config check echoed the document: out=%q err=%v stderr=%q", out, err, errOut)
	}
}

func TestConfigCheckEnabledMaskingHidesRulesAndValues(t *testing.T) {
	clearConfigEnvironment(t)
	setConfigCheckAddress(t, "masking.example:8123")
	const secret = "synthetic-token-value"
	t.Setenv("MCP_CLICKHOUSE_USERNAME", secret)
	t.Setenv("MCP_MASKING_JSON", `{"enabled":true,"values":["`+secret+`"]}`)

	out, errOut, err := runConfigCheck(t, "config", "check")
	if err != nil {
		t.Fatalf("config check: %v", err)
	}
	if strings.Contains(out+errOut, secret) || strings.Contains(out, "value_patterns") || !strings.Contains(out, "masking=enabled source=environment") {
		t.Fatalf("config check output = %q stderr=%q", out, errOut)
	}
	if !strings.Contains(out, "clickhouse.username=[masked]") {
		t.Fatalf("username was not masked: %s", out)
	}
}

func TestConfigCheckConnectActiveMaskingOmitsDriverText(t *testing.T) {
	clearConfigEnvironment(t)
	setConfigCheckAddress(t, "failure.example:8123")
	const secret = "synthetic-token-value"
	t.Setenv("MCP_MASKING_JSON", `{"enabled":true,"values":["`+secret+`"]}`)
	restore := openClickHouseProbe
	t.Cleanup(func() { openClickHouseProbe = restore })
	openClickHouseProbe = func(*ch.Options) (clickHouseProbe, error) {
		return nil, errors.New("dial " + secret)
	}

	out, errOut, err := runConfigCheck(t, "config", "check", "--connect")
	combined := out + errOut
	if err != nil {
		combined += err.Error()
	}
	if err == nil || err.Error() != connectFailedMessage || strings.Contains(combined, secret) {
		t.Fatalf("error = %v output=%q stderr=%q", err, out, errOut)
	}
}

func TestToolsCallPrintsMaskedStdout(t *testing.T) {
	clearConfigEnvironment(t)
	setConfigCheckAddress(t, "127.0.0.1:8123")
	const secret = "synthetic-token-value"
	policy, err := masking.ParseJSON(`{"enabled":true,"values":["` + secret + `"]}`)
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	client := &recordingQueryClient{columns: []string{"label", "note"}, rows: [][]any{{secret, "ordinary"}}}
	bound := clickhousetoolset.NewToolsetWithClient("127.0.0.1:8123", "default", "", client, policy)
	previous := runtimeToolsetsOverride
	runtimeToolsetsOverride = func(context.Context, *config.StaticConfig) ([]toolset.Toolset, error) {
		return []toolset.Toolset{bound}, nil
	}
	t.Cleanup(func() { runtimeToolsetsOverride = previous })

	out, errOut, err := runCommand(t, "tools", "call", "clickhouse_query", "--params", `{"query":"SELECT label"}`)
	if err != nil {
		t.Fatalf("tools call: %v stderr=%q", err, errOut)
	}
	if strings.Contains(out+errOut, secret) || !strings.Contains(out, "[masked]") || !strings.Contains(out, "ordinary") {
		t.Fatalf("stdout=%q stderr=%q", out, errOut)
	}
	if client.query != "SELECT label" {
		t.Fatalf("recorded query = %q", client.query)
	}
}

type recordingQueryClient struct {
	columns []string
	rows    [][]any
	query   string
}

func (c *recordingQueryClient) Query(_ context.Context, query string, _ map[string]string) (clickhousetoolset.RowCursor, error) {
	c.query = query
	return &recordingRows{columns: c.columns, values: c.rows, position: -1}, nil
}

type recordingRows struct {
	columns  []string
	values   [][]any
	position int
}

func (r *recordingRows) Columns() []string       { return append([]string(nil), r.columns...) }
func (*recordingRows) ScanTypes() []reflect.Type { return nil }
func (r *recordingRows) Next() bool {
	r.position++
	return r.position < len(r.values)
}
func (r *recordingRows) Scan(destinations ...any) error {
	if r.position < 0 || r.position >= len(r.values) || len(destinations) != len(r.values[r.position]) {
		return errors.New("invalid scan")
	}
	for i, value := range r.values[r.position] {
		destination, ok := destinations[i].(*any)
		if !ok {
			return errors.New("scan destination is not *any")
		}
		*destination = value
	}
	return nil
}
func (*recordingRows) Err() error   { return nil }
func (*recordingRows) Close() error { return nil }
