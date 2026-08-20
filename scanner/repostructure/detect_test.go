package repostructure

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectMonorepoFixture(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("testdata", "monorepo"))
	require.NoError(t, err)

	structure := Detect(context.Background(), root)
	require.Empty(t, structure.Warnings)
	require.Len(t, structure.BuildRoots, 2)
	assert.True(t, structure.IsMonorepo)
	assert.Equal(t, filepath.Join(root, "backend"), structure.BuildRoots[0].Dir)
	assert.Equal(t, "golang:1.26-alpine", structure.BuildRoots[0].DockerImage)
	assert.Equal(t, filepath.Join(root, "frontend"), structure.BuildRoots[1].Dir)
	assert.Equal(t, "pnpm", structure.BuildRoots[1].Tool)
	assert.Equal(t, "vitest", structure.BuildRoots[1].TestRunner)
	assert.Equal(t, "node:22-alpine", structure.BuildRoots[1].DockerImage)
	assert.Contains(t, structure.SourceRoots, filepath.Join(root, "backend", "cmd"))
	assert.Contains(t, structure.SourceRoots, filepath.Join(root, "frontend", "src"))
	assert.Contains(t, structure.TestDirs, filepath.Join(root, "backend", "tests"))
	assert.Contains(t, structure.EntryPoints, filepath.Join(root, "backend", "cmd", "server", "main.go"))
	assert.Contains(t, structure.EntryPoints, filepath.Join(root, "frontend", "index.ts"))
	assert.Equal(t, []string{filepath.Join(root, "docs")}, structure.DocDirs)
}

func TestDetectSupportedManifests(t *testing.T) {
	tests := []struct {
		name       string
		manifest   string
		contents   string
		extraFile  string
		language   string
		tool       string
		testRunner string
	}{
		{"Go", "go.mod", "module example.com/test\n\ngo 1.25\n", "", "Go", "go", "go test"},
		{"Node", "package.json", `{}`, "", "TypeScript/JS", "npm", ""},
		{"Rust", "Cargo.toml", "[package]\nname='test'\n", "", "Rust", "cargo", "cargo test"},
		{"Maven", "pom.xml", "<project/>", "", "Java", "maven", "junit"},
		{"Gradle", "build.gradle", "", "", "Java", "gradle", "junit"},
		{"GradleKotlin", "build.gradle.kts", "", "", "Java", "gradle", "junit"},
		{"Pyproject", "pyproject.toml", "[tool.poetry]\n", "", "Python", "poetry", "pytest"},
		{"SetupPy", "setup.py", "", "", "Python", "pip", "pytest"},
		{"SetupCfg", "setup.cfg", "", "", "Python", "pip", "pytest"},
		{"Requirements", "requirements.txt", "", "", "Python", "pip", "pytest"},
		{"Ruby", "Gemfile", "", "", "Ruby", "bundler", ""},
		{"PHP", "composer.json", `{}`, "", "PHP", "composer", ""},
		{"Swift", "Package.swift", "// swift-tools-version: 6.0\n", "", "Swift", "swift-pm", "swift test"},
		{"CMake", "CMakeLists.txt", "", "", "C/C++", "cmake", ""},
		{"Make", "Makefile", "", "main.cpp", "C/C++", "make", ""},
		{"DotNet", "Example.csproj", "<Project/>", "", ".NET", "dotnet", "dotnet test"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, test.manifest), []byte(test.contents), 0o600))
			if test.extraFile != "" {
				require.NoError(t, os.WriteFile(filepath.Join(root, test.extraFile), []byte(""), 0o600))
			}

			structure := Detect(context.Background(), root)
			require.Len(t, structure.BuildRoots, 1)
			buildRoot := structure.BuildRoots[0]
			assert.Equal(t, test.language, buildRoot.Language)
			assert.Equal(t, test.tool, buildRoot.Tool)
			assert.Equal(t, test.testRunner, buildRoot.TestRunner)
		})
	}
}

func TestDetectRustBuildEnrichmentFixture(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("testdata", "rust-native"))
	require.NoError(t, err)

	structure := Detect(context.Background(), root)
	require.Empty(t, structure.Warnings)
	require.Len(t, structure.BuildRoots, 1)
	assert.Equal(t, "rust:1.85-slim", structure.BuildRoots[0].DockerImage)
	assert.Equal(t, []NativeDependency{
		{Name: "zig", Version: "0.14.0"},
		{Name: "clang"},
		{Name: "cmake"},
		{Name: "libclang-dev"},
		{Name: "pkg-config"},
		{Name: "protobuf-compiler"},
	}, structure.BuildRoots[0].NativeDeps)
	assert.Equal(t, []string{filepath.Join(root, "src", "main.rs")}, structure.EntryPoints)
}

func TestDetectRuntimeImages(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		files    map[string]string
		expected string
	}{
		{"Rust", "cargo", map[string]string{"rust-toolchain": "nightly\n"}, "rust:nightly-slim"},
		{"Node", "npm", map[string]string{".nvmrc": "v20.11.0\n"}, "node:20-alpine"},
		{"Go", "go", map[string]string{"go.mod": "module test\n\ngo 1.25\n"}, "golang:1.25-alpine"},
		{"Python", "pip", map[string]string{".python-version": "3.13.1\n"}, "python:3.13-slim"},
		{"Maven", "maven", map[string]string{".java-version": "23\n"}, "maven:3-eclipse-temurin-23"},
		{"Gradle", "gradle", map[string]string{".java-version": "21\n"}, "eclipse-temurin:21-jdk-alpine"},
		{"Ruby", "bundler", map[string]string{".ruby-version": "3.4.1\n"}, "ruby:3.4-slim"},
		{"PHP", "composer", map[string]string{"composer.json": `{"require":{"php":"^8.4"}}`}, "php:8.4-cli-alpine"},
		{"Swift", "swift-pm", map[string]string{"Package.swift": "// swift-tools-version: 6.1\n"}, "swift:6.1"},
		{"DotNet", "dotnet", map[string]string{"global.json": `{"sdk":{"version":"10.0.100"}}`}, "mcr.microsoft.com/dotnet/sdk:10.0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for name, contents := range test.files {
				require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600))
			}
			assert.Equal(t, test.expected, detectDockerImage(BuildRoot{Dir: root, Tool: test.tool}))
		})
	}
}

func TestDetectLanguageEntryPoints(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		path     string
		contents string
	}{
		{"Python", "pyproject.toml", "src/__main__.py", "print('hello')\n"},
		{"Java", "pom.xml", "src/main/java/App.java", "class App { public static void main(String[] args) {} }\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, test.manifest), []byte(""), 0o600))
			entryPoint := filepath.Join(root, test.path)
			require.NoError(t, os.MkdirAll(filepath.Dir(entryPoint), 0o700))
			require.NoError(t, os.WriteFile(entryPoint, []byte(test.contents), 0o600))

			structure := Detect(context.Background(), root)
			assert.Equal(t, []string{entryPoint}, structure.EntryPoints)
		})
	}
}

func TestDetectReturnsValidPartialReportForInvalidManifest(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"), []byte("{"), 0o600))

	structure := Detect(context.Background(), root)
	require.Len(t, structure.BuildRoots, 1)
	assert.Equal(t, "npm", structure.BuildRoots[0].Tool)
	require.Len(t, structure.Warnings, 1)
	assert.Contains(t, structure.Warnings[0], "package.json")
}

func TestDetectReturnsValidReportForMissingRootAndCancellation(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	structure := Detect(context.Background(), missing)
	assert.Equal(t, missing, structure.RepoRoot)
	assert.Empty(t, structure.BuildRoots)
	assert.NotEmpty(t, structure.Warnings)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	structure = Detect(ctx, t.TempDir())
	assert.Empty(t, structure.BuildRoots)
	assert.Contains(t, structure.Warnings[0], context.Canceled.Error())
}

func TestContextRoundTrip(t *testing.T) {
	structure := &Structure{RepoRoot: "/repository"}
	ctx := WithContext(context.Background(), structure)

	actual, ok := FromContext(ctx)
	assert.True(t, ok)
	assert.Same(t, structure, actual)
	_, ok = FromContext(context.Background())
	assert.False(t, ok)
}

func TestDetectDoesNotModifyRepository(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "source.go"), []byte("package source\n"), 0o600))
	before := snapshotRepository(t, root)

	Detect(context.Background(), root)

	assert.Equal(t, before, snapshotRepository(t, root))
}

func TestDetectBoundsEntryPointsAndSkipsGeneratedDirectories(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o600))
	for index := 0; index < 12; index++ {
		path := filepath.Join(root, "cmd", string(rune('a'+index)), "main.go")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0o600))
	}
	generated := filepath.Join(root, "VENDOR", "tool", "main.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(generated), 0o700))
	require.NoError(t, os.WriteFile(generated, []byte("package main\nfunc main() {}\n"), 0o600))

	structure := Detect(context.Background(), root)
	assert.Len(t, structure.EntryPoints, 10)
	for _, entryPoint := range structure.EntryPoints {
		assert.NotContains(t, entryPoint, "VENDOR")
	}
}

func TestDetectIgnoresSymlinkedMetadataAndEntryPoints(t *testing.T) {
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "package.json"), []byte(`{"devDependencies":{"vitest":"latest"}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600))

	manifestRoot := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(outside, "package.json"), filepath.Join(manifestRoot, "package.json")))
	assert.Empty(t, Detect(context.Background(), manifestRoot).BuildRoots)

	entryPointRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(entryPointRoot, "go.mod"), []byte("module example.com/test\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(entryPointRoot, "cmd"), 0o700))
	require.NoError(t, os.Symlink(filepath.Join(outside, "main.go"), filepath.Join(entryPointRoot, "cmd", "main.go")))
	structure := Detect(context.Background(), entryPointRoot)
	require.Len(t, structure.BuildRoots, 1)
	assert.Equal(t, "go", structure.BuildRoots[0].Tool)
	assert.Empty(t, structure.EntryPoints)
}

func TestDetectBoundsMetadataReads(t *testing.T) {
	root := t.TempDir()
	oversized := strings.Repeat("x", int(maxMetadataFileSize)+1)
	require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"), []byte(oversized), 0o600))

	structure := Detect(context.Background(), root)
	require.Len(t, structure.BuildRoots, 1)
	assert.Equal(t, "npm", structure.BuildRoots[0].Tool)
	require.Len(t, structure.Warnings, 1)
	assert.Contains(t, structure.Warnings[0], "metadata limit")
}

func TestDetectIgnoresSymlinkedRuntimeVersion(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "node-version")
	require.NoError(t, os.WriteFile(outside, []byte("99\n"), 0o600))
	root := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".node-version")))

	assert.Equal(t, "node:lts-alpine", detectNodeImage(root))
}

type fileSnapshot struct {
	Mode        os.FileMode
	Size        int64
	ModifiedAt  int64
	IsDirectory bool
}

func snapshotRepository(t *testing.T, root string) map[string]fileSnapshot {
	t.Helper()
	snapshot := make(map[string]fileSnapshot)
	require.NoError(t, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot[filepath.ToSlash(relative)] = fileSnapshot{
			Mode: info.Mode(), Size: info.Size(), ModifiedAt: info.ModTime().UnixNano(), IsDirectory: info.IsDir(),
		}
		return nil
	}))
	return snapshot
}
