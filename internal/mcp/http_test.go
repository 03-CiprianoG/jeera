package mcp

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestHTTPEndToEnd starts the real embedded server and drives it with a real MCP
// client over Streamable HTTP, proving the transport, schema inference and tool
// dispatch all wire together and that a tool call lands in the shared store.
func TestHTTPEndToEnd(t *testing.T) {
	svc := newTestService(t)
	srv := NewServer(svc)
	if err := srv.Start("127.0.0.1", 0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	st := srv.Status()
	if !st.Listening || st.URL == "" {
		t.Fatalf("server not listening: %+v", st)
	}

	tctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(tctx, &mcpsdk.StreamableClientTransport{Endpoint: st.URL}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	// The server advertises all registered tools.
	tools, err := session.ListTools(tctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools.Tools) < 15 {
		t.Errorf("expected >=15 tools, got %d", len(tools.Tools))
	}
	if !hasTool(tools.Tools, "create_issue") || !hasTool(tools.Tools, "transition_issue") {
		t.Errorf("missing expected tools in %v", toolNames(tools.Tools))
	}

	// Calling create_issue over the wire creates a real issue in the store.
	res, err := session.CallTool(tctx, &mcpsdk.CallToolParams{
		Name:      "create_issue",
		Arguments: map[string]any{"project": "JEE", "title": "filed by an agent"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %+v", res.Content)
	}
	sc, ok := res.StructuredContent.(map[string]any)
	if !ok || sc["key"] != "JEE-1" {
		t.Errorf("structured result = %#v, want key JEE-1", res.StructuredContent)
	}

	// Confirm it is visible through the same store the TUI would read.
	_, list, err := svc.listIssues(context.Background(), nil, ListIssuesArgs{Project: "JEE"})
	if err != nil {
		t.Fatalf("listIssues: %v", err)
	}
	if len(list.Issues) != 1 || list.Issues[0].Title != "filed by an agent" {
		t.Errorf("store does not reflect the MCP call: %+v", list.Issues)
	}

	// A not-found surfaces as a tool error, not a transport failure.
	bad, err := session.CallTool(tctx, &mcpsdk.CallToolParams{
		Name:      "get_issue",
		Arguments: map[string]any{"key": "JEE-999"},
	})
	if err != nil {
		t.Fatalf("CallTool(bad) transport error: %v", err)
	}
	if !bad.IsError {
		t.Errorf("expected IsError for unknown issue, got %+v", bad)
	}

	// A validation/integrity error (invalid enum) also surfaces as IsError.
	invalid, err := session.CallTool(tctx, &mcpsdk.CallToolParams{
		Name:      "create_issue",
		Arguments: map[string]any{"project": "JEE", "title": "x", "priority": "urgent"},
	})
	if err != nil {
		t.Fatalf("CallTool(invalid) transport error: %v", err)
	}
	if !invalid.IsError {
		t.Errorf("expected IsError for invalid priority, got %+v", invalid)
	}
}

func TestServerStatusLifecycle(t *testing.T) {
	svc := newTestService(t)
	srv := NewServer(svc)

	if st := srv.Status(); st.Listening || st.URL != "" {
		t.Errorf("pre-start status = %+v, want not listening / empty URL", st)
	}
	if err := srv.Start("127.0.0.1", 0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if st := srv.Status(); !st.Listening || st.URL == "" {
		t.Errorf("post-start status = %+v, want listening with a URL", st)
	}

	ctx1, c1 := context.WithTimeout(context.Background(), 2*time.Second)
	defer c1()
	if err := srv.Shutdown(ctx1); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if srv.Status().Listening {
		t.Error("still listening after shutdown")
	}
	// Shutdown is idempotent.
	ctx2, c2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer c2()
	if err := srv.Shutdown(ctx2); err != nil {
		t.Errorf("second shutdown: %v", err)
	}
}

func TestServerPortFallback(t *testing.T) {
	// Occupy a port, then ask the server to prefer it; it must bind a nearby one.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer occupied.Close()
	_, portStr, _ := net.SplitHostPort(occupied.Addr().String())
	port, _ := strconv.Atoi(portStr)

	srv := NewServer(newTestService(t))
	if err := srv.Start("127.0.0.1", port); err != nil {
		t.Fatalf("Start with busy preferred port: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	st := srv.Status()
	if !st.Listening {
		t.Fatal("server did not bind after fallback")
	}
	if strings.HasSuffix(st.Addr, ":"+portStr) {
		t.Errorf("expected fallback off the occupied port %d, but bound %s", port, st.Addr)
	}
}

func TestClientConfigJSONBeforeStart(t *testing.T) {
	srv := NewServer(newTestService(t))
	cfg := srv.ClientConfigJSON()
	if !strings.Contains(cfg, strconv.Itoa(DefaultPort)) {
		t.Errorf("pre-start config should advertise the default port %d:\n%s", DefaultPort, cfg)
	}
}

func hasTool(tools []*mcpsdk.Tool, name string) bool {
	for _, tl := range tools {
		if tl.Name == name {
			return true
		}
	}
	return false
}

func toolNames(tools []*mcpsdk.Tool) []string {
	out := make([]string, len(tools))
	for i, tl := range tools {
		out[i] = tl.Name
	}
	return out
}
