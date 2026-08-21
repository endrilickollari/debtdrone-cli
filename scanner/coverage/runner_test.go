package coverage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/scanner/repostructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCommandRunner struct {
	available    map[string]bool
	allAvailable bool
	name         string
	args         []string
	run          func(directory, name string, args []string) error
}

func (runner *fakeCommandRunner) LookPath(name string) (string, error) {
	if runner.allAvailable || runner.available[name] {
		return "/tools/" + name, nil
	}
	return "", errors.New("not found")
}

func (runner *fakeCommandRunner) Run(_ context.Context, directory string, name string, args ...string) error {
	runner.name = name
	runner.args = append([]string(nil), args...)
	if runner.run != nil {
		return runner.run(directory, name, args)
	}
	return nil
}

func TestLocalCoverageCommandsCoverSupportedRuntimes(t *testing.T) {
	tests := []struct {
		name     string
		root     repostructure.BuildRoot
		expected string
	}{
		{name: "Go", root: repostructure.BuildRoot{Language: "Go"}, expected: "go"},
		{name: "Node", root: repostructure.BuildRoot{Language: "TypeScript/JS", Tool: "pnpm", TestRunner: "vitest"}, expected: "vitest"},
		{name: "Python", root: repostructure.BuildRoot{Language: "Python"}, expected: "python3"},
		{name: "Rust", root: repostructure.BuildRoot{Language: "Rust"}, expected: "cargo"},
		{name: "Java Maven", root: repostructure.BuildRoot{Language: "Java", Tool: "maven"}, expected: "mvn"},
		{name: "Java Gradle", root: repostructure.BuildRoot{Language: "Java", Tool: "gradle"}, expected: "gradle"},
		{name: "Ruby", root: repostructure.BuildRoot{Language: "Ruby"}, expected: "bundle"},
		{name: "PHP", root: repostructure.BuildRoot{Language: "PHP"}, expected: "phpunit"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.root.Dir = t.TempDir()
			command, ok := localCoverageCommand(test.root)
			require.True(t, ok)
			assert.Equal(t, test.expected, command.name)
			assert.NotEmpty(t, command.args)
		})
	}
}

func TestNodeCoverageUsesSafeLocalSymlinkAndTerminatingLCOVArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires additional Windows privileges")
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "node_modules", ".bin")
	runnerDir := filepath.Join(root, "node_modules", "vitest")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.MkdirAll(runnerDir, 0o755))
	target := filepath.Join(runnerDir, "vitest.mjs")
	require.NoError(t, os.WriteFile(target, []byte("#!/usr/bin/env node\n"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join("..", "vitest", "vitest.mjs"), filepath.Join(binDir, "vitest")))

	command, ok := localCoverageCommand(repostructure.BuildRoot{Dir: root, Language: "TypeScript/JS", TestRunner: "vitest"})
	require.True(t, ok)
	resolvedTarget, err := filepath.EvalSymlinks(target)
	require.NoError(t, err)
	assert.Equal(t, resolvedTarget, command.name)
	assert.Equal(t, []string{"run", "--coverage", "--coverage.reporter=lcov", "--coverage.reportsDirectory=coverage"}, command.args)
}

func TestRepositoryExecutableRejectsSymlinkOutsideBuildRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires additional Windows privileges")
	}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "vitest")
	require.NoError(t, os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "node_modules", ".bin"), 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "node_modules", ".bin", "vitest")))

	_, ok := repositoryExecutable(root, filepath.Join("node_modules", ".bin", "vitest"))
	assert.False(t, ok)
}

func TestRubyCoverageUsesRakeForRakeProjects(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "Rakefile"), []byte("task :test\n"), 0o600))

	command, ok := localCoverageCommand(repostructure.BuildRoot{Dir: root, Language: "Ruby"})
	require.True(t, ok)
	assert.Equal(t, []string{"exec", "rake"}, command.args)
}

func TestLocalRunnersProduceDiscoverableCoverageForSupportedLanguages(t *testing.T) {
	tests := []struct {
		name     string
		root     repostructure.BuildRoot
		artifact string
		content  string
		format   string
	}{
		{name: "Go", root: repostructure.BuildRoot{Language: "Go"}, artifact: "coverage.out", content: "mode: atomic\nexample.com/project/main.go:1.1,1.5 1 1\n", format: "go"},
		{name: "Node", root: repostructure.BuildRoot{Language: "TypeScript/JS", TestRunner: "vitest"}, artifact: "coverage/lcov.info", content: "SF:src/app.ts\nDA:1,1\nend_of_record\n", format: "lcov"},
		{name: "Python", root: repostructure.BuildRoot{Language: "Python"}, artifact: "coverage.xml", content: `<coverage><packages><package><classes><class filename="app.py"><lines><line number="1" hits="1"/></lines></class></classes></package></packages></coverage>`, format: "cobertura"},
		{name: "Rust", root: repostructure.BuildRoot{Language: "Rust"}, artifact: "lcov.info", content: "SF:src/lib.rs\nDA:1,1\nend_of_record\n", format: "lcov"},
		{name: "Java", root: repostructure.BuildRoot{Language: "Java", Tool: "maven"}, artifact: "target/site/jacoco/jacoco.xml", content: `<report><package name="app"><sourcefile name="App.java"><line nr="1" ci="1"/></sourcefile></package></report>`, format: "jacoco"},
		{name: "Ruby", root: repostructure.BuildRoot{Language: "Ruby"}, artifact: "coverage/.resultset.json", content: `{"RSpec":{"coverage":{"app.rb":[1]}}}`, format: "simplecov"},
		{name: "PHP", root: repostructure.BuildRoot{Language: "PHP"}, artifact: "clover.xml", content: `<coverage><project><file name="app.php"><line num="1" type="stmt" count="1"/></file></project></coverage>`, format: "clover"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			test.root.Dir = root
			runner := &fakeCommandRunner{allAvailable: true}
			runner.run = func(directory, _ string, _ []string) error {
				path := filepath.Join(directory, filepath.FromSlash(test.artifact))
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				return os.WriteFile(path, []byte(test.content), 0o600)
			}

			result, err := analyzeWithRunner(context.Background(), root, []repostructure.BuildRoot{test.root}, Options{RunLocalTests: true}, runner)
			require.NoError(t, err)
			require.NotNil(t, result.Report)
			assert.Equal(t, test.format, result.Report.Format)
			assert.NotEmpty(t, runner.name)
		})
	}
}

func TestMissingRuntimeIsAWarningAndDoesNotRunCommand(t *testing.T) {
	runner := &fakeCommandRunner{available: map[string]bool{}}
	warnings := runLocalTests(context.Background(), []repostructure.BuildRoot{{Dir: "/repo", Language: "Go", ManifestFile: "go.mod"}}, runner)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "required tool(s) are unavailable: go")
	assert.Empty(t, runner.name)
}

func TestMissingRuntimeWarningPreservesOtherCoverageResults(t *testing.T) {
	repoRoot := t.TempDir()
	goRoot := filepath.Join(repoRoot, "backend")
	pythonRoot := filepath.Join(repoRoot, "worker")
	require.NoError(t, os.MkdirAll(goRoot, 0o755))
	require.NoError(t, os.MkdirAll(pythonRoot, 0o755))
	runner := &fakeCommandRunner{available: map[string]bool{"go": true}}
	runner.run = func(directory, _ string, _ []string) error {
		return os.WriteFile(filepath.Join(directory, "coverage.out"), []byte("mode: atomic\nexample.com/project/main.go:1.1,1.5 1 1\n"), 0o600)
	}

	result, err := analyzeWithRunner(context.Background(), repoRoot, []repostructure.BuildRoot{
		{Dir: goRoot, Language: "Go"},
		{Dir: pythonRoot, Language: "Python"},
	}, Options{RunLocalTests: true}, runner)
	require.NoError(t, err)
	require.NotNil(t, result.Report)
	assert.Equal(t, "go", result.Report.Format)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "python3")
}

func TestAvailableRuntimeRunsBoundedKnownCommand(t *testing.T) {
	runner := &fakeCommandRunner{available: map[string]bool{"go": true}}
	warnings := runLocalTests(context.Background(), []repostructure.BuildRoot{{Dir: "/repo", Language: "Go", ManifestFile: "go.mod"}}, runner)
	assert.Empty(t, warnings)
	assert.Equal(t, "go", runner.name)
	assert.Equal(t, []string{"test", "-coverprofile=coverage.out", "-covermode=atomic", "-timeout=90s", "-p=4", "./..."}, runner.args)
}
