package clickhouse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
	"github.com/futuretea/clickhouse-mcp-server/pkg/toolset"
)

func TestToolDefinitions(t *testing.T) {
	tools := newToolsetWithClient("127.0.0.1:8123", "default", "", &fakeClient{}).GetTools()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Tool.Name)
		if tool.Domain != Domain {
			t.Fatalf("tool %q domain = %q, want %q", tool.Tool.Name, tool.Domain, Domain)
		}
		if tool.Tool.Annotations.ReadOnlyHint == nil || !*tool.Tool.Annotations.ReadOnlyHint ||
			tool.Tool.Annotations.DestructiveHint == nil || *tool.Tool.Annotations.DestructiveHint ||
			tool.Tool.Annotations.IdempotentHint == nil || !*tool.Tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q must declare read-only, non-destructive, idempotent metadata", tool.Tool.Name)
		}
	}
	sort.Strings(names)
	want := []string{"clickhouse_describe_table", "clickhouse_list_databases", "clickhouse_list_tables", "clickhouse_query"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("tool names = %v, want %v", names, want)
	}
}

func TestDiscoveryToolsUseParameters(t *testing.T) {
	client := &fakeClient{responses: []fakeResponse{
		{columns: []string{"name"}, rows: [][]any{{"default"}, {"system"}}},
		{columns: []string{"name", "engine", "comment"}, rows: [][]any{{"events", "MergeTree", "event data"}}},
		{columns: []string{"name", "type", "default_kind", "default_expression", "comment"}, rows: [][]any{{"id", "UInt64", "", "", "identifier"}}},
	}}
	toolset := newToolsetWithClient("127.0.0.1:8123", "default", "", client)
	handlers := handlersByName(toolset.GetTools())

	got, err := handlers["clickhouse_list_databases"](t.Context(), nil)
	if err != nil {
		t.Fatalf("list databases: %v", err)
	}
	if got != `{"databases":["default","system"]}` {
		t.Fatalf("list databases = %s", got)
	}

	got, err = handlers["clickhouse_list_tables"](t.Context(), nil)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if got != `{"database":"default","tables":[{"name":"events","engine":"MergeTree","comment":"event data"}]}` {
		t.Fatalf("list tables = %s", got)
	}

	got, err = handlers["clickhouse_describe_table"](t.Context(), map[string]any{"database": "analytics", "table": "events"})
	if err != nil {
		t.Fatalf("describe table: %v", err)
	}
	if got != `{"database":"analytics","table":"events","columns":[{"name":"id","type":"UInt64","default_kind":"","default_expression":"","comment":"identifier"}]}` {
		t.Fatalf("describe table = %s", got)
	}

	if len(client.calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(client.calls))
	}
	if client.calls[0].parameters != nil {
		t.Fatalf("database query parameters = %v, want nil", client.calls[0].parameters)
	}
	if !reflect.DeepEqual(client.calls[1].parameters, map[string]string{"database": "default"}) {
		t.Fatalf("table parameters = %v", client.calls[1].parameters)
	}
	if !reflect.DeepEqual(client.calls[2].parameters, map[string]string{"database": "analytics", "table": "events"}) {
		t.Fatalf("describe parameters = %v", client.calls[2].parameters)
	}
	if strings.Contains(client.calls[1].query, "analytics") || strings.Contains(client.calls[2].query, "events") {
		t.Fatalf("external identifiers must not be concatenated into SQL: %+v", client.calls)
	}
}

func TestQueryFormatsRowsAndTruncates(t *testing.T) {
	rows := make([][]any, defaultMaxRows+1)
	for i := range rows {
		rows[i] = []any{i, nil}
	}
	client := &fakeClient{responses: []fakeResponse{{columns: []string{"number", "missing"}, rows: rows}}}
	handler := handlersByName(newToolsetWithClient("127.0.0.1:8123", "default", "", client).GetTools())["clickhouse_query"]

	encoded, err := handler(t.Context(), map[string]any{"query": "SELECT number, NULL FROM numbers(1001)"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	var result queryResult
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		fatalf := "decode result: %v"
		t.Fatalf(fatalf, err)
	}
	if result.RowCount != defaultMaxRows || len(result.Rows) != defaultMaxRows || !result.Truncated {
		t.Fatalf("result count=%d rows=%d truncated=%t", result.RowCount, len(result.Rows), result.Truncated)
	}
	if got := result.Rows[0]; !reflect.DeepEqual(got, []any{"0", nil}) {
		t.Fatalf("first row = %#v", got)
	}
	if got := client.calls[0].query; got != "SELECT number, NULL FROM numbers(1001)" {
		t.Fatalf("query = %q", got)
	}
}

func TestQueryCustomMaxRowsReadsExtraRow(t *testing.T) {
	client := &fakeClient{responses: []fakeResponse{{columns: []string{"number"}, rows: [][]any{{1}, {2}, {3}}}}}
	handler := handlersByName(newToolsetWithClient("127.0.0.1:8123", "default", "", client).GetTools())["clickhouse_query"]

	encoded, err := handler(t.Context(), map[string]any{"query": "SELECT 1", "max_rows": float64(2)})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	var result queryResult
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.RowCount != 2 || !result.Truncated || !reflect.DeepEqual(result.Rows, [][]any{{"1"}, {"2"}}) {
		t.Fatalf("result = %#v", result)
	}
	if client.rows[0].nextCalls != 3 || client.rows[0].scanCalls != 2 {
		t.Fatalf("next calls=%d scan calls=%d, want 3 and 2", client.rows[0].nextCalls, client.rows[0].scanCalls)
	}
}

func TestQueryRejectsUnsafeParameters(t *testing.T) {
	client := &fakeClient{}
	handler := handlersByName(newToolsetWithClient("127.0.0.1:8123", "default", "", client).GetTools())["clickhouse_query"]
	tests := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{name: "missing query", params: map[string]any{}, want: "query is required"},
		{name: "multiple statements", params: map[string]any{"query": "SELECT 1; SELECT 2"}, want: "exactly one"},
		{name: "zero max rows", params: map[string]any{"query": "SELECT 1", "max_rows": 0}, want: "positive integer"},
		{name: "negative max rows", params: map[string]any{"query": "SELECT 1", "max_rows": -1}, want: "positive integer"},
		{name: "fractional max rows", params: map[string]any{"query": "SELECT 1", "max_rows": 1.5}, want: "positive integer"},
		{name: "string max rows", params: map[string]any{"query": "SELECT 1", "max_rows": "10"}, want: "positive integer"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := handler(t.Context(), testCase.params)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want %q", err, testCase.want)
			}
		})
	}
	if len(client.calls) != 0 {
		t.Fatalf("unsafe requests must not query ClickHouse: %+v", client.calls)
	}
}

func TestValidateSingleStatement(t *testing.T) {
	tests := []struct {
		query   string
		wantErr string
	}{
		{query: "SELECT ';'"},
		{query: "SELECT `semi;colon` /* ; */ -- ;\n"},
		{query: "SELECT 1; /* trailing comment */"},
		{query: "-- comment only", wantErr: "required"},
		{query: "SELECT 1; SELECT 2", wantErr: "exactly one"},
		{query: "SELECT 'missing", wantErr: "unterminated"},
		{query: "SELECT 1 /* missing", wantErr: "unterminated"},
	}
	for _, testCase := range tests {
		t.Run(testCase.query, func(t *testing.T) {
			err := validateSingleStatement(testCase.query)
			if testCase.wantErr == "" && err != nil {
				t.Fatalf("validateSingleStatement: %v", err)
			}
			if testCase.wantErr != "" && (err == nil || !strings.Contains(err.Error(), testCase.wantErr)) {
				t.Fatalf("error = %v, want %q", err, testCase.wantErr)
			}
		})
	}
}

func TestQueryErrorDoesNotExposePassword(t *testing.T) {
	password := "redaction-value"
	client := &fakeClient{err: fmt.Errorf("request to http://alice:%s@localhost:8123 failed", password)}
	handler := handlersByName(newToolsetWithClient("localhost:8123", "default", password, client).GetTools())["clickhouse_query"]
	_, err := handler(t.Context(), map[string]any{"query": "SELECT 1"})
	if err == nil || strings.Contains(err.Error(), password) || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("error = %v", err)
	}
}

func TestClickHouseOptionsUseHTTP(t *testing.T) {
	options := clickHouseOptions(config.ClickHouseConfig{Address: "localhost:8123", Database: "analytics", Username: "alice", Password: "secret"})
	if options.Protocol != ch.HTTP || options.TLS != nil || options.Auth.Database != "analytics" || options.Addr[0] != "localhost:8123" {
		t.Fatalf("HTTP options = %+v", options)
	}
	secure := clickHouseOptions(config.ClickHouseConfig{Address: "localhost:8443", Database: "default", Secure: true})
	if secure.Protocol != ch.HTTP || secure.TLS == nil {
		t.Fatalf("HTTPS options = %+v", secure)
	}
}

func handlersByName(tools []toolset.ServerTool) map[string]toolset.ToolHandler {
	handlers := make(map[string]toolset.ToolHandler, len(tools))
	for _, tool := range tools {
		handlers[tool.Tool.Name] = tool.Handler
	}
	return handlers
}

type fakeClient struct {
	responses []fakeResponse
	err       error
	calls     []queryCall
	rows      []*fakeRows
}

type fakeResponse struct {
	columns []string
	rows    [][]any
	err     error
}

type queryCall struct {
	query      string
	parameters map[string]string
}

func (c *fakeClient) Query(_ context.Context, query string, parameters map[string]string) (RowCursor, error) {
	call := queryCall{query: query}
	if parameters != nil {
		call.parameters = make(map[string]string, len(parameters))
		for key, value := range parameters {
			call.parameters[key] = value
		}
	}
	c.calls = append(c.calls, call)
	if c.err != nil {
		return nil, c.err
	}
	if len(c.responses) == 0 {
		return nil, errors.New("no fake response configured")
	}
	response := c.responses[0]
	c.responses = c.responses[1:]
	if response.err != nil {
		return nil, response.err
	}
	rows := &fakeRows{columns: response.columns, values: response.rows, position: -1}
	c.rows = append(c.rows, rows)
	return rows, nil
}

type fakeRows struct {
	columns   []string
	values    [][]any
	position  int
	nextCalls int
	scanCalls int
}

func (r *fakeRows) Columns() []string       { return append([]string(nil), r.columns...) }
func (*fakeRows) ScanTypes() []reflect.Type { return nil }

func (r *fakeRows) Next() bool {
	r.nextCalls++
	r.position++
	return r.position < len(r.values)
}

func (r *fakeRows) Scan(destinations ...any) error {
	r.scanCalls++
	if r.position < 0 || r.position >= len(r.values) || len(destinations) != len(r.values[r.position]) {
		return errors.New("invalid fake scan")
	}
	for i, value := range r.values[r.position] {
		destination, ok := destinations[i].(*any)
		if !ok {
			return errors.New("fake scan expects *any destinations")
		}
		*destination = value
	}
	return nil
}

func (*fakeRows) Err() error   { return nil }
func (*fakeRows) Close() error { return nil }
