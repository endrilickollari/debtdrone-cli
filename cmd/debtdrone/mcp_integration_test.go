package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/mcpserver"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	currentMCPProtocolVersion = "2026-07-28"
	legacyMCPProtocolVersion  = "2025-11-25"
	fakeTrivyMarkerEnv        = "DEBTDRONE_TEST_FAKE_TRIVY_MARKER"
)

func TestMCPStdioIntegration(t *testing.T) {
	repositoryRoot := findRepositoryRoot(t)
	binary := buildDebtdroneBinary(t, repositoryRoot)

	t.Run("initializes and discovers the read-only tool", func(t *testing.T) {
		session := connectMCPSubprocess(t, binary, repositoryRoot, nil)

		initialized := session.InitializeResult()
		require.NotNil(t, initialized)
		assert.Equal(t, currentMCPProtocolVersion, initialized.ProtocolVersion)
		require.NotNil(t, initialized.ServerInfo)
		assert.Equal(t, "debtdrone", initialized.ServerInfo.Name)
		require.NotNil(t, initialized.Capabilities)
		assert.NotNil(t, initialized.Capabilities.Tools)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		require.Len(t, result.Tools, 1)
		tool := result.Tools[0]
		assert.Equal(t, "scan_repository", tool.Name)
		require.NotNil(t, tool.Annotations)
		assert.True(t, tool.Annotations.ReadOnlyHint)
		assert.True(t, tool.Annotations.IdempotentHint)
	})

	t.Run("recovers after malformed and rejected requests", func(t *testing.T) {
		transport := startRawMCPSubprocess(t, binary, repositoryRoot)

		initialize := transport.request(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + legacyMCPProtocolVersion + `","capabilities":{},"clientInfo":{"name":"debtdrone-integration-test","version":"test"}}}`)
		require.Nil(t, initialize.Error)
		var initialized mcp.InitializeResult
		require.NoError(t, json.Unmarshal(initialize.Result, &initialized))
		assert.Equal(t, legacyMCPProtocolVersion, initialized.ProtocolVersion)
		require.NotNil(t, initialized.ServerInfo)
		assert.Equal(t, "debtdrone", initialized.ServerInfo.Name)

		transport.notify(`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`)
		malformed := transport.request(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":[]}`)
		require.NotNil(t, malformed.Error)
		assert.Equal(t, -32602, malformed.Error.Code)

		invalidCall := transport.request(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"scan_repository","arguments":"not-an-object"}}`)
		require.Nil(t, invalidCall.Error)
		var rejected mcp.CallToolResult
		require.NoError(t, json.Unmarshal(invalidCall.Result, &rejected))
		assert.True(t, rejected.IsError)

		listed := transport.request(`{"jsonrpc":"2.0","id":4,"method":"tools/list","params":{}}`)
		require.Nil(t, listed.Error)
		var tools mcp.ListToolsResult
		require.NoError(t, json.Unmarshal(listed.Result, &tools))
		require.Len(t, tools.Tools, 1)
		assert.Equal(t, "scan_repository", tools.Tools[0].Name)
		transport.close()
		assert.Empty(t, transport.stderr.String())
	})

	t.Run("matches CLI output and confines repository paths", func(t *testing.T) {
		session := connectMCPSubprocess(t, binary, repositoryRoot, nil)
		fixture := filepath.Join(repositoryRoot, "cmd", "debtdrone", "testdata", "dirty_code")
		fixtureRelative, err := filepath.Rel(repositoryRoot, fixture)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		mcpResult, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "scan_repository",
			Arguments: map[string]any{
				"path":           filepath.ToSlash(fixtureRelative),
				"max_complexity": 3,
				"security_scan":  false,
				"max_findings":   200,
			},
		})
		require.NoError(t, err)
		require.False(t, mcpResult.IsError)
		mcpOutput := decodeMCPOutput(t, mcpResult)

		cliIssues := runCLIScan(t, binary, fixture, 3)
		require.NotEmpty(t, cliIssues)
		assert.Equal(t, sharedCLIFindings(cliIssues), sharedMCPFindings(mcpOutput.Findings))

		escapeCtx, cancelEscape := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelEscape()
		escaped, err := session.CallTool(escapeCtx, &mcp.CallToolParams{
			Name:      "scan_repository",
			Arguments: map[string]any{"path": "../outside", "security_scan": false},
		})
		require.NoError(t, err)
		assert.True(t, escaped.IsError)

		listed, err := session.ListTools(escapeCtx, nil)
		require.NoError(t, err)
		require.Len(t, listed.Tools, 1, "a rejected path must not corrupt the MCP stream")
	})

	t.Run("cancels an active scanner process without corrupting the session", func(t *testing.T) {
		fakeBin := t.TempDir()
		installFakeTrivy(t, repositoryRoot, fakeBin)
		marker := filepath.Join(t.TempDir(), "started")
		environment := []string{
			fakeTrivyMarkerEnv + "=" + marker,
			"PATH=" + fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
		}
		cancelRoot := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(cancelRoot, "README.md"), []byte("fixture"), 0o600))
		session := connectMCPSubprocess(t, binary, cancelRoot, environment)

		callCtx, cancelCall := context.WithCancel(context.Background())
		callDone := make(chan error, 1)
		go func() {
			_, err := session.CallTool(callCtx, &mcp.CallToolParams{
				Name:      "scan_repository",
				Arguments: map[string]any{"security_scan": true},
			})
			callDone <- err
		}()

		waitForFile(t, marker, 10*time.Second)
		cancelCall()
		select {
		case err := <-callDone:
			assert.ErrorIs(t, err, context.Canceled)
		case <-time.After(10 * time.Second):
			t.Fatal("cancelled MCP scan did not return")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		listed, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		require.Len(t, listed.Tools, 1, "cancellation must not corrupt the MCP stream")
	})
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not locate repository go.mod")
		}
		directory = parent
	}
}

func buildDebtdroneBinary(t *testing.T, repositoryRoot string) string {
	t.Helper()
	name := "debtdrone"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/debtdrone")
	cmd.Dir = repositoryRoot
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "build MCP integration binary: %s", output)
	return binary
}

func connectMCPSubprocess(t *testing.T, binary, root string, environment []string) *mcp.ClientSession {
	t.Helper()
	cmd := exec.Command(binary, "mcp", "--root", root)
	cmd.Env = mergedEnvironment(os.Environ(), environment)
	stderr := new(synchronizedBuffer)
	cmd.Stderr = stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "debtdrone-integration-test", Version: "test"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: 2 * time.Second}, nil)
	require.NoError(t, err, "connect to MCP subprocess; stderr: %s", stderr.String())
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close MCP subprocess: %v; stderr: %s", err, stderr.String())
		}
		if diagnostic := stderr.String(); diagnostic != "" {
			t.Errorf("MCP subprocess wrote to stderr: %s", diagnostic)
		}
	})
	return session
}

type rawMCPSubprocess struct {
	t      *testing.T
	cancel context.CancelFunc
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *synchronizedBuffer
	wait   <-chan error
	closed bool
}

type rawJSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func startRawMCPSubprocess(t *testing.T, binary, root string) *rawMCPSubprocess {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	cmd := exec.CommandContext(ctx, binary, "mcp", "--root", root)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stderr := new(synchronizedBuffer)
	cmd.Stderr = stderr
	require.NoError(t, cmd.Start())
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	transport := &rawMCPSubprocess{
		t: t, cancel: cancel, stdin: stdin, stdout: bufio.NewReader(stdout), stderr: stderr, wait: wait,
	}
	t.Cleanup(transport.close)
	return transport
}

func (p *rawMCPSubprocess) notify(message string) {
	p.t.Helper()
	_, err := io.WriteString(p.stdin, message+"\n")
	require.NoError(p.t, err)
}

func (p *rawMCPSubprocess) request(message string) rawJSONRPCResponse {
	p.t.Helper()
	p.notify(message)
	line, err := p.stdout.ReadBytes('\n')
	require.NoError(p.t, err, "read MCP response; stderr: %s", p.stderr.String())
	var response rawJSONRPCResponse
	require.NoError(p.t, json.Unmarshal(line, &response), "stdout contained a non-protocol line: %q", line)
	assert.Equal(p.t, "2.0", response.JSONRPC)
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	require.NoError(p.t, json.Unmarshal([]byte(message), &request))
	assert.JSONEq(p.t, string(request.ID), string(response.ID))
	return response
}

func (p *rawMCPSubprocess) close() {
	p.t.Helper()
	if p.closed {
		return
	}
	p.closed = true
	_ = p.stdin.Close()
	select {
	case err := <-p.wait:
		if err != nil && !errors.Is(err, context.Canceled) {
			p.t.Errorf("MCP subprocess exited with error: %v; stderr: %s", err, p.stderr.String())
		}
	case <-time.After(5 * time.Second):
		p.cancel()
		p.t.Errorf("MCP subprocess did not exit after stdin closed; stderr: %s", p.stderr.String())
	}
	p.cancel()
}

func decodeMCPOutput(t *testing.T, result *mcp.CallToolResult) mcpserver.ScanRepositoryOutput {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	var output mcpserver.ScanRepositoryOutput
	require.NoError(t, json.Unmarshal(encoded, &output))
	return output
}

func runCLIScan(t *testing.T, binary, repository string, maxComplexity int) []models.TechnicalDebtIssue {
	t.Helper()
	cmd := exec.Command(binary, "scan", repository, "--format=json", "--security-scan=false", "--max-complexity="+strconv.Itoa(maxComplexity))
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "AppData="+t.TempDir())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	require.NoError(t, err, "run CLI fixture scan: %s", stderr.String())
	var issues []models.TechnicalDebtIssue
	require.NoError(t, json.Unmarshal(output, &issues))
	return issues
}

type sharedFinding struct {
	Fingerprint        string
	AnalyzerID         string
	RuleID             *string
	Type               string
	Category           string
	Severity           string
	Message            string
	Description        *string
	Path               string
	Line               *int
	Column             *int
	Confidence         float64
	EstimatedDebtHours float64
	EffortMultiplier   float64
	CodeSnippet        *string
}

func sharedCLIFindings(issues []models.TechnicalDebtIssue) []sharedFinding {
	findings := make([]sharedFinding, 0, len(issues))
	for _, issue := range issues {
		findings = append(findings, sharedFinding{
			Fingerprint: issue.FingerprintHash, AnalyzerID: issue.ToolName, RuleID: issue.ToolRuleID,
			Type: issue.IssueType, Category: issue.Category, Severity: issue.Severity, Message: issue.Message,
			Description: issue.Description, Path: issue.FilePath, Line: issue.LineNumber, Column: issue.ColumnNumber,
			Confidence: issue.ConfidenceScore, EstimatedDebtHours: issue.TechnicalDebtHours,
			EffortMultiplier: issue.EffortMultiplier, CodeSnippet: issue.CodeSnippet,
		})
	}
	return findings
}

func sharedMCPFindings(findings []mcpserver.ScanFinding) []sharedFinding {
	shared := make([]sharedFinding, 0, len(findings))
	for _, finding := range findings {
		shared = append(shared, sharedFinding{
			Fingerprint: finding.Fingerprint, AnalyzerID: finding.AnalyzerID, RuleID: finding.RuleID,
			Type: finding.Type, Category: finding.Category, Severity: finding.Severity, Message: finding.Message,
			Description: finding.Description, Path: finding.Location.Path, Line: finding.Location.Line, Column: finding.Location.Column,
			Confidence: finding.Confidence, EstimatedDebtHours: finding.EstimatedDebtHours,
			EffortMultiplier: finding.EffortMultiplier, CodeSnippet: finding.CodeSnippet,
		})
	}
	return shared
}

func installFakeTrivy(t *testing.T, repositoryRoot, directory string) {
	t.Helper()
	name := "trivy"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	target := filepath.Join(directory, name)
	cmd := exec.Command("go", "build", "-o", target, "./cmd/debtdrone/testdata/faketrivy")
	cmd.Dir = repositoryRoot
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "build fake Trivy executable: %s", output)
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("inspect fake Trivy marker: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fake Trivy did not start within %s", timeout)
}

func mergedEnvironment(base, overrides []string) []string {
	result := append([]string(nil), base...)
	for _, override := range overrides {
		key, _, ok := strings.Cut(override, "=")
		if !ok {
			continue
		}
		prefix := key + "="
		filtered := result[:0]
		for _, entry := range result {
			if !strings.HasPrefix(strings.ToUpper(entry), strings.ToUpper(prefix)) {
				filtered = append(filtered, entry)
			}
		}
		result = append(filtered, override)
	}
	return result
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
