// Package mcpserver exposes the canonical DebtDrone scanner through MCP.
package mcpserver

import (
	"context"
	"errors"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/service"
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
	config   localconfig.Values
	scan     scanRunner
	scanSlot chan struct{}
}

type scanRunner func(context.Context, string, service.ScanOptions, func(service.ScanProgress)) (scanner.Report, error)

// New creates an MCP server scoped to root.
func New(root, version string) *Server {
	return NewConfigured(root, version, localconfig.Defaults())
}

func newServer(root, version string, scan scanFunc) *Server {
	return newServerWithRunner(root, version, localconfig.Defaults(), func(ctx context.Context, path string, options service.ScanOptions, progress func(service.ScanProgress)) (scanner.Report, error) {
		return scan(ctx, path, scanner.Options{
			Scope:      scanner.FullScan(),
			Complexity: scanner.ComplexityOptions{Enabled: true, MaxCyclomatic: options.MaxComplexity},
			Security:   scanner.SecurityOptions{Enabled: options.SecurityScan},
			Coverage:   scanner.CoverageOptions{Enabled: options.Coverage},
		})
	})
}

// NewConfigured creates an MCP server whose scan defaults and history behavior
// come from the same resolved local configuration used by CLI and TUI scans.
func NewConfigured(root, version string, config localconfig.Values) *Server {
	scanService := service.NewScanServiceWithHistoryEnabled(config.HistoryEnabled)
	return newServerWithRunner(root, version, config, scanService.RunReport)
}

func newServerWithRunner(root, version string, config localconfig.Values, scan scanRunner) *Server {
	server := &Server{
		root: root,
		protocol: mcp.NewServer(&mcp.Implementation{
			Name:    serverName,
			Version: version,
		}, &mcp.ServerOptions{
			Capabilities: &mcp.ServerCapabilities{},
		}),
		config:   config,
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
func RunStdio(ctx context.Context, root, version string, config localconfig.Values) error {
	return NewConfigured(root, version, config).Run(ctx, &mcp.StdioTransport{})
}

func isNormalDisconnect(err error) bool {
	var rpcErr *jsonrpc.Error
	return errors.As(err, &rpcErr) && rpcErr.Code == serverClosingCode
}
