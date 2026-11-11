package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/futuretea/clickhouse-mcp-server/pkg/core/config"
	internalhttp "github.com/futuretea/clickhouse-mcp-server/pkg/server/http"
	mcpserver "github.com/futuretea/clickhouse-mcp-server/pkg/server/mcp"
	"github.com/futuretea/clickhouse-mcp-server/pkg/toolset"
	clickhousetoolset "github.com/futuretea/clickhouse-mcp-server/pkg/toolset/clickhouse"
)

func TestHealthz(t *testing.T) {
	mcpServer, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{newClickHouseToolset(t)},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer mcpServer.Close()

	httpServer := &http.Server{}
	handler := internalhttp.NewHandler(mcpServer, httpServer, "")

	req := mustNewRequest(t, http.MethodGet, internalhttp.HealthEndpoint)
	rec := &responseRecorder{header: make(http.Header)}
	handler.ServeHTTP(rec, req)
	if rec.status != http.StatusOK || string(rec.body) != "healthy" {
		t.Fatalf("healthy status=%d body=%q", rec.status, rec.body)
	}

	testServer := httptest.NewServer(handler)
	defer testServer.Close()
	client := testServer.Client()
	client.Timeout = 2 * time.Second
	req, err = http.NewRequestWithContext(t.Context(), http.MethodHead, testServer.URL+internalhttp.HealthEndpoint, nil)
	if err != nil {
		t.Fatalf("NewRequest HEAD: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("HEAD healthz: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read HEAD healthz body: %v", err)
	}
	if resp.StatusCode != http.StatusOK || len(body) != 0 || resp.ContentLength != int64(len("healthy")) {
		t.Fatalf("HEAD healthy status=%d content_length=%d body=%q", resp.StatusCode, resp.ContentLength, body)
	}

	req = mustNewRequest(t, http.MethodPost, internalhttp.HealthEndpoint)
	rec = &responseRecorder{header: make(http.Header)}
	handler.ServeHTTP(rec, req)
	if rec.status != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.status)
	}
}

func TestStreamableHTTPListsClickHouseTools(t *testing.T) {
	mcpServer, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{newClickHouseToolset(t)},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer mcpServer.Close()

	testServer := httptest.NewServer(internalhttp.NewHandler(mcpServer, &http.Server{}, ""))
	defer testServer.Close()
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, testServer.URL+internalhttp.MCPEndpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := testServer.Client().Do(req)
	if err != nil {
		t.Fatalf("POST tools/list: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("tools/list status=%d body=%s", resp.StatusCode, body)
	}

	var result struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	names := make(map[string]bool, len(result.Result.Tools))
	for _, tool := range result.Result.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"clickhouse_list_databases", "clickhouse_list_tables", "clickhouse_describe_table", "clickhouse_query"} {
		if !names[name] {
			t.Fatalf("tools/list missing %s: %+v", name, result.Result.Tools)
		}
	}
}

func TestRequestMiddlewareLogsPathWithoutQueryString(t *testing.T) {
	var logs bytes.Buffer
	handler := internalhttp.RequestMiddlewareWithLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), zerolog.New(&logs).Level(zerolog.DebugLevel))

	request := httptest.NewRequest(http.MethodGet, "http://example.test/mcp?access_token=must-not-appear", nil)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	output := logs.String()
	if !strings.Contains(output, `"path":"/mcp"`) || !strings.Contains(output, `"status":204`) {
		t.Fatalf("log output = %q", output)
	}
	if strings.Contains(output, "?") || strings.Contains(output, "must-not-appear") {
		t.Fatalf("log output leaked a query string: %q", output)
	}
}

func TestServeListenerShutdown(t *testing.T) {
	mcpServer, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{newClickHouseToolset(t)},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer mcpServer.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	cfg := &config.StaticConfig{Port: listener.Addr().(*net.TCPAddr).Port, Listen: "127.0.0.1"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- internalhttp.ServeListener(ctx, mcpServer, cfg, listener)
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + listener.Addr().String() + internalhttp.HealthEndpoint)
	if err != nil {
		t.Fatalf("GET healthz: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read healthz body: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "healthy" {
		t.Fatalf("healthz status=%d body=%q", resp.StatusCode, body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ServeListener: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeListener did not exit after cancel")
	}
}

func newClickHouseToolset(t *testing.T) *clickhousetoolset.Toolset {
	t.Helper()
	return clickhousetoolset.NewCatalogToolset(config.ClickHouseConfig{Address: "127.0.0.1:8123", Database: "default"})
}

type responseRecorder struct {
	header http.Header
	status int
	body   []byte
}

func (r *responseRecorder) Header() http.Header { return r.header }
func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	r.body = append(r.body, b...)
	return len(b), nil
}
func (r *responseRecorder) WriteHeader(statusCode int) { r.status = statusCode }

func mustNewRequest(t *testing.T, method, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, path, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	return req
}
