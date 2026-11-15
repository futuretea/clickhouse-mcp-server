package clickhouse

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
	internalhttp "github.com/futuretea/clickhouse-mcp-server/pkg/server/http"
	mcpserver "github.com/futuretea/clickhouse-mcp-server/pkg/server/mcp"
	"github.com/futuretea/clickhouse-mcp-server/pkg/toolset"
)

func TestMCPToolsCallOmitsConfiguredValue(t *testing.T) {
	sensitiveIP := net.ParseIP("203.0.113.10").To4()
	policy := mustPolicy(t, `{"enabled":true,"values":["synthetic-token-value","`+sensitiveIP.String()+`"]}`)
	client := &fakeClient{responses: []fakeResponse{{
		columns: []string{"label", "ip"},
		rows:    [][]any{{sentinelValue, sensitiveIP}},
	}}}
	bound := newToolsetWithClient("127.0.0.1:8123", "default", "", client, policy)
	server, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info", Masking: policy},
		Toolsets:     []toolset.Toolset{bound},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Close()

	httpServer := httptest.NewServer(internalhttp.NewHandler(server, &http.Server{}, ""))
	defer httpServer.Close()
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"clickhouse_query","arguments":{"query":"SELECT label"}}}`)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, httpServer.URL+internalhttp.MCPEndpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatalf("POST tools/call: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
	text := string(body)
	if strings.Contains(text, sentinelValue) || strings.Contains(text, sensitiveIP.String()) || !strings.Contains(text, "[masked]") {
		t.Fatalf("tools/call body = %s", text)
	}
	if client.calls[0].query != "SELECT label" {
		t.Fatalf("recorded query = %q", client.calls[0].query)
	}
}
