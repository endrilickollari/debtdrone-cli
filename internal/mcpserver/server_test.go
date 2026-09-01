package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalDisconnectClassification(t *testing.T) {
	closing := &jsonrpc.Error{Code: serverClosingCode, Message: "server is closing"}
	assert.True(t, isNormalDisconnect(fmt.Errorf("close MCP session: %w", closing)))
	assert.False(t, isNormalDisconnect(&jsonrpc.Error{Code: -32603, Message: "internal error"}))
	assert.False(t, isNormalDisconnect(errors.New("transport failed")))
}

func TestServerInitializesAndStopsOnCancellation(t *testing.T) {
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	server := New(t.TempDir(), "test-version")

	done := make(chan error, 1)
	go func() {
		done <- server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	connectCtx, stopConnecting := context.WithTimeout(context.Background(), time.Second)
	defer stopConnecting()
	session, err := client.Connect(connectCtx, clientTransport, nil)
	require.NoError(t, err)

	result := session.InitializeResult()
	require.NotNil(t, result)
	require.NotNil(t, result.ServerInfo)
	assert.Equal(t, serverName, result.ServerInfo.Name)
	assert.Equal(t, "test-version", result.ServerInfo.Version)

	cancel()
	select {
	case err := <-done:
		assert.True(t, errors.Is(err, context.Canceled), "server returned %v", err)
	case <-time.After(time.Second):
		t.Fatal("server did not stop after cancellation")
	}
}
