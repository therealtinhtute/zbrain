package mcp

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

// ServerName is the MCP server implementation name reported to clients.
const ServerName = "zbrain"

// Options carries the runtime services the MCP layer reuses from the
// CLI/runtime boundary.
type Options struct {
	Paths   zruntime.Paths
	Now     func() time.Time
	Version string
	Stderr  io.Writer
}

// Serve runs the stdio MCP server until stdin closes or ctx is cancelled.
//
// Diagnostics are routed to opts.Stderr (never stdout); SDK protocol frames
// always land on stdout via the stdio transport.
func Serve(ctx context.Context, opts Options) error {
	server, err := newServer(opts)
	if err != nil {
		return err
	}
	return server.Run(ctx, &mcp.StdioTransport{})
}

// NewServerForEval exposes the tool server for the cross-package eval
// runners (internal/eval), which drive both the CLI ask path and the MCP
// memory_ask path over the same fixture set. It is identical to newServer.
func NewServerForEval(opts Options) (*mcp.Server, error) {
	return newServer(opts)
}

// newServer builds the MCP server with the seven tools and two resources
// registered. Tests drive it in-process over in-memory transports.
func newServer(opts Options) (*mcp.Server, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	opts.Stderr = ensureSafeWriter(opts.Stderr)
	server := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: opts.Version}, &mcp.ServerOptions{
		Logger: slog.New(slog.NewTextHandler(opts.Stderr, nil)),
	})
	if err := registerTools(server, opts); err != nil {
		return nil, err
	}
	if err := registerResources(server, opts); err != nil {
		return nil, err
	}
	return server, nil
}

type safeWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *safeWriter) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

func ensureSafeWriter(w io.Writer) io.Writer {
	if _, ok := w.(*safeWriter); ok {
		return w
	}
	return &safeWriter{w: w}
}
