package repostructure

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func enrichBuildRoots(ctx context.Context, structure *Structure, roots []BuildRoot) {
	for index := range roots {
		if err := ctx.Err(); err != nil {
			structure.warn("build enrichment stopped: %v", err)
			return
		}
		roots[index].DockerImage = detectDockerImage(roots[index])
		roots[index].NativeDeps = detectNativeDeps(ctx, structure, roots[index])
	}
}

func detectDockerImage(root BuildRoot) string {
	switch root.Tool {
	case "cargo":
		return detectRustImage(root.Dir)
	case "npm", "yarn", "pnpm":
		return detectNodeImage(root.Dir)
	case "go":
		return detectGoImage(root.Dir)
	case "pip", "poetry":
		return detectPythonImage(root.Dir)
	case "maven":
		return "maven:3-eclipse-temurin-" + detectJavaMajor(root.Dir)
	case "gradle":
		return "eclipse-temurin:" + detectJavaMajor(root.Dir) + "-jdk-alpine"
	case "bundler":
		return detectRubyImage(root.Dir)
	case "composer":
		return detectPHPImage(root.Dir)
	case "swift-pm":
		return detectSwiftImage(root.Dir)
	case "dotnet":
		return detectDotnetImage(root.Dir)
	default:
		return ""
	}
}

func detectNativeDeps(ctx context.Context, structure *Structure, root BuildRoot) []NativeDependency {
	if root.Tool == "cargo" {
		return detectRustNativeDeps(ctx, structure, root.Dir)
	}
	return nil
}

func detectRustImage(directory string) string {
	if data, err := readMetadataFile(filepath.Join(directory, "rust-toolchain.toml")); err == nil {
		if match := regexp.MustCompile(`channel\s*=\s*"([^"]+)"`).FindSubmatch(data); len(match) > 1 {
			return rustChannelImage(string(match[1]))
		}
	}
	if data, err := readMetadataFile(filepath.Join(directory, "rust-toolchain")); err == nil {
		if channel := strings.TrimSpace(string(data)); channel != "" {
			return rustChannelImage(channel)
		}
	}
	return "rust:slim"
}

func rustChannelImage(channel string) string {
	switch channel {
	case "stable":
		return "rust:slim"
	case "nightly":
		return "rust:nightly-slim"
	case "beta":
		return "rust:beta-slim"
	default:
		if version := regexp.MustCompile(`^(\d+\.\d+)`).FindString(channel); version != "" {
			return "rust:" + version + "-slim"
		}
		return "rust:slim"
	}
}

func detectRustNativeDeps(ctx context.Context, structure *Structure, directory string) []NativeDependency {
	aptNeeds := make(map[string]bool)
	needsZig := false
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			structure.warn("inspect native dependency path %s: %v", path, walkErr)
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if path != directory && isSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		name := entry.Name()
		switch {
		case name == "build.zig" || strings.EqualFold(filepath.Ext(name), ".zig"):
			needsZig = true
		case name == "CMakeLists.txt" || strings.EqualFold(filepath.Ext(name), ".cmake"):
			aptNeeds["cmake"] = true
		case strings.EqualFold(filepath.Ext(name), ".proto"):
			aptNeeds["protobuf-compiler"] = true
		case strings.EqualFold(filepath.Ext(name), ".asm") || strings.EqualFold(filepath.Ext(name), ".nasm"):
			aptNeeds["nasm"] = true
		case name == "build.rs":
			data, readErr := readMetadataFile(path)
			if readErr != nil {
				structure.warn("inspect native dependency file %s: %v", path, readErr)
				return nil
			}
			content := string(data)
			if strings.Contains(content, "bindgen") {
				aptNeeds["clang"] = true
				aptNeeds["libclang-dev"] = true
			}
			if strings.Contains(content, `"cmake"`) || strings.Contains(content, "cmake::build") {
				aptNeeds["cmake"] = true
			}
			if strings.Contains(content, `"nasm"`) || strings.Contains(content, `"yasm"`) {
				aptNeeds["nasm"] = true
			}
			if strings.Contains(content, "pkg_config") || strings.Contains(content, "pkg-config") {
				aptNeeds["pkg-config"] = true
			}
			if strings.Contains(content, `"protoc"`) || strings.Contains(content, "prost_build") {
				aptNeeds["protobuf-compiler"] = true
			}
		}
		return nil
	})
	if err != nil {
		structure.warn("native dependency detection stopped: %v", err)
	}

	var dependencies []NativeDependency
	if needsZig {
		dependencies = append(dependencies, NativeDependency{
			Name:    "zig",
			Version: detectToolVersion(ctx, directory, "build.zig.zon", `\.minimum_zig_version\s*=\s*"([^"]+)"`, "0.13.0"),
		})
	}
	names := make([]string, 0, len(aptNeeds))
	for name := range aptNeeds {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dependencies = append(dependencies, NativeDependency{Name: name})
	}
	return dependencies
}

func detectToolVersion(ctx context.Context, directory, filename, expression, fallback string) string {
	compiled := regexp.MustCompile(expression)
	var found string
	_ = filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || found != "" {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if path != directory && isSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.Name() != filename {
			return nil
		}
		data, err := readMetadataFile(path)
		if err != nil {
			return nil
		}
		if match := compiled.FindSubmatch(data); len(match) > 1 {
			found = string(match[1])
		}
		return nil
	})
	if found == "" {
		return fallback
	}
	return found
}

func detectNodeImage(directory string) string {
	for _, filename := range []string{".node-version", ".nvmrc"} {
		if data, err := readMetadataFile(filepath.Join(directory, filename)); err == nil {
			return nodeVersionImage(strings.TrimSpace(string(data)))
		}
	}
	return "node:lts-alpine"
}

func nodeVersionImage(version string) string {
	version = strings.TrimPrefix(version, "v")
	if strings.HasPrefix(version, "lts") || version == "" {
		return "node:lts-alpine"
	}
	if major := regexp.MustCompile(`^(\d+)`).FindString(version); major != "" {
		return "node:" + major + "-alpine"
	}
	return "node:lts-alpine"
}

func detectPythonImage(directory string) string {
	if data, err := readMetadataFile(filepath.Join(directory, ".python-version")); err == nil {
		if version := regexp.MustCompile(`^(\d+\.\d+)`).FindString(strings.TrimSpace(string(data))); version != "" {
			return "python:" + version + "-slim"
		}
	}
	return "python:3.12-slim"
}

func detectGoImage(directory string) string {
	data, err := readMetadataFile(filepath.Join(directory, "go.mod"))
	if err != nil {
		return "golang:1.24-alpine"
	}
	if match := regexp.MustCompile(`(?m)^go\s+(\d+\.\d+)`).FindSubmatch(data); len(match) > 1 {
		return "golang:" + string(match[1]) + "-alpine"
	}
	return "golang:1.24-alpine"
}

func detectJavaMajor(directory string) string {
	if data, err := readMetadataFile(filepath.Join(directory, ".java-version")); err == nil {
		if major := regexp.MustCompile(`^(\d+)`).FindString(strings.TrimSpace(string(data))); major != "" {
			return major
		}
	}
	if data, err := readMetadataFile(filepath.Join(directory, "pom.xml")); err == nil {
		for _, expression := range []string{`<java\.version>(\d+)`, `<maven\.compiler\.source>(\d+)`} {
			if match := regexp.MustCompile(expression).FindSubmatch(data); len(match) > 1 {
				return string(match[1])
			}
		}
	}
	return "21"
}

func detectRubyImage(directory string) string {
	if data, err := readMetadataFile(filepath.Join(directory, ".ruby-version")); err == nil {
		if version := regexp.MustCompile(`^(\d+\.\d+)`).FindString(strings.TrimSpace(string(data))); version != "" {
			return "ruby:" + version + "-slim"
		}
	}
	if data, err := readMetadataFile(filepath.Join(directory, "Gemfile")); err == nil {
		if match := regexp.MustCompile(`(?m)^ruby\s+['"](\d+\.\d+)`).FindSubmatch(data); len(match) > 1 {
			return "ruby:" + string(match[1]) + "-slim"
		}
	}
	return "ruby:3.3-slim"
}

func detectPHPImage(directory string) string {
	if data, err := readMetadataFile(filepath.Join(directory, ".php-version")); err == nil {
		if version := regexp.MustCompile(`^(\d+\.\d+)`).FindString(strings.TrimSpace(string(data))); version != "" {
			return "php:" + version + "-cli-alpine"
		}
	}
	if data, err := readMetadataFile(filepath.Join(directory, "composer.json")); err == nil {
		var composer struct {
			Require map[string]string `json:"require"`
		}
		if json.Unmarshal(data, &composer) == nil {
			if requirement, ok := composer.Require["php"]; ok {
				if version := regexp.MustCompile(`(\d+\.\d+)`).FindString(requirement); version != "" {
					return "php:" + version + "-cli-alpine"
				}
			}
		}
	}
	return "php:8.3-cli-alpine"
}

func detectSwiftImage(directory string) string {
	if data, err := readMetadataFile(filepath.Join(directory, ".swift-version")); err == nil {
		if version := regexp.MustCompile(`^(\d+\.\d+)`).FindString(strings.TrimSpace(string(data))); version != "" {
			return "swift:" + version
		}
	}
	if data, err := readMetadataFile(filepath.Join(directory, "Package.swift")); err == nil {
		if match := regexp.MustCompile(`swift-tools-version:\s*(\d+\.\d+)`).FindSubmatch(data); len(match) > 1 {
			return "swift:" + string(match[1])
		}
	}
	return "swift:6.0"
}

func detectDotnetImage(directory string) string {
	if data, err := readMetadataFile(filepath.Join(directory, "global.json")); err == nil {
		var global struct {
			SDK struct {
				Version string `json:"version"`
			} `json:"sdk"`
		}
		if json.Unmarshal(data, &global) == nil {
			if major := regexp.MustCompile(`^(\d+)`).FindString(global.SDK.Version); major != "" {
				return "mcr.microsoft.com/dotnet/sdk:" + major + ".0"
			}
		}
	}
	entries, _ := os.ReadDir(directory)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(entry.Name()), ".csproj") {
			continue
		}
		if data, err := readMetadataFile(filepath.Join(directory, entry.Name())); err == nil {
			if match := regexp.MustCompile(`<TargetFramework>net(\d+)`).FindSubmatch(data); len(match) > 1 {
				return "mcr.microsoft.com/dotnet/sdk:" + string(match[1]) + ".0"
			}
		}
	}
	return "mcr.microsoft.com/dotnet/sdk:9.0"
}
