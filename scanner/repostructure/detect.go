package repostructure

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Detect performs side-effect-free filesystem inspection and always returns a
// valid structure. Recoverable errors are recorded as warnings on the result.
func Detect(ctx context.Context, repoPath string) *Structure {
	absoluteRoot, err := filepath.Abs(repoPath)
	if err != nil {
		absoluteRoot = filepath.Clean(repoPath)
	}
	structure := &Structure{RepoRoot: absoluteRoot}
	if err != nil {
		structure.warn("resolve repository root: %v", err)
		return structure
	}
	if err := ctx.Err(); err != nil {
		structure.warn("repository structure detection stopped: %v", err)
		return structure
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		structure.warn("inspect repository root: %v", err)
		return structure
	}
	if !info.IsDir() {
		structure.warn("repository root is not a directory")
		return structure
	}

	structure.BuildRoots = detectBuildRoots(ctx, structure, absoluteRoot)
	enrichBuildRoots(ctx, structure, structure.BuildRoots)
	structure.IsMonorepo = len(structure.BuildRoots) > 1
	structure.DocDirs = detectDocDirs(absoluteRoot)
	for _, root := range structure.BuildRoots {
		if err := ctx.Err(); err != nil {
			structure.warn("repository structure detection stopped: %v", err)
			break
		}
		structure.SourceRoots = append(structure.SourceRoots, detectSourceRoots(root)...)
		structure.TestDirs = append(structure.TestDirs, detectTestDirs(structure, root)...)
		structure.EntryPoints = append(structure.EntryPoints, detectEntryPoints(ctx, structure, root)...)
	}
	structure.normalize()
	return structure
}

func (structure *Structure) warn(format string, args ...any) {
	structure.Warnings = append(structure.Warnings, fmt.Sprintf(format, args...))
}

func (structure *Structure) normalize() {
	structure.BuildRoots = uniqueBuildRoots(structure.BuildRoots)
	structure.SourceRoots = uniqueStrings(structure.SourceRoots)
	structure.TestDirs = uniqueStrings(structure.TestDirs)
	structure.EntryPoints = uniqueStrings(structure.EntryPoints)
	structure.DocDirs = uniqueStrings(structure.DocDirs)
	structure.Warnings = uniqueStrings(structure.Warnings)
	structure.IsMonorepo = len(structure.BuildRoots) > 1
}

func uniqueBuildRoots(values []BuildRoot) []BuildRoot {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]BuildRoot, 0, len(values))
	for _, value := range values {
		key := filepath.Clean(value.Dir)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	sort.SliceStable(result, func(i, j int) bool {
		leftDepth := pathDepth(result[i].Dir)
		rightDepth := pathDepth(result[j].Dir)
		if leftDepth == rightDepth {
			return filepath.ToSlash(result[i].Dir) < filepath.ToSlash(result[j].Dir)
		}
		return leftDepth < rightDepth
	})
	return result
}

func pathDepth(value string) int {
	cleaned := filepath.Clean(value)
	return strings.Count(filepath.ToSlash(cleaned), "/")
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func detectBuildRoots(ctx context.Context, structure *Structure, repoPath string) []BuildRoot {
	directories := []string{repoPath}
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		structure.warn("list repository root: %v", err)
		return nil
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			structure.warn("build-root detection stopped: %v", err)
			break
		}
		if entry.IsDir() && !isSkipDir(entry.Name()) {
			directories = append(directories, filepath.Join(repoPath, entry.Name()))
		}
	}

	var roots []BuildRoot
	for _, directory := range directories {
		if err := ctx.Err(); err != nil {
			structure.warn("build-root detection stopped: %v", err)
			break
		}
		if root, ok := detectBuildRootInDir(structure, directory); ok {
			roots = append(roots, root)
			continue
		}
		if root, ok := detectDotnetRootInDir(structure, directory); ok {
			roots = append(roots, root)
		}
	}
	return roots
}

func detectDotnetRootInDir(structure *Structure, directory string) (BuildRoot, bool) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		structure.warn("inspect build-root candidate %s: %v", directory, err)
		return BuildRoot{}, false
	}
	for _, entry := range entries {
		if entry.IsDir() || !regularFileExists(filepath.Join(directory, entry.Name())) {
			continue
		}
		name := entry.Name()
		if strings.EqualFold(filepath.Ext(name), ".sln") || strings.EqualFold(filepath.Ext(name), ".csproj") {
			return BuildRoot{Dir: directory, Language: ".NET", Tool: "dotnet", ManifestFile: name, TestRunner: "dotnet test"}, true
		}
	}
	return BuildRoot{}, false
}

func detectBuildRootInDir(structure *Structure, directory string) (BuildRoot, bool) {
	manifests := []struct {
		file     string
		language string
		tool     string
	}{
		{"go.mod", "Go", "go"},
		{"package.json", "TypeScript/JS", ""},
		{"Cargo.toml", "Rust", "cargo"},
		{"pom.xml", "Java", "maven"},
		{"build.gradle", "Java", "gradle"},
		{"build.gradle.kts", "Java", "gradle"},
		{"pyproject.toml", "Python", ""},
		{"setup.py", "Python", "pip"},
		{"setup.cfg", "Python", "pip"},
		{"requirements.txt", "Python", "pip"},
		{"Gemfile", "Ruby", "bundler"},
		{"composer.json", "PHP", "composer"},
		{"Package.swift", "Swift", "swift-pm"},
		{"CMakeLists.txt", "C/C++", "cmake"},
	}

	for _, manifest := range manifests {
		if !regularFileExists(filepath.Join(directory, manifest.file)) {
			continue
		}
		root := BuildRoot{Dir: directory, Language: manifest.language, Tool: manifest.tool, ManifestFile: manifest.file}
		switch manifest.file {
		case "package.json":
			if err := detectJSProperties(&root); err != nil {
				structure.warn("inspect %s: %v", filepath.Join(directory, manifest.file), err)
			}
		case "pyproject.toml":
			if err := detectPythonProperties(&root); err != nil {
				structure.warn("inspect %s: %v", filepath.Join(directory, manifest.file), err)
			}
		}
		if root.TestRunner == "" {
			switch root.Tool {
			case "go":
				root.TestRunner = "go test"
			case "cargo":
				root.TestRunner = "cargo test"
			case "maven", "gradle":
				root.TestRunner = "junit"
			case "pip", "poetry":
				root.TestRunner = "pytest"
			case "swift-pm":
				root.TestRunner = "swift test"
			}
		}
		return root, true
	}

	if regularFileExists(filepath.Join(directory, "Makefile")) && hasCppSources(directory) {
		return BuildRoot{Dir: directory, Language: "C/C++", Tool: "make", ManifestFile: "Makefile"}, true
	}
	return BuildRoot{}, false
}

func hasCppSources(directory string) bool {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".c", ".cpp", ".cc", ".cxx", ".h", ".hpp", ".hxx":
			return true
		}
	}
	return false
}

func detectJSProperties(root *BuildRoot) error {
	data, err := readMetadataFile(filepath.Join(root.Dir, "package.json"))
	if err != nil {
		root.Tool = "npm"
		return err
	}
	var manifest struct {
		Dependencies    map[string]any `json:"dependencies"`
		DevDependencies map[string]any `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		root.Tool = "npm"
		return err
	}
	dependencies := make(map[string]any, len(manifest.Dependencies)+len(manifest.DevDependencies))
	for name, value := range manifest.Dependencies {
		dependencies[name] = value
	}
	for name, value := range manifest.DevDependencies {
		dependencies[name] = value
	}
	for _, runner := range []string{"jest", "vitest", "mocha", "jasmine"} {
		if _, ok := dependencies[runner]; ok {
			root.TestRunner = runner
			break
		}
	}
	if regularFileExists(filepath.Join(root.Dir, "yarn.lock")) {
		root.Tool = "yarn"
	} else if regularFileExists(filepath.Join(root.Dir, "pnpm-lock.yaml")) {
		root.Tool = "pnpm"
	} else {
		root.Tool = "npm"
	}
	return nil
}

func detectPythonProperties(root *BuildRoot) error {
	root.Tool = "pip"
	root.TestRunner = "pytest"
	data, err := readMetadataFile(filepath.Join(root.Dir, "pyproject.toml"))
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "[tool.poetry]") {
		root.Tool = "poetry"
	}
	return nil
}

func detectDocDirs(repoPath string) []string {
	var directories []string
	for _, candidate := range []string{"docs", "doc", "documentation", "wiki", "pages", ".github/wiki"} {
		path := filepath.Join(repoPath, candidate)
		if directoryExists(path) {
			directories = append(directories, path)
		}
	}
	return directories
}

func detectSourceRoots(root BuildRoot) []string {
	candidates := []string{"src", "lib", "pkg", "internal", "cmd", "app", "core", "source", "main"}
	if root.Language == "Java" || root.Language == "Kotlin" {
		candidates = append(candidates, "src/main/java", "src/main/kotlin")
	}
	var sources []string
	for _, candidate := range candidates {
		path := filepath.Join(root.Dir, candidate)
		if directoryExists(path) {
			sources = append(sources, path)
		}
	}
	if len(sources) == 0 {
		return []string{root.Dir}
	}
	return sources
}

func detectTestDirs(structure *Structure, root BuildRoot) []string {
	var tests []string
	for _, candidate := range []string{"tests", "__tests__", "test", "spec", "testdata", "e2e", "integration"} {
		path := filepath.Join(root.Dir, candidate)
		if directoryExists(path) {
			tests = append(tests, path)
		}
	}
	if root.Language != "Go" {
		return tests
	}
	entries, err := os.ReadDir(root.Dir)
	if err != nil {
		structure.warn("inspect Go test directories in %s: %v", root.Dir, err)
		return tests
	}
	for _, entry := range entries {
		if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && !isSkipDir(entry.Name()) {
			path := filepath.Join(root.Dir, entry.Name())
			if isGoTestDir(path) {
				tests = append(tests, path)
			}
		}
	}
	return tests
}

func isGoTestDir(directory string) bool {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	var goFiles, testFiles int
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		goFiles++
		if strings.HasSuffix(entry.Name(), "_test.go") {
			testFiles++
		}
	}
	return goFiles > 0 && float64(testFiles)/float64(goFiles) > 0.3
}

func detectEntryPoints(ctx context.Context, structure *Structure, root BuildRoot) []string {
	var entries []string
	err := filepath.WalkDir(root.Dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			structure.warn("inspect entry point %s: %v", path, walkErr)
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(entries) >= 10 {
			return fs.SkipAll
		}
		if entry.IsDir() {
			if path != root.Dir && isSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if isEntryPoint(root, path, entry.Name()) {
			entries = append(entries, path)
		}
		return nil
	})
	if err != nil {
		structure.warn("entry-point detection stopped: %v", err)
	}
	return entries
}

func isEntryPoint(root BuildRoot, path, name string) bool {
	switch root.Language {
	case "Go":
		if name != "main.go" {
			return false
		}
		content, err := readMetadataFile(path)
		return err == nil && strings.Contains(string(content), "func main()")
	case "Python":
		return name == "__main__.py"
	case "TypeScript/JS":
		if name != "index.ts" && name != "index.js" && name != "main.ts" && name != "main.js" {
			return false
		}
		relative, err := filepath.Rel(root.Dir, path)
		return err == nil && !strings.Contains(relative, string(filepath.Separator))
	case "Rust":
		relative, err := filepath.Rel(root.Dir, path)
		return err == nil && relative == filepath.Join("src", "main.rs")
	case "Java":
		if !strings.HasSuffix(name, ".java") {
			return false
		}
		content, err := readMetadataFile(path)
		return err == nil && strings.Contains(string(content), "public static void main(")
	default:
		return false
	}
}

func isSkipDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "vendor", ".venv", "venv", "__pycache__", "dist", "build", "target", "bin", ".github", ".idea", ".vscode", "coverage", ".nyc_output":
		return true
	default:
		return false
	}
}
