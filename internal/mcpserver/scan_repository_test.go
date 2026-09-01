package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/service"
	"github.com/endrilickollari/debtdrone-cli/v2/scanner"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func connectTestClient(t *testing.T, server *Server) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	connectCtx, stopConnecting := context.WithTimeout(context.Background(), 5*time.Second)
	session, err := client.Connect(connectCtx, clientTransport, nil)
	stopConnecting()
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = session.Close()
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("MCP server did not stop during cleanup")
		}
	})
	return session
}

func callScanRepository(t *testing.T, session *mcp.ClientSession, arguments map[string]any) (*mcp.CallToolResult, ScanRepositoryOutput) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: scanRepositoryToolName, Arguments: arguments})
	require.NoError(t, err)
	var output ScanRepositoryOutput
	if result.StructuredContent != nil {
		encoded, err := json.Marshal(result.StructuredContent)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(encoded, &output))
	}
	return result, output
}

func TestScanRepositoryToolAdvertisesExplicitReadOnlySchema(t *testing.T) {
	server := newServer(t.TempDir(), "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
		return scanner.Report{}, nil
	})
	session := connectTestClient(t, server)

	result, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.Tools, 1)
	tool := result.Tools[0]
	assert.Equal(t, scanRepositoryToolName, tool.Name)
	assert.Contains(t, tool.Description, "at most one scan at a time")
	require.NotNil(t, tool.Annotations)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	assert.True(t, tool.Annotations.IdempotentHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)

	schema, ok := tool.InputSchema.(map[string]any)
	require.True(t, ok, "unexpected input schema type %T", tool.InputSchema)
	assert.Equal(t, false, schema["additionalProperties"])
	properties, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"path", "max_complexity", "security_scan", "coverage", "max_findings"}, mapKeys(properties))
	securityScan := properties["security_scan"].(map[string]any)
	assert.Equal(t, "boolean", securityScan["type"])
	maxFindings := properties["max_findings"].(map[string]any)
	assert.Equal(t, float64(1), maxFindings["minimum"])
	assert.Equal(t, float64(maximumMaxFindings), maxFindings["maximum"])
}

func TestScanRepositoryToolRejectsUnsupportedAndInvalidInput(t *testing.T) {
	called := false
	server := newServer(t.TempDir(), "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
		called = true
		return scanner.Report{}, nil
	})
	session := connectTestClient(t, server)

	for _, arguments := range []map[string]any{
		{"unsupported": true},
		{"max_complexity": maximumMaxComplexity + 1},
		{"security_scan": nil},
		{"max_findings": maximumMaxFindings + 1},
	} {
		result, _ := callScanRepository(t, session, arguments)
		assert.True(t, result.IsError)
	}
	assert.False(t, called)
}

func TestScanRepositoryToolDelegatesToScannerAndOrdersOutput(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	require.NoError(t, os.Mkdir(repository, 0o700))
	lineOne, lineTwo := 1, 2
	var capturedPath string
	var capturedOptions scanner.Options
	server := newServer(root, "test", func(_ context.Context, path string, options scanner.Options) (scanner.Report, error) {
		capturedPath, capturedOptions = path, options
		return scanner.Report{
			Findings: []scanner.Finding{
				{Fingerprint: "b", AnalyzerID: "z", Message: "second", Location: scanner.Location{Path: "/z.go", Line: &lineTwo}},
				{Fingerprint: "a", AnalyzerID: "a", Message: "first", Location: scanner.Location{Path: "/a.go", Line: &lineOne}},
			},
			Metrics: map[string][]scanner.Metric{
				"z": {{Name: "second", Value: 2}},
				"a": {{Name: "first", Value: 1}},
			},
			Warnings: []scanner.Warning{{AnalyzerID: "z", Message: "second"}, {AnalyzerID: "a", Message: "first"}},
		}, nil
	})
	session := connectTestClient(t, server)

	result, output := callScanRepository(t, session, map[string]any{
		"path":           "repository",
		"max_complexity": 25,
		"security_scan":  true,
		"coverage":       true,
		"max_findings":   10,
	})

	assert.False(t, result.IsError)
	resolvedRepository, err := filepath.EvalSymlinks(repository)
	require.NoError(t, err)
	assert.Equal(t, resolvedRepository, capturedPath)
	assert.Equal(t, scanner.ScopeFull, capturedOptions.Scope.Mode)
	assert.Equal(t, 25, capturedOptions.Complexity.MaxCyclomatic)
	assert.True(t, capturedOptions.Security.Enabled)
	assert.True(t, capturedOptions.Coverage.Enabled)
	assert.False(t, capturedOptions.Coverage.RunLocalTests)
	assert.Nil(t, capturedOptions.Coverage.IsolatedExecutor)
	assert.Equal(t, scanRepositorySchemaVersion, output.SchemaVersion)
	assert.Equal(t, "complete", output.Status)
	assert.Equal(t, "repository", output.Repository)
	require.Len(t, output.Findings, 2)
	assert.Equal(t, "a", output.Findings[0].Fingerprint)
	assert.Equal(t, "b", output.Findings[1].Fingerprint)
	assert.Equal(t, []string{"a", "z"}, []string{output.Metrics[0].AnalyzerID, output.Metrics[1].AnalyzerID})
	assert.Equal(t, []string{"a", "z"}, []string{output.Warnings[0].AnalyzerID, output.Warnings[1].AnalyzerID})
	require.Len(t, result.Content, 1)
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "returned 2 of 2 findings")
}

func TestScanRepositoryToolUsesCLIDefaults(t *testing.T) {
	var capturedOptions scanner.Options
	server := newServer(t.TempDir(), "test", func(_ context.Context, _ string, options scanner.Options) (scanner.Report, error) {
		capturedOptions = options
		return scanner.Report{}, nil
	})

	result, output := callScanRepository(t, connectTestClient(t, server), nil)

	assert.False(t, result.IsError)
	assert.Equal(t, 15, capturedOptions.Complexity.MaxCyclomatic)
	assert.True(t, capturedOptions.Security.Enabled)
	assert.False(t, capturedOptions.Coverage.Enabled)
	assert.Equal(t, defaultMaxFindings, output.Limits.MaxFindings)
}

func TestScanRepositoryToolPreservesExplicitlyDisabledSecurityScan(t *testing.T) {
	var capturedOptions scanner.Options
	server := newServer(t.TempDir(), "test", func(_ context.Context, _ string, options scanner.Options) (scanner.Report, error) {
		capturedOptions = options
		return scanner.Report{}, nil
	})

	result, _ := callScanRepository(t, connectTestClient(t, server), map[string]any{"security_scan": false})

	assert.False(t, result.IsError)
	assert.False(t, capturedOptions.Security.Enabled)
}

func TestScanRepositoryToolMapsFatalAndPartialFailures(t *testing.T) {
	t.Run("fatal scanner error", func(t *testing.T) {
		server := newServer(t.TempDir(), "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
			return scanner.Report{}, errors.New("permission denied")
		})
		result, _ := callScanRepository(t, connectTestClient(t, server), nil)
		assert.True(t, result.IsError)
		require.Len(t, result.Content, 1)
		assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "could not scan")
		assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "permission denied")
	})

	t.Run("partial analyzer failure", func(t *testing.T) {
		failure := scanner.AnalyzerFailure{AnalyzerID: "trivy", AnalyzerName: "Security", Error: "tool unavailable"}
		server := newServer(t.TempDir(), "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
			return scanner.Report{Metrics: map[string][]scanner.Metric{}, Failures: []scanner.AnalyzerFailure{failure}}, &scanner.PartialFailureError{Failures: []scanner.AnalyzerFailure{failure}}
		})
		result, output := callScanRepository(t, connectTestClient(t, server), nil)
		assert.False(t, result.IsError)
		assert.Equal(t, "partial", output.Status)
		require.Len(t, output.Failures, 1)
		assert.Equal(t, "trivy", output.Failures[0].AnalyzerID)
		assert.Equal(t, "tool unavailable", output.Failures[0].Error)
	})
}

func TestScanRepositoryToolRejectsAbsoluteAndTraversalPaths(t *testing.T) {
	called := false
	root := t.TempDir()
	server := newServer(root, "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
		called = true
		return scanner.Report{}, nil
	})
	session := connectTestClient(t, server)

	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "absolute path", path: root, want: "must be relative"},
		{name: "parent traversal", path: "../outside", want: "outside the configured MCP root"},
		{name: "Windows absolute path", path: `C:\outside`, want: "must be relative"},
		{name: "Windows drive-relative path", path: `C:outside`, want: "must be relative"},
		{name: "UNC absolute path", path: `\\server\share`, want: "must be relative"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, _ := callScanRepository(t, session, map[string]any{"path": test.path})
			assert.True(t, result.IsError)
			assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, test.want)
		})
	}
	assert.False(t, called)
}

func TestScanRepositoryToolRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	escape := filepath.Join(root, "escape")
	if err := os.Symlink(t.TempDir(), escape); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	called := false
	server := newServer(root, "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
		called = true
		return scanner.Report{}, nil
	})

	result, _ := callScanRepository(t, connectTestClient(t, server), map[string]any{"path": "escape"})

	assert.True(t, result.IsError)
	assert.False(t, called)
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "resolves outside the configured MCP root")
}

func TestScanRepositoryToolAllowsSymlinkWithinRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "repository")
	require.NoError(t, os.Mkdir(target, 0o700))
	link := filepath.Join(root, "alias")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	var capturedPath string
	server := newServer(root, "test", func(_ context.Context, path string, _ scanner.Options) (scanner.Report, error) {
		capturedPath = path
		return scanner.Report{}, nil
	})

	result, output := callScanRepository(t, connectTestClient(t, server), map[string]any{"path": "alias"})

	assert.False(t, result.IsError)
	resolvedTarget, err := filepath.EvalSymlinks(target)
	require.NoError(t, err)
	assert.Equal(t, resolvedTarget, capturedPath)
	assert.Equal(t, "alias", output.Repository)
}

func TestScanRepositoryToolRejectsMissingRepository(t *testing.T) {
	called := false
	server := newServer(t.TempDir(), "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
		called = true
		return scanner.Report{}, nil
	})
	result, _ := callScanRepository(t, connectTestClient(t, server), map[string]any{"path": "missing"})
	assert.True(t, result.IsError)
	assert.False(t, called)
	assert.Contains(t, result.Content[0].(*mcp.TextContent).Text, "inspect repository path")
}

func TestScanRepositoryToolLimitsConcurrentScans(t *testing.T) {
	const requests = 4
	var active, maximum atomic.Int32
	started := make(chan struct{}, requests)
	release := make(chan struct{})
	server := newServer(t.TempDir(), "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
		current := active.Add(1)
		for observed := maximum.Load(); current > observed && !maximum.CompareAndSwap(observed, current); observed = maximum.Load() {
		}
		started <- struct{}{}
		<-release
		active.Add(-1)
		return scanner.Report{}, nil
	})

	errors := make(chan error, requests)
	for range requests {
		go func() {
			_, _, err := server.scanRepository(context.Background(), nil, ScanRepositoryInput{})
			errors <- err
		}()
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first scan did not start")
	}
	secondStarted := false
	select {
	case <-started:
		secondStarted = true
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	for range requests {
		require.NoError(t, <-errors)
	}

	assert.False(t, secondStarted, "more than one scan started before capacity was released")
	assert.Equal(t, int32(maximumConcurrentScans), maximum.Load())
}

func TestScanRepositoryToolCancelsWhileWaitingForCapacity(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server := newServer(t.TempDir(), "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return scanner.Report{}, nil
	})

	firstDone := make(chan error, 1)
	go func() {
		_, _, err := server.scanRepository(context.Background(), nil, ScanRepositoryInput{})
		firstDone <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first scan did not start")
	}

	waitingCtx, cancelWaiting := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		_, _, err := server.scanRepository(waitingCtx, nil, ScanRepositoryInput{})
		secondDone <- err
	}()
	cancelWaiting()

	select {
	case err := <-secondDone:
		assert.ErrorIs(t, err, context.Canceled)
		assert.Contains(t, err.Error(), "waiting for scan capacity")
	case <-time.After(time.Second):
		t.Fatal("waiting scan did not observe cancellation")
	}
	assert.Equal(t, int32(1), calls.Load())
	close(release)
	require.NoError(t, <-firstDone)
}

func TestScanRepositoryToolDoesNotStartForCancelledRequest(t *testing.T) {
	var called atomic.Bool
	server := newServer(t.TempDir(), "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
		called.Store(true)
		return scanner.Report{}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := server.scanRepository(ctx, nil, ScanRepositoryInput{})

	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, called.Load())
}

func TestScanRepositoryToolPropagatesProtocolCancellationToScanner(t *testing.T) {
	started := make(chan struct{})
	observed := make(chan error, 1)
	server := newServer(t.TempDir(), "test", func(ctx context.Context, _ string, _ scanner.Options) (scanner.Report, error) {
		close(started)
		<-ctx.Done()
		observed <- ctx.Err()
		return scanner.Report{}, ctx.Err()
	})
	session := connectTestClient(t, server)
	callCtx, cancelCall := context.WithCancel(context.Background())
	callDone := make(chan error, 1)
	go func() {
		_, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: scanRepositoryToolName})
		callDone <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("scanner did not start")
	}
	cancelCall()

	select {
	case err := <-observed:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("scanner did not observe protocol cancellation")
	}
	select {
	case err := <-callDone:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancelled protocol call did not return")
	}
}

func TestScanRepositoryToolPropagatesDeadlineToScanner(t *testing.T) {
	observed := make(chan error, 1)
	server := newServer(t.TempDir(), "test", func(ctx context.Context, _ string, _ scanner.Options) (scanner.Report, error) {
		<-ctx.Done()
		observed <- ctx.Err()
		return scanner.Report{}, ctx.Err()
	})
	callCtx, cancelCall := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelCall()

	_, _, err := server.scanRepository(callCtx, nil, ScanRepositoryInput{})

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "timed out while scanning")
	select {
	case scannerErr := <-observed:
		assert.ErrorIs(t, scannerErr, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("scanner did not observe request deadline")
	}
}

func TestScanRepositoryToolBoundsLargeResponses(t *testing.T) {
	largeSnippet := strings.Repeat("x", 200000)
	report := scanner.Report{Metrics: map[string][]scanner.Metric{}}
	for index := range 20 {
		report.Findings = append(report.Findings, scanner.Finding{
			Fingerprint: string(rune('a' + index)), AnalyzerID: "complexity", Message: "finding", CodeSnippet: &largeSnippet,
		})
	}
	server := newServer(t.TempDir(), "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
		return report, nil
	})

	result, output := callScanRepository(t, connectTestClient(t, server), map[string]any{"max_findings": 20})
	assert.False(t, result.IsError)
	assert.True(t, output.Truncated)
	assert.Equal(t, 20, output.Omitted.CodeSnippets)
	for _, finding := range output.Findings {
		assert.Nil(t, finding.CodeSnippet)
	}
	encoded, err := json.Marshal(output)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(encoded), maximumResponseBytes)
}

func TestScanRepositoryToolHonorsFindingLimit(t *testing.T) {
	report := scanner.Report{Metrics: map[string][]scanner.Metric{}}
	for index := range 10 {
		report.Findings = append(report.Findings, scanner.Finding{
			Fingerprint: string(rune('a' + index)), AnalyzerID: "complexity", Message: "finding",
		})
	}
	server := newServer(t.TempDir(), "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
		return report, nil
	})

	result, output := callScanRepository(t, connectTestClient(t, server), map[string]any{"max_findings": 3})
	assert.False(t, result.IsError)
	assert.Equal(t, 10, output.TotalFindings)
	assert.Equal(t, 3, output.ReturnedFindings)
	assert.Equal(t, 7, output.Omitted.Findings)
	assert.True(t, output.Truncated)
}

func TestScanRepositoryToolDropsLargestMetricBeforeMetricGroup(t *testing.T) {
	report := scanner.Report{Metrics: map[string][]scanner.Metric{
		"complexity": {
			{Name: "summary", Value: 12},
			{Name: "records", Value: strings.Repeat("x", maximumResponseBytes+1)},
		},
	}}
	server := newServer(t.TempDir(), "test", func(context.Context, string, scanner.Options) (scanner.Report, error) {
		return report, nil
	})

	result, output := callScanRepository(t, connectTestClient(t, server), nil)
	assert.False(t, result.IsError)
	require.Len(t, output.Metrics, 1)
	require.Len(t, output.Metrics[0].Metrics, 1)
	assert.Equal(t, "summary", output.Metrics[0].Metrics[0].Name)
	assert.Equal(t, 1, output.Omitted.Metrics)
	assert.Zero(t, output.Omitted.MetricGroups)
	assert.True(t, output.Truncated)
}

func TestScanRepositoryToolMatchesCLIResultForFixture(t *testing.T) {
	repository := t.TempDir()
	source := `package example

func classify(value int) int {
	if value > 0 {
		if value > 1 {
			if value > 2 {
				if value > 3 {
					return value
				}
			}
		}
	}
	return 0
}
`
	require.NoError(t, os.WriteFile(filepath.Join(repository, "complex.go"), []byte(source), 0o600))

	server := New(repository, "test")
	result, output := callScanRepository(t, connectTestClient(t, server), map[string]any{
		"max_complexity": 3,
		"security_scan":  false,
		"max_findings":   20,
	})
	require.False(t, result.IsError)

	cliResult, err := service.NewScanService().RunDetailed(context.Background(), repository, service.ScanOptions{MaxComplexity: 3}, nil)
	require.NoError(t, err)
	require.Equal(t, len(cliResult.Issues), len(output.Findings))
	for index, issue := range cliResult.Issues {
		finding := output.Findings[index]
		assert.Equal(t, issue.FingerprintHash, finding.Fingerprint)
		assert.Equal(t, issue.ToolName, finding.AnalyzerID)
		assert.Equal(t, issue.ToolRuleID, finding.RuleID)
		assert.Equal(t, issue.IssueType, finding.Type)
		assert.Equal(t, issue.Severity, finding.Severity)
		assert.Equal(t, issue.Message, finding.Message)
		assert.Equal(t, issue.FilePath, finding.Location.Path)
		assert.Equal(t, issue.LineNumber, finding.Location.Line)
	}
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
