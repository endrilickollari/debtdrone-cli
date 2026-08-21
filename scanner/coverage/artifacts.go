package coverage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/v2/scanner/repostructure"
)

const maxArtifactSize int64 = 20 << 20

var knownRelativePaths = []string{
	"coverage.out",
	"coverage/lcov.info",
	"lcov.info",
	"coverage.info",
	"coverage.xml",
	"cobertura.xml",
	"tarpaulin-report.xml",
	"coverage.cobertura.xml",
	"TestResults/coverage.cobertura.xml",
	"target/site/jacoco/jacoco.xml",
	"build/reports/jacoco/test/jacocoTestReport.xml",
	"build/reports/jacoco/jacoco.xml",
	"coverage/clover.xml",
	"clover.xml",
	"coverage/.resultset.json",
	"coverage/coverage.json",
}

type candidate struct {
	name    string
	content []byte
	root    string
	source  string
}

// Analyze parses repository or supplied artifacts and optionally invokes local
// test runners when no artifact is available.
func Analyze(ctx context.Context, repoRoot string, roots []repostructure.BuildRoot, options Options) (Result, error) {
	return analyzeWithRunner(ctx, repoRoot, roots, options, defaultCommandRunner())
}

func analyzeWithRunner(ctx context.Context, repoRoot string, roots []repostructure.BuildRoot, options Options, runner commandRunner) (Result, error) {
	parsers := defaultParsers()
	if len(roots) == 0 {
		roots = []repostructure.BuildRoot{{Dir: repoRoot}}
	}
	candidates, warnings := discoverCandidates(ctx, roots, parsers)
	if len(candidates) == 0 {
		for _, artifact := range options.Artifacts {
			if len(artifact.Content) > int(maxArtifactSize) {
				warnings = append(warnings, fmt.Sprintf("coverage artifact %q exceeds the %d-byte limit", artifact.Name, maxArtifactSize))
				continue
			}
			if parserFor(artifact.Name, parsers) == nil {
				warnings = append(warnings, fmt.Sprintf("coverage artifact %q has an unsupported format", artifact.Name))
				continue
			}
			artifactRoot, err := artifactBuildRoot(repoRoot, roots, artifact)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("coverage artifact %q: %v", artifact.Name, err))
				continue
			}
			candidates = append(candidates, candidate{
				name:    filepath.Base(artifact.Name),
				content: append([]byte(nil), artifact.Content...),
				root:    artifactRoot,
				source:  artifact.Name,
			})
		}
	}
	if len(candidates) == 0 && options.RunLocalTests {
		runnerWarnings := runLocalTests(ctx, roots, runner)
		warnings = append(warnings, runnerWarnings...)
		if err := ctx.Err(); err != nil {
			return Result{Warnings: uniqueSorted(warnings)}, err
		}
		candidates, runnerWarnings = discoverCandidates(ctx, roots, parsers)
		warnings = append(warnings, runnerWarnings...)
	}

	var reports []*Report
	for _, item := range candidates {
		if err := ctx.Err(); err != nil {
			warnings = append(warnings, fmt.Sprintf("coverage parsing stopped: %v", err))
			return Result{Warnings: uniqueSorted(warnings)}, err
		}
		parser := parserFor(item.name, parsers)
		if parser == nil {
			continue
		}
		report, err := parseContent(parser, item.name, item.content)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("parse coverage artifact %s: %v", item.source, err))
			continue
		}
		normalizeReport(report, repoRoot, item.root)
		reports = append(reports, report)
	}
	if len(reports) == 0 {
		if len(warnings) == 0 {
			warnings = append(warnings, "no supported coverage artifact was found")
		}
		return Result{Warnings: uniqueSorted(warnings)}, nil
	}
	return Result{Report: mergeReports(reports), Warnings: uniqueSorted(warnings)}, nil
}

func artifactBuildRoot(repoRoot string, roots []repostructure.BuildRoot, artifact Artifact) (string, error) {
	if artifact.Root != "" {
		requested, err := cleanArtifactRoot(artifact.Root)
		if err != nil {
			return "", err
		}
		for _, root := range roots {
			relative, relErr := filepath.Rel(repoRoot, root.Dir)
			if relErr == nil && filepath.Clean(relative) == requested {
				return root.Dir, nil
			}
		}
		return "", fmt.Errorf("build root %q was not detected in the repository", artifact.Root)
	}
	if len(roots) == 1 {
		return roots[0].Dir, nil
	}
	cleanName := filepath.ToSlash(filepath.Clean(filepath.FromSlash(artifact.Name)))
	for _, root := range roots {
		relative, err := filepath.Rel(repoRoot, root.Dir)
		if err != nil || !isRelativePath(relative) {
			continue
		}
		prefix := strings.TrimSuffix(filepath.ToSlash(relative), "/") + "/"
		if strings.HasPrefix(cleanName, prefix) {
			return root.Dir, nil
		}
	}
	return "", fmt.Errorf("build root is ambiguous; set Artifact.Root for monorepository uploads")
}

func cleanArtifactRoot(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return "", fmt.Errorf("build root is empty")
	}
	if filepath.IsAbs(filepath.FromSlash(value)) || isWindowsAbsolutePath(value) {
		return "", fmt.Errorf("build root must be repository-relative")
	}
	cleaned := filepath.Clean(filepath.FromSlash(value))
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("build root is outside the repository")
	}
	return cleaned, nil
}

func isWindowsAbsolutePath(value string) bool {
	return len(value) >= 3 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':' && value[2] == '/'
}

func discoverCandidates(ctx context.Context, roots []repostructure.BuildRoot, parsers []Parser) ([]candidate, []string) {
	var candidates []candidate
	var warnings []string
	seen := make(map[string]struct{})
	for _, root := range roots {
		for _, relative := range knownRelativePaths {
			if err := ctx.Err(); err != nil {
				return candidates, append(warnings, fmt.Sprintf("coverage discovery stopped: %v", err))
			}
			path := filepath.Join(root.Dir, filepath.FromSlash(relative))
			if _, ok := seen[path]; ok || parserFor(path, parsers) == nil {
				continue
			}
			data, err := readArtifact(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("inspect coverage artifact %s: %v", path, err))
				continue
			}
			seen[path] = struct{}{}
			candidates = append(candidates, candidate{name: filepath.Base(path), content: data, root: root.Dir, source: path})
		}
	}
	return candidates, warnings
}

func readArtifact(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	if info.Size() > maxArtifactSize {
		return nil, fmt.Errorf("file exceeds the %d-byte limit", maxArtifactSize)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("file changed while being inspected")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxArtifactSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxArtifactSize {
		return nil, fmt.Errorf("file exceeds the %d-byte limit", maxArtifactSize)
	}
	return data, nil
}

func parserFor(name string, parsers []Parser) Parser {
	for _, parser := range parsers {
		if parser.CanParse(name) {
			return parser
		}
	}
	return nil
}

func parseContent(parser Parser, name string, content []byte) (*Report, error) {
	directory, err := os.MkdirTemp("", "debtdrone-coverage-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, filepath.Base(name))
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return nil, err
	}
	return parser.Parse(path)
}

func normalizeReport(report *Report, repoRoot, artifactRoot string) {
	for index := range report.Files {
		report.Files[index].Path = normalizePath(repoRoot, artifactRoot, report.Files[index].Path)
		sort.Ints(report.Files[index].UncoveredLines)
	}
	sort.Slice(report.Files, func(i, j int) bool { return report.Files[i].Path < report.Files[j].Path })
	report.OverallLinePct, report.OverallBranchPct = computeOverall(report.Files)
}

func normalizePath(repoRoot, artifactRoot, value string) string {
	value = filepath.Clean(filepath.FromSlash(strings.TrimSpace(value)))
	if filepath.IsAbs(value) {
		if relative, err := filepath.Rel(repoRoot, value); err == nil && isRelativePath(relative) {
			return filepath.ToSlash(relative)
		}
		value = strings.TrimLeft(filepath.ToSlash(value), "/")
		value = filepath.FromSlash(value)
	}
	if !isRelativePath(value) {
		return filepath.ToSlash(filepath.Base(value))
	}
	if artifactRoot != "" {
		candidate := filepath.Join(artifactRoot, value)
		if info, err := os.Lstat(candidate); err == nil && info.Mode().IsRegular() {
			if relative, err := filepath.Rel(repoRoot, candidate); err == nil && isRelativePath(relative) {
				return filepath.ToSlash(relative)
			}
		}
	}
	return filepath.ToSlash(value)
}

func isRelativePath(value string) bool {
	return value != "." && value != ".." && !filepath.IsAbs(value) && !strings.HasPrefix(value, ".."+string(filepath.Separator))
}

func mergeReports(reports []*Report) *Report {
	if len(reports) == 1 {
		return reports[0]
	}
	filesByPath := make(map[string]FileCoverage)
	for _, report := range reports {
		for _, file := range report.Files {
			filesByPath[file.Path] = file
		}
	}
	paths := make([]string, 0, len(filesByPath))
	for path := range filesByPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	merged := &Report{Files: make([]FileCoverage, 0, len(paths)), Format: "combined"}
	for _, path := range paths {
		merged.Files = append(merged.Files, filesByPath[path])
	}
	merged.OverallLinePct, merged.OverallBranchPct = computeOverall(merged.Files)
	return merged
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
