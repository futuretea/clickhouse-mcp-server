//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	mcpprotocol "github.com/mark3labs/mcp-go/mcp"
)

const (
	serverStartupTimeout  = 10 * time.Second
	serverShutdownTimeout = 5 * time.Second
	requestTimeout        = 2 * time.Second
)

func TestIntegrationMCPHTTP(t *testing.T) {
	if strings.TrimSpace(os.Getenv("MCP_CLICKHOUSE_ADDRESS")) == "" {
		t.Fatal("MCP_CLICKHOUSE_ADDRESS is required; start the local Docker ClickHouse instance documented in README.md")
	}

	address := reserveListenAddress(t)
	server := startServer(t, buildServerBinary(t), address)
	client := &http.Client{}
	endpoint := "http://" + address
	waitForHealth(t, client, endpoint, server)

	initialize := postMCP(t, client, endpoint, 1, "initialize", map[string]any{
		"protocolVersion": mcpprotocol.LATEST_PROTOCOL_VERSION,
		"clientInfo": map[string]string{
			"name":    "clickhouse-mcp-server-e2e",
			"version": "test",
		},
		"capabilities": map[string]any{},
	})
	if initialize.Error != nil {
		t.Fatalf("initialize error: %s", initialize.Error.Message)
	}
	if initialize.Result.ProtocolVersion != mcpprotocol.LATEST_PROTOCOL_VERSION {
		t.Fatalf("initialize protocol version = %q, want %q", initialize.Result.ProtocolVersion, mcpprotocol.LATEST_PROTOCOL_VERSION)
	}

	tools := postMCP(t, client, endpoint, 2, "tools/list", nil)
	if tools.Error != nil {
		t.Fatalf("tools/list error: %s", tools.Error.Message)
	}
	if !containsTool(tools.Result.Tools, "clickhouse_query") {
		t.Fatalf("tools/list missing clickhouse_query: %+v", tools.Result.Tools)
	}

	query := postMCP(t, client, endpoint, 3, "tools/call", map[string]any{
		"name": "clickhouse_query",
		"arguments": map[string]any{
			"query": "SELECT 42 AS value",
		},
	})
	if query.Error != nil {
		t.Fatalf("query protocol error: %s", query.Error.Message)
	}
	if query.Result.IsError {
		t.Fatalf("query tool error: %s", query.Result.text())
	}

	var queryResult struct {
		Columns  []string   `json:"columns"`
		Rows     [][]string `json:"rows"`
		RowCount int        `json:"row_count"`
	}
	if err := json.Unmarshal([]byte(query.Result.text()), &queryResult); err != nil {
		t.Fatalf("decode query result: %v", err)
	}
	if queryResult.RowCount != 1 || len(queryResult.Columns) != 1 || queryResult.Columns[0] != "value" ||
		len(queryResult.Rows) != 1 || len(queryResult.Rows[0]) != 1 || queryResult.Rows[0][0] != "42" {
		t.Fatalf("unexpected query result: %s", query.Result.text())
	}

	mutation := postMCP(t, client, endpoint, 4, "tools/call", map[string]any{
		"name": "clickhouse_query",
		"arguments": map[string]any{
			"query": "CREATE TEMPORARY TABLE clickhouse_mcp_http_e2e_test (id UInt8)",
		},
	})
	if mutation.Error != nil {
		t.Fatalf("temporary DDL protocol error: %s", mutation.Error.Message)
	}
	if !mutation.Result.IsError || !strings.Contains(strings.ToLower(mutation.Result.text()), "readonly") {
		t.Fatalf("temporary DDL result = %+v, want a readonly error", mutation.Result)
	}
}

func buildServerBinary(t *testing.T) string {
	t.Helper()

	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "clickhouse-mcp-server")
	command := exec.Command("go", "build", "-o", binary, "./cmd/mcp-server")
	command.Dir = repoRoot
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build MCP server: %v\n%s", err, output)
	}
	return binary
}

func reserveListenAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve HTTP listen address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release HTTP listen address: %v", err)
	}
	return address
}

func startServer(t *testing.T, binary, address string) *serverProcess {
	t.Helper()

	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split HTTP listen address: %v", err)
	}
	command := exec.Command(binary, "mcp", "--listen", "127.0.0.1", "--port", port)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		t.Fatalf("start MCP server: %v", err)
	}

	server := &serverProcess{command: command, done: make(chan struct{})}
	go func() {
		err := command.Wait()
		server.mu.Lock()
		server.err = err
		server.mu.Unlock()
		close(server.done)
	}()
	t.Cleanup(func() { server.stop(t) })
	return server
}

func waitForHealth(t *testing.T, client *http.Client, endpoint string, server *serverProcess) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), serverStartupTimeout)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		requestCtx, requestCancel := context.WithTimeout(ctx, requestTimeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint+"/healthz", nil)
		if err != nil {
			requestCancel()
			t.Fatalf("create health request: %v", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			requestCancel()
			if readErr != nil {
				t.Fatalf("read health response: %v", readErr)
			}
			if resp.StatusCode == http.StatusOK && string(body) == "healthy" {
				return
			}
		} else {
			requestCancel()
		}

		select {
		case <-server.done:
			t.Fatalf("MCP server exited before becoming healthy: %v", server.exitError())
		case <-ctx.Done():
			t.Fatalf("MCP server did not become healthy: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func postMCP(t *testing.T, client *http.Client, endpoint string, id int, method string, params any) mcpResponse {
	t.Helper()

	payload, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		t.Fatalf("encode %s request: %v", method, err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/mcp", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create %s request: %v", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s status=%d", method, resp.StatusCode)
	}

	var response mcpResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&response); err != nil {
		t.Fatalf("decode %s response: %v", method, err)
	}
	return response
}

func containsTool(tools []mcpTool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

type serverProcess struct {
	command *exec.Cmd
	done    chan struct{}
	mu      sync.Mutex
	err     error
}

func (s *serverProcess) exitError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *serverProcess) stop(t *testing.T) {
	t.Helper()
	select {
	case <-s.done:
		return
	default:
	}

	if err := s.command.Process.Signal(os.Interrupt); err != nil {
		t.Errorf("interrupt MCP server: %v", err)
		return
	}
	select {
	case <-s.done:
		if err := s.exitError(); err != nil {
			t.Errorf("MCP server exited after interrupt: %v", err)
		}
	case <-time.After(serverShutdownTimeout):
		if err := s.command.Process.Kill(); err != nil {
			t.Errorf("kill MCP server after shutdown timeout: %v", err)
			return
		}
		<-s.done
		t.Errorf("MCP server did not exit within %s", serverShutdownTimeout)
	}
}

type mcpResponse struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Result mcpResult `json:"result"`
}

type mcpResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Tools           []mcpTool    `json:"tools"`
	Content         []mcpContent `json:"content"`
	IsError         bool         `json:"isError"`
}

func (r mcpResult) text() string {
	var text strings.Builder
	for _, content := range r.Content {
		if content.Type == "text" {
			text.WriteString(content.Text)
		}
	}
	return text.String()
}

type mcpTool struct {
	Name string `json:"name"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (r mcpResult) String() string {
	return fmt.Sprintf("is_error=%t content=%q", r.IsError, r.text())
}
