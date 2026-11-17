package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
	"github.com/futuretea/clickhouse-mcp-server/pkg/core/masking"
	"github.com/futuretea/clickhouse-mcp-server/pkg/toolset"
	clickhousetoolset "github.com/futuretea/clickhouse-mcp-server/pkg/toolset/clickhouse"
)

func TestBlankAllowlistDoesNotExposeTheCatalog(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		env  map[string]string
		args []string
		want []string
	}{
		{
			name: "yaml blank tool names",
			yaml: "clickhouse:\n  address: 127.0.0.1:8123\nenabled_tools: [\" \", \"\"]\n",
			want: nil,
		},
		{
			name: "yaml blank tool string",
			yaml: "clickhouse:\n  address: 127.0.0.1:8123\nenabled_tools: \" \"\n",
			want: nil,
		},
		{
			name: "yaml blank domains",
			yaml: "clickhouse:\n  address: 127.0.0.1:8123\nenabled_domains: [\" \", \"\"]\n",
			want: nil,
		},
		{
			name: "yaml empty list stays open",
			yaml: "clickhouse:\n  address: 127.0.0.1:8123\nenabled_tools: []\n",
			want: allClickHouseTools,
		},
		{
			name: "empty environment preserves yaml",
			yaml: "clickhouse:\n  address: 127.0.0.1:8123\nenabled_tools: [clickhouse_query]\n",
			env:  map[string]string{"MCP_ENABLED_TOOLS": ""},
			want: []string{"clickhouse_query"},
		},
		{
			name: "blank environment does not reopen the catalog",
			yaml: "clickhouse:\n  address: 127.0.0.1:8123\nenabled_tools: [clickhouse_query]\n",
			env:  map[string]string{"MCP_ENABLED_TOOLS": " "},
			want: nil,
		},
		{
			name: "comma environment does not reopen the catalog",
			yaml: "clickhouse:\n  address: 127.0.0.1:8123\nenabled_tools: [clickhouse_query]\n",
			env:  map[string]string{"MCP_ENABLED_TOOLS": ","},
			want: nil,
		},
		{
			name: "mixed yaml keeps the real name",
			yaml: "clickhouse:\n  address: 127.0.0.1:8123\nenabled_tools: [\"clickhouse_query\", \" \"]\n",
			want: []string{"clickhouse_query"},
		},
		{
			name: "mixed environment keeps the real name",
			env: map[string]string{
				"MCP_CLICKHOUSE_ADDRESS": "127.0.0.1:8123",
				"MCP_ENABLED_TOOLS":      "clickhouse_query, ",
			},
			want: []string{"clickhouse_query"},
		},
		{
			name: "explicit empty flag clears yaml",
			yaml: "clickhouse:\n  address: 127.0.0.1:8123\nenabled_tools: [clickhouse_query]\n",
			args: []string{"--enabled-tools="},
			want: allClickHouseTools,
		},
		{
			name: "mixed flag keeps the real name",
			yaml: "clickhouse:\n  address: 127.0.0.1:8123\n",
			args: []string{"--enabled-tools", "clickhouse_query,"},
			want: []string{"clickhouse_query"},
		},
		{
			name: "blank denylist removes nothing",
			yaml: "clickhouse:\n  address: 127.0.0.1:8123\ndisabled_tools: [\" \", \"\"]\ndisabled_domains: [\" \"]\n",
			want: allClickHouseTools,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			clearConfigEnvironment(t)
			args := append([]string{}, testCase.args...)
			if testCase.yaml != "" {
				args = append(args, "--config", writeConfigCheckYAML(t, testCase.yaml))
			}
			for key, value := range testCase.env {
				t.Setenv(key, value)
			}
			args = append(args, "tools", "list", "--json")
			out, errOut, err := runCommand(t, args...)
			if err != nil {
				t.Fatalf("tools list: %v stderr=%q", err, errOut)
			}
			got := listedToolNames(t, out)
			if strings.Join(got, ",") != strings.Join(testCase.want, ",") {
				t.Fatalf("tools = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestRejectedToolArgumentsDoNotOpenTheBackend(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("MCP_CLICKHOUSE_ADDRESS", "127.0.0.1:1")
	opens := 0
	previous := runtimeToolsetsOverride
	runtimeToolsetsOverride = func(context.Context, *config.StaticConfig) ([]toolset.Toolset, error) {
		opens++
		return nil, fmt.Errorf("backend opened")
	}
	t.Cleanup(func() { runtimeToolsetsOverride = previous })

	cases := []struct {
		name    string
		tool    string
		payload string
		want    string
	}{
		{name: "empty query", tool: "clickhouse_query", payload: `{"query":""}`, want: "query is required"},
		{name: "blank query", tool: "clickhouse_query", payload: `{"query":" "}`, want: "query is required"},
		{name: "omitted query", tool: "clickhouse_query", payload: `{}`, want: "query is required"},
		{name: "null query", tool: "clickhouse_query", payload: `{"query":null}`, want: "query is required"},
		{name: "second statement", tool: "clickhouse_query", payload: `{"query":"SELECT 1; SELECT 2"}`, want: "exactly one"},
		{name: "zero max rows", tool: "clickhouse_query", payload: `{"query":"SELECT 1","max_rows":0}`, want: "positive integer"},
		{name: "blank database", tool: "clickhouse_list_tables", payload: `{"database":" "}`, want: "database must be a non-empty string"},
		{name: "null params", tool: "clickhouse_query", payload: `null`, want: "expected an object"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			opens = 0
			out, errOut, err := runCommand(t, "tools", "call", testCase.tool, "--params", testCase.payload)
			if opens != 0 {
				t.Fatalf("backend opens = %d, want 0; err=%v stdout=%q stderr=%q", opens, err, out, errOut)
			}
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestAcceptedToolCallStillOpensTheBackend(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("MCP_CLICKHOUSE_ADDRESS", "127.0.0.1:8123")
	client := &recordingQueryClient{columns: []string{"value"}, rows: [][]any{{"1"}}}
	bound := clickhousetoolset.NewToolsetWithClient("127.0.0.1:8123", "default", "", client, masking.Disabled())
	opens := 0
	previous := runtimeToolsetsOverride
	runtimeToolsetsOverride = func(context.Context, *config.StaticConfig) ([]toolset.Toolset, error) {
		opens++
		return []toolset.Toolset{bound}, nil
	}
	t.Cleanup(func() { runtimeToolsetsOverride = previous })

	out, errOut, err := runCommand(t, "tools", "call", "clickhouse_query", "--params", `{"query":"SELECT 1","unknown":null}`)
	if err != nil {
		t.Fatalf("tools call: %v stderr=%q", err, errOut)
	}
	if opens != 1 || client.query != "SELECT 1" || !strings.Contains(out, `"1"`) {
		t.Fatalf("opens=%d query=%q stdout=%q", opens, client.query, out)
	}
}

func TestMCPBlankAllowlistStopsBeforeTheBackend(t *testing.T) {
	clearConfigEnvironment(t)
	path := writeConfigCheckYAML(t, "clickhouse:\n  address: 127.0.0.1:8123\nenabled_tools: [\" \"]\n")
	opens := 0
	previous := runtimeToolsetsOverride
	runtimeToolsetsOverride = func(context.Context, *config.StaticConfig) ([]toolset.Toolset, error) {
		opens++
		return nil, fmt.Errorf("backend opened")
	}
	t.Cleanup(func() { runtimeToolsetsOverride = previous })

	_, errOut, err := runCommand(t, "--config", path, "mcp")
	if opens != 0 {
		t.Fatalf("backend opens = %d, want 0 stderr=%q err=%v", opens, errOut, err)
	}
	if err == nil || !strings.Contains(err.Error(), "no tools registered") {
		t.Fatalf("error = %v, want no tools registered", err)
	}
}

var allClickHouseTools = []string{
	"clickhouse_list_databases",
	"clickhouse_list_tables",
	"clickhouse_describe_table",
	"clickhouse_query",
}

func listedToolNames(t *testing.T, out string) []string {
	t.Helper()
	var summaries []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(out), &summaries); err != nil {
		t.Fatalf("decode tools: %v out=%s", err, out)
	}
	names := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		names = append(names, summary.Name)
	}
	return names
}
