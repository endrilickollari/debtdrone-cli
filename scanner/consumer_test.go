package scanner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicPackageCompilesForExternalConsumer(t *testing.T) {
	repositoryRoot, err := filepath.Abs("..")
	require.NoError(t, err)
	consumerRoot := t.TempDir()

	goMod := "module scannerconsumer\n\ngo 1.25.1\n\nrequire github.com/endrilickollari/debtdrone-cli/v2 v2.0.0\n\nreplace github.com/endrilickollari/debtdrone-cli/v2 => " + repositoryRoot + "\n"
	testSource := `package scannerconsumer

import (
	"context"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/scanner"
	"github.com/endrilickollari/debtdrone-cli/v2/scanner/repostructure"
)

func TestConsumer(t *testing.T) {
	_, _ = scanner.Scan(context.Background(), ".", scanner.DefaultOptions())
	_ = repostructure.Detect(context.Background(), ".")
}
`
	require.NoError(t, os.WriteFile(filepath.Join(consumerRoot, "go.mod"), []byte(goMod), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(consumerRoot, "consumer_test.go"), []byte(testSource), 0o600))

	command := exec.Command("go", "test", "-mod=mod", "./...")
	command.Dir = consumerRoot
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestPublicScannerDependencyBoundary(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "github.com/endrilickollari/debtdrone-cli/v2/scanner")
	command.Dir = ".."
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))

	dependencies := string(output)
	for _, forbidden := range []string{
		"github.com/endrilickollari/debtdrone-cli/v2/internal/store",
		"github.com/endrilickollari/debtdrone-cli/v2/internal/service",
		"github.com/endrilickollari/debtdrone-cli/v2/internal/tui",
		"github.com/lib/pq",
		"github.com/redis",
		"charm.land",
	} {
		assert.False(t, strings.Contains(dependencies, forbidden), "scanner depends on forbidden package %q", forbidden)
	}
}
