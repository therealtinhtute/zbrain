// Package mcp implements the trusted-agent stdio MCP gateway.
//
// stdout is protocol-only: the SDK writes JSON-RPC frames to os.Stdout and
// every diagnostic (including SDK server logs) flows to stderr.
package mcp

import (
	"context"
	"io"
	"log/slog"
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

// newServer builds the MCP server with the seven tools and two resources
// registered. Tests drive it in-process over in-memory transports.
func newServer(opts Options) (*mcp.Server, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
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
