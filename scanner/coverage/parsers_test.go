package coverage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/scanner/repostructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeSupportsEveryCoverageFormat(t *testing.T) {
	tests := []struct {
		name     string
		artifact string
		content  string
		format   string
	}{
		{name: "Go", artifact: "coverage.out", format: "go", content: "mode: atomic\nexample.com/project/pkg/a.go:1.1,1.5 1 1\nexample.com/project/pkg/a.go:2.1,2.5 1 0\n"},
		{name: "LCOV", artifact: "lcov.info", format: "lcov", content: "SF:src/a.ts\nDA:1,1\nDA:2,0\nBRF:2\nBRH:1\nend_of_record\n"},
		{name: "Cobertura", artifact: "coverage.xml", format: "cobertura", content: `<coverage><packages><package><classes><class filename="src/a.py" branch-rate="0.5"><lines><line number="1" hits="1"/><line number="2" hits="0"/></lines></class></classes></package></packages></coverage>`},
		{name: "JaCoCo", artifact: "jacoco.xml", format: "jacoco", content: `<report><package name="src"><sourcefile name="A.java"><line nr="1" ci="1"/><line nr="2" mi="1"/><counter type="BRANCH" missed="1" covered="1"/></sourcefile></package></report>`},
		{name: "Clover", artifact: "clover.xml", format: "clover", content: `<coverage><project><file name="src/a.php"><line num="1" type="stmt" count="1"/><line num="2" type="stmt" count="0"/></file></project></coverage>`},
		{name: "SimpleCov", artifact: ".resultset.json", format: "simplecov", content: `{"RSpec":{"coverage":{"src/a.rb":[1,0,null]}}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			result, err := Analyze(context.Background(), root, nil, Options{Artifacts: []Artifact{{Name: test.artifact, Content: []byte(test.content)}}})
			require.NoError(t, err)
			require.NotNil(t, result.Report)
			assert.Equal(t, test.format, result.Report.Format)
			require.Len(t, result.Report.Files, 1)
			assert.Equal(t, 2, result.Report.Files[0].LinesTotal)
			assert.Equal(t, 1, result.Report.Files[0].LinesCovered)
			assert.InDelta(t, 50, result.Report.OverallLinePct, 0.001)
		})
	}
}

func TestUploadedAndGeneratedArtifactsShareNormalization(t *testing.T) {
	root := t.TempDir()
	buildRoot := filepath.Join(root, "frontend")
	require.NoError(t, os.MkdirAll(filepath.Join(buildRoot, "coverage"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(buildRoot, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(buildRoot, "src", "app.ts"), []byte("export {}\n"), 0o600))
	content := []byte("SF:src/app.ts\nDA:1,1\nend_of_record\n")
	roots := []repostructure.BuildRoot{{Dir: buildRoot, Language: "TypeScript/JS", Tool: "npm", ManifestFile: "package.json"}}

	uploaded, err := Analyze(context.Background(), root, roots, Options{Artifacts: []Artifact{{Name: "lcov.info", Content: content}}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(buildRoot, "coverage", "lcov.info"), content, 0o600))
	generated, err := Analyze(context.Background(), root, roots, Options{})
	require.NoError(t, err)

	require.NotNil(t, uploaded.Report)
	require.NotNil(t, generated.Report)
	assert.Equal(t, "frontend/src/app.ts", generated.Report.Files[0].Path)
	assert.Equal(t, generated.Report, uploaded.Report)
}

func TestUploadedArtifactsUseExplicitRootsInMonorepositories(t *testing.T) {
	root := t.TempDir()
	frontend := filepath.Join(root, "frontend")
	backend := filepath.Join(root, "backend")
	for _, directory := range []string{filepath.Join(frontend, "src"), filepath.Join(backend, "src")} {
		require.NoError(t, os.MkdirAll(directory, 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(frontend, "src", "app.ts"), []byte("export {}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(backend, "src", "server.ts"), []byte("export {}\n"), 0o600))
	roots := []repostructure.BuildRoot{{Dir: frontend}, {Dir: backend}}

	result, err := Analyze(context.Background(), root, roots, Options{Artifacts: []Artifact{
		{Name: "lcov.info", Root: "frontend", Content: []byte("SF:src/app.ts\nDA:1,1\nend_of_record\n")},
		{Name: "lcov.info", Root: "backend", Content: []byte("SF:src/server.ts\nDA:1,1\nend_of_record\n")},
	}})
	require.NoError(t, err)
	require.NotNil(t, result.Report)
	assert.Equal(t, "combined", result.Report.Format)
	require.Len(t, result.Report.Files, 2)
	assert.Equal(t, "backend/src/server.ts", result.Report.Files[0].Path)
	assert.Equal(t, "frontend/src/app.ts", result.Report.Files[1].Path)
}

func TestUploadedArtifactRejectsUndetectedRoot(t *testing.T) {
	root := t.TempDir()
	result, err := Analyze(context.Background(), root, []repostructure.BuildRoot{{Dir: filepath.Join(root, "frontend")}}, Options{Artifacts: []Artifact{{
		Name: "lcov.info", Root: "../outside", Content: []byte("SF:src/app.ts\nDA:1,1\nend_of_record\n"),
	}}})
	require.NoError(t, err)
	assert.Nil(t, result.Report)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "outside the repository")
}

func TestUploadedArtifactRequiresRootWhenMonorepoNameIsAmbiguous(t *testing.T) {
	root := t.TempDir()
	roots := []repostructure.BuildRoot{{Dir: filepath.Join(root, "frontend")}, {Dir: filepath.Join(root, "backend")}}
	result, err := Analyze(context.Background(), root, roots, Options{Artifacts: []Artifact{{
		Name: "lcov.info", Content: []byte("SF:src/app.ts\nDA:1,1\nend_of_record\n"),
	}}})
	require.NoError(t, err)
	assert.Nil(t, result.Report)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "build root is ambiguous")
}

func TestGoCoverageUsesStatementCountsInsteadOfPhysicalRange(t *testing.T) {
	root := t.TempDir()
	result, err := Analyze(context.Background(), root, nil, Options{Artifacts: []Artifact{{
		Name: "coverage.out", Content: []byte("mode: atomic\nexample.com/project/pkg/a.go:10.1,100.1 1 0\n"),
	}}})
	require.NoError(t, err)
	require.NotNil(t, result.Report)
	require.Len(t, result.Report.Files, 1)
	assert.Equal(t, 1, result.Report.Files[0].LinesTotal)
	assert.Equal(t, []int{10}, result.Report.Files[0].UncoveredLines)
}

func TestCoberturaMergesOverlappingClassesBySourceLine(t *testing.T) {
	content := []byte(`<coverage><packages><package><classes>
<class filename="src/a.py"><lines><line number="1" hits="0"/></lines></class>
<class filename="src/a.py"><lines><line number="1" hits="1"/><line number="2" hits="0"/></lines></class>
</classes></package></packages></coverage>`)
	result, err := Analyze(context.Background(), t.TempDir(), nil, Options{Artifacts: []Artifact{{Name: "coverage.xml", Content: content}}})
	require.NoError(t, err)
	require.NotNil(t, result.Report)
	require.Len(t, result.Report.Files, 1)
	assert.Equal(t, 2, result.Report.Files[0].LinesTotal)
	assert.Equal(t, 1, result.Report.Files[0].LinesCovered)
	assert.Equal(t, []int{2}, result.Report.Files[0].UncoveredLines)
}

func TestAnalyzeRejectsOversizedArtifactWithoutCorruptingResult(t *testing.T) {
	root := t.TempDir()
	result, err := Analyze(context.Background(), root, nil, Options{Artifacts: []Artifact{{Name: "lcov.info", Content: make([]byte, maxArtifactSize+1)}}})
	require.NoError(t, err)
	assert.Nil(t, result.Report)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "exceeds")
}
