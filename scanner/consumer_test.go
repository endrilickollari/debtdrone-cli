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

	dependencies := strings.Fields(string(output))
	for _, forbiddenModule := range []string{
		"github.com/endrilickollari/debtdrone",
		"github.com/endrilickollari/debtdrone-cli/v2/internal/store",
		"github.com/endrilickollari/debtdrone-cli/v2/internal/service",
		"github.com/endrilickollari/debtdrone-cli/v2/internal/tui",
		"github.com/lib/pq",
		"github.com/redis",
		"charm.land",
	} {
		for _, dependency := range dependencies {
			assert.False(t, packageBelongsToModule(dependency, forbiddenModule),
				"scanner dependency %q belongs to forbidden module %q", dependency, forbiddenModule)
		}
	}
}

func TestPackageBelongsToModuleDistinguishesSaaSFromCLI(t *testing.T) {
	tests := []struct {
		name       string
		dependency string
		module     string
		want       bool
	}{
		{
			name:       "module root",
			dependency: "github.com/endrilickollari/debtdrone",
			module:     "github.com/endrilickollari/debtdrone",
			want:       true,
		},
		{
			name:       "module package",
			dependency: "github.com/endrilickollari/debtdrone/backend/internal/analysis",
			module:     "github.com/endrilickollari/debtdrone",
			want:       true,
		},
		{
			name:       "similarly named CLI module",
			dependency: "github.com/endrilickollari/debtdrone-cli/v2/scanner",
			module:     "github.com/endrilickollari/debtdrone",
			want:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, packageBelongsToModule(test.dependency, test.module))
		})
	}
}

func packageBelongsToModule(packagePath, modulePath string) bool {
	return packagePath == modulePath || strings.HasPrefix(packagePath, modulePath+"/")
}
