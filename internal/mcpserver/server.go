// Package mcpserver exposes the canonical DebtDrone scanner through MCP.
package mcpserver

import (
	"context"
	"errors"

	"github.com/endrilickollari/debtdrone-cli/v2/scanner"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName = "debtdrone"
	// The Go MCP SDK uses this JSON-RPC extension code when a session closes
	// while a cancelled request is still unwinding.
	serverClosingCode = -32004
	// Keep this limit synchronized with the scan_repository tool description.
	maximumConcurrentScans = 1
)

// Server owns one MCP protocol server and its configured repository boundary.
type Server struct {
	root     string
	protocol *mcp.Server
	scan     scanFunc
	scanSlot chan struct{}
}

// New creates an MCP server scoped to root.
func New(root, version string) *Server {
	return newServer(root, version, scanner.Scan)
}

func newServer(root, version string, scan scanFunc) *Server {
	server := &Server{
		root: root,
		protocol: mcp.NewServer(&mcp.Implementation{
			Name:    serverName,
			Version: version,
		}, &mcp.ServerOptions{
			Capabilities: &mcp.ServerCapabilities{},
		}),
		scan:     scan,
		scanSlot: make(chan struct{}, maximumConcurrentScans),
	}
	server.addScanRepositoryTool()
	return server
}

// Run serves a single MCP session until the client disconnects or ctx is cancelled.
func (s *Server) Run(ctx context.Context, transport mcp.Transport) error {
	err := s.protocol.Run(ctx, transport)
	if isNormalDisconnect(err) {
		return nil
	}
	return err
}

// RunStdio serves MCP over the process stdin and stdout streams.
func RunStdio(ctx context.Context, root, version string) error {
	return New(root, version).Run(ctx, &mcp.StdioTransport{})
}

func isNormalDisconnect(err error) bool {
	var rpcErr *jsonrpc.Error
	return errors.As(err, &rpcErr) && rpcErr.Code == serverClosingCode
}
