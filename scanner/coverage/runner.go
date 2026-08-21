package coverage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/endrilickollari/debtdrone-cli/v2/scanner/repostructure"
)

const localTestTimeout = 10 * time.Minute

type commandRunner interface {
	LookPath(name string) (string, error)
	Run(ctx context.Context, directory, name string, args ...string) error
}

type osCommandRunner struct{}

func defaultCommandRunner() commandRunner { return osCommandRunner{} }

func (osCommandRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (osCommandRunner) Run(ctx context.Context, directory, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = directory
	return command.Run()
}

type localCommand struct {
	required []string
	name     string
	args     []string
}

func runLocalTests(ctx context.Context, roots []repostructure.BuildRoot, runner commandRunner) []string {
	var warnings []string
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return append(warnings, fmt.Sprintf("local coverage stopped: %v", err))
		}
		command, ok := localCoverageCommand(root)
		if !ok {
			continue
		}
		missing := missingTools(command.required, runner)
		if len(missing) > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"coverage for %s was skipped because required tool(s) are unavailable: %s",
				displayRoot(root), strings.Join(missing, ", "),
			))
			continue
		}

		runCtx, cancel := context.WithTimeout(ctx, localTestTimeout)
		err := runner.Run(runCtx, root.Dir, command.name, command.args...)
		timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
		cancel()
		if err == nil {
			continue
		}
		if timedOut {
			warnings = append(warnings, fmt.Sprintf("coverage tests for %s exceeded %s", displayRoot(root), localTestTimeout))
			continue
		}
		if ctx.Err() != nil {
			return append(warnings, fmt.Sprintf("local coverage stopped: %v", ctx.Err()))
		}
		warnings = append(warnings, fmt.Sprintf("coverage tests for %s failed: %v", displayRoot(root), err))
	}
	return warnings
}

func localCoverageCommand(root repostructure.BuildRoot) (localCommand, bool) {
	switch strings.ToLower(root.Language) {
	case "go":
		return localCommand{required: []string{"go"}, name: "go", args: []string{"test", "-coverprofile=coverage.out", "-covermode=atomic", "-timeout=90s", "-p=4", "./..."}}, true
	case "typescript/js", "javascript", "typescript":
		testRunner := strings.ToLower(root.TestRunner)
		if testRunner != "jest" && testRunner != "vitest" {
			return localCommand{}, false
		}
		args := nodeCoverageArgs(testRunner)
		if localRunner, ok := repositoryExecutable(root.Dir, filepath.Join("node_modules", ".bin", testRunner)); ok {
			return localCommand{name: localRunner, args: args}, true
		}
		return localCommand{required: []string{testRunner}, name: testRunner, args: args}, true
	case "python":
		return localCommand{required: []string{"python3"}, name: "python3", args: []string{"-m", "pytest", "--cov=.", "--cov-report=xml:coverage.xml", "-q", "--tb=no", "--no-header"}}, true
	case "rust":
		return localCommand{required: []string{"cargo", "cargo-llvm-cov"}, name: "cargo", args: []string{"llvm-cov", "--lcov", "--output-path", "lcov.info"}}, true
	case "java", "kotlin":
		if root.Tool == "maven" {
			return localCommand{required: []string{"mvn"}, name: "mvn", args: []string{"test", "jacoco:report"}}, true
		}
		if root.Tool == "gradle" {
			wrapper := filepath.Join(root.Dir, "gradlew")
			if regularExecutable(wrapper) {
				return localCommand{name: wrapper, args: []string{"test", "jacocoTestReport"}}, true
			}
			return localCommand{required: []string{"gradle"}, name: "gradle", args: []string{"test", "jacocoTestReport"}}, true
		}
	case "ruby":
		testRunner := "rspec"
		if regularFile(filepath.Join(root.Dir, "Rakefile")) {
			testRunner = "rake"
		}
		return localCommand{required: []string{"bundle"}, name: "bundle", args: []string{"exec", testRunner}}, true
	case "php":
		phpunit := filepath.Join(root.Dir, "vendor", "bin", "phpunit")
		if !regularExecutable(phpunit) {
			return localCommand{required: []string{"phpunit"}, name: "phpunit", args: []string{"--coverage-clover", "clover.xml"}}, true
		}
		return localCommand{name: phpunit, args: []string{"--coverage-clover", "clover.xml"}}, true
	}
	return localCommand{}, false
}

func nodeCoverageArgs(testRunner string) []string {
	if testRunner == "vitest" {
		return []string{"run", "--coverage", "--coverage.reporter=lcov", "--coverage.reportsDirectory=coverage"}
	}
	return []string{"--coverage", "--coverageReporters=lcov", "--coverageDirectory=coverage", "--passWithNoTests", "--forceExit"}
}

func missingTools(names []string, runner commandRunner) []string {
	var missing []string
	for _, name := range names {
		if _, err := runner.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	return missing
}

func regularExecutable(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func repositoryExecutable(root, relative string) (string, bool) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	path := filepath.Join(root, relative)
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", false
	}
	relativeToRoot, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
		return "", false
	}
	if !regularExecutable(resolved) {
		return "", false
	}
	return resolved, true
}

func regularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func displayRoot(root repostructure.BuildRoot) string {
	if root.ManifestFile != "" {
		return filepath.Join(root.Dir, root.ManifestFile)
	}
	return root.Dir
}
