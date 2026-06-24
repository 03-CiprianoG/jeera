package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// DefaultPort is the preferred loopback port for the embedded MCP server.
const DefaultPort = 7777

// Status is a snapshot of the embedded MCP server's state, for the TUI to show.
type Status struct {
	Listening bool
	Addr      string // host:port, e.g. 127.0.0.1:7777
	URL       string // http://127.0.0.1:7777
	Err       error
}

// Server runs the MCP server over local HTTP next to the TUI. The TUI owns the
// terminal, so the server is configured to log nothing to stdout/stderr.
type Server struct {
	mcp  *mcpsdk.Server
	http *http.Server

	mu      sync.RWMutex
	addr    string
	listen  bool
	lastErr error
}

// NewServer builds an embeddable HTTP MCP server backed by the store service.
func NewServer(svc *Service) *Server {
	mcpSrv := NewMCPServer(svc)
	handler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return mcpSrv },
		&mcpsdk.StreamableHTTPOptions{Logger: nil}, // nil => silent; keeps the TUI frame clean
	)
	return &Server{
		mcp: mcpSrv,
		http: &http.Server{
			Handler:  handler,
			ErrorLog: stdlog.New(io.Discard, "", 0), // suppress net/http's own stderr logging
		},
	}
}

// Start binds a loopback listener (preferring the given port, falling back to a
// nearby free port, or an OS-chosen one when port is 0) and serves in a
// goroutine. It returns once the listener is bound, so the caller has the URL.
func (s *Server) Start(host string, port int) error {
	if host == "" {
		host = "127.0.0.1"
	}
	ln, err := listenLoopback(host, port)
	if err != nil {
		s.setErr(err)
		return err
	}
	s.mu.Lock()
	s.addr = ln.Addr().String()
	s.listen = true
	s.lastErr = nil
	s.mu.Unlock()

	go func() {
		if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.setErr(err)
		}
	}()
	return nil
}

// listenLoopback tries the requested port, then a small range above it, then an
// OS-assigned port, so a second Jeera instance does not collide.
func listenLoopback(host string, port int) (net.Listener, error) {
	try := func(p int) (net.Listener, error) {
		return net.Listen("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", p)))
	}
	if port == 0 {
		return try(0)
	}
	var lastErr error
	for p := port; p < port+16; p++ {
		if ln, err := try(p); err == nil {
			return ln, nil
		} else {
			lastErr = err
		}
	}
	if ln, err := try(0); err == nil {
		return ln, nil
	}
	return nil, lastErr
}

// Shutdown gracefully stops the HTTP server, closing in-flight MCP sessions.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.listen = false
	s.mu.Unlock()
	return s.http.Shutdown(ctx)
}

// Status returns the current server state for the TUI's MCP indicator.
func (s *Server) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := Status{Listening: s.listen, Addr: s.addr, Err: s.lastErr}
	if s.addr != "" {
		st.URL = "http://" + s.addr
	}
	return st
}

func (s *Server) setErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = err
	s.listen = false
}

// ClientConfigJSON returns a copy-paste MCP client configuration pointing at
// this server, which the TUI shows so a user can connect an agent in one step.
func (s *Server) ClientConfigJSON() string {
	url := s.Status().URL
	if url == "" {
		url = fmt.Sprintf("http://127.0.0.1:%d", DefaultPort)
	}
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"jeera": map[string]any{"type": "http", "url": url},
		},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return string(b)
}
